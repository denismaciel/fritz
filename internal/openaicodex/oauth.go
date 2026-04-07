package openaicodex

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"fritz/internal/authstore"
	"fritz/internal/config"
)

const (
	scope        = "openid profile email offline_access"
	jwtClaimPath = "https://api.openai.com/auth"
)

type OAuthConfig struct {
	AuthorizeURL string
	TokenURL     string
	ClientID     string
	RedirectURL  string
	Originator   string
	HTTPClient   *http.Client
	Now          func() time.Time
}

type AuthorizationFlow struct {
	State    string
	Verifier string
	URL      string
}

type AuthorizationInput struct {
	Code  string
	State string
}

type CallbackServer struct {
	server *http.Server
	ln     net.Listener
	path   string
	codeCh chan result
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

type result struct {
	code string
	err  error
}

func OAuthConfigFromRuntime(cfg config.Runtime) OAuthConfig {
	base := strings.TrimRight(cfg.OpenAICodexAuthBaseURL, "/")
	return OAuthConfig{
		AuthorizeURL: base + "/oauth/authorize",
		TokenURL:     base + "/oauth/token",
		ClientID:     cfg.OpenAICodexClientID,
		RedirectURL:  cfg.OpenAICodexRedirectURL,
		Originator:   cfg.OpenAICodexOriginator,
		HTTPClient:   http.DefaultClient,
		Now:          func() time.Time { return time.Now().UTC() },
	}
}

func CreateAuthorizationFlow(cfg OAuthConfig) (AuthorizationFlow, error) {
	verifier, err := randomBase64URL(32)
	if err != nil {
		return AuthorizationFlow{}, err
	}
	state, err := randomHex(16)
	if err != nil {
		return AuthorizationFlow{}, err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	u, err := url.Parse(cfg.AuthorizeURL)
	if err != nil {
		return AuthorizationFlow{}, err
	}
	query := u.Query()
	query.Set("response_type", "code")
	query.Set("client_id", cfg.ClientID)
	query.Set("redirect_uri", cfg.RedirectURL)
	query.Set("scope", scope)
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	query.Set("state", state)
	query.Set("id_token_add_organizations", "true")
	query.Set("codex_cli_simplified_flow", "true")
	query.Set("originator", firstNonEmpty(cfg.Originator, "fritz"))
	u.RawQuery = query.Encode()
	return AuthorizationFlow{State: state, Verifier: verifier, URL: u.String()}, nil
}

func ParseAuthorizationInput(input string) AuthorizationInput {
	value := strings.TrimSpace(input)
	if value == "" {
		return AuthorizationInput{}
	}
	if u, err := url.Parse(value); err == nil && u.Scheme != "" {
		return AuthorizationInput{
			Code:  strings.TrimSpace(u.Query().Get("code")),
			State: strings.TrimSpace(u.Query().Get("state")),
		}
	}
	if strings.Contains(value, "#") {
		parts := strings.SplitN(value, "#", 2)
		return AuthorizationInput{
			Code:  strings.TrimSpace(parts[0]),
			State: strings.TrimSpace(parts[1]),
		}
	}
	if strings.Contains(value, "code=") {
		query, err := url.ParseQuery(value)
		if err == nil {
			return AuthorizationInput{
				Code:  strings.TrimSpace(query.Get("code")),
				State: strings.TrimSpace(query.Get("state")),
			}
		}
	}
	return AuthorizationInput{Code: value}
}

func ValidateAuthorizationInput(input AuthorizationInput, expectedState string) (string, error) {
	if strings.TrimSpace(input.Code) == "" {
		return "", errors.New("missing authorization code")
	}
	if expectedState != "" && input.State != "" && input.State != expectedState {
		return "", errors.New("oauth state mismatch")
	}
	if expectedState != "" && input.State == "" {
		return "", errors.New("missing oauth state")
	}
	return input.Code, nil
}

func StartCallbackServer(cfg OAuthConfig, expectedState string) (*CallbackServer, error) {
	redirectURL, err := url.Parse(cfg.RedirectURL)
	if err != nil {
		return nil, err
	}
	if redirectURL.Host == "" {
		return nil, errors.New("redirect url must include host")
	}
	ln, err := net.Listen("tcp", redirectURL.Host)
	if err != nil {
		return nil, err
	}
	codeCh := make(chan result, 1)
	server := &CallbackServer{
		ln:     ln,
		path:   redirectURL.Path,
		codeCh: codeCh,
	}
	server.server = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != redirectURL.Path {
				http.NotFound(w, r)
				return
			}
			code, err := ValidateAuthorizationInput(AuthorizationInput{
				Code:  strings.TrimSpace(r.URL.Query().Get("code")),
				State: strings.TrimSpace(r.URL.Query().Get("state")),
			}, expectedState)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				select {
				case codeCh <- result{err: err}:
				default:
				}
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, "OpenAI Codex auth complete. You can close this window.")
			select {
			case codeCh <- result{code: code}:
			default:
			}
		}),
	}
	go func() {
		err := server.server.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case codeCh <- result{err: err}:
			default:
			}
		}
	}()
	return server, nil
}

func (s *CallbackServer) Wait(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-s.codeCh:
		return result.code, result.err
	}
}

func (s *CallbackServer) Close(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

func ExchangeAuthorizationCode(ctx context.Context, cfg OAuthConfig, code string, verifier string) (authstore.OAuthCredential, error) {
	token, err := exchangeToken(ctx, cfg, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {cfg.ClientID},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {cfg.RedirectURL},
	})
	if err != nil {
		return authstore.OAuthCredential{}, err
	}
	accountID, err := ExtractAccountID(token.AccessToken)
	if err != nil {
		return authstore.OAuthCredential{}, err
	}
	return authstore.OAuthCredential{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		AccountID:    accountID,
		ExpiresAt:    cfg.now().Add(time.Duration(token.ExpiresIn) * time.Second),
	}, nil
}

func RefreshAccessToken(ctx context.Context, cfg OAuthConfig, refreshToken string, fallbackAccountID string) (authstore.OAuthCredential, error) {
	token, err := exchangeToken(ctx, cfg, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {cfg.ClientID},
	})
	if err != nil {
		return authstore.OAuthCredential{}, err
	}
	accountID, err := ExtractAccountID(token.AccessToken)
	if err != nil {
		accountID = fallbackAccountID
	}
	if accountID == "" {
		return authstore.OAuthCredential{}, errors.New("missing chatgpt account id")
	}
	return authstore.OAuthCredential{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		AccountID:    accountID,
		ExpiresAt:    cfg.now().Add(time.Duration(token.ExpiresIn) * time.Second),
	}, nil
}

func ExtractAccountID(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("failed to extract account id from token")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.New("failed to extract account id from token")
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return "", errors.New("failed to extract account id from token")
	}
	claim, _ := payload[jwtClaimPath].(map[string]any)
	accountID, _ := claim["chatgpt_account_id"].(string)
	if strings.TrimSpace(accountID) == "" {
		return "", errors.New("failed to extract account id from token")
	}
	return accountID, nil
}

func exchangeToken(ctx context.Context, cfg OAuthConfig, values url.Values) (tokenResponse, error) {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return tokenResponse{}, fmt.Errorf("oauth token exchange failed: %s", strings.TrimSpace(string(body)))
	}
	var token tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return tokenResponse{}, err
	}
	if token.AccessToken == "" || token.RefreshToken == "" || token.ExpiresIn <= 0 {
		return tokenResponse{}, errors.New("oauth token response missing fields")
	}
	return token, nil
}

func (cfg OAuthConfig) now() time.Time {
	if cfg.Now != nil {
		return cfg.Now().UTC()
	}
	return time.Now().UTC()
}

func randomBase64URL(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
