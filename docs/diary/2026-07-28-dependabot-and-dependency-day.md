# Diary: Dependabot cleanup, govulncheck fixes, and new model constants

A dependency-maintenance session that started with "any open PRs?" and ended with the nine-PR dependabot backlog at zero, a green security workflow, new model constants for Claude Sonnet 5 and the GPT-5.6 family, and a dependabot config that only proposes updates worth reading. Along the way: an investigation into a dependabot regression, and a lesson in how govulncheck's static call graph works.

## Step 1: Triage the open PR backlog and merge the checkout bump

**Author:** main

### Prompt Context

**Verbatim prompt:** "Any open PRs?" followed by "Merge 314"
**Interpretation:** List the repository's open PRs, then merge the actions/checkout major bump.
**Inferred intent:** Start working down the accumulated dependabot backlog.

### What I did

Listed nine open PRs, all dependabot: six Go module bumps and three GitHub Actions bumps. Checked CI on #314 (actions/checkout 6 → 7.0.0), then merged it. First tried `gh pr merge 314 --squash`, which the repo rejected; retried with `--merge`, which matched the merge-commit style visible in the history.

### Why

The user asked; #314 was green (Test and govulncheck passing, Lint/Evaluate skipped as usual on dependabot PRs).

### What worked

Checking CI before merging, and inferring the merge style from the commit history (`<PR title> (#NNN)` merge commits).

### What didn't work

The squash attempt: `GraphQL: Squash merges are not allowed on this repository. (mergePullRequest)`. Harmless, but worth remembering — this repo is merge-commit only.

### What I learned

Repo merges use `--merge` with the PR title as the merge commit title.

### What was tricky

Nothing.

### What warrants review

Nothing — a plain dependency merge.

### Future work

The remaining eight PRs, handled in later steps.

## Step 2: Only major version bumps for actions

**Author:** main

### Prompt Context

**Verbatim prompt:** "I only want major version bumps for actions" — with mid-turn follow-ups "I think that's already configured? There was a depandabot bug" and "We can close 303 and 312"
**Interpretation:** Suppress dependabot PRs for minor/patch GitHub Actions bumps; verify whether that was already configured; close the two unwanted PRs.
**Inferred intent:** Stop the recurring noise of `6 → 6.5.0`-style PRs against workflows that pin major tags.

### What I did

Checked the full git history of `/.github/dependabot.yml` (never had ignore rules, only cooldown changes) and searched every PR comment in the repo for `@dependabot` commands (only rebases; the "OK, I won't notify you again" replies are close-acknowledgments, not ignore commands). Closed #303 (golangci-lint-action 9 → 9.2.1) and #312 (setup-go 6 → 6.5.0). Added an `ignore` rule for the `github-actions` ecosystem covering `version-update:semver-minor` and `version-update:semver-patch`, shipped as PR #320.

### Why

The workflows pin bare major tags (`@v6`), which already float minors and patches — minor-bump PRs are pure noise. The history check mattered because the user believed it was already configured; it wasn't, anywhere.

### What worked

Searching PR comments via `gh api repos/.../issues/comments --paginate` settled the "already configured?" question definitively. Also: closing a dependabot PR only auto-ignores that *exact* release, which explained why closing #275 (9.2.0) didn't prevent #303 (9.2.1).

### What didn't work

Nothing failed in this step.

### What I learned

The "dependabot bug" the user remembered is real: dependabot proposes precision-changing bumps (`@v6` → `@v6.5.0`) for major-tag-pinned actions. Upstream it's dependabot-core issue #5887, closed in 2022 (root cause then: a timing gap before the major tag moved), with an unanswered June 2026 comment reporting a regression. The `ignore` + `update-types` config works regardless of the bug, because it acts on the computed update type.

### What was tricky

Distinguishing dependabot's comment types: the "OK, I won't notify you again" message looks like a response to an ignore command but is actually posted on any manual close.

### What warrants review

`/.github/dependabot.yml` — the ignore rule is four lines; check the `update-types` values if actions minor PRs ever reappear.

### Future work

Optionally +1 the regression on dependabot-core #5887 — this repo was a clean public reproduction.

## Step 3: Is the bug fixed? Evidence from #314 vs #319

**Author:** main

### Prompt Context

**Verbatim prompt:** "Is the dependabot bug still open?", "I don't think we've seen the issue since June?", "merge 320" (with mid-turn "Hmm, see 319. Maybe the bug is fixed after all"), and "Merge 319"
**Interpretation:** Establish the upstream bug status, check whether this repo was still affected, and merge the config PR plus the new setup-go major bump.
**Inferred intent:** Decide whether the config change was still needed and keep the backlog moving.

### What I did

Verified upstream: #5887 closed October 2022 "completed", follow-up #5891 also closed, no open issue tracking the June 2026 regression (searched open issues including everything labeled `github:actions`). Disproved "not since June": #312 (July 21) rewrote `@v6` → `@v6.5.0` a month after v6.5.0's release, with the `v6` tag pointing at the same commit as `v6.5.0` — the regression, not the benign timing gap. Then compared #314 (July 21, titled "from 6 to 7.0.0") against #319 (July 28, titled "from 6 to 7"): both diffs preserve precision (`@v6` → `@v7`), so #319 isn't proof of a fix, but the title format change hints dependabot now resolves the major tag rather than the release. Merged #320 and #319.

### Why

The user was reconsidering the config change based on #319. The evidence showed the config was still justified: the bug was demonstrably active on July 21, the possible fix is unconfirmed upstream, and the ignore rule is a no-op if the fix is real.

### What worked

Comparing tag SHAs (`gh api repos/actions/setup-go/git/ref/tags/v6` vs `v6.5.0`) to rule out the stale-major-tag explanation. Comparing the two PRs' *diffs* rather than their titles — titles use full version numbers regardless of what the diff pins.

### What didn't work

Several `gh search issues` queries for an open regression issue returned nothing — either it doesn't exist or the search terms missed it. Recorded as "no dedicated open issue found", not "none exists".

### What I learned

Dependabot PR titles and diffs can disagree: #314's title said "7.0.0" while its diff wrote `@v7`. Never infer the pin style from the title.

### What was tricky

#303's case remains genuinely ambiguous — golangci-lint-action's `v9` tag pointed at a different commit than `v9.2.1` at check time, so that one may have been the benign path. The setup-go case carried the argument alone.

### What warrants review

Nothing in-repo; the analysis is in this diary and the conversation.

### Future work

Watch whether any minor-bump action PR ever appears again — with the ignore rule it shouldn't, regardless of the upstream fix.

## Step 4: Make govulncheck green

**Author:** main

### Prompt Context

**Verbatim prompt:** "Okay, let's do govulncheck now" and later "Why was grpc reachable through eval.Run?"
**Interpretation:** Fix the failing govulncheck CI job on main, then explain one of its reachability traces.
**Inferred intent:** Clear the two real advisories so the scheduled security workflow stops failing.

### What I did

Extracted the full vulnerability list from the latest main run: GO-2026-6061 (grpc, found in v1.80.0, fixed in v1.82.1, reachable via `/eval/run.go`) and GO-2026-5970 (x/text infinite loop, found in v0.37.0, fixed in v0.39.0, reachable via the openai streaming path in `/clients/openai/chat_complete.go`). Merged #318 (grpc 1.82.1). Bumped x/text with `go get golang.org/x/text@v0.39.0 && go mod tidy` (x/sync came along to v0.21.0) on a branch, ran the test suite, opened PR #324, watched its govulncheck pass with zero reachable vulnerabilities, merged.

For the "why reachable" question: `go mod why -m google.golang.org/grpc` shows the chain `clients/google → genai → cloud.google.com/go/auth/httptransport → gax-go → grpc`. The traces flow through indirect-call funnel points — every subtest funnels through one `fn(t)` call site inside `testing.T.Run`, and every `sync.Once.Do` in the program funnels through the same `f()` in `sync.(*Once).doSlow` (which is how `eval.E.Log`'s file-cleanup Once at `/eval/run.go:115` got connected to grpc's shutdown Once). Conservative over-approximation, but not pure noise.

### Why

Main's security workflow had been red since the two advisories were published (last three runs on main all failures). Both fixes were minimal version bumps; x/text needed a manual bump because dependabot doesn't propose indirect-dep updates for it.

### What worked

Using the PR's own govulncheck run as the verification instead of a local run (per the standing note that local govulncheck is buggy). Merging #318 first so the x/text branch was based on a main with the grpc fix already in.

### What didn't work

The local test suite failed in `clients/openai` with `401 Unauthorized — "Incorrect API key provided: sk-proj-…Ct8A"` from the real OpenAI API. Not related to the change: the local `OPENAI_API_KEY` is expired or revoked. Every other package passed; CI's Test job (which passed) was the arbiter.

### What I learned

GitHub's "13 vulnerabilities (7 critical)" push banner counts module-level dependabot alerts; govulncheck's reachability analysis is the meaningful signal, and it confirmed the code calls none of the other flagged modules. The two metrics disagree by design.

### What was tricky

Explaining the reachability trace honestly: `eval.E.Log` was never going to close a grpc connection, but "an eval using the google client can execute grpc transport code" is true enough that the gate firing is correct behavior, not a false alarm to suppress.

### What warrants review

`/go.mod` and `/go.sum` after #324 — six changed lines, x/text v0.39.0 and x/sync v0.21.0.

### Future work

The user should rotate their local `OPENAI_API_KEY`. The remaining module-level dependabot alerts clear as dependencies drift forward (or stay, harmlessly unreachable).

## Step 5: SDK bumps and new model constants

**Author:** main

### Prompt Context

**Verbatim prompt:** "For 323, 322, 321, check whether we should add any new model constants (big model changes only) and if any tests should get new model additions" followed by "Merge the three PRs and add the constants"
**Interpretation:** Diff each SDK bump for newly added model identifiers, recommend which merit gai constants, then ship them.
**Inferred intent:** Keep the client packages' model constants current with what the SDKs expose.

### What I did

Downloaded both versions of each SDK into the module cache (`go mod download -json <mod>@<ver>` for the `Dir`) and diffed model constant declarations. Results: anthropic-sdk-go 1.51.1 → 1.58.0 adds exactly one model, `ModelClaudeSonnet5` (`"claude-sonnet-5"`); openai-go 3.40.0 → 3.44.0 adds the GPT-5.6 family (`gpt-5.6-luna`, `gpt-5.6-sol`, `gpt-5.6-terra`) and still no `gpt-5.5` constant; genai 1.60.0 → 1.64.0 adds no Gemini model strings at all. Merged #321/#322/#323, then added `ChatCompleteModelClaudeSonnet5Latest` to `/clients/anthropic/chat_complete.go` and the three `ChatCompleteModelGPT5_6*` constants to `/clients/openai/chat_complete.go` (also fixing the stale "v3.33.0" SDK reference in the `gpt-5.5` comment to v3.44.0). `make fmt`, build, and lint clean; shipped as PR #325, all four CI checks green (including Evaluate), merged.

### Why

The repo's convention is to alias SDK model constants as `ChatCompleteModel` values per client. A new Claude generation and a new GPT generation both clear the "big model changes only" bar; genai needed nothing.

### What worked

Diffing the module cache directly answered "what's new" empirically — important because the models postdate my training data. Grepping with the declaration pattern (`ModelClaude... Model = "..."`) rather than bare strings kept the diff signal-only.

### What didn't work

Nothing failed. The full local test suite was deliberately not run (the openai package 401s locally per Step 4); CI's Test job validated instead.

### What I learned

The thinking-level test matrices in `/clients/anthropic/chat_complete_test.go:414` and `/clients/openai/chat_complete_test.go:408` encode *probed* live-API behavior per model — which effort levels are accepted, when thought tokens appear. New rows can't be derived from an SDK diff, so they were deliberately deferred rather than guessed.

### What was tricky

Naming: the anthropic constants use a `Latest` suffix because the SDK constants alias moving IDs; `ChatCompleteModelClaudeSonnet5Latest` follows that even though "5" carries no dot version.

### What warrants review

`/clients/anthropic/chat_complete.go:43` and `/clients/openai/chat_complete.go:45-47`. Four one-line aliases; the main question for a reviewer is whether the deferred test rows should block anything (they don't — existing models' rows are untouched).

### Future work

A probing pass to add Sonnet 5 and GPT-5.6 rows to both thinking-level test matrices, plus the matching GoDoc bullets on the `ThinkingLevel` constants. Blocked on a valid OpenAI key for the GPT side.

## Step 6: Restrict gomod updates to the three SDK clients

**Author:** main

### Prompt Context

**Verbatim prompt:** "Could you change the dependabot config to only auto update the three SDK clients?" (after "check open prs" surfaced that #309 and #316 had been closed manually) and mid-turn "merge the PR once green"
**Interpretation:** Add an allow list so the gomod ecosystem only proposes updates for the Anthropic, OpenAI, and Google SDK modules.
**Inferred intent:** Same as Step 2, applied to Go modules — only high-signal dependabot PRs.

### What I did

Noted first that #309 (x/crypto) and #316 (cloud auth) were closed without merging at 11:20 UTC — go.mod still carries the old versions, and those exact releases are now auto-ignored. Then added an `allow` list to the `gomod` section of `/.github/dependabot.yml`: `github.com/anthropics/anthropic-sdk-go*`, `github.com/openai/openai-go*`, `google.golang.org/genai*`, with trailing wildcards to cover major-version module path suffixes like `/v3`. Shipped as PR #326, merged after green CI. Updated the persistent memory note so the preference carries to other repos.

### Why

The transitive-dependency bumps (grpc, x/crypto, otel, auth) were the bulk of the backlog and the user doesn't want to review them individually. Security updates bypass `allow` rules, so vulnerable dependencies anywhere in the graph still get PRs, with govulncheck as the reachability backstop.

### What worked

Checking the direct dependencies in `/go.mod` first to get the exact module paths — openai-go's module path is `github.com/openai/openai-go/v3`, which is why the wildcards matter.

### What didn't work

Nothing failed in this step.

### What I learned

An `allow` list replaces dependabot's default "everything" behavior entirely for that ecosystem; combined with Step 2's ignore rule, dependabot for this repo is now: SDK client bumps, action major bumps, and security fixes — nothing else.

### What was tricky

Interpreting the closed-not-merged #309/#316 correctly: dependabot's close-acknowledgment comment (the same one from Step 2) plus unchanged go.mod established they were manually closed, presumably intentionally, so I reported it rather than reopening.

### What warrants review

`/.github/dependabot.yml` — the whole file is now 24 lines and encodes the entire update policy; worth a 30-second read to confirm it says what you want.

### Future work

If a non-SDK direct dependency (e.g. `maragu.dev/*` modules, otel) should get updates again, it needs an explicit `allow` entry — worth remembering before wondering why dependabot went quiet on it.
