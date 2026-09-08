# Diary: Export `gpt-6-astra` to fix OpenAI model conformance drift

The daily `Conformance` workflow opened issue #355 after OpenAI's live model list grew one ID that `TestModelConformance` in `/clients/openai` neither exports nor ignores. This diary triages it by the written curation policy and lands the fix.

## Step 1: Diagnose the drift and decide by precedent

**Author:** main

### Prompt Context

**Verbatim prompt:** "Fix 355" followed mid-turn by "Make PR after" and "See https://github.com/maragudk/gai/issues/355"
**Interpretation:** Resolve GitHub issue #355, the auto-filed `model-drift` issue from the `Conformance` workflow run 33953409610, and open a pull request when the fix is ready.
**Inferred intent:** Keep the model-constant lists current with minimal ceremony by applying the existing curation policy rather than re-deciding each new model.

### What I did

Read issue #355 with `gh issue view --json`, then pulled the failing test output from CI with `gh run view 33953409610 --log-failed`. The `every listed model ID is exported or ignored` subtest in `/clients/openai/model_conformance_test.go` reports: `export or ignore these model IDs: gpt-6-astra`. Because the CI run is three days old, I re-ran `make conformance` locally against all three providers with the keys in `.env.test.local`: Anthropic and Google are green, and OpenAI still fails on exactly `gpt-6-astra`, so nothing else has drifted since. Checked the pinned `github.com/openai/openai-go/v3 v3.52.0` for an SDK constant: none. Fetched v3.56.0 (the newest) into a scratch module and found `shared.ChatModelGPT6Astra ChatModel = "gpt-6-astra"` sitting alongside the `ChatModelGPT5_6Luna`/`Sol`/`Terra` constants the client already exports. Created worktree `gpt-6-astra-drift` from `origin/main`, started this diary, and delegated the change to a builder.

### Why

The policy exports stable, GA, current-generation models that work through the client's implemented API surface. `gpt-6-astra` is a named model in the same shape as the exported GPT-5.6 Luna/Sol/Terra trio, not a dated snapshot, floating alias, pro/codex/search variant, or non-chat surface, so it is exported as `ChatCompleteModelGPT6Astra`. All existing OpenAI constants are defined via SDK constants rather than string literals, so the SDK is bumped to v3.56.0 to keep that convention rather than introducing the first literal.

### What worked

The failing test's message named the exact ID, and the 5.6 named-model precedent plus the SDK constant made the disposition a lookup.

### What didn't work

The first `gh issue view 355 --comments` printed nothing at all (no error either); `--json title,body,labels,state,comments` returned the issue fine. Cosmetic, but worth knowing.

### What I learned

OpenAI has moved from `gpt-5.x` point releases to named models (`gpt-5.6-luna`, `gpt-6-astra`); the naming pattern for the Go constants is `GPT<major>[_<minor>]<Name>`.

### What was tricky

Nothing technical. The session started on `main` at the repository root rather than in a worktree, so the worktree had to be created explicitly before any work.

### What warrants review

After the builder's change: the new constant sits after the GPT-5.6 trio in both the const block and `exportedModels`; the SDK bump to v3.56.0 leaves `go test ./...` and `golangci-lint run` green with no behavioural change; the constant was live-verified through `ChatCompleter` (a real chat completion, not only `Models.Get`); the OpenAI `TestModelConformance` is green live; `/docs/index.html` and the client README were swept for constant enumerations.

### Future work

None beyond closing #355 when the PR merges.

## Step 2: Bump the SDK, export the constant, and ground it against the live API

**Author:** astra-builder

### Prompt Context

**Verbatim prompt:** "Fix GitHub issue #355 (model conformance drift). `TestModelConformance/every_listed_model_ID_is_exported_or_ignored` in `clients/openai` fails with: `export or ignore these model IDs: gpt-6-astra`. The lead has already decided the disposition: **export it**, by precedent of the GPT-5.6 Luna/Sol/Terra named models already exported." — followed by nine numbered steps covering the SDK bump, the constant, live verification through `ChatCompleter`, a live conformance run, a docs sweep, the quality gates, self-review, a commit, and a pushed PR. Mid-step the lead added: "`.env.test.local` is now in the worktree root (gitignored, confirmed). Continue with steps 3 and 4 … Also run the full `go test -shuffle on ./clients/openai/` with keys so the tool-call subtests exercise the v3.56.0 streaming reorder you found."

**Interpretation:** Land the mechanical change (SDK bump plus one exported constant), prove against the live API that `gpt-6-astra` actually works through chat completions rather than merely resolving via `Models.Get`, and open a PR.

**Inferred intent:** Close the drift issue with a change that is grounded in real API behaviour, not just a list edit that silences a failing test.

### What I did

Bumped `github.com/openai/openai-go/v3` from v3.52.0 to v3.56.0 with `go get` and `go mod tidy`. The result is minimal: `/go.mod` moves one line and `/go.sum` moves the two `openai-go` hashes, with no indirect dependency shifting and no `toolchain` line appearing. `go build ./...` and `go vet ./...` were clean, so the bump needed no adaptation.

Added `ChatCompleteModelGPT6Astra = ChatCompleteModel(openai.ChatModelGPT6Astra)` to the const block in `/clients/openai/chat_complete.go`, directly after `ChatCompleteModelGPT5_6Terra`, and the matching `string(openai.ChatCompleteModelGPT6Astra)` in the same position in `exportedModels` in `/clients/openai/model_conformance_test.go`.

Swept the docs. Neither `/docs/index.html` nor `/clients/openai/README.md` enumerates model constants — the only occurrences anywhere are `/README.md` examples that deliberately pin `GPT5Nano`, and the client README is a roadmap stub. Nothing to update, so nothing was touched.

Ran a competitive two-reviewer self-review via the code-review skill. Both reviewers independently flagged that the `ThinkingLevel` doc block had become factually wrong, so I fixed the scope sentence to read "across the exported chat-completions models" instead of "across the gpt-5.x chat-completions family".

Live verification was blocked at first (see below). Once the lead placed `.env.test.local` in the worktree, I probed `gpt-6-astra` with a throwaway test built on the package's own `newChatCompleter` helper, deleted before committing. A real chat completion works:

```
MODEL REPLY: "AI stands for **artificial intelligence**."
```

Since the key was now available, I went past the brief and probed the whole reasoning-effort range, which turned the doc caveat into real data and let me fill in the coverage both reviewers had asked for. Added six matrix rows for `gpt-6-astra` in `/clients/openai/chat_complete_test.go` matching the sibling six-row shape, replaced the "not probed yet" caveat in the `ThinkingLevel` doc block with a real bullet, and added the model to the `ThinkingLevelXHigh` note.

### Why

The SDK bump exists only so the new constant can be defined from `openai.ChatModelGPT6Astra` like every one of its siblings, rather than introducing the package's first bare string literal.

The `ThinkingLevel` work is a direct consequence of this change and not scope creep. "The union of reasoning_effort values across the gpt-5.x chat-completions family" was still literally true when the GPT-5.6 trio was exported in `4c23bd0` (5.6 is gpt-5.x), and exporting a gpt-6 model is the first edit that falsifies it. Leaving it would have shipped a comment that promises live-probed accuracy while silently annexing an unprobed model. The matrix rows follow from the same commitment: the matrix header in `/clients/openai/chat_complete_test.go` states that it matches the GoDoc bullet list, so a new bullet without new rows would break the invariant the header advertises.

### What worked

The disposition needed no re-litigation. No pattern in `ignoredModels` matches `gpt-6-astra` — every `gpt-*` prefix present (`gpt-3.5-*`, `gpt-4*`, the `gpt-5*` family, `gpt-audio*`, `gpt-image*`, `gpt-live-transcribe`, `gpt-realtime*`, `gpt-transcribe`) fails to reach it — and `isIgnoredModel` is consulted only after `slices.Contains(exportedModels, id)` fails, so the export cannot collide with the ignore list either way.

The two-reviewer split paid off. Both converged on the stale `ThinkingLevel` comment and on live verification being the crux, which is exactly the signal the code-review skill's consensus rule is designed to surface.

Everything is green with keys. The live conformance test passes:

```
GAI_MODEL_CONFORMANCE=1 go test -count=1 -shuffle on -run TestModelConformance ./clients/openai/
ok  	maragu.dev/gai/clients/openai	10.060s
```

So does the full keyed package, tool-call subtests included, at `ok maragu.dev/gai/clients/openai 158.401s`. `golangci-lint run` reports `0 issues.`, and `go build`, `go vet` and `gofmt -l` are clean.

### What didn't work

Live verification was blocked for the first half of this step. `.env.test.local` is gitignored, so creating the worktree left it behind in the primary checkout, and neither `OPENAI_KEY` nor `OPENAI_API_KEY` was set in the environment. The primary checkout is outside this worktree and outside my scope boundary, so I did not read or copy it — my instructions name a missing API key as the specific case where I stop and report rather than scavenge. The probe failed on authentication rather than on the model:

```
POST "https://api.openai.com/v1/chat/completions": 401 Unauthorized {
    "message": "You didn't provide an API key. ..."
}
```

I committed the mechanical work locally, left the branch unpushed, and reported the blocker with the question. The lead placed the file in the worktree and continued me, which unblocked steps 3 and 4.

The first attempt at the throwaway probe also failed to compile twice, from guessing at API shapes instead of reading them: `gai.ChatCompleteRequest.ThinkingLevel` is a `*gai.ThinkingLevel` and needs `gai.Ptr`, and usage is `res.Meta.Usage.ThoughtsTokens` — a field, not a `Meta()` method, and "Thoughts" rather than "Thought".

```
vet: clients/openai/zz_astra_live_test.go:48:21: cannot use level (variable of string type gai.ThinkingLevel) as *gai.ThinkingLevel value in struct literal
```

### What I learned

`gpt-6-astra` has a reasoning-effort shape no other exported model has: it rejects `none` **and** `minimal`, so reasoning cannot be turned off at all. The API states the range itself, which is why the matrix comment quotes it verbatim:

```
"Unsupported value: 'reasoning_effort' does not support 'none' with this model.
 Supported values are: 'low', 'medium', 'high', and 'xhigh'."
```

Every other model that rejects `minimal` accepts `none`, so this is the first exported model where `gai.ThinkingLevelNone` is unavailable. Repeated probes show `low` returns 0 reasoning tokens consistently while `medium` reliably returns roughly 30, so the `low` row makes no thoughts-tokens assertion and the other three do — the same treatment `gpt-5.4-nano` gets for the same reason.

`Models.Get` is not evidence of chat-completions support, and this package has a live counterexample in its own source: `gpt-5.2-pro` is a member of the SDK's `ChatModel` enum — the chat-completions model type — yet `/clients/openai/model_conformance_test.go` ignores the whole `gpt-5.2-pro*` prefix because pro models are Responses-API-only. So neither `Models.Get` succeeding nor SDK `ChatModel` membership can stand in for a real completion, which is exactly why the brief insisted on one.

The SDK bump is also not the no-op it looks like. `streamaccumulator.go` grew from roughly 215 to 970 lines, and upstream fix #757 reordered the state classification in `chatCompletionResponseState.update`: a chunk carrying both `"content": ""` and `tool_calls` used to classify as content, so `acc.JustFinishedContent()` returned true, the `if !ok` branch at `/clients/openai/chat_complete.go:392` was skipped, and the tool call was never yielded as a `gai.ToolCallPart`. v3.56.0 checks `len(delta.ToolCalls) > 0 && delta.Content == ""` first and yields it. That is a silent bug fix for OpenAI-compatible backends riding along in this bump, landing in the exact branch this client depends on.

### What was tricky

The awkward part was deciding how far to correct the `ThinkingLevel` block while the key was missing. Broadening the scope sentence was provably right, but adding a `gpt-6-astra` bullet was not: the block advertises its contents as "probed empirically against the live API", so appending a guessed row would have converted an omission into a false claim. I stated the gap outright instead and left the probing for whoever had the key — which turned out to be me, twenty minutes later, at which point the caveat came back out. Had I guessed, I would have guessed wrong: the obvious guess is that a newer model behaves like gpt-5.5, and gpt-6-astra does not.

Deciding whether to commit the unverified export took some thought, and I followed the precedent in `/docs/diary/2026-09-03-flash-38-drift.md`, where a builder hit this identical wall on `gemini-3.8-flash` five days ago and committed locally without pushing. Committing kept the mechanical work; not pushing kept an unverified claim out of a PR whose body would have asserted a fix.

### What warrants review

The disposition is now grounded, so the remaining judgement calls are documentation ones. The `ThinkingLevel` doc block still carries a caveat for the gpt-5.6 family, which remains the one exported family with neither bullet nor matrix rows; worth deciding whether that caveat is the right way to carry a known gap or whether 5.6 should simply be probed too. The claim that `gpt-6-astra` is "the only model that rejects both `none` and `minimal`" is true of the currently exported set and will need revisiting if the gpt-6 line grows.

The `wantThoughtTokens` choices on the new matrix rows are empirical and could flake if the model's behaviour drifts: `medium`, `high` and `xhigh` assert non-zero reasoning tokens, `low` does not. Repeated probes backed each one, but they are assertions about a remote model, not about this code.

### Future work

Two follow-ups fell out of the review and are not blocked on anything. `AddChunk`'s boolean return is discarded at `/clients/openai/chat_complete.go:373`, and v3.56.0 widened the set of conditions under which it rejects a chunk — choice index bounds, sparse tool-call index growth, byte and work budgets — so a dropped tool call or finish reason would now be silent; logging when it returns false would surface that. And the conformance test never asserts that an exported ID fails to match an ignore pattern, nor that any pattern ever matched something, so dead ignore entries can accumulate undetected; that assertion is pure and offline and needs no API key.

Two smaller ones. The gpt-5.6 trio still has no thinking-level coverage. And v3.53.0 added origin enforcement in the SDK's `internal/requestconfig/origin.go` that rejects redirects to a different scheme, host or port than the configured `BaseURL`; `/clients/openai/client.go` sets `BaseURL` for OpenAI-compatible endpoints, so the `llama32-1b` server in `/docker-compose.yaml` is worth one manual call if anyone relies on it.
