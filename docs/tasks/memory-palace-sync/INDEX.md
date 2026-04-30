# memory-palace-sync task index

Move Fritz memory storage from chunk-first indexing to a real source-sync model: canonical source records and entries as truth, derived chunks for retrieval, and per-source rebuilds keyed by content hash.

Current status summary is in the sibling `STATUS.md`.

## Assumptions

- the existing embedded memory package stays the base; this pack evolves it
- first pass targets outside documents and durable memory files, not session-event ingestion
- source change detection should use content hash as truth and mtime/size only as precheck
- retrieval must remain available with keyword-only search when embedding fails
- missing files should default to logical tombstones, not immediate hard delete
- search should exclude non-active sources by default

## Engineering Constraints

- keep IO at the edges; keep core logic testable
- prefer deep modules with narrow public APIs
- depend on interfaces/protocols, not concrete infrastructure
- validate untrusted input at boundaries
- avoid incidental refactors outside ticket scope
- bias toward stable, boring code over cleverness
- preserve existing search behavior while canonical entry and sync state land
- do not leak SQLite or provider-specific types in the core public API
- keep per-source rebuilds atomic for canonical rows, chunks, FTS, and sync state
- do not let remote embedding failures block canonical ingest
- keep session-event ingestion out of this pack unless a ticket explicitly adds it

## Tasks

- `MPS-001.md`
- `MPS-002.md`
- `MPS-003.md`
- `MPS-004.md`
- `MPS-005.md`
- `MPS-006.md`
- `MPS-007.md`
