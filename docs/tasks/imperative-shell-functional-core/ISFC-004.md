# ISFC-004: refactor config into typed mergeable core

**Type:** Task

## Goal

Replace ad hoc env-only config handling with a typed config core that can later merge defaults, file config, env, and flags.

## Places To Change

- `internal/config/config.go`
- `internal/config/`
- `internal/app/app.go`
- `plan.md`

## Scope

- define typed runtime config structs
- split raw env loading from normalized config resolution
- make precedence rules explicit, even if config-file support is still stubbed
- add tests for merge and validation behavior

## Non-goals

- full config-file implementation if it would bloat the ticket
- adding many new user-facing config fields

## Design Notes

- config resolution is a pure core concern
- env/file/flags are shell inputs; normalized config is the core output
- start with model id, API key, and chat/runtime knobs already implied by current code

## Acceptance Criteria

- config code separates raw sources from resolved config
- merge/validation behavior is tested
- current commands keep working

## Dependencies

- `ISFC-001`

## Validation

- `go test ./...`
- `go test ./internal/config/...`
