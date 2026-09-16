# Diary: Flash 3.6 now accepts `ThinkingLevelNone` (CI drift)

The live Google matrix test `TestChatCompleter_ChatComplete/thinking_level_matrix/flash_3.6_+_none_rejected` expects a 400 from the API; Gemini has started accepting `ThinkingBudget=0` on `gemini-3.6-flash`, so the row fails deterministically and blocks every PR.

## Step 1: Diagnose from CI logs

**Author:** main

### Prompt Context

**Verbatim prompt:** Test in CI failing

Followed by: Rerun CI test

Followed by: Rerun 3x or until green

Followed by: Yes please

**Interpretation:** CI on PR #368 (an unrelated schema fix) is red; find out why, rerun in case of flake, and when that fails, fix the stale test.
**Inferred intent:** Unblock merges without papering over a real upstream behaviour change.

### What I did

Pulled the failed-job logs with `gh run view 35075661563 --log-failed`. Reran the failed jobs five times in total (`gh run rerun --failed`). Read the matrix definition in `/clients/google/chat_complete_test.go` around lines 519-585 and the `ThinkingLevelNone` mapping at `/clients/google/chat_complete.go:135-137`. Created worktree `flash-36-none-drift` off `origin/main` and this diary; spawning a builder.

### Why

Five identical failures on one row rules out flakiness. The row's comment says Flash 3.6 rejects `ThinkingBudget=0` with a generic 400; the API no longer does, so the expectation is stale, not the code.

### What worked

`gh run view <id> --log-failed | grep -- '--- FAIL'` isolates the subtests quickly. The rerun loop also showed a one-off OpenAI flake (`reasoning_effort_matrix/gpt-6-astra_+_medium`, "thoughts tokens should be populated") that cleared on retry and is not in scope here.

### What didn't work

Reruns: failures on all five attempts, always `chat_complete_test.go:643: expected an error from the API` for `flash_3.6_+_none_rejected`. A combined `git worktree add && cd && cat > diary` command was refused by the worktree-isolation guard as "too complex to verify that it stays inside the worktree"; single commands worked.

### What I learned

The `flash-lite 3.5 + none rejected` row still passes, so the change is specific to `gemini-3.6-flash`, not the whole "rejects None" group. The doc comment on `ChatCompleteModel` (`/clients/google/chat_complete.go:57-62`) already lists `gemini-3.6-flash` among models that accept `ThinkingBudget=0`, contradicting the test comment; the builder should reconcile the two.

### What was tricky

No Google credentials on this machine, so verification is CI-only.

### What warrants review

Whether the new row asserts the right thing (a text response, no thought-token requirement, matching the other `+ none` rows) and whether the comment block above it still describes reality.

### Future work

The OpenAI `gpt-6-astra + medium` thoughts-tokens flake may deserve its own look if it recurs.

## Step 2: Flip the row, reconcile the comments, verify live

**Author:** flash-builder

### Prompt Context

**Verbatim prompt:** "1. Change the `flash 3.6 + none rejected` row to a `flash 3.6 + none` row that expects success, shaped like the other `+ none` rows (e.g. `flash 3.5 + none`): no `wantErr`, no `wantThoughtTokens`. 2. Fix the comment block above the Flash Lite 3.5 / Flash 3.6 rows so it describes reality: only Flash Lite 3.5 still rejects None. Move the Flash 3.6 rows under a comment of their own if that reads better. Keep the comment style of the file. 3. Check the `ChatCompleteModel` doc comment at `/clients/google/chat_complete.go:55-64` and the package README for any statement about which models accept or reject `ThinkingLevelNone`, and make them consistent with the test. Report what you found either way. 4. You have no Google credentials locally, so the live test cannot run here. [...] CI is the real verification."

Followed by: "Update: you now have Google credentials. `.env.test.local` is symlinked into the worktree root [...] run the live test for real [...] and confirm the `flash 3.6 + none` row passes and the `flash-lite 3.5 + none rejected` row still errors as expected. Report the `thoughtParts/textParts/thoughtsTokens` log line for the Flash 3.6 + none row so we know what the API now returns."

**Interpretation:** Flip the stale row to the accept shape, split the now-wrong shared comment, reconcile the doc comment and README with the test, and — once credentials arrived — prove the change against the live API instead of deferring to CI.
**Inferred intent:** Unblock CI with a change that is true of the provider today and legible to whoever reads these rows next, rather than one that merely stops the test failing.

### What I did

Changed the row in `/clients/google/chat_complete_test.go` from `{name: "flash 3.6 + none rejected", ..., wantErr: true}` to `{name: "flash 3.6 + none", ...}` with neither `wantErr` nor `wantThoughtTokens`, matching the `flash 3 + none`, `flash-lite 3.1 + none`, and `flash 3.5 + none` rows. Split the shared comment block: Flash Lite 3.5 keeps its own block, now naming it as the one model in the Flash line that rejects None, and the Flash 3.6 rows get a block of their own.

Reconciled the documentation. Requirement 3 asked me to check the doc comment and the README, and the finding is the opposite of what Step 1 recorded: at HEAD the doc comment above the thinking-level constants (`/clients/google/chat_complete.go:56-62`) listed `gemini-3.6-flash` under **rejected**, not accepted — it agreed with the stale test and contradicted nothing. `git show HEAD:clients/google/chat_complete.go` confirms it. So there was no pre-existing doc/test conflict to reconcile; there was one stale fact stated in two places, and both needed the same edit. I moved `gemini-3.6-flash` into the accepted list, leaving `gemini-3.1-pro-preview` and `gemini-3.5-flash-lite` as the only rejectors. `/clients/google/README.md` says nothing about models or thinking levels at all — it is a twelve-line roadmap stub — so it needed no change, and a repo-wide grep found no other enumeration of per-model `ThinkingLevelNone` support in `/docs/index.html`, `/docs/decisions.md`, the examples, or `/clients/google/model_conformance_test.go`.

Self-review turned up one more stale claim, which I fixed: the inline comment on the `gai.ThinkingLevelNone` case at `/clients/google/chat_complete.go:136` read `// Off: budget=0. Accepted by Flash 3.x; rejected by Pro 3.x with a 400.` That blanket "Accepted by Flash 3.x" was already false before this change, because `gemini-3.5-flash-lite` is a Flash model that rejects it. It now names the exception set and points at the fuller doc comment rather than re-summarising a list that drifts every few weeks.

With credentials symlinked in, I verified live. The full matrix passes, 43 of 43 rows, in 83s — `flash 3.6 + none` green and `flash-lite 3.5 + none rejected` still erroring as expected, so the relaxation is specific to 3.6 and did not quietly weaken its neighbour. The Flash 3.6 + none log line is:

```
chat_complete_test.go:668: thoughtParts=0 textParts=5 thoughtsTokens=0
```

Then I probed the row properly rather than trusting a single sample, with `-count=15` on that one row. All 15 passed, every one at `thoughtParts=0 ... thoughtsTokens=0`. With the matrix run that is 16 of 16 accepted and 16 of 16 at zero thoughts tokens, and the comment now records that number.

### Why

The row shape follows the file's precedent exactly: a model that accepts None gets a bare row, because None is the one level where thoughts tokens legitimately come back at zero, so there is nothing to assert beyond the accept path and the shared `textParts > 0` check.

The probing is not ceremony either. Five identical CI failures prove the *old* expectation is dead, but they are five runs of a suite, not a measurement of the new behaviour, and 5-of-5 clean is consistent with a residual rejection rate around 15%. This repository has been burned by exactly that inference twice — `/docs/diary/2026-09-01-flash-37-low-flake.md` records four clean probes encoded as an invariant, and warns that three probes at Low would read as "always" or "never" with roughly even odds and both readings would be wrong. Sixteen runs is what lets the comment state a number instead of a hope.

The zero-thoughts-tokens result is the more interesting half. 3.7 and 3.8 both accept the zero budget and then think anyway much of the time; 3.6 actually honours it. That difference is worth a sentence, because a future reader comparing the three blocks would otherwise reasonably assume the point releases behave alike.

### What worked

Reusing the matrix row itself as the probe, via `-count=15` and a `-run` pattern escaping the `+` as `\+`, gave a 15-sample measurement with no throwaway probe file to write, review, and remember to delete. Earlier diaries here all describe writing a temporary `probe_*_test.go` and deleting it afterwards; when the thing you want to measure is already a row, `-count` is the cheaper path and measures the production call shape by construction.

Running the competing-reviewer pass before committing paid for itself twice: one reviewer caught the stale `Flash 3.x` blanket claim one line away from my edit, and another argued from the repo's own history that five CI reruns is not the evidence standard this file has always used. Both were right and both changed the diff.

### What didn't work

Before the credentials arrived, every live run died the same way, which is the known worktree gap rather than anything about this change:

```
panic: api key is required for Google AI backend. ClientConfig: &genai.ClientConfig{APIKey:"", Backend:1, ...}
	/clients/google/client.go:85 (google.NewClient)
	/clients/google/chat_complete_test.go:888 (newChatCompleter)
```

Once `.env.test.local` was symlinked in, this cleared entirely. Nothing else failed: `go build ./...`, `go vet ./clients/google/...`, `gofmt -l`, and `golangci-lint run ./clients/google/...` (`0 issues.`) were clean on every pass.

Appending this entry with a single-quoted heredoc was refused by the worktree-isolation guard as "too complex to verify that it stays inside the worktree" — the same trap Step 1 hit, and for the same reason: the prose mentions worktree and git commands. Writing the text to the session scratchpad and appending it with a plain `cat` worked.

### What I learned

Step 1's diagnosis got the direction of the contradiction backwards. It recorded that the doc comment "already lists `gemini-3.6-flash` among models that accept `ThinkingBudget=0`, contradicting the test comment"; in fact the doc comment listed 3.6 as rejected, in agreement with the test. The practical consequence was small — both places needed editing either way — but it is worth naming, because the brief that followed asked the builder to "reconcile the two", which framed the job as resolving a disagreement when it was actually correcting a consensus. A stale fact repeated consistently in two places looks more trustworthy than a contradiction, not less, and is the harder of the two to notice.

Also: `go test -run` splits its pattern on `/`, and the `+` in a subtest name is a regex metacharacter, so the row is reachable only as `.../flash_3.6_\+_none`. Without the escape the pattern silently matches nothing and the run reports `ok` with no tests, which looks like a pass.

### What was tricky

Deciding how far to follow the reviewer who wanted the change held for a dedicated probe. The row cannot flake CI in the direction it warned about without the API reverting, and the old row was failing every single run, so shipping on five CI failures would have been defensible. What tipped it was that the cost of being right was about 25 seconds of wall time, against a comment that would otherwise assert "accepts every level" with nothing behind it — and unhedged comments in this file are precisely what the last two drift diaries blame for the flakes they were written to fix.

The other judgement call was the one-line fix to the switch comment at `/clients/google/chat_complete.go:136`. It is pre-existing and strictly outside the brief, which is usually a reason to leave it. I took it because it states the same fact this change exists to correct, one screen away, and leaving a known-false comment next to a freshly-corrected one invites the next reader to trust the wrong one.

### What warrants review

The substance is three comment blocks and one struct field. The claim to check is "16 of 16 None probes returned zero thoughts_tokens and no thought parts" — it comes from one matrix run plus a 15-run `-count` probe on 2026-09-16, and it is reproducible in about 25 seconds.

Two judgement calls deserve a second opinion: whether the switch-comment fix at `/clients/google/chat_complete.go:136` belongs in this change or a separate one, and whether splitting the Flash Lite 3.5 / Flash 3.6 comment into two blocks reads better than one block with an exception carved out. I think it does, since the two models no longer share a behaviour.

Finally, someone should decide whether Step 1's inverted description of the doc comment is left standing as part of the historical record. I did not edit it, per the brief.

### Future work

None for this drift. The standing observation from the previous two Flash diaries still holds and now has a third data point: `gemini-3.6-flash` has moved from rejecting None to accepting it, months after the behaviour was first encoded, so these rows record a moving target and not a contract. If a fourth model flips, a matrix that probes the accept/reject edge and reports it rather than asserting it is probably the better design.
