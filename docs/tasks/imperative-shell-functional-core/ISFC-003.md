# ISFC-003: add Gemini gateway boundary

**Type:** Task

## Goal

Define a narrow model gateway interface and request/response types so the core stops depending on the concrete Gemini client shape.

## Places To Change

- `internal/gemini/client.go`
- `internal/app/app.go`
- `internal/model/` or `internal/gateway/`

## Scope

- introduce typed request/response structs for single-turn generation
- keep HTTP details inside `internal/gemini`
- make shell/core depend on an interface, not direct concrete client methods
- add tests around request mapping and error translation

## Non-goals

- multi-provider support
- streaming transport
- tool-call support

## Design Notes

- the boundary should be Gemini-only but still abstract enough for tests
- keep the gateway small; avoid generic provider frameworks
- make failure modes explicit and boring

## Acceptance Criteria

- core orchestration depends on a narrow interface
- HTTP request details stay in `internal/gemini`
- tests do not need real network access

## Dependencies

- `ISFC-001`
- `ISFC-002`

## Validation

- `go test ./...`
- `go test ./internal/gemini/...`
