package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"fritz/internal/authstore"
	"fritz/internal/brand"
	"fritz/internal/config"
	"fritz/internal/openaicodex"
	"fritz/internal/provider"
)

const refreshLeadTime = time.Minute

type Resolver struct {
	Store          authstore.Store
	Now            func() time.Time
	RefreshCodexFn func(context.Context, openaicodex.OAuthConfig, string, string) (authstore.OAuthCredential, error)
}

func NewResolver(store authstore.Store) Resolver {
	return Resolver{
		Store:          store,
		Now:            func() time.Time { return time.Now().UTC() },
		RefreshCodexFn: openaicodex.RefreshAccessToken,
	}
}

func (r Resolver) Resolve(ctx context.Context, cfg config.Runtime) (provider.RequestAuth, error) {
	switch cfg.Provider {
	case provider.Gemini:
		if !cfg.HasGeminiAPIKey() {
			return provider.RequestAuth{}, errors.New("missing GEMINI_API_KEY")
		}
		return provider.RequestAuth{APIKey: cfg.GeminiAPIKey}, nil
	case provider.OpenAICodex:
		if r.Store == nil {
			return provider.RequestAuth{}, errors.New("missing auth store")
		}
		var resolved provider.RequestAuth
		err := r.Store.Update(provider.OpenAICodex, func(current authstore.Credential, ok bool) (authstore.Credential, bool, error) {
			if !ok || current.OAuth == nil {
				return authstore.Credential{}, false, fmt.Errorf("missing openai-codex auth; run `%s auth login openai-codex`", brand.CLIName)
			}
			next := current
			if r.needsRefresh(current.OAuth.ExpiresAt) {
				if r.RefreshCodexFn == nil {
					return authstore.Credential{}, false, errors.New("missing openai-codex refresher")
				}
				refreshed, err := r.RefreshCodexFn(ctx, openaicodex.OAuthConfigFromRuntime(cfg), current.OAuth.RefreshToken, current.OAuth.AccountID)
				if err != nil {
					return authstore.Credential{}, false, fmt.Errorf("openai-codex auth refresh failed: %w", err)
				}
				next.OAuth = &refreshed
				next.UpdatedAt = r.now()
			}
			resolved = provider.RequestAuth{
				BearerToken: next.OAuth.AccessToken,
				AccountID:   next.OAuth.AccountID,
				Headers: map[string]string{
					"chatgpt-account-id": next.OAuth.AccountID,
					"originator":         cfg.OpenAICodexOriginator,
					"OpenAI-Beta":        "responses=experimental",
				},
			}
			return next, true, nil
		})
		if err != nil {
			return provider.RequestAuth{}, err
		}
		return resolved, nil
	default:
		return provider.RequestAuth{}, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}
}

func (r Resolver) needsRefresh(expiresAt time.Time) bool {
	return !expiresAt.After(r.now().Add(refreshLeadTime))
}

func (r Resolver) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}
