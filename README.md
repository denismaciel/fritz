# fritz

A small coding agent written in Go.

This repo builds two binaries:

- `fritz` — CLI agent
- `fritz-telegram` — Telegram gateway for `fritz`

## Quick start

```sh
nix run github:denismaciel/fritz -- --help
nix run github:denismaciel/fritz#fritz-telegram -- --help
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
```

## Config

Configuration is loaded from, in order:

- `.fritz/config.json`
- environment variables
- command-line flags

Infra outside this repo: NixOS services, Telegram secrets, and host-specific config.
