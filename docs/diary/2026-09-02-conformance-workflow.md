# Diary: Move the live model conformance test out of merge-gating CI

The live `TestModelConformance` in each client package was designed (2026-08-17) to redden CI on provider-side model drift. Two drifts in two days (Google on 2026-09-01, Anthropic on 2026-09-02) blocked three unrelated PRs (#343, #344, #346) until a triage PR landed on main. This work keeps the drift detection but moves it to a scheduled, non-required workflow that surfaces drift as a single GitHub issue instead of a red merge gate.

## Step 1: Design the decoupling with Markus

**Author:** main

### Prompt Context

**Verbatim prompt:** "Maybe we should make the model drift test be skipped in regular CI, and then run it in a different workflow that's not required for merge, like the security workflow? Gets us visibility but not broken pipelines." Then, on the issue mechanism: "But how will we make sure to not open a lot of duplicate issues?" and finally "Minimal, and we don't need a full brainstorm section by section for it. Start the builder"
**Interpretation:** Decouple model-drift detection from the merge gate: skip the live conformance test in regular test runs, run it on a schedule in its own workflow, and report drift through a deduplicated GitHub issue.
**Inferred intent:** Keep the freshness signal the August decision bought, but stop it from taxing every unrelated PR with a rerun-or-triage-first cycle.

### What I did

Ran a short brainstorm (three multiple-choice questions, one at a time). Decisions: a new dedicated `conformance.yml` workflow (daily schedule plus manual dispatch, not a required check) rather than folding into `compatibility.yml` (which upgrades deps first and would conflate two failure causes) or `security.yml`; an env-var gate inside the three tests (`t.Skip` unless `GAI_MODEL_CONFORMANCE` is set) rather than `-skip` flags or build tags, so `make test`, `ci.yml`, and `compatibility.yml` skip it with no further change; and a GitHub issue on failure with minimal dedup — a fixed label and title, create only if no open issue exists, close it on the next green run, no per-run comments. Markus cut the brainstorm short after the dedup question, so the remaining details (schedule time, secrets, docs) are settled in the builder brief rather than reviewed section by section. Recorded the decision as an amendment to the 2026-08-17 entry in `/docs/decisions.md` via the decisions skill, created worktree `conformance-workflow` from `origin/main`, and delegated implementation to a builder.

### Why

The August design accepted "fail both directions" in merge-gating CI as the price of freshness; the price turned out to be paid by whoever happens to have a PR open when a provider ships a model, not by whoever curates the list. A scheduled run with a persistent issue keeps the signal and moves the cost to a deliberate triage step.

### What worked

Having today's three blocked PRs as concrete evidence made the trade-off discussion short; the dedup question was the only design point that needed real thought.

### What didn't work

Nothing failed in this step.

### What I learned

A merge-gating live test is only tolerable when its failures are rare and cheap to discharge; two drifts in two days across two providers showed that neither holds once the curated lists cover current generations.

### What was tricky

Issue dedup: "one open issue at most, auto-closed on green" is simple to state but the naive implementation (create on every failure) produces a daily pile. The minimal variant chosen loses the "a new model joined the drift" signal in exchange for ~10 lines of shell instead of ~30.

### What warrants review

After the builder's change: the three skip guards are byte-identical, `make test` no longer touches the model endpoints, the workflow uses `-count=1`, the issue step cannot create a second open issue, and green runs close an open issue. `AGENTS.md`'s conformance sentence and the client READMEs (if they mention the test) reflect the new mechanism.

### Future work

The behavior matrices (thinking levels, embed) remain in merge-gating CI with their own live-API flake problems (the Vertex embed 429 caused five reruns on 2026-09-01); if those keep tripping, the same decoupling question applies to them.

## Step 2: Gate the tests, add the make target and the daily workflow

**Author:** workflow-builder

### Prompt Context

**Verbatim prompt:** The lead's brief: add an identical `GAI_MODEL_CONFORMANCE` skip guard to the three `TestModelConformance` functions, a `make conformance` target, a `.github/workflows/conformance.yml` daily workflow that opens one deduplicated `model-drift` issue on failure and closes it on green, and update `AGENTS.md` plus any README or `docs/index.html` mention of the test.
**Interpretation:** Implement the 2026-09-02 decision exactly as recorded, without changing what the conformance tests themselves check.
**Inferred intent:** Take model drift off the merge gate today, with the smallest possible surface: three guards, one target, one workflow, one doc sentence.

### What I did

Confirmed first that nothing live runs before the guard would: `newClient` in each of `/clients/openai/client_test.go`, `/clients/anthropic/client_test.go` and `/clients/google/client_test.go` only calls `env.Load("../../.env.test.local")` and constructs a client, and `grep -rn "func init\|func TestMain" clients/` found neither. So the guard sits at the top of `TestModelConformance`, above `newClient(t)`, in all three files:

```go
if os.Getenv("GAI_MODEL_CONFORMANCE") == "" {
	t.Skip("set GAI_MODEL_CONFORMANCE=1 to run the live model conformance test")
}
```

Added the `conformance` target to `/Makefile` in the alphabetical slot between `benchmark` and `cover`, running `GAI_MODEL_CONFORMANCE=1 go test -count=1 -shuffle on -run TestModelConformance ./clients/...`. Wrote `/.github/workflows/conformance.yml`: `schedule` at `44 7 * * *` (the existing daily workflows both use `14 7 * * *`) plus `workflow_dispatch`, a `concurrency` group on the workflow name, `permissions: contents: read` and `issues: write`, and setup copied from `ci.yml`'s test job — `actions/checkout@v7` with `persist-credentials: false`, `actions/setup-go@v7` with `go-version-file: go.mod` and `check-latest: true`. The run step is just `make conformance`, so the env-var gate lives in one place. Two `gh` steps follow: one creates an issue titled "Model conformance drift" when the conformance step failed and no open `model-drift` issue exists, and one closes any open `model-drift` issue with a comment linking the green run. Finally updated the conformance bullet in `/AGENTS.md`, splitting it into two so the new mechanism does not make one bullet three times the length of its neighbours.

### Why

The guard goes inside the test rather than in the CI command line so that every existing `go test ./...` — local, `ci.yml`, `compatibility.yml` — skips without knowing anything about conformance. The run step calls `make conformance` rather than repeating the `go test` line, so the workflow and a local run can never disagree about what "conformance" means.

### Self-review

Ran the `code-review` skill: two independent reviewers over the diff, the workflow, and the three `newClient` helpers. Four findings had consensus and are fixed.

The serious one was the dedup guard, which failed open. GitHub runs `run:` blocks as `bash -e {0}` with **no `pipefail`**, and `-e` is suspended inside an `if` condition anyway, so in

```sh
if gh issue list --label model-drift ... | grep -q .; then
```

the pipeline's status is `grep`'s. A rate-limited or 5xx `gh issue list` prints nothing, `grep` exits 1, and the step files a *second* issue — precisely the case the dedup exists for. Replaced with an assignment (`existing="$(gh issue list ...)"`) so `set -e` aborts the step instead.

Second, `if: failure()` is job-scoped, not step-scoped: a flaky `setup-go` would have opened an issue titled "Model conformance drift" telling the reader to run `make conformance` for failing model IDs there aren't any. The conformance step now carries `id: conformance` and the issue step is gated on `failure() && steps.conformance.outcome == 'failure'`.

Third, `--limit 1` in the close step meant that if two `model-drift` issues were ever open, one stayed open forever and the create step would believe drift was still live. Now it lists up to 30 and closes them in a `while read` loop — which, being a pipeline, also fails open, so a transient `gh` error leaves the issue open rather than reddening a green run. The two steps deliberately fail in opposite directions: the create step must not duplicate, the close step must not redden.

Fourth, both reviewers flagged the copied "Set up GCP credentials" step and `GOOGLE_VERTEX_KEY` as dead: `TestModelConformance` in `/clients/google/model_conformance_test.go` only calls `newClient(t)`, which reads `GOOGLE_KEY` on the Gemini backend, and `newVertexAIClientWithKey`/`newVertexAIClientWithCredentials` are unreachable under `-run TestModelConformance`. Dropped both rather than write a service-account secret to the runner for nothing. This deviates from the brief's "copy the secrets `ci.yml` uses" and is flagged to the lead as such — it is a one-line revert if the intent was to keep the two workflows diffable.

Also applied from consensus nitpicks: `-shuffle on` in the make target to match `test` and `evaluate`, a `concurrency` group so a manual dispatch racing the cron cannot both pass the "no open issue" check, `GAI_MODEL_CONFORMANCE: "1"` on the run step as a second statement of intent alongside the Makefile, and an `AGENTS.md` bullet split that also drops the unverifiable "non-required" claim in favour of the durable reason (no `pull_request` trigger).

Declined one single-reviewer suggestion: filtering both `gh issue list` calls by author so the workflow never closes a human-labelled issue. The label is the contract here — an issue a human labels `model-drift` is a drift issue, and closing it on green is the desired behaviour, not a bug.

### What worked

The red/green was visible directly in the test output. Before the change, `go test -v -run TestModelConformance ./clients/...` ran live and gave `--- FAIL: TestModelConformance (2.44s)` for anthropic plus two live passes; after, all three report `--- SKIP: TestModelConformance (0.00s)`. Byte-identity of the three guards checked with `grep -n -A 3 'GAI_MODEL_CONFORMANCE' clients/*/model_conformance_test.go`, stripped of file prefixes and piped through `sort | uniq -c` — every line counted exactly 3. `go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/*.yml` exits 0 on all four workflows.

### What didn't work

`go test -shuffle on ./clients/google` fails in this worktree, but not because of this change: `panic: project/location or API key must be set when using Vertex AI backend` from `/clients/google/client_test.go:58`, because the copied `.env.test.local` holds only `OPENAI_KEY`, `ANTHROPIC_KEY`, `GOOGLE_KEY` and `GOOGLE_VERTEX_KEY` — there is no service-account `vertex.json` locally, which CI writes from a secret. `clients/openai` and `clients/anthropic` both pass, and anthropic passing is itself the point: before the guard it was red on `claude-fable-5-1`.

Two attempts to sanity-check the workflow's shell snippet in a local shell were refused by the worktree isolation guard ("this command runs `gh` with the text `.[].number` inside a construct too complex to verify"), so the `printf -v body` line rests on actionlint and on reading rather than on execution.

### What I learned

`gh issue list --label model-drift` exits 0 with empty output when the label does not exist yet — verified against `maragudk/gai`, whose label list has no `model-drift`. That matters: the close step's `number="$(gh issue list ...)"` would otherwise abort the step under `bash -e` and turn a green conformance run red on the very first run.

`make conformance` reproduces exactly what the issue body promises: `model_conformance_test.go:84: export or ignore these model IDs: claude-fable-5-1`.

### What was tricky

How much of `ci.yml`'s setup to mirror. Copying it wholesale was the first instinct and the brief's instruction, but it put a service-account secret on the runner for a test that never reads it; the reviewers were right that dead config in a brand-new file is worse than a diff against `ci.yml`.

The shell was trickier than it looks for a dozen lines. Every branch has a direction it should fail in, and `bash -e` without `pipefail` silently picks the wrong one for pipelines used as conditions. Writing the check as an assignment instead of a pipeline is the whole fix, and it is invisible unless you know that `-e` does not apply inside `if`.

The dedup logic keeps one deliberate blind spot worth stating plainly: the workflow cannot tell drift from a transient provider outage or an expired key, so a 500 from OpenAI opens an issue titled "Model conformance drift". The next green run closes it, which is the cheap resolution the minimal design bought.

### What warrants review

Whether dropping the Vertex credentials step and `GOOGLE_VERTEX_KEY` is right, since the brief asked for `ci.yml`'s secrets copied verbatim. Whether the first drift issue's colour and description should be pinned by creating the `model-drift` label by hand rather than leaving it to `gh label create ... || true`. And the asymmetry between the two `gh` steps: the create step aborts on a `gh` failure, the close step swallows it, which is intentional but worth a second opinion.

### Future work

The `model-drift` label does not exist in `maragudk/gai` yet. The workflow creates it on the first failing run via `gh label create ... || true`, so no manual step is required, but creating it by hand ahead of time would make the first drift issue's colour and description deterministic rather than dependent on that step succeeding.
