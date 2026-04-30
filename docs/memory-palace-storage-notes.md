# Memory Palace Storage Notes

Notes from initial research on reimplementing MemPalace-like concepts in Go.

## Goal

Build a local-first memory subsystem for Fritz with concepts roughly like:

- `wing` = project/person scope
- `room` = topic bucket
- `drawer` = canonical verbatim chunk
- `closet` = cheap derived retrieval artifact / pointer doc

Keep it isolated enough that we can promote it out of this repo later.

## What MemPalace appears to do

From repo read + docs:

- canonical data is verbatim text chunks
- retrieval is vector search with some hybrid ranking
- default backend is ChromaDB
- no outside embedding API in normal path
- vectors come from ChromaDB default embeddings
- docs/benchmarks name that default as `all-MiniLM-L6-v2`
- optional LLM use exists for rerank / extra flows, not base indexing

Implication: their strongest idea is not "fancy extraction". It is raw text + decent local retrieval.

## Design constraints

### 1. Retrieval must sit behind an interface

We should assume we may swap:

- embedding provider
- vector index
- hybrid/keyword index

Do not bind app semantics to one storage engine.

### 2. Keep canonical store separate from retrieval index

Split concerns:

1. `MemoryStore`
   - source of truth
   - chunks, metadata, rooms/wings, links, graph facts, audit log
2. `Embedder`
   - text -> vector
3. `RetrievalIndex`
   - upsert/delete/search over vectors and maybe hybrid ranking

This keeps reindexing, model swaps, and backend swaps cheap relative to app logic.

### 3. Package must be promotable out of repo

Treat this as a product module, not helper code.

Rules:

- narrow public API
- no Fritz app types in exported surface
- no env/config loading in core package
- no CLI / MCP / UI code in core package
- no repo-specific logging assumptions
- avoid leaking third-party types in public interfaces

Likely shape:

```text
pkg/memorypalace
pkg/memorypalace/sqlite
pkg/memorypalace/bleve
pkg/memorypalace/gemini
```

Or, if it grows:

```text
memorypalace/core
memorypalace/sqlite
memorypalace/bleve
memorypalace/gemini
```

If promotion looks real, give it its own `go.mod` early.

## Suggested interface split

Sketch only:

```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dim() int
    Name() string
}

type RetrievalIndex interface {
    Upsert(ctx context.Context, docs []IndexedDoc) error
    Delete(ctx context.Context, ids []string) error
    Search(ctx context.Context, req SearchRequest) ([]SearchHit, error)
}

type MemoryStore interface {
    PutChunk(ctx context.Context, c Chunk) error
    GetChunk(ctx context.Context, id string) (Chunk, error)
    ListBySource(ctx context.Context, sourceID string) ([]Chunk, error)
}
```

Then compose them in one service:

```go
type MemoryEngine struct {
    store    MemoryStore
    embedder Embedder
    index    RetrievalIndex
}
```

Do not over-generalize. Keep a small core interface, then add optional capability interfaces for hybrid search, metadata filtering, namespace support, bulk rebuilds, etc.

## Backend options checked

### 1. SQLite + `sqlite-vec` + FTS5

Best current fit.

Why:

- embedded
- local file
- simple deployment
- SQL for canonical metadata
- FTS5 for keyword/BM25
- vectors in same stack
- good shape for wings/rooms/drawers/graph tables

Likely best default architecture for Fritz.

### 2. Bleve

Strong option when retrieval quality matters more than SQL simplicity.

Why:

- Go-native
- embedded
- keyword search
- vector search
- hybrid retrieval support

Best used either:

- as primary retrieval engine, with SQLite as canonical store
- or as a secondary index over canonical SQLite rows

### 3. `chromem-go`

Interesting because it is explicitly Chroma-like and embeddable in Go.

Why it is attractive:

- Go-native
- simple shape
- close to the mental model we already discussed

Current caution:

- less mature than SQLite/Bleve paths
- exact cosine search today; ANN/HNSW is roadmap

Good prototype path. Less clear as long-term default.

### 4. DuckDB + VSS

Interesting, but probably wrong as the default live memory engine.

Why:

- DuckDB has official `vss` extension with HNSW
- DuckDB is excellent for analytical workloads

Concern:

- DuckDB docs/blog note it is not optimized for point queries
- our main workload is likely interactive lookup, not analytical batch work

Maybe useful later for:

- offline evaluation
- mining jobs
- batch reranking / analysis
- benchmark tooling

Not my first choice for the primary runtime store.

### 5. Weaviate

Written in Go, but wrong shape here.

Reason:

- server/cluster product
- not the simplest embedded local-app fit

Good system; likely not the right one for this constraint set.

## Embeddings engine options

### Gemini embeddings

Best near-term default for Fritz.

Why:

- already a first-class provider in Fritz
- shared endpoint/api-key handling with the rest of the repo
- no extra local runtime to provision
- swappable behind the same `Embedder` interface

Treat Gemini as one `Embedder` impl, not as a hard dependency of the whole module.

## Current recommendation

If building now:

1. canonical store in SQLite
2. retrieval via either:
   - `sqlite-vec` + FTS5 first, or
   - Bleve if we want stronger retrieval features sooner
3. embeddings via Gemini API
4. optional reranker later

Practical bias:

- **store is stable**
- **retrieval is swappable**
- **embedding is swappable**

The app should depend on memory semantics, not index quirks.

## Implemented first stack in this repo

Current implementation path landed in Fritz:

- core package: `pkg/memorypalace`
- canonical + retrieval adapter: `pkg/memorypalace/sqlite`
- embedder adapter: `pkg/memorypalace/gemini`
- Fritz-local durable-memory adapter: `internal/memory`

Concrete runtime choices:

- SQLite driver: `github.com/mattn/go-sqlite3`
- `sqlite-vec` integration: `github.com/asg017/sqlite-vec-go-bindings/cgo`
- keyword retrieval: FTS5
- vector retrieval: `vec0`
- embedding provider: Gemini embeddings API

Build/runtime notes:

- FTS5 is enabled with Go build tag `sqlite_fts5`
- Nix shell/package now declare SQLite + compiler deps explicitly
- vector support is gated by the SQLite adapter capability probe and can fall back to keyword-only search

## Initial package boundary guidance

Inside promotable package:

- domain types
- interfaces
- orchestration service
- schema/migrations
- chunking/ingest logic if generic
- retrieval adapters

Outside promotable package:

- Fritz command wiring
- Fritz config/env loading
- Fritz prompts and agent policy
- UI / HTTP / Slack / Telegram bindings
- repo-specific logging and metrics adapters

## Sources

- MemPalace repo: <https://github.com/MemPalace/mempalace>
- chromem-go: <https://github.com/philippgille/chromem-go>
- sqlite-vec: <https://alexgarcia.xyz/sqlite-vec/>
- sqlite-vec repo: <https://github.com/asg017/sqlite-vec>
- Bleve: <https://blevesearch.com/>
- Bleve package docs: <https://pkg.go.dev/github.com/blevesearch/bleve/v2>
- DuckDB VSS docs: <https://duckdb.org/docs/current/core_extensions/vss>
- DuckDB VSS notes: <https://duckdb.org/2024/10/23/whats-new-in-the-vss-extension>
- Weaviate architecture: <https://docs.weaviate.io/contributor-guide/weaviate-modules/architecture>
- Gemini embeddings docs: <https://ai.google.dev/api/embeddings>
