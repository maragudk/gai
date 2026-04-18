# Diary: span observability for chat completers and embedders

Expanding the OpenTelemetry span instrumentation across `gai`'s three chat clients (OpenAI, Anthropic, Google) and the `robust` wrapper, inspired by Honeycomb's "built for the agent era" post. Two user-facing PRs (#214, #217) merged and one (#218) open for test coverage at the time of writing. Two follow-up issues filed.

## Step 1: Audit existing spans against Honeycomb's recommendations

### Prompt Context

**Verbatim prompt:** "Based on this blog post, are there any interesting data points we could add to the open telemetry tracing throughout the clients? https://www.honeycomb.io/blog/honeycomb-is-built-for-the-agent-era-pt1?utm_source=fnf&utm_medium=blogbanner&utm_campaign=april&utm_term=simon-willison&utm_content=aiblog"

**Interpretation:** Read the blog post, compare its recommended span dimensions against what the three `gai` clients already emit, list concrete gaps worth filling.

**Inferred intent:** Markus wants to tighten the observability story for agent-era workloads without over-engineering — identify high-signal additions that fit the existing `ai.*` attribute convention.

### What I did

Fetched the Honeycomb post and greppped the three `clients/*/chat_complete.go` files plus `robust/*.go` for existing span attributes. Produced a ranked list: TTFT, per-tool-call child spans, cache token metrics, provider-side response IDs, normalised error classifications, finish-reason consistency, and GenAI semantic-convention alignment. Flagged that hallucination/sentiment/thumbs metrics belong in the calling app, not a client library.

### Why

Honeycomb's list is the external forcing function; the audit is what makes the gaps concrete. Also keeps the inventory of "what we already have" handy for the follow-up implementation.

### What worked

Running two parallel greps (`SetAttributes|attribute\.|tracer\.` across `clients/` and `robust/`) at the start gave me a complete map of current attributes in one shot.

### What didn't work

Nothing; pure research step.

### What I learned

`gai` already emits a surprisingly complete baseline — model IDs, token counts, tool lists, temperature, thinking level, retry/fallback actions. The missing pieces are almost entirely about *latency and cost shape*, not volume.

### What was tricky

Distinguishing "would be nice in theory" from "would move a dashboard needle". Landed on TTFT + cache tokens as the two with clearest day-one value.

### What warrants review

Nothing to review at this step.

### Future work

Per-tool-call child spans, provider response IDs, and error classification are real follow-ups but were de-scoped to avoid ballooning the first PR.

## Step 2: Implement TTFT and cache tokens (PR #214)

### Prompt Context

**Verbatim prompt:** "1, 3"

**Interpretation:** Implement items 1 (time-to-first-token) and 3 (cache token metrics) from the audit list, across all three clients.

**Inferred intent:** Start with the cheapest, highest-signal additions. Keep the PR focused.

### What I did

Added `ai.time_to_first_token_ms` in all three clients — measured from just after the streaming call to the first yielded content part. Added `ai.cache_read_tokens` via `chunk.Usage.PromptTokensDetails.CachedTokens` (OpenAI), `chunk.UsageMetadata.CachedContentTokenCount` (Google), and `message.Usage.CacheReadInputTokens` (Anthropic). For Anthropic, also added `ai.cache_creation_tokens` and — after discovering the latent bug below — full `ai.prompt_tokens` / `ai.completion_tokens`.

Bugfix discovered mid-implementation: `clients/anthropic/chat_complete.go` was resetting `message = anthropic.Message{}` inside every `ContentBlockStopEvent`, which wiped `message.Usage` populated by `MessageStartEvent`. Net effect: the Anthropic client had been emitting *zero* token usage attributes silently. Fixed by swapping to `message.Content = nil`, which preserves `Usage` while still preventing re-yield of tool-use blocks.

Flagged the scope expansion to the user before merging. PR #214 merged as `d8975a5`.

### Why

TTFT is the most-asked-about latency metric for agent UIs. Cache tokens are the first thing you look at when a bill spikes. Both are free lifts from the SDKs' existing response structs.

### What worked

The grep-then-edit loop was tight — one edit per client, a single-line SetAttributes addition plus a `time.Since` calculation, then build+test. Full test suite (including integration tests against real APIs) stayed green.

### What didn't work

Nothing catastrophic, but the Anthropic Usage wipe was a surprise — I'd assumed the existing Anthropic client was emitting usage and had to stop and widen scope mid-stream.

### What I learned

Anthropic's `Message.Accumulate`:
- `MessageStartEvent`: `*acc = event.Message` (sets Usage including cache tokens).
- `MessageDeltaEvent`: only updates `OutputTokens` — nothing else.
- So any reset between Start and Delta destroys input+cache counts without obvious symptoms.

### What was tricky

Knowing when to expand scope. The user said "1, 3" but fixing cache tokens for Anthropic without also emitting prompt/completion would have been incoherent. Communicated and proceeded — that was the right call.

### What warrants review

`clients/anthropic/chat_complete.go` — the swap from `message = anthropic.Message{}` to `message.Content = nil` is the subtle bit. If the SDK's `Accumulate` ever starts relying on other fields being reset per-block, this will need revisiting.

### Future work

The TTFT measurement for tool-only responses was collapsed to total generation time (fixed in step 3 after code review flagged it).

## Step 3: Address review on PR #214

### Prompt Context

**Verbatim prompt:** "Code review skill" → "Address review comments skill"

**Interpretation:** Run a dual-reviewer code review on the PR, triage findings one-by-one, then apply the agreed subset.

**Inferred intent:** Catch semantic gotchas before merge; keep the cross-provider contract coherent.

### What I did

Dispatched two competing reviewer subagents. Synthesised their findings into a 10-item ranked list. Walked through each with the user. Applied six fixes:

1. Normalised Anthropic's `ai.prompt_tokens` to `InputTokens + CacheReadInputTokens + CacheCreationInputTokens` so the invariant `ai.cache_read_tokens ⊂ ai.prompt_tokens` holds across all three providers.
2. & 3. Fired TTFT on the earliest signal: `ContentBlockStartEvent` in Anthropic (catches text, thinking, and tool-use blocks), first non-empty `Delta.Content` OR `Delta.ToolCalls` in OpenAI, first part of any type in Google.
4. Moved Anthropic's final usage `SetAttributes` into a `defer` so partial `Usage` is emitted on error paths too.
5. Moved Google's per-chunk `SetAttributes` to a single emission after the stream loop using the last-seen `UsageMetadata`.

Skipped four: int64→int truncation (theoretical), `sync.Once` for closure safety (iterator is single-consumer in practice), adding span tests (too big for this PR — filed issue #215), empty `TextPart("")` yielding in OpenAI (pre-existing — filed issue #216).

### Why

Cross-provider attribute semantics matter more than most things on this list. Users will sum `ai.prompt_tokens` across providers in their dashboards; if Anthropic is systematically low by cache reads + cache creation, that's a silent lie.

### What worked

Parallel reviewer dispatch surfaced real issues neither I nor a single reviewer would have caught alone (e.g. tool-only TTFT collapsing, cross-provider semantic mismatch). Both reviewers independently landed on the `ai.prompt_tokens` normalisation — strong signal.

### What didn't work

Nothing; the review-then-apply loop was smooth.

### What I learned

The earliest TTFT signal in Anthropic streams is `ContentBlockStartEvent`, not `ContentBlockDeltaEvent` with a `TextDelta`. This catches thinking and tool-use blocks automatically without special-casing. Good unifying insight.

### What was tricky

Deciding whether to document the cache-token semantics asymmetry (Google's cached tokens are a subset of prompt tokens; Anthropic's were disjoint pre-fix) in code. Decided the normalisation itself is self-documenting via the diff and a short code comment at the usage point.

### What warrants review

PR #214 commits `d24cd5c` and `a8e12c2` — the latter has the review-driven changes.

### Future work

Issue #215 (span tests) and #216 (empty TextPart) filed.

## Step 4: Fix OpenAI empty TextPart yield (PR #217)

### Prompt Context

**Verbatim prompt:** "Let's fix one of the issues we filed." → "You pick"

**Interpretation:** Pick one of #215 / #216 and ship the fix as a standalone PR.

**Inferred intent:** Keep momentum with a small clean win; #216 is the small one.

### What I did

One-line change in `clients/openai/chat_complete.go`: gated the `yield(gai.TextPart(...), nil)` call on `chunk.Choices[0].Delta.Content != ""` so that empty deltas (role-only chunks, tool-call deltas, finish chunks) no longer produce empty text parts. PR #217 opened, auto-merge enabled, merged as `e050ccf`.

### Why

Pre-existing quirk made visible by the TTFT gate in step 2 — callers were seeing empty `TextPart("")` values interleaved with real text. Harmless if handled defensively, surprising otherwise.

### What worked

Auto-merge + watching checks in the background was the cleanest flow — no polling, no back-and-forth.

### What didn't work

Nothing.

### What I learned

Auto-merge in `gh pr merge --auto` waits out branch-protection rules automatically; when all checks pass it squash-merges and deletes the branch in one atomic step.

### What was tricky

Nothing substantial.

### What warrants review

None; single-line diff with a clear closing issue.

### Future work

None.

## Step 5: Add span-attribute tests (PR #218)

### Prompt Context

**Verbatim prompt:** "Now 215" → "Also for the embedder spans and robust package." → "Yes. Work unattended and create a PR after. Make sure all tests and lint pass."

**Interpretation:** Build the `internal/oteltest` helper infrastructure and use it to add span-attribute tests for all three chat completers, both embedders (OpenAI and Google — Anthropic has no embedder), and the `robust` package wrappers.

**Inferred intent:** Lock in the observability contracts added in #214 so future refactors can't silently drop attributes or reintroduce the Anthropic Usage-wipe bug.

### What I did

Added `/internal/oteltest/oteltest.go`, a small helper cribbed from `maragudk/glue/oteltest`:

- `NewSpanRecorder(t)` installs a `tracetest.SpanRecorder` as the global `otel.TracerProvider` for the test's duration, restores the previous provider on cleanup.
- `FindAttribute` / `HasAttribute` / `FindSpan` / `SpansByName` lookups.
- `RequirePositiveIntAttribute` / `RequireAttributePresent` / (initially also `RequireNonNegativeInt64Attribute`, later dropped) assertion helpers.

Tests added:

- `/clients/{openai,anthropic,google}/spans_test.go` — integration tests hitting real APIs. Each asserts `ai.model`, TTFT presence, token counts, and cache attribute presence on the recorded span.
- `/robust/spans_test.go` — hermetic tests using the existing `fakeChatCompleter` and `fakeEmbedder` from the package's test file. Covers first-try success, retry-then-success, fallback, and exhaustion for both chat and embed paths. Asserts root span config attributes, attempt span counts, `ai.robust.action` values, and the error-path status description.

Pulled in `go.opentelemetry.io/otel/sdk v1.43.0` as a direct dep via `go mod tidy` (transitively brought `github.com/google/uuid` and bumped `golang.org/x/sys`). Aligned to the same 1.43.0 as the existing `otel/trace` pin.

PR #218 opened with all tests and lint green.

### Why

Integration tests lock in the cross-provider observability contract at the edge where it's actually used. Hermetic robust-package tests lock in retry/fallback semantics without burning API quota, and exercise span shapes (root + attempt pairings) that would otherwise need careful manual review.

### What worked

The `oteltest` helper is 108 lines including docstrings and covers every assertion pattern the five test files need. Reusing the existing `fakeChatCompleter` / `fakeEmbedder` doubles in the robust tests meant the hermetic tests were faster to write than the integration ones.

After a first pass I refactored per-client span-assertion helpers (`findSpan`, `requirePositiveIntAttribute`, etc.) out of each test file and into `internal/oteltest` as exported functions. Visible quality improvement — removed ~80 lines of duplication.

### What didn't work

First `go mod tidy` pulled `go.opentelemetry.io/otel/sdk v1.35.0` because that's what was already in the module graph as an indirect dep; the rest of the otel stack was on 1.43.0. Ran `go get go.opentelemetry.io/otel/sdk@v1.43.0 && go mod tidy` to align. The mismatch was cosmetic — tests passed — but left for reviewers the uncomfortable "why is one otel package two minor versions behind?" question.

### What I learned

`tracetest.SpanRecorder` implements `sdktrace.SpanProcessor` directly, so you wire it via `sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))`. No separate exporter needed. `sr.Ended()` returns spans in completion order (not start order) — fine for our assertions because attempt numbers are checked via attribute value, not slice position.

### What was tricky

Ensuring the span recorder is installed *before* the client is constructed. The clients cache their tracer via `otel.Tracer(...)` in `NewChatCompleter` / `NewEmbedder`, not in each call. If a test built the client first and installed the recorder second, the cached tracer would still route to the previous provider and no spans would be recorded — a false-positive passing test. Pattern: call `NewSpanRecorder(t)` as the first line of every test.

### What warrants review

- `/internal/oteltest/oteltest.go` doc comment on `NewSpanRecorder` for the construction-order caveat (tightened in step 6).
- `/robust/spans_test.go` hermetic span assertions — confirm the attempt-span counts and `ai.robust.action` values match the actual retry/fallback path semantics.

### Future work

Thinking-specific TTFT and tool-only TTFT are still thinly tested. No-mock unit tests with synthetic event streams are bigger infra than warranted right now.

## Step 6: Address review on PR #218

### Prompt Context

**Verbatim prompt:** "Use your code review skill"

**Interpretation:** Same pattern as step 3 — dual-reviewer review, triage, apply the agreed subset.

**Inferred intent:** Polish before merge.

### What I did

Dispatched two competing reviewers. Synthesised into six ranked items, walked through each with the user. Applied four fixes:

1. Dropped the Anthropic tool-call "regression" subtest. Reviewer and I both realised that `MessageDeltaEvent.Accumulate` only updates `OutputTokens`, meaning any single `ContentBlockStopEvent` under the old bug would wipe `InputTokens` — so the simple-text subtest *already* catches the regression. The tool-call scaffolding added dependencies (`testdata/`, `os.OpenRoot`, tool execution) for no coverage gain. Added a one-line comment on the remaining subtest calling out its regression-test role.
2. Swapped `ai.time_to_first_token_ms` assertion from `RequireNonNegativeInt64Attribute` to `RequireAttributePresent` — the old check was tautological because `time.Since().Milliseconds()` is always ≥ 0. Deleted the now-unused `RequireNonNegativeInt64Attribute` helper.
3. Added `RequireCacheReadSubsetOfPromptTokens(t, attrs)` asserting `ai.cache_read_tokens ≤ ai.prompt_tokens`. Used in all three client span tests. Locks in the cross-provider contract established in step 3.
4. Tightened the `NewSpanRecorder` doc: construct clients AFTER the call, since earlier-captured tracers may still route to the previous provider.

Skipped two: Google `ai.completion_tokens` flakiness (user judged always-nonzero in practice for "reply with one word" prompts); `tp.Shutdown` timeout (in-memory recorder never blocks).

Committed as `169fc37` and pushed.

### Why

The Anthropic subtest drop is the most interesting call here — reviewer caught that the test's justification was wrong (the "multiple ContentBlockStopEvents" angle wasn't load-bearing). Cheaper test, same guarantee.

The cache-subset-of-prompt invariant closes the loop started in step 3: we normalised the semantics, then in step 5 tested presence, then in step 6 tested the actual invariant.

### What worked

Walking through comments one-by-one with the user stayed focused — avoided the classic "apply everything reviewers said" trap. Two of six were correctly skipped as over-engineering or flakiness that won't actually materialise.

### What didn't work

Nothing.

### What I learned

Regression tests are easy to over-justify. "More scenarios = more coverage" isn't true when the different scenarios all hit the same code path. If a simple case exercises the bug, the elaborate case is weight without value.

### What was tricky

Arguing myself *out* of keeping the tool-call subtest was the main tension — it felt intuitively more thorough, but the mechanics of `Accumulate` meant it wasn't. Stayed disciplined and dropped it.

### What warrants review

The final commit on PR #218 (`169fc37`). Verify the invariant check fires on real token counts, not silently against zeros.

### Future work

Once #218 merges, this series is done. Further observability improvements (per-tool-call spans, response IDs, error classification) are deferred until there's a concrete trigger.
