package authstore

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"fritz/internal/provider"
)

func TestFileStorePutGetDeleteOAuth(t *testing.T) {
	store := NewFileStore(t.TempDir())
	expiry := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	if err := store.PutOAuth(provider.OpenAICodex, OAuthCredential{
		AccessToken:  "access",
		RefreshToken: "refresh",
		AccountID:    "acct_123",
		ExpiresAt:    expiry,
	}); err != nil {
		t.Fatalf("PutOAuth() error = %v", err)
	}

	got, ok, err := store.Get(provider.OpenAICodex)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("expected credential")
	}
	if got.Provider != provider.OpenAICodex {
		t.Fatalf("Provider = %q", got.Provider)
	}
	if got.OAuth == nil || got.OAuth.AccountID != "acct_123" || !got.OAuth.ExpiresAt.Equal(expiry) {
		t.Fatalf("OAuth = %#v", got.OAuth)
	}

	deleted, err := store.Delete(provider.OpenAICodex)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !deleted {
		t.Fatal("expected delete")
	}
	_, ok, err = store.Get(provider.OpenAICodex)
	if err != nil {
		t.Fatalf("Get() after delete error = %v", err)
	}
	if ok {
		t.Fatal("expected no credential after delete")
	}
}

func TestFileStorePutAPIKeyAndList(t *testing.T) {
	store := NewFileStore(t.TempDir())
	if err := store.PutAPIKey(provider.Gemini, "key-123"); err != nil {
		t.Fatalf("PutAPIKey() error = %v", err)
	}
	list, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() len = %d", len(list))
	}
	if list[0].Provider != provider.Gemini || list[0].Kind != "api_key" {
		t.Fatalf("List() = %#v", list)
	}
}

func TestFileStoreResolvePaths(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/state")
	paths := ResolvePaths("/tmp/work")
	if paths.File != filepath.Join("/tmp/state", "fritz", "workspaces", "--tmp--work", "auth.json") {
		t.Fatalf("File = %q", paths.File)
	}
	if paths.LockFile != filepath.Join("/tmp/state", "fritz", "workspaces", "--tmp--work", "auth.lock") {
		t.Fatalf("LockFile = %q", paths.LockFile)
	}
}

func TestFileStoreUpdateIsLockSafe(t *testing.T) {
	store := NewFileStore(t.TempDir())
	expiry := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	if err := store.PutOAuth(provider.OpenAICodex, OAuthCredential{
		AccessToken:  "access-0",
		RefreshToken: "refresh",
		AccountID:    "acct_123",
		ExpiresAt:    expiry,
	}); err != nil {
		t.Fatalf("PutOAuth() error = %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := store.Update(provider.OpenAICodex, func(current Credential, ok bool) (Credential, bool, error) {
				if !ok || current.OAuth == nil {
					t.Fatalf("missing credential in update: ok=%t current=%#v", ok, current)
				}
				next := *current.OAuth
				next.AccessToken += "x"
				return Credential{OAuth: &next}, true, nil
			}); err != nil {
				t.Errorf("Update() error = %v", err)
			}
		}()
	}
	wg.Wait()

	got, ok, err := store.Get(provider.OpenAICodex)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || got.OAuth == nil {
		t.Fatalf("Get() = %#v ok=%t", got, ok)
	}
	if got.OAuth.AccessToken != "access-0xxxxxxxx" {
		t.Fatalf("AccessToken = %q", got.OAuth.AccessToken)
	}
}

func TestMemoryStoreUpdateDelete(t *testing.T) {
	store := NewMemoryStore()
	if err := store.PutAPIKey(provider.Gemini, "key-1"); err != nil {
		t.Fatalf("PutAPIKey() error = %v", err)
	}
	if err := store.Update(provider.Gemini, func(current Credential, ok bool) (Credential, bool, error) {
		if !ok {
			t.Fatal("expected current credential")
		}
		current.APIKey = "key-2"
		return current, true, nil
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	got, ok, err := store.Get(provider.Gemini)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || got.APIKey != "key-2" {
		t.Fatalf("Get() = %#v ok=%t", got, ok)
	}
	deleted, err := store.Delete(provider.Gemini)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !deleted {
		t.Fatal("expected delete")
	}
}
