# nix fritz service task index

Package `fritz` as a reusable Nix flake output, expose a reusable NixOS service module, then wire it on host `ben` with `sops-nix` for Telegram/Gemini bootstrap secrets. Goal: one pkg + one module, reused locally and by downstream flakes, with host-specific config kept thin.

Current status summary is in the sibling `STATUS.md`.

## Assumptions

- package source stays this repo
- host deployment target is `ben`
- first deploy mode is Telegram polling, not webhook
- service runs as dedicated system user in an empty workspace, not in source checkout
- bootstrap secrets come from `sops-nix`; dynamic app secrets can still live in `.fritz/secrets.json`

## Engineering Constraints

- keep IO at the edges; keep core logic testable
- prefer deep modules with narrow public APIs
- depend on interfaces/protocols, not concrete infrastructure
- validate untrusted input at boundaries
- avoid incidental refactors outside ticket scope
- bias toward stable, boring code over cleverness
- avoid duplicated Nix logic between package export, module, and `ben`
- keep host-specific policy in host config; keep service mechanics in reusable module
- keep agent-editable docs in workspace root; keep harness state under `.fritz/`
- use `sops.templates.*` env files for bootstrap secrets instead of hardcoding env in unit files

## Tasks

- `NCS-001.md`
- `NCS-002.md`
- `NCS-003.md`
- `NCS-004.md`
- `NCS-005.md`
- `NCS-006.md`
