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
