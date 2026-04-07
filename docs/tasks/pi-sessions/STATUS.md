# pi-sessions status

## TODO

## IN_PROGRESS

## BLOCKED

## DONE

- `PIS-001` session file model and storage layout
- `PIS-002` append-only persistence and delayed flush
- `PIS-003` tree/path replay into model context
- `PIS-004` session discovery, continue, open, fork
- `PIS-005` runtime host for new/resume/fork switching
- `PIS-006` manual compaction pipeline
- `PIS-007` auto-compaction and overflow recovery
- `PIS-008` tree navigation and branch summarization
- `PIS-009` session metadata, stats, import/export, migrations

## Notes

- pack is Pi-informed, not Pi-clone-by-default
- first target for our repo is milestone 9 in `plan.md`
- config-file work remains separate in `plan.md` milestone 10
- first pass is intentionally narrower than Pi:
  - no TUI tree browser
  - no HTML export
  - migrations are minimal, version-header only
- validation green:
  - `go test ./...`
  - `uv run scripts/validate_pi_tool_test_parity.py`
