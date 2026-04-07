# pi-tool-parity status

## TODO

## IN_PROGRESS

## BLOCKED

## DONE

- `PTP-001` path resolution parity for file tools
- `PTP-002` read tool parity
- `PTP-003` write tool parity
- `PTP-004` edit core parity
- `PTP-005` edit normalization parity
- `PTP-006` edit compatibility + mutation queue parity
- `PTP-007` bash parity
- `PTP-008` grep/find/ls parity

## SUPERSEDED

## Notes

- pack source: Pi tests under `/tmp/pi-mono/packages/coding-agent/test`
- exact Pi test names live in ticket checklists; tick as implemented
- omit TUI-only tests like `edit-tool-no-full-redraw` and `bash-execution-width`
- if a Pi test maps poorly to current Go shape, preserve behavior first, name second
- validated with `go test ./...` and `uv run scripts/validate_pi_tool_test_parity.py`
