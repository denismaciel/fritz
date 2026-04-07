# fritz

Small coding agent in Go.

This repo now builds 2 binaries:

- `fritz`: coding agent cli
- `fritz-telegram`: Telegram gateway for `fritz`

## install

```sh
nix run github:denismaciel/fritz -- --help
nix run github:denismaciel/fritz#fritz-telegram -- --help
```

or:

```sh
make install
```

## dev

```sh
nix develop
make test
make build
```

## use

```sh
fritz --help
fritz doctor
fritz run "hi"
fritz chat
fritz-telegram --help
```

Config comes from `.fritz/config.json`, env, and flags.

Infra wiring like NixOS services, Telegram secrets, and host config stays outside this repo.
