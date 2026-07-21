# fritz

A small coding agent written in Go.

This repo builds two binaries:

- `fritz` — CLI agent
- `fritz-telegram` — Telegram gateway for `fritz`
- `fritz-slack` — Slack Socket Mode gateway for `fritz`

## Quick start

```sh
nix run github:denismaciel/fritz -- --help
nix run github:denismaciel/fritz#fritz-telegram -- --help
nix run github:denismaciel/fritz#fritz-slack -- --help
```

Or install locally:

```sh
make install
```

## Development

```sh
nix develop
make test
make build
```

## Usage

```sh
fritz --help
fritz doctor
fritz run "hi"
fritz chat
fritz-telegram --help
fritz-slack --help
```

## Config

Configuration is loaded from, in order:

- `.fritz/config.json`
- environment variables
- command-line flags

Infra outside this repo: NixOS services, Telegram secrets, and host-specific config.

## Slack gateway

`fritz-slack` is the native Slack transport. It uses Slack Socket Mode and the Slack Web API directly instead of a separate Slack-side wrapper.

Required env/config:

- `SLACK_BOT_TOKEN`
- `SLACK_APP_TOKEN`
- optional `FRITZ_SLACK_ENDPOINT`
- optional `FRITZ_SLACK_ALLOWED_USERS`
- optional `FRITZ_SLACK_ALLOWED_CHANNELS`
- optional `FRITZ_SLACK_ASSISTANT_ENABLED`

Typical local run:

```sh
fritz-slack --help
fritz-slack
```

Current behavior:

- native DM + thread routing through Fritz ingress/session state
- assistant thread context persistence and assistant API calls
- streamed Slack replies via `chat.startStream` / `chat.appendStream` / `chat.stopStream`
- run-scoped file upload via Slack external upload APIs
- native `/clear` session reset

## Telegram training plan command

Set `FRITZ_TRAINING_DB` (or `telegram.trainingDbPath` in config) to a Fritz
training SQLite database. Authorized users then get native, model-free commands:

- `/training` or `/training today` — today's workout, including structured steps
- `/training week` or `/training_week` — the current Monday-Sunday plan

The gateway registers these commands in the bot's Telegram command menu at startup.
