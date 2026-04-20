// Package robust provides [gai.ChatCompleter] and [gai.Embedder] wrappers around
// a prioritized list of underlying implementations, with jittered
// exponential-backoff retries and cascading fallbacks.
package robust

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"math"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"maragu.dev/gai"
)

// Action is the decision an [ErrorClassifierFunc] makes about how to handle an error.
type Action int

const (
	// actionNone is the zero value; it indicates no classification has been made.
	// Used internally by tryOnce to mark success. Not exported — the switch on
	// Action panics on this value, same as any other unknown value.
	actionNone Action = iota
	// ActionRetry retries the same completer after exponential backoff.
	ActionRetry
	// ActionFallback moves to the next completer in the priority list.
	ActionFallback
	// ActionFail bubbles the error up to the caller immediately.
	ActionFail
)

// String satisfies [fmt.Stringer].
func (a Action) String() string {
	switch a {
	case actionNone:
		return "none"
	case ActionRetry:
		return "retry"
	case ActionFallback:
		return "fallback"
	case ActionFail:
		return "fail"
	default:
		return "unknown"
	}
}

// ErrorClassifierFunc inspects an error and returns the [Action] a robust wrapper should take.
// It is used by both [ChatCompleter] and [Embedder].
type ErrorClassifierFunc func(error) Action

// ChatCompleter wraps a prioritized list of [gai.ChatCompleter]s with retries and fallbacks.
// Construct with [NewChatCompleter].
type ChatCompleter struct {
	completers  []gai.ChatCompleter
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	classifier  ErrorClassifierFunc
	log         *slog.Logger
	tracer      trace.Tracer
}

// NewChatCompleterOptions configures a new [ChatCompleter].
type NewChatCompleterOptions struct {
	// Completers is the prioritized list of underlying completers. Must be non-empty.
	Completers []gai.ChatCompleter
	// MaxAttempts per completer. Defaults to 3. Set to 1 to disable retrying.
	MaxAttempts int
	// BaseDelay is the initial exponential-backoff delay. Defaults to 500ms.
	BaseDelay time.Duration
	// MaxDelay caps the backoff sleep. Defaults to 30s.
	MaxDelay time.Duration
	// ErrorClassifier decides how to handle errors. Defaults to a conservative built-in.
	ErrorClassifier ErrorClassifierFunc
	// Log receives debug messages on failover and final exhaustion. Defaults to discarding output.
	Log *slog.Logger
}

// NewChatCompleter constructs a [ChatCompleter]. Panics if:
//   - Completers is empty,
//   - MaxAttempts, BaseDelay, or MaxDelay is negative,
//   - MaxDelay equals [math.MaxInt64],
//   - BaseDelay exceeds MaxDelay.
func NewChatCompleter(opts NewChatCompleterOptions) *ChatCompleter {
	if len(opts.Completers) == 0 {
		panic("Completers must not be empty")
	}
	if opts.MaxAttempts < 0 {
		panic("MaxAttempts must not be negative")
	}
	if opts.MaxAttempts == 0 {
		opts.MaxAttempts = 3
	}
	if opts.BaseDelay < 0 {
		panic("BaseDelay must not be negative")
	}
	if opts.BaseDelay == 0 {
		opts.BaseDelay = 500 * time.Millisecond
	}
	if opts.MaxDelay < 0 {
		panic("MaxDelay must not be negative")
	}
	if opts.MaxDelay == 0 {
		opts.MaxDelay = 30 * time.Second
	}
	if opts.MaxDelay == time.Duration(math.MaxInt64) {
		panic("MaxDelay must be less than math.MaxInt64")
	}
	if opts.BaseDelay > opts.MaxDelay {
		panic("BaseDelay must not exceed MaxDelay")
	}
	if opts.ErrorClassifier == nil {
		opts.ErrorClassifier = defaultErrorClassifier
	}
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	return &ChatCompleter{
		completers:  opts.Completers,
		maxAttempts: opts.MaxAttempts,
		baseDelay:   opts.BaseDelay,
		maxDelay:    opts.MaxDelay,
		classifier:  opts.ErrorClassifier,
		log:         opts.Log,
		tracer:      otel.Tracer("maragu.dev/gai/robust"),
	}
}

// ChatComplete satisfies [gai.ChatCompleter].
// The returned [gai.ChatCompleteResponse]'s Parts iterator MUST be drained by the caller,
// even if only to read the first part, otherwise spans stay open and the iterator's
// internal goroutine leaks. See https://github.com/maragudk/gai/issues/211.
func (c *ChatCompleter) ChatComplete(ctx context.Context, req gai.ChatCompleteRequest) (gai.ChatCompleteResponse, error) {
	ctx, rootSpan := c.tracer.Start(ctx, "robust.chat_complete",
		trace.WithAttributes(
			attribute.Int("ai.robust.completer_count", len(c.completers)),
			attribute.Int("ai.robust.max_attempts", c.maxAttempts),
			attribute.Int64("ai.robust.base_delay_ms", c.baseDelay.Milliseconds()),
			attribute.Int64("ai.robust.max_delay_ms", c.maxDelay.Milliseconds()),
		),
	)

	var lastErr error
	for completerIdx, completer := range c.completers {
		fallback := false
		for attempt := 1; attempt <= c.maxAttempts && !fallback; attempt++ {
			res, act, err := c.tryOnce(ctx, completer, req, completerIdx, attempt, rootSpan)
			if err == nil {
				// rootSpan is ended when the wrapped response's iterator terminates.
				return res, nil
			}
			lastErr = err

			switch act {
			case ActionFail:
				rootSpan.RecordError(err)
				rootSpan.SetStatus(codes.Error, "classifier returned fail")
				rootSpan.End()
				return gai.ChatCompleteResponse{}, err
			case ActionFallback:
				fallback = true
			case ActionRetry:
				if attempt < c.maxAttempts {
					if sleepErr := sleep(ctx, c.baseDelay, c.maxDelay, attempt); sleepErr != nil {
						rootSpan.RecordError(sleepErr)
						rootSpan.SetStatus(codes.Error, "backoff interrupted: "+sleepErr.Error())
						rootSpan.End()
						return gai.ChatCompleteResponse{}, sleepErr
					}
				}
			default:
				panic(fmt.Sprintf("classifier returned unknown Action %d", act))
			}
		}
		if completerIdx < len(c.completers)-1 {
			c.log.Debug("robust: falling over to next completer",
				"from_index", completerIdx, "to_index", completerIdx+1, "error", lastErr)
		}
	}

	c.log.Debug("robust: all completers exhausted", "final_error", lastErr)
	rootSpan.RecordError(lastErr)
	rootSpan.SetStatus(codes.Error, "all completers exhausted")
	rootSpan.End()
	return gai.ChatCompleteResponse{}, lastErr
}

// tryOnce runs a single attempt against one completer, including first-part peek.
// On success returns (committed, actionNone, nil); the attempt span is ended when the
// wrapped iterator terminates. On failure returns (zero, classifiedAction, err) and ends
// the attempt span before returning.
func (c *ChatCompleter) tryOnce(ctx context.Context, completer gai.ChatCompleter, req gai.ChatCompleteRequest, completerIdx, attempt int, rootSpan trace.Span) (gai.ChatCompleteResponse, Action, error) {
	ctx, attemptSpan := c.tracer.Start(ctx, "robust.chat_complete_attempt",
		trace.WithAttributes(
			attribute.Int("ai.robust.completer_index", completerIdx),
			attribute.Int("ai.robust.attempt_number", attempt),
		),
	)

	res, err := completer.ChatComplete(ctx, req)
	if err == nil {
		committed, peekErr := commitOnFirstPart(res, attemptSpan, rootSpan)
		if peekErr == nil {
			attemptSpan.SetAttributes(attribute.String("ai.robust.action", "success"))
			return committed, actionNone, nil
		}
		err = peekErr
	}

	act := c.classifier(err)
	attemptSpan.SetAttributes(attribute.String("ai.robust.action", act.String()))
	attemptSpan.RecordError(err)
	attemptSpan.SetStatus(codes.Error, act.String())
	attemptSpan.End()
	return gai.ChatCompleteResponse{}, act, err
}

// commitOnFirstPart eagerly pulls the first (part, err) from the underlying response's iterator.
// If an error or no part is yielded, returns an error so the caller can classify it and retry.
// Otherwise returns a wrapped response that yields the buffered first part and then delegates
// to the underlying iterator. Mid-stream errors after commit pass through to the caller.
//
// On the success path, attemptSpan and rootSpan are both ended when the wrapper's iterator
// terminates. On the failure paths, only attemptSpan is ended before returning; the caller is
// responsible for rootSpan's lifetime in that case.
//
// Callers of the wrapped response MUST drain [gai.ChatCompleteResponse.Parts] — see
// https://github.com/maragudk/gai/issues/211 — otherwise the iter.Pull2 goroutine and both
// spans leak.
func commitOnFirstPart(res gai.ChatCompleteResponse, attemptSpan, rootSpan trace.Span) (gai.ChatCompleteResponse, error) {
	next, stop := iter.Pull2(res.Parts())
	firstPart, firstErr, ok := next()
	if !ok {
		stop()
		attemptSpan.End()
		return gai.ChatCompleteResponse{}, errors.New("robust: underlying completer returned an empty stream")
	}
	if firstErr != nil {
		stop()
		attemptSpan.End()
		return gai.ChatCompleteResponse{}, firstErr
	}

	wrapped := gai.NewChatCompleteResponse(func(yield func(gai.Part, error) bool) {
		defer func() {
			stop()
			attemptSpan.End()
			rootSpan.End()
		}()
		if !yield(firstPart, nil) {
			return
		}
		for {
			p, e, ok := next()
			if !ok {
				return
			}
			if !yield(p, e) {
				return
			}
		}
	})
	wrapped.Meta = res.Meta
	return wrapped, nil
}

var (
	_ gai.ChatCompleter = (*ChatCompleter)(nil)
	_ fmt.Stringer      = Action(0)
)
