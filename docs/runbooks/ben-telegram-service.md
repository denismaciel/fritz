# ben telegram service runbook

## Build + eval

From `/home/denis/dotfiles`:

```bash
nix build .#fritz
nix eval .#nixosConfigurations.ben.config.services.fritzTelegram.enable
nix eval .#nixosConfigurations.ben.config.sops.templates.fritz_env.path
nix build .#nixosConfigurations.ben.config.system.build.toplevel --dry-run
```

## Deploy

```bash
cd /home/denis/dotfiles
nix run .#rebuild-ben
```

## Service shape

- service: `fritz-telegram.service`
- setup: `fritz-setup.service`
- user: `fritz`
- home: `/var/lib/fritz`
- workspace: `/var/lib/fritz/work`

## Workspace layout

Agent-editable:

- `/var/lib/fritz/work/AGENTS.md`
- `/var/lib/fritz/work/MEMORY.md`
- `/var/lib/fritz/work/HEARTBEAT.md`
- `/var/lib/fritz/work/memory/`

Harness/runtime state:

- `/var/lib/fritz/work/.fritz/`
- `/var/lib/fritz/work/.fritz/gateway/`
- `/var/lib/fritz/work/.fritz/secrets.json`

## Bootstrap secrets

Rendered env file:

- `/run/secrets/rendered/fritz_env`

Provides:

- `GEMINI_API_KEY`
- `TELEGRAM_BOT_TOKEN`
- `FRITZ_TELEGRAM_PAIRING_TOKEN`

## Smoke test

On host:

```bash
ssh ben "systemctl status fritz-setup fritz-telegram --no-pager"
ssh ben "sudo -u fritz ls -la /var/lib/fritz/work"
ssh ben "sudo sed -n '1,40p' /run/secrets/rendered/fritz_env"
```

In Telegram:

- DM `@borinho_bot`
- send `/start <pairing-token>`
- then send `hi`

Inspect after:

```bash
ssh ben "sudo -u fritz find /var/lib/fritz/work -maxdepth 3 -type f | sort"
ssh ben "sudo -u fritz sed -n '1,120p' /var/lib/fritz/work/MEMORY.md"
ssh ben "sudo -u fritz sed -n '1,120p' /var/lib/fritz/work/.fritz/gateway/telegram/allowlist.json"
```

## Notes

- heartbeat is wired but disabled on `ben` initially
- Telegram is polling-only in this deploy
- rotate the current bot token later
