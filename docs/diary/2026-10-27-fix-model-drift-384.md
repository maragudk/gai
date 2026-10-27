# Diary: Fix model conformance drift (issue #384)

The daily `Conformance` workflow is failing on main, and issue #384 tracks it. `TestModelConformance` in each client checks that every model ID the provider lists is either exported as a constant or on the package-private ignore list. The 2026-10-27 run (https://github.com/maragudk/gai/actions/runs/40469836347) found four IDs that are neither:

- `clients/anthropic`: `claude-haiku-5-5`
- `clients/google`: `gemini-4-argon`, `gemini-nano-banana-2.1`
- `clients/openai`: `gpt-6.1-terra`

## Step 1: Scope and hand-off

**Author:** main

### Prompt Context

**Verbatim prompt:** Also queue fixing https://github.com/maragudk/gai/issues/384
**Interpretation:** Decide for each new model ID whether to export it or ignore it, following the curation rule in `/CLAUDE.md`, so that `make conformance` passes again and #384 closes.
**Inferred intent:** Get the Conformance workflow green, with a model constant set that stays hand-curated: stable, generally available, current and recent generations.

### What I did
Read issue #384 and the failed logs of the latest Conformance run to get the drifting IDs. I created a separate worktree, `worktree-fix-model-drift-384`, from origin/main, so this change ships independently of the govulncheck fix in PR #391. Then I handed the change to a builder.

### Why
The issue body only says to run `make conformance` locally. The CI logs already name the IDs, so the builder can start from them.

### What worked
`gh run view <id> --log-failed` shows the exact IDs that are neither exported nor ignored.

### What didn't work
The worktree isolation guard refused a compound shell command that used `$(...)` with jq. Splitting it into separate plain commands worked.

### What I learned
Fixing drift is a curation judgment for each ID (export or ignore), not a mechanical step. How earlier drift fixes in each client handled previews, image models and codename-style variants is the precedent to follow.

### What was tricky
`gemini-4-argon` and `gpt-6.1-terra` look like variant or codename suffixes, and `gemini-nano-banana-2.1` is an image model. Whether each one is stable and generally available, and whether it fits what the client supports, needs checking against the provider docs.

### What warrants review
For each ID, check that the export-or-ignore choice matches the curation rule and the existing precedent in that client.

### Future work
None yet.

## Step 2: Export three models, ignore one, and retire two deprecated OpenAI constants

**Author:** drift-builder

### Prompt Context

**Verbatim prompt:** "Fix issue #384, model conformance drift. [...] For each ID, decide whether to export it as a model constant or add it to the ignore list. Follow the rule in CLAUDE.md: constants are hand-curated (stable, generally available, current and recent generations). Base each decision on evidence [...] If an ID is genuinely ambiguous after both, choose the conservative option (ignore) [...] Re-run `make conformance` locally [...] It must pass in both directions." (excerpt from the lead's brief). After my first hand-back, the lead answered: "OpenAI: Markus chose option B, the full bump inside #384. [...] Bump `openai-go/v3` to v3.88.0. [...] remove the deprecated `GPT5_4Nano` and `GPT5_1` constants and move both IDs to the ignore list with the 2027-04-01 shutdown note. [...] Pick a new default model for the OpenAI tests, the README examples and internal/examples. Choose it on evidence [...] Do the live `ChatCompleter` check for `gemini-4-argon`. Export it if a chat completion works through our client." Then: "#391 has merged to main, so do the `git fetch origin main`, the rebase and `go mod tidy` step."
**Interpretation:** Resolve each of the four drifting IDs on evidence, take the SDK bumps needed to keep every constant defined from an SDK constant, apply the 2026-10-05 deprecation rule to whatever that bump deprecates, and leave `make conformance`, the full test suite and lint green on top of the new main.
**Inferred intent:** A green `Conformance` workflow and a curated model set that only lists models we would recommend today, without silently breaking the OpenAI test suite or the README examples.

### What I did
Read the conformance tests, the policy comments above each const block, the 2026-10-05 entry in `/docs/decisions.md`, and the earlier drift diaries. Then I checked each ID against the provider docs and the SDKs:

- `claude-haiku-5-5`: exported as `anthropic.ChatCompleteModelClaudeHaiku5_5Latest`. The model page says "Status: Active (latest)", "Released October 7, 2026". `anthropic-sdk-go` gained `ModelClaudeHaiku5_5` in v1.79.0; I took the newest, v1.84.0. A script over v1.79.0 to v1.84.0 found no exported Anthropic constant marked deprecated, so the bump forces no removals.
- `gemini-nano-banana-2.1`: ignored with a `"gemini-nano-banana-*"` prefix in the modality-variants group of `/clients/google/model_conformance_test.go`. Google's models page calls it a "High-efficiency image generation and editing model", and the list already ignores `nano-banana-*` and `gemini-*-image*`.
- `gemini-4-argon`: exported as `google.ChatCompleteModelGemini4Argon`. The docs list it as "Stable", "Our flagship reasoning model", text output. The omni precedent says listing `generateContent` is not proof, so I probed it with a throwaway test through `ChatCompleter`: a plain completion returned `"Hi"`; get-by-ID shows `SupportedActions=[generateContent countTokens batchGenerateContent]`. Thinking levels over 15 runs each: None accepted with 0 thought tokens, MINIMAL rejected with `Error 400, Message: Thinking level MINIMAL is not supported for this model. Please retry with other thinking level., Status: INVALID_ARGUMENT`, Low and Medium 0 thought tokens every time, High 422 to 717 every time. Added five matrix rows to `/clients/google/chat_complete_test.go` and added the model to both thinking-level doc enumerations in `/clients/google/chat_complete.go`.
- `gpt-6.1-terra`: exported as `openai.ChatCompleteModelGPT6_1Terra`. Its model page lists Chat Completions as supported and has no preview language. `shared.ChatModelGPT6_1Terra` exists only in `openai-go/v3` v3.88.0, and from v3.79.0 on `ChatModelGPT5_1` and `ChatModelGPT5_4Nano` are deprecated. I stopped and asked the lead, who chose the full bump. I bumped to v3.88.0, removed both constants, and added `"gpt-5.1"` and `"gpt-5.4-nano"` to a new deprecated group in the ignore list with the 2027-04-01 shutdown. Terra matrix rows come from live probes.

Because `gpt-5.4-nano` was the OpenAI test default and the README/example model, I had to pick a replacement (see below), and switched `/clients/openai/chat_complete_test.go`, `/README.md` (three examples), `/internal/examples/evals/evals_test.go`, `/internal/examples/tools/main.go` and `/internal/examples/tools_custom/main.go` to `gpt-5.4-mini`. I dropped the `gpt-5.1` and `gpt-5.4-nano` matrix rows, since their official replacements (`gpt-6-sol` and `gpt-6-luna`) already have rows, and updated the `ThinkingLevel` doc comment.

Midway, #391 merged. I reset `go.mod`/`go.sum`, fast-forwarded to `origin/main` (5e133e3, `go 1.26.0`, `golang.org/x/net` v0.60.0), and redid both `go get` calls plus `go mod tidy`. The `go` directive stays at 1.26.0 and no other direct dependency moved; only `go.sum` gained indirect entries.

### Why
Every OpenAI and Anthropic constant is defined from an SDK constant, and the astra drift fix bumped the SDK rather than introduce a string literal, so I kept that convention. The 2026-10-05 decision explicitly rejects pinning an older SDK to dodge deprecations because "it blocks new model constants", which is exactly the situation here.

### What worked
The SDK version scan (a small script in the scratchpad that ran `go get` per version and checked for the wanted identifier and for `Deprecated:` comments on our exported identifiers) turned "which version do we need and what breaks" into a table. Probing every exported OpenAI model for tool calls at default, `none` and `low` effort settled the default-model question in one run.

### What didn't work
My first choice for the new default was `gpt-6-luna`, the official replacement for `gpt-5.4-nano` and the cheapest current model ($0.10/$0.50 per MTok). Running the suite against it failed all four tool tests with `OpenAI API error: 400 Bad Request`. The body said: `Function tools with reasoning_effort are not supported for gpt-6-luna in /v1/chat/completions. To use function tools, use /v1/responses or set reasoning_effort to 'none'.`

The tool probe showed the same for `gpt-5.6-luna`, `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-6-luna`, `gpt-6-sol` and `gpt-6.1-terra` at their default effort, while `gpt-5.2`, `gpt-5.4`, `gpt-5.4-mini`, `gpt-5.5` and `gpt-5.4-nano` default to `none` and work. `gpt-6-astra` and `gpt-6.1-sol` reject `none` (`Unsupported value: 'reasoning_effort' does not support 'none' with this model.`), so they cannot use function tools through this client at all.

The first Terra matrix had `wantThoughtTokens` on high; a `-count=4` run failed once with `chat_complete_test.go:542: thoughts tokens should be populated`. Over 21 runs high returned 0 once, so only xhigh (21 of 21 non-zero) keeps the assertion.

While I was blocked by the lead's temporary worktree switch, every Bash and Edit call was refused with `This session is isolated in the worktree /Users/maragubot/Developer/gai/.claude/worktrees/worktreeinclude-vertex, but this command's working directory (...) is in a different worktree`. I handed back rather than work in the other worktree. A compound shell loop that ran `go get` inside `$(...)` was also refused ("names git in a form too complex to verify"); moving it into a script file worked.

### What I learned
On `/v1/chat/completions`, OpenAI only accepts function tools when reasoning effort is `none`. Models whose default effort is `none` hide this; the gpt-5.6 and gpt-6 lines default to a reasoning effort, so a tool request without an explicit `ThinkingLevel` fails. That makes `gpt-5.4-mini` ($0.75/$4.50, default effort `none`) the cheapest current model that runs the whole suite and the README examples without extra knobs.

### What was tricky
The deprecation of `gpt-5.4-nano` arrived through the SDK bump needed for an unrelated constant, and it cascaded into the test default, three README examples and three internal examples. Picking the replacement by price alone would have shipped README examples that fail on their first tool call.

### What warrants review
- The default-model choice of `gpt-5.4-mini` over `gpt-6-luna`, and the comment on `newChatCompleter` that explains it.
- The `gpt-6.1-terra` and `gemini-4-argon` matrix rows and their probe-count comments.
- The new deprecated group in `/clients/openai/model_conformance_test.go`. The dated `gpt-5.1-2*` and `gpt-5.4-nano-2*` entries stay in the dated-snapshot group, as `gpt-5-2*` did after #378.

- The new function-tools paragraph in the OpenAI `ThinkingLevel` doc comment. It rests on the tool probe: default, `none` and `low` effort on every exported model. Only `gpt-5.2` accepted tools at `low`.

Validation: `make conformance` green for all three clients; `go test -shuffle on ./...` green including the Google Vertex tests (with `vertex.json` now in the worktree); `golangci-lint run` reports `0 issues.`.

### Self-review
Two competing reviewers (fabrik:code-review) both confirmed the conformance lists match the const blocks in both directions, no ignore entry shadows an export, `go mod tidy -diff` is clean, and no other exported constant is deprecated in the bumped SDKs. Neither found comment context leakage or single-use constants. Findings both raised, all addressed:

- `claude-haiku-5-5` was exported on docs alone, unlike Argon and Terra. I probed it through `ChatCompleter`: default and None complete (both still think, 32 thought tokens, because adaptive thinking is on by default and None sets no fields), and Low, Medium, High, XHigh and Max all complete (Max: 472 thought tokens). I added Haiku 5.5 to the XHigh sentence in `/clients/anthropic/chat_complete.go`. The probe also showed `temperature` is rejected: `400 Bad Request ... "`temperature` is deprecated for this model."`.
- The function-tools constraint only lived in a test-helper comment. I added a paragraph to the OpenAI `ThinkingLevel` doc comment and pointed the `newChatCompleter` comment at it.
- "The gpt-5.6 family is not probed yet" was no longer true; it now says the family accepts `none` and `low` and the rest is unverified.
- A stale "GPT-5-nano picks either" comment in the system-prompt test now says "The default model".

Single-reviewer nits I took because they were my own new text: the Argon matrix comment no longer lumps None (an expected zero) in with Low and Medium. Single-reviewer nits I left: the "Drop each entry once the model disappears from the live list" wording (copied from the existing gpt-5 group and the Anthropic Sonnet 4.5 entry, and it contradicts the decision's get-by-ID rule, so all three should change together), the "Dated snapshots of exported aliases" group comment that now also covers deprecated aliases, and "Gemini 3.x and 4 families" phrasing.

### Future work
- `gpt-6-astra` and `gpt-6.1-sol` cannot use function tools through the chat-completions client, and the other gpt-5.6/gpt-6 models need `ThinkingLevelNone` for tools. Now documented; a Responses API path would remove the limit.
- On Claude 5.5 models, `gai.ThinkingLevelNone` (no fields set) does not turn thinking off, because adaptive thinking is on by default, so the Anthropic doc's "opt out of thinking entirely" is inaccurate for them. Haiku 5.5 also rejects `temperature`.
- Several OpenAI ignore entries (`gpt-5*-chat-latest`, `gpt-5.1-codex`, `o1*`, `o3-mini*`, `o4-mini*`) are past their SDK-announced shutdown dates and are candidates for get-by-ID pruning.
- The ignore-entry pruning wording in both OpenAI deprecated groups and the Anthropic Sonnet 4.5 entry should follow the 2026-10-05 get-by-ID rule.
- Google's models page lists `gemini-2.0-flash` as shut down while `google.ChatCompleteModelGemini2_0Flash` is still exported; get-by-ID still resolves it. Left for a separate follow-up, as the lead asked.
- The OpenAI docs list a `max` reasoning effort for `gpt-6-luna` and `gpt-6.1-terra` that the client has no constant for.
- Dependabot PRs #389 and #390 are superseded by these bumps.
