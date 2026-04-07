# imperative-shell-functional-core status

## TODO

## BLOCKED

## DONE

- `ISFC-001` split command parsing from CLI IO
- `ISFC-002` extract transcript and chat state core
- `ISFC-003` add Gemini gateway boundary
- `ISFC-004` refactor config into typed mergeable core
- `ISFC-005` define effect-driven tool loop seam
- `ISFC-006` add session and compaction planning core

## SUPERSEDED

## Notes

- keep tickets small enough to land without freezing feature work
- after each ticket, `go test ./...` must stay green
- prefer new internal packages over widening `internal/app`
- validated `ISFC-001` with `go test ./...`, `go run ./cmd/fritz help`, and `go run ./cmd/fritz run chat`
- validated the remaining pack with `go test ./...`, `go run ./cmd/fritz help`, `go run ./cmd/fritz run chat`, and `rg -n "type .*Effect|CallModel|RunTool" internal`
