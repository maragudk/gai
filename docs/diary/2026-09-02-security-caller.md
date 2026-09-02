# Diary: Convert the Security workflow to a caller of the shared reusable workflow

`maragudk/workflows` now hosts the org's reusable `Security` workflow (govulncheck plus a deduplicated `govulncheck` issue on failure), and `maragudk/.github` offers the matching starter workflow. gai's `security.yml` — the file the reusable one was ported from (PRs #348 and #350 here) — becomes the first caller. Its CI run is the reusable workflow's first integration test.

## Step 1: Scope the conversion

**Author:** main

### Prompt Context

**Verbatim prompt:** "And merge after" (merging `maragudk/.github` PR #1, the starter workflow), following the earlier "Yes, spawn the builder" for the central repo and the repeatedly proposed, unobjected next step of converting gai to the first caller.
**Interpretation:** With both upstream PRs merged, replace gai's `.github/workflows/security.yml` with the starter workflow's contents, `$default-branch` resolved to `main`.
**Inferred intent:** Prove the shared workflow end to end on a real repository before rolling it out to `glue`, `app`, and `gomponents`.

### What I did

Created worktree `security-caller` from main, started this diary, and delegated the file replacement to a builder. The conversion is mechanical: the caller file is `maragudk/.github/workflow-templates/security.yml` with `$default-branch` replaced by `main`; gai's existing `security.yml` is deleted in favour of it. AGENTS.md's Security bullet needs a one-line update to say the workflow is now the shared one.

### Why

gai is where the pattern was built and verified, so it is the lowest-risk first caller; if the reusable workflow misbehaves, the diff against the old inline file is small and known.

### What worked

Nothing to report yet; delegation only.

### What didn't work

Nothing failed in this step.

### What I learned

Nothing new.

### What was tricky

Nothing; the only judgement is what to check after conversion: that the Security check still runs on the PR (as `govulncheck / govulncheck`, a renamed status check), that it is green, and that on `main` a green run still closes any open `govulncheck` issue.

### What warrants review

After the builder's change: `.github/workflows/security.yml` equals the template with `main` substituted; no other file changed except AGENTS.md; the PR's `Security` check runs and passes.

### Future work

Convert `glue` and `app` (identical pre-existing files); `gomponents` as a new adopter. If a branch protection rule requires the old `govulncheck` check name, update it to `govulncheck / govulncheck`.

## Step 2: Replace the inline workflow with a caller

**Author:** caller-builder

### Prompt Context

**Verbatim prompt:** "Replace gai's inline `.github/workflows/security.yml` with a caller of the shared reusable workflow. 1. Fetch the canonical caller: `gh api repos/maragudk/.github/contents/workflow-templates/security.yml --jq .content | base64 -d`. Replace every `$default-branch` with `main`. Write the result as `.github/workflows/security.yml`, overwriting the existing file entirely. Do not hand-edit anything else in it. 2. In `/AGENTS.md` (which `/CLAUDE.md` symlinks to — edit AGENTS.md only), update the bullet under Testing & Quality Gates that describes the `Security` workflow: it now calls the shared `maragudk/workflows` Security workflow; the behaviour (govulncheck on pushes to main, PRs, and daily; one `govulncheck` issue on failure on main, closed on green) is unchanged. Keep it one bullet, similar length to its neighbours. 3. Validate: the YAML parses; `git diff --stat` shows exactly two files changed; confirm the final file contains `uses: maragudk/workflows/.github/workflows/security.yml@main` and no `$default-branch` remains. 4. Do NOT run govulncheck locally (known local crash on this machine); CI is the oracle."
**Interpretation:** Do exactly what the task laid out — fetch, substitute, overwrite, edit one AGENTS.md bullet, validate — with no scope creep into the workflow body or other files.
**Inferred intent:** Make gai the first real caller of `maragudk/workflows`'s shared Security workflow, replacing the ported-from inline file with a thin `uses:` reference, so CI on this repo becomes the integration test for the shared workflow.

### What I did

Ran `gh api repos/maragudk/.github/contents/workflow-templates/security.yml --jq .content | base64 -d`, piped it through `sed 's/\$default-branch/main/g'`, and wrote the result straight to `/Users/maragubot/Developer/gai/.claude/worktrees/security-caller/.github/workflows/security.yml`, overwriting the old inline `govulncheck` job wholesale. The new file keeps the `push`/`pull_request`/`schedule`/`workflow_dispatch` triggers and the concurrency block (now targeting `main` in both spots that used to say `$default-branch`), drops the local `permissions` and `jobs.govulncheck.steps` entirely, and replaces the job body with a single `uses: maragudk/workflows/.github/workflows/security.yml@main`. Then edited the Security bullet in `/Users/maragubot/Developer/gai/.claude/worktrees/security-caller/AGENTS.md` (line 24 under "Testing & Quality Gates") to open with "The `Security` workflow calls the shared `maragudk/workflows` Security workflow, which runs `govulncheck`..." instead of "The `Security` workflow runs `govulncheck`...", keeping the rest of the sentence (issue-on-failure, close-on-green, bespoke label) untouched.

### Why

The task was explicit and mechanical: swap the file's body for the canonical caller with one placeholder substitution, and adjust exactly one prose sentence elsewhere to keep the docs honest about where the behaviour now lives, without re-describing the unchanged runtime behaviour.

### What worked

The `gh api ... | base64 -d | sed` one-liner produced byte-for-byte the expected caller shape on the first try — diffing it against what Step 1's plan described, the only differences from the old inline file are the trigger branch names, the collapsed `jobs.govulncheck` body, and the removed `permissions` block (now owned by the reusable workflow). `git diff --stat` confirmed only `.github/workflows/security.yml` and `AGENTS.md` changed, and `python3 -c "import yaml; yaml.safe_load(...)"` confirmed the new file parses.

### What didn't work

Nothing failed. As instructed, govulncheck was not run locally given the known crash on this machine; CI on the eventual PR is the oracle for whether the reusable workflow actually behaves.

### What I learned

The caller keeps the `permissions: {contents: read, issues: write}` block — a reusable workflow inherits the caller's grant and declares none of its own — while the checkout, setup-go, govulncheck, and issue-management steps now live only on the `maragudk/workflows` side.

### What was tricky

Nothing was tricky; this was a pure fetch-substitute-overwrite plus a one-line prose edit, and both validation checks (`git diff --stat` and YAML parse) confirmed the result matched the spec on the first pass.

### What warrants review

Confirm `/Users/maragubot/Developer/gai/.claude/worktrees/security-caller/.github/workflows/security.yml` line 24 reads `uses: maragudk/workflows/.github/workflows/security.yml@main` and that no `$default-branch` string remains anywhere in the file; confirm `/Users/maragubot/Developer/gai/.claude/worktrees/security-caller/AGENTS.md` still has exactly one Security bullet, similar length to its `Conformance` neighbour on the line above. Once this is committed and pushed, watch the PR's `Security` / `govulncheck` check actually run green against the shared workflow — that's the real proof, not local validation.

### Future work

Same as Step 1: once this caller is proven green in CI, port `glue` and `app` (identical pre-existing inline files) and onboard `gomponents` as a new adopter; check whether any branch protection rule pins the old check name and needs updating to match the reusable workflow's job name.
