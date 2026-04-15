# slack native gateway task index

Build a native Slack transport for Fritz so Slack interactions are owned directly by the harness and gateway, replacing Bill Nick as the active Slack runtime while keeping the old Bill implementations around for reference.

Current status summary is in the sibling `STATUS.md`.

## Assumptions

- Slack support lands as a new native Fritz transport, parallel to Telegram, not as an SDK bridge to Bill Nick
- Socket Mode is the first transport because it fits internal deployment and avoids a public ingress dependency
- raw Slack Web API + raw WebSocket handling is preferred over Bolt so Fritz owns transport behavior end to end
- existing engine, session, prompt, ingress, heartbeat, and tool layers are reused rather than redesigned
- Telegram behavior and current `fritz` CLI behavior must keep working throughout the pack
- Bill Nick code stays around outside this repo, but production Slack interactions move to Fritz

## Engineering Constraints

- keep IO at the edges; keep core logic testable
- prefer deep modules with narrow public APIs
- depend on interfaces/protocols, not concrete infrastructure
- validate untrusted input at boundaries
- avoid incidental refactors outside ticket scope
- bias toward stable, boring code over cleverness
- make Fritz the single owner of Slack session routing, prompt construction, and artifact publication
- keep transport-specific logic inside `internal/adapters/slack` and transport-neutral logic inside ingress/engine/session
- do not collapse engine event streams into final text when Slack can consume richer progress
- use explicit run-scoped artifact handling; do not reintroduce conversation-wide artifact sweeps
- keep Slack support shippable in small steps: DM + mention flows before assistant streaming polish

## Tasks

- `SNG-001.md`
- `SNG-002.md`
- `SNG-003.md`
- `SNG-004.md`
- `SNG-005.md`
- `SNG-006.md`
- `SNG-007.md`
- `SNG-008.md`
- `SNG-009.md`
