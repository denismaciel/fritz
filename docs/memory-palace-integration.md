# Memory Palace Integration

Concrete integration plan for the first Fritz memory-palace rollout.

## Goal

Add memory-palace support as an **optional subsystem**.

When disabled:

- Fritz behaves as today
- no indexing
- no retrieval tool/command

When enabled:

- Fritz persists selected runtime/chat material into the memory store
- Fritz can query that memory later

## Mental model

The system has 3 parts:

1. **control path**
   - feature on/off
   - DB path
   - embedding settings
   - indexing policy

2. **write path**
   - decide what becomes memory
   - normalize/chunk
   - write canonical rows
   - rebuild derived indexes

3. **read path**
   - query/search memory
   - return chunks/snippets/metadata
   - agent decides what to do with them

Important: embeddings are **not** the source of truth.

Source of truth is:

- canonical text chunks
- metadata
- source identity

Derived indexes are:

- FTS5
- vector rows

## Feature flag model

First pass should be explicit and boring.

Suggested runtime flags/config:

- `FRITZ_MEMORY_PALACE_ENABLED`
- `FRITZ_MEMORY_PALACE_DB`
- `FRITZ_GEMINI_EMBEDDING_MODEL`
- `FRITZ_GEMINI_EMBEDDING_DIMENSION`

Suggested behavior:

- default: disabled
- if disabled, no runtime cost except config parsing
- if enabled, Fritz opens the DB lazily on first memory use

## First write path

Do **not** start by saving every internal event.

Start with these sources only:

1. durable memory files
   - `MEMORY.md`
   - `memory/*.md`

2. session transcript turns
   - user message
   - assistant message
   - optional tool call/result summaries later

This keeps phase 1 narrow.

### What to persist

Canonical source types:

- `memory-file`
- `session-turn`

Canonical chunk metadata:

- source id
- session id, if any
- turn index, if any
- role
- path
- wing
- room
- timestamp
- content hash

### What not to persist yet

- raw streaming deltas
- every low-level runtime event
- debug logs
- giant tool outputs by default
- secrets

### Chunking

Phase 1:

- memory files: one file -> one chunk, unless obviously too large
- session turns: one turn pair or one message-sized unit -> one chunk

Phase 2:

- stable paragraph/window chunking for long sources

## First read path

Start with **explicit retrieval**, not automatic prompt injection.

That means:

1. command surface
   - `fritz memory index`
   - `fritz memory search "query"`

2. tool surface
   - a memory search tool the agent can call explicitly

This is the cleanest first product shape.

Why:

- easy to reason about
- easy to debug
- avoids silent prompt bloat
- avoids retrieval noise being injected automatically

## Agent-facing retrieval

First tool should do one thing:

- search memory by query

Input:

- query
- limit
- optional filters:
  - session id
  - source kind
  - wing
  - room

Output:

- chunk text
- source/path/session metadata
- scores

Do not make the first tool mutate memory.

Mutation/write should stay in runtime/indexing path first.

## Indexing timing

First pass should use **explicit indexing** plus a small automatic path.

Recommended order:

1. explicit:
   - `fritz memory index`
   - indexes durable memory files
   - can later also index stored sessions

2. automatic:
   - after session persistence/compaction boundaries
   - or at end of a turn batch / session checkpoint

Avoid re-embedding on every token.

Index at coarse boundaries:

- turn completed
- compaction completed
- session closed
- explicit command

## Retrieval policy

First policy:

- keyword-only if no embedder/vector capability
- hybrid if embeddings available
- no automatic retrieval unless user/tool asks

Later policy:

- optional prompt-time retrieval for selected profiles
- probably gateway/agent only

## Session integration

Recommended first session model:

- session manager remains source of runtime/session truth
- memory palace gets a derived copy for retrieval

So:

- do **not** replace session storage with the memory palace
- do **not** make memory palace own chat replay/resume semantics

It is a retrieval subsystem, not the whole session system.

## Proposed rollout

### Phase 1

- memory palace optional
- index durable memory files only
- add search command
- add search tool

### Phase 2

- index session turns
- add explicit session filters
- add incremental reindex by content hash

### Phase 3

- optional automatic retrieval into prompts
- guarded by config/profile

## Recommended immediate next tasks

1. add config flag for memory-palace enablement + DB path
2. add `fritz memory index`
3. add `fritz memory search`
4. add session-turn source model
5. add memory search tool for agents

## Non-goals for now

- replacing session storage
- writing every event to memory
- automatic prompt injection by default
- autonomous self-editing memory behavior
- knowledge graph integration

## Decision summary

The correct first integration is:

- **optional**
- **explicit**
- **derived from existing state**
- **search-first**

Not:

- mandatory
- automatic-everywhere
- embeddings-only
- replacement for the current session system
