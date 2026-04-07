# Coding Agent

Goal: build a custom coding agent inspired by Pi.

## Intent

I like Pi's minimalism and relative lack of features. I want a similar agent in that spirit:

- small
- simple
- minimal UI/UX
- few moving parts
- no broad provider abstraction
- no unnecessary features

## Scope

This agent should be less featured than Pi.

For now it only needs to work with `GEMINI_API_KEY`.

It does not need:

- multiple LLM providers
- heavy framework abstractions
- feature breadth for its own sake

## Implementation

I want to write it in Go.

## Reference

Use Pi as design inspiration, especially its minimal feel, but keep the implementation narrower and simpler.

## Scaffold

Initial project scaffold:

- `cmd/fritz` CLI entrypoint
- `internal/app` command dispatch
- `internal/config` env loading
- `internal/gemini` Gemini-only client stub

Current commands:

- `go run ./cmd/fritz --help`
- `go run ./cmd/fritz doctor`
- `go run ./cmd/fritz run "hi"`
- `go run ./cmd/fritz chat`
- `go run ./cmd/fritz serve`
- `go run ./cmd/fritz run "/skill:task-pack-create make a pack"`

Install locally:

- `make build`
- `make install`
- binary path: `~/.local/bin/fritz`

Nix:

- `nix build .#fritz`
- `nix run .#fritz -- --help`
- `nix develop`

Deployment-specific NixOS/Home Manager modules and secret wiring live outside this repo.

## Config

Config merges in this order:

- defaults
- `.fritz/config.json`
- env
- flags

Example:

```json
{
  "model": "gemini-3-flash-preview",
  "geminiEndpoint": "https://generativelanguage.googleapis.com",
  "telegram": {
    "endpoint": "https://api.telegram.org",
    "pollTimeout": "20s",
    "pairingToken": "<pairing-token>",
    "allowedUsers": ["123456789"]
  },
  "log": {
    "file": ".fritz/logs/agent.jsonl",
    "level": "info"
  },
  "chat": {
    "showHelpOnStart": true
  },
  "session": {
    "dir": ".fritz/sessions",
    "enabled": true,
    "autoCompact": true,
    "compactThresholdTurns": 20,
    "compactKeepTurns": 8
  },
  "prompt": {
    "noSkills": false,
    "skillPaths": ["./skills"]
  },
  "runtime": {
    "commandTimeout": "30s"
  }
}
```

Useful env vars:

- `GEMINI_API_KEY`
- `FRITZ_MODEL`
- `FRITZ_GEMINI_ENDPOINT`
- `TELEGRAM_BOT_TOKEN`
- `FRITZ_TELEGRAM_ENDPOINT`
- `FRITZ_TELEGRAM_POLL_TIMEOUT`
- `FRITZ_TELEGRAM_PAIRING_TOKEN`
- `FRITZ_TELEGRAM_ALLOWED_USERS`
- `FRITZ_LOG_FILE`
- `FRITZ_LOG_LEVEL`
- `FRITZ_CHAT_HELP`
- `FRITZ_SESSION_ENABLED`
- `FRITZ_SESSION_DIR`
- `FRITZ_AUTO_COMPACT`
- `FRITZ_COMPACT_THRESHOLD`
- `FRITZ_COMPACT_KEEP`
- `FRITZ_COMMAND_TIMEOUT`
- `FRITZ_NO_SKILLS`
- `FRITZ_SKILLS`

Useful flags:

- `--config <path>`
- `--model <id>`
- `--gemini-endpoint <url>`
- `--telegram-bot-token <token>`
- `--telegram-endpoint <url>`
- `--telegram-poll-timeout <duration>`
- `--telegram-pairing-token <token>`
- `--telegram-allow-user <id>`
- `--heartbeat=<bool>`
- `--heartbeat-interval <duration>`
- `--log-file <path>`
- `--log-level <level>`
- `--server <url>`
- `--listen <addr>`
- `--session-dir <path>`
- `--chat-help=<bool>`
- `--auto-compact=<bool>`
- `--compact-threshold <n>`
- `--compact-keep <n>`
- `--command-timeout <duration>`
- `--skill <path>`
- `--no-skills`

`doctor` shows:

- provider
- endpoint
- log file
- log level
- model
- session dir
- skills enabled/disabled state

Logs:

- default file: `.fritz/logs/agent.jsonl`
- JSONL, one event per line
- examples:
  - `tail -f .fritz/logs/agent.jsonl | jq`
  - `rg '"event":"heartbeat' .fritz/logs/agent.jsonl`
  - `jq 'select(.level=="error")' .fritz/logs/agent.jsonl`

## Prompt Layer

Project instructions load from:

- `AGENTS.md` or `CLAUDE.md` in cwd and ancestor dirs
- `~/.fritz/AGENTS.md`

System prompt files:

- `.fritz/SYSTEM.md`
- `.fritz/APPEND_SYSTEM.md`
- `~/.fritz/SYSTEM.md`
- `~/.fritz/APPEND_SYSTEM.md`

Skills load from:

- `.fritz/skills`
- `.agents/skills` in cwd and ancestor dirs
- `~/.fritz/skills`
- `~/.agents/skills`
- explicit `--skill <path>` entries

Explicit skill invocation:

- `/skill:name`
- `/skill:name extra args`

Durable memory loads from:

- `MEMORY.md`
- `memory/*.md`

These files are prompt inputs, not gateway ops state.

Telegram service workspace:

- workspace root:
  - `AGENTS.md`
  - `MEMORY.md`
  - `HEARTBEAT.md`
  - `memory/`
- harness state:
  - `.fritz/`

## Server Mode

Canonical API protocol is `AG-UI` over SSE.

Start server:

- `fritz serve`
- `fritz --listen :8080 serve`

Use existing CLI as thin client:

- `fritz --server http://127.0.0.1:8080 run "hi"`
- `fritz --server http://127.0.0.1:8080 chat`

HTTP endpoints:

- `POST /runs`
- `POST /runs/{id}/cancel`
- `POST /ai-sdk/chat`

`/runs` request:

```json
{
  "prompt": "summarize README.md",
  "session": {
    "continue": false,
    "sessionPath": "",
    "forkPath": "",
    "noSession": false,
    "newSession": false
  }
}
```

## NixOS Service

Reusable module:

- `/home/denis/dotfiles/modules/nixos/services/fritz-telegram.nix`

Host `ben` enables:

- `services.fritzTelegram.enable = true`
- env file from `sops.templates.fritz_env.path`

Default service layout:

- user: `fritz`
- home: `/var/lib/fritz`
- workspace: `/var/lib/fritz/work`

Bootstrap secrets via `sops-nix`:

- `GEMINI_API_KEY`
- `TELEGRAM_BOT_TOKEN`
- `FRITZ_TELEGRAM_PAIRING_TOKEN`

Runbook:

- [ben-telegram-service.md](/home/denis/dotfiles/fritz/docs/runbooks/ben-telegram-service.md)

AG-UI stream emits:

- `RUN_STARTED`
- `STEP_STARTED`
- `TEXT_MESSAGE_START`
- `TEXT_MESSAGE_CONTENT`
- `TEXT_MESSAGE_END`
- `TOOL_CALL_START`
- `TOOL_CALL_ARGS`
- `TOOL_CALL_END`
- `TOOL_CALL_RESULT`
- `STEP_FINISHED`
- `RUN_FINISHED`
- `RUN_ERROR`

Example:

```bash
curl -N http://127.0.0.1:8080/runs \
  -H 'content-type: application/json' \
  -d '{"prompt":"hi"}'
```

## Telegram

Polling-only v0:

- `fritz telegram`
- `fritz telegram --poll-once`

Typical setup:

```bash
export GEMINI_API_KEY=...
export TELEGRAM_BOT_TOKEN=...
fritz --telegram-pairing-token <pairing-token> telegram
```

Or pre-allow one user:

```bash
fritz --telegram-allow-user 123456789 telegram
```

Current policy:

- unauthorized DM => `not authorized`
- unauthorized group => ignored
- `/start <token>` or `/pair <token>` in DM pairs and persists allow

## Heartbeat

Heartbeat v0 is gateway-owned.

- scheduler lives in gateway/runtime code
- engine stays unaware
- Telegram adapter is delivery only

Flags/env:

- `--heartbeat=<bool>`
- `--heartbeat-interval <duration>`
- `FRITZ_HEARTBEAT_ENABLED`
- `FRITZ_HEARTBEAT_INTERVAL`

No-op contract:

- heartbeat prompt must reply exactly `HEARTBEAT_OK` when there is nothing to do
- no-op replies are suppressed

## State Layout

Transcript/session state:

- `.fritz/sessions/...`

Gateway ops state:

- `.fritz/gateway/meta.json`
- `.fritz/gateway/routing/session-map.json`
- `.fritz/gateway/telegram/offset.json`
- `.fritz/gateway/telegram/allowlist.json`
- `.fritz/gateway/telegram/pairing.json`
- `.fritz/gateway/bindings/current.json`
- `.fritz/gateway/heartbeat/state.json`

Durable memory:

- `MEMORY.md`
- `memory/*.md`

Keep these concerns separate:

- transcripts = conversation history
- gateway state = routing/runtime/authorization
- durable memory = long-lived facts/instructions

Secrets:

- `.fritz/secrets.json`
- use secret tools, not memory files
- secret values are not injected into prompt context

## Browser / AI SDK

If you want low-plumbing React/browser clients, use the AI SDK adapter endpoint:

- `POST /ai-sdk/chat`

Response headers:

- `content-type: text/event-stream`
- `x-vercel-ai-ui-message-stream: v1`

Minimal request shape:

```json
{
  "messages": [
    {
      "role": "user",
      "parts": [
        { "type": "text", "text": "hi" }
      ]
    }
  ]
}
```

Implemented AI SDK parts:

- `start`
- `start-step`
- `text-start`
- `text-delta`
- `text-end`
- `tool-input-available`
- `tool-output-available`
- `finish-step`
- `finish`
- `error`
- `abort`
- `[DONE]`

## Local Terminal UI

Local `fritz chat` now uses an embedded Go terminal UI when stdin/stdout are real ttys.

Stack:

- `bubbletea`
- `bubbles`
- `lipgloss`

Current local controls:

- `Enter` send prompt
- `Ctrl+S` send prompt
- `Esc` cancel active run
- `Esc` clear draft when idle
- `Ctrl+L` reset local transcript
- `Ctrl+C` quit
- `PgUp` / `PgDn` scroll transcript

What it shows:

- streamed assistant text
- reasoning blocks
- tool calls with compact previews
- tool errors

Notes:

- non-tty `chat` still falls back to the old line-based loop
- `serve` is optional; local terminal use does not require a background server
