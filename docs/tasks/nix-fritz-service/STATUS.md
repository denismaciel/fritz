# nix fritz service status

## TODO

## IN_PROGRESS

## BLOCKED

## DONE

- `NCS-001` package derivation + flake exports
- `NCS-002` reusable NixOS service module
- `NCS-003` workspace bootstrap + service hardening
- `NCS-004` `ben` host wiring
- `NCS-005` `sops-nix` secrets + env template
- `NCS-006` docs + smoke-runbook

## SUPERSEDED

## Notes

- validate both reusable outputs and concrete `ben` deployment path
- do not mix app runtime refactors into this pack
- deploy with heartbeat off first; smoke test DM pairing/reply before enabling more
- validated with `nix build .#fritz`
- validated with `nix eval .#nixosConfigurations.ben.config.services.fritzTelegram.enable`
- validated with `nix eval .#nixosConfigurations.ben.config.sops.templates.fritz_env.path`
- validated with `nix build .#nixosConfigurations.ben.config.system.build.toplevel --dry-run`
