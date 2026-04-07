# embedded-go-terminal-ui status

## TODO

## IN_PROGRESS

## BLOCKED

## DONE

- `EGT-001` surface reasoning in internal runtime events
- `EGT-002` define terminal view-model for text, reasoning, tool calls
- `EGT-003` add Bubble Tea shell and transcript/input layout
- `EGT-004` wire embedded interactive mode to `internal/agent`
- `EGT-005` render compact tool outputs and reasoning blocks
- `EGT-006` docs, regression tests, and CLI switch-over

## SUPERSEDED

## Notes

- default local mode stays embedded and in-process
- `serve` remains optional and should not become required for terminal usage
- UI stack target:
  - `bubbletea`
  - `bubbles`
  - `lipgloss`
- validate with `go test ./...` after each ticket or coherent batch
- preserve AG-UI and AI SDK server paths while adding the local frontend
- implemented shape:
  - reasoning events in `internal/agent`, `internal/model`, `internal/gemini`
  - terminal reducer + Bubble Tea UI in `internal/terminalui`
  - tty local `chat` uses embedded UI
  - non-tty local `chat` keeps line-based fallback
