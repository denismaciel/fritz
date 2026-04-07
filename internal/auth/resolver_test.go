package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"fritz/internal/authstore"
	"fritz/internal/config"
	"fritz/internal/openaicodex"
	"fritz/internal/provider"
)

func TestResolverResolveGemini(t *testing.T) {
	cfg := config.Resolve(config.Sources{
		Defaults: config.DefaultSource(),
		Env:      config.Source{GeminiAPIKey: "key-123"},
	})
	auth, err := NewResolver(authstore.NewMemoryStore()).Resolve(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if auth.APIKey != "key-123" {
		t.Fatalf("APIKey = %q", auth.APIKey)
	}
}

func TestResolverResolveOpenAICodexMissing(t *testing.T) {
	cfg := config.Resolve(config.Sources{
		Defaults: config.DefaultSource(),
		Flags:    config.Source{Provider: "openai-codex"},
	})
	_, err := NewResolver(authstore.NewMemoryStore()).Resolve(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected missing auth")
	}
}

func TestResolverResolveOpenAICodexPresent(t *testing.T) {
	store := authstore.NewMemoryStore()
	if err := store.PutOAuth(provider.OpenAICodex, authstore.OAuthCredential{
		AccessToken:  "access",
		RefreshToken: "refresh",
		AccountID:    "acct_123",
		ExpiresAt:    time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("PutOAuth() error = %v", err)
	}
	resolver := NewResolver(store)
	resolver.Now = func() time.Time { return time.Date(2026, 4, 6, 11, 0, 0, 0, time.UTC) }
	cfg := config.Resolve(config.Sources{
		Defaults: config.DefaultSource(),
		Flags: config.Source{
			Provider:              "openai-codex",
			OpenAICodexOriginator: "fritz-test",
		},
	})
	auth, err := resolver.Resolve(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if auth.BearerToken != "access" || auth.AccountID != "acct_123" {
		t.Fatalf("auth = %#v", auth)
	}
	if auth.Headers["originator"] != "fritz-test" {
		t.Fatalf("Headers = %#v", auth.Headers)
	}
}

func TestResolverRefreshesExpiringOpenAICodex(t *testing.T) {
	store := authstore.NewMemoryStore()
	if err := store.PutOAuth(provider.OpenAICodex, authstore.OAuthCredential{
		AccessToken:  "access-old",
		RefreshToken: "refresh-old",
		AccountID:    "acct_123",
		ExpiresAt:    time.Date(2026, 4, 6, 12, 0, 30, 0, time.UTC),
	}); err != nil {
		t.Fatalf("PutOAuth() error = %v", err)
	}
	resolver := NewResolver(store)
	resolver.Now = func() time.Time { return time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC) }
	resolver.RefreshCodexFn = func(_ context.Context, _ openaicodex.OAuthConfig, refreshToken string, accountID string) (authstore.OAuthCredential, error) {
		if refreshToken != "refresh-old" || accountID != "acct_123" {
			t.Fatalf("refresh args = %q %q", refreshToken, accountID)
		}
		return authstore.OAuthCredential{
			AccessToken:  "access-new",
			RefreshToken: "refresh-new",
			AccountID:    "acct_123",
			ExpiresAt:    time.Date(2026, 4, 6, 13, 0, 0, 0, time.UTC),
		}, nil
	}
	cfg := config.Resolve(config.Sources{
		Defaults: config.DefaultSource(),
		Flags:    config.Source{Provider: "openai-codex"},
	})
	auth, err := resolver.Resolve(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if auth.BearerToken != "access-new" {
		t.Fatalf("BearerToken = %q", auth.BearerToken)
	}
	stored, ok, err := store.Get(provider.OpenAICodex)
	if err != nil || !ok || stored.OAuth == nil || stored.OAuth.AccessToken != "access-new" {
		t.Fatalf("stored = %#v ok=%t err=%v", stored, ok, err)
	}
}

func TestResolverRefreshError(t *testing.T) {
	store := authstore.NewMemoryStore()
	_ = store.PutOAuth(provider.OpenAICodex, authstore.OAuthCredential{
		AccessToken:  "access-old",
		RefreshToken: "refresh-old",
		AccountID:    "acct_123",
		ExpiresAt:    time.Date(2026, 4, 6, 12, 0, 30, 0, time.UTC),
	})
	resolver := NewResolver(store)
	resolver.Now = func() time.Time { return time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC) }
	resolver.RefreshCodexFn = func(context.Context, openaicodex.OAuthConfig, string, string) (authstore.OAuthCredential, error) {
		return authstore.OAuthCredential{}, errors.New("boom")
	}
	cfg := config.Resolve(config.Sources{
		Defaults: config.DefaultSource(),
		Flags:    config.Source{Provider: "openai-codex"},
	})
	if _, err := resolver.Resolve(context.Background(), cfg); err == nil {
		t.Fatal("expected refresh error")
	}
}
