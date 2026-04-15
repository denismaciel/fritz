# slack native gateway run loop

Read `AGENTS.md`.

Read these pack files before coding:

- `docs/tasks/slack-native-gateway/INDEX.md`
- `docs/tasks/slack-native-gateway/STATUS.md`
- relevant ticket files under `docs/tasks/slack-native-gateway/`

Treat the pack files as source of truth. Do not rely on prior chat memory.

Follow the pack's engineering constraints:

- keep scope tight to the active ticket(s)
- keep IO at the edges; keep core logic testable
- prefer narrow interfaces and deep modules
- use dependency inversion at boundaries
- validate inputs and make failure modes explicit
- avoid incidental refactors unless the pack requires them
- optimize for stable, boring code
- keep Slack-specific code inside `internal/adapters/slack`
- keep ingress/engine/session transport-neutral
- preserve existing CLI and Telegram behavior while adding Slack
- use explicit run-scoped artifact handling; do not add conversation-wide artifact sweeps

Execution rules:

- work the highest-leverage ready ticket first
- respect ticket dependencies and validation steps
- update `STATUS.md` and any relevant ticket files as progress changes
- leave concise notes on blockers, remaining work, and next step
- do not stop for a progress update if work remains and you are not blocked

After code changes:

- run the relevant validation commands from the pack
- update the pack files to reflect what is done vs remaining

Done sentinel: `VIBE_DONE`

If and only if the full pack is complete and validation passes, answer only with `VIBE_DONE`.

If blocked, update the pack files with the blocker and answer with one concise sentence.
