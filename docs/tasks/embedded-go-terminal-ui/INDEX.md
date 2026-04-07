# embedded-go-terminal-ui task index

Build the default local terminal frontend in Go, in-process, Pi-style. Use the existing `internal/agent` runtime as the source of truth. The first real UI must show streamed assistant text, compact tool calls/results, and reasoning pieces.

Current status summary is in the sibling `STATUS.md`.

## Assumptions

- default local usage should stay in-process; `serve` remains optional integration mode
- the current `internal/agent` runtime is the correct base seam for the UI
- use Go only; no Node sidecar, browser dependency, or custom terminal protocol
- use Charm stack for the frontend: `bubbletea`, `bubbles`, `lipgloss`
- tool calls must be visible in the UI, with reduced previews when output is large
- reasoning must be surfaced as explicit UI content, not silently discarded
- red, green, refactor always

## Engineering Constraints

- keep IO at the edges; keep core logic testable
- prefer deep modules with narrow public APIs
- depend on interfaces/protocols, not concrete infrastructure
- validate untrusted input at boundaries
- avoid incidental refactors outside ticket scope
- bias toward stable, boring code over cleverness
- keep runtime/core packages UI-agnostic
- define a terminal view-model layer between agent events and Bubble Tea state
- preserve existing `run`, `chat`, `serve`, `--server` behavior while the embedded UI lands
- start simple; avoid building a custom TUI framework

## Tasks

- `EGT-001.md`
- `EGT-002.md`
- `EGT-003.md`
- `EGT-004.md`
- `EGT-005.md`
- `EGT-006.md`
