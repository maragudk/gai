# Diary: Relax the flaky `flash 3.7 + low` thoughts-token assertion

The `thinking level matrix` row for `gemini-3.7-flash` + `ThinkingLevelLow` in `/clients/google/chat_complete_test.go` asserts `wantThoughtTokens: true`, but the model now returns zero thoughts tokens in roughly 10% of runs. This flaked CI twice in a row on Dependabot PR #344 today, initially casting suspicion on the genai 1.68→1.69 bump.

## Step 1: Diagnose the flake as server-side and scope the relaxation

**Author:** main

### Prompt Context

**Verbatim prompt:** "Spawn a builder for 1" (referring to follow-up 1 in the lead's summary: "Relax the `flash 3.7 + low` matrix row (~10% zero-thought flake, precedent exists) — probe `medium`/`high` first to size it.")
**Interpretation:** Land the test relaxation for the `flash 3.7 + low` row, with a probe of `medium` and `high` first to decide how wide the relaxation must be.
**Inferred intent:** Stop the recurring CI flake without weakening assertions more than the observed nondeterminism requires, following the precedent set for the `+ none` row.

### What I did

During the Dependabot merge work earlier today, `flash 3.7 + low` failed "thoughts tokens should be populated" in 2 of 3 CI runs on PR #344 (genai 1.69.0) while passing 3 of 3 on genai 1.68.0, which looked like an SDK regression. An investigation agent diffed genai v1.68.0→v1.69.0 (nothing in the thinking path changed: `ThinkingConfig`, `ThoughtsTokenCount`, and stream aggregation are byte-identical; the sole `thinkingBudget` hunk is in the unrelated `ReinforcementTuningHyperParameters`) and ran an interleaved live A/B probe of 30 runs per version with the gai call shape: both versions produced exactly 3/30 zero-thought runs at `low`. Verdict: server-side nondeterminism at ~10%, the same phenomenon the test already documents for `flash 3.7 + none` ("0 in roughly one run in sixteen", commit `b9ac30d`), now reaching the `low` row. Probe artifacts (source plus `probe68`/`probe69` binaries) were left in the session scratchpad. Created worktree `flash-37-low-flake`, started this diary, and delegated the probe-then-relax change to a builder.

### Why

The assertion encodes "low always thinks", which was true at encoding time (2026-08-17, every probe populated thoughts tokens) but is false now; asserting a ~90% behavior plants a recurring CI flake. The investigator only probed `low`, so `medium` and `high` need the same measurement before deciding whether the relaxation covers one row or three.

### What worked

Interleaving the two SDK versions in one probe session controlled for time-varying server behavior and turned "2-of-3 vs 0-of-3 CI runs" from suggestive into conclusive: identical 3/30 rates.

### What didn't work

Nothing failed in this step; the initial SDK-regression hypothesis was simply wrong, and the A/B probe is what disproved it.

### What I learned

A live-API assertion that passed on every probe at encoding time can still rot as the provider's behavior drifts; the failure pattern (cluster of failures on one branch) can mimic a dependency regression when the branch is just the one running CI most often that day.

### What was tricky

Distinguishing the SDK bump from server drift required holding the merge of an otherwise-fine Dependabot PR until the A/B probe settled it — the CI sample (2-of-3 vs 0-of-3) was small enough to be pure luck, and was.

### What warrants review

After the builder's change: the `low` row keeps asserting the accept path and text parts, only the thoughts-token flag is dropped; the comment records the measured ~10% rate; `medium`/`high` rows are only touched if their probes show the same nondeterminism, with measured rates in the diary.

### Future work

The Vertex embed-quota 429 flake (five CI reruns today) is a separate follow-up, not yet scheduled. If Gemini 3.x thinking nondeterminism keeps spreading, the matrix's thoughts-token assertions may need a systematic policy rather than row-by-row relaxation.

## Step 2: Probe medium/high, relax only the `low` row

**Author:** flake-builder

### Prompt Context

**Verbatim prompt:** "Probe `flash 3.7 + medium` and `flash 3.7 + high` first to size the relaxation. Run each a meaningful number of times (at least 15-20 runs per level — the diary records that 4 probes once planted a flake) and record zero-thought counts. […] Relax the `flash 3.7 + low` row: drop `wantThoughtTokens: true`, keep the accept-path and text-part assertions, and update the row comment to record the measured ~10% zero-thought rate […]. Relax `medium`/`high` rows ONLY if your probes show zero-thought runs there too […]. Check whether the surrounding block comment […] needs its wording extended to cover `low`."
**Interpretation:** Measure the zero-thoughts-token rate at Low, Medium, and High on `gemini-3.7-flash` with the production call shape, then relax exactly the rows the measurement justifies and record the numbers in the comment.
**Inferred intent:** Kill the recurring CI flake with the narrowest possible weakening of the matrix, so the rows that still hold keep holding.

### What I did

Copied `/Users/maragubot/Developer/gai/.env.test.local` into the worktree root (confirmed gitignored via `git check-ignore -v .env.test.local` → `.gitignore:1:/.env.*.local`) and wrote a throwaway `/clients/google/probe_thinking_test.go` in `package google_test`, reusing the matrix's `newChatCompleter` helper and its exact request (same sheep prompt, `Temperature(0)`, `ThinkingLevel` pointer). Each level ran 25 completions through a five-worker pool, draining `res.Parts()` and recording `res.Meta.Usage.ThoughtsTokens` per run. Two full rounds, so 50 runs per level:

```
round 1: low zeroThoughts=1/25  medium 0/25  high 0/25
round 2: low zeroThoughts=0/25  medium 0/25  high 0/25
```

No errors and no zero-text runs in any of the 150 completions. Thoughts-token counts cluster cleanly by level — Low around 90-233, Medium around 158-327, High around 255-550 — so the levels are doing distinct work; Low is simply close enough to the floor that the server occasionally emits none. Combined with the lead's A/B probe (3/30 on each of genai 1.68.0 and 1.69.0), Low sits at 7 zero-thought runs in 110, and Medium/High at 0 in 50 each.

So the change is one row and one comment in `/clients/google/chat_complete_test.go`: `flash 3.7 + low` loses `wantThoughtTokens: true` (keeping the accept path via `is.NotError` and the shared `textParts > 0` assertion), and the Flash 3.7 block comment now covers both nondeterministic rows, records "roughly one run in sixteen" for None and "roughly one run in ten" for Low, notes the rate was identical on genai 1.68 and 1.69 so the source is the server rather than the SDK, and states that Medium and High never returned 0 across 50 probes each. Then deleted the probe file.

### Why

The task explicitly forbade relaxing Medium and High without evidence, and there is none: 100 completions across those two levels produced no zero-thought run. Naming the measured rates in the comment is what stops the next reader from re-tightening the row, which is exactly how this assertion got planted in the first place (Step 3 of `/docs/diary/2026-08-17-model-conformance.md`: four probes, all non-zero, encoded as always).

### What worked

Using the matrix's own helper and request shape meant the probe measured the thing under test rather than an approximation, and the five-worker pool brought 25 runs down to about 150 seconds per level, which made a second confirming round cheap. Reading `/docs/diary/2026-08-18-relax-thought-assertions.md` before starting turned out to be worth it: it records `flash 3.7 + low` failing `wantThoughtTokens` once in 9 runs on 2026-08-18, so this flake is three weeks old and was reported-but-not-fixed at the time, which corroborates the server-drift reading rather than anything about genai 1.69.

### What didn't work

The probe file failed to build first time — `vet: clients/google/probe_thinking_test.go:76:30: cannot use res.Meta.Usage.ThoughtsTokens (variable of type int) as int64 value in argument to append` — because I had guessed `int64` for the usage counter; it is `int`.

The full-package `go test -shuffle on ./clients/google` cannot complete in this worktree. Two subtests call `newVertexAIClientWithCredentials`, which **panics** rather than fails without `/vertex.json`: `panic: project/location or API key must be set when using Vertex AI backend`. A panic takes the whole test binary down, so the first one to run aborts every remaining test in the package — the known environment gap is not merely a red row, it hides the rest of the run. `-skip "service_account"` did not help, because a `-skip` pattern without a slash only matches top-level test names; `-skip "/.*service_account"` is what reaches subtests. With those skipped, `TestEmbedder_Embed/can_embed_a_text` hit the other known issue — `Error 429 […] Quota exceeded for aiplatform.googleapis.com/global_embed_content_requests_per_minute_per_base_model` — and a third Vertex-credential subtest (`can embed a text with Embedding 2 on Vertex AI backend`, `/clients/google/embed_test.go:146`) panicked again. Running `go test -shuffle on -count=1 ./clients/google -skip "/.*service_account|^TestEmbedder_Embed$"` finally gave `ok maragu.dev/gai/clients/google 161.141s`, covering the chat completer and model conformance. Neither failure is related to this change.

### What I learned

`go test -skip` and `-run` split their pattern on `/` and match each element against the corresponding test-name element, so a bare word never matches a subtest — it is silently interpreted as a top-level test name and the subtest runs anyway. Also worth knowing: a panicking test helper is much more expensive than a failing one in a package whose tests are otherwise live and green, because it costs the entire package's signal.

### What was tricky

Choosing the sample size. The 2026-08-17 diary records exactly this trap — four clean probes encoded as an invariant — and a single round of 25 clean runs at Medium would have been only a slightly larger version of the same mistake. Two independent rounds separated in time, on top of the lead's interleaved A/B, is what makes "Medium and High keep the assertion" a measurement rather than a hope. Even so, 0/50 bounds the rate at roughly 6% with 95% confidence, not at zero, so these rows are plausible future flakes rather than proven-safe ones.

### What warrants review

The judgement call is that Medium and High stay strict. The diff is six comment lines and one dropped struct field in `/clients/google/chat_complete_test.go`; there is no production-code change, and `/clients/google/chat_complete.go`'s thinking-level GoDoc needed no edit because it only documents the None edge and the Minimal rejection, never claims Low always thinks. Validation: the matrix subtest ran green live twice (85s and 247s), `golangci-lint run` reports `0 issues`, and `gofmt`/`go vet` are clean.

### Future work

If Medium or High ever flakes, the fix is the same one-line relaxation, but at that point the pattern (three of five 3.7 rows nondeterministic) argues for the systematic policy the lead already flagged — perhaps a tolerance-based assertion that samples a row twice and accepts a single zero, rather than dropping the assertion entirely and losing the signal. The `/clients/google` package would also benefit from `newVertexAIClientWithCredentials` skipping via `t.Skip` when `/vertex.json` is absent instead of panicking, so a missing local credential file costs one row instead of the package.
