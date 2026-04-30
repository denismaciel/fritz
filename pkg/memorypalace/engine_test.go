package memorypalace

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	sources map[string]Source
	syncs   map[string]SourceSync
	entries map[string][]Entry
	chunks  map[string][]Chunk
	links   map[string][]ChunkEntry
	deleted string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		sources: map[string]Source{},
		syncs:   map[string]SourceSync{},
		entries: map[string][]Entry{},
		chunks:  map[string][]Chunk{},
		links:   map[string][]ChunkEntry{},
	}
}

func (f *fakeStore) Migrate(context.Context) error { return nil }

func (f *fakeStore) UpsertSource(_ context.Context, src Source) error {
	f.sources[src.ID] = src
	return nil
}

func (f *fakeStore) GetSource(_ context.Context, sourceID string) (Source, error) {
	src, ok := f.sources[sourceID]
	if !ok {
		return Source{}, ErrNotFound
	}
	return src, nil
}

func (f *fakeStore) ReplaceSourceDocument(_ context.Context, src Source, entries []Entry, chunks []Chunk, links []ChunkEntry, sync SourceSync) error {
	f.sources[src.ID] = src
	f.entries[src.ID] = append([]Entry(nil), entries...)
	f.chunks[src.ID] = append([]Chunk(nil), chunks...)
	f.links[src.ID] = append([]ChunkEntry(nil), links...)
	if sync.SourceID != "" {
		f.syncs[src.ID] = sync
	}
	return nil
}

func (f *fakeStore) ReplaceEntries(_ context.Context, sourceID string, entries []Entry) error {
	f.entries[sourceID] = append([]Entry(nil), entries...)
	return nil
}

func (f *fakeStore) ReplaceChunks(_ context.Context, sourceID string, chunks []Chunk) error {
	f.chunks[sourceID] = append([]Chunk(nil), chunks...)
	return nil
}

func (f *fakeStore) ReplaceChunkEntries(_ context.Context, sourceID string, links []ChunkEntry) error {
	f.links[sourceID] = append([]ChunkEntry(nil), links...)
	return nil
}

func (f *fakeStore) ListEntries(_ context.Context, sourceID string) ([]Entry, error) {
	return append([]Entry(nil), f.entries[sourceID]...), nil
}

func (f *fakeStore) UpsertSourceSync(_ context.Context, sync SourceSync) error {
	f.syncs[sync.SourceID] = sync
	return nil
}

func (f *fakeStore) GetSourceSync(_ context.Context, sourceID string) (SourceSync, error) {
	sync, ok := f.syncs[sourceID]
	if !ok {
		return SourceSync{}, ErrNotFound
	}
	return sync, nil
}

func (f *fakeStore) GetSourceSyncByExternalRef(_ context.Context, syncKind string, externalRef string) (SourceSync, error) {
	for _, sync := range f.syncs {
		if sync.SyncKind == syncKind && sync.ExternalRef == externalRef {
			return sync, nil
		}
	}
	return SourceSync{}, ErrNotFound
}

func (f *fakeStore) ListSourceSyncByKind(_ context.Context, syncKind string) ([]SourceSync, error) {
	var out []SourceSync
	for _, sync := range f.syncs {
		if sync.SyncKind == syncKind {
			out = append(out, sync)
		}
	}
	return out, nil
}

func (f *fakeStore) MarkSourceMissing(_ context.Context, syncKind string, externalRef string, seenAt time.Time) error {
	for sourceID, sync := range f.syncs {
		if sync.SyncKind == syncKind && sync.ExternalRef == externalRef {
			sync.Status = SyncStatusMissing
			sync.LastSeenAt = seenAt
			sync.LastScannedAt = seenAt
			f.syncs[sourceID] = sync
			return nil
		}
	}
	return nil
}

func (f *fakeStore) DeleteSource(_ context.Context, sourceID string) error {
	delete(f.sources, sourceID)
	delete(f.syncs, sourceID)
	delete(f.entries, sourceID)
	delete(f.chunks, sourceID)
	delete(f.links, sourceID)
	f.deleted = sourceID
	return nil
}

func (f *fakeStore) GetChunk(_ context.Context, chunkID string) (Chunk, error) {
	for _, chunks := range f.chunks {
		for _, chunk := range chunks {
			if chunk.ID == chunkID {
				return chunk, nil
			}
		}
	}
	return Chunk{}, ErrNotFound
}

func (f *fakeStore) GetChunks(ctx context.Context, chunkIDs []string) ([]Chunk, error) {
	out := make([]Chunk, 0, len(chunkIDs))
	for _, chunkID := range chunkIDs {
		chunk, err := f.GetChunk(ctx, chunkID)
		if err == nil {
			out = append(out, chunk)
		}
	}
	return out, nil
}

func (f *fakeStore) ListBySource(_ context.Context, sourceID string) ([]Chunk, error) {
	return append([]Chunk(nil), f.chunks[sourceID]...), nil
}

type fakeIndex struct {
	deletedBySource string
	upserted        []IndexRecord
	searchReq       SearchRequest
	searchHits      []SearchHit
	caps            Capabilities
	upsertErr       error
}

func (f *fakeIndex) Migrate(context.Context) error { return nil }

func (f *fakeIndex) DeleteBySource(_ context.Context, sourceID string) error {
	f.deletedBySource = sourceID
	return nil
}

func (f *fakeIndex) Upsert(_ context.Context, records []IndexRecord) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.upserted = append([]IndexRecord(nil), records...)
	return nil
}

func (f *fakeIndex) Search(_ context.Context, req SearchRequest) ([]SearchHit, error) {
	f.searchReq = req
	return append([]SearchHit(nil), f.searchHits...), nil
}

func (f *fakeIndex) Capabilities(context.Context) (Capabilities, error) { return f.caps, nil }

type fakeEmbedder struct {
	vectors [][]float32
	err     error
}

func (f fakeEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.vectors, nil
}

func (f fakeEmbedder) Dim() int     { return 3 }
func (f fakeEmbedder) Name() string { return "fake" }

func TestEngineIndexSourceEmbedsAndUpserts(t *testing.T) {
	store := newFakeStore()
	index := &fakeIndex{}
	engine, err := NewEngine(store, index, fakeEmbedder{
		vectors: [][]float32{{1, 2, 3}},
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	src := Source{ID: "src-1", Kind: "memory", Path: "MEMORY.md", Title: "Memory"}
	chunks := []Chunk{{ID: "chunk-1", Content: "hello"}}
	if err := engine.IndexSource(context.Background(), src, chunks); err != nil {
		t.Fatalf("IndexSource() error = %v", err)
	}
	if store.sources["src-1"].ID != "src-1" {
		t.Fatalf("source = %#v", store.sources["src-1"])
	}
	if len(index.upserted) != 1 {
		t.Fatalf("upserted = %#v", index.upserted)
	}
	if got := index.upserted[0].Vector; len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("vector = %#v", got)
	}
}

func TestEngineSyncDocumentCreateUpdateUnchanged(t *testing.T) {
	store := newFakeStore()
	index := &fakeIndex{}
	engine, err := NewEngine(store, index, fakeEmbedder{
		vectors: [][]float32{{1, 2, 3}},
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	req := SyncSource{
		SyncKind:    "filesystem",
		ExternalRef: "/tmp/note.md",
		Path:        "note.md",
		Title:       "note.md",
		SourceKind:  "document",
		Content:     "hello sqlite world",
		SeenAt:      time.Unix(10, 0).UTC(),
	}
	created, err := engine.SyncDocument(context.Background(), req)
	if err != nil {
		t.Fatalf("SyncDocument(create) error = %v", err)
	}
	if created.Action != "created" {
		t.Fatalf("create action = %#v", created)
	}
	if len(store.entries[created.SourceID]) != 1 || len(store.chunks[created.SourceID]) != 1 {
		t.Fatalf("store state = %#v %#v", store.entries, store.chunks)
	}

	unchanged, err := engine.SyncDocument(context.Background(), req)
	if err != nil {
		t.Fatalf("SyncDocument(unchanged) error = %v", err)
	}
	if unchanged.Action != "unchanged" {
		t.Fatalf("unchanged action = %#v", unchanged)
	}

	req.Content = "hello sqlite world updated"
	updated, err := engine.SyncDocument(context.Background(), req)
	if err != nil {
		t.Fatalf("SyncDocument(updated) error = %v", err)
	}
	if updated.Action != "updated" {
		t.Fatalf("updated action = %#v", updated)
	}
	if got := store.chunks[updated.SourceID][0].Content; got != "hello sqlite world updated" {
		t.Fatalf("chunk content = %q", got)
	}
}

func TestEngineSyncDocumentKeepsKeywordRowsOnVectorFailure(t *testing.T) {
	store := newFakeStore()
	index := &fakeIndex{upsertErr: errors.New("vector upsert failed")}
	engine, err := NewEngine(store, index, fakeEmbedder{
		vectors: [][]float32{{1, 2, 3}},
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	got, err := engine.SyncDocument(context.Background(), SyncSource{
		SyncKind:    "filesystem",
		ExternalRef: "/tmp/note.md",
		Path:        "note.md",
		SourceKind:  "document",
		Content:     "hello sqlite world",
	})
	if err != nil {
		t.Fatalf("SyncDocument() error = %v", err)
	}
	if got.IndexError == "" {
		t.Fatalf("IndexError = %q", got.IndexError)
	}
	if len(store.chunks[got.SourceID]) == 0 {
		t.Fatalf("chunks missing after vector failure")
	}
	sync, err := store.GetSourceSync(context.Background(), got.SourceID)
	if err != nil {
		t.Fatalf("GetSourceSync() error = %v", err)
	}
	if sync.LastError == "" {
		t.Fatalf("sync = %#v", sync)
	}
}

func TestEngineRebuildSource(t *testing.T) {
	store := newFakeStore()
	index := &fakeIndex{}
	engine, err := NewEngine(store, index, nil)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	created, err := engine.SyncDocument(context.Background(), SyncSource{
		SyncKind:    "filesystem",
		ExternalRef: "/tmp/note.md",
		Path:        "note.md",
		SourceKind:  "document",
		Content:     "hello sqlite world",
	})
	if err != nil {
		t.Fatalf("SyncDocument() error = %v", err)
	}
	store.entries[created.SourceID][0].Text = "rebuilt world"
	if err := engine.RebuildSource(context.Background(), created.SourceID); err != nil {
		t.Fatalf("RebuildSource() error = %v", err)
	}
	if got := store.chunks[created.SourceID][0].Content; got != "rebuilt world" {
		t.Fatalf("rebuilt chunk = %q", got)
	}
}

func TestEngineSearchHybridFallsBackWithoutEmbedder(t *testing.T) {
	index := &fakeIndex{searchHits: []SearchHit{{Chunk: Chunk{ID: "chunk-1"}}}}
	engine, err := NewEngine(newFakeStore(), index, nil)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	hits, err := engine.Search(context.Background(), SearchRequest{
		Query: "hello",
		Mode:  SearchModeHybrid,
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %#v", hits)
	}
	if len(index.searchReq.QueryVector) != 0 {
		t.Fatalf("query vector = %#v", index.searchReq.QueryVector)
	}
}

func TestEngineSearchVectorNeedsEmbedder(t *testing.T) {
	engine, err := NewEngine(newFakeStore(), &fakeIndex{}, nil)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	_, err = engine.Search(context.Background(), SearchRequest{
		Query: "hello",
		Mode:  SearchModeVector,
	})
	if !errors.Is(err, ErrMissingEmbedder) {
		t.Fatalf("Search() error = %v, want ErrMissingEmbedder", err)
	}
}
