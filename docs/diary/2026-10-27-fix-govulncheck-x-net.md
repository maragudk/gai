# Diary: Fix govulncheck with a minimal `golang.org/x/net` bump

The daily Security workflow on main has failed since 2026-10-09 (issue #385). govulncheck reports three reachable vulnerabilities in `golang.org/x/net@v0.58.0`: GO-2026-6617, GO-2026-6612 and GO-2026-6611. All three are fixed in v0.60.0. The same failure blocks Dependabot PRs #388 and #389. Dependabot PR #390 (openai-go 3.71.1 to 3.77.0) happens to fix it by pulling in x/net v0.60.0, but this task bumps only x/net, so the security fix does not depend on an unrelated SDK upgrade.

## Step 1: Scope and hand-off

**Author:** main

### Prompt Context

**Verbatim prompt:** Launch a builder to fix govulncheck with minimal deps bump
**Interpretation:** Bump `golang.org/x/net` to v0.60.0 in `/go.mod` and `/go.sum`, and change nothing else in the dependency graph unless the bump forces it.
**Inferred intent:** Get the Security workflow on main green and close #385. Keep the change small and reviewable, separate from the pending SDK bumps.

### What I did
Checked the open PRs. I confirmed from the CI logs that the govulncheck failures on #388 and #389 come from x/net v0.58.0, and that #390 passes because it bumps x/net to v0.60.0. The Security workflow on main is failing on the same findings. I created the worktree `worktree-fix-govulncheck-x-net` and handed the change to a builder.

### Why
A targeted bump fixes the security finding without also shipping openai-go behavior changes. Once this lands, the Dependabot PRs can rebase onto a green main.

### What worked
The CI logs named the vulnerable module, the version it was found in and the version that fixes it, so no further investigation was needed.

### What didn't work
Nothing failed in this step.

### What I learned
govulncheck crashes locally because of a known bug, so the CI `govulncheck` job is the source of truth.

### What was tricky
Bumping x/net may also pull newer versions of its sibling modules (`golang.org/x/sys`, `golang.org/x/text`, `golang.org/x/crypto`) through its `go.mod` requirements. That is acceptable only if the bump itself forces it.

### What warrants review
Check that the `/go.mod` diff is limited to x/net and whatever it strictly requires.

### Future work
After merge, have Dependabot rebase #388, #389 and #390. #390 may then become a no-op for x/net.

## Step 2: Bump `golang.org/x/net` to v0.60.0

**Author:** xnet-builder

### Prompt Context

**Verbatim prompt:** Fix the failing govulncheck on main (issue #385) with the smallest dependency change that works. Bump `golang.org/x/net` to v0.60.0 with `go get golang.org/x/net@v0.60.0`, then run `go mod tidy`. Change nothing else unless the bump forces it. Leave the provider SDKs (openai-go, anthropic-sdk-go, genai) and the `go` directive alone. Don't change any code.
**Interpretation:** Run the bump and tidy, verify build, vet and tests, and record exactly which modules moved.
**Inferred intent:** Turn the Security workflow green with a minimal, reviewable `/go.mod` and `/go.sum` change.

### What I did
Ran `go get golang.org/x/net@v0.60.0` and `go mod tidy` in the worktree. Go reported these upgrades:

- `go` directive: 1.25.8 to 1.26.0
- `golang.org/x/net`: v0.58.0 to v0.60.0
- `golang.org/x/crypto`: v0.55.0 to v0.57.0
- `golang.org/x/sync`: v0.22.0 to v0.23.0
- `golang.org/x/sys`: v0.47.0 to v0.48.0
- `golang.org/x/text`: v0.41.0 to v0.42.0

Only `/go.mod` and `/go.sum` changed. No Go code was touched. No provider SDK moved.

### Why
v0.60.0 contains the fixes for GO-2026-6617, GO-2026-6612 and GO-2026-6611. The sibling modules and the `go` directive moved because MVS requires it, not by choice.

### What worked
`go build ./...` and `go vet ./...` pass. `go list -m golang.org/x/net` reports v0.60.0. Tests pass in every package except for missing local credentials (see below).

### What didn't work
`go test -shuffle on ./...` fails in `/clients/google` with a panic, because the Vertex AI service-account tests need `GOOGLE_VERTEX_CREDENTIALS_PATH`, which is not set here:

    panic: project/location or API key must be set when using Vertex AI backend.
    maragu.dev/gai/clients/google_test.newVertexAIClientWithCredentials(...)
        /clients/google/client_test.go:58

The panic aborts the whole package run. Re-running with `go test -shuffle on ./clients/google/ -skip '.*/.*(service_account|Embedding_2|credentials)'` passes (`ok maragu.dev/gai/clients/google 126.299s`).

### What I learned
`golang.org/x/net` v0.60.0 declares `go 1.26.0` in its own `go.mod`, and so do the new x/crypto, x/sys, x/text and x/sync. A main module cannot declare a lower `go` version than its dependencies, so the `go` directive cannot stay at 1.25.8. The task brief said to leave it alone; this is a conflict that the bump forces. CI uses `go-version-file: go.mod`, so it will pick up Go 1.26 automatically.

### What was tricky
The `go` directive change is the one effect that reaches users: consumers of `maragu.dev/gai` now need Go 1.26.0 or newer.

### What warrants review
Confirm that raising the `go` directive to 1.26.0 is acceptable. Check that the `/go.mod` diff holds only x/net, its x/* siblings and the `go` line. Check that the `govulncheck` CI job passes.

### Future work
None beyond the post-merge Dependabot rebases in Step 1.
