package ingress

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fritz/internal/config"
)

func TestResolveStatePaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	cfg := config.Resolve(config.Sources{
		Defaults: config.DefaultSource(),
	})
	paths := ResolveStatePaths(dir, cfg)
	if paths.Root != config.DefaultWorkspaceGatewayRoot(dir) {
		t.Fatalf("Root = %q", paths.Root)
	}
	if paths.RoutingSessionMapPath != filepath.Join(config.DefaultWorkspaceGatewayRoot(dir), "routing", "session-map.json") {
		t.Fatalf("RoutingSessionMapPath = %q", paths.RoutingSessionMapPath)
	}
	if paths.TelegramAllowlistPath != filepath.Join(config.DefaultWorkspaceGatewayRoot(dir), "telegram", "allowlist.json") {
		t.Fatalf("TelegramAllowlistPath = %q", paths.TelegramAllowlistPath)
	}
	if paths.BindingsCurrentPath != filepath.Join(config.DefaultWorkspaceGatewayRoot(dir), "bindings", "current.json") {
		t.Fatalf("BindingsCurrentPath = %q", paths.BindingsCurrentPath)
	}
	if paths.HeartbeatStatePath != filepath.Join(config.DefaultWorkspaceGatewayRoot(dir), "heartbeat", "state.json") {
		t.Fatalf("HeartbeatStatePath = %q", paths.HeartbeatStatePath)
	}
}

func TestWriteJSONFileAtomicAndReadJSONFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	want := SessionMapFile{
		Version:  CurrentStoreVersion,
		Sessions: map[string]string{"a": "/tmp/a.jsonl"},
	}
	if err := WriteJSONFileAtomic(path, want); err != nil {
		t.Fatalf("WriteJSONFileAtomic() error = %v", err)
	}
	got, exists, err := ReadJSONFile(path, SessionMapFile{})
	if err != nil {
		t.Fatalf("ReadJSONFile() error = %v", err)
	}
	if !exists {
		t.Fatal("exists = false")
	}
	if got.Version != want.Version || got.Sessions["a"] != want.Sessions["a"] {
		t.Fatalf("got = %#v", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp") {
			t.Fatalf("temp file leaked: %q", entry.Name())
		}
	}
}

func TestEnsureLayoutWritesMetaAndBindings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	cfg := config.Resolve(config.Sources{
		Defaults: config.Source{
			Session: config.SessionConfigSource{
				Dir: filepath.Join(dir, ".fritz", "sessions"),
			},
		},
	})
	paths := ResolveStatePaths(dir, cfg)
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)
	if err := EnsureLayout(paths, now); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	meta, exists, err := ReadJSONFile(paths.MetaPath, MetaFile{})
	if err != nil {
		t.Fatalf("ReadJSONFile(meta) error = %v", err)
	}
	if !exists || meta.Version != CurrentStoreVersion || meta.CreatedAt == "" {
		t.Fatalf("meta = %#v", meta)
	}
	bindings, exists, err := ReadJSONFile(paths.BindingsCurrentPath, BindingsFile{})
	if err != nil {
		t.Fatalf("ReadJSONFile(bindings) error = %v", err)
	}
	if !exists || bindings.Version != CurrentStoreVersion || len(bindings.Bindings) != 0 {
		t.Fatalf("bindings = %#v", bindings)
	}
}

func TestResolveStatePathsUsesCustomSessionDirRoot(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Resolve(config.Sources{
		Defaults: config.Source{
			Session: config.SessionConfigSource{
				Dir: filepath.Join(dir, ".fritz", "sessions"),
			},
		},
	})
	paths := ResolveStatePaths(dir, cfg)
	if paths.Root != filepath.Join(dir, ".fritz", "gateway") {
		t.Fatalf("Root = %q", paths.Root)
	}
}
