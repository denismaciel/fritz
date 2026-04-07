# prompt-layer-skills task index

Build the first real prompt layer for this agent. Keep it Pi-shaped, but narrower: repo instructions, system prompt files, and skills are in; prompt templates and extension hooks stay out for now.

Current status summary is in the sibling `STATUS.md`.

## Assumptions

- skills are required in the first prompt-layer cut, not optional later work
- prompt templates are explicitly deferred
- first pass can be narrower than Pi on package discovery and UI surfacing
- repo/global file discovery is enough; no extension system work in this pack
- red, green, refactor always

## Engineering Constraints

- keep IO at the edges; keep core logic testable
- prefer deep modules with narrow public APIs
- depend on interfaces/protocols, not concrete infrastructure
- validate untrusted input at boundaries
- avoid incidental refactors outside ticket scope
- bias toward stable, boring code over cleverness
- keep provider code unaware of AGENTS/SYSTEM/skills loading details
- keep prompt assembly deterministic from typed inputs

## Tasks

- `PLS-001.md`
- `PLS-002.md`
- `PLS-003.md`
- `PLS-004.md`
- `PLS-005.md`
- `PLS-006.md`
