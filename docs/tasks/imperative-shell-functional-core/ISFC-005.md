# ISFC-005: define effect-driven tool loop seam

**Type:** Task

## Goal

Create the core shell boundary needed for future tool-calling by representing model and tool work as effects instead of mixing decisions with execution.

## Places To Change

- `internal/chat/`
- `internal/tool/`
- `internal/app/app.go`

## Scope

- define effect/result types for `CallModel`, `RunTool`, `Print`, or similar minimal set
- make the chat/agent core return effects rather than directly doing work
- keep current behavior working even if only `CallModel` is exercised now
- add tests that assert emitted effects

## Non-goals

- actual coding tools
- autonomous multi-step tool loop
- streaming

## Design Notes

- this ticket is about the seam, not the full tool system
- effect types should be small and concrete, not a speculative framework
- prefer one interpreter in shell code over many callback layers

## Acceptance Criteria

- core can request external work without performing it directly
- current chat/run commands still work through the effect interpreter
- tests assert effect emission and state updates

## Dependencies

- `ISFC-002`
- `ISFC-003`

## Validation

- `go test ./...`
- `rg -n "type .*Effect|CallModel|RunTool" internal`
