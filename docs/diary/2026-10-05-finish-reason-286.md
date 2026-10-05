# Diary: Set finish reason in the Google and Anthropic clients (#286)

Issue #286 asks for the response finish reason to be visible on the Google and Anthropic chat-completion spans, so a `MAX_TOKENS` truncation (for example, mid-JSON with a `ResponseSchema`) is diagnosable at the source instead of surfacing as a generic `json.Unmarshal` error further up. The OpenAI client already sets both `gai.ChatCompleteMetadata.FinishReason` and the `ai.finish_reason` span attribute; Google and Anthropic set neither. The provider-to-enum mapping was recorded in `/docs/decisions.md` ("Finish reason normalisation", 2025-02-14), so the work is wiring, not design.

## Step 1: Scope the requirements and hand off to a builder

**Author:** main

### Prompt Context

**Verbatim prompt:** Fix issue 286
**Interpretation:** Implement what the (narrowed) issue title says: populate `meta.FinishReason` and the `ai.finish_reason` span attribute in `clients/google` and `clients/anthropic`, the same way `clients/openai` does.
**Inferred intent:** Make token-cap truncation and other abnormal stops observable and programmatically detectable on every backend, with one portable attribute and enum.

### What I did

Read the issue and its comment, the finish-reason entry in `/docs/decisions.md`, and the OpenAI implementation in `/clients/openai/chat_complete.go` (`mapChatFinishReason`, plus the stream-loop and post-stream fallback that set `meta.FinishReason` and `ai.finish_reason`). Wrote requirements and spawned one builder in this worktree.

Requirements given to the builder:

- Both clients set `meta.FinishReason` (normalised `gai.ChatCompleteFinishReason`) and the `ai.finish_reason` span attribute with the same string value, mirroring OpenAI.
- Mapping follows `/docs/decisions.md`, checked against the constants the vendored SDK versions actually expose. Anthropic: `end_turn`/`stop_sequence` to `stop`, `max_tokens` (and a context-window-exceeded value if the SDK has one) to `length`, `tool_use` to `tool_calls`, `refusal` to `refusal`, anything else to `unknown`. Google: `STOP` to `stop`, `MAX_TOKENS` to `length`, safety/recitation/blocklist/prohibited-content/SPII/image-safety style values to `content_filter`, everything else to `unknown`. Gemini `STOP` stays `stop` even when the response has function calls, per the recorded decision.
- No raw-provider-value attribute, since OpenAI does not emit one either; keeping the three clients symmetric beats partial adherence to the decision's "capture raw value" note.
- Tests: unit tests for each mapping function, plus assertions in existing live tests where a finish reason is naturally observable.

### Why

The mapping already exists as a decision, and the OpenAI client is the reference implementation, so the cheapest correct change is to replicate its shape in the other two clients.

### What worked

The issue had already been narrowed in a comment, so there was no ambiguity about OpenAI being in scope.

### What didn't work

Nothing failed at this stage; no code was run.

### What I learned

The decisions entry describes Anthropic refusals as a `tool_use` block named `refusal`. The current Messages API has a dedicated `stop_reason: "refusal"`, so that part of the entry is stale.

### What was tricky

Deciding whether Gemini `STOP` with function-call parts should become `tool_calls` for parity with OpenAI. The recorded decision says no, and changing it would be a product decision outside this issue.

### What warrants review

The mapping tables in both clients against the SDK constants, and that the attribute is set exactly once per response with the final value (streaming may report the reason only on the last chunk).

### Future work

Refresh the Anthropic part of the finish-reason decision entry if the builder confirms the SDK's `refusal` stop reason.

## Step 2: Wire finish reasons into the Google and Anthropic clients

**Author:** finish-reason-builder

### Prompt Context

**Verbatim prompt:** "You are building GitHub issue #286 in the `maragu.dev/gai` Go library. [...] Set the response finish reason in the Google (`clients/google`) and Anthropic (`clients/anthropic`) chat completers, matching what `clients/openai/chat_complete.go` already does." (The lead's full brief also gave the mapping tables, the no-raw-attribute rule, the test expectations and the out-of-scope list summarised in Step 1.)
**Interpretation:** Add a package-private `mapChatFinishReason` to each client, set `meta.FinishReason` from the provider's stream, and emit `ai.finish_reason` with the same value. Use SDK constants, and check them against the vendored SDK versions.
**Inferred intent:** A token-cap truncation, or any other abnormal stop, should be visible on the span and on `res.Meta` for every backend, not only OpenAI.

### What I did

I checked the vendored SDKs with `go doc -all`. `anthropic-sdk-go` v1.78.0 has `StopReasonEndTurn`, `StopReasonMaxTokens`, `StopReasonStopSequence`, `StopReasonToolUse`, `StopReasonPauseTurn`, `StopReasonRefusal` and `StopReasonModelContextWindowExceeded`. The `compaction` value exists only on `BetaStopReason`, so it does not apply here. `genai` v1.71.0 has the expected values plus some image and tool-call values that the brief did not list: `IMAGE_PROHIBITED_CONTENT`, `IMAGE_RECITATION`, `UNEXPECTED_TOOL_CALL`, `TOO_MANY_TOOL_CALLS`, `NO_IMAGE` and `IMAGE_OTHER`.

I wrote the table-driven unit tests first, in `/clients/anthropic/chat_complete_internal_test.go` and `/clients/google/chat_complete_internal_test.go`. They failed to compile (`undefined: mapChatFinishReason`). Then I added the mapping functions:

- Anthropic: `end_turn` and `stop_sequence` map to `stop`. `max_tokens` and `model_context_window_exceeded` map to `length`. `tool_use` maps to `tool_calls` and `refusal` maps to `refusal`. Everything else, including `pause_turn`, maps to `unknown`.
- Google: `STOP` maps to `stop` and `MAX_TOKENS` maps to `length`. These map to `content_filter`: `SAFETY`, `RECITATION`, `BLOCKLIST`, `PROHIBITED_CONTENT`, `SPII`, `IMAGE_SAFETY`, `IMAGE_PROHIBITED_CONTENT` and `IMAGE_RECITATION`. Everything else maps to `unknown`.

In `/clients/google/chat_complete.go`, the stream loop now reads `chunk.Candidates[0].FinishReason`. It does so before the `Content == nil` guard, because a policy stop can arrive on a chunk with no content. The loop updates `meta.FinishReason`, and the existing deferred usage block emits `ai.finish_reason` once at the end.

In `/clients/anthropic/chat_complete.go`, the client never set `res.Meta` before this change. I added a `meta` and set `res.Meta = meta`. Leaving `Usage` at zero on a now non-nil `Meta` would be misleading, so I also populate `Usage` from the accumulated `message.Usage`, with the same prompt-token normalisation the span already used.

Live tests: Google's tool-call test now asserts `stop` on the function-call turn, which pins the recorded decision, and `stop` on the follow-up turn. The max-tokens test asserts `length` when capped and `stop` when not. The span test asserts `ai.finish_reason=stop`. Anthropic's tool-call test asserts `tool_calls`, then `stop`. A new cheap subtest uses `MaxCompletionTokens: 5` on Haiku and asserts `length` and non-zero usage. The span test asserts `ai.finish_reason=stop`.

### Why

This mirrors the OpenAI client's shape (a mapping function, then meta and attribute set from the stream), so the three clients behave the same way. No raw-value attribute was added, because the brief kept the clients symmetric.

### What worked

The red/green loop on the mapping functions was quick. `go test -shuffle on ./clients/anthropic/...` passed in full. Google's `TestChatCompleter` passed, and the max-token test returned `MAX_TOKENS` as expected.

### What didn't work

`go test -shuffle on ./clients/...` failed in Google on every test that calls `newVertexAIClientWithCredentials`: "panic: project/location or API key must be set when using Vertex AI backend". The worktree's `/.env.test.local` has `OPENAI_KEY`, `ANTHROPIC_KEY`, `GOOGLE_KEY` and `GOOGLE_VERTEX_KEY`, but no `GOOGLE_VERTEX_CREDENTIALS_PATH`. This is environment-only and unrelated to the change, so I did not look for the credentials outside the worktree. My first `-skip 'service_account'` did not skip anything, because `-skip` splits on `/` and matches per level. `-skip '/(service_account|Embedding_2_on_Vertex_AI)'` worked. That run then hit one Vertex embed 429: "Error 429, Message: Resource exhausted". This is a rate-limit flake in `TestEmbedder_Embed`.

The sandbox refused compound shell commands with `go list -m -f '{{.Dir}}'` and heredoc file writes, so I used `go doc` and the Write tool instead.

### What I learned

Anthropic's accumulator (`anthropic.Message.Accumulate`) already copies `stop_reason` and the cumulative usage, including `output_tokens_details.thinking_tokens`, from `message_delta` into the message. A separate event switch for those values is not necessary.

### What was tricky

With Gemini, the finish reason and the last content can arrive on the same chunk. A safety stop can arrive with nil content. The old combined `len(...) == 0 || Content == nil` guard would have skipped the safety case, so I split it.

### What warrants review

The decision to populate Anthropic `Usage` goes beyond the brief. Review it, in particular the `ThoughtsTokens` semantics described in Step 3. Also check the Google test assertion that a function-call turn reports `stop`. It intentionally locks in the recorded decision.

### Future work

See Step 3.

## Step 3: Self-review and follow-up fixes

**Author:** finish-reason-builder

### Prompt Context

**Verbatim prompt:** "Self-review when done." (from the lead's brief)
**Interpretation:** Run the code-review skill over the diff, fix the real issues, and report the rest.
**Inferred intent:** Hand back a change that is ready for review and that states its limits honestly.

### What I did

I ran `/code-review high`. It returned seven findings, and I acted on three of them:

- I removed the dedicated `anthropic.MessageDeltaEvent` case. After every event, `meta.Usage` and `meta.FinishReason` are now refreshed from the accumulated `message`, so `Meta` updates continuously, as the `gai.ChatCompleteResponse.Meta` doc says. The deferred block now only emits span attributes, and it reads prompt and completion tokens from `meta.Usage`.
- `ThoughtsTokens` now comes from `message.Usage.OutputTokensDetails.ThinkingTokens`. My first comment, "Anthropic bundles thinking tokens into OutputTokens", was wrong for SDK v1.78. This matches OpenAI's `ReasoningTokens` shape, where thinking tokens are a subset of completion tokens.
- The stale comment in the adaptive-thinking matrix test is now an assertion: when `requireThoughts` is set, `ThoughtsTokens > 0` and `ThoughtsTokens <= CompletionTokens`. The `sonnet 4.6 + medium` row passed with `thoughtsTokens=33`.

I deliberately did not act on four findings. They are listed under Future work.

Final checks: `go test -shuffle on ./clients/anthropic/...` passed (45s). `go test -shuffle on ./clients/google/ -run 'TestChatCompleter|TestMapChatFinishReason' -skip '/service_account'` passed. `golangci-lint run` reported 0 issues.

### Why

The accumulator is the single source of truth for end-of-stream state, so reading from it removes duplicated tracking. Continuous updates also stop `Meta` from showing zeros that look authoritative mid-stream.

### What worked

The reviewer caught the incorrect thinking-tokens comment, and the SDK source confirmed that `Accumulate` copies `OutputTokensDetails`.

### What didn't work

Nothing failed in this step.

### What I learned

Usage semantics already differ across clients. Google's `ThoughtsTokenCount` is separate from `CandidatesTokenCount`, while OpenAI's and now Anthropic's thinking tokens are a subset of completion tokens. `gai.ChatCompleteResponseUsage` does not document which one is correct.

### What was tricky

I had to decide how far to take the `Usage` work. It was not in the brief, but a non-nil `Meta` with zero usage would have been actively wrong.

### What warrants review

Review the per-event refresh in `/clients/anthropic/chat_complete.go`, and whether populating Anthropic `Usage` belongs in this change at all.

### Future work

- Gemini reports `STOP` when the response contains function calls, so a provider-agnostic loop that branches on `tool_calls` will not work on Google. This is kept per the recorded decision, but it is a real portability gap.
- When Gemini blocks a prompt (`PromptFeedback.BlockReason`, with no candidates), `FinishReason` stays nil. Arguably it should be `content_filter`.
- The OpenAI client yields an error on refusal. Anthropic now reports `refusal` as the finish reason but still ends the stream cleanly.
- Anthropic's `stop_reason` arrives on `message_delta`, after the tool-call part is yielded. A consumer that breaks out of `Parts()` early sees a nil `FinishReason`. OpenAI has the same limitation.
- `ChatCompleteResponseUsage.ThoughtsTokens` semantics differ between clients (subset or separate), and the type does not document which is intended.
- Anthropic does not emit an `ai.thoughts_tokens` span attribute, although Google and OpenAI do.
- The `/docs/decisions.md` finish-reason entry still describes Anthropic refusals as a `tool_use` block. The SDK confirms a dedicated `refusal` stop reason, plus `model_context_window_exceeded` and `pause_turn`.
