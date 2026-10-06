# Diary: Align token usage across chat completer clients

Make `gai.ChatCompleteResponseUsage` mean the same thing for the OpenAI, Google, and Anthropic clients. Before this change the type had no documented semantics, and the clients filled it in incompatible ways.

## Step 1: Investigate current mapping and agree on requirements

**Author:** main

### Prompt Context

**Verbatim prompt:** Let's align the token usage between clients (input, thought, output, whatever else). First, investigate how they're currently mapped, and see whether we can find a common ground.
**Interpretation:** Map how each client populates usage today, check each provider's own semantics in the SDK sources, and propose one shared meaning for every usage field.
**Inferred intent:** Callers should be able to read, sum, and cost token usage the same way whichever provider they use.

### What I did

A research sub-agent read `/chat_complete.go`, the three `/clients/*/chat_complete.go` files, `/robust/chat_completer.go`, the tests, `/docs/observability.md`, recent diaries, and the usage structs and doc comments in the SDK sources in the module cache (openai-go v3.71.1, genai v1.71.0, anthropic-sdk-go v1.78.0). I presented the findings to Markus and asked three questions. He chose:

- `ThoughtsTokens` as a subset of `CompletionTokens`.
- Both cache read and cache write counts added to the usage struct.
- No extra scope. He left the scope question unanswered, so the stated default applies: embeddings, `robust` aggregation, eval logs, and Google tool-use tokens are all out.

I recorded the decision as "Token usage normalisation (2026-10-06)" in `/docs/decisions.md`.

### Why

Without documented semantics, a caller cannot sum or cost usage across providers. The 2026-10-05 finish-reason diary already flagged the thoughts mismatch as future work.

### What worked

Prompt tokens were already consistent across clients. PR #214 made Anthropic sum `InputTokens + CacheReadInputTokens + CacheCreationInputTokens`, and OpenAI and Google report prompt counts that already include cache. So the inclusive "prompt" convention already existed. This change extends the same rule to output tokens.

### What didn't work

Nothing failed. This step was research only.

### What I learned

- **OpenAI**: `CompletionTokens` includes reasoning. The `RejectedPredictionTokens` doc comment says so: "like reasoning tokens, these tokens are still counted in the total completion tokens". `CachedTokens` is part of `PromptTokens`.
- **Anthropic**: `output_tokens` is "the inclusive, authoritative total". `OutputTokensDetails.ThinkingTokens` is always ≤ `output_tokens`, but it is re-tokenized, so it is approximate. `input_tokens` excludes both cache reads and cache writes.
- **Google**: `PromptTokenCount` includes cached content. `TotalTokenCount` is documented as prompt + candidates + tool-use prompt + thoughts. So `CandidatesTokenCount` excludes thoughts. This is inferred from the total's definition; no doc comment says it directly.
- **Streaming timing differs**:
  - OpenAI sends usage only on the final chunk.
  - Google sends partial usage on every chunk.
  - Anthropic sends final input counts at `message_start` and final output counts at `message_delta`.

  All three are correct once `Parts()` is drained. That is the only guarantee we document.

### What was tricky

Google's thoughts-excluded behaviour is inferred, not stated. The live thinking tests should confirm it after the change by checking `ThoughtsTokens <= CompletionTokens`.

### What warrants review

- The decision entry in `/docs/decisions.md`.
- The silent behaviour change: Google's `CompletionTokens` rises on thinking models.

### Future work

- Usage on `EmbedResponse`.
- Summing usage across `robust` attempts.
- Token counts in `evals.jsonl` (the TODO at `/eval/run.go:88`).
- Folding Google `ToolUsePromptTokenCount` into `PromptTokens` if gai ever exposes built-in Gemini tools.

## Step 2: Implement the usage contract in the core type and the three clients

**Author:** usage-builder

### Prompt Context

**Verbatim prompt:** "You're building the "align token usage across chat completer clients" feature in the Go repo `maragu.dev/gai`. [...]" (The lead's full brief gave the GoDoc semantics, the new field layout, the per-client mapping, the span attribute set, the live test assertions, the docs to update, the gates, and the out-of-scope list summarised in Step 1.)
**Interpretation:** Document `gai.ChatCompleteResponseUsage`, add `CacheReadTokens` and `CacheWriteTokens`, reorder the fields, move each client's mapping into a package-private `mapChatUsage` with table-driven internal tests, derive one consistent set of span attributes from the mapped usage, tighten the live tests, and update `/docs/observability.md`.
**Inferred intent:** Callers and dashboards can read, sum and cost token usage the same way whatever provider sits behind the `gai.ChatCompleter`.

### What I did

I checked the SDK doc comments with `go doc` first. OpenAI's `CompletionUsagePromptTokensDetails.CacheWriteTokens` says "The unadjusted number of prompt tokens written to cache", inside a struct documented as "Breakdown of tokens used in the prompt". That reads as part of the prompt tokens, so I went ahead, as the brief allowed. The word "unadjusted" is not explained anywhere in the SDK.

Red: I wrote `TestMapChatUsage` in `/clients/openai/chat_complete_internal_test.go` (new file), `/clients/google/chat_complete_internal_test.go` and `/clients/anthropic/chat_complete_internal_test.go`. Each covers zero usage, plain prompt and completion, thinking, and cache. Google also has a row that pins `ToolUsePromptTokenCount` as excluded. `go vet ./clients/...` failed with `unknown field CacheReadTokens in struct literal of type gai.ChatCompleteResponseUsage` and `undefined: mapChatUsage`.

Green:

- `/chat_complete.go`: the type and each field now have GoDoc, in the order Prompt, CacheRead, CacheWrite, Completion, Thoughts. The only struct literal outside the clients (`/robust/chat_completer_test.go`) is keyed, so the reorder broke nothing.
- Each client has a `mapChatUsage` next to `mapChatFinishReason`. OpenAI maps fields one to one. Google sets Completion to `CandidatesTokenCount + ThoughtsTokenCount` and CacheRead to `CachedContentTokenCount`. Anthropic keeps the prompt sum and adds CacheRead and CacheWrite.
- The span attributes now come from `meta.Usage` in a defer in all three clients. Every client emits `ai.prompt_tokens`, `ai.cache_read_tokens`, `ai.completion_tokens`, `ai.thoughts_tokens` and `ai.total_tokens` (prompt + completion). OpenAI and Anthropic also emit `ai.cache_creation_tokens`. OpenAI previously set its attributes inline on the usage chunk and took `ai.total_tokens` from the provider. Both changed: the attributes now come from the defer, and the total is computed. The provider's documented total is the same sum. When `meta.Usage` is updated is unchanged in all three clients.
- Live tests: each test file gets a `requireUsageSubsets` helper that logs the usage and asserts Thoughts ≤ Completion, CacheRead ≤ Prompt and CacheWrite ≤ Prompt. It is called from the usage, max-token and thinking-matrix tests. The span tests now also check `ai.thoughts_tokens` and `ai.total_tokens`. OpenAI also checks `ai.cache_creation_tokens`. Google checks that `ai.cache_creation_tokens` is absent. Google's usage test also asserts that Completion > Thoughts.
- `/docs/observability.md`: the attribute table rows, a short note that the token attributes mirror `gai.ChatCompleteResponseUsage`, the invariants section, and the GenAI mapping row for `ai.completion_tokens`. I also corrected the `ai.finish_reason` row from "openai" to "all", because #381 made it stale. That is the only edit outside the brief. No README and no `/docs/index.html` mentions usage, so I left them alone.

### Why

One mapping function per client keeps the provider-specific arithmetic in one place, where table tests that need no API key can check it. Deriving the span attributes from `meta.Usage` means the spans and the Go API cannot drift apart.

### What worked

The red/green loop on the mapping functions was quick. The live runs passed:

- `go test -count=1 -shuffle on -v ./clients/openai/ -run 'TestChatCompleter|TestMapChat'` passed in 106.6s.
- The same for `./clients/anthropic/` passed in 44.4s.
- The same for `./clients/google/` with `-skip '/service_account'` passed in 114.8s.

`golangci-lint run` reported 0 issues.

### What didn't work

My first attempt to append the tests used a shell heredoc. The sandbox refused it: "this command is too complex to verify that it stays inside the worktree". I used the Edit tool instead.

A temporary probe failed to build: `new(float32(0)) requires go1.26 or later`. The go skill prefers `new()`, but this module's `go.mod` targets an older Go, so I used `genai.Ptr` in the throwaway probe.

`go test -count=1 -shuffle on ./...` failed only in `/clients/google`: `panic: project/location or API key must be set when using Vertex AI backend`, from `can_chat-complete_via_Vertex_AI_with_service_account`. `/.env.test.local` has no `GOOGLE_VERTEX_CREDENTIALS_PATH`, which the 2026-10-05 diary also hit. The panic aborts the rest of that package's tests. So I reran it with `go test -count=1 -shuffle on ./clients/google/... -skip '/(service_account|Embedding_2_on_Vertex_AI)'`, which passed in 122.1s. Every other package passed.

### What I learned

- A throwaway probe calling `Models.GenerateContent` directly confirmed the Step 1 inference that Gemini's candidates count excludes thoughts. `gemini-2.5-flash` returned prompt=24, candidates=94, thoughts=470, total=588. `gemini-3-flash-preview` returned 24/104/162/290. In both, total = prompt + candidates + thoughts exactly.
- Under `MaxOutputTokens: 3`, both Gemini models returned candidates=0, thoughts=0, total=prompt, with `MAX_TOKENS`. So the Google max-tokens test still passes with Completion including thoughts, at Completion=0. It is surprising that Gemini reports no thought tokens even though the cap was evidently used up before any text.
- A second probe, a ~3,600-token system prompt sent three times, showed OpenAI `gpt-5.4-nano` cache reads working: CacheRead=2816, Prompt=3616 on calls 2 and 3. OpenAI reported `CacheWriteTokens: 0` even on the cold first call, so the "unadjusted" cache-write field could not be observed live. Gemini 2.5 Flash's implicit cache did not hit in three calls. Anthropic prompt caching needs `cache_control`, which gai does not set, so Anthropic cache counts are always zero through gai today.

### What was tricky

The probes had to run as `_test.go` files inside the worktree, because the module's dependencies are only reachable from there. I deleted them (`/clients/google/zz_probe_test.go`, `/clients/openai/zz_probe_test.go`) straight after each run.

### What warrants review

- The GoDoc on `gai.ChatCompleteResponseUsage` in `/chat_complete.go`.
- The three `mapChatUsage` functions and their table tests.
- That the attribute defers run before `span.End()`. They are registered after it, so they run first under LIFO.
- The OpenAI `CacheWriteTokens` "unadjusted" wording (see Future work).

### Future work

See Step 3.

## Step 3: Self-review and follow-up fixes

**Author:** usage-builder

### Prompt Context

**Verbatim prompt:** "Self-review your work." (from the lead's brief)
**Interpretation:** Run the code-review skill over the diff, fix the real findings, and report the rest honestly.
**Inferred intent:** Hand the lead a change that is ready to review, with its limits stated.

### What I did

I ran `/code-review high`. It returned six findings.

I acted on two:

- **Anthropic emitted token attributes with no usage.** The Anthropic defer set every token attribute unconditionally, so a call that failed before `message_start` recorded `ai.total_tokens=0` and so on. OpenAI and Google leave the attributes off in that case. I added a `hasUsage` guard, set after the first accumulated event, because `message_start` carries usage. I also added a sentence to `/docs/observability.md` saying that the token attributes are set only when the provider reported usage. This also removes the old `ai.prompt_tokens=0` noise on failed Anthropic calls, which is a small behaviour change for dashboards.
- **Unverified claim in the Google max-tokens test.** The test comment claimed that Gemini's cap covers thoughts, which was an inference. I reworded it to state the observation: under a cap this small, Gemini has reported zero candidates and zero thoughts tokens.

I deliberately left four alone:

- **Google `ToolUsePromptTokenCount` is excluded, so the GoDoc's "PromptTokens + CompletionTokens is the total billed token count" is not strictly true for Gemini built-in tools.** The brief and the decision put this out of scope, and gai exposes no built-in Gemini tools, so the count is zero in practice. Raised as an open question for the lead.
- **Share the six-attribute block across clients through an internal package.** Each client already owns its finish-reason mapping in the same way, and Google omits `ai.cache_creation_tokens` on purpose. A shared helper would need a new internal package for a few lines.
- **Move `requireUsageSubsets` into `/internal/oteltest`.** That package is about spans, and `drainParts` is already duplicated per client test file, so I kept the pattern.
- **OpenAI `CacheWriteTokens` "unadjusted" may not be a subset.** This is a real uncertainty, but the SDK places it in the prompt-token breakdown, and the probe in Step 2 shows OpenAI returns 0 for it on `gpt-5.4-nano`. Raised as an open question.

Final checks after the fixes:

- `golangci-lint run`: 0 issues.
- `go test -count=1 -shuffle on ./clients/anthropic/`: passed (45.0s).
- The Google max-tokens subtest and `TestMapChat`: passed.
- `go test -count=1 ./ ./robust/`: passed.

### Why

Consistent "no usage, no attributes" behaviour across the three clients is what requirement 5 asked for in spirit. The documentation should state what the live runs showed, not what I inferred.

### What worked

The reviewer's inconsistency finding was cheap to fix, and it made the observability doc statement true for all three clients.

### What didn't work

Nothing failed in this step.

### What I learned

The live tests only trivially check the cache-subset invariants, because no test produces a cache hit. CacheRead and CacheWrite were 0 on every logged test run.

### What was tricky

Deciding whether the Anthropic guard counts as changing streaming timing. It does not: it only affects span attributes on failed calls, and `meta.Usage` is still updated after every event.

### What warrants review

- The new `hasUsage` guard in `/clients/anthropic/chat_complete.go`.
- The `ai.finish_reason` row fix in `/docs/observability.md`, which is outside the brief.

### Future work

- A live OpenAI test that sends a ~3,600-token system prompt twice and asserts `CacheReadTokens > 0` on the second call. This would make the cache-read invariant non-trivial. The probe showed it works with `gpt-5.4-nano`, but a cache hit is not guaranteed, so it could flake.
- Clarify what OpenAI means by "unadjusted" cache write tokens if a model ever reports a non-zero value.
- `GOOGLE_VERTEX_CREDENTIALS_PATH` is still missing from `/.env.test.local`, so the service-account Vertex test cannot run locally.

## Step 4: Second opinion and closing the open questions

**Author:** main

### Prompt Context

**Verbatim prompt:** Do second-opinion after builder done
**Interpretation:** Once the builder finishes, have an independent model (codex) review the uncommitted diff, and settle the builder's open questions.
**Inferred intent:** Catch anything that the builder's own review and mine missed before the change lands.

### What I did

- **Codex review.** I ran `codex exec -s read-only` with `gpt-6-astra` at high effort and an adversarial prompt. It checked the diff and each mapping against the provider SDK doc comments, and gave a view on the builder's three open questions.
- **My own review.** I read the core, OpenAI, Google and Anthropic diffs myself while codex ran.
- **Settled with Markus:**
  1. Gemini tool-use prompt tokens stay unmapped. gai will not support Gemini's built-in server-side tools (Search grounding, code execution, URL context), and that count only covers those tools, so it is always zero. I amended the decision entry in `/docs/decisions.md` to say so and removed the item from the out-of-scope list. I changed no code or GoDoc.
  2. No live cache-hit test.

### Why

The builder flagged an unverified assumption: whether OpenAI's cache-write tokens are a subset of prompt tokens. It also flagged a possible overclaim in the GoDoc's billed-total sentence. Both needed an independent check before we commit to that contract.

### What worked

- **The core mappings held up.** Codex verified all three against the pinned SDK sources and found no introduced bug apart from the total claim. It also confirmed that the deferred span attributes run before `span.End()`.
- **OpenAI cache writes are a subset of prompt tokens.** I confirmed this independently in OpenAI's prompt-caching guide, which computes ordinary input tokens as `inputTokens - cachedTokens - cacheWriteTokens`.

### What didn't work

My first combined preflight command was refused by the worktree guard: "this command names git in a form too complex to verify that it stays inside the worktree". The command was `codex login status; mkdir -p ...; cd <worktree> && git status --short`. Splitting it into separate plain commands fixed it.

### What I learned

- **Codex's one finding was the billed-total claim.** The doc comment says `PromptTokens + CompletionTokens` is the billed total, but Gemini's `TotalTokenCount` also includes `ToolUsePromptTokenCount`. The builder's internal test row shows provider total 65 against our 15. The claim is only true because gai will never enable Gemini's built-in tools. That is a product decision, so it is now recorded instead of implied.
- **Cache hits are not guaranteed.** OpenAI's guide says "maintaining a session doesn't guarantee a cache hit", which is why a live cache-hit test would flake.

### What was tricky

Deciding between folding the tool-use count into `PromptTokens` and documenting a caveat. Both were reasonable. Markus's product call that built-in tools will never be supported made the question moot.

### What warrants review

The amended paragraph in the "Token usage normalisation (2026-10-06)" entry in `/docs/decisions.md`.

### Future work

None beyond the out-of-scope items already listed in step 1.
