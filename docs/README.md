# Docs

This directory collects project notes, feature docs, task packs, runbooks, and reference material.

## Start here

- Project overview: [`../README.md`](../README.md)
- High-level roadmap: [`../plan.md`](../plan.md)

## Feature and design docs

- [`context-compaction-plan.md`](context-compaction-plan.md) — notes on Codex-style compaction and the local implementation plan
- [`openai-codex-subscription-auth.md`](openai-codex-subscription-auth.md) — commands, config, and auth flow for `openai-codex`

## Task packs

Task packs live under [`tasks/`](tasks/README.md). Each pack is organized the same way:

- `INDEX.md` — scope, assumptions, constraints, and ticket list
- `STATUS.md` — current state of the pack
- `RUN_LOOP.md` — implementation loop notes/checklist
- `<PACK>-NNN.md` — individual tickets

See [`tasks/README.md`](tasks/README.md) for a directory-level index.

## Reference material

- [`reference/README.md`](reference/README.md) — generated inventories and source-derived references used by the docs and implementation work
- [`../internal/prompt/reference/`](../internal/prompt/reference/) — prompt/reference snapshots used by the runtime

## Runbooks

- [`runbooks/README.md`](runbooks/README.md) — home for operator and deployment procedures
