# imperative-shell-functional-core run loop

Read `AGENTS.md`.

Read these pack files before coding:

- `docs/tasks/imperative-shell-functional-core/INDEX.md`
- `docs/tasks/imperative-shell-functional-core/STATUS.md`
- relevant ticket files under `docs/tasks/imperative-shell-functional-core/`

Treat the pack files as source of truth. Do not rely on prior chat memory.

Follow the pack's engineering constraints:

- keep scope tight to the active ticket(s)
- keep IO at the edges; keep core logic testable
- prefer narrow interfaces and deep modules
- use dependency inversion at boundaries
- validate inputs and make failure modes explicit
- avoid incidental refactors unless the pack requires them
- optimize for stable, boring code
- use red, green, refactor on every implementation step

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
