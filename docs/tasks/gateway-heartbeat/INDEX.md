# gateway heartbeat task index

Implement `G8` heartbeat v0 for the gateway wrapper. Goal: let the gateway wake periodically, decide if there is due work, invoke the engine, and deliver actionable output without mixing scheduler logic into Telegram or engine internals.

Current status summary is in the sibling `STATUS.md`.

## Assumptions

- gateway owns wake/scheduler decisions
- engine stays unaware of heartbeat
- Telegram is the only outbound channel for now
- no full cron subsystem in `G8`
- durable memory/state split from `G6`/`G7` is already in place

## Engineering Constraints

- keep IO at the edges; keep core logic testable
- prefer deep modules with narrow public APIs
- depend on interfaces/protocols, not concrete infrastructure
- validate untrusted input at boundaries
- avoid incidental refactors outside ticket scope
- bias toward stable, boring code over cleverness
- keep heartbeat generic; do not bury it in Telegram adapter
- coalesce duplicate wakes by target key
- make no-op behavior explicit and suppress outbound noise

## Tasks

- `GHB-001.md`
- `GHB-002.md`
- `GHB-003.md`
- `GHB-004.md`
- `GHB-005.md`
