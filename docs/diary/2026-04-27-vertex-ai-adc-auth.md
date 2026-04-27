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
