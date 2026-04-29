# Diary: per-client ThinkingLevel constants

Alternative design to PR-251 (`add-thought-token-streaming`). The goal is to
move every `ThinkingLevel` constant except `gai.ThinkingLevelNone` out of the
core `gai` package and republish them per client (`clients/openai`,
`clients/google`, `clients/anthropic`), targeting the newest models on each
provider. We keep the universal off-switch (`gai.ThinkingLevelNone`) in core,
plumb thought parts/usage end-to-end, and target the same set of providers as
PR-251 — but with constants that match each API's own vocabulary instead of a
shared lowest-common-denominator enum.

## Step 1: Probe live providers before coding

**Author:** main

### Prompt Context

**Verbatim prompt:** "You're the builder on team `thinking-redesign`. Pick up
task #1 from the shared task list (TaskList → TaskGet 1). Mark it in_progress,
claim ownership, and follow the description carefully."

**Interpretation:** The lead has already drafted a detailed redesign in
`TaskGet 1` and explicitly asked for empirical validation against the live
APIs before committing. Throwaway probes go in `internal/probe/main.go` and
must not ship.

**Inferred intent:** Catch SDK shape mismatches and provider-side errors
before they leak into the public API of this library, where each constant has
to map cleanly onto a value the provider actually accepts on the newest models.

### What I did

Built `internal/probe/main.go` to drive each provider's SDK directly and
recorded what worked and what failed:

- OpenAI: ran `chat.completions` against `gpt-5`, `gpt-5.1`, `gpt-5.2`,
  `gpt-5.2-pro` with each `ReasoningEffort` constant
  (`none/minimal/low/medium/high/xhigh`).
- Google: ran `Models.GenerateContent` against `gemini-3-pro-preview` and
  `gemini-3-flash-preview` with `ThinkingBudget=0` and the four symbolic
  `ThinkingLevel` values.
- Anthropic: ran `Messages.New` against `claude-sonnet-4-6`, `claude-opus-4-6`,
  and `claude-opus-4-7` exercising every combination of "no thinking",
  "Thinking.Adaptive only", "OutputConfig.Effort only", and
  "Adaptive + Effort" across all five effort levels.

### Why

The task description repeatedly emphasised "every mapping should be
empirically validated, not just docs-validated". The alternative design must
not ship constants that 400 against the very models they were designed for,
and the SDK shape for Anthropic adaptive thinking was specifically flagged as
unverified.

### What worked

The probe surfaced concrete answers across all three providers:

- OpenAI: at probe time I tested up to `gpt-5.2`, which accepts
  `none/low/medium/high/xhigh` (not `minimal`). `gpt-5.2-pro` is
  responses-only and 404s on chat-completions. `gpt-5.1` accepts
  `none/low/medium/high` (no `minimal`, no `xhigh`). `gpt-5` accepts
  `minimal/low/medium/high` (no `none`, no `xhigh`). The union is exactly
  the spec's `Minimal/Low/Medium/High/XHigh` plus the universal `none`.
  (See Step 4 below: I missed that `gpt-5.3-chat-latest` and `gpt-5.4*`
  also exist in the pinned SDK; re-probing them was a follow-up. The
  level union held; the test target moved.)
- Google: `gemini-3-flash-preview` accepts `ThinkingBudget=0` and the four
  symbolic levels (`MINIMAL/LOW/MEDIUM/HIGH`); the *batch* `Models.GenerateContent`
  endpoint returns `Thought: true` parts on Low/Medium/High, but the
  *streaming* endpoints (`Chats.SendStream` / `Models.GenerateContentStream`)
  that our client uses do not — Flash surfaces thought summaries only via
  batch. `gemini-3-pro-preview` rejects `ThinkingBudget=0` and `MINIMAL`
  ("This model only works in thinking mode") but takes `LOW/MEDIUM/HIGH` and,
  unlike Flash, *does* reliably emit `Thought: true` parts on the streaming
  path. That streaming-vs-batch split forced the test design in Step 2:
  Flash 3 covers the off-switch; Pro 3 covers thought-part streaming.
- Anthropic: `Effort` lives on `OutputConfigParam`, not on
  `ThinkingConfigAdaptiveParam`. Adaptive enables thinking; Effort sets the
  level. `xhigh` only works on `claude-opus-4-7`; Sonnet 4.6 / Opus 4.6 reject
  it. `low/medium/high/max` work on all three. "Effort alone" returns no
  thinking blocks on Sonnet 4.6/Opus 4.6 — adaptive is required to actually
  trigger thinking.

### What didn't work

Several mapping assumptions in the spec turned out to be slightly off after
probing:

- `gai.ThinkingLevelNone` -> OpenAI `"none"` 400s on `gpt-5`:
  `Unsupported value: 'reasoning_effort' does not support 'none' with this
  model.` The spec already anticipates this ("older models will get a 400 —
  let it surface"), so we keep the mapping.
- `gai.ThinkingLevelNone` -> Google `ThinkingBudget=0` 400s on
  `gemini-3-pro-preview`: `Budget 0 is invalid. This model only works in
  thinking mode.` Same call: works fine on `gemini-3-flash-preview`. Same
  "let it surface" trade-off.
- Anthropic `ThinkingConfigAdaptiveParam` does not expose an `Effort` field
  in `anthropic-sdk-go` v1.37.0; only `Display` and `Type`. The matching
  Effort enum lives on a separate struct: `OutputConfigParam{Effort:
  OutputConfigEffort{Low,Medium,High,Xhigh,Max}}`. The API expects both
  fields together: `Thinking.Adaptive` to enable, `OutputConfig.Effort` to
  set the level. I sent the lead a heads-up before continuing.

### What I learned

Three things I would not have got from docs alone:

1. OpenAI's per-model effort matrix is genuinely jagged — every gpt-5.x
   minor revision adds or drops a value. The right per-client API is the
   union, with 400s as the "this model doesn't support that level" signal.
2. Google Pro 3.x literally cannot run without thinking — the abstraction
   "thinking off" is provider-shaped, not model-shaped, on Gemini.
3. Anthropic's adaptive-thinking + effort split is two coupled fields. If a
   future caller sets Effort without Adaptive on Sonnet 4.6, they will pay
   for thinking with no thinking blocks back. The client always sets both
   together for non-`None` levels.

### What was tricky

The Anthropic SDK shape was the biggest snag. The spec wrote
`ThinkingConfigAdaptiveParam{Effort: <effort>}` but the SDK has no such
field. I had to read `message.go` carefully — including the
`thinking_warn.go` deprecation notice for `thinking.type=enabled` on Opus
4.6 — to figure out that `OutputConfigParam.Effort` is the actual landing
spot. The probe matrix (adaptive-only / effort-only / both) was the only
reliable way to confirm it.

### What warrants review

Confirm that the per-client mapping handles two edge cases gracefully
before merging:

- An Anthropic request that already sets `OutputConfig` for a response schema
  must still receive `OutputConfig.Effort` without losing
  `OutputConfig.Format`. The implementation must merge into the existing
  struct, not overwrite it.
- The probe is not part of the build/test gate. Reviewers should confirm it
  is not referenced from `internal/examples` or any ship path before merge.

### Future work

- After this PR lands, consider whether `gai.ThinkingLevelNone` should be
  documented as "best-effort off" since two providers (OpenAI gpt-5,
  Google gemini-3-pro-preview) can't honour it on their newest models. Out
  of scope here.
- Anthropic multi-turn signature round-trip is still deferred (issue #250)
  per the spec; this PR's diff matches PR-251's behaviour: inbound
  `gai.PartTypeThought` to the Anthropic client returns the same hard error.

## Step 2: Wire up per-client constants and PartTypeThought

**Author:** main

### Prompt Context

**Verbatim prompt:** "Pick up task #1 from the shared task list (TaskList → TaskGet 1)."

**Interpretation:** Probes settled the design questions (see Step 1). Now implement: drop the shared constants from `gai`, publish per-client ones with the empirically validated levels, plumb thought parts and thoughts-tokens, and update tests to actually exercise the new mappings against the targeted models.

**Inferred intent:** Land a public-API redesign that's identical in shape to PR-251 except every concrete level constant lives next to the SDK type it maps onto. Future-proofs the library against the next round of provider-effort divergence by giving each client its own vocabulary.

### What I did

Touched five surfaces:

1. `chat_complete.go` (core): removed `ThinkingLevelMinimal/Low/Medium/High/XHigh/Max`; kept `type ThinkingLevel string` and `ThinkingLevelNone`. Added `PartTypeThought`, `ThoughtPart`, `Part.Thought()`, and threaded `Thought` into `Part.MarshalText`.
2. `clients/openai/chat_complete.go`: published `ThinkingLevelMinimal/Low/Medium/High/XHigh`; rewired the switch to use the new constants and the typed SDK constants (`shared.ReasoningEffortNone/Minimal/.../Xhigh`); added `Usage.ThoughtsTokens` from `chunk.Usage.CompletionTokensDetails.ReasoningTokens` and the matching `ai.thoughts_tokens` span attribute.
3. `clients/google/chat_complete.go`: added `ChatCompleteModelGemini3FlashPreview/ProPreview` constants; published `ThinkingLevelMinimal/Low/Medium/High`; mapped `gai.ThinkingLevelNone` to `ThinkingConfig{ThinkingBudget: gai.Ptr(int32(0))}`; routed `genai.Part.Thought=true` parts through `gai.ThoughtPart` instead of `gai.TextPart`.
4. `clients/anthropic/chat_complete.go`: added `ChatCompleteModelClaudeOpus4_7Latest`; published `ThinkingLevelLow/Medium/High/XHigh/Max`; replaced the panic-on-anything thinking handler with a real switch that sets both `params.Thinking = ...OfAdaptive` and `params.OutputConfig.Effort = ...`. Switched `if req.ResponseSchema != nil` to merge into `params.OutputConfig.Format` rather than overwriting `OutputConfig`. Added a typed `errThoughtRoundTripUnsupported` returned when an inbound `gai.PartTypeThought` is passed (matches PR-251's deferral). Added `anthropic.ThinkingDelta` -> `gai.ThoughtPart` streaming.
5. Tests: rewrote each `thinking_level_test.go` to assert "client publishes constant X" / "panics on Y", added `TestChatCompleter_ChatComplete_Gemini3` (single-turn, dual-model: Flash 3 covers the off path with `gai.ThinkingLevelNone`, Pro 3 covers the on path and asserts `PartTypeThought` parts stream — Flash doesn't surface thought parts on the streaming endpoint, only on batch), added `TestChatCompleter_AdaptiveThinking` (against Sonnet 4.6), added an OpenAI thinking integration test originally named `TestChatCompleter_ChatComplete_GPT5_2` (renamed to `TestChatCompleter_ChatComplete_GPT5_4` in Step 4 once I noticed gpt-5.4 existed). Updated the Anthropic spans test for the bumped default model. Held the Google integration default at `gemini-2.5-flash` (Step 3 below).

`docs/decisions.md` records the architectural choice; this diary holds the empirical narrative.

### Why

The probe results in Step 1 settled three otherwise-tenuous design points: the OpenAI mapping uses real SDK constants instead of string casts, the Anthropic mapping splits across `Thinking` and `OutputConfig` because `Effort` lives on the latter, and the Google mapping for `gai.ThinkingLevelNone` is `ThinkingBudget=0` rather than a symbolic level. PR-251's PartTypeThought shape was re-derived rather than copied — the streaming hooks ended up similar but the level mappings differ everywhere they touch concrete SDK types.

### What worked

`go test -shuffle on ./clients/openai/ ./clients/anthropic/` — green against the live APIs after one fix per provider:

- Anthropic: bumping the default test model to Sonnet 4.6 broke `spans_test.go` which had hard-coded the Haiku model name. One-line fix to use the new constant.
- OpenAI: thoughts-tokens assertion at `ThinkingLevelHigh` was flaky on gpt-5.2 because the model happily skipped reasoning on a one-line prompt and returned `ReasoningTokens=0`. Probed `xhigh` separately, confirmed it consistently triggers reasoning, switched the test.

`golangci-lint run ./...` reports `0 issues.`.

### What didn't work

Bumping the Google default test model to `gemini-3-flash-preview` (per the spec) broke both `can use a tool` integration tests with:

```
Error 400, Message: Function call is missing a thought_signature in functionCall parts.
This is required for tools to work correctly, and missing thought_signature may lead to
degraded model performance. Additional data, function call `default_api:read_file`,
position 2.
```

Ran with `ThinkingLevel: gai.Ptr(gai.ThinkingLevelNone)` to disable thinking — same 400. Gemini 3.x enforces the round-trip even when thinking is off, because the model still returns a signature and the API requires it back. The genai SDK exposes the signature on `genai.Part.ThoughtSignature []byte`, but `gai.Part` doesn't preserve it — same gap as Anthropic's deferred multi-turn signature work (issue #250).

Messaged the lead about the spec conflict; chose option (1) from that message: keep `newChatCompleter` on `gemini-2.5-flash` and add a single-turn `TestChatCompleter_ChatComplete_Gemini3` against `gemini-3-flash-preview` that exercises the new mappings without requiring tool round-trip. Recorded it as a deferred follow-up.

### What I learned

Gemini 3.x's `thought_signature` requirement is a model-shape feature, not a thinking-config feature. Disabling thinking doesn't remove the requirement, so any future work that wants 3.x as the default test model must plumb the signature first.

OpenAI gpt-5.2 is more reluctant to actually reason at lower effort levels than gpt-5/gpt-5.1, even on prompts written to elicit step-by-step thinking. `xhigh` is the reliable signal for "did the model reason at all" in a test.

### What was tricky

The Anthropic two-field shape was the surprise. When the test that confirmed `Effort` alone returns no thinking blocks landed, I rewrote the mapping to set `Thinking.Adaptive` *and* `OutputConfig.Effort` together. That meant the existing `if req.ResponseSchema != nil { params.OutputConfig = ... }` pattern had to change from assignment to merging — otherwise setting a thinking level after a response schema would silently wipe the schema.

The diary's `What didn't work` (Step 1 / Step 2) ate ~30 minutes between probe and code. Worth it: every mapping in the final code is grounded in something I watched return OK or ERR.

### What warrants review

- `clients/anthropic/chat_complete.go`: confirm the OutputConfig merge keeps `Format` and `Effort` independent. The two integration tests that now run against Sonnet 4.6 cover the both-set, neither-set, and effort-only paths, but the schema-only path (`ResponseSchema set, ThinkingLevel nil`) only re-uses the existing structured-output assertion.
- `clients/google/chat_complete.go`: the `Thought:true` -> `gai.ThoughtPart` routing is exercised by the new `TestChatCompleter_ChatComplete_Gemini3/streams_PartTypeThought_and_populates_thoughts_tokens_on_Pro_3`. The subtest hard-asserts `thoughtParts > 0` (along with `Usage.ThoughtsTokens > 0`); the Pro split was chosen exactly because Pro reliably emits thought parts where Flash does not. The off path is asserted by the sibling `disables_thinking_via_gai.ThinkingLevelNone_on_Flash_3` subtest, which asserts zero thought parts and zero thoughts tokens.
- The probe under `internal/probe/` was deleted before commit per the spec ("don't ship the probe"); the findings live in this diary instead.

### Future work

- Plumb `ThoughtSignature` through `gai.Part` so Gemini 3.x and Anthropic adaptive thinking both work for multi-turn tool flows. Same shape of work as issue #250.
- Consider a typed sentinel error in core (`gai.ErrThoughtRoundTripUnsupported`) so callers can `errors.Is` against it instead of regexing the message — currently `clients/anthropic/chat_complete.go:errThoughtRoundTripUnsupported` is package-private.
- A pre-existing test failure exists on `main`: `TestNewClient/can_create_a_new_client_with_the_Vertex_AI_backend_and_a_service_account` panics when `GOOGLE_VERTEX_CREDENTIALS_PATH` is not set (the test reads the path from `.env.test.local` but it's not there for me). My branch doesn't touch `client_test.go`. Worth a separate ticket to gate the test with `t.Skip` when the env var is absent.

## Step 3: Address QA findings — gofmt violations

**Author:** main

### Prompt Context

**Verbatim prompt:** "QA review on PR #257 found two files that are not gofmt-formatted. ... Fix: `gofmt -w chat_complete.go clients/google/chat_complete.go`. Push to the same branch. Do NOT amend the existing commit — make a new fixup commit on `per-client-thinking-levels`."

**Interpretation:** Two const blocks in this PR have stale column padding from before I added new entries. `golangci-lint` missed them because the project's `.golangci.yml` doesn't enable the `gofmt`/`gofumpt` linter; `gofmt -l` catches them. Apply `gofmt -w`, new commit, push.

**Inferred intent:** Keep the working tree gofmt-clean even though CI lint isn't catching it, so this PR doesn't introduce a regression that the next PR has to clean up.

### What I did

Ran `gofmt -w chat_complete.go clients/google/chat_complete.go`. Both files re-padded their const blocks: the `PartType` block lost the now-unnecessary extra spaces (the new `PartTypeThought` line is comment-prefixed and doesn't participate in column alignment), and the Google `ChatCompleteModel*` block re-padded the older 2.x entries to match the longer `Gemini3FlashPreview`/`Gemini3ProPreview` names. Verified with `gofmt -l .` that the tree is clean and `go build ./...` still passes.

Committed as a new commit (`Run gofmt on chat_complete.go and clients/google/chat_complete.go`) rather than amending — keeps the QA fix legible in history.

### Why

The fix is mechanical, but recording it ties the diary to the QA loop: whoever reviews the PR sees the same two-step shape (build commit + format commit) reflected in the narrative.

### What worked

Clean diff: 6 lines moved, no semantic change. `go build ./...` and `golangci-lint run ./...` both still pass.

### What didn't work

Nothing this round.

### What I learned

The project's `golangci-lint` config doesn't include `gofmt`/`gofumpt`. Worth noting if I add a const block in a future PR: don't trust the linter alone; run `gofmt -l .` before pushing.

### What was tricky

Nothing — straightforward mechanical fix.

### What warrants review

Just the diff: padding-only changes to two const blocks. No runtime behaviour changes.

### Future work

Consider adding `gofmt`/`gofumpt` to `.golangci.yml` so this category of issue gets caught in CI rather than human review. Out of scope for this PR.

## Step 4: Add gpt-5.3 / gpt-5.4 constants and re-probe the effort matrix

**Author:** main

### Prompt Context

**Verbatim prompt:** "Markus spotted that gpt-5.2 isn't the newest in the openai-go v3.32.0 SDK we pin — `ChatModelGPT5_4` (and `5_4Mini`, `5_4Nano`, plus `5_3ChatLatest`) exist. Our project constants stop at 5_2, the integration test targets 5.2, and the diary claims 5.2 is the newest. All wrong. Pick up task #5..."

**Interpretation:** I missed four newer constants in the pinned SDK. Add them, re-probe the per-model effort matrix, bump the integration test, update the GoDoc/decisions/diary.

**Inferred intent:** Get the public surface and integration coverage actually targeting the newest reasoning model in the pinned SDK, since the whole point of the per-client redesign was "track what each provider's newest models actually accept."

### What I did

Re-checked `openai-go@v3.32.0/shared/shared.go`: confirmed `ChatModelGPT5_3ChatLatest`, `ChatModelGPT5_4`, `ChatModelGPT5_4Mini`, `ChatModelGPT5_4Nano` exist (plus dated variants I deliberately skipped to match the existing constant convention). Restored the throwaway `internal/probe/main.go` to drive `Chat.Completions.New` against each model with every `ReasoningEffort` value, then deleted the probe before commit (same as Step 1).

Added the four constants to `clients/openai/chat_complete.go` matching the existing block convention. Renamed `TestChatCompleter_ChatComplete_GPT5_2` to `TestChatCompleter_ChatComplete_GPT5_4` and bumped its model to `ChatCompleteModelGPT5_4`. Rewrote the `ThinkingLevel*` GoDoc into a per-model bullet list reflecting probe results. Updated `docs/decisions.md` and the older diary lines to drop the "newest" claim about 5.2.

### Why

The diary's assertion that "gpt-5.2 is the newest chat-completions model" was empirically false the moment I wrote it — the SDK already had 5.3 and 5.4 constants, I just hadn't grepped for them. Markus caught it on his read-through. The fix had to be probe-backed because the SDK enum is just opaque strings; there's no way to know which efforts a given model accepts without asking the API.

### What worked

The probe surfaced concrete answers for the new models:

- `gpt-5.4` / `gpt-5.4-mini` / `gpt-5.4-nano`: accept `none/low/medium/high/xhigh`. No `minimal`. Same matrix as gpt-5.2. gpt-5.4 reliably returns `reasoning_tokens > 0` at every level except `none` (88 tokens at xhigh on the test prompt).
- `gpt-5.3-chat-latest`: accepts ONLY `medium`. Every other level returns 400 with `Supported values are: 'medium'`. It's a chat-tuned model, not a reasoning model — `reasoning_tokens = 0` even at the one level it accepts.

The xhigh test against gpt-5.4 passes deterministically (88 reasoning tokens for the farmer/sheep prompt — well above the >0 bar), same shape as the previous gpt-5.2 test.

### What didn't work

`gpt-5.3-chat-latest` is a worst-of-both for our test design: it appears in the SDK in the gpt-5.x family, but it's chat-tuned with no reasoning effort range, so it can't exercise the level mapping meaningfully. Considered dropping the constant; kept it because callers will still want a typed reference to that model and the per-level error surfaces are honest ("supported values are: 'medium'").

### What I learned

The openai-go SDK ships every advertised model as a string constant, which means "what's pinned in the SDK" is the right reference for "what's available right now" — but I have to actually read the constant block, not extrapolate from the latest one I happen to remember. A `grep ChatModelGPT5_` would have caught this before the first commit.

The chat-tuned vs reasoning-tuned split inside one version family (5.3-chat-latest vs 5.4) is a new shape — earlier 5.x models were uniformly reasoning-capable across efforts. The GoDoc has to call this out per model now, not per major version.

### What was tricky

Deciding whether to drop `gpt-5.3-chat-latest` from the project constants. Net it stays in: the constant surface is "models you can pass to `NewChatCompleter`", not "models that exercise our level matrix". Documenting the constraint at the level constants is enough.

### What warrants review

- `clients/openai/chat_complete.go:25-38` — the new constants block. Confirm the alignment is gofmt-clean (added one constant longer than the previous max width).
- `clients/openai/chat_complete.go:37-66` — the rewritten GoDoc; per-model bullets must match probe findings.
- `clients/openai/chat_complete_test.go:415-422` — the renamed test and the rationale comment for picking gpt-5.4 over gpt-5.3-chat-latest.

### Future work

When openai-go ships a new pinned version, this matrix needs re-probing. Worth a 10-line `TestProbingScript_ManualOnly` (skipped by default) committed alongside the test so the matrix is reproducible without re-reading this diary.
