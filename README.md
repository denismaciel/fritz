# fritz

Small coding agent harness in Go.

## install

```sh
nix run github:denismaciel/fritz -- --help
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
```

Config comes from `.fritz/config.json`, env, and flags.

Infra wiring like NixOS services, Telegram secrets, and host config stays outside this repo.
