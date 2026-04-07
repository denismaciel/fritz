# tool-runtime-upgrades task index

Upgrade the agent after Pi test-parity. Focus on cleaner runtime boundaries and the next feature-enabling seams, not more parity chasing.

Current status summary is in sibling `STATUS.md`.

## Assumptions

- current repo already has working Pi-style tool coverage
- Windows support is out of scope
- Gemini remains the only provider for now
- parity validator stays as guardrail during refactors
- tool package split is already done

## Engineering Constraints

- red, green, refactor. always
- preserve current behavior unless ticket says otherwise
- prefer typed data over `map[string]any`
- keep provider-specific shapes out of core types
- keep shell/io at edges
- keep tickets small, dependency-ordered, shippable
- validate with `go test ./...` after each ticket
- where relevant, also validate with `uv run scripts/validate_pi_tool_test_parity.py`

## Tasks

- `TRU-001.md`
- `TRU-002.md`
- `TRU-003.md`
- `TRU-004.md`
- `TRU-005.md`
