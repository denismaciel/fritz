# gateway state + memory task index

Formalize `G6` and `G7` for the gateway wrapper. Goal: split ops state from transcripts, then add durable file-backed memory without mixing it into routing/runtime stores.

Current status summary is in the sibling `STATUS.md`.

## Assumptions

- session transcripts stay under the existing `.fritz/sessions/` machinery
- gateway-owned mutable state lives under `.fritz/gateway/`
- durable memory is file-backed and inspectable, not vector-backed
- Telegram is the only real channel for now
- no multi-agent or multi-account support yet

## Engineering Constraints

- keep IO at the edges; keep core logic testable
- prefer deep modules with narrow public APIs
- depend on interfaces/protocols, not concrete infrastructure
- validate untrusted input at boundaries
- avoid incidental refactors outside ticket scope
- bias toward stable, boring code over cleverness
- keep transcript/session persistence separate from gateway ops state
- keep gateway ops state separate from durable memory files
- use versioned JSON files for mutable stores
- use atomic writes for mutable stores

## Tasks

- `GSM-001.md`
- `GSM-002.md`
- `GSM-003.md`
- `GSM-004.md`
- `GSM-005.md`
- `GSM-006.md`
