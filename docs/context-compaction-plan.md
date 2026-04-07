# Codex Context Compaction Notes

Date: 2026-04-05

Upstream repo cloned to `/tmp/openai-codex`.

## Sources

- `openai/codex` repo: <https://github.com/openai/codex>
- Inline compaction: <https://github.com/openai/codex/blob/main/codex-rs/core/src/compact.rs>
- Remote compaction: <https://github.com/openai/codex/blob/main/codex-rs/core/src/compact_remote.rs>
- Trigger points: <https://github.com/openai/codex/blob/main/codex-rs/core/src/codex.rs>
- Resume/rollback reconstruction: <https://github.com/openai/codex/blob/main/codex-rs/core/src/codex/rollout_reconstruction.rs>
- Compaction prompt: <https://github.com/openai/codex/blob/main/codex-rs/core/templates/compact/prompt.md>

## What Codex does

OpenAI Codex does not treat compaction as "drop old turns, keep N latest". It treats compaction as a first-class history rewrite.

Key parts:

1. Trigger by token pressure, not turn count.
2. Run compaction before sampling when context already too large.
3. Run compaction mid-turn when a follow-up tool/model step would exceed limit.
4. Persist a compaction artifact with replacement history, not just a text summary.
5. Rebuild exact post-compaction history on resume/fork/rollback.
6. Reinject canonical initial context at precise boundaries after compaction.
7. Trim low-value old items first during compaction attempts if the compaction prompt itself is too large.
8. Keep ghost/tool state needed for undo/recovery.

## Our current design

Current local behavior is much simpler:

- Turn-count trigger only: [`planning.go`](/home/denis/github.com/denismaciel/fritz/internal/session/planning.go)
- Summary entry persisted, but no replacement-history checkpoint: [`compaction.go`](/home/denis/github.com/denismaciel/fritz/internal/session/compaction.go)
- Context rebuild = single `"Compaction summary"` user msg + kept raw messages: [`manager.go`](/home/denis/github.com/denismaciel/fritz/internal/session/manager.go)
- Retry only after provider returns `"context overflow"`: [`service.go`](/home/denis/github.com/denismaciel/fritz/internal/agent/service.go)

Net: ours is easy, but lossy and reactive.

## Biggest gaps

### 1. Bad trigger metric

We compact by completed turns. Real context cost comes from:

- tool outputs
- long user prompts
- reasoning text
- system prompt growth

A short 30-turn thread may fit. A 2-turn thread with huge tool output may not.

### 2. No pre-turn compaction

We wait until overflow or end-of-turn auto-compact. Codex compacts before sampling, which avoids one failed request and keeps UX smoother.

### 3. No mid-turn compaction path

If tool calls create a long follow-up prompt, our next model call may fail. Codex can compact inside the same turn and continue.

### 4. Summary-only checkpoint

Our persisted compaction stores:

- summary text
- first kept entry id

It does not store the exact compacted replacement history. Resume/fork/nav cannot reconstruct the exact model-visible context after multiple compactions.

### 5. No canonical context reinjection model

Codex distinguishes:

- compacted history
- canonical initial context
- reference context baseline

We currently just prepend a summary msg and continue. That is enough for plain chat, weak for richer stateful agent behavior.

### 6. No low-value item trimming strategy

Codex trims oldest items and paired tool artifacts when even the compaction request itself would overflow. We do not.

## What to copy vs not copy

Copy:

- token-based trigger
- pre-turn compaction
- explicit compaction checkpoint item
- replacement-history persistence
- resume reconstruction from checkpoints
- mid-turn compaction after tool-heavy steps

Do not copy yet:

- remote `/responses/compact` path
- ghost snapshot complexity
- realtime/reference-context machinery

Those solve Codex-specific protocol/state problems we do not have yet.

## Proposed design here

### Phase 1: token-aware pre-turn compaction

Goal: avoid overflow before `Generate`.

Changes:

- add rough token estimator for `model.Request`
- add `SessionConfig.CompactThresholdTokens`
- compact before first model call when estimated prompt exceeds threshold
- keep current summary format for now

Touch:

- [`model.go`](/home/denis/github.com/denismaciel/fritz/internal/model/model.go)
- [`config.go`](/home/denis/github.com/denismaciel/fritz/internal/config/config.go)
- [`service.go`](/home/denis/github.com/denismaciel/fritz/internal/agent/service.go)
- [`compaction.go`](/home/denis/github.com/denismaciel/fritz/internal/session/compaction.go)

Why first: biggest win, least structural churn.

### Phase 2: replacement-history checkpoints

Goal: make compaction resumable and exact.

Changes:

- extend compaction line with `ReplacementMessages []model.Message`
- maybe also persist compacted transcript form for UI/debug
- `BuildContext()` should prefer newest replacement-history checkpoint as base
- replay only newer entries after that checkpoint

Touch:

- [`manager.go`](/home/denis/github.com/denismaciel/fritz/internal/session/manager.go)
- [`compaction.go`](/home/denis/github.com/denismaciel/fritz/internal/session/compaction.go)
- session tests

This is the main Codex idea worth adopting.

### Phase 3: mid-turn compaction

Goal: survive tool-heavy turns.

Changes:

- after tool results appended, estimate next prompt size
- if above threshold, compact before follow-up model call
- preserve current pending user goal in summary instructions

Touch:

- [`service.go`](/home/denis/github.com/denismaciel/fritz/internal/agent/service.go)
- [`chat/core.go`](/home/denis/github.com/denismaciel/fritz/internal/chat/core.go)
- session tests around tool loops

### Phase 4: compaction-quality improvements

Changes:

- dedicated compaction prompt template file
- summary schema: goal / done / constraints / next / tool findings
- summarize dropped tool outputs into structured bullets
- keep last few user prompts in raw form plus summary

This should improve post-compact answer quality more than tweaking thresholds.

## Concrete implementation sketch

### Step A

Add config:

- `compactThresholdTokens`
- optional `compactTargetTokens`
- keep existing turn-based knobs as fallback

Rule:

- compact if estimated tokens >= threshold
- after compaction, target "summary + last K turns" under `target`

### Step B

Add request estimator:

- `EstimateTokensForMessages([]model.Message) int`
- include system prompt and tool schemas
- rough byte/4 heuristic is enough first pass

### Step C

Change compaction output type from:

- `summary + firstKeptEntryID`

to:

- `summary`
- `replacementMessages`
- `replacementTranscript`
- metadata about dropped/kept spans

### Step D

Rebuild context from newest checkpoint:

1. scan branch for latest compaction with replacement history
2. seed context with that replacement history
3. replay later entries

This matches Codex's replay/checkpoint model in a simpler form.

### Step E

Mid-turn guard:

- before each `Generate`, estimate request size
- compact if needed
- on overflow, compact once more and retry

This makes overflow retry a fallback, not primary path.

## Risks

- rough estimator may compact too early
- if replacement history serialization is wrong, resume bugs get subtle
- summary prompt quality matters more once old raw context disappears
- branch navigation logic may need updates once checkpoints become richer

## Recommended order

1. Phase 1
2. Phase 2
3. Phase 3
4. Phase 4

## Minimal MVP

If we want only one small high-value change:

1. add token estimator
2. pre-turn compact before `Generate`
3. keep existing summary entry format

If we want the real Codex-style win:

1. add replacement-history checkpoints
2. rebuild from latest checkpoint on resume
3. add pre-turn compaction
