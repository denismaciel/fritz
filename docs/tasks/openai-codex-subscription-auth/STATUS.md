# openai codex subscription auth status

## TODO

## IN_PROGRESS

## BLOCKED

## DONE

- `OCS-001` provider abstraction + config surface
- `OCS-002` auth store + locked credential persistence
- `OCS-003` codex oauth login/logout command flow
- `OCS-004` request-auth resolver + bootstrap seam
- `OCS-005` codex sse gateway client
- `OCS-006` runtime wiring + provider selection
- `OCS-007` refresh hardening + docs + operator checks

## SUPERSEDED

## Notes

- Pi reference auth flow:
  - `/tmp/pi-mono/packages/ai/src/utils/oauth/openai-codex.ts`
- Pi reference request shape:
  - `/tmp/pi-mono/packages/ai/src/providers/openai-codex-responses.ts`
- current local seam to preserve:
  - `internal/model/model.go` `Gateway`
- architecture guardrails:
  - keep `model.Gateway` unchanged
  - keep refresh token handling inside auth layer only
  - app/bootstrap resolves request auth, gateway only sends requests
- pack validation should include:
  - `go test ./...`
- added operator doc:
  - `docs/openai-codex-subscription-auth.md`
- likely new local state path:
  - `.fritz/auth.json`
- first pass out of scope:
  - websocket transport
  - provider marketplace / dynamic model registry
  - standard OpenAI platform API-key auth
  - web UI/browser-only auth UX
