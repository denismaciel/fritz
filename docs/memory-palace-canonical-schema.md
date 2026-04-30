# Memory Palace Canonical Schema

Minimal Fritz-native schema for memory storage.

This is the shape I would target next.

It keeps:

- canonical runtime/doc records as source of truth
- derived retrieval chunks as a separate layer
- FTS/vector indexes as disposable rebuildable state

It does **not** copy MemPalace's closets, room inference, or normalization pipeline.

## Design rules

1. canonical rows first
2. retrieval chunks derived deterministically
3. embeddings never source of truth
4. package stays promotable out of Fritz
5. session/runtime semantics stay explicit

## Minimal canonical tables

### `memory_sources`

Top-level source object.

Examples:

- one chat/session
- one durable memory file
- one imported document

Suggested columns:

- `source_id TEXT PRIMARY KEY`
- `kind TEXT NOT NULL`
  - `session`
  - `memory_file`
  - `document`
  - `note`
- `scope TEXT NOT NULL`
  - project/workspace/user scope
- `title TEXT NOT NULL`
- `path TEXT NOT NULL DEFAULT ''`
- `external_ref TEXT NOT NULL DEFAULT ''`
- `content_hash TEXT NOT NULL DEFAULT ''`
- `created_at TEXT NOT NULL`
- `updated_at TEXT NOT NULL`
- `metadata_json TEXT NOT NULL`

Notes:

- `kind` is app-level source type
- `scope` replaces overloading `wing` as a core schema concept
- `metadata_json` is escape hatch, not primary design tool

### `memory_entries`

Canonical records inside a source.

This is the main table.

Suggested columns:

- `entry_id TEXT PRIMARY KEY`
- `source_id TEXT NOT NULL REFERENCES memory_sources(source_id) ON DELETE CASCADE`
- `seq INTEGER NOT NULL`
- `parent_entry_id TEXT NOT NULL DEFAULT ''`
- `kind TEXT NOT NULL`
  - `user_message`
  - `assistant_message`
  - `tool_call`
  - `tool_result`
  - `system_message`
  - `document_body`
  - `note`
- `role TEXT NOT NULL DEFAULT ''`
- `name TEXT NOT NULL DEFAULT ''`
  - tool name, doc section label, etc.
- `status TEXT NOT NULL DEFAULT ''`
  - useful for tool call lifecycle
- `text TEXT NOT NULL`
- `payload_json TEXT NOT NULL`
- `content_hash TEXT NOT NULL`
- `event_at TEXT NOT NULL`
- `created_at TEXT NOT NULL`
- `metadata_json TEXT NOT NULL`
- `UNIQUE(source_id, seq)`

Notes:

- `text` is canonical human-readable content
- `payload_json` holds structured args/results when needed
- `parent_entry_id` links tool result to tool call, assistant reply to prior call, etc.
- for a file import, one `document_body` entry is fine in v1

## Derived retrieval tables

### `memory_chunks`

Retrieval units derived from one or more canonical entries.

Suggested columns:

- `chunk_id TEXT PRIMARY KEY`
- `source_id TEXT NOT NULL REFERENCES memory_sources(source_id) ON DELETE CASCADE`
- `ordinal INTEGER NOT NULL`
- `chunk_kind TEXT NOT NULL`
  - `message`
  - `turn`
  - `tool_summary`
  - `document_span`
- `rendered_text TEXT NOT NULL`
- `content_hash TEXT NOT NULL`
- `start_seq INTEGER NOT NULL`
- `end_seq INTEGER NOT NULL`
- `metadata_json TEXT NOT NULL`
- `created_at TEXT NOT NULL`
- `updated_at TEXT NOT NULL`
- `UNIQUE(source_id, ordinal)`

### `memory_chunk_entries`

Link table from chunk back to exact canonical entries.

Suggested columns:

- `chunk_id TEXT NOT NULL REFERENCES memory_chunks(chunk_id) ON DELETE CASCADE`
- `entry_id TEXT NOT NULL REFERENCES memory_entries(entry_id) ON DELETE CASCADE`
- `ordinal INTEGER NOT NULL`
- `PRIMARY KEY(chunk_id, entry_id)`

## Disposable index layer

### `memory_chunks_fts`

FTS5 over `memory_chunks.rendered_text`.

Suggested indexed fields:

- `chunk_id`
- `source_id`
- `scope`
- `kind`
- `rendered_text`

### `memory_chunk_vec`

Vector table keyed by `chunk_id`.

Suggested columns:

- `chunk_id`
- `source_id`
- embedding vector

Plus meta keys:

- embedder name
- vector dimension
- schema/index version

## How derivation works

Canonical rows are written once.

Retrieval rows are rebuilt from canonical rows by policy.

### Session sources

Good v1 policy:

- one user message => one entry
- one assistant message => one entry
- one tool call => one entry
- one tool result => one entry

Good v1 chunk policy:

- short path: one message per chunk
- better path: one `turn` chunk per user message plus adjacent assistant/tool activity

Example rendered chunk:

```text
user: find previous discussion about sqlite vec
assistant: use sqlite + sqlite-vec + FTS5 as first stack
tool_call(search_memory): query="sqlite vec"
tool_result(search_memory): 3 hits from durable memory
```

### Document sources

Good v1 policy:

- one imported file => one `memory_source`
- one file body => one `document_body` entry
- derive one or more `document_span` chunks by stable paragraph/window chunking

## What to keep out of canonical storage

Do not store these by default:

- token deltas
- transport logs
- giant raw tool outputs
- transient debug noise
- inferred facts/graphs as source-of-truth records

Those can exist later as optional derived products.

## Why this is better than copying MemPalace

It keeps the useful parts:

- source-of-truth text
- metadata
- deterministic chunking
- rebuildable indexes

And drops the parts Fritz does not need:

- arbitrary transcript normalization
- metaphor-driven storage model
- heuristic hall/room/entity extraction
- secondary closet docs

## Relation to current package

Current package already has the right split in spirit:

- `sources`
- `chunks`
- FTS
- vectors

But it is still chunk-first.

Next refinement should be:

1. keep current retrieval path working
2. add canonical `memory_entries`
3. derive `memory_chunks` from entries
4. let indexing rebuild from canonical rows

That gives us a stable promotable core without locking app semantics to today's chunk format.
