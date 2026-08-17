# Diary: Model-constant policy and live conformance test for the clients

Establish a hand-curated model-constant policy for the three clients (`openai`, `anthropic`, `google`) and enforce it in both directions with a live-API `TestModelConformance` per client. The feature was designed in a brainstorm with Markus after OpenAI's server-side deprecation of `gpt-5.3-chat-latest` broke CI on main (fixed in PR #333) and exposed that the constant lists rot silently. The full design and its rejected alternatives are recorded in `/docs/decisions.md` under "2026-08-17: Curate model constants per client and enforce the list with a live conformance test".

## Step 1: Design the policy and enforcement

**Author:** main

### Prompt Context

**Verbatim prompt:** "Maybe we should do a PR that, for each client, just re-exports all model constants, and says so in a comment? So we always mirror? Especially the \"latest\", non-preview ones"
**Interpretation:** Markus proposed mechanically mirroring the SDKs' model constants in each gai client so the lists stay fresh without hand-written PRs.
**Inferred intent:** Stop the model-constant lists from lagging SDK releases and rotting when providers kill models -- with low maintenance, good discoverability, and one consistent policy across the three clients.

### What I did

Ran a brainstorm with Markus, one question at a time: goal (all of: freshness, low upkeep, discoverability, consistency), scope (current + recent generations, not a full SDK mirror), enforcement (live conformance test), and drift policy (fail both directions, with hand-curated per-client ignore lists). Presented the design in three sections -- policy, test mechanics, rollout -- each approved. Recorded the decision in `/docs/decisions.md` via the decisions skill. Created worktree `model-conformance` from main including PR #333's merge, and started this diary. Implementation is delegated to a builder next.

### Why

A full SDK mirror fails on the facts: openai-go retains dead models (v3.50.0 still ships `ChatModelGPT5_3ChatLatest`), its full list is dominated by dated snapshots, Go cannot enumerate constants at runtime, and Google's genai SDK exports no model constants at all. Curation with machine-checked drift detection serves all four goals without importing corpses.

### What worked

Grounding the brainstorm in the freshly-diagnosed gpt-5.3 failure made the trade-offs concrete: "mirror the SDK" would have re-added the exact constant we had just removed.

### What didn't work

Nothing failed; the design converged in four questions. Worth noting: a same-day CI flake in google's `thinking level matrix` ("should stream PartTypeThought parts", passed on rerun) illustrated the design's known limitation -- the conformance test catches existence drift, not behavior drift.

### What I learned

Providers' list endpoints omit some alias IDs (e.g. Anthropic's `claude-sonnet-4-5`), so the existence check must resolve constants via get-by-ID, not by searching list output.

### What was tricky

Balancing freshness pressure against noise: failing on unexported new models is only tolerable because the ignore list supports prefix patterns that absorb the steady stream of dated snapshots.

### What warrants review

After the builder's change: identical policy-comment phrasing across the three clients, per-ID existence checks (not list-membership), ignore lists that make CI green today, and the triage outcomes -- any newly exported constants and the fate of `ChatCompleteModelClaudeOpus4_1Latest` (EOL 2026-08-05, may already 404).

### Future work

Behavior-drift coverage remains with the per-client capability matrices; if provider-side behavior changes become frequent, they may warrant their own guardrails.

## Step 2: Implement the policy comments and TestModelConformance, and triage the live results

**Author:** conformance-builder

### Prompt Context

**Verbatim prompt:** "You are \"conformance-builder\", a builder working in the worktree at /Users/maragubot/Developer/gai/.claude/worktrees/model-conformance (branch worktree-model-conformance) of the maragu.dev/gai Go library. [...] ### 1. Policy comment (all three clients) [...] ### 2. TestModelConformance (one per client package) [...] ### 3. Triage (run the tests live and dispose of every failure)" (excerpted from the lead's full task brief; the elided sections spell out the phrasing requirements, the two subtest checks, the ignore-list matcher, and the triage decision rule)
**Interpretation:** Implement the already-designed policy: identical policy comments above each client's model const block, a live `TestModelConformance` per client with per-ID existence checks and a list-freshness check against a package-private ignore list, then run everything live and dispose of each failure by exporting, ignoring, or removing.
**Inferred intent:** Land the enforcement mechanism designed in Step 1 with the ignore lists already triaged, so CI is green today and any future provider-side change reddens CI with an actionable message.

### What I did

Copied `/Users/maragubot/Developer/gai/.env.test.local` into the worktree root (verified gitignored by the `/.env.*.local` rule; never committed). Added the identical four-line policy comment above the model const blocks in `/clients/openai/chat_complete.go`, `/clients/anthropic/chat_complete.go`, and `/clients/google/chat_complete.go` — openai and google get an extra sentence pointing at `[EmbedModel]`, google an extra note that the genai SDK exports no model constants. Wrote `/clients/openai/model_conformance_test.go`, `/clients/anthropic/model_conformance_test.go`, and `/clients/google/model_conformance_test.go`, each with an `exportedModels` slice, an `ignoredModels` slice (exact IDs plus trailing-`*` prefix patterns, matched by a ~10-line `isIgnoredModel`), and a `TestModelConformance` with two subtests: get-by-ID existence for every exported constant, and list freshness failing on any ID that is neither exported nor ignored. All three use the SDKs' own model endpoints (`Models.Get`/`ListAutoPaging` for openai and anthropic, `Models.Get`/`Models.All` for google, the latter scoped to models whose `SupportedActions` include `generateContent` or `embedContent`).

Live triage: anthropic's `claude-opus-4-1` 404s on get-by-ID, so `ChatCompleteModelClaudeOpus4_1Latest` is removed under the killed-server-side rule (no test rows referenced it). Freshness hits exported `ChatCompleteModelClaudeOpus4_8Latest` and `ChatCompleteModelClaudeOpus5Latest` (both GA per get-by-ID metadata and shipped as constants in the pinned SDK v1.61.0) and, for google, `ChatCompleteModelGemini3_5FlashLite`, `ChatCompleteModelGemini3_6Flash`, and `ChatCompleteModelGemini3_7Flash` (stable IDs, no preview suffix). Ignored: anthropic's three dated snapshots via per-family prefixes plus `claude-fable-5` (new family, flagged to the lead); google's floating `-latest` aliases, previews, modality variants, and non-Gemini artifacts (20 entries). Added the one-line policy note to `/AGENTS.md` (which `/CLAUDE.md` symlinks to) under Testing & Quality Gates. After self-review, added an identical cross-reference comment above the `EmbedModel` const blocks in `/clients/openai/embed.go` and `/clients/google/embed.go`.

### Why

The design in Step 1 fixed the mechanism; this step makes it real and discharges today's drift so the tests are green at merge. Per-ID lookup (not list membership) is load-bearing: anthropic's list output omits alias IDs for newer families, and google's aliases never appear in list output at all.

### What worked

Red/green TDD with the live API as the oracle: writing the tests with empty ignore lists first made the providers themselves enumerate the triage backlog — the anthropic run printed exactly six IDs to dispose of, the google run thirty-one. The SDK get-by-ID metadata (display name, created_at, capabilities) plus the pinned SDK's own constant set made most export-vs-ignore calls mechanical.

### What didn't work

The `OPENAI_KEY` in `.env.test.local` is dead: every OpenAI call fails with `401 Unauthorized ... "Incorrect API key provided: sk-proj-***...Ct8A"`, `"code": "invalid_api_key"`. This is not new breakage from this change — the pre-existing live test fails identically (`go test ./clients/openai -run "TestEmbedder_Embed/can_embed"`). The anthropic and google keys in the same file work. Consequence: the OpenAI `TestModelConformance` is written but untriaged — its `ignoredModels` is empty and the test fails on 401 until the key is rotated. I did not guess an ignore list for it. Also minor: `golangci-lint run` initially reported a phantom `unused` issue pointing into the deleted sibling worktree `remove-gpt-5-3-chat-latest`; `golangci-lint cache clean` resolved it (0 issues).

### What I learned

Anthropic's list output currently surfaces dated snapshots only for the 4.5-era families (`claude-haiku-4-5-20251001`, `claude-opus-4-5-20251101`, `claude-sonnet-4-5-20250929`); newer families appear as bare aliases only. Gemini's list is dominated by non-chat artifacts (image, TTS, robotics, music, deep-research, IDE products), so the freshness check filters by `SupportedActions` before consulting the ignore list. `gemini-embedding-2` resolves fine via the Gemini API's `models.get` despite being multi-region-only on Vertex.

### What was tricky

Prefix patterns that absorb dated snapshots without swallowing siblings: `claude-opus-4-*` would match the real model `claude-opus-4-8`, so the snapshot patterns pin the family and the date's leading digit (`claude-opus-4-5-2*`). Similarly, google's image variants need per-family prefixes (`gemini-3.1-flash-image*`) because `gemini-3.1-flash-*` would swallow `gemini-3.1-flash-lite`.

### What warrants review

Self-review ran two competing reviewers over the diff. Consensus finding (fixed): the "same policy applies" pointer lived only on the chat const blocks, invisible to a reader landing on `EmbedModel` — both `embed.go` files now carry the cross-reference. Deferred single-reviewer notes: the anthropic snapshot patterns cover only the 4.5 families (newer families do not currently list dated snapshots; a future one reddens CI by design, discharged with one ignore line), and the google thinking-level doc comment's Flash enumeration does not mention the three new constants (their `ThinkingLevelNone` behavior is unprobed, and the brief forbids touching the thinking-level docs/matrices). Reviewers verified `exportedModels` matches the const blocks 1:1 in all three packages and that no `ChatCompleteModelClaudeOpus4_1Latest` references remain. To validate: `go test -shuffle on ./clients/anthropic ./clients/google -run TestModelConformance` (green, run live twice), `golangci-lint run` (0 issues); `./clients/openai` stays red until the key is rotated.

### Future work

Rotate `OPENAI_KEY` in `.env.test.local`, run the OpenAI `TestModelConformance`, and triage its (large) freshness backlog — openai's list has no capability metadata, so scoping happens entirely through ignore prefixes. Decide whether `claude-fable-5` (new family, GA-looking metadata, absent from the pinned SDK) should be exported or stay ignored.

## Step 3: Export Fable 5, triage OpenAI with the rotated key, and probe the new Gemini models' thinking levels

**Author:** conformance-builder

### Prompt Context

**Verbatim prompt:** "Lead here — Markus answered both of your questions. Two new tasks in the same worktree; the OpenAI key is NOT rotated yet, so leave the openai triage parked. 1. Export `claude-fable-5` [...] 2. Probe and document the three new Gemini constants' thinking-level behavior [...] The earlier prohibition on touching thinking-level docs/matrices is lifted for exactly these three models — nothing else." (excerpted; the elided text specifies matrix row style, the single-turn constraint, and the flake warning). A rotated `OPENAI_KEY` then landed mid-session, unparking the OpenAI triage from Step 2's future work.
**Interpretation:** Export Fable 5 into the anthropic client and re-verify conformance live; establish the thinking-level accept/reject matrix for `gemini-3.5-flash-lite`, `gemini-3.6-flash`, and `gemini-3.7-flash` by live probing, then encode only non-flaky expectations; and complete the parked OpenAI triage once credentials worked.
**Inferred intent:** Finish the two curation decisions Markus made, close the OpenAI gap from Step 2, and extend behavior coverage to the newly exported Gemini models without introducing CI flakes.

### What I did

Exported `ChatCompleteModelClaudeFable5Latest` in `/clients/anthropic/chat_complete.go` and moved it from the ignore list to `exportedModels`; the anthropic `TestModelConformance` is green live. With the rotated key, ran the OpenAI conformance test: the existence check flagged `gpt-5.1-mini` (404 `model_not_found`, absent from list output), so `ChatCompleteModelGPT5_1Mini` is removed under the killed-server-side rule — including its `exportedModels` row, a stale matrix comment in `/clients/openai/chat_complete_test.go` (which had already noted the model 404ing months ago), and `/internal/examples/robust/main.go`, which now uses `ChatCompleteModelGPT5_4Mini`. Triaged the 111-ID freshness backlog into a 46-entry ignore list: dated-snapshot prefixes, ChatGPT-tracking floaters (including `gpt-5.3-chat-latest`, the ID that started this whole feature), Responses-only pro models, codex/search variants, audio/realtime/image/video/moderation surfaces, and legacy generations. Zero new OpenAI exports: curl probes showed `gpt-5-pro`, `gpt-5.4-pro`, and `gpt-5.5-pro` all reject `/v1/chat/completions` ("This model is only supported in v1/responses" / "This is not a chat model").

For Gemini, added 15 rows to the `thinking level matrix` subtest in `/clients/google/chat_complete_test.go`, first as flag-less probe rows, then encoded from two full live runs plus targeted reruns: `gemini-3.5-flash-lite` and `gemini-3.6-flash` reject `ThinkingLevelNone` with a generic 400 INVALID_ARGUMENT; `gemini-3.7-flash` accepts None but rejects MINIMAL ("Thinking level MINIMAL is not supported for this model"); minimal yields zero thoughts_tokens where accepted; low/medium/high populate thoughts_tokens on every probe. Updated the thinking-level doc comment in `/clients/google/chat_complete.go` to enumerate the new accept/reject split.

### Why

Both tasks discharge open questions from Step 2 with Markus's explicit decisions, and the OpenAI triage completes the conformance rollout so all three clients are enforced. The example-file swap keeps a removed constant from breaking the build; the flag-less-probe-rows-first approach let the live API dictate the matrix instead of my assumptions.

### What worked

Using the matrix test itself as the probe harness (lenient rows, read the `thoughtParts/textParts/thoughtsTokens` logs, then tighten) — no throwaway scripts, and the probe data is reproducible by rerunning the subtest. The competing-reviewer self-review earned its cost twice over (see below).

### What didn't work

My initial claim that the pinned anthropic-sdk-go v1.61.0 lacks a `ModelClaudeFable5` constant was wrong: my verification grep used `ModelClaude(Fable|Opus4_8|...)\b`, and the `\b` after the alternation can never match between `Fable` and `5` (both word characters), so the existing constant was invisible. I shipped a bare string with a false comment; reviewer A caught it and the constant now wraps `anthropic.ModelClaudeFable5` like its siblings. Separately, my four probes of `flash 3.7 + none` all showed thoughts_tokens > 0, so I encoded `wantThoughtTokens: true` — reviewer A's larger sample (17 runs) found two zero-token runs (~1 in 16), exactly the flaky-expectation trap the lead warned about; the flag is removed and both comments now say "usually".

### What I learned

A regex word boundary directly after an alternation group silently kills matches when the next character is alphanumeric — verify SDK constants with `go doc <pkg> <identifier>`, not grep patterns. Four probes is not enough to encode a nondeterministic assertion; the reviewers' independent larger sample was the difference between a documented behavior and a planted CI flake. OpenAI's pro-tier models are Responses-API-only across every generation probed (including the already-exported `gpt-5.2-pro`), and its models list carries no capability metadata, so the ignore list is the only scoping tool.

### What was tricky

Encoding `gemini-3.7-flash + none`: the model accepts the request but usually thinks anyway despite `ThinkingBudget=0` — behavior worth documenting but not asserting. The row now asserts only that the call succeeds, with the nondeterminism recorded in the comment. On the OpenAI side, distinguishing "killed" from "restricted": `gpt-5.1-mini` 404s on get-by-ID *and* is absent from list output, which together justify removal, whereas a bare get-by-ID failure could also mean a key-scoping artifact.

### What warrants review

The self-review (two competing reviewers, both running live verification) had consensus on two issues, both fixed: the false Fable 5 SDK comment (now uses the SDK constant) and the pro-models ignore comment reading as if `gpt-5.2-pro` were unaffected (now carries a parenthetical). Reviewer A alone flagged `gpt-4*` as an over-broad legacy prefix that also absorbs actively-shipped 4o modality variants; reviewer B checked the same wildcards and found no wrongful swallowing, so it stays — noted for the lead. Open question raised to the lead: `ChatCompleteModelGPT5_2Pro` is exported but cannot work through `ChatCompleter` (chat-completions rejects it); removing it is an API break outside this change's rules. To validate: `go test -count=1 ./clients/... -run TestModelConformance` (all three green live) and `go test ./clients/google -run "TestChatCompleter_ChatComplete/thinking_level_matrix"`.

### Future work

Decide the fate of the exported-but-unusable `ChatCompleteModelGPT5_2Pro`. The pinned anthropic SDK also ships `ModelClaudeMythos5` (`claude-mythos-5`), which does not appear in this key's list output and was therefore never a freshness hit — worth a curation glance when it surfaces. If `flash 3.7 + none`'s zero-budget-but-thinking behavior changes server-side, the matrix row's comment (not an assertion) is the only thing to update.

## Step 4: Remove the alive-but-unusable `gpt-5.2-pro` and extend the policy to API-surface fit

**Author:** conformance-builder

### Prompt Context

**Verbatim prompt:** "Lead again — Markus decided: remove `openai.ChatCompleteModelGPT5_2Pro`. Rationale: a constant that can only produce a runtime API error through gai's ChatCompleter is a trap; alive-but-unusable gets the same treatment as dead. [...] 2. This establishes a policy nuance, so extend the shared policy comment with one clause — models that cannot work through the client's implemented API surface (e.g. Responses-API-only) are not exported — keeping the phrasing IDENTICAL across all three clients as before." (excerpted; the elided text specifies the sweep discipline, the ignore-list placement, and the process steps)
**Interpretation:** Remove the constant Step 3 flagged as an open question, fold `gpt-5.2-pro` into the pro-models ignore block, and encode the new alive-but-unusable rule as one identical clause in all three policy comments.
**Inferred intent:** Close the last open curation question so the feature lands with no known traps in the exported API, and make the rule that resolved it part of the written policy so future triage applies it without re-litigating.

### What I did

Removed `ChatCompleteModelGPT5_2Pro` from `/clients/openai/chat_complete.go` and its row from `exportedModels` in `/clients/openai/model_conformance_test.go`. In the ignore list, added `gpt-5.2-pro*` to the pro-models block (whose Responses-API-only comment now covers it), dropped the now-subsumed `gpt-5.2-pro-2*` snapshot entry, and deleted the special-case parenthetical that described the constant as deliberately exported. Extended the policy comment in all three clients' `chat_complete.go` with the API-surface clause; a `grep | sort | uniq -c` over the five comment lines confirms all three copies are byte-identical. The lead's own amendment to `/docs/decisions.md` (recording the alive-but-unusable rule) was already sitting uncommitted in the worktree and rides along in this commit untouched.

### Why

`gpt-5.2-pro` resolves on get-by-ID but rejects `/v1/chat/completions` ("This is not a chat model"), so the constant's only reachable behavior through `ChatCompleter` was a runtime error. Markus's rule makes API-surface fit part of the curation bar, and the identical-phrasing constraint keeps the three clients telling one story.

### What worked

The sweep was cheap because Step 3's removal of `ChatCompleteModelGPT5_1Mini` had already established the reference checklist (const, `exportedModels`, matrix rows, examples): a single grep found exactly four live references and no matrix rows or example usages existed for this one.

### What didn't work

Nothing failed in this step; both live OpenAI conformance runs and the full lint pass were green on the first attempt.

### What I learned

The prefix consolidation mattered more than it looked: keeping both `gpt-5.2-pro-2*` (snapshots) and `gpt-5.2-pro*` (everything) would have been harmless but misleading, implying the alias and its snapshots are triaged under different rules when the new policy treats the whole family as one exclusion.

### What was tricky

Only the phrasing: folding "cannot work through the client's implemented API surface (e.g. Responses-API-only)" into the existing exclusion sentence without breaking the identical-across-clients constraint, given the example is OpenAI-flavored while the clause must also read sensibly in the anthropic and google files.

### What warrants review

The delta is mechanical (one constant removal, one ignore entry, one comment clause ×3), so this step's self-review was a solo pass rather than the competing-reviewer setup: full diff read, grep-clean sweep verification (`GPT5_2Pro`/`gpt-5.2-pro` survive only in diary history and the ignore entry), mechanical comment-identity check, and live reruns. To validate: `go test -count=1 ./clients/openai -run TestModelConformance` (green twice, once shuffled) and `golangci-lint run` (0 issues). Reviewers should confirm the decisions-entry amendment matches what Markus decided, since it was authored by the lead and only transported by this commit.

### Future work

None new. The Step 3 items stand: `claude-mythos-5` deserves a curation glance when it surfaces in list output, and the openai key's possible scope narrowing (`missing_scope` on the vision test) should be checked before the PR merges.

## Step 5: Switch the gpt-5.5 constant to the SDK enum

**Author:** conformance-builder

### Prompt Context

**Verbatim prompt:** "Lead — final polish task. I rebased the branch onto current main [...] openai-go is now v3.50.0, which ships `ChatModelGPT5_5` — the exact condition the gpt-5.5 bare-string constant's comment says to act on." (excerpted; the elided text specifies the verification and process steps)
**Interpretation:** The Dependabot bump fulfilled the condition the bare-string constant's comment named, so wrap `openai.ChatModelGPT5_5` like the sibling constants and drop the SDK-pin comment.
**Inferred intent:** Land the feature with no self-referential TODOs left in the const block, and retire the comment whose version reference (v3.44.0) had already drifted.

### What I did

Switched `ChatCompleteModelGPT5_5` in `/clients/openai/chat_complete.go` from the bare `"gpt-5.5"` string to `ChatCompleteModel(openai.ChatModelGPT5_5)`, deleted the three-line SDK-pin comment, and folded the constant into the single aligned const group (removing the blank line that had separated it from the 5.6 trio). Verified the SDK constant's value is byte-identical (`go doc github.com/openai/openai-go/v3/shared ChatModelGPT5_5` shows `"gpt-5.5"`), so the exported string set is unchanged.

### Why

The comment was an explicit instruction to make this change once the SDK shipped the enum; after the rebase onto main with openai-go v3.50.0, keeping the bare string would have preserved a falsified comment (it named v3.44.0 as the pin, already stale against the previous v3.46.0).

### What worked

Verifying the enum's exact value with `go doc <pkg> <identifier>` before editing — the Step 3 lesson applied deliberately, instead of pattern-grepping the SDK.

### What didn't work

Nothing failed; build, vet, the live OpenAI `TestModelConformance`, and the full lint pass were green on the first attempt.

### What I learned

Nothing new; this step cashed in Step 3's lesson rather than generating one.

### What was tricky

Nothing — a value-preserving one-constant swap. The only care point was confirming the rebase had not otherwise changed the const block before editing on top of it.

### What warrants review

The diff is nine lines of realignment plus the swap; confirm the constant's value is unchanged (`gpt-5.5`) and that the removed comment carried no other obligation. Validate with `go test -count=1 ./clients/openai -run TestModelConformance` (green live) and `golangci-lint run` (0 issues).

### Future work

None new; the standing Step 3/4 items are unchanged.
