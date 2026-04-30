# embedded-memory-palace task index

Build a first in-process memory-palace stack for Fritz using SQLite as canonical store, FTS5 for keyword retrieval, `sqlite-vec` for vector retrieval, and Ollama for local embeddings. Keep the module isolated enough to promote out of this repo later.

Current status summary is in the sibling `STATUS.md`.

## Assumptions

- first stack must stay fully in process; no server/client split
- SQLite is the canonical store for chunks, metadata, and future graph-like records
- retrieval must sit behind interfaces so index and embedder impls can change
- public package surface must stay Fritz-agnostic and promotable
- first implementation should target durable memory and small local corpora before broad agent-wide ingestion
- FTS-first fallback is acceptable while vector pieces land

## Engineering Constraints

- keep IO at the edges; keep core logic testable
- prefer deep modules with narrow public APIs
- depend on interfaces/protocols, not concrete infrastructure
- validate untrusted input at boundaries
- avoid incidental refactors outside ticket scope
- bias toward stable, boring code over cleverness
- keep canonical store separate from retrieval index semantics
- do not leak SQLite, Ollama, or third-party types in the core public API
- keep Fritz-specific wiring out of the promotable package
- keep schema and migrations explicit, deterministic, and test-covered
- preserve existing `MEMORY.md` / `memory/*.md` behavior while new indexing lands

## Tasks

- `EMP-001.md`
- `EMP-002.md`
- `EMP-003.md`
- `EMP-004.md`
- `EMP-005.md`
- `EMP-006.md`
- `EMP-007.md`
