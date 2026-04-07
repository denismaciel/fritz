# pi-sessions run loop

Read first:

- `docs/tasks/pi-sessions/INDEX.md`
- `docs/tasks/pi-sessions/STATUS.md`
- target ticket file

Rules:

- red, green, refactor
- smallest vertical slice first
- keep session semantics pure where possible, IO at shell edges
- leave Pi reference files in each ticket as source of truth

Validation baseline:

- `go test ./...`

Add ticket-specific validation as session code lands.
