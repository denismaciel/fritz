package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"fritz/pkg/memorypalace"
)

func openTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	path := filepath.Join(t.TempDir(), "palace.db")
	adapter, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := adapter.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})
	return adapter
}

func TestAdapterReplaceSourceDocumentAndKeywordSearch(t *testing.T) {
	adapter := openTestAdapter(t)
	now := time.Unix(10, 0).UTC()
	src := memorypalace.Source{
		ID:          "source-1",
		Kind:        "document",
		Scope:       "workspace",
		Path:        "notes/note.md",
		Title:       "note.md",
		ExternalRef: "/tmp/note.md",
		ContentHash: "abc",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	entries := []memorypalace.Entry{{
		ID:          "entry-1",
		SourceID:    src.ID,
		Seq:         0,
		Kind:        "document_body",
		Text:        "prefers sqlite with fts",
		ContentHash: "abc",
		EventAt:     now,
		CreatedAt:   now,
	}}
	chunks := []memorypalace.Chunk{{
		ID:        "chunk-1",
		SourceID:  src.ID,
		Ordinal:   0,
		Kind:      "document_span",
		Wing:      "durable-memory",
		Room:      "general",
		Path:      "notes/note.md",
		Content:   "prefers sqlite with fts",
		StartSeq:  0,
		EndSeq:    0,
		CreatedAt: now,
		UpdatedAt: now,
	}}
	links := []memorypalace.ChunkEntry{{ChunkID: "chunk-1", EntryID: "entry-1", Ordinal: 0}}
	sync := memorypalace.SourceSync{
		SourceID:      src.ID,
		SyncKind:      "filesystem",
		ExternalRef:   "/tmp/note.md",
		LastSeenAt:    now,
		LastScannedAt: now,
		ContentHash:   "abc",
		Status:        memorypalace.SyncStatusActive,
	}
	if err := adapter.ReplaceSourceDocument(context.Background(), src, entries, chunks, links, sync); err != nil {
		t.Fatalf("ReplaceSourceDocument() error = %v", err)
	}

	gotSource, err := adapter.GetSource(context.Background(), src.ID)
	if err != nil {
		t.Fatalf("GetSource() error = %v", err)
	}
	if gotSource.ExternalRef != src.ExternalRef || gotSource.Scope != src.Scope {
		t.Fatalf("source = %#v", gotSource)
	}

	gotEntries, err := adapter.ListEntries(context.Background(), src.ID)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	if len(gotEntries) != 1 || gotEntries[0].Text != "prefers sqlite with fts" {
		t.Fatalf("entries = %#v", gotEntries)
	}

	gotSync, err := adapter.GetSourceSyncByExternalRef(context.Background(), "filesystem", "/tmp/note.md")
	if err != nil {
		t.Fatalf("GetSourceSyncByExternalRef() error = %v", err)
	}
	if gotSync.Status != memorypalace.SyncStatusActive {
		t.Fatalf("sync = %#v", gotSync)
	}

	gotChunk, err := adapter.GetChunk(context.Background(), "chunk-1")
	if err != nil {
		t.Fatalf("GetChunk() error = %v", err)
	}
	if gotChunk.Content != "prefers sqlite with fts" || gotChunk.Kind != "document_span" {
		t.Fatalf("chunk = %#v", gotChunk)
	}

	hits, err := adapter.Search(context.Background(), memorypalace.SearchRequest{
		Query: "sqlite",
		Mode:  memorypalace.SearchModeKeyword,
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(hits) != 1 || hits[0].Chunk.ID != "chunk-1" {
		t.Fatalf("hits = %#v", hits)
	}
}

func TestAdapterMarkSourceMissingExcludesDefaultSearch(t *testing.T) {
	adapter := openTestAdapter(t)
	now := time.Unix(10, 0).UTC()
	src := memorypalace.Source{
		ID:          "source-1",
		Kind:        "document",
		Path:        "note.md",
		Title:       "note.md",
		ExternalRef: "/tmp/note.md",
		ContentHash: "abc",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	entries := []memorypalace.Entry{{
		ID:          "entry-1",
		SourceID:    src.ID,
		Seq:         0,
		Kind:        "document_body",
		Text:        "prefers sqlite with fts",
		ContentHash: "abc",
		EventAt:     now,
		CreatedAt:   now,
	}}
	chunks := []memorypalace.Chunk{{
		ID:        "chunk-1",
		SourceID:  src.ID,
		Ordinal:   0,
		Kind:      "document_span",
		Wing:      "documents",
		Room:      "general",
		Path:      "note.md",
		Content:   "prefers sqlite with fts",
		StartSeq:  0,
		EndSeq:    0,
		CreatedAt: now,
		UpdatedAt: now,
	}}
	if err := adapter.ReplaceSourceDocument(context.Background(), src, entries, chunks, nil, memorypalace.SourceSync{
		SourceID:      src.ID,
		SyncKind:      "filesystem",
		ExternalRef:   "/tmp/note.md",
		LastSeenAt:    now,
		LastScannedAt: now,
		ContentHash:   "abc",
		Status:        memorypalace.SyncStatusActive,
	}); err != nil {
		t.Fatalf("ReplaceSourceDocument() error = %v", err)
	}
	if err := adapter.MarkSourceMissing(context.Background(), "filesystem", "/tmp/note.md", now.Add(time.Minute)); err != nil {
		t.Fatalf("MarkSourceMissing() error = %v", err)
	}

	hits, err := adapter.Search(context.Background(), memorypalace.SearchRequest{
		Query: "sqlite",
		Mode:  memorypalace.SearchModeKeyword,
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits = %#v", hits)
	}

	hits, err = adapter.Search(context.Background(), memorypalace.SearchRequest{
		Query: "sqlite",
		Mode:  memorypalace.SearchModeKeyword,
		Limit: 3,
		Filter: memorypalace.SearchFilter{
			IncludeInactive: true,
		},
	})
	if err != nil {
		t.Fatalf("Search(include inactive) error = %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %#v", hits)
	}
}

func TestAdapterDeleteSourceRemovesRows(t *testing.T) {
	adapter := openTestAdapter(t)
	now := time.Unix(10, 0).UTC()
	src := memorypalace.Source{
		ID:          "source-1",
		Kind:        "document",
		Path:        "note.md",
		Title:       "note.md",
		ExternalRef: "/tmp/note.md",
		ContentHash: "abc",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := adapter.ReplaceSourceDocument(context.Background(), src, []memorypalace.Entry{{
		ID:          "entry-1",
		SourceID:    src.ID,
		Seq:         0,
		Kind:        "document_body",
		Text:        "hello",
		ContentHash: "abc",
		EventAt:     now,
		CreatedAt:   now,
	}}, []memorypalace.Chunk{{
		ID:        "chunk-1",
		SourceID:  src.ID,
		Ordinal:   0,
		Kind:      "document_span",
		Wing:      "documents",
		Room:      "general",
		Path:      "note.md",
		Content:   "hello",
		CreatedAt: now,
		UpdatedAt: now,
	}}, nil, memorypalace.SourceSync{
		SourceID:      src.ID,
		SyncKind:      "filesystem",
		ExternalRef:   "/tmp/note.md",
		LastSeenAt:    now,
		LastScannedAt: now,
		ContentHash:   "abc",
		Status:        memorypalace.SyncStatusActive,
	}); err != nil {
		t.Fatalf("ReplaceSourceDocument() error = %v", err)
	}
	if err := adapter.DeleteSource(context.Background(), src.ID); err != nil {
		t.Fatalf("DeleteSource() error = %v", err)
	}
	_, err := adapter.GetChunk(context.Background(), "chunk-1")
	if !errors.Is(err, memorypalace.ErrNotFound) {
		t.Fatalf("GetChunk() error = %v, want ErrNotFound", err)
	}
	_, err = adapter.GetSourceSync(context.Background(), src.ID)
	if !errors.Is(err, memorypalace.ErrNotFound) {
		t.Fatalf("GetSourceSync() error = %v, want ErrNotFound", err)
	}
}

func TestAdapterVectorAndHybridSearch(t *testing.T) {
	adapter := openTestAdapter(t)
	if !adapter.caps.Vector {
		t.Skip("sqlite-vec unavailable in test env")
	}
	now := time.Unix(10, 0).UTC()
	src := memorypalace.Source{
		ID:          "source-1",
		Kind:        "document",
		Path:        "note.md",
		Title:       "note.md",
		ExternalRef: "/tmp/note.md",
		ContentHash: "abc",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	chunks := []memorypalace.Chunk{
		{ID: "chunk-1", SourceID: src.ID, Ordinal: 0, Kind: "document_span", Path: "note.md", Content: "sqlite search", Wing: "documents", Room: "general", CreatedAt: now, UpdatedAt: now},
		{ID: "chunk-2", SourceID: src.ID, Ordinal: 1, Kind: "document_span", Path: "note.md", Content: "duckdb batch", Wing: "documents", Room: "general", CreatedAt: now, UpdatedAt: now},
	}
	if err := adapter.ReplaceSourceDocument(context.Background(), src, []memorypalace.Entry{{
		ID:          "entry-1",
		SourceID:    src.ID,
		Seq:         0,
		Kind:        "document_body",
		Text:        "sqlite search\nduckdb batch",
		ContentHash: "abc",
		EventAt:     now,
		CreatedAt:   now,
	}}, chunks, []memorypalace.ChunkEntry{
		{ChunkID: "chunk-1", EntryID: "entry-1", Ordinal: 0},
		{ChunkID: "chunk-2", EntryID: "entry-1", Ordinal: 1},
	}, memorypalace.SourceSync{
		SourceID:      src.ID,
		SyncKind:      "filesystem",
		ExternalRef:   "/tmp/note.md",
		LastSeenAt:    now,
		LastScannedAt: now,
		ContentHash:   "abc",
		Status:        memorypalace.SyncStatusActive,
	}); err != nil {
		t.Fatalf("ReplaceSourceDocument() error = %v", err)
	}
	if err := adapter.Upsert(context.Background(), []memorypalace.IndexRecord{
		{Chunk: chunks[0], Vector: []float32{1, 0, 0}},
		{Chunk: chunks[1], Vector: []float32{0, 1, 0}},
	}); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	vectorHits, err := adapter.Search(context.Background(), memorypalace.SearchRequest{
		Query:       "sqlite",
		QueryVector: []float32{1, 0, 0},
		Mode:        memorypalace.SearchModeVector,
		Limit:       2,
	})
	if err != nil {
		t.Fatalf("vector Search() error = %v", err)
	}
	if len(vectorHits) == 0 || vectorHits[0].Chunk.ID != "chunk-1" {
		t.Fatalf("vector hits = %#v", vectorHits)
	}

	hybridHits, err := adapter.Search(context.Background(), memorypalace.SearchRequest{
		Query:       "sqlite",
		QueryVector: []float32{1, 0, 0},
		Mode:        memorypalace.SearchModeHybrid,
		Limit:       2,
	})
	if err != nil {
		t.Fatalf("hybrid Search() error = %v", err)
	}
	if len(hybridHits) == 0 || hybridHits[0].Chunk.ID != "chunk-1" {
		t.Fatalf("hybrid hits = %#v", hybridHits)
	}
}
