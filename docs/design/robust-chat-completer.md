# Robust Chat Completer

## Problem

`gai.ChatCompleter` implementations are single-provider, single-attempt. Transient failures (rate limits, 5xx, network blips) surface as immediate errors, and there is no built-in way to cascade to a secondary provider when the primary is misbehaving. Callers have to build this themselves and typically get it subtly wrong around streaming semantics.

## Goals

- Provide a `gai.ChatCompleter` wrapper that combines exponential-backoff retries and prioritized fallback across multiple underlying completers.
- Keep the API minimal and consistent with existing gai style.
- Behave correctly around streaming: never duplicate partial output to the caller.
- Be observable via OpenTelemetry and `slog`, silent by default.

## Non-goals

- Honoring `Retry-After` headers. gai does not currently surface these; revisit when it does.
- Per-completer retry configuration. Global settings cover the common case; revisit if needed.
- Circuit-breaker state across calls. Each `ChatComplete` call is independent.
- SDK-specific error classification. The default classifier stays provider-agnostic; see issue #210 for the planned gai-native error type that will replace regex-based string inspection.

## Package and layout

New subpackage `maragu.dev/gai/robust`:

- `chat_completer.go` — types, constructor, `ChatComplete` implementation.
- `classify.go` — private `defaultErrorClassifier` and status-code helper.
- `chat_completer_test.go` — external (`package robust_test`) tests against the public API.
- `classify_test.go` — internal (`package robust`) tests for unexported helpers.

## API

```go
package robust

type Action int

const (
    ActionNone     Action = iota // zero value; used internally to mark success
    ActionRetry
    ActionFallback
    ActionFail
)

type ErrorClassifierFunc func(error) Action

type NewChatCompleterOptions struct {
    Completers      []gai.ChatCompleter // priority order; non-empty
    MaxAttempts     int                 // 0 → default 3; 1 disables retry
    BaseDelay       time.Duration       // 0 → default 500ms
    MaxDelay        time.Duration       // 0 → default 30s
    ErrorClassifier ErrorClassifierFunc // nil → default classifier
    Log             *slog.Logger        // nil → discard
}

type ChatCompleter struct { /* unexported */ }

func NewChatCompleter(opts NewChatCompleterOptions) *ChatCompleter
func (c *ChatCompleter) ChatComplete(ctx context.Context, req gai.ChatCompleteRequest) (gai.ChatCompleteResponse, error)

var _ gai.ChatCompleter = (*ChatCompleter)(nil)
```

`NewChatCompleter` panics on invalid inputs: empty `Completers`, negative `MaxAttempts`/`BaseDelay`/`MaxDelay`, `MaxDelay == math.MaxInt64`, or `BaseDelay > MaxDelay`.

## Behavior

### Attempt loop

1. Iterate `Completers` in priority order.
2. For each completer, attempt up to `MaxAttempts` times.
3. Call `ChatComplete` on the underlying completer.
4. On pre-stream error: classify.
   - `ActionFail` → return the error to the caller, abort everything.
   - `ActionFallback` → stop retrying this completer, move to the next.
   - `ActionRetry` → sleep with full jitter, retry if attempts remain; otherwise move to the next completer.
   - Any other (unknown) value → panic. Classifiers must not return `ActionNone`.
5. On success, peek the first streamed part (see streaming below).
6. When all completers are exhausted, return the final error.

### Streaming: commit on first part

A `ChatCompleteResponse` can fail either before any part is emitted (the `ChatComplete` call returns an error) or mid-stream (the iterator yields an error). The design treats these symmetrically up to the commit point:

- Inside `ChatComplete`, after a successful underlying call, pull the first part from the iterator eagerly (still inside `ChatComplete`, before returning).
- If the iterator yields an error before yielding any part, treat it as a pre-stream error and run the classifier. If the iterator yields no parts at all, surface an anonymous "empty stream" error so the classifier can retry.
- As soon as one part has been yielded, commit: return a `ChatCompleteResponse` whose `Meta` pointer is the underlying one and whose iterator yields the buffered first part followed by the underlying iterator's remaining yields. No further retry or fallback happens after commit, even if the stream later errors.

Callers **must** drain `Parts()` on the returned response (even if they only read the first part), otherwise the `iter.Pull2` goroutine and the still-open OTel spans leak. See issue #211 for a planned proper fix.

### Backoff

Full jitter per retry, with a 1-indexed `retryNumber`:

```
delay = rand[0, min(MaxDelay, BaseDelay * 2^(retryNumber-1))]
```

So the first retry draws from `[0, BaseDelay]`, the second from `[0, 2*BaseDelay]`, etc., capped at `MaxDelay`. Backoff state resets when moving to the next completer.

Sleep is context-interruptible.

### Default classifier

The package must not import any provider SDK — that would force every caller to pull in all three dependencies even if they only use one. The default classifier is SDK-agnostic and applies these rules in order:

1. `context.Canceled` / `context.DeadlineExceeded` → `ActionFail` (via `errors.Is`).
2. A 4xx/5xx HTTP status code found in the error string (via a targeted regex that rejects matches adjacent to `:`, `.`, `/`, or digits to avoid false positives on ports, IPs, path segments, and longer numbers) classifies by status.
3. Anything else → `ActionRetry` (optimistic default).

Status-to-action mapping: 429 and 5xx retry; other 4xx fall back; anything else retries.

String inspection is best-effort. Callers with provider-specific needs should supply their own `ErrorClassifierFunc`. The planned gai-native error type (issue #210) will allow precise interface-based classification in the future.

### Observability

Tracer: `otel.Tracer("maragu.dev/gai/robust")`. Child spans automatically parent under any caller span on the incoming context.

- Root span `robust.chat_complete` with attributes: `ai.robust.completer_count`, `ai.robust.max_attempts`, `ai.robust.base_delay_ms`, `ai.robust.max_delay_ms`.
- Child span `robust.attempt` per attempt with attributes: `ai.robust.completer_index`, `ai.robust.attempt_number`, and `ai.robust.action` (`"success"` on the successful attempt, or the classified action on failures).
- Errors recorded on attempt spans via `RecordError` and `SetStatus(codes.Error, ...)`.
- On the committed path, both the attempt span and the root span stay open until the wrapped iterator terminates, so traces show the real streaming duration.

Logging: `slog.Debug` only, on failover to next completer and on final exhaustion. Silent in production by default.

## Testing

Red/green TDD. The streaming commit-point logic, context-cancellation edges, and `Meta` pointer forwarding have enough corner cases that writing the failing test first for each one is worth the discipline.

External tests (`chat_completer_test.go`, `package robust_test`) cover end-to-end behavior via a `fakeChatCompleter` driving scenarios: happy path, retries, streaming commit, fallback, exhaustion, context cancellation, constructor panics, classifier behavior, Meta forwarding, `MaxAttempts=1`, empty-stream retry, unknown-Action panic.

Internal tests (`classify_test.go`, `package robust`) cover the unexported `defaultErrorClassifier`, `findStatusCode` regex (table-driven with positive and negative cases), and `nextDelay` jitter bounds.

## Open questions

- Whether to add a `TestEval`-style evaluation comparing robust vs. single-completer success rates under simulated flakiness. Deferred.
- A proper fix for the `iter.Pull2` goroutine leak when callers drop the response without iterating. Tracked in issue #211.
