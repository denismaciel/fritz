# Reference Material

This directory holds source-derived or generated reference artifacts used by the implementation docs.

## Contents

- [`codex/compaction-tests.json`](codex/compaction-tests.json) — normalized inventory of Codex compaction tests used as a parity guardrail

## Related prompt references

Prompt/reference snapshots used by the runtime live outside this directory under [`../../internal/prompt/reference/`](../../internal/prompt/reference/):

- [`../../internal/prompt/reference/codex/`](../../internal/prompt/reference/codex/) — Codex-derived compaction prompt references
- [`../../internal/prompt/reference/local/`](../../internal/prompt/reference/local/) — local compaction prompt/reference files
- [`../../internal/prompt/reference/openclaw/`](../../internal/prompt/reference/openclaw/) — heartbeat and memory section references
- [`../../internal/prompt/reference/pi/`](../../internal/prompt/reference/pi/) — Pi-derived prompt template references

## Notes

Keep files here source-derived where possible so docs and tests can be refreshed from upstream references instead of hand-maintained copies.
