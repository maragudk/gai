# Diary: Open a deduplicated GitHub issue when the Security workflow fails

The `Conformance` workflow (PR #348) opens one `model-drift` issue on failure and closes it on the next green run; the pattern was verified end-to-end on 2026-09-02 (issue #349 opened, not duplicated, auto-closed). Markus wants the same for the `Security` workflow (`govulncheck`), whose scheduled failures otherwise reach nobody.

## Step 1: Scope the pattern for Security

**Author:** main

### Prompt Context

**Verbatim prompt:** "I like the pattern of opening an issue. Make a new builder to do the same for the security workflow."
**Interpretation:** Port the issue-open/close steps from `conformance.yml` to `security.yml`, so a `govulncheck` failure surfaces as a persistent, deduplicated issue.
**Inferred intent:** Scheduled workflows should never fail silently; a red run must produce something a human sees and can close.

### What I did

Read `/.github/workflows/security.yml`: it runs `govulncheck` on push to main, pull requests, a daily schedule (07:14), and manual dispatch, with `permissions: contents: read`, and is currently green. Scoped the port: add `issues: write`, an issue step gated on the govulncheck step's outcome, and a close step on success — both restricted to non-`pull_request` events, since a PR failure is already visible on the PR and `github.token` lacks issue permissions on fork PRs anyway. Label `security`, title `govulncheck found vulnerabilities`. Chose to duplicate the ~15 lines of shell rather than extract a composite action: two consumers do not justify the indirection; a third would. Created worktree `security-issue` from `origin/main`, started this diary, and delegated to a builder. No decision record — this extends the 2026-09-02 conformance decision's reporting mechanism to a sibling workflow without changing any policy.

### Why

`Security` already runs on a schedule, so it has the same silent-failure shape the conformance workflow had; the fix is the same and now proven.

### What worked

Having a verified reference implementation in the same repo made the brief a diff description rather than a design.

### What didn't work

Nothing failed in this step.

### What I learned

Nothing new; this is a port.

### What was tricky

Only the event gating: unlike `Conformance`, `Security` runs on pull requests, where opening repo issues would be noise and, for forks, a permissions error.

### What warrants review

After the builder's change: the issue steps are skipped on `pull_request`; the dedup check is an assignment (not a pipeline) so `gh` errors abort instead of filing a duplicate; the issue step is gated on the govulncheck step's outcome so a setup failure cannot file one; `actionlint` is clean.

### Future work

If a third workflow adopts the pattern, extract a composite action under `.github/actions/`.

## Step 2: Port the issue steps into the Security workflow

**Author:** security-builder

### Prompt Context

**Verbatim prompt:** "Edit `.github/workflows/security.yml` only (plus docs/diary): 1. `permissions`: add `issues: write` (keep `contents: read`). 2. Give the govulncheck run step an `id` (e.g. `govulncheck`) and a `name`. 3. Add an "Open security issue" step mirroring conformance.yml's "Open drift issue" step: `if: failure() && steps.govulncheck.outcome == 'failure' && github.event_name != 'pull_request'`. Label `security` […], title `govulncheck found vulnerabilities`, body linking the run and saying to run `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` locally. Dedup exactly as conformance.yml does […] 4. Add a "Close security issue" step mirroring "Close drift issue" […] 5. Do NOT run the issue steps on `pull_request` events […] 6. Add a `concurrency` group on the workflow name like conformance.yml has, so a dispatch racing the cron cannot double-file. […] 7. Keep phrasing, env var names […] as close to conformance.yml as possible […] 8. Check /AGENTS.md for any Security workflow mention worth a one-line update."
**Interpretation:** Copy the two `gh` steps from `/.github/workflows/conformance.yml` into `/.github/workflows/security.yml`, changing only the label, title, body, step names and the event gating, and add the supporting `permissions`, `id` and `concurrency` scaffolding.
**Inferred intent:** A scheduled `govulncheck` failure should leave a durable, deduplicated artefact a human will see, and the two workflows should read as one recognisable pattern rather than two similar-looking scripts.

### What I did

Edited `/.github/workflows/security.yml`: added `issues: write` alongside `contents: read`; named the previously anonymous `Checkout`, `Setup Go` and `Govulncheck` steps and gave the last one `id: govulncheck`; and appended `Open security issue` and `Close security issue`, copied verbatim from `conformance.yml`'s `Open drift issue` and `Close drift issue` apart from the label, the title (`govulncheck found vulnerabilities`), the body and close-comment wording, the step names, and the extra branch condition on both. Added one bullet to `/AGENTS.md` under Testing & Quality Gates, next to the existing `Conformance` bullet.

The label and that branch condition both changed after self-review, on the lead's call: the label is now the bespoke `govulncheck` (colour `B60205`, description "govulncheck found vulnerabilities in CI") rather than the generic `security`, and the gate on both steps is `github.ref_name == 'main'` rather than `github.event_name != 'pull_request'`. The reasoning is under Self-review below.

For concurrency I did not use `conformance.yml`'s bare `group: ${{ github.workflow }}`. `Security` also runs on `push` and `pull_request`, and a single group shared by every branch does not merely serialise runs — with `cancel-in-progress` unset, a third run arriving while one is in progress and one is pending *cancels the pending one*, so two PRs pushing at once could cancel each other's security check. `/.github/workflows/ci.yml`, which has exactly the same trigger shape, already solves this in the repo's own idiom, so I copied it: `group: ${{ github.workflow }}-${{ github.ref_name }}` with `cancel-in-progress: ${{ github.ref_name != 'main' }}`. This still satisfies the stated goal — cron, `push` to main and a dispatch on main all have `ref_name` `main`, so they share a group and cannot double-file — while `false` on main means those runs queue and complete rather than cancelling each other, and `true` elsewhere saves CI on rapid PR pushes.

I verified the dedup logic rather than trusting it by eye. I extracted the `Open security issue` shell out of the YAML with `yaml.safe_load`, put a stub `gh` on `PATH`, and ran it under `bash -e` — the shell GitHub Actions actually uses — across three cases:

```
--- no open issue, gh healthy      exit=0  -> FILED an issue
--- issue #42 already open         exit=0  -> did NOT file
--- gh issue list ERRORS (rc=1)    exit=1  -> did NOT file
```

I then confirmed the counterfactual the brief warned about, that the pipeline form fails *open*:

```
pipeline form:      WOULD CREATE DUPLICATE   exit=0
assignment form:                             exit=1
```

### Why

The brief's insistence on `existing="$(gh …)"` rather than a pipeline is the one non-obvious line in the whole change, and it is invisible to `actionlint`. A stub-`gh` harness turns "I read it carefully" into evidence, and cost about ten minutes.

### What worked

`ci.yml` turned out to have already answered the concurrency question for a workflow with these exact triggers, which converted a judgement call into following an existing convention. Comparing the two workflows by parsing both with `yaml.safe_load` and diffing the *parsed steps* — rather than eyeballing the text — made it easy to assert that the only differences between the drift and security steps are the label, title, body, names and gating; the `env:` blocks came out byte-identical.

### What didn't work

Nothing failed. Worth recording that `shellcheck` is not installed on this machine, so `actionlint` silently skipped the shell inside the `run:` blocks — its exit 0 says less than it appears to, which is part of why I built the stub harness. There is also no `actionlint` step in CI, so nothing downstream re-checks these files.

### What I learned

`bash -e` aborts on a failed command substitution in an assignment but not on a failed non-final stage of a pipeline; without `set -o pipefail` the pipeline's exit status is the last stage's. For issue-filing shell that is the difference between staying quiet and spamming duplicates every time the GitHub API has a bad minute.

Also: GitHub's `concurrency` without `cancel-in-progress` is not "queue everything". Only one run can be pending per group; a newer pending run cancels the older one. A group keyed on the workflow name alone is safe for a schedule-only workflow and actively harmful for one that also runs on pull requests.

### What was tricky

Requirement 6 asked for a group "on the workflow name like conformance.yml has", which conflicts with the workflow's own trigger set. I followed the stated *purpose* (a dispatch racing the cron cannot double-file) over the stated *form*, using the sibling workflow's convention, and flagged it; the lead kept it.

The label and the gating had the same shape of tension, and there I resolved it the other way — implementing the brief literally and reporting the problem instead of pre-empting it. That turned out to be the right split: the concurrency question had an in-repo precedent that made the answer objective, whereas the label and the gate were judgement calls the lead had context for that I did not. The rule I would extract is that deviating is defensible when the repo itself already answers the question, and reporting is better when only the person who wrote the brief can.

### Self-review

I reviewed the diff myself and then ran the `code-review` skill over it at high effort. Because each candidate fix contradicted something the brief stated explicitly, I reported rather than resolved, and the lead ruled on all four; the edits are applied and described below. The mechanics the review cleared: the `steps.govulncheck.outcome == 'failure'` gate does stop a checkout or setup-go failure from filing an issue (the outcome is then `skipped`, not `failure`); `printf -v` is fine on the default bash; `persist-credentials: false` does not affect `gh`, which reads `GH_TOKEN` from the environment; and the concurrency block is correct.

The one thing neither the brief nor I had noticed: **`security` is not a bespoke label the way `model-drift` is.** The conformance pattern is safe only because nothing but that workflow ever applies `model-drift`. `security` is exactly the label a maintainer reaches for on a human-filed vulnerability report, and the pattern breaks in both directions when it collides:

- The dedup check treats *any* open `security`-labelled issue as "already open for vulnerabilities" and exits 0. While an unrelated human-filed report sits open, a real `govulncheck` failure files nothing — which is precisely the silent-failure hole this change exists to close.
- The close step closes *every* open `security`-labelled issue. A green nightly run would close a human's vulnerability report with the comment "govulncheck is green again".

The lead chose the bespoke `govulncheck` label, which restores the `model-drift` property exactly: the workflow is the only thing that ever applies the label, so "any open issue with this label" is once again a sound proxy for "already reported". Nothing else in the two steps had to change.

The second finding was that `github.event_name != 'pull_request'` gates pull requests out but not a `workflow_dispatch` on a feature branch — a green branch dispatch would close an issue filed by main's cron while main is still vulnerable, and a red one would file an issue blaming main for a branch's dependency. Both steps now gate on `github.ref_name == 'main'`, which covers pull requests (`ref_name` is `N/merge` there) and stray dispatches in one simpler condition. A pleasant side effect is that the gate and the concurrency block now agree exactly: the runs that can file issues are precisely the ones with `cancel-in-progress: false`, so a run that might file is never cancelled out from under itself.

Two smaller findings, both inherited from the pattern rather than introduced here. `govulncheck` exits non-zero for errors as well as findings — I confirmed in `golang.org/x/vuln@v1.7.0` that `errVulnerabilitiesFound` carries code 3 (`internal/scan/errors.go:17`) while a generic error exits 1 and bad usage exits 2, and `doc.go` documents only "0 if none, non-zero if any" — so a vuln-database fetch failure or a proxy hiccup on `@latest` files an issue titled "govulncheck found vulnerabilities" that is not strictly true. The lead accepted this as-is, consistent with the conformance decision: an outage files an issue that self-closes on the next green run, and the cost of a self-healing false positive is lower than the cost of the extra machinery to distinguish exit 3 from exit 1. Now that the label is bespoke, such a false positive no longer suppresses anything but a genuine finding during the same outage window.

The other is that `issues: write` is workflow-level, so it also applies to `pull_request` runs that build PR code; fork PRs get a read-only token regardless, but same-repo PRs (Dependabot, collaborators) previously had `contents: read` alone. Job-level is the finest granularity GitHub offers and `conformance.yml` has the same shape, so this stands.

### What warrants review

Nothing is outstanding. The three things to confirm on merge are that the `govulncheck` label does not already exist in the repo with a different meaning (`gh label create … || true` will not repurpose it, but a pre-existing one would still be matched by the dedup and close steps); that `github.ref_name == 'main'` is the intended gate rather than `github.event.repository.default_branch`, if the default branch is ever renamed; and that the first live failure actually files, since like the conformance change this cannot be exercised until CI goes red.

### Future work

Add an `actionlint` step to CI, with `shellcheck` available so `run:` blocks are actually linted — right now workflow changes are checked only when someone remembers to run it locally.

The exit-code ambiguity is accepted, not fixed. If false-positive issues from transient `govulncheck` errors turn out to be a nuisance in practice, the fix is to capture the step's exit code and gate the open step on 3 specifically rather than on any failure. Both workflows would want that shape, which is another argument for the composite action Step 1 deferred.
