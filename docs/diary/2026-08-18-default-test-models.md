# Diary: Switch the default test models in the three clients

Markus wants the variadic `newChatCompleter(t)` test-helper defaults moved to the current best performance/price models for tests: GPT-5.6 Luna (openai, currently GPT-5 Nano), the most recent Gemini Flash Lite, i.e. 3.5 Flash Lite (google, currently Gemini 2.5 Flash), and Claude Sonnet 5 (anthropic, currently Haiku 4.5). The google switch is the risky one: the 2.5 Flash default was a deliberate hold because Gemini 3.x enforces a `thought_signature` round-trip on multi-turn tool follow-ups that `gai.Part` does not preserve (issue #256, still open; #250 is the Anthropic equivalent, also open, but only bites with thinking enabled, which the default helper does not do). Markus chose to probe first with real runs before deciding whether #256 blocks the google switch or gets fixed.

## Step 1: Scope the switch and the #256 risk

**Author:** main

### Prompt Context

**Verbatim prompt:** "Now check that default OpenAI tests use GPT-5.6 Luna, and Google one's use Gemini most recent Flash Lite, Anthropic ones Sonnet 5. Those are currently the best for performance/price ratio for tests IMO"
**Interpretation:** Verify and update the default models used by each client's test helper to the three named models.
**Inferred intent:** Keep the live test suite on the cheapest models that still exercise current-generation behavior, now that the conformance work (PR #334) refreshed the constant lists.

### What I did

Confirmed the current defaults in the three `newChatCompleter` helpers (`/clients/openai/chat_complete_test.go` GPT-5 Nano, `/clients/google/chat_complete_test.go` Gemini 2.5 Flash, `/clients/anthropic/chat_complete_test.go` Haiku 4.5 Latest), checked issues #256 and #250 (both OPEN), and asked Markus how to handle a potential #256 trip on the google switch — answer: probe first with all three switched, then decide with real data. Created worktree `default-test-models` and started this diary; implementation and probing delegated to a builder next.

### Why

The decisions log explicitly records the google default as held back for #256; switching it blind would convert a known deferral into a surprise CI failure.

### What worked

The decisions log paid for itself: the #256/#250 deferrals were recorded with enough context that the risk assessment took one grep and two issue lookups.

### What didn't work

Nothing failed; this step was pure scoping. Known environmental caveat for the probe: the local `OPENAI_KEY` may still lack the vision scope (the "describe an image" live test 401s with `missing_scope` locally; CI's key is fine).

### What I learned

The #250 Anthropic deferral does not block the Sonnet 5 default switch: Anthropic thinking is opt-in and the default helper never enables it, whereas Gemini 3.x enforces the signature round-trip unconditionally on tool follow-ups.

### What was tricky

Nothing yet; the probe will tell whether the google switch is a three-character diff or a plumbing project.

### What warrants review

After the builder's probe: the exact failure set for google on 3.5 Flash Lite (which tests, verbatim errors, single- vs multi-turn), and that openai/anthropic switches ran the full live suites green apart from the known local key-scope 401.

### Future work

Depending on probe results: either fix #256 in this branch or hold the google default with a comment, per Markus's call.

## Step 2: Probe all three switches live, and hold all three defaults

**Author:** defaults-builder

### Prompt Context

**Verbatim prompt:** "1. Switch the defaults: `clients/openai/chat_complete_test.go`: `ChatCompleteModelGPT5Nano` → `ChatCompleteModelGPT5_6Luna`; `clients/anthropic/chat_complete_test.go`: `ChatCompleteModelClaudeHaiku4_5Latest` → `ChatCompleteModelClaudeSonnet5Latest`; `clients/google/chat_complete_test.go`: `ChatCompleteModelGemini2_5Flash` → `ChatCompleteModelGemini3_5FlashLite`. […] 2. Probe: run the FULL live suite per client […] each at least twice to separate flake from real failure. […] For every real failure, capture the verbatim error and classify: (a) #256 thought_signature enforcement, (b) model-behavior difference […], (c) flake, (d) other. 3. Disposition by result: openai and anthropic: if green (modulo the environmental 401), keep the switch. google: if the 3.5 Flash Lite default fails on #256 or anything else real, REVERT the google switch […] If a failure is class (b) — a test encoding old-default behavior — fix the test only if the fix is obviously mechanical […]; anything non-obvious goes in the report as a question instead."
**Interpretation:** Flip the three defaults, run every client's live suite twice, classify each failure against a live baseline, and keep only the switches that come back green.
**Inferred intent:** Find out empirically whether the current best price/performance models can carry the integration suite, without gambling CI on the answer.

### What I did

Baselined first: with the old defaults, `go test -count=1 -shuffle on ./clients/anthropic` was green (66s), `./clients/openai` failed only the known local vision 401, and `./clients/google` was green once the four subtests needing a Vertex service-account JSON were skipped. Then flipped all three defaults (helper fallback, the `ai.model` span assertion in each "records standard attributes" subtest, and the helper doc comments) and ran each client's suite twice. Every switch failed, each for its own reason, and every failure reproduced on both runs, so I reverted all three defaults and left each helper documenting which model it is *not* using and why. The change is three doc comments — in `/clients/openai/chat_complete_test.go`, `/clients/anthropic/chat_complete_test.go`, and `/clients/google/chat_complete_test.go` — plus this diary entry.

Two extra experiments (edited into the test file, run, then restored from a backup copy) pinned down what each blocker actually needs:

- Anthropic without `Temperature`: `claude-sonnet-5` then reached the model but failed `can use a tool` and `can use a system prompt` with `unexpected message parts` (the `t.Fatal` default arms at `chat_complete_test.go:125` and `:282`) — it streams thinking blocks with no thinking level requested, and those subtests accept only text (and, in the tool case, tool-call) parts.
- OpenAI with `ThinkingLevel: gai.Ptr(gai.ThinkingLevelNone)` added to the two tool subtests: both passed on `gpt-5.6-luna`, while the untouched `tool choice` subtests kept 400ing — so `reasoning_effort=none` is exactly the missing ingredient.

With the defaults back in place all three suites are green again — openai 137s, anthropic 60s, google 105s (skipping the service-account subtests) — and `golangci-lint run ./clients/...` reports 0 issues.

### Why

Markus wanted the probe before the decision, and the disposition rule was "keep only what is green". Nothing was green, so the honest outcome is three unchanged defaults plus written-down reasons — otherwise the next agent re-runs this same probe and re-discovers the same three 400s.

### What worked

Baselining the old defaults first paid for itself. It proved the local environment was healthy for google and anthropic before anything was switched, and it caught one failure that would otherwise have looked like fallout: the first google baseline run failed `thinking_level_matrix/flash-lite_3.1_+_high` on `should stream PartTypeThought parts`, on the old default, and no later run of the same row failed. That row pins its model explicitly, so no change to the default helper can touch it — it is simply a flaky strict assertion, and the comment above it (`clients/google/chat_complete_test.go:546-549`) currently claims the thought part is reliable at High.

`go test -skip '/(service_account|Embedding_2_on_Vertex)'` was the tool that made the google suite usable at all here. The leading slash matters: `-skip` splits on `/` like `-run`, so a bare `-skip service_account` only filters top-level test names and silently changes nothing.

### What didn't work

All three switches failed, with the same primary errors on both runs:

- **openai / `gpt-5.6-luna`** — class (d), an API-surface limit. `can use a tool with args`, `can use a tool with no args`, and both `tool choice` subtests fail with `400 Bad Request {"message": "Function tools with reasoning_effort are not supported for gpt-5.6-luna in /v1/chat/completions. To use function tools, use /v1/responses or set reasoning_effort to 'none'.", "type": "invalid_request_error", "param": "reasoning_effort"}`. The tests never set a thinking level, so the effort is the model's own server-side default; only an explicit `none` clears it.
- **anthropic / `claude-sonnet-5`** — class (b). Seven subtests fail with ``400 Bad Request {"type":"error","error":{"type":"invalid_request_error","message":"`temperature` is deprecated for this model."}}``, i.e. every subtest that pins `Temperature: gai.Ptr(gai.Temperature(0))`. Drop the field and two more fail on unprompted thinking blocks (see above).
- **google / `gemini-3.5-flash-lite`** — class (a), #256 exactly as predicted: `can use a tool` fails on the follow-up turn with ``Error 400, Message: Function call is missing a thought_signature in functionCall parts. This is required for tools to work correctly, and missing thought_signature may lead to degraded model performance. Additional data, function call `default_api:read_file` , position 2., Status: INVALID_ARGUMENT`` Plus class (b): `tracks token usage` fails `should have thoughts tokens` because Flash Lite spends zero thought tokens on `Hi!` (`chat_complete_test.go:426: 3 10 0`), where 2.5 Flash always thinks a little. One run also failed `can describe audio` on wording — the model answered "I hear a person whispering in the audio.", which misses every keyword in `voice|speech|says|hello` — a class (c)/(b) borderline that only showed up on one of the two runs.

Two environmental failures, both pre-existing and both present in the baseline: the local `OPENAI_KEY` intermittently 401s the vision test with `missing_scope` (it failed in the nano baseline and passed in both Luna runs), and the four google subtests that build a Vertex client from `GOOGLE_VERTEX_CREDENTIALS_PATH` panic outright — `panic: project/location or API key must be set when using Vertex AI backend` — because the worktree has no service-account JSON. That panic kills the whole test binary, so those subtests must be skipped for any local google run to be meaningful.

### What I learned

The three failures share a shape worth naming: each new model tightens what the *request* may contain, not what the response looks like. Luna forbids tools alongside a non-`none` reasoning effort, Sonnet 5 forbids `temperature`, Gemini 3.x demands a `thought_signature` it previously tolerated missing. A "cheapest model" default was a cost decision; a "newest model" default is an API-compatibility decision, and gai's own request struct is what gets squeezed.

Step 1 filed #250 as harmless here because "Anthropic thinking is opt-in and the default helper never enables it". The probe disproves the premise, not just the conclusion: `claude-sonnet-5` thinks with no thinking fields in the request at all, and `can use a tool` appends every streamed part into the model message it sends back on the follow-up turn, where `anthropic.ChatCompleter` refuses inbound thought parts (`clients/anthropic/chat_complete.go:127-134`). So loosening the part assertion only trades the `t.Fatal` for `errThoughtRoundTripUnsupported` — Sonnet 5 is blocked by #250 exactly as `gemini-3.5-flash-lite` is blocked by #256. It also makes the public GoDoc on `anthropic.ThinkingLevel` ("Pass [gai.ThinkingLevelNone] to opt out of thinking entirely") an overstatement on the Claude 5 line: sending no thinking fields is not the same as getting no thinking.

The openai workaround is not model-neutral, which is what makes it a question rather than a fix: `gai.ThinkingLevelNone` is what unblocks tools on `gpt-5.6-luna`, but the matrix rows record that `gpt-5` and `gpt-5-nano` *reject* `none` outright. Pinning `none` in the tool subtests would tie them to the newer models.

### What was tricky

Distinguishing "the switch broke this" from "this was already broken" needed the baseline, and the google baseline took three attempts to get clean. `-skip service_account` skipped nothing at all (no leading slash, so it only filtered top-level test names), and `-skip '/service_account'` still missed `TestEmbedder_Embed/can_embed_a_text_with_Embedding_2_on_Vertex_AI_backend`, which also builds the service-account client and panicked the binary three seconds in.

Also: the anthropic "unexpected message parts" failures name a `t.Fatal` in a part-type switch, not a part type. Working out that the intruder was `gai.PartTypeThought` meant reading the switch (only `PartTypeText` is accepted in `can use a system prompt`, and that request carries no tools) rather than trusting the message.

### What warrants review

The three helper doc comments are the whole diff — check that each quoted provider error matches what the API actually returns, and that documenting the rejected model belongs there at all rather than only in this diary. Both comments naming a blocked model will age: the google one if #256 lands, the anthropic one if #250 does.

Worth a second opinion: whether the anthropic `temperature` rejection deserves a library-level response rather than a test note. `anthropic.ChatCompleter` sends `temperature` whenever `gai.ChatCompleteRequest.Temperature` is set, so any caller pairing that field with a Claude 5 model gets a 400 out of gai today. Same question for the `ThinkingLevelNone` GoDoc noted above.

### Future work

Three open questions for Markus, all in the report: pin `reasoning_effort=none` in the openai tool and tool-choice subtests to unlock Luna (at the cost of tying them to models that accept `none`); fix #250 to unlock Sonnet 5, since dropping `Temperature` alone only moves the failure one turn later; and whether #256 now gets fixed rather than deferred, since it is the larger of the two google blockers (a google switch would also need `tracks token usage` to stop demanding thought tokens). No decision record was written — the existing google hold in `/docs/decisions.md` still describes the code as it stands, and the rest is Markus's call. Worth deciding at the same time: all three holds now have a reason, but only google's is in `/docs/decisions.md`; either all three belong there with the helpers pointing at it, or none do.

Separately, the newest models on two clients are exported and conformance-checked but unexercised by the thinking matrices: the gpt-5.6 family (`Luna`, `Sol`, `Terra`) has no `reasoning effort matrix` rows and no bullet in the `ThinkingLevel` doc list in `/clients/openai/chat_complete.go`, and the Claude 5 family (`Sonnet 5`, `Opus 5`, `Fable 5`, plus `Opus 4.8`) has no `adaptive thinking matrix` rows and no mention in the equivalent doc block in `/clients/anthropic/chat_complete.go`. This probe established one fact about each — Luna needs `reasoning_effort=none` for function tools, Sonnet 5 thinks unprompted and rejects `temperature` — that belongs in those matrices whichever way the default question is decided.
