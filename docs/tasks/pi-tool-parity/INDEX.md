# pi-tool-parity task index

Port Pi tool behavior into this Go agent, grounded in exact Pi test names. Focus on tool semantics and thin agent integration, not Pi's TUI or package/runtime surface.

Current status summary is in sibling `STATUS.md`.

## Assumptions

- `/tmp/pi-mono/packages/coding-agent` is source of truth for parity
- repo already has partial `read`/`write`/`edit`/`bash`
- `grep`/`find`/`ls` are still missing locally
- TUI-only regressions stay out unless they prove core tool semantics
- exact Pi test names stay in ticket checklists; agents tick as landed

## Engineering Constraints

- red, green, refactor. always
- port behavior from Pi before adding local polish
- keep imperative shell, functional core boundaries intact
- keep tickets small, dependency-ordered, shippable
- prefer stdlib + boring code
- preserve working CLI after each ticket
- update ticket checklists and `STATUS.md` as work lands

## Tasks

- `PTP-001.md`
- `PTP-002.md`
- `PTP-003.md`
- `PTP-004.md`
- `PTP-005.md`
- `PTP-006.md`
- `PTP-007.md`
- `PTP-008.md`
