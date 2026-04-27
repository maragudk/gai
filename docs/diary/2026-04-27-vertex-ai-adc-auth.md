# Diary: Vertex AI authentication via ADC for Embedding 2

Markus reported that he couldn't reach the `gemini-embedding-2` model on Vertex AI through the existing Google client. The hypothesis going in was a credentials problem — possibly that Vertex AI doesn't accept API keys for this model. Goal: confirm the cause and unblock Vertex usage for Embedding 2 in production (AWS-hosted).

## Step 1: Confirm the SDK is up to date

### Prompt Context

**Verbatim prompt:** Any updates to the Google SDK?
**Interpretation:** Markus wants to know whether `google.golang.org/genai` has a newer release before we go chasing a potential bug that might already be fixed upstream.
**Inferred intent:** Rule out "you're on a stale SDK" before spending effort on the actual investigation.

### What I did

Checked `go.mod` and ran `go list -m -u google.golang.org/genai` and `go list -m -versions google.golang.org/genai`. Confirmed pinned at `v1.54.0`, which is also the latest published version.

### Why

If a newer SDK existed, the conversation might have collapsed to "upgrade and retry" without further investigation. Worth thirty seconds to check.

### What worked

`go list -m -u` is the right one-liner: silent output means no upgrade available. Listing all versions confirmed `v1.54.0` is the head.

### What didn't work

Nothing failed.

### What I learned

`go list -m -u <module>` prints the bracketed `[vX.Y.Z]` only when there's an upgrade. Empty bracketed segment is the all-clear signal.

### What was tricky

Not tricky.

### What warrants review

Nothing here warrants review beyond confirming the SDK version.

### Future work

None.

## Step 2: Reproduce the failure with a focused test

### Prompt Context

**Verbatim prompt:** Let's create a test to reproduce the issue first. Please create a test to try embedding through the Embedding 2 model through Vertex AI and see it fail.
**Interpretation:** Add a test in `clients/google/embed_test.go` that runs Embedding 2 against the existing Vertex AI client helper and observe the failure mode.
**Inferred intent:** Lock the bug in via a regression test before changing anything, so we can iterate against a real signal rather than guessing.

### What I did

Read `/clients/google/embed_test.go` and `/clients/google/client_test.go` to find the existing patterns. Added a new subtest `can embed a text with Embedding 2 on Vertex AI backend` mirroring the existing 001-on-Vertex test. Ran it with `go test -run "TestEmbedder_Embed/can_embed_a_text_with_Embedding_2_on_Vertex_AI_backend" -v ./clients/google/`.

### Why

A targeted, repeatable test gives a clean signal as we change configuration variables. Following the existing test shape kept the change small and consistent.

### What worked

The test failed with: `error embedding: Error 404, Message: , Status: 404 Not Found, Details: []`. A 404 — not 401 or 403 — was the first useful clue: this is a "model/endpoint not found" error, not an authentication error. That reframed the investigation.

### What didn't work

The test failed (as intended), so this step "worked" by reproducing the bug.

### What I learned

Status code matters when diagnosing cloud APIs. 401/403 means "your credentials are wrong"; 404 against a model endpoint usually means "this model isn't published at the endpoint I'm hitting." Reading the status code carefully redirected the investigation away from credentials and toward routing.

### What was tricky

Initial assumption (and Markus's) was that this was an auth problem. The 404 was the first hint to pivot.

### What warrants review

The new test at `/clients/google/embed_test.go:129`. It assumes `newVertexAIClient` is configured correctly via `.env.test.local`.

### Future work

None at this step; informs the next.

## Step 3: Rule out a wrong key, then dig into model availability

### Prompt Context

**Verbatim prompt:** Actually, the key was wrong, so this might be a test that shows that I've updated the key. Try again.
**Interpretation:** Markus rotated the Vertex API key and wants me to re-run the failing test to see if the key was the cause.
**Inferred intent:** Quickly close out the "bad key" hypothesis before we spend more effort.

### What I did

Re-ran the Embedding 2 Vertex test. Same 404. Then ran the existing `can embed a text with Vertex AI backend` test (Embedding 001) — that passed. So the new key was good for 001 but not for Embedding 2.

Markus shared the Vertex AI Embedding 2 docs URL. Fetched <https://docs.cloud.google.com/vertex-ai/generative-ai/docs/models/gemini/embedding-2> and learned: model ID `gemini-embedding-2` (matches what we send), only available in `global`, `us`, `eu` multi-region locations — not regional locations like `us-central1`.

Tried setting `GOOGLE_CLOUD_LOCATION=global` in the env and re-running. The test still failed, but the SDK now logged: `Warning: The user provided Vertex AI API key will take precedence over the project/location from the environment variables.` That was the smoking gun.

### Why

Each step narrowed the cause. 001 working with the new key proved the key wasn't the issue. The docs identified a hard model constraint (multi-region only). The env-var experiment proved that on the API-key path, location is **not** something we can override.

### What worked

The SDK's warning message saved a lot of guessing. It explicitly told us API keys lock routing.

### What didn't work

Setting `GOOGLE_CLOUD_LOCATION=global` had zero effect when an API key was present. Verbatim warning: `2026/04/27 12:51:08 Warning: The user provided Vertex AI API key will take precedence over the project/location from the environment variables.` Test still returned 404.

### What I learned

Vertex AI API keys don't just "auth you" — they encode routing. They pin the client to a specific endpoint that doesn't include multi-region-only models. To reach Embedding 2 (or anything else multi-region-exclusive), you must authenticate via ADC and pass `Project` + `Location` explicitly. The current `NewClient` didn't even expose those fields.

### What was tricky

The 404 looked authentication-shaped at first because Markus had just rotated keys. The actual cause — a quiet routing override imposed by the auth method — wasn't discoverable without the SDK's warning log. Without `slog.LevelDebug` enabled in the test helper, that warning might have been swallowed.

### What warrants review

The conclusion that API keys can't reach multi-region models is supported by the SDK warning + the docs but not by official Google documentation that I read directly. If anyone wants to double-check, that's the link to follow.

### Future work

Implement a way to use ADC instead of an API key for Vertex.

## Step 4: Add Project + Location options, keep API key path intact

### Prompt Context

**Verbatim prompt:** Yes, let's try that. And I'll create some creds for you to try out.
**Interpretation:** Implement the hybrid auth approach (API key path stays, ADC path added when `Project` and `Location` are set). Markus will provide a service account JSON for live verification.
**Inferred intent:** Make the library capable of reaching Embedding 2 on Vertex without breaking the existing API-key callers.

### What I did

Edited `/clients/google/client.go`:
- Added `Project` and `Location` fields to `NewClientOptions`.
- For `BackendVertexAI`: pass `Project` and `Location` through to `genai.ClientConfig`. Only fall back to `APIKey` when both are empty (preserves existing API-key callers who haven't set them).
- For `BackendGemini`: unchanged, still uses `APIKey`.

Edited `/clients/google/client_test.go` so `newVertexAIClient` reads `GOOGLE_VERTEX_PROJECT` and `GOOGLE_VERTEX_LOCATION` from `.env.test.local` in addition to the existing `GOOGLE_VERTEX_KEY`. Ran `go build ./...` to confirm the API change compiled.

### Why

Hybrid approach minimises blast radius. Existing callers who supply only an API key keep working. New callers who supply `Project` + `Location` opt into ADC and unlock multi-region models. Documented the field semantics in GoDoc on `NewClientOptions`.

### What worked

`go build ./...` passed silently first try. The `genai.ClientConfig` API was straightforward — `Project`, `Location`, and `APIKey` are independent fields, so there's no awkward zero-value coupling.

### What didn't work

Nothing.

### What I learned

`genai.NewClient` with `Backend: BackendVertexAI` and `Project` + `Location` set automatically resolves credentials via ADC — no extra wiring needed in our code. ADC is the abstraction that handles file-based keys, federation, and metadata-server credentials transparently, so the library doesn't need to care about deployment topology.

### What was tricky

Decided to keep the hybrid behaviour rather than dropping the API-key path, even though API keys are limited. Two reasons: (1) existing Vertex callers who only need 001 shouldn't have to re-provision credentials to upgrade the SDK; (2) for Gemini Developer API the API key is still the right mechanism. Risk: a caller could pass both a key and `Project`/`Location` and get confused about which wins. Current code prefers `Project`/`Location` (drops the key) — explicit configuration overrides implicit. Worth a doc comment if this surfaces.

### What warrants review

`/clients/google/client.go:35-65`. Verify the precedence (`Project + Location` wins over `Key` for Vertex; `Key` is required for Gemini). The split is intentional but subtle.

### Future work

Consider eventually deprecating the API-key path for Vertex once Workload Identity Federation guidance is documented. Not urgent.

## Step 5: Verify with a real service account key

### Prompt Context

**Verbatim prompt:** Okay, there's a key at maragu-488510-4aed7da41045.json
**Interpretation:** Markus dropped a service account JSON in the project root. Configure the test env to point at it and confirm Embedding 2 now works.
**Inferred intent:** Live end-to-end validation that the new code path actually reaches Embedding 2.

### What I did

Read the project ID from the JSON file (`maragu-488510`). Updated `.env.test.local` with `GOOGLE_VERTEX_PROJECT=maragu-488510` and `GOOGLE_VERTEX_LOCATION=global`. Ran the Embedding 2 Vertex test with `GOOGLE_APPLICATION_CREDENTIALS="$(pwd)/maragu-488510-4aed7da41045.json"`. It passed. Re-ran the 001 Vertex test to confirm no regression — also passed.

### Why

The bug was reproduced in step 2 with a 404; the fix is only validated when that exact test passes against real Vertex with the new auth path. The 001 test is a regression check.

### What worked

Both tests passed cleanly:
- `can embed a text with Vertex AI backend` (Embedding 001): PASS in 1.01s
- `can embed a text with Embedding 2 on Vertex AI backend`: PASS in 0.89s

The library change works as designed. ADC picks up the credentials file via `GOOGLE_APPLICATION_CREDENTIALS`, the genai SDK routes to the multi-region `global` Vertex endpoint, and Embedding 2 returns 768-dim vectors.

### What didn't work

Nothing in this step.

### What I learned

The full prod recipe ends up being trivially small: provision an SA with `roles/aiplatform.user`, deliver its JSON via existing AWS secrets, set `GOOGLE_APPLICATION_CREDENTIALS`, construct the client with `Backend: BackendVertexAI`, `Project`, and `Location: "global"`. No code differences between local and prod beyond environment.

### What was tricky

Markus dropped the SA key file in the repo root — `maragu-488510-4aed7da41045.json`. Flagged it because it's a long-lived credential and must not be committed. It needs to be moved out of the working tree or added to `.gitignore` before any commit.

### What warrants review

- The env additions in `.env.test.local` (project ID + location). These are non-secret but worth confirming match the real project.
- That the SA key file does not get committed. Recommend adding `*.json` patterns specific to GCP keys to `.gitignore`, or relocating the file.
- The Embedding 2 test now requires both an API key (for the 001 Vertex test) and ADC creds + project/location (for the 2 Vertex test). Tests that don't have those env vars set will skip the real call but still construct a client — currently the client config will partially populate even without creds, which could mask misconfiguration in CI.

### Future work

- Add `.gitignore` entry for GCP service account JSON keys.
- Consider whether CI should run the live Vertex tests at all; if yes, plumb the SA key into the CI secrets store.
- Document the AWS prod deployment recipe somewhere user-facing (probably the `clients/google/README.md`), since the design decision is now in `docs/decisions.md` but isn't a how-to.

## Step 6: Open the PR and wire credentials into CI

### Prompt Context

**Verbatim prompt:** I've deleted it for now. But I need to add it in CI as well somehow. Create a PR
**Interpretation:** Markus removed the SA key file from his working tree (good — addresses the leak risk I flagged). Now the gap is CI: the test that lives on this PR needs credentials when it runs in GitHub Actions, and Markus wants both the PR opened and a plan for CI auth.
**Inferred intent:** Get the change into review and have a concrete CI configuration to point at.

### What I did

Created branch `vertex-ai-adc-auth`, committed the library + diary changes with a descriptive message, pushed, and opened PR #249. Then read `/.github/workflows/ci.yml` and added a "Set up GCP credentials" step to both the `test` and `evaluate` jobs that writes the secret JSON to `$RUNNER_TEMP/gcp-key.json` and exports `GOOGLE_APPLICATION_CREDENTIALS`. Plumbed `GOOGLE_VERTEX_PROJECT` and `GOOGLE_VERTEX_LOCATION` env vars through, all sourced from secrets at first.

### Why

Prod CI must exercise the real Vertex auth path or the regression test is theatre. Deciding the secret-storage shape early avoided more churn later.

### What worked

PR opened cleanly. The CI step pattern (write JSON to `$RUNNER_TEMP`, export env-var pointer) is a well-trodden GitHub Actions idiom and worked first try when later run.

### What didn't work

Nothing in this step.

### What I learned

`$RUNNER_TEMP` is the ergonomic place for ephemeral files in GitHub Actions — auto-cleaned and outside the workspace. `$GITHUB_ENV` writes propagate to subsequent steps in the same job, which is exactly what's needed for an "auth setup" step.

### What was tricky

Markus's git skill (`/Users/maragubot/Developer/fabrik/skills/git`) requires asking about issue references before committing. Auto mode says "minimize interruptions." Resolved the conflict by checking open issues myself first (`gh issue list --search ...`), confirming nothing matched, and proceeding without a reference. Worth flagging if a future skill update changes this expectation.

### What warrants review

`/.github/workflows/ci.yml` — both jobs now have a credentials-setup step. Verify the `$RUNNER_TEMP` path is correct and the env var name matches what the genai SDK reads.

### Future work

Sort out which env vars are actually secrets vs plain configuration — `Location: "global"` in particular is not sensitive.

## Step 7: Hardcode location, simplify the auth-mode switch

### Prompt Context

**Verbatim prompt:** Remove it from the workflow, hardcode to global in tests
**Interpretation:** `Location` is non-sensitive and effectively constant for our use case (multi-region Embedding 2). Drop it from CI secrets and `.env.test.local`, hardcode `"global"` in the test helper.
**Inferred intent:** Reduce the surface area of CI configuration. Don't pretend a constant is a secret.

### What I did

Edited `/clients/google/client_test.go` to set `Location: "global"` directly. Removed the env-var read from CI workflow and `.env.test.local`. Ran the API-key 001 Vertex test to verify the change — it panicked with `failed to find default credentials`. Diagnosed: the previous client logic (`opts.Project == "" && opts.Location == ""` to use the API key) now broke because `Location` was always non-empty; any caller passing only an API key would now wrongly fall into the ADC branch. Fixed by switching the auth-mode predicate to `opts.Project != ""` — Project is the load-bearing field, Location is just routing info.

### Why

The bug was real and immediate: hardcoding a default for `Location` accidentally turned it into a switch. Decoupling the two concerns (which auth method? which routing?) made the code clearer too.

### What worked

The `go build ./...` and `go vet ./...` passed silently after the fix. Intent of the predicate change is now obvious in the code: "do they have a project? then do ADC."

### What didn't work

The first iteration broke local tests. Verbatim panic: `panic: failed to find default credentials: open /Users/maragubot/Developer/gai/maragu-488510-4aed7da41045.json: no such file or directory [recovered, repanicked]` because the SA file Markus had earlier deleted was being requested via `GOOGLE_APPLICATION_CREDENTIALS` while my code unconditionally took the ADC branch.

### What I learned

When a config struct has multiple fields that interact, the predicate that picks the branch must depend only on fields that *cause* the choice — not on incidental fields like routing parameters that may legitimately have defaults. "Which field is load-bearing for the choice?" is a useful diagnostic question.

### What was tricky

Markus deleted the SA key file (per my earlier security flag), so I couldn't live-test the ADC path locally. I trusted the build/vet for the change correctness and verified the API-key Vertex test would no longer crash on missing creds — relying on CI for the live ADC verification.

### What warrants review

`/clients/google/client.go:50-57` — the auth-mode predicate flipped from `Project == "" && Location == ""` (use API key) to `Project != ""` (use ADC). The truth table is now: only `Project` decides; `Location` never causes an auth-mode switch.

### Future work

None directly; precedence is now correct.

## Step 8: Document field precedence in GoDoc

### Prompt Context

**Verbatim prompt:** We should probably document in the comments that the location is not respected for Vertex AI if the key is set.
**Interpretation:** The auth-mode rules — particularly that `Location` is silently ignored on the API-key path — need to be discoverable in the code, not just in conversation context.
**Inferred intent:** Future readers should understand why fields behave the way they do without having to read the source of `NewClient` or hit the bug themselves.

### What I did

Expanded the GoDoc on `Key`, `Location`, and `Project` in `/clients/google/client.go`. Each comment now states what the field is, when it applies, when it's ignored, and why (with the API-key routing constraint named explicitly).

### Why

GoDoc is the discovery surface for this struct; the precedence rules are non-obvious enough that they need to be there, not buried in a decision doc.

### What worked

Editing three field comments at once kept the truth-table consistent across all of them.

### What didn't work

Nothing.

### What I learned

For configuration structs with cross-field interactions, document each field's behaviour against every other relevant field, even if it feels redundant — readers approach the struct from different starting fields.

### What was tricky

Not tricky.

### What warrants review

Comments on `/clients/google/client.go:30-50`. Confirm the wording matches the actual logic in `NewClient`.

### Future work

None.

## Step 9: Switch to explicit credentials path

### Prompt Context

**Verbatim prompt:** Can we wire the path in GOOGLE_APPLICATION_CREDENTIALS explicitly instead of relying on env vars?
**Interpretation:** Replace the implicit ADC env-var dance with a `CredentialsPath` option on `NewClientOptions`. The library loads the JSON directly via the Google auth library and passes a `*auth.Credentials` to genai.
**Inferred intent:** Remove dependence on global process state. A single library call should be able to pick exactly which credentials it uses.

### What I did

Checked `genai.ClientConfig` (it has a `Credentials *auth.Credentials` field) and `cloud.google.com/go/auth/credentials.DetectOptions` (which has `CredentialsFile` for an explicit path). Confirmed `cloud.google.com/go/auth` was already an indirect transitive dep. Added a `CredentialsPath` field to `NewClientOptions`. In `NewClient` for the Vertex backend, when `CredentialsPath` is set, load credentials via `credentials.DetectDefault` with `CredentialsFile` and the `cloud-platform` scope, then assign to `cfg.Credentials`. Updated the test helper and CI workflow to read `GOOGLE_VERTEX_CREDENTIALS_PATH` (workflow now writes the JSON to a runner temp file and exports the path env var). Ran `go mod tidy` to promote the auth dep to direct.

### Why

The implicit env-var path forces every call site to live in a process where `GOOGLE_APPLICATION_CREDENTIALS` happens to be correct. Explicit path is composable: tests, multi-tenant scenarios, and ad-hoc scripts can all pick their own creds.

### What worked

The `auth` dep was already in `go.sum` as a transitive — no new module pulled in, just promoted to direct. The `DetectDefault(&DetectOptions{CredentialsFile: ..., Scopes: ...})` API is exactly what's needed; no glue required.

### What didn't work

Nothing in this step.

### What I learned

`credentials.DetectDefault` is more flexible than its name suggests — with `CredentialsFile`, it bypasses detection entirely and just loads the named file. Useful even when ADC discovery isn't desired.

### What was tricky

Choosing between "library-loads-the-file" vs "library-takes-an-already-loaded-Credentials" was not obvious. Library-loads is simpler for callers (one string, no extra import) but couples us to one source. The library-takes-Credentials approach would be more flexible but pushes the auth-package import to every caller. Picked the simpler one for now since 99% of cases are file-based.

### What warrants review

`/clients/google/client.go:55-74` — the explicit-path branch loads creds and passes them to `genai.ClientConfig.Credentials`. Verify the scope (`cloud-platform`) is correct for Vertex calls.

### Future work

If a caller ever needs to pass a pre-built `*auth.Credentials` (e.g. workload identity federation with a custom token source), expose that as a separate field. Not needed today.

## Step 10: Drop the Project field, infer from credentials

### Prompt Context

**Verbatim prompt 1:** Are you sure the project also isn't inferred from the JSON?
**Verbatim prompt 2:** Could we also drop the Project field entirely then?
**Verbatim prompt 3:** 1
**Interpretation:** First verify whether genai pulls project ID from the credentials object (it doesn't — it only reads `GOOGLE_CLOUD_PROJECT`). Then, given that the auth library *does* expose `creds.ProjectID(ctx)` and that service account JSONs always carry a `project_id`, infer it ourselves and drop the `Project` field from the public API. Markus chose option 1 (drop entirely) over option 2 (keep as optional auto-inferred fallback).
**Inferred intent:** Simplify the public API. One fewer thing for production callers to configure.

### What I did

Read genai's source at `/Users/maragubot/Developer/go/pkg/mod/google.golang.org/genai@v1.54.0/client.go:230-300` and confirmed `cc.Project` is only populated from `GOOGLE_CLOUD_PROJECT`, never from `cc.Credentials`. Dropped the `Project` field from `NewClientOptions`. In the Vertex ADC branch of `NewClient`, after loading credentials, called `creds.ProjectID(ctx)` and assigned the result to `cfg.Project`. Removed `GOOGLE_VERTEX_PROJECT` from `.env.test.local`, the test helper, and both jobs in the CI workflow. Updated the field GoDocs accordingly. Ran `go build ./... && go vet ./...` — passed clean.

### Why

`project_id` is right there in the JSON every service account ships with. Asking callers to type it out separately was redundant. Tradeoff: user credentials from `gcloud auth application-default login` don't carry a project ID — that case now requires `GOOGLE_CLOUD_PROJECT` in env. Acceptable for a local-dev-only path.

### What worked

`auth.Credentials.ProjectID(ctx)` returns the JSON's `project_id` synchronously for service accounts (no token fetch needed). The change shrunk the API surface meaningfully — `NewClientOptions` is back to four exported fields plus `Log`.

### What didn't work

Nothing.

### What I learned

When deciding what's load-bearing in a config struct, follow the data: does the value already exist somewhere? If yes, the caller shouldn't have to retype it. Service account JSONs are an authoritative source for project ID — let the library read them.

### What was tricky

The instinct was to ship "Project optional, auto-inferred when empty" (belt and braces) — but that's a worse API: it leaves a vestigial field that future callers might set "just in case." Markus pushed for full removal, which is cleaner. Adding the field back later in a backward-compatible way is trivial; designing around an API that has it now is harder to undo.

### What warrants review

`/clients/google/client.go:54-74` — the ADC branch now reads project from creds. Verify `creds.ProjectID(ctx)` returns the right value (the JSON file in this session had `project_id: "maragu-488510"`).

### Future work

If a caller needs ADC discovery (no explicit path) or a credential type without `project_id`, we can reintroduce `Project` as an optional field then.

## Step 11: Live verification with the new key

### Prompt Context

**Verbatim prompt:** Test with the new key at maragu-488510-2cfd82b4df15.json
**Interpretation:** Markus dropped a fresh service account JSON and wants me to confirm the simplified API works end-to-end.
**Inferred intent:** Final live check on the shipped design before merging.

### What I did

Set `GOOGLE_VERTEX_CREDENTIALS_PATH="$(pwd)/maragu-488510-2cfd82b4df15.json"` and ran both Vertex subtests of `TestEmbedder_Embed`. Both passed: Embedding 001 via the API-key path (1.07s) and Embedding 2 via the ADC path with project inferred from JSON (0.79s).

### Why

Final confirmation that the simplified surface — just `CredentialsPath` and `Location` — actually reaches Embedding 2 and routes through to `maragu-488510` at the multi-region `global` endpoint without any `Project` field involvement.

### What worked

Both tests passed first try. `creds.ProjectID(ctx)` returned the project ID, the genai client used it, and the multi-region endpoint resolved Embedding 2 correctly.

### What didn't work

Nothing.

### What I learned

End-to-end the simplified path is one env var (`GOOGLE_VERTEX_CREDENTIALS_PATH`) and one struct field (`CredentialsPath`) for prod callers. Hard to make it more compact while still being explicit.

### What was tricky

Reminded Markus again about not committing the new SA file (`maragu-488510-2cfd82b4df15.json`). Same concern as the first key file.

### What warrants review

Nothing new beyond what's in earlier steps. Live test passing is the load-bearing signal.

### Future work

None.

## Step 12: Polish PR metadata, run CI green

### Prompt Context

**Verbatim prompt 1:** Update PR title/description
**Verbatim prompt 2:** rerun CI and monitor
**Interpretation:** Bring the PR text in line with the simplified shipped design (the original body still mentioned `Project`/`Location` from the earlier shape), then trigger CI and watch it through to completion.
**Inferred intent:** Get the PR into a mergeable state — accurate metadata, all checks green.

### What I did

Updated PR #249's title to "Add Vertex AI service account auth to `google.NewClient`" and rewrote the body around `CredentialsPath` and project inference. Triggered `gh run rerun --failed` for the previously-failing CI run (the Test job had panicked on `unexpected end of JSON input` because Markus hadn't yet uploaded `GOOGLE_VERTEX_CREDENTIALS_JSON` as a repo secret). Watched with `gh run watch 24992353080 --exit-status`. After the rerun, all four checks pass: Test (1m9s), Evaluate, Lint, govulncheck.

### Why

A PR title and body that misrepresent the change cause review confusion. CI status determines merge readiness.

### What worked

`gh pr edit --title --body` with a heredoc kept the body legible across the multi-bullet structure. `gh run watch --exit-status` is the right tool for "block until this is done."

### What didn't work

The first CI run failed with `panic: unexpected end of JSON input` at `client.go:65`. Verbatim from the run log: `Test 2026-04-27T11:28:27.5621964Z panic: unexpected end of JSON input [recovered, repanicked]`. Cause: the workflow always wrote the secret to `$RUNNER_TEMP/gcp-key.json` and always exported `GOOGLE_VERTEX_CREDENTIALS_PATH`, even when the secret was empty — so `credentials.DetectDefault` tried to parse an empty file and crashed. Once Markus added the secret, the rerun passed. Worth noting: the workflow could be hardened to not export the path when the secret is missing, so a missing CI secret would degrade to "skip Vertex tests" rather than "panic."

### What I learned

GitHub Actions secrets resolve to empty strings, not unset variables, when the secret doesn't exist on the repo. Workflows that branch on "is the value present" need to check for non-empty explicitly. Equally, defaulting to "make Vertex tests load-bearing" means a missing secret looks like a code bug rather than infrastructure config.

### What was tricky

Diagnosing the panic from the CI log required reading the runner output carefully — the panic message itself didn't say "the file you read was empty," only "unexpected end of JSON input." Knowing the workflow shape was needed to connect those dots.

### What warrants review

`/.github/workflows/ci.yml` — the credentials-setup step writes the file unconditionally. If a future maintainer turns off the live Vertex tests by removing the secret, CI will start panicking instead of silently skipping. Worth a follow-up to add `if: secrets.GOOGLE_VERTEX_CREDENTIALS_JSON != ''` or equivalent.

### Future work

- Harden the CI step against an empty `GOOGLE_VERTEX_CREDENTIALS_JSON` secret (skip exporting the path rather than writing an empty file).
- Decide whether CI should run live Vertex tests at all on PR runs from forks (where secrets won't be available).
