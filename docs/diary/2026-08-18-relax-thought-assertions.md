# Diary: Relax the nondeterministic google thought-part assertions

The google `thinking level matrix` rows assert that Gemini streams `PartTypeThought` parts at given thinking levels. That assertion has failed five times in two days across three different rows (`flash-lite 3.1 + high` twice on 2026-08-17, `pro 3.1 + medium` twice and `pro 3.1 + high` once on 2026-08-18), each time passing on rerun or being contradicted by an adjacent run -- most CI runs across PRs #333-#336 needed a manual rerun because of it. Google has evidently made thought-summary *streaming* nondeterministic across the 3.x generation, the same behavior the 2026-08-18 default-test-models probe found on 3.7 Flash, where the assertion was made conditional for exactly this reason. Markus first chose to leave the assertions strict (two flakes, one row); with five occurrences on three rows he approved this fix as its own small PR.

## Step 1: Track the flake class and approve the relaxation

**Author:** main

### Prompt Context

**Verbatim prompt:** "Fix in a tiny separate PR" (Markus's answer to the revisit question, options: tiny separate PR / fold into #336 / keep rerunning)
**Interpretation:** Relax the strict thought-part assertions on the affected rows in a dedicated small PR, keeping PR #336's reviewed diff untouched.
**Inferred intent:** Stop burning a manual CI rerun per PR on assertions that no longer reflect provider behavior, without weakening what the matrix genuinely verifies (API accept/reject per level).

### What I did

Tracked the failure class across CI runs (PR #333's first run, PR #336's three runs, plus a local baseline sighting recorded in the default-test-models diary), confirmed each occurrence was the same `should stream PartTypeThought parts` assertion at `/clients/google/chat_complete_test.go:644` on rows that pin their own models, and confirmed with reruns that the same row passes on retry. Asked Markus to revisit his earlier "leave it" call with the updated tally; he approved the separate-PR fix. Created worktree `relax-thought-assertions`, started this diary; the change goes to a builder next.

### Why

An assertion that fails on provider whim is a tax, not a check: it had us manually rerunning nearly every CI run of the day, and normalizing reruns trains everyone to ignore red -- which is how a real regression slips through.

### What worked

Keeping a running tally per row across PRs turned "feels flaky" into evidence that changed the earlier decision -- two flakes on one row was tolerable, five on three rows was a class.

### What didn't work

Two reruns of PR #336's Test job in a row failed on the same class (`pro 3.1 + high`, then `pro 3.1 + medium`), the first time a rerun did not clear it -- the strongest signal the nondeterminism rate has increased.

### What I learned

Streamed thought summaries and billed thought tokens are separate signals: the probe showed `thoughtsTokens > 0` assertions behaving deterministically on 3.5/3.6 models while streamed summary parts vary run to run. Relaxations should drop the part-streaming requirement, not necessarily the token accounting.

### What was tricky

Distinguishing "provider changed behavior" from "our PR broke thought streaming": PR #336 touches google stream-read, but the failing rows predate it, pin their own models, and failed identically on PR #333 before any of this code existed.

### What warrants review

The relaxed rows should still assert the call succeeds and keep any stable assertions (level accept/reject, thought-token accounting where the probe showed it deterministic); comments should say "usually streams" rather than promise. The matrix must still fail if a level that should be rejected is accepted.

### Future work

If Google's behavior settles (or a `thought_summary` request knob appears), the strict assertions can return; the tally method here is the evidence pattern to reuse.

## Step 2: Relax the three flagged rows, and find a second flake class along the way

**Author:** relax-builder

### Prompt Context

**Verbatim prompt:** Requirements handed off by the lead: "In `clients/google/chat_complete_test.go`, for every matrix row on a Gemini 3.x model that strictly requires streamed thought parts: drop the required streamed-thought-part assertion, following the existing 3.7 Flash precedent... Keep intact: accept/reject assertions per thinking level..., thought-token-accounting assertions (`thoughtsTokens > 0` style) where they exist... do NOT relax them unless you personally observe one fail across runs, in which case report it rather than silently relaxing."
**Interpretation:** Remove `requireThoughts: true` from the three rows named in the task (`flash-lite 3.1 + high`, `pro 3.1 + medium`, `pro 3.1 + high`), soften the comments above them, leave every `wantThoughtTokens` assertion untouched, and stay inside this one test file.
**Inferred intent:** Stop the CI reruns caused by this specific flake class without quietly widening the change into other assertions that were still believed reliable.

### What I did

Confirmed the three flagged rows in the matrix table (`/Users/maragubot/Developer/gai/.claude/worktrees/relax-thought-assertions/clients/google/chat_complete_test.go:554,604,605`) were exactly `flash-lite 3.1 + high`, `pro 3.1 + medium`, and `pro 3.1 + high` — matching the diary's tally. Dropped `requireThoughts: true` from all three struct literals, leaving `wantThoughtTokens: true` in place on each. Rewrote the two comment blocks directly above them (originally claiming High "does emit" a thought part on Flash Lite 3.1, and Medium/High are "reliable" on Pro 3.1) to describe the sporadic behavior in past tense with the flake counts, matching the em-dash style already used elsewhere in the file (the codebase consistently uses "—", not "--", for this kind of aside — I used "--" in my first pass and corrected it on self-review). Copied `/Users/maragubot/Developer/gai/.env.test.local` into the worktree root (already covered by `/.env.*.local` in `.gitignore`) so the live-key tests could run. `go build ./...` and `go vet ./clients/google/...` were both clean throughout.

### Why

The three rows had genuinely flaked five times on `PartTypeThought` streaming per the Step 1 tally; the fix drops only that specific assertion on only those rows, so the matrix still fails loudly if a level starts being accepted/rejected incorrectly or if thought-token accounting regresses.

### What worked

Running the isolated `thinking level matrix` subtest repeatedly (`go test -count=1 -shuffle on ./clients/google -run 'TestChatCompleter_ChatComplete/thinking_level_matrix' -skip '/(service_account|Embedding_2_on_Vertex)' -v`) is fast enough (~60s/run) that running it nine times, plus a full-package run, was cheap. Across all nine matrix runs the three relaxed rows never failed once — the fix does what it's supposed to.

### What didn't work

Run 2 of 9 failed, but not on any row I touched: `flash_3.7 + low` failed its `wantThoughtTokens` assertion with `thoughts tokens should be populated` / `thoughtsTokens=0`. Run 4 of 9 also failed the overall test, but I lost the specific failing subtest name to a grep filter that only matched `--- FAIL` lines for a target-row allowlist plus the top-level summary — the nested per-subtest `--- FAIL` line never appeared, which I can't fully explain (possibly the run failed by some other route, e.g. a transient network error rather than a false assertion; I didn't capture enough to say). Runs 1, 3, 5, 6, 7, 8, 9 were all fully green, including the touched rows. I did not relax `flash_3.7 + low`'s `wantThoughtTokens` assertion or any other token-accounting assertion — per the task's explicit instruction, I'm reporting this instead: **`thoughtsTokens > 0` is not as deterministic as Step 1's probe suggested; it flaked at least once (confirmed) and possibly twice (unconfirmed) in 9 runs today**, on a row (`flash_3.7 + low`) outside the three this PR touches.

### What I learned

The `requireThoughts` struct field and its `if test.requireThoughts { ... }` branch are now dead in the sense that no row sets it to `true` anymore, but the field is still read, so nothing flags it as unused. I left it in place rather than deleting it, since the diary's "Future work" section says the strict assertions may return once Google's behavior settles — deleting the plumbing now would just mean re-adding it later. This is a judgment call the lead should double check they're happy with; it was small enough not to seem worth a separate decision entry.

### What was tricky

Distinguishing "this run failed because of the flake class I'm fixing" from "this run failed because of something else" in real time — the second failure class (`thoughtsTokens` on `flash_3.7 + low`) surfaced by surprise on run 2 while I was still gathering the three required green runs for verification. Rather than chase down whether it was a fluke or shrug it off, I logged full output to scratchpad files for the later runs and kept going until I had a large enough sample (9 runs) to be confident the three rows I changed are solid, while treating the new flake as its own separate finding rather than something to fix in this PR.

### What warrants review

The lead should look at the two comment rewrites above the `flash-lite 3.1` and `pro 3.1` row blocks in `/Users/maragubot/Developer/gai/.claude/worktrees/relax-thought-assertions/clients/google/chat_complete_test.go` to confirm the tone/wording fits, and decide whether the `flash_3.7 + low` `wantThoughtTokens` flake (and the unidentified run-4 failure) warrants its own tally-and-fix cycle like this one did, or is a single fluke worth watching. I did not touch that assertion.

### Future work

If the `flash_3.7 + low` `thoughtsTokens` flake repeats, it's the same evidence-gathering pattern from Step 1: track occurrences across CI runs, and if it becomes a class, bring it back to Markus as a follow-up rather than relaxing it unilaterally.
