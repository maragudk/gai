# Diary: Add `PartTypeThought` so chat completions can stream model reasoning alongside text

**Author:** builder

Goal: extend the shared `gai` abstraction with a thought/reasoning part type and surface streamed reasoning content from each provider client (Google, Anthropic, OpenAI) as `PartTypeThought` parts. Caller already requests thinking via `ChatCompleteRequest.ThinkingLevel`; this step closes the loop by letting callers see the thoughts.

## Step 1: Read the spec, sketch the abstraction, and clarify scope with the lead

### Prompt Context

**Verbatim prompt:** "You're the builder on team `thought-tokens`. Pick up task #1 from the shared task list (TaskList → TaskGet 1). Mark it in_progress, claim ownership, and follow the description carefully."
**Interpretation:** Implement the task described in #1 in red/green TDD style, do not commit, leave staged for QA.
**Inferred intent:** Ship a clean diff that QA can review without mutation, with a diary entry capturing decisions (especially Anthropic's budget mapping).

### What I did

Read `/CLAUDE.md`, the gai abstraction in `/chat_complete.go`, all three client implementations, and the relevant SDK source (openai-go v3.31.0, anthropic-sdk-go v1.35.1, google-genai v1.54.0). Sent a message to the lead flagging two scope questions: (a) the OpenAI client uses Chat Completions, not Responses, and Chat Completions does not stream reasoning text — only `CompletionTokensDetails.ReasoningTokens`; (b) my proposed Anthropic budget defaults (Minimal=1024, Low=2048, Medium=4096, High=8192, XHigh=12288).

### Why

The task description assumed the Responses API for OpenAI, and demanded a budget-per-level table for Anthropic. Both warranted explicit confirmation rather than guessing.

### What worked

The cross-SDK reading paid off. I pinned down:
- `genai.Part.Thought bool` is the discriminator on Google's side, controlled by `ThinkingConfig{IncludeThoughts: true}`.
- `anthropic.ThinkingDelta` and `anthropic.ThinkingConfigParamOfEnabled(budget int64)` are the right Anthropic primitives.
- OpenAI Chat Completions has no `reasoning` field on the streaming delta — only token counts in `CompletionTokensDetails.ReasoningTokens`.

### What didn't work

Nothing in this step.

### What I learned

OpenAI's reasoning text only streams via the Responses API (`ResponseReasoningSummaryTextDeltaEvent` etc.). The current gai client is committed to Chat Completions, so the only honest option for OpenAI in this task is to populate token counts and document the gap.

### What was tricky

Recognising that the task description's "OpenAI Responses API streams reasoning summaries" did not match the actual implementation. I chose to surface that to the lead instead of silently switching APIs.

### What warrants review

The decision to limit OpenAI's surface to `Usage.ThoughtsTokens` rather than migrating to Responses. If the lead wants `PartTypeThought` parts from OpenAI too, that is a separate, much larger change.

### Future work

Migrate the OpenAI client to the Responses API in a follow-up so it can yield `PartTypeThought` deltas. Make Anthropic's `budget_tokens` configurable per request rather than tied to `ThinkingLevel` (the level→budget table is a default; some callers may want to override).

## Step 2: Red/green for the abstraction (`PartTypeThought`, `ThoughtPart`, `Thought()`)

### Prompt Context

**Verbatim prompt:** Same task #1.
**Interpretation:** Add a thought part type and constructor mirroring `TextPart`, with a matching accessor.
**Inferred intent:** Keep the new surface minimal and consistent with how text parts work.

### What I did

Wrote a failing test in `/chat_complete_test.go` for `gai.ThoughtPart`, `gai.PartTypeThought`, the `Thought()` accessor, the panic on wrong-type access, and the `MarshalText` round-trip. Confirmed red:

```
./chat_complete_test.go:23:15: undefined: gai.ThoughtPart
./chat_complete_test.go:25:19: undefined: gai.PartTypeThought
./chat_complete_test.go:43:30: gai.TextPart("plain text").Thought undefined
```

Then implemented the surface in `/chat_complete.go`: added `PartTypeThought = "thought"` to the constants, added a `ThoughtPart(text string) Part` constructor, added `Part.Thought() string`, and extended `MarshalText` to emit the thought text. Reused the existing `text *string` field rather than introducing a new one — both `Text()` and `Thought()` read from it, distinguished by `Type`.

### Why

Mirroring `TextPart`/`Text()` keeps the API consistent and lets callers pattern-match on `Type` without learning new accessor names. Reusing the `text` field avoids growing the `Part` struct's footprint.

### What worked

`go test -shuffle on -run TestThoughtPart ./...` went green on the first compile after adding the three pieces. The whole-project build stayed green.

### What didn't work

Nothing in this step.

### What I learned

`Part` already encodes "string-shaped content" via `text *string`. The cleanest extension is to add a `Type` value rather than a parallel field.

### What was tricky

Keeping the panic message style consistent with `Text()` — `"not text type"` → `"not thought type"`. Easy to overlook and would have shown up as a mismatched assertion error.

### What warrants review

`/chat_complete.go` lines around the new constant, `ThoughtPart`, and `Thought()`. Confirm the GoDoc style matches Markus's preference (identifier first, full sentence). Confirm sharing the `text` field is acceptable.

### Future work

If a future provider returns thought signatures or structured reasoning blocks, we may need a richer `thought` value object instead of a plain string — defer until a real need shows up.

## Step 3: Google client — yield `ThoughtPart` for `Thought` parts

### Prompt Context

**Verbatim prompt:** Same task #1.
**Interpretation:** When `ThinkingLevel` is set, ask Gemini to include thoughts and route `Thought == true` parts to `ThoughtPart`.
**Inferred intent:** Preserve existing `ThoughtsTokens` accounting; only add the part-stream.

### What I did

Added a failing integration test under `/clients/google/chat_complete_test.go` that asks Gemini to think about a small word problem with `ThinkingLevel: gai.ThinkingLevelLow` and asserts at least one `PartTypeThought` plus non-empty text output. Then changed `/clients/google/chat_complete.go` in two spots: set `IncludeThoughts: true` on the `ThinkingConfig`, and inside the streaming loop check `part.Thought` to dispatch to `gai.ThoughtPart` vs `gai.TextPart`. The existing `ThoughtsTokens` plumbing was already correct and stayed untouched.

### Why

Without `IncludeThoughts: true` Gemini does not surface thoughts, even with a thinking level set — the thoughts flag is independent of the budget/level.

### What worked

`go build ./clients/google/...` stayed green. The change is isolated to the two minimal spots.

### What didn't work

Cannot run the integration test locally — no API keys in this worktree (no `.env.test.local`). All API-backed tests fail with 401 on this machine; that is environmental and unchanged from before my work.

### What I learned

`genai.ThinkingConfig{ThinkingLevel: ...}` alone does not flip on thought emission; you also need `IncludeThoughts`. Easy to miss because the SDK doesn't error if you forget.

### What was tricky

Nothing meaningful — Google's API is the cleanest of the three for this task.

### What warrants review

That the `if part.Thought { ... } else { ... }` branch in the streaming loop covers all the cases (it does — text-bearing parts only; `FunctionCall` is handled separately). And the new test's prompt should still be small enough to keep the eval cheap.

### Future work

Consider exposing `ThoughtSignature` on the part for multi-turn continuity if Markus ever wants to feed thoughts back into a follow-up turn.

## Step 4: Anthropic client — extended thinking with `budget_tokens` mapping

### Prompt Context

**Verbatim prompt:** Same task #1.
**Interpretation:** Replace the `panic` on `ThinkingLevel` with proper `thinking` config, yield `thinking_delta` as `ThoughtPart`, observe API constraints (no temperature when thinking, max_tokens > budget_tokens).
**Inferred intent:** Document the chosen budget table.

### What I did

Rewrote `/clients/anthropic/thinking_level_test.go` so that all supported levels accept (no panic) and only `Max` panics. Added a unit test for a new private helper `thinkingBudgetForLevel`. Confirmed red:

```
clients/anthropic/thinking_level_test.go:80:15: undefined: thinkingBudgetForLevel
clients/anthropic/thinking_level_test.go:87:12: undefined: thinkingBudgetForLevel
```

Then in `/clients/anthropic/chat_complete.go`:

1. Replaced the panic block with a mapping that:
   - Treats `ThinkingLevelNone` as "no thinking config" (not a panic).
   - Calls `thinkingBudgetForLevel` for the supported levels.
   - Panics on anything else (preserves the existing panic behaviour for `Max`).
2. Added the `thinkingBudgetForLevel` helper with the table:
   - Minimal → 1024 (the API minimum)
   - Low → 2048
   - Medium → 4096
   - High → 8192
   - XHigh → 12288
3. Suppressed `temperature` when thinking is enabled (Anthropic requires it unset/1.0 with extended thinking).
4. Bumped `MaxTokens` so it strictly exceeds `BudgetTokens` if the caller did not override (`max(maxTokens, budget+4096)`).
5. Wired `params.Thinking = anthropic.ThinkingConfigParamOfEnabled(thinkingBudget)`.
6. Added a `case anthropic.ThinkingDelta:` branch in the streaming loop that yields `gai.ThoughtPart(delta.Thinking)`.
7. Added an integration test asserting at least one `PartTypeThought` with `ThinkingLevelLow`.

### Why

The 1024 minimum is the API floor. Doubling at each step keeps the table simple and predictable; Markus can override later by extending the request struct. Suppressing temperature avoids the 400-on-thinking error. The `+4096` headroom is enough for a normal response on top of the budget — under the existing 16,384 default it is still well within bounds.

### What worked

`go test -shuffle on -run "TestChatCompleter_ThinkingLevel|TestThinkingBudgetForLevel" ./clients/anthropic/` went green. `golangci-lint run ./...` reported `0 issues.`

### What didn't work

The Anthropic `Usage` struct does not expose a separate thinking-tokens field — `OutputTokens` includes them. So I left `meta.Usage.ThoughtsTokens` unset for Anthropic; the task description explicitly said "if exposed", which it is not.

### What I learned

Anthropic's `ThinkingConfigParamOfEnabled` returns the union directly; the surrounding `params.Thinking` field is a `ThinkingConfigParamUnion`, not a pointer, so assignment is plain. The SDK will marshal a zero union as omitted.

### What was tricky

The interaction between `temperature`, `thinking`, and `max_tokens` is implicit in the Anthropic API and easy to violate. Lining up all three in the same builder block keeps the invariants visible.

### What warrants review

- Confirm the budget table is sensible for Markus's typical use cases. Easy to adjust later.
- Confirm dropping the user-supplied `temperature` (silently) when thinking is on is acceptable. Alternative: panic to surface the constraint, or expose a `RequireExactTemperature` flag.
- Confirm the `+4096` headroom on `MaxTokens` is a reasonable default.

### Future work

Make `BudgetTokens` configurable per request (struct field on `ChatCompleteRequest` or a per-client option). Surface a clearer error if the caller passes a temperature that would be ignored.

## Step 5: OpenAI client — best-effort given Chat Completions limits

### Prompt Context

**Verbatim prompt:** Same task #1.
**Interpretation:** The task suggested Responses API but the client is on Chat Completions. Do what's possible.
**Inferred intent:** Don't silently break by skipping OpenAI; do the token-count piece and flag the rest.

### What I did

Added a test under `/clients/openai/chat_complete_test.go` that exercises `ThinkingLevelMedium` and asserts `Usage.ThoughtsTokens > 0`. Then updated `/clients/openai/chat_complete.go` to populate `meta.Usage.ThoughtsTokens` from `chunk.Usage.CompletionTokensDetails.ReasoningTokens` and added a span attribute `ai.thoughts_tokens`.

### Why

Chat Completions does not stream reasoning text. The test and the comment in the test body make the limitation explicit so the next reviewer (and future-me) immediately understands why no `PartTypeThought` is yielded for OpenAI in this task.

### What worked

Build clean, lint clean. The relevant unit/abstraction tests pass.

### What didn't work

Cannot verify the live API behaviour without keys. The 401s on this machine are unchanged from before my work — all integration tests in all three client packages share that gating.

### What I learned

`openai.ChatCompletionAccumulator` accumulates `Usage.CompletionTokensDetails.ReasoningTokens` across the stream, so reading from the per-chunk `chunk.Usage.CompletionTokensDetails.ReasoningTokens` (when populated, i.e. on the final chunk) is correct.

### What was tricky

Resisting the temptation to migrate OpenAI to the Responses API mid-task. That would have inflated the diff and made review harder. The lead can scope it as a separate task.

### What warrants review

The new test prompt and the comment block explaining the Chat Completions limitation. The `ai.thoughts_tokens` span attribute name (matches what Google uses).

### Future work

Migrate OpenAI to the Responses API and yield `ResponseReasoningSummaryTextDeltaEvent` deltas as `PartTypeThought`.

## Step 6: Lead held OpenAI for scope; reverted those edits

### Prompt Context

**Verbatim prompt:** "On #1 (OpenAI): hold. Escalating to Markus — it's a scope call. Don't proceed on OpenAI yet. Continue with google + anthropic in the meantime; come back to OpenAI once I've answered."
**Interpretation:** Roll back the OpenAI client and test changes; keep Google + Anthropic in place. Wait for the lead to come back with the scope call.
**Inferred intent:** Don't bake the conservative "ThoughtsTokens-only" interpretation into the diff while Markus is still deciding whether to migrate to the Responses API.

### What I did

`git checkout -- clients/openai/chat_complete.go clients/openai/chat_complete_test.go`. Verified build still clean and lint still clean. Confirmed the diff stat dropped to 8 files / +250 / -11 (was 10 files / +283 / -11). Moved task #1 back to `in_progress` since OpenAI is part of its description.

The lead also reaffirmed the Anthropic budget table as proposed (Minimal=1024, Low=2048, Medium=4096, High=8192, XHigh=12288, None=no config, Max=panics) and the temperature/max-tokens handling. They asked me to record the chosen `MaxTokens` headroom default in this diary — I went with `budget + 4096`. Rationale: the existing `MaxTokens` default is 16,384, so even at XHigh=12288 the bump only matters when the caller explicitly sets a low cap. 4k of headroom is enough for a typical answer on top of the thinking, while still leaving room under the 16k default.

### Why

The lead explicitly said hold on OpenAI; reverting is cheaper and safer than carrying speculative changes through QA. The remaining work (Google + Anthropic + abstraction) is still solid on its own.

### What worked

The revert was clean — no other files reference my OpenAI changes. Build and lint stayed green afterwards.

### What didn't work

Nothing.

### What I learned

When asking the lead a scope question, give them an explicit "default if you don't reply" so the rework on a hold is as small as possible. I had already made the OpenAI change before the lead replied; if I had waited, no revert would have been needed. Next time, the answer to "the lead might say no" is to skip the speculative work, not to do it.

### What was tricky

Distinguishing between "build the conservative interpretation" (what I did) and "wait for the answer" (what the lead actually wanted). The prompt was clear in retrospect — "Ask the lead if requirements are ambiguous" — so I should have held entirely.

### What warrants review

Confirm the worktree is clean of OpenAI changes (git diff shows none). Confirm the diary's earlier OpenAI section (Step 5) is now stale but kept as a historical record of what I attempted. Step 6 onwards is the corrected timeline.

### Future work

Once the lead returns with the OpenAI scope call, redo Step 5 either as conservative (token counts, current PR) or as a Responses-API migration (separate PR).

## Step 7: Markus picked "token count only"; re-applied OpenAI patch

### Prompt Context

**Verbatim prompt:** "Scope decided: **token count only** for OpenAI. No Responses API migration — Markus has a public blog post arguing the Responses API is vendor lock-in, so we keep `clients/openai` on Chat Completions deliberately. Action: re-apply the OpenAI patch from your diary Step 5..."
**Interpretation:** Re-apply the conservative patch (token count + span attribute + test), add an inline code comment naming the rationale, link/reference Markus's blog post in the diary (not in the code comment).
**Inferred intent:** Make it obvious to future readers — and to copy-pasting agents — that Chat Completions is a deliberate choice, not an oversight.

### What I did

Re-applied the diff from Step 5 to `/clients/openai/chat_complete.go` and `/clients/openai/chat_complete_test.go`. Added an inline comment immediately above the `meta.Usage` assignment that explains both halves of the why:
1. Chat Completions does not stream reasoning text, so `PartTypeThought` parts cannot be yielded here.
2. Migrating to the Responses API is a deliberate non-goal — we keep the OpenAI client on Chat Completions for portability across providers.

The test got a one-line follow-up comment pointing at the `chat_complete.go` rationale rather than duplicating it.

The Responses-API-as-vendor-lock-in argument is Markus's, articulated publicly in his blog. The lead asked me to keep that reference in the diary rather than in the code comment, so it stays here for context but does not bleed into the source.

### Why

Per the lead, the rationale is durable enough that a future reader should not need a git blame archaeology session to understand why we did not "fix" the OpenAI client to yield thought parts. An inline comment hits that requirement; a blog-post link in source would couple the file to an external URL we do not control.

### What worked

`go build ./...` clean, `golangci-lint run ./...` reports `0 issues.`, all unit tests pass. The patch was small enough that recovering from the diary was faster than git reflog spelunking would have been.

### What didn't work

Nothing.

### What I learned

When the lead says "hold," interpret as "park the change, not necessarily revert it." A working-tree edit is its own form of staging — keeping it around costs nothing and makes a re-apply free. Reverting was defensible but cost a small re-application later.

### What was tricky

The OpenAI test file had been edited by other contributors between my drafts (helper functions like `requireContainsAny` showed up). I re-anchored the new test on the same insertion point (just before `panics on unsupported MIME type`) to keep the diff readable.

### What warrants review

- Inline comment in `/clients/openai/chat_complete.go` for both technical correctness (Chat Completions really does not stream reasoning text) and tone/length.
- Test prompt in `/clients/openai/chat_complete_test.go` — reasoning-capable model is required for `ReasoningTokens > 0`. The default `ChatCompleteModelGPT5Nano` qualifies.
- That `Usage.ThoughtsTokens` reflects what Markus expects to see in his observability tooling (matches what Google already populates).

### Future work

If a future caller actually wants reasoning text from OpenAI, the path is to add a parallel `clients/openai_responses` package or feature-flag the API choice — either way, do not migrate `clients/openai` itself.

## Step 8: QA flagged a doc gap on ThinkingLevel and ThoughtsTokens

### Prompt Context

**Verbatim prompt:** "QA review surfaced a doc gap... `gai.ThinkingLevel` GoDoc warns about provider-specific silent overrides of `Temperature` and `MaxCompletionTokens`. `ChatCompleteResponseUsage.ThoughtsTokens` GoDoc explains the per-provider differences (so a caller understands `0` may mean 'didn't think' or 'provider doesn't expose')."
**Interpretation:** Lift the trade-offs out of `docs/decisions.md` and into the GoDoc surface so a caller using `go doc` finds them without leaving their editor.
**Inferred intent:** No code behaviour change. Make the silent overrides discoverable from where the API is actually consumed.

### What I did

Extended the GoDoc on `gai.ThinkingLevel` with a paragraph naming both Anthropic-side overrides (drops `Temperature`, bumps `MaxCompletionTokens`) and pointing readers at `docs/decisions.md` for full per-provider detail. Replaced the bare `ChatCompleteResponseUsage` struct with full per-field GoDoc, including a paragraph on `ThoughtsTokens` that spells out the three providers' behaviour and warns that `0` is ambiguous (didn't think vs. provider does not surface the count). Verified the rendered output via `go doc maragu.dev/gai.ThinkingLevel` and `go doc maragu.dev/gai.ChatCompleteResponseUsage` — both read cleanly.

### Why

QA's point was sharp: callers who use `go doc` (Markus's primary doc-discovery tool, per CLAUDE.md) had no way to learn about the silent overrides without reading the decision log. Lifting the constraints into the GoDoc keeps the API self-describing.

### What worked

The fields struct GoDoc renders correctly with one trailing comment-block per field. `golangci-lint run ./...` reports `0 issues.`

### What didn't work

Nothing.

### What I learned

`go doc` shows struct field comments inline when they sit immediately above each field — easy to forget when bulk-adding docs. Stuck with that idiom for consistency with the rest of the file (e.g. `ChatCompleteResponseMetadata.FinishReason`).

### What was tricky

Naming the Anthropic-as-zero behaviour for `ThoughtsTokens` without making it sound like a bug. Settled on "the SDK's Usage struct does not expose a separate thinking-token count (those tokens are folded into output_tokens)" — matter-of-fact, blames the SDK rather than our code.

### What warrants review

The wording of both GoDoc paragraphs. They are user-visible API surface, so QA / Markus might want to tighten the prose. The `docs/decisions.md` entry is referenced but not linked (Go GoDoc does not render relative file links the way GitHub does); I assumed a quoted path is the convention.

### Future work

Once `MaxCompletionTokens` becomes per-request configurable for thinking budgets (queued elsewhere), the corresponding GoDoc paragraph can simplify. Same goes for the day Anthropic's SDK exposes a thinking-token field.

## Step 9: Lead tightened the GoDoc spec; folded ThoughtsTokens semantics onto ThinkingLevel; cleaned up dead import

### Prompt Context

**Verbatim prompt:** "Pick up tasks #3 and #6 now... #3 Add GoDoc to `gai.ThinkingLevel` documenting [four bullets]. Keep it tight — one paragraph, no marketing. #6 Remove the dead `anthropic-sdk-go` import in `thinking_level_test.go`."
**Interpretation:** Re-do #3 with everything in a single tight paragraph on `ThinkingLevel`, including the per-provider `ThoughtsTokens` semantics (which I had previously split out onto `ChatCompleteResponseUsage`). Remove the `var _ = anthropic.ModelClaudeSonnet4_0` line and its import.
**Inferred intent:** Single source of truth for the thinking-mode trade-offs at the entry point a caller actually reads when setting up a request. Strip the scaffolding I left behind.

### What I did

Rewrote the `ThinkingLevel` GoDoc as one paragraph hitting all four points: panic on unsupported levels, Anthropic dropping `Temperature`, Anthropic bumping `MaxCompletionTokens`, and the per-provider `ThoughtsTokens` semantics with the explicit "no `PartTypeThought` parts on Chat Completions" callout for OpenAI and "Anthropic does not populate" for Anthropic. Simplified `ChatCompleteResponseUsage.ThoughtsTokens` to a one-liner pointing at `ThinkingLevel` for the cross-provider story — keeps a single source of truth.

For #6, removed the `github.com/anthropics/anthropic-sdk-go` import and the trailing `var _ = anthropic.ModelClaudeSonnet4_0` line in `clients/anthropic/thinking_level_test.go`. The line was a leftover from an earlier iteration that briefly considered referencing an SDK type — never materialised, and the `var _` did nothing useful.

Verified the rendered output:

```
$ go doc maragu.dev/gai.ThinkingLevel
type ThinkingLevel string
    ThinkingLevel controls how much reasoning effort the model applies.
    Not all levels are supported by all providers; unsupported levels
    will panic. When set, the Anthropic client drops caller-supplied
    ChatCompleteRequest.Temperature (the API requires it) and may
    bump ChatCompleteRequest.MaxCompletionTokens past the caller's
    value if it would collide with the per-level thinking budget.
    ChatCompleteResponseUsage.ThoughtsTokens semantics differ per provider:
    Google populates it, OpenAI populates it as a token count only (Chat
    Completions does not stream reasoning text, so PartTypeThought parts are not
    yielded for OpenAI), and Anthropic does not populate it (the SDK does not
    expose a separate thinking-token field).
```

`go build ./...`, `golangci-lint run ./...` (`0 issues.`), and the unit-test sweep are all green.

### Why

The lead was right to consolidate. The earlier two-paragraph split on `ChatCompleteResponseUsage` was load-bearing prose that callers had to find by drilling into a field comment. Hoisting it onto the entry point (`ThinkingLevel`) means setting up a thinking request and reading the entry-point doc surfaces all the trade-offs together. The dead import was a code smell that only didn't trip lint because the package-level var-blank counted as a use.

### What worked

The rewrite was straight prose surgery — no logic changes, no test churn. `go doc` rendered the paragraph cleanly with normal line wrapping.

### What didn't work

Nothing.

### What I learned

When a step nests many semantic bullets under one identifier, write them as a single paragraph rather than as a bullet list — Go GoDoc renders fine but the reading experience in `go doc` is more linear. Save bullets for genuinely parallel options.

### What was tricky

Preserving the cross-references — `[ChatCompleteRequest.Temperature]`, `[ChatCompleteRequest.MaxCompletionTokens]`, `[ChatCompleteResponseUsage.ThoughtsTokens]`, `[PartTypeThought]` — without making the paragraph feel hyperlink-heavy. Settled on referencing each identifier exactly once, in the order a caller would mentally trace through them.

### What warrants review

- The wording of the `ThinkingLevel` paragraph; QA / Markus might still want to tighten further.
- Whether the deletion of the `var _` in `thinking_level_test.go` accidentally lost any future-proofing intent. I do not think so — the comment "in case the SDK type drifts" describes a non-mechanism. If the SDK drifts, the actual `ChatCompleteModelClaudeSonnet4Latest` constant in `chat_complete.go` is what would need updating, and that is what the test exercises via `model: ChatCompleteModelClaudeSonnet4Latest`.

### Future work

None for #3 / #6. Tasks #4, #5, #7 remain pending Markus's scope call per the lead's instruction.

## Step 10: Markus + lead redesigned the whole thinking surface; subsumed #4/#5/#7

### Prompt Context

**Verbatim prompt:** "Markus and I redesigned. Pick up task #8 — it's a consolidated redesign that subsumes #4, #5, #7 and tightens the Anthropic side significantly... gai does not pre-validate provider constraints. API errors surface verbatim. That means the Temperature drop and MaxCompletionTokens auto-bump you added on Anthropic come out. Multi-tier budget table also comes out. Anthropic is now None + Minimal only. Other levels panic."
**Interpretation:** Tear down the cleverness. Anthropic supports only `None` (explicit disabled) and `Minimal` (1024 budget); everything else panics. Drop the silent overrides. Round-trip `PartTypeThought` correctly per provider (Google: emit `Thought: true`; Anthropic & OpenAI: silent drop). Replace the previous decisions entry; create a GitHub issue for the deferred multi-turn signature problem; rewrite `ThinkingLevel` GoDoc and the diary.
**Inferred intent:** Stop guessing at provider semantics in the abstraction layer. Surface honest API errors instead of pretending portability that does not exist.

### What I did

This was a sweeping change covering the abstraction, all three clients, GoDoc, the decisions log, and a new GitHub issue. Concretely:

**Anthropic** (`/clients/anthropic/chat_complete.go`, `/clients/anthropic/thinking_level_test.go`, `/clients/anthropic/parts_test.go`, `/clients/anthropic/chat_complete_test.go`):

- Replaced the level→budget switch with a tight three-way: `None` → `ThinkingConfigDisabledParam{}` (explicit disable), `Minimal` → `ThinkingConfigParamOfEnabled(1024)`, anything else → panic.
- Removed `thinkingBudgetForLevel` entirely — only one supported level needs no helper.
- Removed the temperature-drop guard. Caller's `Temperature` flows through unchanged.
- Removed the `max_tokens > budget` auto-bump. Caller's `MaxCompletionTokens` flows through unchanged.
- Added `case gai.PartTypeThought: continue` in the request-build switch with a comment pointing at the deferred multi-turn signature work.
- Updated `thinking_level_test.go` so only `none` and `minimal` accept; the rest panic. Removed `TestThinkingBudgetForLevel` (helper is gone).
- Updated the integration test in `chat_complete_test.go` from `ThinkingLevelLow` to `ThinkingLevelMinimal`.
- Added `parts_test.go` with `TestChatCompleter_AcceptsInboundThoughtParts` asserting no panic when a thought part appears in history.

**Google** (`/clients/google/chat_complete.go`, `/clients/google/thinking_level_test.go`, `/clients/google/parts_test.go`):

- Added `case gai.ThinkingLevelNone:` setting `ThinkingConfig{ThinkingBudget: genai.Ptr[int32](0)}`. Confirmed `genai.Ptr` exists in the SDK at `genai@v1.54.0/common.go:36`.
- Added `case gai.PartTypeThought:` in the request-build switch that emits `&genai.Part{Text: part.Thought(), Thought: true}` so streamed thoughts can round-trip into history.
- Updated `thinking_level_test.go` from `panics on none` to `accepts none`.
- Added `parts_test.go` with the same round-trip assertion.

**OpenAI** (`/clients/openai/chat_complete.go`, `/clients/openai/parts_test.go`):

- Added `case gai.PartTypeThought: continue` in both the user and model-role request-build branches with an inline comment ("Chat Completions has no concept of inbound reasoning content").
- Added `parts_test.go` with the same round-trip assertion.

**Abstraction** (`/chat_complete.go`):

- Rewrote `ThinkingLevel` GoDoc. Removed the now-stale "Anthropic drops Temperature" / "Anthropic bumps MaxCompletionTokens" language. Added the new "None + Minimal only on Anthropic" line and the "None is sent as explicit-disable" line. Kept the cross-provider `ThoughtsTokens` paragraph; it is still the canonical place for that asymmetry.

**Decisions log** (`/docs/decisions.md`):

- Replaced the previous "Anthropic extended-thinking budget mapping per `gai.ThinkingLevel` (2026-04-27)" entry in place. New entry "Anthropic extended-thinking scope: None and Minimal only (2026-04-28)" documents the redesign, the principle ("gai does not pre-validate provider constraints; API errors surface verbatim"), and the deferred follow-ups (per-request budget knob, full level coverage, multi-turn signature round-trip linked to issue #250).

**GitHub issue** (https://github.com/maragudk/gai/issues/250):

- Created via `gh issue create --repo maragudk/gai`. Title: "Round-trip Anthropic extended-thinking blocks with signatures for multi-turn tool use". Body explains the constraint, the current silent-drop behaviour, what a fix would entail (extend `Part` to carry opaque provider metadata; populate signature on stream-read; emit `ThinkingBlockParam` on request-build; handle `redacted_thinking`), and references the decisions log entry.

### Why

Markus's principle is the right one: a portability layer that pre-validates provider constraints ends up wrong on every provider in some way, and the wrongness shows up as silent surprises (the `MaxCompletionTokens: 4000 → 8192` bump being the worst-case footgun). Letting API errors surface verbatim makes the contract honest. The doubling-table I proposed at the start was clever-feeling but exactly the kind of fake portability the new principle calls out.

The drop on `Low/Medium/High/XHigh` for Anthropic is the painful piece. It means a caller writing portable code with `ThinkingLevel: Medium` now has to branch on provider. That is the right cost: the alternative was to keep inventing budget numbers that do not mean anything.

### What worked

- The redesign was actually smaller than the original implementation. Less code to maintain, less prose in the decisions log, fewer test cases.
- `genai.Ptr[int32](0)` is a clean way to express "explicit disable" without writing a one-liner helper.
- All three `parts_test.go` files share the same shape, which makes them easy to read in a review.

### What didn't work

Nothing major. One small nit: `gh issue create` printed the issue URL, and I used that in the decisions log. If the upstream issue is later moved or renumbered, the link rots — but that is the standard tradeoff with cross-document references.

### What I learned

When the lead says "redesign, here's the principle," the first move is to delete code, not to add code. I caught myself reaching for "well, what if we kept Low and Medium with a small linear table" — and then realised that is exactly what the principle forbids.

### What was tricky

The `ThinkingConfigParamUnion` struct has an `OfDisabled` field that is `*ThinkingConfigDisabledParam`. The SDK exposes a `NewThinkingConfigDisabledParam()` constructor (returns the value, not the pointer); I had to use `&anthropic.ThinkingConfigDisabledParam{}` to fit the union shape, since `NewThinkingConfigDisabledParam` does not return a pointer. Confirmed this matches the union's expected shape by reading `message.go:6334-6346`.

### What warrants review

- The drop on `Low/Medium/High/XHigh` for Anthropic is a public-API breaking change relative to my earlier draft (which QA had already verified). Worth confirming this is what Markus wants — the lead's message was clear, but Markus has not directly confirmed the panic-on-Low behaviour.
- The Google round-trip emits `Thought: true` parts back to the API. I have not exercised this with a real Gemini call (no API keys in this worktree). If the API rejects round-tripped thoughts — for example because they need an opaque signature like Anthropic's — the existing integration test will reveal that on Markus's machine.
- The new `parts_test.go` files are minimal. They prove no-panic but do not prove the request-build emits the right thing for Google. Stronger guarantee would require extracting message-build into a testable private function — out of scope for this task.

### Future work

- If demand emerges for higher Anthropic thinking budgets, add a per-request `BudgetTokens` knob rather than reintroducing the level table. The decisions log says so.
- Resolve issue #250 once a multi-turn-thinking-with-tools caller actually exists.
- Consider whether OpenAI's silent-drop should also fire a debug log line, for consistency with how other "unexpected input" cases are handled. Currently it really is silent.

## Step 11: Quality gates and handoff

### Prompt Context

**Verbatim prompt:** Same task #1.
**Interpretation:** Run the gates listed in the task before marking it complete.
**Inferred intent:** Hand off to QA without doubt about format/lint/test breakage from my edits.

### What I did

Ran:
- `go build ./...` → clean
- `go test -shuffle on -run "TestThought|TestPart|TestThinkingBudget|TestChatCompleter_ThinkingLevel|TestDataPart|TestPart_MarshalText|TestGenerateSchema" ./...` → all pass
- `golangci-lint run ./...` → `0 issues.`
- `go test -shuffle on ./...` → only API-key-gated tests fail (401 Unauthorized), pre-existing environmental.

### Why

These are the gates the task description called out, plus one extra (`go build`) for early signal.

### What worked

The unit/abstraction tests all pass without an API key. Lint is clean. No new lint warnings introduced.

### What didn't work

Cannot drive the integration tests against real APIs from this worktree — no `.env.test.local`. That is the same state any builder agent would face here. The lead/QA should run them with keys before merging.

### What I learned

Confirms my earlier assumption: the test gating on this repo is "have the env file or expect 401s," not `testing.Short`. Future tests should follow the same pattern (no `t.Skip`).

### What was tricky

Discriminating "my changes broke tests" from "tests need keys." The 401 messages from the OpenAI SDK are clear once you know to look for them.

### What warrants review

Run `go test -shuffle on ./...` with `.env.test.local` present and confirm:
- `clients/google` `yields thought parts when thinking is enabled` passes
- `clients/anthropic` `yields thought parts when thinking is enabled` passes
- `clients/openai` `populates thoughts tokens when thinking is enabled` passes (and that the gpt-5-nano default produces `ReasoningTokens > 0` — if not, switch to a different model in the test)
- All previously-passing integration tests still pass

### Future work

Consider gating integration tests on a build tag or env-var sentinel so a clean `go test ./...` without keys can be green. Out of scope here.
