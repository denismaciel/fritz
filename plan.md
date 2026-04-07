# Plan

Goal: build a minimal Pi-inspired coding agent in Go, incrementally, always moving from one working state to the next.

## Principle

Each step must end in a usable state. No big-bang build. No speculative architecture first.

We use a test-first approach for all feature work:

- write a failing test first
- make it pass with the smallest change
- refactor after green

Rule: red, green, refactor. Always.

## Milestones

### 1. `hello` CLI

- parse args
- load `GEMINI_API_KEY`
- add `doctor`
- print plain output

Working state:

- binary runs
- env check works

### 2. Single prompt -> single response

- Gemini client
- send one prompt
- print one response
- no tools
- no streaming
- no session

Working state:

- `fritz run "hi"` works

### 3. Chat loop

- REPL mode
- keep in-memory message history
- add `:quit`
- add `:reset`
- add `:help`

Working state:

- multi-turn chat works in one process

### 4. Streaming

- stream text deltas
- support cancel
- assemble final assistant message cleanly

Working state:

- interactive chat feels live

### 5. Tool-call protocol

- define Go tool interface
- map tool schemas to Gemini
- define tool result message format
- start with `read`

Working state:

- model can inspect files

### 6. Minimal coding toolset

- add `read`
- add `write`
- add `edit`
- add `bash`
- keep implementations small and strict

Working state:

- real coding tasks possible

### 7. Agent loop

- send prompt
- detect tool calls
- execute tool calls
- append tool results
- continue until no more tool calls

Working state:

- basic autonomous coding agent works

### 8. Safety rails

- cwd sandbox
- path normalization
- file size limits
- command timeout
- safe defaults

Working state:

- usable without obvious footguns

### 9. Sessions

- persist transcript to JSONL
- resume latest
- explicit new session
- no branching yet

Working state:

- conversations survive process restart

### 10. Structured configuration

- add config file support
- define typed config struct
- merge defaults, config file, env, flags
- start small: model, cwd policy, timeouts, session settings
- keep format simple, likely JSON or YAML

Working state:

- agent behavior is configured in one coherent place
- env vars stop being the only control surface

### 11. Prompt layer

- base system prompt
- load repo `AGENTS.md`
- support local/global `SYSTEM.md` and `APPEND_SYSTEM.md`
- support skills from day one

Working state:

- agent follows project-specific rules
- skills are visible in prompt and invokable via `/skill:name`

### 12. Ergonomics

- `@file` insertion
- improve edit diffs
- compact help
- maybe usage/cost output

Working state:

- nicer daily-driver CLI

### 13. Compaction

- summarize old context near limit
- keep recent turns verbatim

Working state:

- long sessions remain usable

### 14. Polish

- tests for loop and tools
- retries
- better errors
- packaging/install

Working state:

- solid `v0`

## Suggested releases

- `v0`: steps 1-3
- `v0.1`: step 4
- `v0.2`: steps 5-7
- `v0.3`: steps 8-11
- `v0.4`: steps 12-14

## Rule

Prefer the smallest thing that works. Skip provider abstraction, extension systems, and UI complexity until the core loop is solid.

All implementation work should follow test first development: red, green, refactor.

## Next track: Nix package + host service

Goal: package the agent once, expose a reusable NixOS module, and deploy it as an isolated Telegram polling service on `ben` using `sops-nix` for bootstrap secrets.

### N1. Package export

- add flake package/app outputs
- keep package reusable from GitHub inputs

Working state:

- `nix build .#fritz`

### N2. Reusable service module

- dedicated service user
- dedicated home/workspace
- polling service via systemd

Working state:

- downstream NixOS configs can import one module and enable the service

### N3. Workspace bootstrap

- seed `AGENTS.md`
- seed `MEMORY.md`
- seed `HEARTBEAT.md`
- create `memory/`

Working state:

- empty workspace boots cleanly

### N4. ben deploy

- enable module on `ben`
- wire `sops` env template
- keep heartbeat off first

Working state:

- bot can be rebuilt and started on `ben`

### N5. Smoke runbook

- operator docs
- deploy/test commands
- inspection paths

Working state:

- bring-up steps are documented end to end

## Next track: Gateway wrapper

Goal: wrap the coding agent in a thin control plane, similar in spirit to OpenClaw, while keeping the agent engine independent.

### Design rule

- engine knows nothing about Telegram, heartbeat, pairing, or scheduling
- gateway owns routing, persistence, wakeups, and channel adapters
- channel adapters normalize transport-specific events into gateway requests
- each step below must end in a working, runnable state

## Gateway milestones

### G1. Engine adapter seam

- define a small gateway-facing engine interface around the current agent runtime
- map inbound request -> agent run
- map agent events -> gateway event stream
- keep in-process only

Working state:

- gateway can call the coding agent without importing terminal UI or CLI code paths

### G2. Gateway skeleton

- add a gateway package and process entrypoint
- define state dir layout
- define `handleInbound()`
- define normalized inbound/outbound/event types
- add config for gateway basics

Working state:

- gateway process starts
- synthetic inbound messages can be routed into the engine

### G3. Session routing

- add `sessionKey` routing
- Telegram DM -> `telegram:dm:<userId>`
- Telegram group -> `telegram:group:<chatId>`
- bind routed sessions to current transcript/session machinery

Working state:

- repeated inbound events from same source land in same conversation
- different users/chats are isolated

### G4. Telegram adapter v0

- polling only
- receive bot updates
- normalize inbound text/media metadata
- send plain outbound replies
- no webhook yet

Working state:

- Telegram DM can talk to the agent through the gateway

### G5. Telegram access control

- add allowlist / pairing-lite
- persist allow decisions in gateway state dir
- keep group policy simple at first

Working state:

- unauthorized senders are blocked
- authorized senders can use the bot safely

### G6. Durable state v0

- formalize state dir paths
- persist:
  - sessions/transcripts
  - Telegram auth/runtime state
  - allowlists/pairing
  - current conversation bindings if needed
- keep file formats simple and inspectable

Working state:

- gateway restart does not lose channel or routing state

### G7. Durable memory v0

- support `MEMORY.md`
- support `memory/*.md`
- expose them to the engine prompt path
- no vector index yet

Working state:

- durable facts/instructions survive outside chat sessions

### G8. Heartbeat v0

- simple interval scheduler
- simple wake queue
- one heartbeat prompt contract
- gateway decides when to invoke agent
- deliver actionable output back to Telegram

Working state:

- agent can wake periodically and act without user typing first
- no-op heartbeat output is suppressed via explicit sentinel

### G9. Scheduled tasks / reminders v0

- add minimal scheduled work source
- feed due work into heartbeat wake queue
- start with one-shot reminders before full cron

Working state:

- gateway can wake agent because there is queued work to do

### G10. Operator surface

- add minimal admin CLI / commands
- inspect sessions
- inspect allowlists
- trigger heartbeat manually
- basic status output

Working state:

- system can be operated and debugged without reading raw files

### G11. Hardening

- retries and restart behavior for Telegram polling
- better gateway errors
- state migration/versioning where needed
- focused e2e tests

Working state:

- wrapper is stable enough for daily use

## Recommended order

Build in this exact order:

1. `G1` engine adapter seam
2. `G2` gateway skeleton
3. `G3` session routing
4. `G4` Telegram adapter v0
5. `G5` Telegram access control
6. `G6` durable state v0
7. `G7` durable memory v0
8. `G8` heartbeat v0
9. `G9` scheduled tasks / reminders v0
10. `G10` operator surface
11. `G11` hardening

## Why this order

- `G1-G3` create the correct boundaries first
- `G4-G6` make the wrapper actually usable
- `G7-G9` add long-lived behavior after the core is stable
- `G10-G11` make it operable and robust

## Working release cuts

- `wrapper-v0`: `G1-G4`
- `wrapper-v0.1`: `G5-G6`
- `wrapper-v0.2`: `G7-G8`
- `wrapper-v0.3`: `G9-G11`
