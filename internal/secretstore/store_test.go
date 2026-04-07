package secretstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolvePaths(t *testing.T) {
	dir := t.TempDir()
	paths := ResolvePaths(dir)
	if paths.Root != filepath.Join(dir, ".fritz") {
		t.Fatalf("Root = %q", paths.Root)
	}
	if paths.File != filepath.Join(dir, ".fritz", "secrets.json") {
		t.Fatalf("File = %q", paths.File)
	}
}

func TestStoreRoundTripAndListRedaction(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	store.now = func() time.Time { return time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC) }

	if err := store.Set("strava.api_key", "secret-1"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := store.Set("github.token", "secret-2"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	entry, ok, err := store.Get("strava.api_key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || entry.Value != "secret-1" {
		t.Fatalf("Get() = %#v, %t", entry, ok)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 || list[0].Name != "github.token" || list[1].Name != "strava.api_key" {
		t.Fatalf("List() = %#v", list)
	}

	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestStoreOverwriteDeleteAndValidation(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)

	if err := store.Set("bad name", "x"); err == nil {
		t.Fatal("expected invalid name error")
	}
	if err := store.Set("strava.api_key", "one"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := store.Set("strava.api_key", "two"); err != nil {
		t.Fatalf("Set() overwrite error = %v", err)
	}
	entry, ok, err := store.Get("strava.api_key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || entry.Value != "two" {
		t.Fatalf("Get() = %#v, %t", entry, ok)
	}
	deleted, err := store.Delete("strava.api_key")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !deleted {
		t.Fatal("Delete() = false")
	}
	_, ok, err = store.Get("strava.api_key")
	if err != nil {
		t.Fatalf("Get() after delete error = %v", err)
	}
	if ok {
		t.Fatal("expected missing secret")
	}
}
