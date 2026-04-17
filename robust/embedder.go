package robust

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"maragu.dev/gai"
)

// Embedder wraps a prioritized list of [gai.Embedder] implementations with retries and fallbacks.
// Construct with [NewEmbedder].
type Embedder[T gai.VectorComponent] struct {
	embedders   []gai.Embedder[T]
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	classifier  ErrorClassifierFunc
	log         *slog.Logger
	tracer      trace.Tracer
}

// NewEmbedderOptions configures a new [Embedder].
type NewEmbedderOptions[T gai.VectorComponent] struct {
	// Embedders is the prioritized list of underlying embedders. Must be non-empty.
	Embedders []gai.Embedder[T]
	// MaxAttempts per embedder. Defaults to 3. Set to 1 to disable retrying.
	MaxAttempts int
	// BaseDelay is the initial exponential-backoff delay. Defaults to 100ms.
	BaseDelay time.Duration
	// MaxDelay caps the backoff sleep. Defaults to 5s.
	MaxDelay time.Duration
	// ErrorClassifier decides how to handle errors. Defaults to a conservative built-in.
	ErrorClassifier ErrorClassifierFunc
	// Log receives debug messages on failover and final exhaustion. Defaults to discarding output.
	Log *slog.Logger
}

// NewEmbedder constructs an [Embedder]. Panics if:
//   - Embedders is empty,
//   - MaxAttempts, BaseDelay, or MaxDelay is negative,
//   - MaxDelay equals [math.MaxInt64],
//   - BaseDelay exceeds MaxDelay.
func NewEmbedder[T gai.VectorComponent](opts NewEmbedderOptions[T]) *Embedder[T] {
	if len(opts.Embedders) == 0 {
		panic("robust: Embedders must not be empty")
	}
	if opts.MaxAttempts < 0 {
		panic("robust: MaxAttempts must not be negative")
	}
	if opts.MaxAttempts == 0 {
		opts.MaxAttempts = 3
	}
	if opts.BaseDelay < 0 {
		panic("robust: BaseDelay must not be negative")
	}
	if opts.BaseDelay == 0 {
		opts.BaseDelay = 100 * time.Millisecond
	}
	if opts.MaxDelay < 0 {
		panic("robust: MaxDelay must not be negative")
	}
	if opts.MaxDelay == 0 {
		opts.MaxDelay = 5 * time.Second
	}
	if opts.MaxDelay == time.Duration(math.MaxInt64) {
		panic("robust: MaxDelay must be less than math.MaxInt64")
	}
	if opts.BaseDelay > opts.MaxDelay {
		panic("robust: BaseDelay must not exceed MaxDelay")
	}
	if opts.ErrorClassifier == nil {
		opts.ErrorClassifier = defaultErrorClassifier
	}
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	return &Embedder[T]{
		embedders:   opts.Embedders,
		maxAttempts: opts.MaxAttempts,
		baseDelay:   opts.BaseDelay,
		maxDelay:    opts.MaxDelay,
		classifier:  opts.ErrorClassifier,
		log:         opts.Log,
		tracer:      otel.Tracer("maragu.dev/gai/robust"),
	}
}

// Embed satisfies [gai.Embedder].
func (e *Embedder[T]) Embed(ctx context.Context, req gai.EmbedRequest) (gai.EmbedResponse[T], error) {
	ctx, rootSpan := e.tracer.Start(ctx, "robust.embed",
		trace.WithAttributes(
			attribute.Int("ai.robust.completer_count", len(e.embedders)),
			attribute.Int("ai.robust.max_attempts", e.maxAttempts),
			attribute.Int64("ai.robust.base_delay_ms", e.baseDelay.Milliseconds()),
			attribute.Int64("ai.robust.max_delay_ms", e.maxDelay.Milliseconds()),
		),
	)
	defer rootSpan.End()

	var lastErr error
	for embedderIdx, embedder := range e.embedders {
		fallback := false
		for attempt := 1; attempt <= e.maxAttempts && !fallback; attempt++ {
			res, act, err := e.tryOnce(ctx, embedder, req, embedderIdx, attempt)
			if err == nil {
				return res, nil
			}
			lastErr = err

			switch act {
			case ActionFail:
				rootSpan.RecordError(err)
				rootSpan.SetStatus(codes.Error, "classifier returned fail")
				return gai.EmbedResponse[T]{}, err
			case ActionFallback:
				fallback = true
			case ActionRetry:
				if attempt < e.maxAttempts {
					if sleepErr := sleep(ctx, e.baseDelay, e.maxDelay, attempt); sleepErr != nil {
						rootSpan.RecordError(sleepErr)
						rootSpan.SetStatus(codes.Error, "context cancelled during backoff")
						return gai.EmbedResponse[T]{}, sleepErr
					}
				}
			default:
				panic(fmt.Sprintf("robust: classifier returned unknown Action %d", act))
			}
		}
		if embedderIdx < len(e.embedders)-1 {
			e.log.Debug("robust: falling over to next embedder",
				"from_index", embedderIdx, "to_index", embedderIdx+1, "error", lastErr)
		}
	}

	e.log.Debug("robust: all embedders exhausted", "final_error", lastErr)
	rootSpan.RecordError(lastErr)
	rootSpan.SetStatus(codes.Error, "all embedders exhausted")
	if lastErr == nil {
		lastErr = errors.New("robust: no embedders attempted")
	}
	return gai.EmbedResponse[T]{}, lastErr
}

// tryOnce runs a single Embed attempt against one embedder. Returns (res, ActionNone, nil) on
// success; (zero, classifiedAction, err) on failure. Ends the attempt span before returning.
func (e *Embedder[T]) tryOnce(ctx context.Context, embedder gai.Embedder[T], req gai.EmbedRequest, embedderIdx, attempt int) (gai.EmbedResponse[T], Action, error) {
	ctx, attemptSpan := e.tracer.Start(ctx, "robust.embed_attempt",
		trace.WithAttributes(
			attribute.Int("ai.robust.completer_index", embedderIdx),
			attribute.Int("ai.robust.attempt_number", attempt),
		),
	)
	defer attemptSpan.End()

	res, err := embedder.Embed(ctx, req)
	if err == nil {
		attemptSpan.SetAttributes(attribute.String("ai.robust.action", "success"))
		return res, ActionNone, nil
	}

	act := e.classifier(err)
	attemptSpan.SetAttributes(attribute.String("ai.robust.action", act.String()))
	attemptSpan.RecordError(err)
	attemptSpan.SetStatus(codes.Error, act.String())
	return gai.EmbedResponse[T]{}, act, err
}

var _ gai.Embedder[float64] = (*Embedder[float64])(nil)
