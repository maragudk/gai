# Diary: robust embedder

Adding `robust.Embedder[T]` alongside the existing `robust.ChatCompleter` — same retry + fallback + classifier policy, applied to `gai.Embedder`. Branch: `robust-embedder`.

## Step 1: Brainstorm

### Prompt Context

**Verbatim prompt:** "Now do the same for gai.Embedder"

**Interpretation:** Build a generic `robust.Embedder[T]` that wraps a prioritized list of `gai.Embedder[T]` with the same retries-and-fallbacks policy as `robust.ChatCompleter`. Reuse the shared policy types.

**Inferred intent:** Round out the `robust` subpackage so both pillars of the `gai` public API have resilience wrappers.

### What I did

Ran `fabrik:brainstorm` through three focused rounds: (1) type reuse across the two wrappers — fully shared `Action`, `ErrorClassifierFunc`, default classifier; (2) API shape and observability — generic `Embedder[T]`, parity with ChatCompleter for otel spans; (3) code sharing internals — extract `sleep` and `nextDelay` as package-private free functions, duplicate the (slightly different) retry loop.

Design doc described across four sections: API and layout, behavior, testing, example+decisions.

### Why

`gai.Embedder` is generic over `VectorComponent` while `gai.ChatCompleter` is not, and `gai.Embedder` has no streaming. The shape of the wrapper is forced by those two facts. Deciding up-front to share policy types and split the backoff helper into `/robust/backoff.go` saved me from either duplicating the backoff logic or trying to unify the two retry loops under a generic helper (which would have been awkward given ChatCompleter's streaming branch).

### What worked

The user steering toward tighter embedding-scale defaults (100ms base, 5s max instead of 500ms/30s) was a clean catch I would otherwise have missed. Embeddings are typically 50-200ms end-to-end; a 500ms base backoff is nonsensical at that timescale.

### What didn't work

I initially imagined a cascading OpenAI→Google fallback example. Turns out OpenAI's embedder returns `float64` and Google's returns `float32`, so they cannot share a `gai.Embedder[T]` list — the generic `T` is a hard type boundary. Flagged this back to the user, agreed on a single-OpenAI-with-retries example instead.

### What I learned

The generic `T gai.VectorComponent` constraint is a feature, not a bug — it stops callers from accidentally mixing incompatible embedders into one list. But it also forecloses the "heterogeneous fallback with conversion" scenario without extra machinery. Worth noting in the design doc's non-goals.

### What was tricky

Deciding how much to factor. Landing on "extract `nextDelay` and `sleep` as free functions, duplicate the rest" was the pragmatic middle. A generic `run[Req, Res]` helper would have collapsed both retry loops into one, but ChatCompleter's commit-on-first-part branch has no analogue on the Embed side, so the generalisation would have been leaky.

### What warrants review

The design doc `/docs/design/robust.md` (renamed from `robust-chat-completer.md` to cover both wrappers) for factual accuracy on the shared/differing bits.

### Future work

If a third wrapper ever shows up (reranker? audio transcription?), that's when the retry loop itself should get abstracted. Not now.

## Step 2: Refactor shared helpers

### Prompt Context

Design step; no explicit prompt for this sub-step. Followed from brainstorm Q3.

### What I did

Created `/robust/backoff.go` with package-private `sleep` and `nextDelay` free functions, taking `baseDelay` and `maxDelay` as parameters. Removed the `*ChatCompleter.sleep` and `*ChatCompleter.nextDelay` methods from `/robust/chat_completer.go`; updated the one caller inside `ChatComplete` to invoke the free function as `sleep(ctx, c.baseDelay, c.maxDelay, attempt)`. Dropped the `math/rand/v2` import from `chat_completer.go` since it moved with `nextDelay`.

Renamed `TestChatCompleter_nextDelay` to `TestNextDelay` in `/robust/classify_test.go` and rewrote it to call the free function directly. Removed the `stubCompleter` helper (only needed for the old method-based test) and the `maragu.dev/gai` import from `classify_test.go`.

Committed as "Extract backoff helpers as package-private free functions".

### Why

The Embedder wrapper needs the same backoff math. Duplicating it in `embedder.go` would create the possibility of the two falling out of sync — future tuning would have to land in two places. Free functions with explicit parameters are the simplest shared surface.

### What worked

The refactor was mechanical: one new file, one import tweak, one call-site update, one test rewrite. `go test -shuffle on ./robust/` and `golangci-lint run ./robust/` both stayed green on the first try.

### What didn't work

Nothing; smooth refactor.

### What I learned

When a method on a struct uses only a couple of fields (not the whole receiver), promoting it to a free function with those fields as parameters is usually cheap and worth it for sharing.

### What was tricky

Nothing substantial.

### What warrants review

`/robust/backoff.go` is tiny; the caller update in `ChatCompleter.ChatComplete` is the only non-trivial edit. Verify the parameter order `(ctx, baseDelay, maxDelay, retryNumber)` reads right at call sites.

### Future work

None.

## Step 3: Implement Embedder

### Prompt Context

Design step; proceeded after brainstorm approval.

### What I did

Scaffolded `/robust/embedder.go` with:
- Generic `Embedder[T gai.VectorComponent]` struct mirroring `ChatCompleter`'s shape.
- Generic `NewEmbedderOptions[T]` with the same panics-on-misuse construction pattern, but defaults of `BaseDelay: 100ms` / `MaxDelay: 5s` tailored to embedding timescales.
- `Embed` method running the same cascading retry-then-fallback loop as `ChatComplete`, minus the streaming commit-on-first-part logic. Root span `robust.embed`, child span `robust.embed_attempt`, both ended on every return (no wrapper iterator to keep them open).
- `tryOnce` helper returning `(res, Action, error)` with `ActionNone` on success, same footgun prevention as the ChatCompleter version. `default:` switch case panics on unknown `Action`.
- Interface check `var _ gai.Embedder[float64] = (*Embedder[float64])(nil)` at the bottom.

Then `/robust/embedder_test.go` in `package robust_test` with `fakeEmbedder[T]` generic test double and 15 subtests covering: happy path, single retry, context.Canceled bubble-up, context cancellation during backoff sleep, classifier-driven fallback, retry exhaustion then fallback, full exhaustion, defaults, `MaxAttempts=1`, default classifier on DeadlineExceeded, unknown-Action panic, empty-Embedders panic, negative-MaxAttempts panic, BaseDelay > MaxDelay panic, and a `float64` instantiation smoke test.

`go test -shuffle on -count=3 ./robust/` and `golangci-lint run ./robust/` both clean.

### Why

Direct port of the ChatCompleter pattern. Most design calls were settled in brainstorm; implementation was mechanical.

### What worked

The generic `fakeEmbedder[T]` test double with a `fakeEmbedResponse[T]{err | embedding}` queue made every subtest a 3-5 line setup. Tests stay readable even with the generic type parameter.

Running `-count=3 -shuffle on` caught nothing — the wrapper has no shared state across calls, so concurrency issues are not really possible given the struct is read-only after construction.

### What didn't work

Nothing this time.

### What I learned

Generic test doubles in Go work well when you pick a concrete `T` (here `float32` in most tests) and let type inference do the rest. Mixing `float32` and `float64` tests in the same file is fine — Go handles generic instantiation cleanly.

### What was tricky

The interface check for generics. `var _ gai.Embedder[T] = (*Embedder[T])(nil)` doesn't compile directly at package scope because you can't use a type parameter there. The workaround: check a concrete instantiation, e.g. `var _ gai.Embedder[float64] = (*Embedder[float64])(nil)`. It's only a smoke check — the Go type system validates the generic constraint when users actually instantiate — but it's informative for readers.

### What warrants review

- `/robust/embedder.go` `Embed` — confirm the cascading loop matches `ChatComplete`'s semantics minus streaming.
- `/robust/embedder_test.go` — subtest coverage for all the public-API-visible behaviors.

### Future work

None required for this step.

## Step 4: Example and doc updates

### Prompt Context

Proceeded after implementation; the design included an example and a renamed design doc.

### What I did

Renamed `/docs/design/robust-chat-completer.md` → `/docs/design/robust.md` via `git mv`, then rewrote the contents to cover both wrappers: shared policy types, separate API sections for `ChatCompleter` and `Embedder`, unified behavior section with the streaming caveat marked as ChatCompleter-only, shared backoff / classifier / observability / testing sections. Added a non-goal note about mixed-component-type embedder fallback.

Added `/internal/examples/robust_embed/main.go` — single OpenAI embedder with `MaxAttempts: 3`, `BaseDelay: 100ms`, `MaxDelay: 5s`. Documented inline why this isn't OpenAI+Google: incompatible `T` (float64 vs float32).

Verified with `go run ./internal/examples/robust_embed` — no API key → 401 → regex classifies as `ActionFallback` → no more embedders → "all embedders exhausted" debug log fires, error bubbles up. Same smoke-test pattern as the chat example.

### Why

The example is the first thing a new user looks at. Explaining the generic-T constraint right in the comments saves them from copying a broken cascade and debugging type errors.

### What didn't work

First iteration of the example had an OpenAI+Google cascade that wouldn't compile — caught before writing it, after I ran `grep -n EmbedResponse` across `/clients/` and saw the float64/float32 split. Adjusted the plan and flagged it to the user.

### What I learned

OpenAI's embedding SDK returns `float64` because the OpenAI wire format sends JSON numbers (which are float64 in Go). Google's genai SDK returns `float32` because the Google wire format uses actual binary tensors. Neither is wrong; they just can't mix.

### What was tricky

Deciding whether to keep the embed example at all or rely on the chat example for the cascade demo. Went with keeping it because `gai.Embedder`'s generic shape deserves at least one concrete example in the repo.

### What warrants review

- `/docs/design/robust.md` for factual accuracy on the shared vs divergent bits.
- `/internal/examples/robust_embed/main.go` — the inline explanation of why there's no fallback.

### Future work

If / when `gai` grows a gai-native error type (issue #210), the inline regex classification could become interface-based and the two examples can be side-by-side in a single consolidated example. Not urgent.
