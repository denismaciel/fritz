# gateway state + memory status

## TODO

## IN_PROGRESS

## BLOCKED

## DONE

- `GSM-001` state paths + atomic JSON store
- `GSM-002` split routing/session map store
- `GSM-003` split Telegram runtime stores
- `GSM-004` gateway state metadata + reserved bindings layout
- `GSM-005` durable memory filesystem layout + loader
- `GSM-006` durable memory prompt wiring + explicit write path

## SUPERSEDED

## Notes

- validated with `go test ./...`
- validated with `uv run scripts/validate_pi_tool_test_parity.py`
- `MEMORY.md` and `memory/*.md` stay out of mutable ops-state codepaths
- OpenClaw refs used for layout shape only
