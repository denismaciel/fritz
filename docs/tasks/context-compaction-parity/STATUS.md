# context compaction parity status

## TODO

## IN_PROGRESS

## BLOCKED

## DONE

- `CCP-001` Codex parity validator + local compaction inventory
- `CCP-002` local compaction prompt templates + structured summary contract
- `CCP-003` token accounting + compaction config thresholds
- `CCP-004` pre-turn proactive compaction
- `CCP-005` compaction checkpoint persistence schema
- `CCP-006` checkpoint-based context rebuild + resume/fork parity
- `CCP-007` mid-turn compaction + overflow fallback cleanup
- `CCP-008` compaction fit trimming + repeated-compaction hardening

## SUPERSEDED

## OUT_OF_SCOPE

- Codex remote `/responses/compact` API behavior
- realtime restatement / start-end reinjection behavior
- thread API compact-start cases
- model-switch-to-smaller-context special pre-compaction path
- ghost snapshot semantics and other Codex-only transport artifacts
- parity beyond the adopted local scope in `PARITY_TESTS.md`

## Notes

- upstream prompt refs:
  - `internal/prompt/reference/codex/compact_prompt.md`
  - `internal/prompt/reference/codex/summary_prefix.md`
- upstream compaction test inventory:
  - `docs/reference/codex/compaction-tests.json`
- adopted parity scope:
  - `docs/tasks/context-compaction-parity/PARITY_TESTS.md`
- validation for this pack should include:
  - `go test ./...`
  - `uv run scripts/extract_codex_compaction_prompts_test.py`
  - `uv run scripts/extract_codex_compaction_tests_test.py`
  - `uv run scripts/validate_codex_compaction_test_parity_test.py`
  - `uv run scripts/validate_codex_compaction_test_parity.py`
