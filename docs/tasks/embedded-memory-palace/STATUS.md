# embedded-memory-palace status

## TODO

## IN_PROGRESS

## BLOCKED

## DONE

- `EMP-001` core package boundary + interfaces
- `EMP-002` SQLite runtime + extension loading strategy
- `EMP-003` canonical SQLite store + schema
- `EMP-004` FTS5 keyword retrieval adapter
- `EMP-005` Ollama embedder adapter
- `EMP-006` `sqlite-vec` vector index + hybrid retrieval
- `EMP-007` Fritz adapter + durable-memory ingest smoke path

## SUPERSEDED

## Notes

- treat `docs/memory-palace-storage-notes.md` as the design note for stack selection
- package boundary is part of the feature, not cleanup to defer
- keep first pass narrow: chunks, metadata, retrieval, and thin Fritz wiring only
- validated with:
  - `nix develop -c sh -lc 'export GOFLAGS="-tags=sqlite_fts5"; go test ./pkg/memorypalace/... ./internal/memory ./internal/prompt'`
  - `nix build .#fritz`
