# prompt-layer-skills status

## TODO

## IN_PROGRESS

## BLOCKED

## DONE

- `PLS-001` context and prompt resource discovery
- `PLS-002` skill loader and prompt formatting core
- `PLS-003` system prompt builder and request boundary
- `PLS-004` skill invocation expansion
- `PLS-005` CLI and config seams for skills and prompt files
- `PLS-006` integration, tests, docs

## SUPERSEDED

## Notes

- source of truth for Pi behavior is the referenced Pi files, but implementation may stay narrower where tickets say so
- prompt templates are out of scope for this pack
- validate with `go test ./...` after each ticket or coherent batch
- preserve existing sessions/tool parity behavior while adding prompt-layer features
- implemented shape: AGENTS/CLAUDE discovery, `.fritz/SYSTEM.md`, `.fritz/APPEND_SYSTEM.md`, skills in prompt, `/skill:name`, `--skill`, `--no-skills`
