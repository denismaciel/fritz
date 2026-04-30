package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fritz/pkg/memorypalace"
)

const (
	driverName      = "sqlite3"
	defaultLimit    = 10
	rrfConstant     = 60.0
	metaVectorDim   = "vector_dimension"
	metaVectorName  = "vector_embedder"
	activeSyncState = string(memorypalace.SyncStatusActive)
)

type Adapter struct {
	db   *sql.DB
	path string
	caps memorypalace.Capabilities
}

func Open(path string) (*Adapter, error) {
	if err := ensureDriver(); err != nil {
		return nil, err
	}
	if path != ":memory:" && path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open(driverName, path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	a := &Adapter{db: db, path: path}
	if err := a.configure(); err != nil {
		db.Close()
		return nil, err
	}
	caps, err := a.probeCapabilities(context.Background())
	if err != nil {
		db.Close()
		return nil, err
	}
	a.caps = caps
	return a, nil
}

func (a *Adapter) Close() error {
	if a == nil || a.db == nil {
		return nil
	}
	return a.db.Close()
}

func (a *Adapter) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sources (
			source_id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			scope TEXT NOT NULL DEFAULT '',
			path TEXT NOT NULL,
			title TEXT NOT NULL,
			external_ref TEXT NOT NULL DEFAULT '',
			content_hash TEXT NOT NULL,
			metadata_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS entries (
			entry_id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL REFERENCES sources(source_id) ON DELETE CASCADE,
			seq INTEGER NOT NULL,
			parent_entry_id TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			text TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			event_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			metadata_json TEXT NOT NULL,
			UNIQUE(source_id, seq)
		)`,
		`CREATE INDEX IF NOT EXISTS entries_by_source ON entries(source_id, seq)`,
		`CREATE TABLE IF NOT EXISTS chunks (
			chunk_id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL REFERENCES sources(source_id) ON DELETE CASCADE,
			ordinal INTEGER NOT NULL,
			kind TEXT NOT NULL DEFAULT 'document_span',
			wing TEXT NOT NULL,
			room TEXT NOT NULL,
			path TEXT NOT NULL,
			content TEXT NOT NULL,
			start_seq INTEGER NOT NULL DEFAULT 0,
			end_seq INTEGER NOT NULL DEFAULT 0,
			metadata_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(source_id, ordinal)
		)`,
		`CREATE INDEX IF NOT EXISTS chunks_by_source ON chunks(source_id, ordinal)`,
		`CREATE TABLE IF NOT EXISTS chunk_entries (
			chunk_id TEXT NOT NULL REFERENCES chunks(chunk_id) ON DELETE CASCADE,
			entry_id TEXT NOT NULL REFERENCES entries(entry_id) ON DELETE CASCADE,
			ordinal INTEGER NOT NULL,
			PRIMARY KEY(chunk_id, entry_id)
		)`,
		`CREATE INDEX IF NOT EXISTS chunk_entries_by_entry ON chunk_entries(entry_id, ordinal)`,
		`CREATE TABLE IF NOT EXISTS source_sync (
			source_id TEXT PRIMARY KEY REFERENCES sources(source_id) ON DELETE CASCADE,
			sync_kind TEXT NOT NULL,
			external_ref TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			last_scanned_at TEXT NOT NULL,
			source_version TEXT NOT NULL DEFAULT '',
			content_hash TEXT NOT NULL DEFAULT '',
			sync_status TEXT NOT NULL,
			last_error TEXT NOT NULL DEFAULT '',
			metadata_json TEXT NOT NULL,
			UNIQUE(sync_kind, external_ref)
		)`,
		`CREATE INDEX IF NOT EXISTS source_sync_by_kind ON source_sync(sync_kind, external_ref)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
			chunk_id UNINDEXED,
			source_id UNINDEXED,
			wing UNINDEXED,
			room UNINDEXED,
			path UNINDEXED,
			content,
			tokenize = 'unicode61'
		)`,
	}
	for _, stmt := range stmts {
		if _, err := a.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	columns := []struct {
		table      string
		column     string
		definition string
	}{
		{"sources", "scope", "TEXT NOT NULL DEFAULT ''"},
		{"sources", "external_ref", "TEXT NOT NULL DEFAULT ''"},
		{"chunks", "kind", "TEXT NOT NULL DEFAULT 'document_span'"},
		{"chunks", "start_seq", "INTEGER NOT NULL DEFAULT 0"},
		{"chunks", "end_seq", "INTEGER NOT NULL DEFAULT 0"},
	}
	for _, column := range columns {
		if err := a.ensureColumn(ctx, column.table, column.column, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) Capabilities(context.Context) (memorypalace.Capabilities, error) {
	return a.caps, nil
}

func (a *Adapter) UpsertSource(ctx context.Context, src memorypalace.Source) error {
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO sources (
			source_id, kind, scope, path, title, external_ref, content_hash, metadata_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id) DO UPDATE SET
			kind = excluded.kind,
			scope = excluded.scope,
			path = excluded.path,
			title = excluded.title,
			external_ref = excluded.external_ref,
			content_hash = excluded.content_hash,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at
	`,
		src.ID,
		defaultString(src.Kind, "memory"),
		src.Scope,
		src.Path,
		defaultString(src.Title, filepath.Base(src.Path)),
		src.ExternalRef,
		src.ContentHash,
		mustJSON(src.Metadata),
		src.CreatedAt.UTC().Format(time.RFC3339Nano),
		src.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (a *Adapter) GetSource(ctx context.Context, sourceID string) (memorypalace.Source, error) {
	var src memorypalace.Source
	var metadataJSON, createdAt, updatedAt string
	err := a.db.QueryRowContext(ctx, `
		SELECT source_id, kind, scope, path, title, external_ref, content_hash, metadata_json, created_at, updated_at
		FROM sources WHERE source_id = ?
	`, sourceID).Scan(
		&src.ID,
		&src.Kind,
		&src.Scope,
		&src.Path,
		&src.Title,
		&src.ExternalRef,
		&src.ContentHash,
		&metadataJSON,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return memorypalace.Source{}, memorypalace.ErrNotFound
	}
	if err != nil {
		return memorypalace.Source{}, err
	}
	src.Metadata = parseJSONMap(metadataJSON)
	src.CreatedAt = parseTime(createdAt)
	src.UpdatedAt = parseTime(updatedAt)
	return src, nil
}

func (a *Adapter) ReplaceSourceDocument(ctx context.Context, src memorypalace.Source, entries []memorypalace.Entry, chunks []memorypalace.Chunk, links []memorypalace.ChunkEntry, sync memorypalace.SourceSync) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := a.upsertSourceTx(ctx, tx, src); err != nil {
		return err
	}
	if err := a.replaceEntriesTx(ctx, tx, src.ID, entries); err != nil {
		return err
	}
	if err := a.replaceChunksTx(ctx, tx, src.ID, chunks); err != nil {
		return err
	}
	if err := a.replaceChunkEntriesTx(ctx, tx, src.ID, links); err != nil {
		return err
	}
	if sync.SourceID != "" {
		if err := a.upsertSourceSyncTx(ctx, tx, sync); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *Adapter) ReplaceEntries(ctx context.Context, sourceID string, entries []memorypalace.Entry) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := a.replaceEntriesTx(ctx, tx, sourceID, entries); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *Adapter) ReplaceChunks(ctx context.Context, sourceID string, chunks []memorypalace.Chunk) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := a.replaceChunksTx(ctx, tx, sourceID, chunks); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *Adapter) ReplaceChunkEntries(ctx context.Context, sourceID string, links []memorypalace.ChunkEntry) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := a.replaceChunkEntriesTx(ctx, tx, sourceID, links); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *Adapter) ListEntries(ctx context.Context, sourceID string) ([]memorypalace.Entry, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT entry_id, source_id, seq, parent_entry_id, kind, role, name, status, text, payload_json, content_hash, event_at, created_at, metadata_json
		FROM entries
		WHERE source_id = ?
		ORDER BY seq ASC
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

func (a *Adapter) UpsertSourceSync(ctx context.Context, sync memorypalace.SourceSync) error {
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO source_sync (
			source_id, sync_kind, external_ref, last_seen_at, last_scanned_at, source_version, content_hash, sync_status, last_error, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id) DO UPDATE SET
			sync_kind = excluded.sync_kind,
			external_ref = excluded.external_ref,
			last_seen_at = excluded.last_seen_at,
			last_scanned_at = excluded.last_scanned_at,
			source_version = excluded.source_version,
			content_hash = excluded.content_hash,
			sync_status = excluded.sync_status,
			last_error = excluded.last_error,
			metadata_json = excluded.metadata_json
	`,
		sync.SourceID,
		sync.SyncKind,
		sync.ExternalRef,
		sync.LastSeenAt.UTC().Format(time.RFC3339Nano),
		sync.LastScannedAt.UTC().Format(time.RFC3339Nano),
		sync.SourceVersion,
		sync.ContentHash,
		defaultSyncStatus(sync.Status),
		sync.LastError,
		mustJSON(sync.Metadata),
	)
	return err
}

func (a *Adapter) GetSourceSync(ctx context.Context, sourceID string) (memorypalace.SourceSync, error) {
	row := a.db.QueryRowContext(ctx, `
		SELECT source_id, sync_kind, external_ref, last_seen_at, last_scanned_at, source_version, content_hash, sync_status, last_error, metadata_json
		FROM source_sync WHERE source_id = ?
	`, sourceID)
	return scanSourceSync(row)
}

func (a *Adapter) GetSourceSyncByExternalRef(ctx context.Context, syncKind string, externalRef string) (memorypalace.SourceSync, error) {
	row := a.db.QueryRowContext(ctx, `
		SELECT source_id, sync_kind, external_ref, last_seen_at, last_scanned_at, source_version, content_hash, sync_status, last_error, metadata_json
		FROM source_sync
		WHERE sync_kind = ? AND external_ref = ?
	`, syncKind, externalRef)
	return scanSourceSync(row)
}

func (a *Adapter) ListSourceSyncByKind(ctx context.Context, syncKind string) ([]memorypalace.SourceSync, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT source_id, sync_kind, external_ref, last_seen_at, last_scanned_at, source_version, content_hash, sync_status, last_error, metadata_json
		FROM source_sync
		WHERE sync_kind = ?
		ORDER BY external_ref ASC
	`, syncKind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSourceSyncRows(rows)
}

func (a *Adapter) MarkSourceMissing(ctx context.Context, syncKind string, externalRef string, seenAt time.Time) error {
	_, err := a.db.ExecContext(ctx, `
		UPDATE source_sync
		SET last_seen_at = ?,
			last_scanned_at = ?,
			sync_status = ?
		WHERE sync_kind = ? AND external_ref = ?
	`,
		seenAt.UTC().Format(time.RFC3339Nano),
		seenAt.UTC().Format(time.RFC3339Nano),
		string(memorypalace.SyncStatusMissing),
		syncKind,
		externalRef,
	)
	return err
}

func (a *Adapter) DeleteSource(ctx context.Context, sourceID string) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks_fts WHERE source_id = ?`, sourceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunk_vec WHERE source_id = ?`, sourceID); err != nil && !isNoSuchTable(err) {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sources WHERE source_id = ?`, sourceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *Adapter) GetChunk(ctx context.Context, chunkID string) (memorypalace.Chunk, error) {
	var chunk memorypalace.Chunk
	var metadataJSON, createdAt, updatedAt string
	err := a.db.QueryRowContext(ctx, `
		SELECT chunk_id, source_id, ordinal, kind, wing, room, path, content, start_seq, end_seq, metadata_json, created_at, updated_at
		FROM chunks WHERE chunk_id = ?
	`, chunkID).Scan(
		&chunk.ID,
		&chunk.SourceID,
		&chunk.Ordinal,
		&chunk.Kind,
		&chunk.Wing,
		&chunk.Room,
		&chunk.Path,
		&chunk.Content,
		&chunk.StartSeq,
		&chunk.EndSeq,
		&metadataJSON,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return memorypalace.Chunk{}, memorypalace.ErrNotFound
	}
	if err != nil {
		return memorypalace.Chunk{}, err
	}
	chunk.Metadata = parseJSONMap(metadataJSON)
	chunk.CreatedAt = parseTime(createdAt)
	chunk.UpdatedAt = parseTime(updatedAt)
	return chunk, nil
}

func (a *Adapter) GetChunks(ctx context.Context, chunkIDs []string) ([]memorypalace.Chunk, error) {
	chunks := make([]memorypalace.Chunk, 0, len(chunkIDs))
	for _, chunkID := range chunkIDs {
		chunk, err := a.GetChunk(ctx, chunkID)
		if err != nil {
			if errors.Is(err, memorypalace.ErrNotFound) {
				continue
			}
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

func (a *Adapter) ListBySource(ctx context.Context, sourceID string) ([]memorypalace.Chunk, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT chunk_id, source_id, ordinal, kind, wing, room, path, content, start_seq, end_seq, metadata_json, created_at, updated_at
		FROM chunks
		WHERE source_id = ?
		ORDER BY ordinal ASC
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChunks(rows)
}

func (a *Adapter) Upsert(ctx context.Context, records []memorypalace.IndexRecord) error {
	if len(records) == 0 {
		return nil
	}
	var withVectors []memorypalace.IndexRecord
	for _, record := range records {
		if len(record.Vector) > 0 {
			withVectors = append(withVectors, record)
		}
	}
	if len(withVectors) == 0 {
		return nil
	}
	if err := a.ensureVectorTable(ctx, len(withVectors[0].Vector)); err != nil {
		return err
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, record := range withVectors {
		blob, err := serializeVector(record.Vector)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO chunk_vec (chunk_id, source_id, embedding)
			VALUES (?, ?, ?)
		`, record.Chunk.ID, record.Chunk.SourceID, blob); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *Adapter) DeleteBySource(ctx context.Context, sourceID string) error {
	_, err := a.db.ExecContext(ctx, `DELETE FROM chunk_vec WHERE source_id = ?`, sourceID)
	if isNoSuchTable(err) {
		return nil
	}
	return err
}

func (a *Adapter) Search(ctx context.Context, req memorypalace.SearchRequest) ([]memorypalace.SearchHit, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, memorypalace.ErrInvalidRequest
	}
	if req.Limit <= 0 {
		req.Limit = defaultLimit
	}
	if req.Mode == "" {
		req.Mode = memorypalace.SearchModeHybrid
	}
	switch req.Mode {
	case memorypalace.SearchModeKeyword:
		return a.searchKeyword(ctx, req)
	case memorypalace.SearchModeVector:
		if len(req.QueryVector) == 0 {
			return nil, memorypalace.ErrMissingEmbedder
		}
		if !a.caps.Vector {
			return nil, memorypalace.ErrVectorUnsupported
		}
		return a.searchVector(ctx, req)
	case memorypalace.SearchModeHybrid:
		keywordHits, err := a.searchKeyword(ctx, req)
		if err != nil {
			return nil, err
		}
		if len(req.QueryVector) == 0 || !a.caps.Vector {
			return keywordHits, nil
		}
		vectorHits, err := a.searchVector(ctx, req)
		if err != nil {
			if errors.Is(err, memorypalace.ErrVectorUnsupported) {
				return keywordHits, nil
			}
			return nil, err
		}
		return fuseHybrid(keywordHits, vectorHits, req.Limit), nil
	default:
		return nil, memorypalace.ErrInvalidRequest
	}
}

func (a *Adapter) configure() error {
	pragmas := []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA busy_timeout = 5000`,
	}
	if a.path != ":memory:" {
		pragmas = append(pragmas, `PRAGMA journal_mode = WAL`)
	}
	for _, stmt := range pragmas {
		if _, err := a.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) probeCapabilities(ctx context.Context) (memorypalace.Capabilities, error) {
	caps := memorypalace.Capabilities{
		Driver:  driverName,
		Keyword: false,
		Vector:  false,
	}
	if err := a.db.QueryRowContext(ctx, `SELECT sqlite_version()`).Scan(&caps.SQLiteVersion); err != nil {
		return caps, err
	}
	if err := a.db.QueryRowContext(ctx, `SELECT vec_version()`).Scan(&caps.VectorVersion); err == nil {
		caps.Vector = true
	}
	if _, err := a.db.ExecContext(ctx, `CREATE VIRTUAL TABLE temp.__fts_probe USING fts5(content)`); err == nil {
		caps.Keyword = true
		_, _ = a.db.ExecContext(ctx, `DROP TABLE temp.__fts_probe`)
	}
	return caps, nil
}

func (a *Adapter) ensureVectorTable(ctx context.Context, dim int) error {
	if !a.caps.Vector {
		return nil
	}
	currentDim, err := a.vectorDimension(ctx)
	if err != nil {
		return err
	}
	if currentDim == 0 {
		stmt := fmt.Sprintf(`
			CREATE VIRTUAL TABLE IF NOT EXISTS chunk_vec USING vec0(
				chunk_id TEXT PRIMARY KEY,
				source_id TEXT,
				embedding FLOAT[%d]
			)
		`, dim)
		if _, err := a.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
		if _, err := a.db.ExecContext(ctx, `
			INSERT INTO meta(key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value
		`, metaVectorDim, fmt.Sprintf("%d", dim)); err != nil {
			return err
		}
		return nil
	}
	if currentDim != dim {
		return fmt.Errorf("%w: vector dimension mismatch have=%d want=%d", memorypalace.ErrInvalidRequest, currentDim, dim)
	}
	return nil
}

func (a *Adapter) vectorDimension(ctx context.Context) (int, error) {
	var raw string
	err := a.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, metaVectorDim).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var dim int
	_, err = fmt.Sscanf(raw, "%d", &dim)
	return dim, err
}

func (a *Adapter) searchKeyword(ctx context.Context, req memorypalace.SearchRequest) ([]memorypalace.SearchHit, error) {
	query := `
		SELECT c.chunk_id, c.source_id, c.ordinal, c.kind, c.wing, c.room, c.path, c.content, c.start_seq, c.end_seq, c.metadata_json, c.created_at, c.updated_at, bm25(chunks_fts) AS rank
		FROM chunks_fts
		JOIN chunks AS c ON c.chunk_id = chunks_fts.chunk_id
		LEFT JOIN source_sync AS ss ON ss.source_id = c.source_id
		WHERE chunks_fts MATCH ?
	`
	args := []any{req.Query}
	if !req.Filter.IncludeInactive {
		query += ` AND (ss.source_id IS NULL OR ss.sync_status = ?)`
		args = append(args, activeSyncState)
	}
	query += ` ORDER BY rank ASC LIMIT ?`
	args = append(args, req.Limit*5)

	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []memorypalace.SearchHit
	for rows.Next() {
		var hit memorypalace.SearchHit
		var metadataJSON, createdAt, updatedAt string
		if err := rows.Scan(
			&hit.Chunk.ID,
			&hit.Chunk.SourceID,
			&hit.Chunk.Ordinal,
			&hit.Chunk.Kind,
			&hit.Chunk.Wing,
			&hit.Chunk.Room,
			&hit.Chunk.Path,
			&hit.Chunk.Content,
			&hit.Chunk.StartSeq,
			&hit.Chunk.EndSeq,
			&metadataJSON,
			&createdAt,
			&updatedAt,
			&hit.KeywordScore,
		); err != nil {
			return nil, err
		}
		hit.Chunk.Metadata = parseJSONMap(metadataJSON)
		hit.Chunk.CreatedAt = parseTime(createdAt)
		hit.Chunk.UpdatedAt = parseTime(updatedAt)
		if !matchesFilter(hit.Chunk, req.Filter) {
			continue
		}
		hit.Score = 1.0 / (1.0 + abs(hit.KeywordScore))
		hits = append(hits, hit)
		if len(hits) == req.Limit {
			break
		}
	}
	return hits, rows.Err()
}

func (a *Adapter) searchVector(ctx context.Context, req memorypalace.SearchRequest) ([]memorypalace.SearchHit, error) {
	blob, err := serializeVector(req.QueryVector)
	if err != nil {
		return nil, err
	}
	query := `
		SELECT c.chunk_id, c.source_id, c.ordinal, c.kind, c.wing, c.room, c.path, c.content, c.start_seq, c.end_seq, c.metadata_json, c.created_at, c.updated_at, v.distance
		FROM chunk_vec AS v
		JOIN chunks AS c ON c.chunk_id = v.chunk_id
		LEFT JOIN source_sync AS ss ON ss.source_id = c.source_id
		WHERE v.embedding MATCH ?
		  AND k = ?
	`
	args := []any{blob, req.Limit * 5}
	if !req.Filter.IncludeInactive {
		query += ` AND (ss.source_id IS NULL OR ss.sync_status = ?)`
		args = append(args, activeSyncState)
	}
	query += ` ORDER BY v.distance ASC`

	rows, err := a.db.QueryContext(ctx, query, args...)
	if isNoSuchTable(err) {
		return nil, memorypalace.ErrVectorUnsupported
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []memorypalace.SearchHit
	for rows.Next() {
		var hit memorypalace.SearchHit
		var metadataJSON, createdAt, updatedAt string
		if err := rows.Scan(
			&hit.Chunk.ID,
			&hit.Chunk.SourceID,
			&hit.Chunk.Ordinal,
			&hit.Chunk.Kind,
			&hit.Chunk.Wing,
			&hit.Chunk.Room,
			&hit.Chunk.Path,
			&hit.Chunk.Content,
			&hit.Chunk.StartSeq,
			&hit.Chunk.EndSeq,
			&metadataJSON,
			&createdAt,
			&updatedAt,
			&hit.VectorDistance,
		); err != nil {
			return nil, err
		}
		hit.Chunk.Metadata = parseJSONMap(metadataJSON)
		hit.Chunk.CreatedAt = parseTime(createdAt)
		hit.Chunk.UpdatedAt = parseTime(updatedAt)
		if !matchesFilter(hit.Chunk, req.Filter) {
			continue
		}
		hit.Score = 1.0 / (1.0 + hit.VectorDistance)
		hits = append(hits, hit)
		if len(hits) == req.Limit {
			break
		}
	}
	return hits, rows.Err()
}

func (a *Adapter) ensureColumn(ctx context.Context, table string, column string, definition string) error {
	ok, err := a.columnExists(ctx, table, column)
	if err != nil || ok {
		return err
	}
	_, err = a.db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, definition))
	return err
}

func (a *Adapter) columnExists(ctx context.Context, table string, column string) (bool, error) {
	rows, err := a.db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (a *Adapter) upsertSourceTx(ctx context.Context, tx *sql.Tx, src memorypalace.Source) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO sources (
			source_id, kind, scope, path, title, external_ref, content_hash, metadata_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id) DO UPDATE SET
			kind = excluded.kind,
			scope = excluded.scope,
			path = excluded.path,
			title = excluded.title,
			external_ref = excluded.external_ref,
			content_hash = excluded.content_hash,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at
	`,
		src.ID,
		defaultString(src.Kind, "memory"),
		src.Scope,
		src.Path,
		defaultString(src.Title, filepath.Base(src.Path)),
		src.ExternalRef,
		src.ContentHash,
		mustJSON(src.Metadata),
		src.CreatedAt.UTC().Format(time.RFC3339Nano),
		src.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (a *Adapter) replaceEntriesTx(ctx context.Context, tx *sql.Tx, sourceID string, entries []memorypalace.Entry) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM entries WHERE source_id = ?`, sourceID); err != nil {
		return err
	}
	for _, entry := range entries {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO entries (
				entry_id, source_id, seq, parent_entry_id, kind, role, name, status, text, payload_json, content_hash, event_at, created_at, metadata_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			entry.ID,
			sourceID,
			entry.Seq,
			entry.ParentEntryID,
			entry.Kind,
			entry.Role,
			entry.Name,
			entry.Status,
			entry.Text,
			defaultString(entry.PayloadJSON, "{}"),
			entry.ContentHash,
			entry.EventAt.UTC().Format(time.RFC3339Nano),
			entry.CreatedAt.UTC().Format(time.RFC3339Nano),
			mustJSON(entry.Metadata),
		); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) replaceChunksTx(ctx context.Context, tx *sql.Tx, sourceID string, chunks []memorypalace.Chunk) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks_fts WHERE source_id = ?`, sourceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE source_id = ?`, sourceID); err != nil {
		return err
	}
	for _, chunk := range chunks {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO chunks (
				chunk_id, source_id, ordinal, kind, wing, room, path, content, start_seq, end_seq, metadata_json, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			chunk.ID,
			sourceID,
			chunk.Ordinal,
			defaultString(chunk.Kind, "document_span"),
			defaultString(chunk.Wing, "durable-memory"),
			defaultString(chunk.Room, "general"),
			chunk.Path,
			chunk.Content,
			chunk.StartSeq,
			chunk.EndSeq,
			mustJSON(chunk.Metadata),
			chunk.CreatedAt.UTC().Format(time.RFC3339Nano),
			chunk.UpdatedAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO chunks_fts (chunk_id, source_id, wing, room, path, content)
			VALUES (?, ?, ?, ?, ?, ?)
		`,
			chunk.ID,
			sourceID,
			defaultString(chunk.Wing, "durable-memory"),
			defaultString(chunk.Room, "general"),
			chunk.Path,
			chunk.Content,
		); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) replaceChunkEntriesTx(ctx context.Context, tx *sql.Tx, sourceID string, links []memorypalace.ChunkEntry) error {
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM chunk_entries
		WHERE chunk_id IN (SELECT chunk_id FROM chunks WHERE source_id = ?)
	`, sourceID); err != nil {
		return err
	}
	for _, link := range links {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO chunk_entries (chunk_id, entry_id, ordinal)
			VALUES (?, ?, ?)
		`, link.ChunkID, link.EntryID, link.Ordinal); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) upsertSourceSyncTx(ctx context.Context, tx *sql.Tx, sync memorypalace.SourceSync) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO source_sync (
			source_id, sync_kind, external_ref, last_seen_at, last_scanned_at, source_version, content_hash, sync_status, last_error, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id) DO UPDATE SET
			sync_kind = excluded.sync_kind,
			external_ref = excluded.external_ref,
			last_seen_at = excluded.last_seen_at,
			last_scanned_at = excluded.last_scanned_at,
			source_version = excluded.source_version,
			content_hash = excluded.content_hash,
			sync_status = excluded.sync_status,
			last_error = excluded.last_error,
			metadata_json = excluded.metadata_json
	`,
		sync.SourceID,
		sync.SyncKind,
		sync.ExternalRef,
		sync.LastSeenAt.UTC().Format(time.RFC3339Nano),
		sync.LastScannedAt.UTC().Format(time.RFC3339Nano),
		sync.SourceVersion,
		sync.ContentHash,
		defaultSyncStatus(sync.Status),
		sync.LastError,
		mustJSON(sync.Metadata),
	)
	return err
}

func scanChunks(rows *sql.Rows) ([]memorypalace.Chunk, error) {
	var chunks []memorypalace.Chunk
	for rows.Next() {
		var chunk memorypalace.Chunk
		var metadataJSON, createdAt, updatedAt string
		if err := rows.Scan(
			&chunk.ID,
			&chunk.SourceID,
			&chunk.Ordinal,
			&chunk.Kind,
			&chunk.Wing,
			&chunk.Room,
			&chunk.Path,
			&chunk.Content,
			&chunk.StartSeq,
			&chunk.EndSeq,
			&metadataJSON,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		chunk.Metadata = parseJSONMap(metadataJSON)
		chunk.CreatedAt = parseTime(createdAt)
		chunk.UpdatedAt = parseTime(updatedAt)
		chunks = append(chunks, chunk)
	}
	return chunks, rows.Err()
}

func scanEntries(rows *sql.Rows) ([]memorypalace.Entry, error) {
	var entries []memorypalace.Entry
	for rows.Next() {
		var entry memorypalace.Entry
		var metadataJSON, eventAt, createdAt string
		if err := rows.Scan(
			&entry.ID,
			&entry.SourceID,
			&entry.Seq,
			&entry.ParentEntryID,
			&entry.Kind,
			&entry.Role,
			&entry.Name,
			&entry.Status,
			&entry.Text,
			&entry.PayloadJSON,
			&entry.ContentHash,
			&eventAt,
			&createdAt,
			&metadataJSON,
		); err != nil {
			return nil, err
		}
		entry.EventAt = parseTime(eventAt)
		entry.CreatedAt = parseTime(createdAt)
		entry.Metadata = parseJSONMap(metadataJSON)
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func scanSourceSync(row *sql.Row) (memorypalace.SourceSync, error) {
	var sync memorypalace.SourceSync
	var seenAt, scannedAt, metadataJSON, status string
	err := row.Scan(
		&sync.SourceID,
		&sync.SyncKind,
		&sync.ExternalRef,
		&seenAt,
		&scannedAt,
		&sync.SourceVersion,
		&sync.ContentHash,
		&status,
		&sync.LastError,
		&metadataJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return memorypalace.SourceSync{}, memorypalace.ErrNotFound
	}
	if err != nil {
		return memorypalace.SourceSync{}, err
	}
	sync.LastSeenAt = parseTime(seenAt)
	sync.LastScannedAt = parseTime(scannedAt)
	sync.Status = memorypalace.SyncStatus(status)
	sync.Metadata = parseJSONMap(metadataJSON)
	return sync, nil
}

func scanSourceSyncRows(rows *sql.Rows) ([]memorypalace.SourceSync, error) {
	var out []memorypalace.SourceSync
	for rows.Next() {
		var sync memorypalace.SourceSync
		var seenAt, scannedAt, metadataJSON, status string
		if err := rows.Scan(
			&sync.SourceID,
			&sync.SyncKind,
			&sync.ExternalRef,
			&seenAt,
			&scannedAt,
			&sync.SourceVersion,
			&sync.ContentHash,
			&status,
			&sync.LastError,
			&metadataJSON,
		); err != nil {
			return nil, err
		}
		sync.LastSeenAt = parseTime(seenAt)
		sync.LastScannedAt = parseTime(scannedAt)
		sync.Status = memorypalace.SyncStatus(status)
		sync.Metadata = parseJSONMap(metadataJSON)
		out = append(out, sync)
	}
	return out, rows.Err()
}

func fuseHybrid(keywordHits, vectorHits []memorypalace.SearchHit, limit int) []memorypalace.SearchHit {
	type aggregate struct {
		hit   memorypalace.SearchHit
		score float64
	}
	combined := map[string]*aggregate{}
	merge := func(hits []memorypalace.SearchHit, isKeyword bool) {
		for idx, hit := range hits {
			agg, ok := combined[hit.Chunk.ID]
			if !ok {
				copyHit := hit
				agg = &aggregate{hit: copyHit}
				combined[hit.Chunk.ID] = agg
			}
			agg.score += 1.0 / (rrfConstant + float64(idx) + 1.0)
			if isKeyword {
				agg.hit.KeywordScore = hit.KeywordScore
			} else {
				agg.hit.VectorDistance = hit.VectorDistance
			}
		}
	}
	merge(keywordHits, true)
	merge(vectorHits, false)

	out := make([]memorypalace.SearchHit, 0, len(combined))
	for _, agg := range combined {
		agg.hit.Score = agg.score
		out = append(out, agg.hit)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Chunk.ID < out[j].Chunk.ID
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultSyncStatus(status memorypalace.SyncStatus) string {
	if strings.TrimSpace(string(status)) == "" {
		return activeSyncState
	}
	return string(status)
}

func mustJSON(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func parseJSONMap(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]string{}
	}
	if out == nil {
		return map[string]string{}
	}
	return out
}

func parseTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func matchesFilter(chunk memorypalace.Chunk, filter memorypalace.SearchFilter) bool {
	if filter.SourceID != "" && chunk.SourceID != filter.SourceID {
		return false
	}
	if filter.Wing != "" && chunk.Wing != filter.Wing {
		return false
	}
	if filter.Room != "" && chunk.Room != filter.Room {
		return false
	}
	if filter.PathPrefix != "" && !strings.HasPrefix(chunk.Path, filter.PathPrefix) {
		return false
	}
	return true
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func isNoSuchTable(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table")
}
