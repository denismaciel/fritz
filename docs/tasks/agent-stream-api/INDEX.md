# agent-stream-api task index

Build a multi-client agent API around the existing runtime. Canonical protocol is AG-UI over SSE. Add a thin AI SDK UI stream adapter after the canonical path is in place.

Current status summary is in the sibling `STATUS.md`.

## Assumptions

- the existing chat/model/session/prompt/tool layers are good enough to reuse; this pack should extract and expose them, not redesign them
- canonical backend protocol is `AG-UI` over `text/event-stream`
- AI SDK compatibility is valuable, but only as an adapter over the same internal event stream
- initial transport is HTTP for browser/client reach; stdio RPC can come later if still wanted
- browser/client code stays out of core runtime packages
- red, green, refactor always

## Engineering Constraints

- keep IO at the edges; keep core logic testable
- prefer deep modules with narrow public APIs
- depend on interfaces/protocols, not concrete infrastructure
- validate untrusted input at boundaries
- avoid incidental refactors outside ticket scope
- bias toward stable, boring code over cleverness
- define one internal event model first; do not let protocol types leak into runtime/core packages
- preserve current CLI behavior while adding API/server entrypoints
- make cancellation and failure states explicit in typed APIs

## Tasks

- `ASA-001.md`
- `ASA-002.md`
- `ASA-003.md`
- `ASA-004.md`
- `ASA-005.md`
- `ASA-006.md`
- `ASA-007.md`
- `ASA-008.md`
