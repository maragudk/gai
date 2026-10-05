# Diary: OpenAI client forwards `MaxCompletionTokens` (#291)

The OpenAI chat client in `/clients/openai/chat_complete.go` never applies `req.MaxCompletionTokens`. The value is not sent to the API, so the cap does nothing. The `ai.max_completion_tokens` span attribute is also not recorded. Google applies both only when the request sets the value. Anthropic always applies both, with a default of 16384. Issue #291 asks for OpenAI to match Google's conditional behaviour.

## Step 1: Scope the fix

**Author:** main

### Prompt Context

**Verbatim prompt:** Fix issue 291
**Interpretation:** Implement the fix described in GitHub issue #291 for the OpenAI client.
**Inferred intent:** Callers who set `MaxCompletionTokens` should get the same behaviour from the OpenAI client as from the other clients. Telemetry should show the cap.

### What I did
I read issue #291 and grepped for `MaxCompletionTokens` across the repo. The issue still applies on `main` at `63c3a6d`. The Google client (`/clients/google/chat_complete.go`, around line 131) is the reference: it sets the field and the span attribute only when `req.MaxCompletionTokens != nil`. Its live test is in `/clients/google/chat_complete_test.go`, around line 433. The catalogue row in `/docs/observability.md` (line 59) lists only anthropic and google for `ai.max_completion_tokens`. The research diary `/docs/diary/2026-09-16-openai-compat-only-research.md` had already flagged this gap.

I set these requirements for the builder:
- When `req.MaxCompletionTokens != nil`, set the Chat Completions `max_completion_tokens` field, not the deprecated `max_tokens`. Record `ai.max_completion_tokens` on the span. When the value is nil, send nothing and record nothing.
- Add a live test that mirrors Google's test: a capped request stays at or under the cap, and an uncapped request goes over it.
- Update the `/docs/observability.md` row so it names openai and describes the conditional behaviour.

### Why
The fix should match the existing conditional pattern in the Google client and not invent a new default. A default cap would change behaviour for current OpenAI callers who set nothing.

### What worked
The issue text is precise and the Google client gives a clear pattern to copy. No clarification from Markus was needed.

### What didn't work
Nothing failed at this stage.

### What I learned
On OpenAI reasoning models, `max_completion_tokens` also counts reasoning tokens. A very small cap in a live test can therefore produce empty visible output, so the test needs a non-reasoning model or a suitable assertion.

Markus pointed out why the Anthropic client is different. The Anthropic Messages API requires `max_tokens` on every request, so that client has to send a default (16384). OpenAI and Google make the cap optional, so the conditional pattern is the right fit for them. The difference between the clients comes from the providers' APIs. It is not a gap to "fix".

### What was tricky
I had to choose between `max_completion_tokens` and `max_tokens`. OpenAI deprecated `max_tokens`, and reasoning models reject it, so the fix uses `max_completion_tokens`. Some OpenAI-compatible servers may accept only `max_tokens`. That is out of scope for this fix.

### What warrants review
Check that a nil value sends nothing and records no attribute. Check that the test model is not a reasoning model, or that the test still works on one.

### Future work
None yet. The research diary also noted that the Anthropic client does not populate `res.Meta`. That is a separate gap and not part of #291.

## Step 2: Forward `MaxCompletionTokens` in the OpenAI client

**Author:** builder-291

### Prompt Context

**Verbatim prompt:** Fix GitHub issue #291 in the `maragu.dev/gai` Go library. [...] When `req.MaxCompletionTokens != nil`, set the Chat Completions `max_completion_tokens` field on the params [...] Add a live integration test [...] Update `docs/observability.md` line 59 [...]
**Interpretation:** Implement the conditional pattern from the Google client in the OpenAI client, add a live test, and update the observability catalogue row.
**Inferred intent:** OpenAI callers who set a cap get it sent to the API and recorded in telemetry, with no behaviour change for callers who set nothing.

### What I did
In `/clients/openai/chat_complete.go` I added a block next to the temperature and thinking-level handling. When `req.MaxCompletionTokens != nil` it sets `params.MaxCompletionTokens = openai.Int(int64(*req.MaxCompletionTokens))` and records `ai.max_completion_tokens` on the span. In `/clients/openai/chat_complete_test.go` I added the subtest "respects max completion tokens", which mirrors Google's: cap of 5, assert completion tokens are in `1..5`, then clear the cap and assert completion tokens exceed 5 and the output is longer. It also checks the span attribute is present with the cap and absent without it. In `/docs/observability.md` I updated the `ai.max_completion_tokens` row to list openai and say Google and OpenAI emit it only when the request sets one. `/docs/index.html`, `/README.md` and `/clients/openai/README.md` say nothing about max tokens, so I left them alone.

### Why
Matches Google's conditional behaviour and avoids a default cap that would change behaviour for existing OpenAI callers.

### What worked
The test uses the default `gpt-5.4-nano` with `gai.ThinkingLevelNone`. Every OpenAI model constant is a reasoning model, but `none` effort produces no reasoning tokens, so the tiny cap applies to visible output only and the test is reliable. I confirmed red/green: with the `params.MaxCompletionTokens` line removed the test fails with "should respect max completion tokens"; with it restored it passes. `go test -shuffle on -count=1 ./clients/openai/...` passes and `golangci-lint run` reports 0 issues.

### What didn't work
`go test -shuffle on ./...` fails in `/clients/google` and not in code I touched: `panic: project/location or API key must be set when using Vertex AI backend` from `google_test.newVertexAIClientWithCredentials` (client_test.go:58), in the "Vertex AI backend and service account" subtests. The local `.env.test.local` lacks the Vertex project and location configuration those tests need. All other packages pass.

### What I learned
`oteltest.NewSpanRecorder` swaps the global tracer provider, and the chat completer binds its tracer at construction. A second recorder created mid-test would therefore not see spans from an existing completer. The test uses one recorder and `oteltest.SpansByName` to inspect both calls.

### What was tricky
Choosing a non-reasoning model: none exists in the OpenAI constants, so the reliability comes from `ThinkingLevelNone` rather than the model choice.

### What warrants review
That nil sends nothing (the `if` guards both the param and the attribute), and that `ThinkingLevelNone` is accepted by `gpt-5.4-nano` (the existing thinking-level test table says so). The Google Vertex failure should be confirmed as environmental in CI.

### Future work
None. `max_tokens` fallback for OpenAI-compatible servers stays out of scope.
