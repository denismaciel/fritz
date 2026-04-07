package openaicodex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCreateAuthorizationFlow(t *testing.T) {
	flow, err := CreateAuthorizationFlow(OAuthConfig{
		AuthorizeURL: "https://auth.openai.test/oauth/authorize",
		ClientID:     "client-123",
		RedirectURL:  "http://localhost:1455/auth/callback",
		Originator:   "fritz-test",
	})
	if err != nil {
		t.Fatalf("CreateAuthorizationFlow() error = %v", err)
	}
	if flow.State == "" || flow.Verifier == "" {
		t.Fatalf("flow = %#v", flow)
	}
	if !strings.Contains(flow.URL, "client_id=client-123") || !strings.Contains(flow.URL, "originator=fritz-test") {
		t.Fatalf("URL = %q", flow.URL)
	}
}

func TestParseAuthorizationInput(t *testing.T) {
	input := ParseAuthorizationInput("http://localhost:1455/auth/callback?code=abc&state=xyz")
	if input.Code != "abc" || input.State != "xyz" {
		t.Fatalf("input = %#v", input)
	}
	input = ParseAuthorizationInput("abc#xyz")
	if input.Code != "abc" || input.State != "xyz" {
		t.Fatalf("hash input = %#v", input)
	}
	input = ParseAuthorizationInput("just-code")
	if input.Code != "just-code" {
		t.Fatalf("raw input = %#v", input)
	}
}

func TestValidateAuthorizationInput(t *testing.T) {
	if _, err := ValidateAuthorizationInput(AuthorizationInput{Code: "abc", State: "xyz"}, "xyz"); err != nil {
		t.Fatalf("ValidateAuthorizationInput() error = %v", err)
	}
	if _, err := ValidateAuthorizationInput(AuthorizationInput{Code: "abc", State: "bad"}, "xyz"); err == nil {
		t.Fatal("expected state mismatch")
	}
	if _, err := ValidateAuthorizationInput(AuthorizationInput{Code: "abc"}, "xyz"); err == nil {
		t.Fatal("expected missing state")
	}
}

func TestExtractAccountID(t *testing.T) {
	token := fakeJWT(map[string]any{
		jwtClaimPath: map[string]any{
			"chatgpt_account_id": "acct_123",
		},
	})
	accountID, err := ExtractAccountID(token)
	if err != nil {
		t.Fatalf("ExtractAccountID() error = %v", err)
	}
	if accountID != "acct_123" {
		t.Fatalf("accountID = %q", accountID)
	}
}

func TestExchangeAuthorizationCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if r.Form.Get("grant_type") != "authorization_code" {
			t.Fatalf("grant_type = %q", r.Form.Get("grant_type"))
		}
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"refresh","expires_in":60}`, fakeJWT(map[string]any{
			jwtClaimPath: map[string]any{"chatgpt_account_id": "acct_123"},
		}))
	}))
	defer server.Close()

	creds, err := ExchangeAuthorizationCode(context.Background(), OAuthConfig{
		TokenURL:    server.URL,
		ClientID:    "client-123",
		RedirectURL: "http://localhost:1455/auth/callback",
		HTTPClient:  server.Client(),
		Now: func() time.Time {
			return time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
		},
	}, "code-123", "verifier-123")
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v", err)
	}
	if creds.AccountID != "acct_123" || creds.RefreshToken != "refresh" {
		t.Fatalf("creds = %#v", creds)
	}
	if !creds.ExpiresAt.Equal(time.Date(2026, 4, 6, 12, 1, 0, 0, time.UTC)) {
		t.Fatalf("ExpiresAt = %s", creds.ExpiresAt)
	}
}

func TestRefreshAccessTokenFallsBackAccountID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"access_token":"a.b.c","refresh_token":"refresh-2","expires_in":120}`)
	}))
	defer server.Close()

	creds, err := RefreshAccessToken(context.Background(), OAuthConfig{
		TokenURL:   server.URL,
		ClientID:   "client-123",
		HTTPClient: server.Client(),
	}, "refresh-1", "acct_fallback")
	if err != nil {
		t.Fatalf("RefreshAccessToken() error = %v", err)
	}
	if creds.AccountID != "acct_fallback" {
		t.Fatalf("AccountID = %q", creds.AccountID)
	}
}

func fakeJWT(payload map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	data, _ := json.Marshal(payload)
	body := base64.RawURLEncoding.EncodeToString(data)
	return header + "." + body + ".sig"
}
