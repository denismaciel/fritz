# pi-sessions task index

Map Pi's session impl into incremental Go work. Keep scope on session storage, resume/fork/tree, and compaction. Skip TUI-only UI.

## Pi architecture map

Pi session system is split across a few real layers:

1. file format + append-only storage
- JSONL session file
- header + typed entries
- append-only writes
- delayed flush until first assistant msg

2. session graph model
- entries have `id` + `parentId`
- one file can hold many branches
- current position is `leafId`
- branch switch is pointer move, not file rewrite

3. context reconstruction
- LLM context is rebuilt from branch path, not raw file order
- compaction + branch summary entries alter rebuilt context
- model/thinking changes also come from path replay

4. runtime switching
- `/new`, `/resume`, `/fork`, import
- `AgentSessionRuntimeHost` replaces active runtime/session cleanly

5. compaction + branch summarization
- manual compaction
- auto compaction on threshold / overflow
- branch summary when switching trees

6. session metadata + ops
- list sessions
- continue recent
- session name
- stats
- migrations

## Ticket order

- `PIS-001` session file model and storage layout
- `PIS-002` append-only persistence and delayed flush
- `PIS-003` tree/path replay into model context
- `PIS-004` session discovery, continue, open, fork
- `PIS-005` runtime host for new/resume/fork switching
- `PIS-006` manual compaction pipeline
- `PIS-007` auto-compaction and overflow recovery
- `PIS-008` tree navigation and branch summarization
- `PIS-009` session metadata, stats, import/export, migrations

## Constraints

- red, green, refactor always
- each ticket ends in usable state
- do not pull in Pi extension system unless ticket explicitly needs the seam
- keep first pass narrower than Pi where it helps delivery

## Shared Pi references

- [`/tmp/pi-mono/packages/coding-agent/README.md`](/tmp/pi-mono/packages/coding-agent/README.md)
- [`/tmp/pi-mono/packages/coding-agent/docs/session.md`](/tmp/pi-mono/packages/coding-agent/docs/session.md)
- [`/tmp/pi-mono/packages/coding-agent/docs/compaction.md`](/tmp/pi-mono/packages/coding-agent/docs/compaction.md)
- [`/tmp/pi-mono/packages/coding-agent/src/core/session-manager.ts`](/tmp/pi-mono/packages/coding-agent/src/core/session-manager.ts)
- [`/tmp/pi-mono/packages/coding-agent/src/core/agent-session.ts`](/tmp/pi-mono/packages/coding-agent/src/core/agent-session.ts)
- [`/tmp/pi-mono/packages/coding-agent/src/core/agent-session-runtime.ts`](/tmp/pi-mono/packages/coding-agent/src/core/agent-session-runtime.ts)
