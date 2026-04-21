# Diary: flaky test hunt (2026-04-21)

Task brief: hunt flaky tests across the suite, classify them by root cause, and propose fixes. CI has flaked twice in the last 24 hours on LLM-client subtests; we want a broader picture rather than chasing one test at a time.

Worked on branch `flaky-test-hunt-2026-04-21` off `main` (origin tip `8782d59`).

## What I ran

All runs used `go test -shuffle on -count=1` in a loop, one package per loop iteration, serialized to keep provider-side request rates steady.

| Scope                                     | Iterations | Approx wall time | Failures |
|-------------------------------------------|------------|------------------|----------|
| Non-client packages (one `go test` call)  | 100        | ~7 min           | 0        |
| Non-client packages (`-race`)             | 50         | ~10 s            | 0        |
| `clients/openai`                          | 20         | ~10 min          | 0        |
| `clients/anthropic`                       | 20         | ~6 min           | 0        |
| `clients/google` (includes Vertex AI)     | 20         | ~13 min          | 1        |

Non-client scope = `./robust/`, `./eval/`, `./eval/internal/evals/`, `./tools/`, `./internal/oteltest/`, `./clients/google/internal/schema/`, and the top-level `.` package. `./tools/` contains `TestNewFetch` with a real DNS lookup against `http://invalid-url-that-does-not-exist.example` that retries 3x — this dominates non-client runtime (~400 s over 100 iterations) but never failed.

Raw logs kept at `/tmp/flake-hunt/*.log` for the duration of the branch.

## Results

### Ranked flakiness

| Test                                                                                   | Runs | Failures | Flake rate |
|----------------------------------------------------------------------------------------|------|----------|------------|
| `TestChatCompleter_ChatComplete_VertexAI/can_chat-complete_with_Vertex_AI_backend`     | 20   | 1        | 5%         |
| Everything else                                                                        | —    | 0        | 0%         |

One failure, in the Google Vertex AI subtest:

```
chat_complete_test.go:527: Expected nil error, but got "Error 429, Message: Resource exhausted.
Please try again later. Please refer to https://cloud.google.com/vertex-ai/generative-ai/docs/error-code-429
for more details., Status: RESOURCE_EXHAUSTED, Details: []" (type genai.APIError)
```

No ordering-dependent failures under `-shuffle on`. No `-race` warnings across 50 non-client iterations.

### Root cause per test

- **`TestChatCompleter_ChatComplete_VertexAI/can_chat-complete_with_Vertex_AI_backend`**
  (`clients/google/chat_complete_test.go:509`) — provider-side 429 rate limit (`RESOURCE_EXHAUSTED`). The test issues a single unretried `ChatComplete` call. Any transient quota squeeze (shared project, concurrent CI runs, Google's per-minute buckets) surfaces as a test failure. The companion `TestEmbedder_Embed/can_embed_a_text_with_Vertex_AI_backend` in `clients/google/embed_test.go:115` has the identical shape and shares the same exposure; it just didn't happen to trip in this sample.

Historical context fits: the task brief cites `TestChatCompleter_ChatComplete/can_use_a_system_prompt` in `clients/openai` flaking once in the last 24 h. OpenAI's version asserts a substring `"bonjour"` inside the model output, which is loose enough that model wobble is unlikely to trip it repeatedly. The Google version at `clients/google/chat_complete_test.go:249` is `is.Equal(t, "Bonjour !", output)` — an exact match that would eventually flake if Gemini drifted on capitalization, trailing punctuation, or word choice. It didn't flake in the 20-run sample (Temperature=0 holds the response steady for this particular model version), but it's a latent flake waiting on a model update.

### Other latent risks (no failures in this run, worth flagging)

Nothing I'd block a PR on, but worth a future look:

- `clients/google/chat_complete_test.go:291-293` (`can use structured output`) — asserts `Dune / Frank Herbert / 1965` exactly. Survives thanks to Temperature=0 plus the fact that Gemini's most-likely sci-fi recommendation is extremely stable. A model rev could change that.
- `clients/google/chat_complete_test.go:249` (`can use a system prompt`) — `is.Equal(t, "Bonjour !", output)`. Strict match; loosen to a substring like the OpenAI and Anthropic versions.
- `clients/google/chat_complete_test.go:388` (`can describe a video`) — asserts normalized output contains `"thumbs up"`. Assuming the Gemini 2.5 Flash vision pipeline keeps recognizing the gesture this is fine, but it's a narrow hook.

## Proposed fixes

### Priority 1 — Vertex AI 429 (the actively-biting flake)

The structural fix is to wrap the Vertex AI completer/embedder with `robust.ChatCompleter` / `robust.Embedder` for the tests, since we already own a retry wrapper with the exact classifier behavior we want (429 → retry with jittered exponential backoff; see `robust/classify.go:37`).

Minimal shape:

```go
// clients/google/chat_complete_test.go near line 509
c := newVertexAIClient(t)
cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
    Completers: []gai.ChatCompleter{c.NewChatCompleter(google.NewChatCompleterOptions{
        Model: google.ChatCompleteModelGemini2_5Flash,
    })},
    BaseDelay: 500 * time.Millisecond,
    MaxDelay:  5 * time.Second,
})
```

Same pattern for the Vertex AI embed subtest (`clients/google/embed_test.go:115`). This keeps the provider coverage — we still verify the real Vertex AI backend — but survives the 429s that CI's shared quota keeps producing. Scope is isolated to tests, no public API change.

This is not a strict one-line fix (three-ish lines per call site, import added for `robust`), so I'm leaving implementation for a dedicated PR rather than drive-by-patching it here.

Alternative if we don't want test-only retry logic: skip the test on 429 (`t.Skipf`). Cheaper to implement, arguably the wrong message — the test stops reporting failures but also stops providing signal if Vertex AI's error shape changes.

### Priority 2 — Loosen strict Google assertions

Low-risk preventive fixes; each is a one-file, few-line change:

- `clients/google/chat_complete_test.go:249` — change `is.Equal(t, "Bonjour !", output)` to a case-insensitive `strings.Contains(strings.ToLower(output), "bonjour")` check, matching the OpenAI/Anthropic equivalents.
- `clients/google/chat_complete_test.go:291-293` — replace the exact `Dune` / `Frank Herbert` / `1965` assertions with structural checks (non-empty title, non-empty author, positive year) matching the OpenAI and Anthropic versions at `clients/openai/chat_complete_test.go:254-256` and `clients/anthropic/chat_complete_test.go:252-254`.

These didn't flake in the 20-run sample but are the only tests in the client suites where strict equality sits on top of generated output. Addressing them pre-emptively avoids a future one-off "this test flaked once" loop.

### Defer

- Add `robust` wrapping to all client tests as a general hedge. Tempting but not justified: OpenAI and Anthropic ran 20x clean. Only add retries where the signal says we need them (Vertex AI, and only if P1 lands).
- Normalize the tool-result second-turn pattern into a shared test helper — follow-up suggestion from PR #219's diary. Still out of scope.

## Summary recommendation

**Fix first:** Wrap `TestChatCompleter_ChatComplete_VertexAI` and `TestEmbedder_Embed/can_embed_a_text_with_Vertex_AI_backend` in `robust.ChatCompleter` / `robust.Embedder` so 429s retry instead of failing the test. This is the only test with observed non-zero flake rate in a 20-run sample, and we already own the retry wrapper. Estimated change size: ~10 lines across two files.

**Do next (preventive, low priority):** Loosen the two strict Google equality assertions (`Bonjour !` and `Dune/Frank Herbert/1965`) to match the shape used by the OpenAI and Anthropic versions. No immediate failures, but they're the remaining strict-match hooks on non-deterministic model output.

**Defer:** Wrapping OpenAI and Anthropic client tests in `robust`. No observed flakes in 20x, adding retry machinery there is premature.

## No code changes in this branch

The Vertex AI fix is three-ish lines but requires touching two files plus an import, and I want the decision about `robust`-wrapping tests reviewed before committing. Branch `flaky-test-hunt-2026-04-21` is push-worthy only to land this diary; I'll let the follow-up PR carry the actual fix.

## OpenAI deep-dive follow-up

Follow-up brief (same day): the first pass saw 0/20 on `clients/openai`, but CI observed one live flake of `TestChatCompleter_ChatComplete/can_use_a_system_prompt` earlier that day. 20 iterations was clearly undersampled; this follow-up hammers harder.

Rebased this branch onto `origin/main` first (two doc-only commits, `f2082ff`). No base-layer changes affect the OpenAI client code.

### Iteration counts and commands

Focused run (cheapest per iteration, most iterations per dollar):

```
go test -count=1 -shuffle on -run 'TestChatCompleter_ChatComplete/can_use_a_system_prompt' -v ./clients/openai/
```

Looped 100 times, serialized. Roughly 4 s per iteration, ~7 min wall time.

Full-suite run (so any other latent flakes surface too):

```
go test -count=1 -shuffle on ./clients/openai/
```

Looped 30 times, serialized. Roughly 40 s per iteration, ~20 min wall time.

Total API spend: well under a dollar (GPT-5-nano priced around $0.05/M input, $0.40/M output; the system-prompt subtest is ~20 tokens in, ~30–80 tokens out).

### Results

| Scope                                                     | Iterations | Failures | Rate |
|-----------------------------------------------------------|------------|----------|------|
| `TestChatCompleter_ChatComplete/can_use_a_system_prompt`  | 100        | 2        | 2%   |
| Full `clients/openai` suite                               | 30         | 1        | 3.3% |

Combined: 3 flakes out of 130 runs (~2.3%). **The flake reproduces cleanly and consistently** — it wasn't a provider blip.

Every single failure was in the same subtest with the same root cause.

### Root cause

Two sample failures (raw):

```
chat_complete_test.go:285: expected output "Salut ! Comment puis-je vous aider aujourd'hui ? Dites-moi ce dont vous avez besoin — parler d'un sujet, écrire quelque chose, traduire, ou aider pour du code, etc." to contain "bonjour"

chat_complete_test.go:285: expected output "Salut ! Comment puis-je t'aider aujourd'hui ? Je peux répondre à tes questions, t'aider à écrire ou corriger un texte, traduire, expliquer un concept, coder, planifier un projet, et bien plus encore. Dis-moi ce que tu veux faire ou donne un sujet." to contain "bonjour"
```

The model is obeying the system prompt — it's answering in French — but it picks `Salut` (informal) instead of `Bonjour` (formal) ~2–3% of the time. Both are canonical French greetings; the test is asserting on *which* greeting, not *whether* the output is French. That's over-specified.

Why this affects OpenAI but not Anthropic even though the assertion string is identical: the Anthropic test (`clients/anthropic/chat_complete_test.go:265`) explicitly sets `Temperature: gai.Ptr(gai.Temperature(0))`. The OpenAI test does not, so GPT-5-nano samples with its default temperature and occasionally lands on `Salut`. Setting `Temperature: 0` isn't an option for GPT-5 reasoning models (OpenAI only allows temperature=1 for these), so matching Anthropic's approach doesn't port.

### Fix

Broaden the matcher from "must contain `bonjour`" to "must contain `bonjour` or `salut`". This keeps the actual test intent (model honored the French system prompt) and drops the implicit register requirement. Same file already has a `requireContainsAny` helper used by the first subtest for exactly this kind of either/or check, so no new helpers needed.

Concrete change applied in this branch:

- `clients/openai/chat_complete_test.go:285` — `requireContainsAll(t, output, "bonjour")` → `requireContainsAny(t, output, "bonjour", "salut")` with a brief comment explaining the register caveat.

No other subtests flaked across the 130 runs, so the change is scoped to this one assertion.

### Validation

After the patch, ran the focused subtest once to confirm it still passes on a "Bonjour" response and compiles cleanly; `golangci-lint run ./clients/openai/...` reports 0 issues. I did not re-hammer 100x post-fix — the fix strictly expands the accepted set, so mathematically the flake rate can only go down. If QA wants empirical confirmation, the same 100-iteration script at `/tmp/flake-hunt-openai/run_system_prompt.sh` can be re-run.

### Cross-client note

The Anthropic version (`clients/anthropic/chat_complete_test.go:284`) uses `Temperature=0`, which probably masks the same register drift. If Anthropic ever loses that Temperature=0 pin, or if the Claude default starts to hedge toward `Salut`, it'd flake too. Pre-emptively broadening that assertion is cheap but not urgent; noting it here rather than drive-by-patching a second client in a flake-hunt branch. The Google version (`clients/google/chat_complete_test.go:249`, `is.Equal(t, "Bonjour !", output)`) was already flagged as latent in the first pass.

## Follow-up: Google P2 fixes applied

Loosened the two latent-but-not-actively-flaking Google assertions called out under Priority 2 above:

- `clients/google/chat_complete_test.go:249` — `is.Equal(t, "Bonjour !", output)` → case-insensitive substring match accepting either `bonjour` or `salut`, matching the OpenAI fix.
- `clients/google/chat_complete_test.go:291-293` — exact `Dune` / `Frank Herbert` / `1965` check → structural checks (non-empty title, non-empty author, positive year), matching the OpenAI and Anthropic structured-output assertions.

The Priority 1 Vertex AI 429 retrofit (wrapping the Vertex subtests in `robust.ChatCompleter` / `robust.Embedder`) is deferred to a separate PR by the user's call; this branch carries only the cheap assertion fixes.
