# agent-stream-api status

## TODO

## IN_PROGRESS

## BLOCKED

## DONE

- `ASA-001` extract headless agent service boundary
- `ASA-002` define internal event model and run lifecycle ids
- `ASA-003` add cancellation and run registry
- `ASA-004` implement AG-UI SSE encoder
- `ASA-005` add HTTP server endpoints for runs and cancel
- `ASA-006` add thin CLI client over AG-UI SSE
- `ASA-007` add AI SDK UI message stream adapter
- `ASA-008` conformance tests, docs, examples

## SUPERSEDED

## Notes

- canonical protocol is AG-UI SSE; AI SDK stream is adapter-only
- source of truth for protocol shape is the referenced external docs plus the pack tickets
- validate with `go test ./...` after each ticket or coherent batch
- keep server/runtime seams typed and provider-agnostic
- do not couple browser-facing protocol structs to `internal/chat` or `internal/model`
- implemented shape:
  - headless runtime in `internal/agent`
  - AG-UI encoder in `internal/protocol/agui`
  - AI SDK encoder in `internal/protocol/aisdk`
  - HTTP endpoints in `internal/httpapi`
  - thin remote CLI via `--server`
