# tool-runtime-upgrades status

## TODO

## IN_PROGRESS

## BLOCKED

## DONE

- `TRU-001` structured tool result payloads
- `TRU-002` provider-agnostic model/tool boundary
- `TRU-003` unix bash runtime upgrade
- `TRU-004` discovery tool hardening
- `TRU-005` injectable fs ops seams

## SUPERSEDED

## Notes

- this pack starts after `pi-tool-parity`
- parity validator remains useful even when ticket is structural
- no Windows work in this pack
- validation green:
  - `go test ./...`
  - `uv run scripts/validate_pi_tool_test_parity.py`
