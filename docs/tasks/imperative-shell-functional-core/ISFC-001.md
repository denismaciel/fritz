# ISFC-001: split command parsing from CLI IO

**Type:** Task

## Goal

Move command parsing and dispatch decisions out of `internal/app` shell code into a small pure command layer.

## Places To Change

- `internal/app/app.go`
- `cmd/fritz/main.go`
- `internal/command/`

## Scope

- add a typed command model for `help`, `doctor`, `run`, and `chat`
- parse argv into command values in a pure package
- keep stdin/stdout/env handling in `internal/app`
- add tests for parse behavior and invalid invocations

## Non-goals

- changing CLI features
- adding flags beyond what is needed for current behavior
- restructuring Gemini or chat logic

## Design Notes

- command parsing should not touch env, stdin, stdout, or network
- `internal/app` should orchestrate IO from already-parsed commands
- prefer a narrow parse API like `Parse([]string) (Command, error)`

## Acceptance Criteria

- command parsing lives outside `internal/app`
- parse tests cover valid and invalid inputs
- current CLI behavior remains intact

## Dependencies

- `None.`

## Validation

- `go test ./...`
- `go run ./cmd/fritz help`
- `go run ./cmd/fritz run chat || true`
