// Package robust provides a [gai.ChatCompleter] that wraps a prioritized list of
// underlying completers with jittered exponential-backoff retries and cascading fallbacks.
package robust

import (
	"context"
	"iter"
	"log/slog"
	"math/rand/v2"
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
	// ActionRetry retries the same completer after exponential backoff.
	ActionRetry Action = iota
	// ActionFallback moves to the next completer in the priority list.
	ActionFallback
	// ActionFail bubbles the error up to the caller immediately.
	ActionFail
)

// String satisfies [fmt.Stringer].
func (a Action) String() string {
	switch a {
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

// ErrorClassifierFunc inspects an error and returns the [Action] the [ChatCompleter] should take.
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
	// MaxAttempts per completer. Zero defaults to 3.
	MaxAttempts int
	// BaseDelay is the initial exponential-backoff delay. Zero defaults to 500ms.
	BaseDelay time.Duration
	// MaxDelay caps the backoff sleep. Zero defaults to 30s.
	MaxDelay time.Duration
	// Classifier decides how to handle errors. Nil defaults to [DefaultErrorClassifier].
	Classifier ErrorClassifierFunc
	// Log receives debug messages on failover and final exhaustion. Nil discards output.
	Log *slog.Logger
}

// NewChatCompleter constructs a [ChatCompleter]. Panics if Completers is empty.
func NewChatCompleter(opts NewChatCompleterOptions) *ChatCompleter {
	if len(opts.Completers) == 0 {
		panic("robust: Completers must not be empty")
	}
	if opts.MaxAttempts == 0 {
		opts.MaxAttempts = 3
	}
	if opts.BaseDelay == 0 {
		opts.BaseDelay = 500 * time.Millisecond
	}
	if opts.MaxDelay == 0 {
		opts.MaxDelay = 30 * time.Second
	}
	if opts.Classifier == nil {
		opts.Classifier = DefaultErrorClassifier
	}
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	return &ChatCompleter{
		completers:  opts.Completers,
		maxAttempts: opts.MaxAttempts,
		baseDelay:   opts.BaseDelay,
		maxDelay:    opts.MaxDelay,
		classifier:  opts.Classifier,
		log:         opts.Log,
		tracer:      otel.Tracer("maragu.dev/gai/robust"),
	}
}

// ChatComplete satisfies [gai.ChatCompleter].
func (c *ChatCompleter) ChatComplete(ctx context.Context, req gai.ChatCompleteRequest) (gai.ChatCompleteResponse, error) {
	ctx, rootSpan := c.tracer.Start(ctx, "robust.chat_complete",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.Int("ai.robust.completer_count", len(c.completers)),
			attribute.Int("ai.robust.max_attempts", c.maxAttempts),
			attribute.Int64("ai.robust.base_delay_ms", c.baseDelay.Milliseconds()),
			attribute.Int64("ai.robust.max_delay_ms", c.maxDelay.Milliseconds()),
		),
	)
	defer rootSpan.End()

	var lastErr error
	for completerIdx, completer := range c.completers {
		fallback := false
		for attempt := 1; attempt <= c.maxAttempts && !fallback; attempt++ {
			res, act, err := c.tryOnce(ctx, completer, req, completerIdx, attempt)
			if err == nil {
				return res, nil
			}
			lastErr = err

			switch act {
			case ActionFail:
				rootSpan.RecordError(err)
				rootSpan.SetStatus(codes.Error, "classifier returned fail")
				return gai.ChatCompleteResponse{}, err
			case ActionFallback:
				fallback = true
			case ActionRetry:
				if attempt < c.maxAttempts {
					if sleepErr := c.sleep(ctx, attempt); sleepErr != nil {
						rootSpan.RecordError(sleepErr)
						rootSpan.SetStatus(codes.Error, "context cancelled during backoff")
						return gai.ChatCompleteResponse{}, sleepErr
					}
				}
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
	return gai.ChatCompleteResponse{}, lastErr
}

// tryOnce runs a single attempt against one completer, including first-part peek,
// and returns the committed response (on success), or the classified action and error.
func (c *ChatCompleter) tryOnce(ctx context.Context, completer gai.ChatCompleter, req gai.ChatCompleteRequest, completerIdx, attempt int) (gai.ChatCompleteResponse, Action, error) {
	ctx, span := c.tracer.Start(ctx, "robust.attempt",
		trace.WithAttributes(
			attribute.Int("ai.robust.completer_index", completerIdx),
			attribute.Int("ai.robust.attempt_number", attempt),
		),
	)
	defer span.End()

	res, err := completer.ChatComplete(ctx, req)
	if err == nil {
		committed, peekErr := commitOnFirstPart(res)
		if peekErr == nil {
			return committed, ActionRetry, nil
		}
		err = peekErr
	}

	act := c.classifier(err)
	span.SetAttributes(attribute.String("ai.robust.action", act.String()))
	span.RecordError(err)
	span.SetStatus(codes.Error, act.String())
	return gai.ChatCompleteResponse{}, act, err
}

// commitOnFirstPart eagerly pulls the first (part, err) from the response's iterator.
// If an error is yielded before any part, returns that error so the caller can classify it.
// Otherwise returns a wrapped response that yields the buffered first part and then
// delegates to the underlying iterator for the remainder. Once committed, mid-stream errors
// pass through to the caller unchanged.
func commitOnFirstPart(res gai.ChatCompleteResponse) (gai.ChatCompleteResponse, error) {
	next, stop := iter.Pull2(res.Parts())
	firstPart, firstErr, ok := next()
	if !ok {
		stop()
		empty := gai.NewChatCompleteResponse(func(yield func(gai.Part, error) bool) {})
		empty.Meta = res.Meta
		return empty, nil
	}
	if firstErr != nil {
		stop()
		return gai.ChatCompleteResponse{}, firstErr
	}

	wrapped := gai.NewChatCompleteResponse(func(yield func(gai.Part, error) bool) {
		defer stop()
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

// sleep waits for the full-jitter backoff duration for the given retry number (1-indexed),
// or returns the context error if the context is cancelled first.
func (c *ChatCompleter) sleep(ctx context.Context, retryNumber int) error {
	exp := c.baseDelay << retryNumber
	if exp <= 0 || exp > c.maxDelay {
		exp = c.maxDelay
	}
	d := time.Duration(rand.Int64N(int64(exp) + 1))
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

var _ gai.ChatCompleter = (*ChatCompleter)(nil)
