# Diary: Call the shared `lint` workflow from CI

`maragudk/workflows` now hosts reusable `lint`, `test`, `compatibility`, `build`, and `cd` workflows (its PR #3). gai converts only its lint job: its test and compatibility jobs need provider API keys and a Vertex credentials step, which a called workflow cannot inherit, so they stay inline (recorded in `maragudk/workflows/docs/decisions.md`, 2026-09-02). This PR is the proving caller for `lint.yml` and is left for Markus to merge.

## Step 1: Scope the conversion

**Author:** main

### Prompt Context

**Verbatim prompt:** "b" (choosing "I open a gai conversion PR for lint + test + compatibility and leave it unmerged for you to judge"), later narrowed by the secrets finding; then "merge 3".
**Interpretation:** With the reusable workflows on `main`, replace gai's inline lint job with a `uses:` line and open the PR without merging.
**Inferred intent:** Prove `lint.yml` on a real repository before other repositories adopt the templates.

### What I did

Created worktree `lint-caller` from main, started this diary, and delegated the edit to a builder: in `/.github/workflows/ci.yml`, the `lint` job becomes `uses: maragudk/workflows/.github/workflows/lint.yml@main` (the dependabot guard now lives inside the reusable workflow, so the caller's `if:` goes); `test` and `evaluate` untouched. AGENTS.md needs no change — it does not describe the lint job's mechanics.

### Why

The lint job is byte-identical across the org; gai is the repository where the shared-workflow pattern has been proven twice already (Security, #351).

### What worked

Nothing to report yet; delegation only.

### What didn't work

Nothing failed in this step.

### What I learned

Nothing new.

### What was tricky

Only the check-name change: the lint check will report as `lint / lint` instead of `Lint`, so a branch protection rule requiring `Lint` would need updating before merge.

### What warrants review

After the builder's change: only the `lint` job in `ci.yml` differs; the PR's `lint / lint` check runs and passes; the dependabot skip still applies (inside the reusable workflow).

### Future work

Test and compatibility conversion for gai depends on a secrets story in `maragudk/workflows`.

## Step 2: Make the edit and validate

**Author:** lint-caller-builder

### Prompt Context

**Verbatim prompt:** "In `.github/workflows/ci.yml`, replace the inline `lint` job with a call to the shared reusable workflow. Read the current file and `/Users/maragubot/Developer/workflows/.github/workflows/lint.yml` (the reusable workflow, on main) first. 1. The `lint` job becomes exactly: `lint:\n    uses: maragudk/workflows/.github/workflows/lint.yml@main`. Remove its `name:`, `runs-on:`, `if:` ... and `steps:`. Keep the job id `lint` and its position in the file. 2. Do NOT touch the `test` or `evaluate` jobs, triggers, concurrency, or anything else. If `ci.yml` has a top-level `permissions:` block, leave it; if it has none, leave it that way ... 3. Check whether anything else in the repo references the lint job's old name (`Lint`) — grep `.github/`, `AGENTS.md`, `Makefile`, `README.md`. Report findings; only edit AGENTS.md if it describes the lint job's mechanics (it likely does not). 4. Validate: YAML parses; `git diff --stat` shows exactly one file changed (plus the diary); `grep -n \"uses: maragudk/workflows\" .github/workflows/ci.yml` finds the line."
**Interpretation:** Perform the mechanical edit main already scoped in Step 1, verify no stray references need updating, validate the result, and report the diff without committing.
**Inferred intent:** Get a clean, minimal, reviewable diff proving the `lint.yml` reusable workflow works as a drop-in replacement, with no collateral edits.

### What I did

Read `/Users/maragubot/Developer/gai/.claude/worktrees/lint-caller/.github/workflows/ci.yml` and `/Users/maragubot/Developer/workflows/.github/workflows/lint.yml` to confirm the reusable workflow's `job.if` dependabot guard and steps matched the inline job exactly. Edited the `lint` job in `ci.yml` to:

```yaml
  lint:
    uses: maragudk/workflows/.github/workflows/lint.yml@main
```

removing `name: Lint`, `runs-on: ubuntu-latest`, the dependabot `if:` guard, and all four steps (checkout, setup-go, lint). Left the job id, its position (last job in the file), and everything above it (`test`, `evaluate`, triggers, `concurrency:`) untouched. Confirmed `ci.yml` has no top-level `permissions:` block, so nothing to add. Ran `grep -rn "Lint" .github/ AGENTS.md Makefile README.md`, which found only `AGENTS.md:21`, an unrelated bullet about `golangci-lint run` / `make lint` local tooling that doesn't describe the CI job's mechanics — left as-is per instructions. Validated the YAML with `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"` (printed `YAML OK`), confirmed `git diff --stat` shows exactly `.github/workflows/ci.yml | 21 +--------------------` (one file, 1 insertion, 20 deletions), and confirmed `grep -n "uses: maragudk/workflows" .github/workflows/ci.yml` matches line 85.

### Why

A byte-for-byte-equivalent inline job should collapse to the `uses:` call with no behavioral change, so the diff should be as small as the job definition permits and touch nothing else.

### What worked

The reusable workflow's steps and `if:` guard were already identical to gai's inline job (both copied from the same origin), so no behavioral judgment calls were needed — the change was purely mechanical.

### What didn't work

Nothing failed. No blocking ambiguity arose.

### What I learned

`golangci-lint-action@v9` and `actions/checkout@v7` / `actions/setup-go@v7` pins matched exactly between gai's inline job and the reusable workflow, which is what made this a safe drop-in rather than a version bump in disguise.

### What was tricky

Nothing tricky in the edit itself. The only judgment call was confirming the `AGENTS.md` lint bullet is about local `make lint` usage, not CI job mechanics, so it's out of scope per the task's own carve-out.

### What warrants review

The full diff is: `.github/workflows/ci.yml` lines 84-104 (old) collapse to lines 84-85 (new), replacing the entire inline job body with the two-line `uses:` call. Reviewers should confirm this matches Step 1's plan and that CI actually runs `lint / lint` green on this branch before merge. No other files were changed by this step (diary aside).

### Future work

None beyond what Step 1 already flagged (test/evaluate conversion pending a secrets story upstream).
