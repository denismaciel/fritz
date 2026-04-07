package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryLookupAndRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry := NewRegistry()
	registry.Register(NewReadTool(dir, 1024))
	registry.Register(NewWriteTool(dir))
	registry.Register(NewEditTool(dir, 1024))
	registry.Register(NewBashTool(dir))
	registry.Register(NewSecretSetTool(dir))
	registry.Register(NewSecretListTool(dir))
	registry.Register(NewSecretDeleteTool(dir))

	defs := registry.Definitions()
	if len(defs) != 7 {
		t.Fatalf("Definitions() = %#v", defs)
	}

	result, err := registry.Run(context.Background(), Call{
		ID:   "call-1",
		Name: "read",
		Args: map[string]any{"path": "note.txt"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Text() != "hello" {
		t.Fatalf("Content = %q", result.Text())
	}
}

func TestReadRejectsDirectoryAndMissingPath(t *testing.T) {
	dir := t.TempDir()
	registry := NewRegistry()
	registry.Register(NewReadTool(dir, 1024))

	tests := []Call{
		{ID: "a", Name: "read", Args: map[string]any{}},
		{ID: "b", Name: "read", Args: map[string]any{"path": "."}},
		{ID: "c", Name: "read", Args: map[string]any{"path": "missing.txt"}},
	}

	for _, call := range tests {
		result, err := registry.Run(context.Background(), call)
		if err == nil {
			t.Fatalf("expected error for %#v", call)
		}
		if !result.IsError {
			t.Fatalf("expected IsError for %#v", call)
		}
	}
}

func TestWriteCreatesFile(t *testing.T) {
	dir := t.TempDir()
	registry := NewRegistry()
	registry.Register(NewWriteTool(dir))

	result, err := registry.Run(context.Background(), Call{
		ID:   "call-1",
		Name: "write",
		Args: map[string]any{
			"path":    "notes/out.txt",
			"content": "written",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result = %#v", result)
	}

	data, err := os.ReadFile(filepath.Join(dir, "notes/out.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "written" {
		t.Fatalf("file = %q", string(data))
	}
}

func TestEditReplacesText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	if err := os.WriteFile(path, []byte("before old after"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry := NewRegistry()
	registry.Register(NewEditTool(dir, 1024))

	result, err := registry.Run(context.Background(), Call{
		ID:   "call-1",
		Name: "edit",
		Args: map[string]any{
			"path":     "edit.txt",
			"old_text": "old",
			"new_text": "new",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result = %#v", result)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "before new after" {
		t.Fatalf("file = %q", string(data))
	}
}

func TestBashRunsCommand(t *testing.T) {
	dir := t.TempDir()
	registry := NewRegistry()
	registry.Register(NewBashTool(dir))

	result, err := registry.Run(context.Background(), Call{
		ID:   "call-1",
		Name: "bash",
		Args: map[string]any{
			"command": "printf hello",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result = %#v", result)
	}
	if result.Text() != "hello" {
		t.Fatalf("content = %q", result.Text())
	}
}

func TestBashNonZeroIsError(t *testing.T) {
	dir := t.TempDir()
	registry := NewRegistry()
	registry.Register(NewBashTool(dir))

	result, err := registry.Run(context.Background(), Call{
		ID:   "call-1",
		Name: "bash",
		Args: map[string]any{
			"command": "printf nope && exit 7",
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !result.IsError {
		t.Fatalf("expected error result = %#v", result)
	}
	if result.Text() == "" {
		t.Fatalf("expected command output in error result")
	}
}

func TestSecretToolsStoreListAndDelete(t *testing.T) {
	dir := t.TempDir()
	registry := NewRegistry()
	registry.Register(NewSecretSetTool(dir))
	registry.Register(NewSecretListTool(dir))
	registry.Register(NewSecretDeleteTool(dir))

	result, err := registry.Run(context.Background(), Call{
		ID:   "set",
		Name: "secret_set",
		Args: map[string]any{"name": "strava.api_key", "value": "super-secret"},
	})
	if err != nil || result.IsError {
		t.Fatalf("secret_set error = %v %#v", err, result)
	}

	listResult, err := registry.Run(context.Background(), Call{
		ID:   "list",
		Name: "secret_list",
		Args: map[string]any{},
	})
	if err != nil || listResult.IsError {
		t.Fatalf("secret_list error = %v %#v", err, listResult)
	}
	if !strings.Contains(listResult.Text(), "strava.api_key") {
		t.Fatalf("list = %q", listResult.Text())
	}
	if strings.Contains(listResult.Text(), "super-secret") {
		t.Fatalf("list leaked secret value = %q", listResult.Text())
	}

	deleteResult, err := registry.Run(context.Background(), Call{
		ID:   "delete",
		Name: "secret_delete",
		Args: map[string]any{"name": "strava.api_key"},
	})
	if err != nil || deleteResult.IsError {
		t.Fatalf("secret_delete error = %v %#v", err, deleteResult)
	}
}
