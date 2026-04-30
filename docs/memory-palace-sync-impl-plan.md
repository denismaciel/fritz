# Memory Palace Sync Impl Plan

Detailed plan for moving Fritz memory storage to a proper source-sync model.

This plan follows the earlier recommendation:

- canonical source records are truth
- content hash drives change detection
- retrieval chunks are derived
- FTS/vector indexes are disposable

## Outcome

At the end of this work, Fritz should be able to:

- ingest outside documents into memory
- detect when a source changed
- rebuild only that source's derived chunks/indexes
- detect when a previously known source disappeared
- mark it tombstoned or delete it by policy
- keep retrieval stable across embedder/index changes

## Non-goals

Not part of first pass:

- graph facts
- automatic fact extraction
- cross-source dedupe
- incremental in-document patching
- file watching / live daemon sync

## Current state

Today the package is roughly:

- `sources`
- `chunks`
- FTS
- vectors

Good enough for first retrieval, but weak for sync because:

- chunks are too close to source-of-truth
- no canonical entry layer
- no explicit sync state
- no tombstone model
- no clean rebuild contract by source version/hash

## Target model

### Canonical tables

Use the schema from [memory-palace-canonical-schema.md](/home/denis/github.com/denismaciel/fritz/docs/memory-palace-canonical-schema.md), with one addition:

#### `memory_source_sync`

Track sync state separately from content tables.

Suggested columns:

- `source_id TEXT PRIMARY KEY REFERENCES memory_sources(source_id) ON DELETE CASCADE`
- `sync_kind TEXT NOT NULL`
  - `filesystem`
  - `session`
  - `manual`
- `external_ref TEXT NOT NULL`
  - abs path, session id, external doc id
- `last_seen_at TEXT NOT NULL`
- `last_scanned_at TEXT NOT NULL`
- `source_version TEXT NOT NULL DEFAULT ''`
  - mtime/etag/revision if available
- `content_hash TEXT NOT NULL DEFAULT ''`
- `sync_status TEXT NOT NULL`
  - `active`
  - `missing`
  - `error`
  - `tombstoned`
- `last_error TEXT NOT NULL DEFAULT ''`
- `metadata_json TEXT NOT NULL`

Reason:

- sync metadata changes more often than source content
- keeps sync concerns out of user-facing source model

## Sync semantics

### Identity

Each outside document needs a stable identity.

Filesystem source:

- `source_id = stable hash(sync_kind + normalized absolute path)`
- `external_ref = normalized absolute path`

Rule:

- rename/move counts as new source in v1
- later we can support rename detection if needed

### Change detection

Use two-stage detection:

1. cheap precheck
   - file exists?
   - mtime/size changed?
2. exact check
   - read content
   - compute `sha256(content)`

Rule:

- hash is truth
- mtime/size only avoids needless reads where possible

### Missing source handling

If source was previously known but not present on sync:

1. set `sync_status = 'missing'`
2. stop returning its chunks in normal search
3. keep canonical rows for a grace period or until explicit prune

First-pass default:

- tombstone logically, do not hard-delete immediately

Reason:

- safer
- debuggable
- supports accidental delete/recovery

### Rebuild contract

When hash changed:

1. rewrite canonical entries for that source
2. regenerate chunks for that source
3. regenerate FTS rows for that source
4. regenerate vectors for that source
5. update `memory_source_sync`

Important:

- rebuild is per source
- no global reindex required

## API changes

## Core package

Add explicit source sync API in [pkg/memorypalace](/home/denis/github.com/denismaciel/fritz/pkg/memorypalace).

Suggested new types:

```go
type SyncSource struct {
    SyncKind     string
    ExternalRef  string
    Title        string
    Scope        string
    SourceKind   string
    SourceVersion string
    Content      string
    Metadata     map[string]string
    SeenAt       time.Time
}

type SyncResult struct {
    SourceID     string
    Action       string // created, updated, unchanged, missing, tombstoned
    ChunkCount   int
    ContentHash  string
}
```

Suggested engine methods:

```go
SyncDocument(ctx context.Context, src SyncSource) (SyncResult, error)
MarkMissing(ctx context.Context, syncKind string, externalRef string, seenAt time.Time) error
PruneSource(ctx context.Context, sourceID string) error
RebuildSource(ctx context.Context, sourceID string) error
```

Keep current search API intact.

## Storage adapter changes

In [pkg/memorypalace/sqlite/adapter.go](/home/denis/github.com/denismaciel/fritz/pkg/memorypalace/sqlite/adapter.go):

### Phase 1

- add `memory_entries`
- add `memory_chunk_entries`
- add `memory_source_sync`
- keep current `sources` / `chunks` naming if we want minimal churn

Practical point:

- if renaming existing tables is noisy, keep table names as-is now and evolve semantics

### Required adapter ops

- `UpsertSource`
- `ReplaceEntries`
- `ReplaceChunks`
- `UpsertSourceSync`
- `GetSourceSyncByExternalRef`
- `ListSourcesBySyncKind`
- `MarkSourceMissing`
- `ListTombstonedSources`
- `DeleteSourceHard`

## Derivation layer

Need one explicit renderer/chunker layer between canonical entries and retrieval chunks.

Suggested package/module:

- `pkg/memorypalace/derive`, or an equivalent helper inside `pkg/memorypalace` if we want to avoid package cycles

Responsibilities:

- render canonical entries into retrieval text
- choose chunk boundaries
- attach stable metadata
- emit chunk-entry mapping rows

First policies:

### For outside docs

- one `document_body` entry per source
- paragraph/window chunker over entry text
- deterministic boundaries

### For sessions

- one entry per message/tool event
- first chunking policy: one message per chunk
- second policy later: turn-based chunk grouping

## Sync pipeline

### Filesystem document sync

1. enumerate candidate files
2. normalize path
3. check existing sync row by `external_ref`
4. if file missing:
   - mark missing
   - continue
5. stat file
6. if version unchanged and policy allows:
   - optionally skip read
7. read file
8. hash content
9. if hash unchanged:
   - update `last_seen_at`, `last_scanned_at`, `sync_status='active'`
   - return unchanged
10. build canonical source + entries
11. derive chunks
12. write in one transaction:
   - upsert source
   - replace entries
   - replace chunks
   - replace chunk-entry links
   - replace FTS rows
   - replace vector rows
   - upsert sync state
13. return created/updated

## Transaction rules

Per-source sync should be atomic.

If embedding fails:

- canonical rows may still succeed
- vector rows can be absent
- source remains searchable by keyword

So use this split:

1. transaction A
   - canonical rows
   - chunks
   - FTS
   - sync state
2. vector upsert after
3. if vector upsert fails, store error state in sync metadata or index metadata

Reason:

- do not let remote embedding outage block ingestion

## Migration plan

### Step 1

Extend current schema without breaking search:

- add `memory_entries`
- add `memory_chunk_entries`
- add `memory_source_sync`

No behavior change yet.

### Step 2

Refactor durable memory indexing path in [internal/memory/palace.go](/home/denis/github.com/denismaciel/fritz/internal/memory/palace.go):

- stop building raw source/chunk directly
- build canonical source + entries
- derive chunks from entries

This gives us one real source type first.

### Step 3

Add document sync service for outside files.

Suggested package:

- `internal/memory/syncdocs.go`

Responsibilities:

- file enumeration
- precheck
- sync calls into engine
- missing-file detection

### Step 4

Hook session persistence onto canonical entries.

Suggested shape:

- session id => one `memory_source`
- each persisted user/assistant/tool event => one entry
- retrieval chunks derived from those entries

### Step 5

Add prune/tombstone commands.

Examples:

- `fritz memory sync`
- `fritz memory prune`
- `fritz memory reindex --source ...`

## Retrieval behavior changes

Search should exclude non-active sources by default.

Default filter:

- include only `sync_status='active'`

Optional later flags:

- include tombstoned
- include missing

## Testing plan

### Unit tests

In [pkg/memorypalace/sqlite/adapter_test.go](/home/denis/github.com/denismaciel/fritz/pkg/memorypalace/sqlite/adapter_test.go):

- create source + sync row
- unchanged sync updates timestamps only
- changed content replaces entries/chunks
- missing source marks `sync_status='missing'`
- tombstoned source excluded from search
- hard delete removes source, entries, chunks, vectors, sync row

In engine tests:

- `SyncDocument(created)`
- `SyncDocument(unchanged)`
- `SyncDocument(updated)`
- `RebuildSource`
- vector failure still leaves keyword-searchable rows

### Integration tests

For filesystem sync:

- ingest file
- search finds content
- edit file
- resync
- old chunk text gone
- new chunk text searchable
- delete file
- resync
- source no longer returned by default

### Live tests

Keep one guarded Gemini live test:

- sync doc with embeddings enabled
- confirm vector rows created

## Rollout order

1. schema extension
2. canonical entry write path for durable memory docs
3. derivation layer
4. sync state table + filesystem sync service
5. session/event ingestion onto entries
6. prune/tombstone UX
7. optional full reindex tooling

## Risks

### Risk: too much migration churn

Mitigation:

- keep existing table names where useful
- evolve semantics first

### Risk: embedding outages block ingestion

Mitigation:

- canonical + FTS path must succeed without vectors

### Risk: chunk policy changes break search behavior

Mitigation:

- make chunk renderer versioned
- store derivation version in chunk metadata

### Risk: duplicate source identities

Mitigation:

- normalize filesystem paths before hashing

## Concrete next tickets

1. add `memory_entries`, `memory_chunk_entries`, `memory_source_sync`
2. add engine `SyncDocument` path
3. add document-entry -> chunk derivation layer
4. convert durable memory indexing to entry-first writes
5. add filesystem sync service
6. add missing/tombstone filtering to search
7. add tests for update/delete/resync flows

This is the smallest plan that gives Fritz a real sync model instead of the reference impl's mtime-based delete-and-reinsert loop.
