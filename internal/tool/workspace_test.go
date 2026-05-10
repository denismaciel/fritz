package tool

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWorkspaceFileOperations(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(filepath.Join(other, "secret.txt"), filepath.Join(root, "leak")); err != nil {
			t.Fatal(err)
		}
	}

	ops := NewWorkspaceFileOperations(WorkspaceConfig{
		Root:          root,
		ReadOnlyPaths: []string{"AGENTS.md"},
	})

	if data, err := ops.ReadFile(filepath.Join(root, "ok.txt")); err != nil || string(data) != "ok" {
		t.Fatalf("ReadFile ok = %q, %v", data, err)
	}
	if _, err := ops.ReadFile(filepath.Join(other, "secret.txt")); err == nil {
		t.Fatal("expected absolute path outside workspace to fail")
	}
	if _, err := ops.ReadFile("../secret.txt"); err == nil {
		t.Fatal("expected relative escape to fail")
	}
	if runtime.GOOS != "windows" {
		if _, err := ops.ReadFile("leak"); err == nil {
			t.Fatal("expected symlink escape to fail")
		}
	}
	if err := ops.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("x"), 0o644); err == nil {
		t.Fatal("expected read-only path write to fail")
	}
}

func TestWorkspaceToolsRejectEscapes(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	ops := NewWorkspaceFileOperations(WorkspaceConfig{Root: root, ReadOnlyPaths: []string{"AGENTS.md"}})

	result, err := NewReadTool(root, DefaultReadMaxBytes, WithReadFileOperations(ops)).Run(context.Background(), Call{
		ID:   "call-1",
		Name: "read",
		Args: map[string]any{"path": filepath.Join(other, "secret.txt")},
	})
	if err == nil || !result.IsError {
		t.Fatalf("expected read outside workspace error, got err=%v result=%#v", err, result)
	}

	result, err = NewWriteTool(root, WithWriteFileOperations(ops)).Run(context.Background(), Call{
		ID:   "call-2",
		Name: "write",
		Args: map[string]any{"path": "AGENTS.md", "content": "mutated"},
	})
	if err == nil || !strings.Contains(result.Text(), "read-only") {
		t.Fatalf("expected read-only write error, got err=%v text=%q", err, result.Text())
	}
}
