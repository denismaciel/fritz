# ISFC-002: extract transcript and chat state core

**Type:** Task

## Goal

Move chat history and prompt assembly into a pure chat/transcript core with explicit state transitions.

## Places To Change

- `internal/app/app.go`
- `internal/chat/`

## Scope

- define transcript and chat-turn types
- move `buildChatPrompt` and reset behavior into pure functions
- represent chat commands like input, reset, and quit explicitly
- test state transitions without using stdin/stdout

## Non-goals

- streaming
- sessions on disk
- tool calls

## Design Notes

- the shell should read lines and print output; the core should decide state changes
- prompt construction should depend only on transcript state and current input
- keep the core reusable for future session replay

## Acceptance Criteria

- chat prompt assembly no longer lives in `internal/app`
- chat reset/help/quit behavior is covered by focused tests
- `fritz chat` behavior remains unchanged

## Dependencies

- `ISFC-001`

## Validation

- `go test ./...`
- `GEMINI_API_KEY=test-key go test ./internal/app ./internal/chat/...`
