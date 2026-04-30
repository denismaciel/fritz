package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"fritz/pkg/memorypalace"
	mpsqlite "fritz/pkg/memorypalace/sqlite"
)

func newTestEngine(t *testing.T, root string) *memorypalace.Engine {
	t.Helper()
	adapter, err := mpsqlite.Open(filepath.Join(root, "palace.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})
	engine, err := memorypalace.NewEngine(adapter, adapter, nil)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	if err := engine.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return engine
}

func writeMemoryFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func TestIndexDocumentsAndSearchDocuments(t *testing.T) {
	root := t.TempDir()
	writeMemoryFile(t, filepath.Join(root, "MEMORY.md"), "Uses sqlite and gemini embeddings.")
	writeMemoryFile(t, filepath.Join(root, "memory", "2026-04-15.md"), "Plan: add vector search.")

	engine := newTestEngine(t, root)
	if err := IndexDocuments(context.Background(), root, engine); err != nil {
		t.Fatalf("IndexDocuments() error = %v", err)
	}

	hits, err := SearchDocuments(context.Background(), engine, "sqlite", 5)
	if err != nil {
		t.Fatalf("SearchDocuments() error = %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("hits = %#v", hits)
	}
	if hits[0].Chunk.Path != "MEMORY.md" {
		t.Fatalf("hit = %#v", hits[0])
	}
}

func TestIndexDocumentsUpdatesChangedContentAndTombstonesRemovedFiles(t *testing.T) {
	root := t.TempDir()
	mainPath := filepath.Join(root, "MEMORY.md")
	journalPath := filepath.Join(root, "memory", "2026-04-15.md")
	writeMemoryFile(t, mainPath, "Uses sqlite and gemini embeddings.")
	writeMemoryFile(t, journalPath, "Plan: add vector search.")

	engine := newTestEngine(t, root)
	if err := IndexDocuments(context.Background(), root, engine); err != nil {
		t.Fatalf("IndexDocuments(first) error = %v", err)
	}

	writeMemoryFile(t, mainPath, "Uses duckdb and gemini embeddings.")
	if err := os.Remove(journalPath); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if err := IndexDocuments(context.Background(), root, engine); err != nil {
		t.Fatalf("IndexDocuments(second) error = %v", err)
	}

	duckHits, err := SearchDocuments(context.Background(), engine, "duckdb", 5)
	if err != nil {
		t.Fatalf("SearchDocuments(duckdb) error = %v", err)
	}
	if len(duckHits) == 0 || duckHits[0].Chunk.Path != "MEMORY.md" {
		t.Fatalf("duck hits = %#v", duckHits)
	}

	vectorHits, err := SearchDocuments(context.Background(), engine, "vector", 5)
	if err != nil {
		t.Fatalf("SearchDocuments(vector) error = %v", err)
	}
	if len(vectorHits) != 0 {
		t.Fatalf("vector hits = %#v", vectorHits)
	}
}
