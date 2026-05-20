# Diary: Add `ToolChoice` to `ChatCompleteRequest` for forced / required tool calls

Surface a cross-provider way to constrain tool-calling behavior on `gai.ChatCompleteRequest`.
Today callers only get the providers' default ("auto") — there's no way to force the model to
call *some* tool, or to force it to call a *specific* tool. Forced tool calls are the backbone of
agent loops and structured-output termination; without them, Gemini in particular tends to free-text
a fenced ```json block instead of calling the tool, silently breaking the contract.

Tracked in GitHub issue #269 (reported by @nzoschke, who also has a reference branch `force-tool`).

## Step 1: Shape requirements and scope

**Author:** main (fabrik:lead)

### Prompt Context

**Verbatim prompt:** Have a look at issue 269. What do you think?
**Interpretation:** Read issue #269, evaluate whether the proposed `ToolChoice` API belongs in gai,
and form an opinion on the design before any building starts.
**Inferred intent:** Decide whether to accept this contribution and, if so, set clear requirements
and scope boundaries before handing it to the team.

### What I did
Read issue #269 (proposed `*ToolChoice` field with a three-mode enum: auto / any / tool), both
comments, and the existing optional-field wiring for `ResponseSchema` and `ThinkingLevel` across the
three clients (`clients/anthropic`, `clients/google`, `clients/openai`). Assessed the proposal
against gai's stated philosophy of *not* being a lowest-common-denominator wrapper. Confirmed two
design decisions with the product owner via questions:

- **Fresh implementation** from the issue's API proposal — the contributor's `force-tool` branch is a
  reference for SDK wiring details only, not the basis.
- **Validate in core (`gai`)** so all three providers reject bad input identically, rather than
  letting each provider's SDK error differently.

### Why
`ToolChoice` clears the "is this worth standardizing?" bar because it's a *true intersection* of all
three providers (auto / any / named are exactly the modes each SDK supports), not a compromise subset.
It's also load-bearing for agent and structured-output workflows, which is what gai is for.

### What worked
The existing `*ResponseSchema` / `*ThinkingLevel` pointer-field pattern gives a clear template for an
optional `*ToolChoice` field where nil preserves current behavior.

### What didn't work
Nothing yet — no code written in this step.

### What I learned
gai deliberately splits abstractions: `ThinkingLevel` constants live *per-client* because providers
disagree on vocabulary, while a true intersection like tool choice belongs in *core*. The deciding
question is whether the providers genuinely share the concept or whether core would be papering over a
difference.

### What was tricky
Drawing the gai/not-gai line. The product owner had explicitly flagged the risk of gai becoming a
lowest-common-denominator wrapper. The resolution: this is an intersection (all three support exactly
these modes), not an LCD subset, so it belongs in core.

### What warrants review
Nothing to review in this step — requirements only.

### Future work
Implementation handed to a builder (see subsequent steps): add the core types + validation, wire
through all three clients, add tests, update client READMEs and docs.

## Step 2: Implement core types, validation, and wire all three clients

**Author:** builder (fabrik:go)

### Prompt Context

**Verbatim prompt:** (team-lead handoff) Implement issue #269: add a `ToolChoice` field to
`ChatCompleteRequest` for forced / required tool calls, validate in core, wire OpenAI / Anthropic /
Google, add tests, update docs.
**Interpretation:** Build the feature fresh from the API proposal in `chat_complete.go`, add a core
validation helper each client calls, translate the three modes per provider SDK, and cover with both
fast validation tests and live per-client translation tests.
**Inferred intent:** A clean, consistently-validated cross-provider tool-choice surface that ships in
a building, tested, lint-clean state for the lead to review.

### What I did
- Core (`chat_complete.go`): added `ToolChoice` (pointer field on `ChatCompleteRequest`),
  `ToolChoiceMode` with `auto`/`any`/`tool` constants, and a `(*ToolChoice).Validate(tools []Tool)
  error` method. Nil receiver validates trivially so clients can call it unconditionally.
- Validation rules: `tool` mode requires a `Name` matching one of `req.Tools`; `auto`/`any` reject a
  non-empty `Name`; unknown mode is rejected. All as returned errors (caller data), not panics —
  contrasting with the `ThinkingLevel` constant-panic convention, as the lead specified.
- Wired each client to call `req.ToolChoice.Validate(req.Tools)` at the top of `ChatComplete` (after
  the existing empty-message panics), returning the error after `span.RecordError` + `span.End()`.
- Translations: OpenAI `ChatCompletionToolChoiceOptionUnionParam` (`auto` string / `required` string
  / `OfFunctionToolChoice` named); Anthropic `ToolChoiceUnionParam` (`OfAuto`/`OfAny`/
  `ToolChoiceParamOfTool`); Google `ToolConfig.FunctionCallingConfig` (`AUTO`/`ANY`, and `ANY` +
  `AllowedFunctionNames:[name]` for `tool` mode). Each sets an `ai.tool_choice` span attribute.
- Tests: 9 core validation subtests; per-client `TestChatCompleter_ChatCompleteToolChoice` with three
  subtests each (any forces a call, tool forces the named tool, invalid choice is rejected pre-API).
- Docs: added a `Tool choice` roadmap item under `Tool use` in all three client READMEs.
  `docs/index.html` is a marketing landing page (feature cards, no request-field reference) and does
  not document `ResponseSchema`/`ThinkingLevel` either, so `ToolChoice` does not belong there — left
  unchanged deliberately.

### Why
Validation in core (not per-SDK) gives identical rejection across providers, matching the lead's
decision. The `tool` -> Gemini `ANY` + `AllowedFunctionNames` mapping is the only non-obvious
translation: genai has no dedicated single-tool mode, so a one-element allow-list is the documented
way to force a specific function.

### What worked
The existing `*ResponseSchema` / `*ThinkingLevel` optional-field pattern transferred directly. All
ToolChoice tests pass against live OpenAI, Anthropic, and Google APIs.

### What didn't work
`go doc` on the SDKs failed until I ran `go mod download` for `openai-go/v3` and `genai` — the module
cache wasn't populated in the fresh worktree. Resolved by downloading, then reading the vendored
type definitions directly.

### What was tricky
The full `make test` run shows one FAILing google subtest: `can chat-complete via Vertex AI with
service account` panics in `NewClient` because Vertex service-account credentials aren't configured
in this environment. Verified this fails identically on the clean base branch (git stash), so it is
**pre-existing and unrelated** to ToolChoice. All ToolChoice tests and the rest of the suite pass.

### What I learned
genai's `FunctionCallingConfig` only documents `AllowedFunctionNames` as meaningful under `ANY` mode —
there is no `TOOL`/single-function mode — so forcing a specific tool is "ANY constrained to one name".

### What warrants review
- The `tool` -> Gemini `ANY` + single-name mapping (above) — confirm it matches the lead's intent.
- I copied the repo's `.env.test.local` into the worktree so the live integration tests could run; it
  is gitignored and will not be committed. Flagging for awareness.
- The pre-existing Vertex service-account test failure (above) — not mine, but noting it.

### Future work
None required for #269. If desired later, the validation helper could also reject duplicate tool
names, but that is out of scope here.

## Step 3: Apply two review fixes (fold tests into per-client test, soften GoDoc wording)

**Author:** builder (fabrik:go)

### Prompt Context

**Verbatim prompt:** (team-lead handoff) Apply two agreed code-review fixes to the ToolChoice work.
FIX 1: fold the new top-level `TestChatCompleter_ChatCompleteToolChoice` in each of the three client
test files into the existing `TestChatCompleter_ChatComplete` as subtests, then delete the standalone
function; preserve all assertions and the three scenarios; leave the core `TestToolChoiceValidate`
as-is. FIX 2: in `chat_complete.go`, change "All three clients translate it identically:" to
"...equivalently:" on the `ToolChoice` doc comment, that one word only.
**Interpretation:** Restructure the live per-client tests to match the maintainer's convention of one
capability test per client (subtests under `TestChatCompleter_ChatComplete`), and soften one word of
GoDoc, with no other changes.
**Inferred intent:** Bring the ToolChoice work in line with the maintainer's established test layout
(PR #257 feedback) and avoid implying the three providers share an identical translation mechanism.

### What I did
- Folded the three tool-choice scenarios from each standalone `TestChatCompleter_ChatCompleteToolChoice`
  into the existing `TestChatCompleter_ChatComplete` as a single `t.Run("tool choice", ...)` block that
  holds the shared `weather` tool definition plus the three original sub-subtests, then deleted the
  now-empty top-level function. Done in `clients/anthropic/chat_complete_test.go`,
  `clients/google/chat_complete_test.go`, `clients/openai/chat_complete_test.go`. Preserved every
  assertion verbatim, including Google's `// See #269` comment on the named-tool subtest.
- `chat_complete.go`: changed "translate it identically:" to "translate it equivalently:" on the
  `ToolChoice` doc comment — the single-word swap, nothing else.
- Left the core `TestToolChoiceValidate` top-level function untouched.

### Why
The maintainer's convention (PR #257 review) is one capability test per client, with scenarios as
subtests, rather than a separate top-level test function per capability. The wording swap reflects that
Google emulates `tool` mode via `ANY` + `AllowedFunctionNames` (Step 2), so the mechanism isn't
identical across providers even though the observable behavior is equivalent.

### What worked
`make lint` reported 0 issues. The three folded `tool choice` subtests pass against live OpenAI,
Anthropic, and Google APIs when run on their own, and `go build ./...` plus the core
`TestToolChoiceValidate` pass.

### What didn't work
A full `make test` run showed an Anthropic failure: `TestChatCompleter_ChatComplete/tool_choice/
any_mode_forces_a_tool_call` returned `200 OK ... {"type":"error","error":{"type":"overloaded_error",
"message":"Overloaded"}}` from `api.anthropic.com`. It is a transient server-side overload, not a code
defect — the same subtest passes in isolation (`go test -run
'TestChatCompleter_ChatComplete$/tool_choice' ./clients/anthropic/...` -> PASS), and on the failing
full run several untouched subtests (`can chat-complete`, `can use a tool`, `can describe a PDF`) failed
the same way, consistent with rate-limited/overloaded responses while the thinking-level matrix hammered
the API.

### What I learned
Anthropic surfaces an `overloaded_error` as an HTTP 200 with an error body on the streaming path, so it
shows up as a non-nil error from the Parts iterator rather than a 429/529 status — easy to mistake for a
test defect.

### What was tricky
Distinguishing the transient Anthropic overload from a real regression. Confirmed it was transient by
re-running the tool-choice subtests in isolation (PASS) and noting that untouched subtests failed
identically in the full run.

### What warrants review
- The folded `t.Run("tool choice", ...)` block in each of the three client test files — confirm it
  matches the maintainer's preferred subtest layout and that no assertion was dropped.
- The pre-existing Vertex service-account `make test` failure (Step 2) still occurs and is still
  unrelated.

### Future work
None. The two review fixes are self-contained.
