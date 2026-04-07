# pi-tool-parity run loop

Read `AGENTS.md`.

Read these pack files before coding:

- `docs/tasks/pi-tool-parity/INDEX.md`
- `docs/tasks/pi-tool-parity/STATUS.md`
- relevant ticket files under `docs/tasks/pi-tool-parity/`

Treat pack files as source of truth. Do not rely on prior chat memory.

Follow pack constraints:

- keep scope tight to active ticket(s)
- red, green, refactor. always
- preserve exact Pi behavior where ticket checklists say so
- tick checklist items in ticket files as each test lands
- keep shell/io at edges; keep core logic testable
- prefer narrow interfaces, explicit errors, boring code
- avoid incidental refactors unless ticket needs them

Execution rules:

- work highest-leverage ready ticket first
- respect dependencies
- update `STATUS.md` and ticket checklists as progress changes
- leave concise notes on blockers, remaining work, next step
- do not stop for progress update if work remains and no blocker

After code changes:

- run validation commands from active ticket
- update pack files to match reality

Done sentinel: `VIBE_DONE`

If and only if full pack is complete and validation passes, answer only with `VIBE_DONE`.

If blocked, update pack files with blocker and answer with one concise sentence.
