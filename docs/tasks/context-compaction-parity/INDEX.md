# context compaction parity task index

Bring `fritz` compaction closer to `openai/codex` in small, testable steps. Keep prompt refs and test-name parity inventories source-derived from upstream code, not hand-copied notes.

Current status summary is in the sibling `STATUS.md`.

## Assumptions

- source-derived Codex compaction prompt refs live under `internal/prompt/reference/codex/`
- source-derived Codex compaction test inventory lives in `docs/reference/codex/compaction-tests.json`
- the local agent does not need Codex realtime or remote `/responses/compact` behavior for parity
- prompt extraction and test inventory extraction stay programmatic and refreshable
- local parity means normalized test-name coverage for Codex compaction tests in our Go codebase

## Engineering Constraints

- keep IO at the edges; keep core logic testable
- prefer deep modules with narrow public APIs
- depend on interfaces/protocols, not concrete infrastructure
- validate untrusted input at boundaries
- avoid incidental refactors outside ticket scope
- bias toward stable, boring code over cleverness
- use red-green-refactor on every ticket:
  - add or update failing tests first
  - make the smallest change to go green
  - refactor only after green
- preserve backward compatibility for existing session files unless a ticket explicitly migrates them
- prompts must be extracted from source code or dedicated local prompt templates, not embedded ad hoc in tests/docs
- Codex compaction parity inventory scripts are guardrails, not optional docs
- every ticket that lands behavior must either add mirrored local tests or reduce parity-script failures

## Tasks

- `CCP-001.md`
- `CCP-002.md`
- `CCP-003.md`
- `CCP-004.md`
- `CCP-005.md`
- `CCP-006.md`
- `CCP-007.md`
- `CCP-008.md`
