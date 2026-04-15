# Task Packs

This directory groups longer-running work into themed packs. Each pack has an `INDEX.md`, `STATUS.md`, `RUN_LOOP.md`, and one ticket file per step.

## Packs

- [`agent-stream-api/`](agent-stream-api/INDEX.md) — expose the runtime as a multi-client agent API over AG-UI SSE
- [`context-compaction-parity/`](context-compaction-parity/INDEX.md) — bring local compaction closer to `openai/codex` in small, testable steps
- [`embedded-go-terminal-ui/`](embedded-go-terminal-ui/INDEX.md) — build the default in-process Go terminal UI
- [`gateway-heartbeat/`](gateway-heartbeat/INDEX.md) — add heartbeat wake/scheduling behavior to the gateway wrapper
- [`gateway-state-memory/`](gateway-state-memory/INDEX.md) — separate gateway ops state from durable memory files
- [`imperative-shell-functional-core/`](imperative-shell-functional-core/INDEX.md) — push IO to the edges and move logic into testable core modules
- [`openai-codex-subscription-auth/`](openai-codex-subscription-auth/INDEX.md) — add ChatGPT subscription-backed `openai-codex` auth and SSE transport
- [`pi-sessions/`](pi-sessions/INDEX.md) — map Pi-style session storage, replay, resume, fork, and compaction into Go
- [`pi-tool-parity/`](pi-tool-parity/INDEX.md) — port Pi tool behavior using exact Pi test names as guardrails
- [`polish-stabilization/`](polish-stabilization/INDEX.md) — tighten errors, retries, packaging, docs, and regression coverage
- [`prompt-layer-skills/`](prompt-layer-skills/INDEX.md) — add repo instructions, system prompt files, and skills
- [`secret-store-v0/`](secret-store-v0/INDEX.md) — add harness-owned named secret storage without leaking plaintext into docs or session files
- [`slack-native-gateway/`](slack-native-gateway/INDEX.md) — replace Bill Nick with a native Fritz Slack transport and gateway
- [`tool-runtime-upgrades/`](tool-runtime-upgrades/INDEX.md) — clean up runtime seams after tool parity lands

## Conventions

Use each pack's files as follows:

- start at `INDEX.md` for scope and ticket order
- check `STATUS.md` before starting work
- use `RUN_LOOP.md` for implementation sequencing and validation notes
- keep ticket files small, dependency-ordered, and shippable
