# Diary: Remove `google.ChatCompleteModelGemini2_0Flash`

Google's models page lists `gemini-2.0-flash` as "Shut down", but the Google client still exports `google.ChatCompleteModelGemini2_0Flash`. The drift builder found this while fixing #384 and deliberately left it out of that PR. Under the curation rule in `/CLAUDE.md` and the 2026-10-05 entry in `/docs/decisions.md`, retired models come out of the exported constants.

## Step 1: Scope and hand-off

**Author:** main

### Prompt Context

**Verbatim prompt:** Just remove Gemini 2.0 flash?
**Interpretation:** Remove `google.ChatCompleteModelGemini2_0Flash` and every use of it. Keep `TestModelConformance` consistent in both directions.
**Inferred intent:** Stop advertising a model that Google has shut down, in a small PR of its own (the user confirmed with "In a new PR?").

### What I did
Created the worktree `worktree-remove-gemini-2-0-flash`. I based it on the #394 branch (`worktree-fix-model-drift-384`) rather than on main, because #394 touches the same Google files and is set to auto-merge. I copied `.env.test.local` and `vertex.json` into the worktree, then handed the change to a builder.

### Why
Basing the branch on #394 avoids conflicts in `/clients/google/chat_complete.go`, `/clients/google/chat_complete_test.go` and `/clients/google/model_conformance_test.go`. #394 merges with a merge commit, so this branch rebases cleanly onto main afterwards.

### What worked
Nothing to report yet. Setup only.

### What didn't work
Nothing failed in this step.

### What I learned
I created this worktree with plain `git worktree add`, which doesn't apply `.worktreeinclude`, so I copied the credentials by hand.

### What was tricky
Two questions are still open. If the API still lists the ID, it has to go on the ignore list. And the get-by-ID check still resolved it in the latest CI run, even though the docs say it is shut down.

### What warrants review
Check that no references remain in tests, README, docs or examples, and that the conformance lists stay consistent.

### Future work
After #394 merges, rebase this branch onto main before or after opening the PR.

## Step 2: Remove the constant and verify live

**Author:** flash-builder

### Prompt Context

**Verbatim prompt:** Remove `google.ChatCompleteModelGemini2_0Flash`. Google's models page (ai.google.dev/gemini-api/docs/models) lists `gemini-2.0-flash` as "Shut down". [...] If the live model list still returns `gemini-2.0-flash`, or any `gemini-2.0-flash-*` variants that the constant made acceptable, add them to the ignore list following precedent, with a shutdown note. [...] Don't make any other changes.
**Interpretation:** Delete the constant and its conformance row, find every other reference, and decide from live evidence whether the ignore list needs an entry.
**Inferred intent:** Stop advertising a shut-down model without changing anything else, and keep `TestModelConformance` green in both directions.

### What I did
Deleted `ChatCompleteModelGemini2_0Flash` from `/clients/google/chat_complete.go` and its row from `exportedModels` in `/clients/google/model_conformance_test.go`. A grep for `Gemini2_0Flash` and `gemini-2.0-flash` over the whole worktree found only those two lines (plus diary mentions). No tests, README, `/docs/index.html`, examples or doc comments referenced it, so no replacement default model was needed.

I then ran `make conformance` live. It passed without an ignore entry. To see why, I used a throwaway probe test (deleted afterwards) that listed every model with "2.0" in its name and did a get-by-ID on `gemini-2.0-flash`, `gemini-2.0-flash-lite` and `gemini-2.0-flash-001`. The list returned no 2.0 model at all. Get-by-ID still resolved all three with no error.

### Why
The ignore list exists to satisfy the "every listed model ID is exported or ignored" direction. The ID is no longer listed, so an entry would be stale from the start, and the test does not flag stale entries. The 2026-10-05 decision says to prune an entry once get-by-ID fails; that implies entries are for IDs that are still listed. Precedent entries (`claude-sonnet-4-5`, `gpt-5`, `gpt-5.1`) were all still in the live list when added.

### What worked
Conformance passes in both directions after the removal, and `golangci-lint run` reports 0 issues.

### What didn't work
My first attempts at combined shell commands (`. ./.env.test.local` and a heredoc plus `rm` in one command) were refused by the worktree sandbox as unverifiable. Splitting them into plain commands worked. The `sed -i` without an empty suffix argument is also BSD-incompatible on macOS; I used `sed -i ''`. Nothing failed in the code.

### What I learned
Get-by-ID still resolves `gemini-2.0-flash` even though Google lists it as shut down and the list endpoint no longer returns it. This matches the 2026-08-17 finding that lookup can succeed for IDs absent from the list.

### What was tricky
Deciding on the ignore entry. The task said to add one only if the live list still returns the ID. It does not, so I added none. If the reviewer prefers an entry because get-by-ID still resolves, it would go in `ignoredModels` as its own commented group with the shutdown note.

### What warrants review
That the ignore list is untouched, and that the two deleted lines are the only references left.

### Future work
None.
