# Diary: drop system-prompt text from spans and document observability

An observability checkup of `maragu.dev/gai` that turned into a shipped PR (#288): remove the
full system-prompt text from chat spans, publish a `docs/observability.md` attribute catalogue,
add a positive test for the surviving boolean, and consolidate the span tests into the per-unit
test files. Also a side quest to disable Claude Code's session-link attribution in commits and PRs.

## Step 1: Assess the observability story

**Author:** main

### Prompt Context

**Verbatim prompt:** "Use your observability skill and assess the use of observability in this project."
**Interpretation:** Load the observability skill, survey what `gai` actually emits, and grade it against modern practice.
**Inferred intent:** Markus wants a grounded verdict — strengths and concrete gaps — not a generic lecture.

### What I did

Loaded the `fabrik:observability` skill, then surveyed the codebase: OpenTelemetry usage, `slog`,
metrics, span attributes, error handling. Read `clients/anthropic/chat_complete.go`,
`robust/chat_completer.go`, `internal/oteltest/oteltest.go`, and the prior span-observability diary.
Mapped every emitted `ai.*` attribute, the span names, and which provider emits what. Produced a
ranked five-gap assessment: (1) `tools/` uninstrumented, (2) full `ai.system_prompt` text on spans
with no opt-out, (3) no `gen_ai.*` semconv alignment, (4) no published attribute schema, (5) no
metrics.

### Why

A real assessment has to come from what the code emits, not priors. Reading the actual emission
sites is the only way to catch things like the system-prompt asymmetry.

### What worked

Parallel greps over `clients/` and `robust/` produced a complete attribute map in one shot. The
existing `internal/oteltest` harness and the prior diary gave a strong picture of intent quickly.

### What didn't work

My first attribute inventory used a regex (`attribute\.[A-Za-z]+\(`) that silently skipped
`Int64`/`Float64` attributes — the digits in `Int64` broke `[A-Za-z]+`. Missed `ai.temperature`,
`ai.time_to_first_token_ms`, and the `*_delay_ms` family until I re-grepped for them explicitly.

### What I learned

`gai` is unusually well-instrumented for a library: trace-first, no exporter baked in, consistent
`SpanKindClient`, cross-provider token normalisation enforced by a test. The genuine gap is the
content asymmetry — the embedder records `ai.input_length` (a length) while the chat clients
recorded the full system prompt.

### What was tricky

Separating "defensible by design" from "actual defect." Metrics-absence is correct for a
unified-storage library; the system-prompt text is the one real defect.

### What warrants review

The five-gap framing in the assessment message — each gap's severity claim.

### Future work

Discuss the gaps one at a time and decide which to act on.

## Step 2: Triage the gaps one at a time

**Author:** main

### Prompt Context

**Verbatim prompt:** "Let's discuss the gaps one at a time, like in the address-code-review skill"
**Interpretation:** Treat the five gaps as review comments; walk through each, decide, no code until all triaged.
**Inferred intent:** A deliberate decision per gap, not a bulk "apply everything."

### What I did

Loaded `fabrik:address-code-review` and walked the five gaps. Decisions: (1) skip `tools/` — Markus
doesn't use it; (2) drop `ai.system_prompt`, keep `ai.has_system_prompt`; (3) defer `gen_ai.*`,
document the mapping; (4) ship `docs/observability.md`; (5) document the metrics stance, no code.

### Why

Triage-before-code keeps each decision honest and avoids shipping things the user won't use.

### What worked

Markus knocked out my cost/cardinality argument for the system prompt: in a columnar store a
system prompt is *low* cardinality (few distinct values, dictionary-encoded), so "kilobytes on
every span" was wrong. That left sensitivity as the sole concern — which flipped my recommendation
from "delete the attribute" toward "make it opt-in," and he then chose deletion anyway.

### What didn't work

My initial cost framing was simply incorrect for the storage model `gai` targets. Recorded because
it's a reasoning error worth not repeating.

### What I learned

For unified-storage observability, repeated large-but-stable string values are the cheap case, not
the expensive one. High cardinality (many *distinct* values) is the cost driver, not value size.

### What was tricky

Revising a recommendation mid-conversation after conceding the cost point, without overcorrecting.

### What warrants review

The decision table in the triage summary.

### Future work

Implement decisions 2–5.

## Step 3: Drop the attribute and write the docs

**Author:** main

### Prompt Context

**Verbatim prompt:** "Sounds good. Make sure to use your writing skill before writing it." (and "Launch a subagent to check whether the semconv has stabilized around this, it didn't when this was first instrumented.")
**Interpretation:** Remove `ai.system_prompt` from all three clients, write `docs/observability.md` after running the writing skill, and base the `gen_ai.*` deferral on fresh research.
**Inferred intent:** Ship correct, well-written docs grounded in the current state of the OTel GenAI spec.

### What I did

Removed `attribute.String("ai.system_prompt", *req.System)` from
`/clients/anthropic/chat_complete.go`, `/clients/openai/chat_complete.go`, and
`/clients/google/chat_complete.go`, keeping `ai.has_system_prompt`. Dispatched a research subagent
to check the GenAI semconv stability. Loaded `fabrik:writing-clearly-and-concisely`, verified the
full attribute inventory by reading every emission site (including `ai.input_length` = byte length
of the embed input), then wrote `/docs/observability.md`: telemetry model, span table, attribute
catalogue per group, invariants, content policy, and the `ai.* <-> gen_ai.*` mapping.

### Why

The doc absorbs three of the five decisions at once (gen_ai mapping, attribute schema, metrics
stance), so it's the highest-leverage artifact.

### What worked

The subagent's finding was decisive: the GenAI semconv is *less* stable than before — still
`Development`, and as of v1.42.0 (2026-06-12) the `gen_ai.*` attributes were deprecated in the main
repo and moved to a dedicated `semantic-conventions-genai` repo with zero tagged releases. That
turned "should we migrate?" into a clear "defer," matching Markus's boring-technology preference.

### What didn't work

Nothing broke. `go build`, `go vet`, gofmt, and the hermetic tests stayed green after the edits.

### What I learned

The cache-token conventions now exist (`gen_ai.usage.cache_read.input_tokens`,
`gen_ai.usage.cache_creation.input_tokens`, v1.40.0) and map cleanly onto `gai`'s
`ai.cache_*_tokens`, but they're `Development` too.

### What was tricky

Building an accurate catalogue. Provider coverage is uneven — `ai.cache_creation_tokens` is
Anthropic-only, `ai.thoughts_tokens` is OpenAI+Google, `ai.total_tokens`/`ai.finish_reason` are
OpenAI-only — so the doc had to read the emission sites, not assume symmetry.

### What warrants review

`/docs/observability.md` — verify the per-provider "Providers" column against the code, and the
`gen_ai.*` mapping rows.

### Future work

Open the PR; revisit `gen_ai.*` once the spec tags a release.

## Step 4: Open PR #288

**Author:** main

### Prompt Context

**Verbatim prompt:** "Make a PR"
**Interpretation:** Branch, commit, push, open a PR following the git skill's conventions.
**Inferred intent:** A clean PR, no issue references unless I ask.

### What I did

Loaded `fabrik:git`, asked about issues (answer: none), branched
`document-observability-and-drop-system-prompt-attribute`, committed the attribute removal and the
doc, pushed, and opened PR #288.

### Why

The git skill requires asking about issue references before committing.

### What worked

`index.html` turned out to be a hand-built landing page that links to no markdown docs (neither do
the existing `docs/design`, `decisions.md`, or `diary` files), so leaving `observability.md`
unlinked follows the established convention rather than shoehorning a styled link.

### What didn't work

Nothing at this step.

### What I learned

The repo's other markdown docs are all unlinked from the landing page — a deliberate-looking split
between the marketing page and contributor docs.

### What was tricky

Nothing.

### What warrants review

PR #288 scope and commit message.

### Future work

Verify CI; consider test coverage for the surviving boolean.

## Step 5: Add a positive test for `ai.has_system_prompt`

**Author:** main

### Prompt Context

**Verbatim prompt:** "No tests need updating? Note: don't test that a historic attribute doesn't exist." then "See internal/oteltest"
**Interpretation:** No existing test breaks, but the boolean the PR relies on is untested; add a positive assertion using the oteltest helper, never an absence check.
**Inferred intent:** Lock in the contract the PR creates without writing a brittle "attribute is gone" test.

### What I did

Confirmed no test referenced `ai.system_prompt`. Added a `System` prompt to each client's
chat-span test request and asserted
`oteltest.HasAttribute(attrs, attribute.Bool("ai.has_system_prompt", true))` in the anthropic,
openai, and google span tests. Committed and pushed.

### Why

The PR's behavioural claim — "emit the boolean instead of the text" — had zero test coverage, and
the span tests didn't even send a system prompt.

### What worked

`oteltest.HasAttribute` matches key *and* value, so the assertion is a positive value-check. The
attribute derives from the request, not the model reply, so it's deterministic even inside the
live-API span tests.

### What didn't work

Nothing — gofmt and `go vet` clean. (Couldn't run the live tests locally; no API keys.)

### What I learned

A removal PR is best guarded by asserting the *replacement's* presence, never the removed thing's
absence — an absence test passes forever against every attribute we never emit.

### What was tricky

Nothing; the helper was already built for exactly this.

### What warrants review

The three new assertions in the client `*_test.go` span subtests.

### Future work

Get CI green to actually exercise the new assertions.

## Step 6: The CI billing saga

**Author:** main

### Prompt Context

**Verbatim prompt:** "rerun CI, failed on API key billing issue" → "Quota should be restorted, monitor CI" → "try again"
**Interpretation:** CI is red on an external billing problem, not the code; rerun and monitor until green.
**Inferred intent:** Confirm the change is sound and get the PR to passing.

### What I did

Inspected the failed run logs, confirmed every failure was OpenAI `429 ... "type":
"insufficient_quota"`, and reran the failed jobs several times. The first reruns stayed red on the
same quota error. Eventually, after the quota was actually restored, a rerun went fully green —
Test (3m23s), Evaluate, Lint, govulncheck, Security all passing.

### Why

`insufficient_quota` is account-state, not a code defect, so the right move was to verify the
cause, not chase it in the diff.

### What worked

The failure isolation was clean: `clients/google` passed in full (live API) including the moved
span subtests, and Anthropic passed too — only OpenAI-backed tests failed, all on billing. That
both diagnosed the blocker and validated everything else against real APIs.

### What didn't work

Two reruns failed identically before the quota landed — `insufficient_quota` is hard exhaustion,
not transient rate-limiting, so it won't clear on a rerun until credits actually reach the key's
project.

### What I learned

OpenAI scopes quota per-project now; a top-up on the wrong project looks exactly like an
unresolved failure. Worth checking the key's project, not just "is there a balance."

### What was tricky

Resisting the urge to "fix" red CI in the code when the cause was entirely external. Reported the
state honestly and asked how to proceed rather than rerunning blindly forever.

### What warrants review

Nothing in the diff; the saga was infrastructure.

### Future work

None.

## Step 7: Consolidate the span tests into the per-unit files

**Author:** main

### Prompt Context

**Verbatim prompt:** "Move all spans_test.go content into the relevant chat_complete_test.go/client_test.go and make sure to use subtests"
**Interpretation:** Delete the standalone `spans_test.go` files; fold each span test into the existing test for the unit it covers, as `t.Run` subtests.
**Inferred intent:** One test file per unit; span checks live beside the behaviour they verify.

### What I did

Confirmed no target test uses `t.Parallel()` (so appending span subtests preserves serial,
deterministic ordering — important because `oteltest.NewSpanRecorder` swaps the global tracer
provider). Dispatched four parallel subagents, one per package (anthropic, openai, google, robust),
each moving the chat-span subtests into `TestChatCompleter_ChatComplete` and the embed-span
subtests into `TestEmbedder_Embed`, reconciling imports, deleting the `spans_test.go` file, and
verifying with gofmt + `go vet`. Then reviewed: restored a regression-context comment the anthropic
agent had dropped, and verified 12 `NewSpanRecorder` subtests survived (3 client chat + 2 client
embed + 4 robust chat + 3 robust embed). Committed and pushed.

### Why

Four packages, disjoint directories, identical mechanical transform — a clean parallel fan-out.

### What worked

All four agents succeeded; module-wide gofmt/vet clean and hermetic tests green. The robust agent
even ran `go test ./robust/...` (hermetic, no keys) and confirmed pass.

### What didn't work

The anthropic agent interpreted "move the `t.Run` block verbatim" strictly and left behind the
function-level comment explaining the subtest guards against the Anthropic `Usage`-wipe regression.
Caught it in review and re-added the comment above the moved subtest.

### What I learned

When fanning out a verbatim-move to subagents, call out attached comments explicitly — a
"move the block" instruction drops anything that sat outside the block.

### What was tricky

The global-tracer-provider caveat: span subtests must not run under `t.Parallel()`, or the
recorder swap races. Verified the targets were serial before fanning out.

### What warrants review

The moved subtests in `chat_complete_test.go` / `embed_test.go` / `chat_completer_test.go` /
`embedder_test.go`, and the restored regression comment in
`/clients/anthropic/chat_complete_test.go`.

### Future work

None — the consolidation is complete and CI-green.

## Step 8: Disable the session-link attribution (config side quest)

**Author:** main

### Prompt Context

**Verbatim prompt:** "Why do you add that link in the PR description?" → "Figure out how I disable that in your configuration" → "This should be global, not project-local" → "Just from the PR"
**Interpretation:** The `claude.ai/code/session_...` link is a harness instruction; find the setting that disables it globally, apply it, and strip the link from PR #288.
**Inferred intent:** Stop the session link appearing in future commits/PRs, user-wide.

### What I did

Owned that the link came from a harness instruction I'd followed without flagging. Inspected
`~/.claude/settings.json` (found the deprecated `includeCoAuthoredBy: false`) and dispatched a
claude-code-guide agent. The control is the newer `attribution` setting: `{"commit": "", "pr": ""}`
removes the `Claude-Session` trailer and the PR-body link. Added it to the global
`~/.claude/settings.json`, kept `includeCoAuthoredBy: false` as a fallback, validated the JSON, and
removed the trailing link from PR #288's body (left the commit trailers as-is per Markus's call).

### Why

The setting belongs in the user-wide file so it applies across all projects.

### What worked

The claude-code-guide agent cleanly separated the two concerns: `includeCoAuthoredBy` (co-author
byline only, deprecated) versus `attribution` (governs the session links too).

### What didn't work

I can't fully verify the installed Claude Code honours `attribution` without a fresh commit, and
the change won't affect the current session (the link instruction is baked into this session's
prompt). New sessions will pick it up.

### What I learned

`includeCoAuthoredBy` is deprecated in favour of `attribution`, which controls commit *and* PR
attribution text independently — empty strings strip everything including the session links.

### What was tricky

Being honest about two earlier overreaches in the same session — asserting "your usual is squash"
(the repo actually has squash *disabled*; only merge commits are allowed) and adding the session
link without flagging it. Both were assumptions dressed as facts.

### What warrants review

`~/.claude/settings.json` — the new `attribution` block; confirm the next session's commits/PRs
omit the link.

### Future work

Verify on the next session's first commit; if the link persists, the key name or version differs.
Optionally decide whether squash-disabled is intentional.
