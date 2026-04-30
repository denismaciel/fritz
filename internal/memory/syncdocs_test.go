package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncFilesystemDocumentsCreateUpdateDeleteRestore(t *testing.T) {
	root := t.TempDir()
	docPath := filepath.Join(root, "docs", "note.md")
	writeMemoryFile(t, docPath, "sqlite first")

	engine := newTestEngine(t, root)
	results, err := SyncFilesystemDocuments(context.Background(), root, engine)
	if err != nil {
		t.Fatalf("SyncFilesystemDocuments(create) error = %v", err)
	}
	if len(results) != 1 || results[0].Action != "created" {
		t.Fatalf("results = %#v", results)
	}

	hits, err := SearchDocuments(context.Background(), engine, "sqlite", 5)
	if err != nil {
		t.Fatalf("SearchDocuments(sqlite) error = %v", err)
	}
	if len(hits) != 1 || hits[0].Chunk.Path != "docs/note.md" {
		t.Fatalf("hits = %#v", hits)
	}

	writeMemoryFile(t, docPath, "duckdb second")
	results, err = SyncFilesystemDocuments(context.Background(), root, engine)
	if err != nil {
		t.Fatalf("SyncFilesystemDocuments(update) error = %v", err)
	}
	if len(results) != 1 || results[0].Action != "updated" {
		t.Fatalf("results = %#v", results)
	}
	hits, err = SearchDocuments(context.Background(), engine, "duckdb", 5)
	if err != nil {
		t.Fatalf("SearchDocuments(duckdb) error = %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %#v", hits)
	}

	if err := os.Remove(docPath); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	results, err = SyncFilesystemDocuments(context.Background(), root, engine)
	if err != nil {
		t.Fatalf("SyncFilesystemDocuments(delete) error = %v", err)
	}
	if len(results) != 1 || results[0].Action != "missing" {
		t.Fatalf("results = %#v", results)
	}
	hits, err = SearchDocuments(context.Background(), engine, "duckdb", 5)
	if err != nil {
		t.Fatalf("SearchDocuments(after delete) error = %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits = %#v", hits)
	}

	writeMemoryFile(t, docPath, "duckdb second")
	results, err = SyncFilesystemDocuments(context.Background(), root, engine)
	if err != nil {
		t.Fatalf("SyncFilesystemDocuments(restore) error = %v", err)
	}
	if len(results) != 1 || results[0].Action != "updated" {
		t.Fatalf("results = %#v", results)
	}
	hits, err = SearchDocuments(context.Background(), engine, "duckdb", 5)
	if err != nil {
		t.Fatalf("SearchDocuments(after restore) error = %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %#v", hits)
	}
}
