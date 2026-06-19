# Robust Wrappers

## Problem

`gai.ChatCompleter` and `gai.Embedder` implementations are single-provider, single-attempt. Transient failures (rate limits, 5xx, network blips) surface as immediate errors, and there is no built-in way to cascade to a secondary provider when the primary is misbehaving. Callers have to build this themselves and typically get it subtly wrong — around streaming semantics for chat completion, or around the generic component type for embeddings.

## Goals

- Provide `gai.ChatCompleter` and `gai.Embedder` wrappers that combine exponential-backoff retries and prioritized fallback across multiple underlying implementations.
- Keep the API minimal and consistent with existing gai style.
- Behave correctly around streaming: never duplicate partial output to the caller.
- Be observable via OpenTelemetry and `slog`, silent by default.
- Share policy types (`Action`, `ErrorClassifierFunc`, default classifier, backoff helpers) between the two wrappers.

## Non-goals

- Honoring `Retry-After` headers. gai does not currently surface these; revisit when it does.
- Per-completer / per-embedder retry configuration. Global settings cover the common case; revisit if needed.
- Circuit-breaker state across calls. Each call is independent.
- SDK-specific error classification. The default classifier stays provider-agnostic; see issue #210 for the planned gai-native error type that will replace regex-based string inspection.
- Mixed-component-type fallback for embedders. `gai.Embedder[T]` is generic and all fallbacks must share `T`.

## Package and layout

Subpackage `maragu.dev/gai/robust`:

- `chat_completer.go` — `ChatCompleter` type, constructor, `ChatComplete`.
- `embedder.go` — generic `Embedder[T]` type, constructor, `Embed`.
- `classify.go` — private `defaultErrorClassifier` and status-code helper.
- `backoff.go` — private `sleep` and `nextDelay` helpers shared between wrappers.
- `chat_completer_test.go`, `embedder_test.go` — external (`package robust_test`) tests against the public API, including the OpenTelemetry span shape each wrapper emits.
- `classify_test.go` — internal (`package robust`) tests for unexported helpers.

## Shared policy types

```go
type Action int

const (
    ActionRetry Action = iota + 1
    ActionFallback
    ActionFail
)

// The zero value of Action is an unexported internal sentinel; classifiers must return one
// of the three constants above. Any other value causes the retry switch to panic.

type ErrorClassifierFunc func(error) Action
```

Both wrappers use the same classifier type. The default classifier is operation-agnostic (just inspects errors).

## ChatCompleter API

```go
type NewChatCompleterOptions struct {
    Completers      []gai.ChatCompleter // priority order; non-empty
    MaxAttempts     int                 // 0 → default 3; 1 disables retry
    BaseDelay       time.Duration       // 0 → default 500ms
    MaxDelay        time.Duration       // 0 → default 30s
    AttemptTimeout  time.Duration       // 0 → no per-attempt timeout; bounds time-to-first-part
    ErrorClassifier ErrorClassifierFunc // nil → default classifier
    Log             *slog.Logger        // nil → discard
}

type ChatCompleter struct { /* unexported */ }

func NewChatCompleter(opts NewChatCompleterOptions) *ChatCompleter
func (c *ChatCompleter) ChatComplete(ctx context.Context, req gai.ChatCompleteRequest) (gai.ChatCompleteResponse, error)
```

## Embedder API

```go
type NewEmbedderOptions[T gai.VectorComponent] struct {
    Embedders       []gai.Embedder[T]   // priority order; non-empty, shared T
    MaxAttempts     int                 // 0 → default 3; 1 disables retry
    BaseDelay       time.Duration       // 0 → default 100ms
    MaxDelay        time.Duration       // 0 → default 5s
    AttemptTimeout  time.Duration       // 0 → no per-attempt timeout
    ErrorClassifier ErrorClassifierFunc // nil → default classifier
    Log             *slog.Logger        // nil → discard
}

type Embedder[T gai.VectorComponent] struct { /* unexported */ }

func NewEmbedder[T gai.VectorComponent](opts NewEmbedderOptions[T]) *Embedder[T]
func (e *Embedder[T]) Embed(ctx context.Context, req gai.EmbedRequest) (gai.EmbedResponse[T], error)
```

Both constructors panic on invalid inputs: empty list, negative `MaxAttempts`/`BaseDelay`/`MaxDelay`/`AttemptTimeout`, `MaxDelay == math.MaxInt64`, or `BaseDelay > MaxDelay`. `AttemptTimeout` has no upper bound, since the loop never shifts it the way it shifts `MaxDelay`. Defaults differ: embeddings are typically 50-200ms, so the backoff scale is one order of magnitude tighter.

## Behavior

### Attempt loop

Both wrappers follow the same cascading retry-then-fallback pattern:

1. Iterate implementations in priority order.
2. For each, attempt up to `MaxAttempts` times.
3. Call the underlying operation.
4. On error: classify.
   - `ActionFail` → return the error to the caller, abort everything.
   - `ActionFallback` → stop retrying this implementation, move to the next.
   - `ActionRetry` → sleep with full jitter, retry if attempts remain; otherwise move to the next.
   - Any other value, including the unexported zero sentinel → panic. Classifiers must return one of the three exported `Action` constants.
5. On success for `Embedder`: return the response immediately. For `ChatCompleter`: peek the first streamed part (see streaming below).
6. When all implementations are exhausted, return the final error.

### Per-attempt timeout

A hung backend — slow or stuck rather than erroring — is the one failure the wrapper otherwise can't handle. The caller's overall deadline is the only bound, and the default classifier maps it to `ActionFail`, so the first hang consumes the whole budget and the wrapper returns without retrying or falling over. `AttemptTimeout` fixes this by bounding a single attempt.

When `AttemptTimeout` is zero, behaviour is identical to having no timeout: no context wrapping, no timer. When set, `tryOnce` derives a sub-context with that timeout from the caller's context and runs the attempt against it.

A fired per-attempt timeout is **retryable, handled out of band**. The classifier never sees it. After the underlying call returns, `tryOnce` checks whether our sub-context's deadline fired while the parent context is still live; if so, it returns `ActionRetry` directly. Everything else flows through the classifier as before. The predicate is the crux: a per-attempt timeout and a caller cancellation both surface as `context.DeadlineExceeded`, so the only way to tell them apart is the parent context's liveness. The sub-context's own state — not `errors.Is` on the returned error — is the authoritative signal that *our* deadline fired.

A timeout-driven `ActionRetry` runs through the normal jittered backoff, then retries the same backend up to `MaxAttempts` (each retry gets a fresh clock) before falling over to the next. Returning `ActionRetry` is enough; the loop already does the rest.

The caller's own cancellation or deadline stays a hard stop. If the parent context is done, the error flows through the classifier — fatal by default — and the loop returns at once. The overall deadline therefore acts as a backstop ceiling above the per-attempt timeout.

For `Embedder` the mechanics are simple: `context.WithTimeout(ctx, AttemptTimeout)`, `defer cancel()`, detect via `attemptCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil`.

For `ChatCompleter` the per-attempt timeout bounds **time-to-first-part only** — the `ChatComplete` call plus the first iterator pull — not the whole stream. After commit, the wrapped iterator keeps reading from the attempt context for the rest of the stream, so a timer left running would cancel a healthy long stream mid-flight. `tryOnce` instead uses `context.WithCancel` plus a `time.AfterFunc` that records its firing in an `atomic.Bool` (the AfterFunc runs on its own goroutine) and cancels the sub-context. At commit the timer is stopped and ownership of the cancel transfers to the wrapped iterator, which cancels exactly once at stream end. On the failure paths the timer is stopped and the context cancelled before returning. If the timer fires right at commit, the committed first part still returns; the rest of the stream may then error through, consistent with the commit-on-first-part contract. Detection here keys off the timer-fired flag, not the returned error, because the cancel surfaces as `context.Canceled` rather than `context.DeadlineExceeded`.

### Streaming: commit on first part (ChatCompleter only)

A `gai.ChatCompleteResponse` can fail either before any part is emitted (the `ChatComplete` call returns an error) or mid-stream (the iterator yields an error). The design treats these symmetrically up to the commit point:

- Inside `ChatComplete`, after a successful underlying call, pull the first part from the iterator eagerly (still inside `ChatComplete`, before returning).
- If the iterator yields an error before yielding any part, treat it as a pre-stream error and run the classifier. If the iterator yields no parts at all, surface an anonymous "empty stream" error so the classifier can retry.
- As soon as one part has been yielded, commit: return a `ChatCompleteResponse` whose `Meta` pointer is the underlying one and whose iterator yields the buffered first part followed by the underlying iterator's remaining yields. No further retry or fallback happens after commit, even if the stream later errors.

Callers **must** drain `Parts()` on the returned response (even if they only read the first part), otherwise the `iter.Pull2` goroutine and the still-open OTel spans leak. See issue #211 for a planned proper fix.

`Embedder.Embed` has no streaming — a successful attempt returns the `EmbedResponse[T]` directly.

### Backoff

Full jitter per retry, with a 1-indexed `retryNumber`:

```
delay = rand[0, min(MaxDelay, BaseDelay * 2^(retryNumber-1))]
```

So the first retry draws from `[0, BaseDelay]`, the second from `[0, 2*BaseDelay]`, etc., capped at `MaxDelay`. Backoff state resets when moving to the next implementation. Sleep is context-interruptible. Implemented as package-private free functions in `backoff.go`, shared between the two wrappers.

### Default classifier

The package must not import any provider SDK — that would force every caller to pull in all three dependencies even if they only use one. The default classifier is SDK-agnostic and applies these rules in order:

1. `context.Canceled` / `context.DeadlineExceeded` → `ActionFail` (via `errors.Is`).
2. A 4xx/5xx HTTP status code found in the error string (via a targeted regex that rejects matches adjacent to `:`, `.`, `/`, or digits to avoid false positives on ports, IPs, path segments, and longer numbers) classifies by status.
3. Anything else → `ActionRetry` (optimistic default).

Status-to-action mapping: 429 and 5xx retry; other 4xx fall back; anything else retries.

String inspection is best-effort. Callers with provider-specific needs should supply their own `ErrorClassifierFunc`. The planned gai-native error type (issue #210) will allow precise interface-based classification in the future.

### Observability

Tracer: `otel.Tracer("maragu.dev/gai/robust")`. Child spans automatically parent under any caller span on the incoming context.

- `ChatCompleter` spans: root `robust.chat_complete`, child `robust.chat_complete_attempt`. Root attributes: `ai.robust.completer_count`, `ai.robust.max_attempts`, `ai.robust.base_delay_ms`, `ai.robust.max_delay_ms`. Per-attempt attributes: `ai.robust.completer_index`, `ai.robust.attempt_number`, `ai.robust.action`.
- `Embedder` spans: root `robust.embed`, child `robust.embed_attempt`. Root attributes: `ai.robust.embedder_count`, `ai.robust.max_attempts`, `ai.robust.base_delay_ms`, `ai.robust.max_delay_ms`. Per-attempt attributes: `ai.robust.embedder_index`, `ai.robust.attempt_number`, `ai.robust.action`.
- `ai.robust.action` is `"success"` on the successful attempt, or the action taken on failures. A timeout-driven retry records `"retry"`, the same as any other retryable error.
- `ai.robust.attempt_timed_out` (bool) marks the timeout-driven path. It is set to `true` only on an attempt that timed out, and omitted otherwise, so operators can distinguish a timeout-driven retry from an error-driven one.
- Errors recorded on attempt spans via `RecordError` and `SetStatus(codes.Error, ...)`, including the deadline error on the timeout path.
- For `ChatCompleter` on the committed path, both the attempt span and the root span stay open until the wrapped iterator terminates. For `Embedder` the spans close at `Embed` return.

Logging: `slog.Debug` only, on failover transitions and final exhaustion. Silent in production by default.

## Testing

External tests exercise end-to-end behavior via queue-driven fakes (`fakeChatCompleter`, `fakeEmbedder[T]`) that fail the test rather than panic when the queue is exhausted. The fakes can hang on the attempt context to simulate a stuck backend. Subtests have sentence-style names. Coverage includes happy path, retries, classifier-driven fallback, retry exhaustion then fallback, context cancellation (immediate and mid-backoff), full exhaustion, defaults, `MaxAttempts=1`, constructor panics, and unknown-Action panics. Per-attempt timeout coverage includes a hung backend retrying then falling over, exhausting `MaxAttempts` on one backend, the caller's own deadline staying fatal, the classifier never being invoked for the synthetic timeout, and the timeout-path span attributes. ChatCompleter additionally covers streaming commit, mid-stream passthrough, iterator-error-before-first-part retry, empty-stream retry, `Meta` pointer forwarding, and — the key streaming regression — a stream that commits quickly then runs past `AttemptTimeout` without being killed.

The timing-sensitive subtests — per-attempt timeout, backoff sleep, caller deadline, and the slow-stream regression — run inside `testing/synctest` bubbles (`synctest.Test`). The fake clock advances only when every bubble goroutine is durably blocked, so `time.AfterFunc`, `context.WithTimeout`, and the backoff `time.After` fire in a deterministic order without real waits or wall-clock margins. The bubble does not return until every goroutine exits, including the `iter.Pull2` and timer goroutines, so any ChatCompleter timing test that commits a stream must drain it to completion — the issue #211 drain contract, here enforced by the bubble.

Internal tests cover the unexported `defaultErrorClassifier`, `findStatusCode` regex (table-driven with positive and negative cases), and `nextDelay` jitter bounds.

## Open questions

- Whether to add a `TestEval`-style evaluation comparing robust vs. single-implementation success rates under simulated flakiness. Deferred.
- A proper fix for the `iter.Pull2` goroutine leak when callers drop the `ChatComplete` response without iterating. Tracked in issue #211.
