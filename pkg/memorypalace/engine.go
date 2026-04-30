package memorypalace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type Engine struct {
	store    MemoryStore
	index    RetrievalIndex
	embedder Embedder
}

func NewEngine(store MemoryStore, index RetrievalIndex, embedder Embedder) (*Engine, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: store is nil", ErrInvalidRequest)
	}
	if index == nil {
		return nil, fmt.Errorf("%w: index is nil", ErrInvalidRequest)
	}
	return &Engine{store: store, index: index, embedder: embedder}, nil
}

func (e *Engine) Migrate(ctx context.Context) error {
	if err := e.store.Migrate(ctx); err != nil {
		return err
	}
	return e.index.Migrate(ctx)
}

func (e *Engine) Capabilities(ctx context.Context) (Capabilities, error) {
	return e.index.Capabilities(ctx)
}

func (e *Engine) ListSourceSyncByKind(ctx context.Context, syncKind string) ([]SourceSync, error) {
	if strings.TrimSpace(syncKind) == "" {
		return nil, fmt.Errorf("%w: sync kind required", ErrInvalidRequest)
	}
	return e.store.ListSourceSyncByKind(ctx, syncKind)
}

func (e *Engine) IndexSource(ctx context.Context, src Source, chunks []Chunk) error {
	if strings.TrimSpace(src.ID) == "" {
		return fmt.Errorf("%w: source id required", ErrInvalidRequest)
	}
	now := time.Now().UTC()
	if src.CreatedAt.IsZero() {
		src.CreatedAt = now
	}
	src.UpdatedAt = now
	for i := range chunks {
		chunks[i].SourceID = src.ID
		if strings.TrimSpace(chunks[i].ID) == "" {
			return fmt.Errorf("%w: chunk id required", ErrInvalidRequest)
		}
		if chunks[i].CreatedAt.IsZero() {
			chunks[i].CreatedAt = now
		}
		chunks[i].UpdatedAt = now
	}

	vectors, err := e.embedChunks(ctx, chunks)
	if err != nil {
		return err
	}
	records := make([]IndexRecord, 0, len(chunks))
	for i, chunk := range chunks {
		record := IndexRecord{Chunk: chunk}
		if len(vectors) == len(chunks) {
			record.Vector = vectors[i]
		}
		records = append(records, record)
	}
	if err := e.store.UpsertSource(ctx, src); err != nil {
		return err
	}
	if err := e.store.ReplaceChunks(ctx, src.ID, chunks); err != nil {
		return err
	}
	if err := e.index.DeleteBySource(ctx, src.ID); err != nil {
		return err
	}
	return e.index.Upsert(ctx, records)
}

func (e *Engine) SyncDocument(ctx context.Context, doc SyncSource) (SyncResult, error) {
	if strings.TrimSpace(doc.SyncKind) == "" {
		return SyncResult{}, fmt.Errorf("%w: sync kind required", ErrInvalidRequest)
	}
	if strings.TrimSpace(doc.ExternalRef) == "" {
		return SyncResult{}, fmt.Errorf("%w: external ref required", ErrInvalidRequest)
	}
	seenAt := doc.SeenAt.UTC()
	if seenAt.IsZero() {
		seenAt = time.Now().UTC()
	}
	existingSync, err := e.store.GetSourceSyncByExternalRef(ctx, doc.SyncKind, doc.ExternalRef)
	if err != nil && err != ErrNotFound {
		return SyncResult{}, err
	}
	sourceID := strings.TrimSpace(doc.SourceID)
	if existingSync.SourceID != "" {
		sourceID = existingSync.SourceID
	}
	if sourceID == "" {
		sourceID = stableID("src", doc.SyncKind, doc.ExternalRef)
	}
	contentHash := hashString(doc.Content)
	result := SyncResult{
		SourceID:    sourceID,
		ContentHash: contentHash,
	}
	sync := SourceSync{
		SourceID:      sourceID,
		SyncKind:      doc.SyncKind,
		ExternalRef:   doc.ExternalRef,
		LastSeenAt:    seenAt,
		LastScannedAt: seenAt,
		SourceVersion: strings.TrimSpace(doc.SourceVersion),
		ContentHash:   contentHash,
		Status:        SyncStatusActive,
		Metadata:      cloneMap(doc.Metadata),
	}

	if existingSync.SourceID != "" && existingSync.ContentHash == contentHash && existingSync.Status == SyncStatusActive {
		source, err := e.existingSourceOrDefault(ctx, sourceID, doc, seenAt, contentHash)
		if err != nil {
			return SyncResult{}, err
		}
		if err := e.store.UpsertSource(ctx, source); err != nil {
			return SyncResult{}, err
		}
		if err := e.store.UpsertSourceSync(ctx, sync); err != nil {
			return SyncResult{}, err
		}
		result.Action = "unchanged"
		return result, nil
	}

	source, err := e.existingSourceOrDefault(ctx, sourceID, doc, seenAt, contentHash)
	if err != nil {
		return SyncResult{}, err
	}
	entries := e.documentEntries(source, doc, seenAt, contentHash)
	chunks, links, err := deriveDocument(source, entries)
	if err != nil {
		return SyncResult{}, err
	}
	for i := range entries {
		if entries[i].CreatedAt.IsZero() {
			entries[i].CreatedAt = seenAt
		}
	}
	for i := range chunks {
		if chunks[i].CreatedAt.IsZero() {
			chunks[i].CreatedAt = seenAt
		}
		chunks[i].UpdatedAt = seenAt
	}

	if err := e.store.ReplaceSourceDocument(ctx, source, entries, chunks, links, sync); err != nil {
		return SyncResult{}, err
	}
	result.ChunkCount = len(chunks)
	if existingSync.SourceID == "" {
		result.Action = "created"
	} else {
		result.Action = "updated"
	}

	if err := e.index.DeleteBySource(ctx, sourceID); err != nil {
		return SyncResult{}, err
	}
	indexErr := e.upsertChunkVectors(ctx, sourceID, chunks, sync)
	if indexErr != "" {
		result.IndexError = indexErr
	}
	return result, nil
}

func (e *Engine) MarkMissing(ctx context.Context, syncKind string, externalRef string, seenAt time.Time) error {
	if strings.TrimSpace(syncKind) == "" {
		return fmt.Errorf("%w: sync kind required", ErrInvalidRequest)
	}
	if strings.TrimSpace(externalRef) == "" {
		return fmt.Errorf("%w: external ref required", ErrInvalidRequest)
	}
	if seenAt.IsZero() {
		seenAt = time.Now().UTC()
	}
	return e.store.MarkSourceMissing(ctx, syncKind, externalRef, seenAt.UTC())
}

func (e *Engine) RebuildSource(ctx context.Context, sourceID string) error {
	if strings.TrimSpace(sourceID) == "" {
		return fmt.Errorf("%w: source id required", ErrInvalidRequest)
	}
	source, err := e.store.GetSource(ctx, sourceID)
	if err != nil {
		return err
	}
	entries, err := e.store.ListEntries(ctx, sourceID)
	if err != nil {
		return err
	}
	sync, err := e.store.GetSourceSync(ctx, sourceID)
	if err != nil && err != ErrNotFound {
		return err
	}
	now := time.Now().UTC()
	source.UpdatedAt = now
	if sync.SourceID != "" {
		sync.LastScannedAt = now
		if sync.Status == "" {
			sync.Status = SyncStatusActive
		}
	}
	chunks, links, err := deriveDocument(source, entries)
	if err != nil {
		return err
	}
	for i := range chunks {
		if chunks[i].CreatedAt.IsZero() {
			chunks[i].CreatedAt = now
		}
		chunks[i].UpdatedAt = now
	}
	if err := e.store.ReplaceSourceDocument(ctx, source, entries, chunks, links, sync); err != nil {
		return err
	}
	if err := e.index.DeleteBySource(ctx, sourceID); err != nil {
		return err
	}
	_ = e.upsertChunkVectors(ctx, sourceID, chunks, sync)
	return nil
}

func (e *Engine) PruneSource(ctx context.Context, sourceID string) error {
	return e.DeleteSource(ctx, sourceID)
}

func (e *Engine) DeleteSource(ctx context.Context, sourceID string) error {
	if strings.TrimSpace(sourceID) == "" {
		return fmt.Errorf("%w: source id required", ErrInvalidRequest)
	}
	if err := e.index.DeleteBySource(ctx, sourceID); err != nil {
		return err
	}
	return e.store.DeleteSource(ctx, sourceID)
}

func (e *Engine) Search(ctx context.Context, req SearchRequest) ([]SearchHit, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("%w: query required", ErrInvalidRequest)
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Mode == "" {
		req.Mode = SearchModeHybrid
	}
	switch req.Mode {
	case SearchModeKeyword:
	case SearchModeVector:
		if err := e.attachQueryVector(ctx, &req); err != nil {
			return nil, err
		}
	case SearchModeHybrid:
		if err := e.attachQueryVector(ctx, &req); err != nil && err != ErrMissingEmbedder {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: unknown mode %q", ErrInvalidRequest, req.Mode)
	}
	return e.index.Search(ctx, req)
}

func (e *Engine) embedChunks(ctx context.Context, chunks []Chunk) ([][]float32, error) {
	if e.embedder == nil || len(chunks) == 0 {
		return nil, nil
	}
	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		texts = append(texts, chunk.Content)
	}
	return e.embedder.Embed(ctx, texts)
}

func (e *Engine) attachQueryVector(ctx context.Context, req *SearchRequest) error {
	if len(req.QueryVector) > 0 {
		return nil
	}
	if e.embedder == nil {
		return ErrMissingEmbedder
	}
	vectors, err := e.embedder.Embed(ctx, []string{req.Query})
	if err != nil {
		return err
	}
	if len(vectors) != 1 {
		return fmt.Errorf("%w: embedder returned %d query vectors", ErrInvalidRequest, len(vectors))
	}
	req.QueryVector = vectors[0]
	return nil
}

func (e *Engine) existingSourceOrDefault(ctx context.Context, sourceID string, doc SyncSource, now time.Time, contentHash string) (Source, error) {
	existing, err := e.store.GetSource(ctx, sourceID)
	switch {
	case err == nil:
		existing.Kind = defaultString(doc.SourceKind, existing.Kind, "document")
		existing.Scope = defaultString(doc.Scope, existing.Scope)
		existing.Path = defaultString(doc.Path, existing.Path, displayPath(doc.ExternalRef))
		existing.Title = defaultString(doc.Title, existing.Title, filepath.Base(existing.Path))
		existing.ExternalRef = defaultString(doc.ExternalRef, existing.ExternalRef)
		existing.ContentHash = contentHash
		existing.UpdatedAt = now
		existing.Metadata = mergeMaps(existing.Metadata, doc.Metadata)
		return existing, nil
	case err != ErrNotFound:
		return Source{}, err
	default:
		return Source{
			ID:          sourceID,
			Kind:        defaultString(doc.SourceKind, "document"),
			Scope:       strings.TrimSpace(doc.Scope),
			Path:        defaultString(doc.Path, displayPath(doc.ExternalRef)),
			Title:       defaultString(doc.Title, filepath.Base(defaultString(doc.Path, doc.ExternalRef))),
			ExternalRef: strings.TrimSpace(doc.ExternalRef),
			ContentHash: contentHash,
			Metadata:    cloneMap(doc.Metadata),
			CreatedAt:   now,
			UpdatedAt:   now,
		}, nil
	}
}

func (e *Engine) documentEntries(source Source, doc SyncSource, now time.Time, contentHash string) []Entry {
	entryID := stableID("entry", source.ID, "document_body")
	return []Entry{{
		ID:          entryID,
		SourceID:    source.ID,
		Seq:         0,
		Kind:        "document_body",
		Role:        "document",
		Text:        doc.Content,
		ContentHash: contentHash,
		EventAt:     now,
		CreatedAt:   now,
		Metadata:    cloneMap(doc.Metadata),
	}}
}

func (e *Engine) upsertChunkVectors(ctx context.Context, sourceID string, chunks []Chunk, sync SourceSync) string {
	vectors, err := e.embedChunks(ctx, chunks)
	if err != nil {
		sync.LastError = err.Error()
		_ = e.store.UpsertSourceSync(ctx, sync)
		return err.Error()
	}
	if len(vectors) == 0 {
		sync.LastError = ""
		_ = e.store.UpsertSourceSync(ctx, sync)
		return ""
	}
	records := make([]IndexRecord, 0, len(chunks))
	for i, chunk := range chunks {
		record := IndexRecord{Chunk: chunk}
		if i < len(vectors) {
			record.Vector = vectors[i]
		}
		records = append(records, record)
	}
	if err := e.index.Upsert(ctx, records); err != nil {
		sync.LastError = err.Error()
		_ = e.store.UpsertSourceSync(ctx, sync)
		return err.Error()
	}
	sync.LastError = ""
	_ = e.store.UpsertSourceSync(ctx, sync)
	_ = sourceID
	return ""
}

func displayPath(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	return filepath.Base(ref)
}

func defaultString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeMaps(a map[string]string, b map[string]string) map[string]string {
	out := cloneMap(a)
	for k, v := range b {
		out[k] = v
	}
	return out
}

func hashString(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func stableID(prefix string, parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	sum := hash.Sum(nil)
	return prefix + "-" + hex.EncodeToString(sum[:8])
}
