# imperative-shell-functional-core task index

Refactor the agent toward imperative shell, functional core. Keep terminal, env, HTTP, fs, and subprocess work at the edges. Move chat, transcript, config merge, and later agent-loop decisions into pure or near-pure modules with narrow APIs.

Current status summary is in the sibling `STATUS.md`.

## Assumptions

- current repo is still early, so small structural changes are cheap
- Gemini-only remains the product scope for now
- command-line UX should stay minimal and boring
- no TUI work is in scope for this pack
- provider abstraction is explicitly out of scope

## Engineering Constraints

- keep IO at the edges; keep core logic testable
- prefer deep modules with narrow public APIs
- depend on interfaces/protocols, not concrete infrastructure
- validate untrusted input at boundaries
- avoid incidental refactors outside ticket scope
- bias toward stable, boring code over cleverness
- follow red, green, refactor on every ticket
- preserve working CLI behavior after each ticket

## Tasks

- `ISFC-001.md`
- `ISFC-002.md`
- `ISFC-003.md`
- `ISFC-004.md`
- `ISFC-005.md`
- `ISFC-006.md`
