# openai codex subscription auth task index

Bring ChatGPT subscription-backed `openai-codex` auth and transport into `fritz` in small, testable steps. Keep first pass boring: small provider seam, locked auth storage, OAuth login, request-auth resolver, SSE transport, then runtime wiring.

Current status summary is in the sibling `STATUS.md`.

## Assumptions

- Pi is the implementation reference, especially:
  - `/tmp/pi-mono/packages/ai/src/utils/oauth/openai-codex.ts`
  - `/tmp/pi-mono/packages/ai/src/providers/openai-codex-responses.ts`
  - `/tmp/pi-mono/packages/coding-agent/src/core/auth-storage.ts`
  - `/tmp/pi-mono/packages/coding-agent/src/core/model-registry.ts`
- initial local scope is ChatGPT subscription auth for `openai-codex`, not standard `api.openai.com` API-key auth
- initial transport target is SSE only; websocket support can wait
- Gemini remains supported and should stay the default until explicit provider selection is added
- auth state should live under `.fritz/` and stay outside session/history files
- `internal/model.Gateway` is already the runtime seam and should stay stable in v1

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
- never write OAuth access or refresh tokens into session files, logs, prompts, snapshots, or tool output
- auth persistence must use file locking for refresh/update paths
- provider wiring should be minimal and additive; do not build a giant registry if a small provider switch suffices
- keep Pi comparison source-derived from code refs in tickets/docs, not memory
- keep functional core / imperative shell:
  - `internal/app`: select provider, wire commands, doctor, bootstrap
  - `internal/authstore`: persist creds, lock update paths, refresh orchestration
  - `internal/openaicodex`: OAuth + SSE request/response mapping
  - gateways/transports should receive request-ready auth, not refresh tokens

## Tasks

- `OCS-001.md`
- `OCS-002.md`
- `OCS-003.md`
- `OCS-004.md`
- `OCS-005.md`
- `OCS-006.md`
- `OCS-007.md`
