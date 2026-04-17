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
- Automatic classification of arbitrary user completers. The default classifier only recognizes the three built-in SDK error types; users supply a custom classifier for anything else.

## Package and layout

New subpackage `maragu.dev/gai/robust`:

- `chat_completer.go` — types, constructor, `ChatComplete` implementation.
- `classify.go` — `DefaultErrorClassifier` and private status-code helper.
- `chat_completer_test.go` — all tests, in external package `robust_test`.

## API

```go
package robust

type Action int

const (
    ActionRetry    Action = iota // retry same completer after exponential backoff
    ActionFallback                // move to next completer in priority list
    ActionFail                    // bubble up error immediately
)

type ErrorClassifierFunc func(error) Action

var DefaultErrorClassifier ErrorClassifierFunc

type NewChatCompleterOptions struct {
    Completers  []gai.ChatCompleter // priority order; panics if empty
    MaxAttempts int                 // 0 → default 3
    BaseDelay   time.Duration       // 0 → default 500ms
    MaxDelay    time.Duration       // 0 → default 30s
    ErrorClassifier ErrorClassifierFunc // nil → DefaultErrorClassifier
    Log         *slog.Logger        // nil → discard
}

type ChatCompleter struct { /* unexported */ }

func NewChatCompleter(opts NewChatCompleterOptions) *ChatCompleter
func (c *ChatCompleter) ChatComplete(ctx context.Context, req gai.ChatCompleteRequest) (gai.ChatCompleteResponse, error)

var _ gai.ChatCompleter = (*ChatCompleter)(nil)
```

## Behavior

### Attempt loop

1. Iterate `Completers` in priority order.
2. For each completer, attempt up to `MaxAttempts` times.
3. Call `ChatComplete` on the underlying completer.
4. On pre-stream error: classify.
   - `ActionFail` → return the error to the caller, abort everything.
   - `ActionFallback` → stop retrying this completer, move to the next.
   - `ActionRetry` → sleep with full jitter, retry if attempts remain; otherwise move to the next completer.
5. On success, peek the first streamed part (see streaming below).
6. When all completers are exhausted, return the final error.

### Streaming: commit on first part

A `ChatCompleteResponse` can fail either before any part is emitted (the `ChatComplete` call returns an error) or mid-stream (the iterator yields an error). The design treats these symmetrically up to the commit point:

- Inside `ChatComplete`, after a successful underlying call, pull the first part from the iterator eagerly (still inside `ChatComplete`, before returning).
- If the iterator yields an error before yielding any part, treat it as a pre-stream error and run the classifier.
- As soon as one part has been yielded, commit: return a `ChatCompleteResponse` whose `Meta` pointer is the underlying one and whose iterator yields the buffered first part followed by the underlying iterator's remaining yields. No further retry or fallback happens after commit, even if the stream later errors.

This keeps the caller from ever seeing duplicated partial output while still catching "provider accepted the request and immediately failed" cases.

### Backoff

Full jitter per retry:

```
delay = rand[0, min(MaxDelay, BaseDelay << retryNumber)]
```

Sleep is interruptible:

```go
select {
case <-ctx.Done():
    return gai.ChatCompleteResponse{}, ctx.Err()
case <-time.After(c.nextDelay(retry)):
}
```

Backoff state resets when moving to the next completer.

### Default classifier

The package must not import any provider SDK — that would force every caller to pull in all three dependencies even if they only use one. Instead, `DefaultErrorClassifier` is SDK-agnostic and applies these rules in order:

1. `context.Canceled` / `context.DeadlineExceeded` → `ActionFail` (via `errors.Is`).
2. Any error in the tree that satisfies a private `interface { error; StatusCode() int }` is classified by the returned status. None of the current SDKs expose this method, but the hook is cheap and catches caller-wrapped errors or future SDK changes.
3. A regex scans the error string for a bare 4xx/5xx number and classifies by that status.
4. Anything else → `ActionRetry` (optimistic default).

Status-to-action mapping: 429 and 5xx retry; other 4xx fall back; anything else retries.

String inspection is best-effort. Callers with provider-specific needs should supply their own `ErrorClassifierFunc`.

### Observability

Tracer: `otel.Tracer("maragu.dev/gai/robust")`. Child spans automatically parent under any caller span on the incoming context.

- Root span `robust.chat_complete` with attributes: `ai.robust.completer_count`, `ai.robust.max_attempts`, `ai.robust.base_delay_ms`, `ai.robust.max_delay_ms`.
- Child span `robust.attempt` per attempt with attributes: `ai.robust.completer_index`, `ai.robust.attempt_number`, `ai.robust.action` (set after classification).
- Errors recorded on attempt spans via `RecordError` and `SetStatus(codes.Error, ...)`.

Logging: `slog.Debug` only, on failover to next completer and on final exhaustion. Silent in production by default.

## Testing

Red/green TDD. The streaming commit-point logic, context-cancellation edges, and `Meta` pointer forwarding have enough corner cases that writing the failing test first for each one is worth the discipline.

All tests live in `chat_completer_test.go`, in external package `robust_test`, using `maragu.dev/is` assertions and subtests with sentence-style names. A package-private `fakeChatCompleter` drives scenarios:

- succeeds on first try
- retries pre-stream error then succeeds
- retries error before first part then succeeds
- passes through mid-stream error after first part is emitted
- falls back when classifier says fallback
- exhausts retries then falls back to next completer
- bubbles up context canceled immediately
- returns final error when all completers exhausted
- respects context cancellation during backoff sleep
- applies defaults when options are zero
- panics on empty completers
- uses custom classifier when provided
- forwards meta pointer from succeeding completer
- full jitter delay stays within bounds

Separate subtests cover `DefaultErrorClassifier` against real instances of each provider's error types.

## Open questions

- Exact identifiers for provider SDK error types (resolved at implementation time via `go doc` and test feedback).
- Whether to add a `TestEval`-style evaluation comparing robust vs. single-completer success rates under simulated flakiness. Deferred.
