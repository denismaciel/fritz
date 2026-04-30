package memorypalace

import (
	"context"
	"time"
)

type MemoryStore interface {
	Migrate(ctx context.Context) error
	UpsertSource(ctx context.Context, src Source) error
	GetSource(ctx context.Context, sourceID string) (Source, error)
	ReplaceSourceDocument(ctx context.Context, src Source, entries []Entry, chunks []Chunk, links []ChunkEntry, sync SourceSync) error
	ReplaceEntries(ctx context.Context, sourceID string, entries []Entry) error
	ReplaceChunks(ctx context.Context, sourceID string, chunks []Chunk) error
	ReplaceChunkEntries(ctx context.Context, sourceID string, links []ChunkEntry) error
	ListEntries(ctx context.Context, sourceID string) ([]Entry, error)
	UpsertSourceSync(ctx context.Context, sync SourceSync) error
	GetSourceSync(ctx context.Context, sourceID string) (SourceSync, error)
	GetSourceSyncByExternalRef(ctx context.Context, syncKind string, externalRef string) (SourceSync, error)
	ListSourceSyncByKind(ctx context.Context, syncKind string) ([]SourceSync, error)
	MarkSourceMissing(ctx context.Context, syncKind string, externalRef string, seenAt time.Time) error
	DeleteSource(ctx context.Context, sourceID string) error
	GetChunk(ctx context.Context, chunkID string) (Chunk, error)
	GetChunks(ctx context.Context, chunkIDs []string) ([]Chunk, error)
	ListBySource(ctx context.Context, sourceID string) ([]Chunk, error)
}

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dim() int
	Name() string
}

type RetrievalIndex interface {
	Migrate(ctx context.Context) error
	Upsert(ctx context.Context, records []IndexRecord) error
	DeleteBySource(ctx context.Context, sourceID string) error
	Search(ctx context.Context, req SearchRequest) ([]SearchHit, error)
	Capabilities(ctx context.Context) (Capabilities, error)
}
