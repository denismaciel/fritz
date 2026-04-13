# openai codex subscription auth

`fritz` now supports ChatGPT subscription-backed `openai-codex` auth and SSE transport.

## commands

- `fritz auth login openai-codex`
- `fritz auth logout openai-codex`
- `fritz auth status [openai-codex|gemini]`
- `fritz --provider openai-codex doctor`
- `fritz --provider openai-codex run "<prompt>"`
- `fritz --provider openai-codex chat`

## local state

- auth file: `$XDG_DATA_HOME/fritz/auth.json`
- fallback auth file: `~/.local/share/fritz/auth.json`
- lock file: same path with `.lock` suffix

OAuth tokens are kept out of session/history files.

## config

Supported config/env knobs:

- `provider`
- `openAICodexEndpoint`
- `openAICodexAuthBaseURL`
- `openAICodexClientID`
- `openAICodexOriginator`
- `openAICodexRedirectURL`

Env forms:

- `FRITZ_PROVIDER`
- `FRITZ_OPENAI_CODEX_ENDPOINT`
- `FRITZ_OPENAI_CODEX_AUTH_BASE_URL`
- `FRITZ_OPENAI_CODEX_CLIENT_ID`
- `FRITZ_OPENAI_CODEX_ORIGINATOR`
- `FRITZ_OPENAI_CODEX_REDIRECT_URL`

## flow

1. run `fritz auth login openai-codex`
2. browser opens to OpenAI auth, or copy the printed URL manually
3. local callback captures `code`, or paste redirected URL/code back into terminal
4. agent stores `access`, `refresh`, `expires`, `accountId`
5. normal `run/chat/serve/telegram` bootstrap resolves request auth before first model call
6. expired OAuth tokens refresh automatically behind the auth resolver

## notes

- transport target: `https://chatgpt.com/backend-api/codex/responses`
- auth target: `https://auth.openai.com/oauth/*`
- request headers include:
  - `Authorization: Bearer <token>`
  - `chatgpt-account-id: <accountId>`
  - `originator: fritz`
  - `OpenAI-Beta: responses=experimental`

## caveats

- this depends on ChatGPT backend behavior, not stable `api.openai.com` API-key contracts
- first pass is SSE only
- no websocket transport
- no generic provider registry
- gemini web-search tool remains gemini-backed
