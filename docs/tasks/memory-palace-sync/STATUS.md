# memory-palace-sync status

## TODO

## IN_PROGRESS

## BLOCKED

## DONE

- `MPS-001` core sync types + canonical entry model
- `MPS-002` SQLite schema + adapter ops for entries, chunk links, and sync state
- `MPS-003` derivation layer for document entries -> retrieval chunks
- `MPS-004` engine sync/rebuild flow with keyword-first failure handling
- `MPS-005` migrate durable-memory ingest to entry-first sync path
- `MPS-006` filesystem sync service with missing/tombstone handling
- `MPS-007` search filtering and update/delete/resync coverage

## SUPERSEDED

## Notes

- treat [docs/memory-palace-canonical-schema.md](/home/denis/github.com/denismaciel/fritz/docs/memory-palace-canonical-schema.md) as the schema source
- treat [docs/memory-palace-sync-impl-plan.md](/home/denis/github.com/denismaciel/fritz/docs/memory-palace-sync-impl-plan.md) as the sequencing/source-sync note
- preserve the existing embedded-memory package boundary; this pack deepens it rather than replacing it
- validation should use `nix develop -c sh -lc 'export GOFLAGS="-tags=sqlite_fts5"; ...'` where SQLite vector/FTS support is needed
- pack ends at document sync; session-event canonicalization is a follow-on pack
- implemented with `entries`, `chunk_entries`, and `source_sync` tables in the existing SQLite adapter rather than a separate derive subpackage
- validated with:
  - `nix develop -c sh -lc 'export GOFLAGS="-tags=sqlite_fts5"; go test ./pkg/memorypalace/... ./internal/memory'`
  - `nix develop -c sh -lc 'export GOFLAGS="-tags=sqlite_fts5"; go test ./...'`
  - `nix build .#fritz`
