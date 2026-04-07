# tool-runtime-upgrades run loop

Read `AGENTS.md`.

Read these pack files before coding:

- `docs/tasks/tool-runtime-upgrades/INDEX.md`
- `docs/tasks/tool-runtime-upgrades/STATUS.md`
- relevant ticket files under `docs/tasks/tool-runtime-upgrades/`

Treat pack files as source of truth. Do not rely on prior chat memory.

Follow pack constraints:

- red, green, refactor. always
- preserve behavior while changing boundaries
- prefer typed payloads over generic maps
- keep provider-specific logic out of core types
- keep shell/io at edges
- keep scope tight to active ticket(s)
- avoid incidental refactors unless ticket needs them

Execution rules:

- work highest-leverage ready ticket first
- respect dependencies
- update `STATUS.md` as progress changes
- leave concise notes on blockers, remaining work, next step
- do not stop for progress update if work remains and no blocker

After code changes:

- run the validation commands from active ticket
- update pack files to match reality

Done sentinel: `VIBE_DONE`

If and only if full pack is complete and validation passes, answer only with `VIBE_DONE`.

If blocked, update pack files with blocker and answer with one concise sentence.
