package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrdersMemoryDocsDeterministically(t *testing.T) {
	dir := t.TempDir()
	root := dir
	if err := os.MkdirAll(filepath.Join(root, "memory"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte("main"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory", "b.md"), []byte("b"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory", "a.md"), []byte("a"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory", "ignore.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	docs, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(docs) != 3 {
		t.Fatalf("docs = %#v", docs)
	}
	if docs[0].Content != "main" || docs[1].Content != "a" || docs[2].Content != "b" {
		t.Fatalf("docs = %#v", docs)
	}
}

func TestResolvePathsUsesWorkspaceRoot(t *testing.T) {
	dir := t.TempDir()
	paths := ResolvePaths(dir)
	if paths.Root != dir {
		t.Fatalf("Root = %q", paths.Root)
	}
	if paths.MainPath != filepath.Join(dir, "MEMORY.md") {
		t.Fatalf("MainPath = %q", paths.MainPath)
	}
	if paths.Dir != filepath.Join(dir, "memory") {
		t.Fatalf("Dir = %q", paths.Dir)
	}
}

func TestLoadMissingMemoryIsEmpty(t *testing.T) {
	docs, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("docs = %#v", docs)
	}
}
