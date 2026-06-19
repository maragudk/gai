package robust_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/internal/oteltest"
	"maragu.dev/gai/robust"
)

// fakeEmbedder drives Embed scenarios via a queue of responses. Each call pops the next one;
// running out fails the test.
type fakeEmbedder[T gai.VectorComponent] struct {
	t         *testing.T
	name      string
	responses []fakeEmbedResponse[T]
	calls     int
}

type fakeEmbedResponse[T gai.VectorComponent] struct {
	err       error
	embedding []T
}

func newFakeEmbedder[T gai.VectorComponent](t *testing.T, name string, responses []fakeEmbedResponse[T]) *fakeEmbedder[T] {
	t.Helper()
	return &fakeEmbedder[T]{t: t, name: name, responses: responses}
}

func (f *fakeEmbedder[T]) Embed(_ context.Context, _ gai.EmbedRequest) (gai.EmbedResponse[T], error) {
	f.t.Helper()
	if f.calls >= len(f.responses) {
		f.t.Fatalf("fakeEmbedder %s: no more queued responses", f.name)
	}
	r := f.responses[f.calls]
	f.calls++
	if r.err != nil {
		return gai.EmbedResponse[T]{}, r.err
	}
	return gai.EmbedResponse[T]{Embedding: r.embedding}, nil
}

func TestEmbedder_Embed(t *testing.T) {
	t.Run("succeeds on first try when primary embedder returns no error", func(t *testing.T) {
		primary := newFakeEmbedder(t, "primary", []fakeEmbedResponse[float32]{
			{embedding: []float32{0.1, 0.2, 0.3}},
		})

		e := robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
			Embedders: []gai.Embedder[float32]{primary},
		})

		res, err := e.Embed(t.Context(), gai.NewTextEmbedRequest("hi"))
		is.NotError(t, err)
		is.EqualSlice(t, []float32{0.1, 0.2, 0.3}, res.Embedding)
		is.Equal(t, 1, primary.calls)
	})

	t.Run("retries a transient error then succeeds on the second attempt", func(t *testing.T) {
		primary := newFakeEmbedder(t, "primary", []fakeEmbedResponse[float32]{
			{err: errors.New("transient sparkle storm")},
			{embedding: []float32{1, 2, 3}},
		})

		e := robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
			Embedders: []gai.Embedder[float32]{primary},
			BaseDelay: time.Nanosecond,
			MaxDelay:  time.Nanosecond,
		})

		res, err := e.Embed(t.Context(), gai.EmbedRequest{})
		is.NotError(t, err)
		is.EqualSlice(t, []float32{1, 2, 3}, res.Embedding)
		is.Equal(t, 2, primary.calls)
	})

	t.Run("bubbles up context.Canceled immediately without falling back", func(t *testing.T) {
		primary := newFakeEmbedder(t, "primary", []fakeEmbedResponse[float32]{{err: context.Canceled}})
		secondary := newFakeEmbedder(t, "secondary", []fakeEmbedResponse[float32]{
			{embedding: []float32{9, 9, 9}},
		})

		e := robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
			Embedders: []gai.Embedder[float32]{primary, secondary},
			BaseDelay: time.Nanosecond,
			MaxDelay:  time.Nanosecond,
		})

		_, err := e.Embed(t.Context(), gai.EmbedRequest{})
		is.Error(t, context.Canceled, err)
		is.Equal(t, 1, primary.calls)
		is.Equal(t, 0, secondary.calls)
	})

	t.Run("interrupts the backoff sleep when the caller cancels the context mid-sleep", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			primary := newFakeEmbedder(t, "primary", []fakeEmbedResponse[float32]{
				{err: errors.New("first fails")},
				{err: errors.New("should not be called")},
			})

			ctx, cancel := context.WithCancel(t.Context())

			e := robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
				Embedders:   []gai.Embedder[float32]{primary},
				MaxAttempts: 3,
				BaseDelay:   time.Hour,
				MaxDelay:    time.Hour,
			})

			go func() {
				time.Sleep(10 * time.Millisecond)
				cancel()
			}()

			_, err := e.Embed(ctx, gai.EmbedRequest{})
			is.Error(t, context.Canceled, err)
			is.Equal(t, 1, primary.calls)
		})
	})

	t.Run("skips remaining retries and falls back when classifier returns ActionFallback", func(t *testing.T) {
		fallbackErr := errors.New("confetti jam")
		primary := newFakeEmbedder(t, "primary", []fakeEmbedResponse[float32]{{err: fallbackErr}})
		secondary := newFakeEmbedder(t, "secondary", []fakeEmbedResponse[float32]{
			{embedding: []float32{4, 2}},
		})

		e := robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
			Embedders: []gai.Embedder[float32]{primary, secondary},
			BaseDelay: time.Nanosecond,
			MaxDelay:  time.Nanosecond,
			ErrorClassifier: func(err error) robust.Action {
				if errors.Is(err, fallbackErr) {
					return robust.ActionFallback
				}
				return robust.ActionRetry
			},
		})

		res, err := e.Embed(t.Context(), gai.EmbedRequest{})
		is.NotError(t, err)
		is.EqualSlice(t, []float32{4, 2}, res.Embedding)
		is.Equal(t, 1, primary.calls)
		is.Equal(t, 1, secondary.calls)
	})

	t.Run("exhausts MaxAttempts retries then falls back to the next embedder", func(t *testing.T) {
		primary := newFakeEmbedder(t, "primary", []fakeEmbedResponse[float32]{
			{err: errors.New("flake 1")},
			{err: errors.New("flake 2")},
		})
		secondary := newFakeEmbedder(t, "secondary", []fakeEmbedResponse[float32]{
			{embedding: []float32{7, 7}},
		})

		e := robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
			Embedders:   []gai.Embedder[float32]{primary, secondary},
			MaxAttempts: 2,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
		})

		res, err := e.Embed(t.Context(), gai.EmbedRequest{})
		is.NotError(t, err)
		is.EqualSlice(t, []float32{7, 7}, res.Embedding)
		is.Equal(t, 2, primary.calls)
		is.Equal(t, 1, secondary.calls)
	})

	t.Run("returns the final error when all embedders are exhausted", func(t *testing.T) {
		finalErr := errors.New("final sparkle failure")
		primary := newFakeEmbedder(t, "primary", []fakeEmbedResponse[float32]{
			{err: errors.New("p1")},
			{err: errors.New("p2")},
		})
		secondary := newFakeEmbedder(t, "secondary", []fakeEmbedResponse[float32]{
			{err: errors.New("s1")},
			{err: finalErr},
		})

		e := robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
			Embedders:   []gai.Embedder[float32]{primary, secondary},
			MaxAttempts: 2,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
		})

		_, err := e.Embed(t.Context(), gai.EmbedRequest{})
		is.Error(t, finalErr, err)
		is.Equal(t, 2, primary.calls)
		is.Equal(t, 2, secondary.calls)
	})

	t.Run("defaults MaxAttempts to 3 when zero", func(t *testing.T) {
		primary := newFakeEmbedder(t, "primary", []fakeEmbedResponse[float32]{
			{err: errors.New("a")},
			{err: errors.New("b")},
			{err: errors.New("c")},
		})
		secondary := newFakeEmbedder(t, "secondary", []fakeEmbedResponse[float32]{
			{embedding: []float32{1}},
		})

		e := robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
			Embedders: []gai.Embedder[float32]{primary, secondary},
			BaseDelay: time.Nanosecond,
			MaxDelay:  time.Nanosecond,
		})

		_, err := e.Embed(t.Context(), gai.EmbedRequest{})
		is.NotError(t, err)
		is.Equal(t, 3, primary.calls)
		is.Equal(t, 1, secondary.calls)
	})

	t.Run("does not retry when MaxAttempts is 1", func(t *testing.T) {
		primary := newFakeEmbedder(t, "primary", []fakeEmbedResponse[float32]{
			{err: errors.New("one shot")},
		})
		secondary := newFakeEmbedder(t, "secondary", []fakeEmbedResponse[float32]{
			{embedding: []float32{0}},
		})

		e := robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
			Embedders:   []gai.Embedder[float32]{primary, secondary},
			MaxAttempts: 1,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
		})

		_, err := e.Embed(t.Context(), gai.EmbedRequest{})
		is.NotError(t, err)
		is.Equal(t, 1, primary.calls)
		is.Equal(t, 1, secondary.calls)
	})

	t.Run("uses the default classifier when none is provided", func(t *testing.T) {
		primary := newFakeEmbedder(t, "primary", []fakeEmbedResponse[float32]{
			{err: context.DeadlineExceeded},
		})

		e := robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
			Embedders: []gai.Embedder[float32]{primary},
			BaseDelay: time.Nanosecond,
			MaxDelay:  time.Nanosecond,
		})

		_, err := e.Embed(t.Context(), gai.EmbedRequest{})
		is.Error(t, context.DeadlineExceeded, err)
		is.Equal(t, 1, primary.calls)
	})

	t.Run("panics when the classifier returns an unknown Action", func(t *testing.T) {
		primary := newFakeEmbedder(t, "primary", []fakeEmbedResponse[float32]{
			{err: errors.New("anything")},
		})

		e := robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
			Embedders: []gai.Embedder[float32]{primary},
			BaseDelay: time.Nanosecond,
			MaxDelay:  time.Nanosecond,
			ErrorClassifier: func(error) robust.Action {
				return robust.Action(999)
			},
		})

		defer func() {
			r := recover()
			is.True(t, r != nil, "expected panic from unknown Action")
		}()

		_, _ = e.Embed(t.Context(), gai.EmbedRequest{})
	})

	t.Run("panics when Embedders is empty", func(t *testing.T) {
		defer func() {
			r := recover()
			is.Equal(t, "Embedders must not be empty", r)
		}()

		robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{})
	})

	t.Run("panics when MaxAttempts is negative", func(t *testing.T) {
		defer func() {
			r := recover()
			is.Equal(t, "MaxAttempts must not be negative", r)
		}()

		robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
			Embedders:   []gai.Embedder[float32]{newFakeEmbedder[float32](t, "p", nil)},
			MaxAttempts: -1,
		})
	})

	t.Run("panics when BaseDelay exceeds MaxDelay", func(t *testing.T) {
		defer func() {
			r := recover()
			is.Equal(t, "BaseDelay must not exceed MaxDelay", r)
		}()

		robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
			Embedders: []gai.Embedder[float32]{newFakeEmbedder[float32](t, "p", nil)},
			BaseDelay: 10 * time.Second,
			MaxDelay:  time.Second,
		})
	})

	t.Run("works with float64 component type as well", func(t *testing.T) {
		primary := newFakeEmbedder(t, "primary", []fakeEmbedResponse[float64]{
			{embedding: []float64{0.1, 0.2}},
		})

		e := robust.NewEmbedder[float64](robust.NewEmbedderOptions[float64]{
			Embedders: []gai.Embedder[float64]{primary},
		})

		res, err := e.Embed(t.Context(), gai.NewTextEmbedRequest("hi"))
		is.NotError(t, err)
		is.EqualSlice(t, []float64{0.1, 0.2}, res.Embedding)
	})

	t.Run("emits a root span with config and one success attempt span on first-try success", func(t *testing.T) {
		sr := oteltest.NewSpanRecorder(t)

		primary := newFakeEmbedder(t, "primary", []fakeEmbedResponse[float32]{
			{embedding: []float32{0.1, 0.2}},
		})

		e := robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
			Embedders:   []gai.Embedder[float32]{primary},
			MaxAttempts: 2,
			BaseDelay:   time.Millisecond,
			MaxDelay:    time.Second,
		})

		_, err := e.Embed(t.Context(), gai.NewTextEmbedRequest("hi"))
		is.NotError(t, err)

		root := oteltest.FindSpan(t, sr.Ended(), "robust.embed")
		is.True(t, oteltest.HasAttribute(root.Attributes(), attribute.Int("ai.robust.embedder_count", 1)))
		is.True(t, oteltest.HasAttribute(root.Attributes(), attribute.Int("ai.robust.max_attempts", 2)))
		is.True(t, oteltest.HasAttribute(root.Attributes(), attribute.Int64("ai.robust.base_delay_ms", 1)))
		is.True(t, oteltest.HasAttribute(root.Attributes(), attribute.Int64("ai.robust.max_delay_ms", 1000)))

		attempts := oteltest.SpansByName(sr.Ended(), "robust.embed_attempt")
		is.Equal(t, 1, len(attempts))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.Int("ai.robust.embedder_index", 0)))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.Int("ai.robust.attempt_number", 1)))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.String("ai.robust.action", "success")))
	})

	t.Run("records a fallback action on the primary attempt and success on the secondary", func(t *testing.T) {
		sr := oteltest.NewSpanRecorder(t)

		fallbackErr := errors.New("confetti jam")
		primary := newFakeEmbedder(t, "primary", []fakeEmbedResponse[float32]{{err: fallbackErr}})
		secondary := newFakeEmbedder(t, "secondary", []fakeEmbedResponse[float32]{{embedding: []float32{4, 2}}})

		e := robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
			Embedders: []gai.Embedder[float32]{primary, secondary},
			BaseDelay: time.Nanosecond,
			MaxDelay:  time.Nanosecond,
			ErrorClassifier: func(err error) robust.Action {
				if errors.Is(err, fallbackErr) {
					return robust.ActionFallback
				}
				return robust.ActionRetry
			},
		})

		_, err := e.Embed(t.Context(), gai.EmbedRequest{})
		is.NotError(t, err)

		attempts := oteltest.SpansByName(sr.Ended(), "robust.embed_attempt")
		is.Equal(t, 2, len(attempts))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.Int("ai.robust.embedder_index", 0)))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.String("ai.robust.action", "fallback")))
		is.True(t, oteltest.HasAttribute(attempts[1].Attributes(), attribute.Int("ai.robust.embedder_index", 1)))
		is.True(t, oteltest.HasAttribute(attempts[1].Attributes(), attribute.String("ai.robust.action", "success")))
	})

	t.Run("marks the root span as an error when all embedders are exhausted", func(t *testing.T) {
		sr := oteltest.NewSpanRecorder(t)

		primary := newFakeEmbedder(t, "primary", []fakeEmbedResponse[float32]{
			{err: errors.New("p1")},
			{err: errors.New("p2")},
		})

		e := robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
			Embedders:   []gai.Embedder[float32]{primary},
			MaxAttempts: 2,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
		})

		_, err := e.Embed(t.Context(), gai.EmbedRequest{})
		is.True(t, err != nil)

		root := oteltest.FindSpan(t, sr.Ended(), "robust.embed")
		is.Equal(t, "all embedders exhausted", root.Status().Description)
	})
}
