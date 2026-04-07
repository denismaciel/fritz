# polish-stabilization task index

Stabilize the current agent without widening product scope. Focus on boring quality: clearer errors, bounded retries, packaging/install, tighter docs, and regression coverage.

Current status summary is in the sibling `STATUS.md`.

## Assumptions

- no new major subsystems in this pack
- no provider abstraction work
- no new tools beyond current set
- no TUI or interactive UI expansion
- red, green, refactor always

## Engineering Constraints

- keep IO at the edges; keep core logic testable
- prefer deep modules with narrow public APIs
- depend on interfaces/protocols, not concrete infrastructure
- validate untrusted input at boundaries
- avoid incidental refactors outside ticket scope
- bias toward stable, boring code over cleverness
- retries must be explicit, bounded, and narrow
- docs must describe actual behavior only

## Tasks

- `POL-001.md`
- `POL-002.md`
- `POL-003.md`
- `POL-004.md`
- `POL-005.md`
- `POL-006.md`
