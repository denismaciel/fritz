# ISFC-006: add session and compaction planning core

**Type:** Task

## Goal

Prepare for persistent sessions and later compaction by introducing pure planning logic for transcript persistence and compaction decisions before adding file IO.

## Places To Change

- `internal/session/`
- `internal/chat/`
- `internal/app/app.go`
- `plan.md`

## Scope

- define serializable transcript/session types
- add pure decisions for when to start new session state, resume state, and compact state
- keep file IO out of the core for now
- add tests for session and compaction decision logic

## Non-goals

- writing session files
- implementing summarization
- tree branching

## Design Notes

- this is planning logic only, not persistence wiring
- separate “should compact?” from “how to compact?” from “where to store session?”
- keep future session JSONL format choices out of the core until needed

## Acceptance Criteria

- session/compaction decisions live in pure or near-pure modules
- tests cover decision points
- future session IO can plug in without reshaping chat core APIs

## Dependencies

- `ISFC-002`
- `ISFC-004`
- `ISFC-005`

## Validation

- `go test ./...`
- `go test ./internal/session/...`
