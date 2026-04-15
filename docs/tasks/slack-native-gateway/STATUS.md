# slack native gateway status

## TODO

## IN_PROGRESS

## BLOCKED

## DONE

- `SNG-001` add Slack config, command wiring, and nix package skeleton
- `SNG-002` add Slack Web API + Socket Mode transport clients
- `SNG-003` normalize inbound Slack events and persist routing state
- `SNG-004` ship DM and thread mention runs on native Slack transport
- `SNG-005` add native `/clear` and explicit session reset semantics
- `SNG-006` add assistant thread flows and context-aware Slack APIs
- `SNG-007` expose streamed engine events to Slack responses
- `SNG-008` add run-scoped artifact upload and Slack heartbeat sender
- `SNG-009` harden, document, and validate native Slack deployment

## SUPERSEDED

## Notes

- keep Bill Nick out of the execution path; this pack is about native Fritz ownership
- preserve `fritz` and `fritz-telegram` buildability on every step
- prefer fake Slack websocket/http tests over brittle live-service assumptions
- use Socket Mode first, but keep room for later HTTP ingress if Marketplace/public deployment becomes important
- live Slack workspace validation still depends on real app tokens and a real workspace
