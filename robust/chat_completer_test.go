package robust_test

import (
	"context"
	"errors"
	"iter"
	"testing"
	"testing/synctest"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/internal/oteltest"
	"maragu.dev/gai/robust"
)

// fakeChatCompleter drives scenarios by consuming queued responses on each call.
// Each call pops the next fakeResponse from the queue; running out fails the test.
type fakeChatCompleter struct {
	t         *testing.T
	name      string
	responses []fakeResponse
	calls     int
}

type fakeResponse struct {
	preStreamErr error      // returned from ChatComplete
	parts        []gai.Part // parts yielded in order
	iterErr      error      // error yielded after parts (or before, if errBeforeFirstPart)
	meta         *gai.ChatCompleteResponseMetadata
	// hangBeforeStream blocks ChatComplete until the attempt context is done, then returns
	// its error. Simulates a backend that hangs before delivering any part.
	hangBeforeStream bool
	// hangInStream returns a response whose iterator blocks until the attempt context is done,
	// then yields ctx.Err() — before any part. Simulates a backend that returns promptly but
	// stalls before the first streamed part, exercising the timeout via commitOnFirstPart.
	hangInStream bool
	// partDelay sleeps before yielding each part. Simulates a slow-but-healthy stream that
	// keeps streaming past the per-attempt timeout once committed.
	partDelay time.Duration
}

// newFakeChatCompleter constructs a fakeChatCompleter bound to t.
func newFakeChatCompleter(t *testing.T, name string, responses []fakeResponse) *fakeChatCompleter {
	t.Helper()
	return &fakeChatCompleter{t: t, name: name, responses: responses}
}

func (f *fakeChatCompleter) ChatComplete(ctx context.Context, _ gai.ChatCompleteRequest) (gai.ChatCompleteResponse, error) {
	f.t.Helper()
	if f.calls >= len(f.responses) {
		f.t.Fatalf("fakeChatCompleter %s: no more queued responses", f.name)
	}
	r := f.responses[f.calls]
	f.calls++

	if r.hangBeforeStream {
		<-ctx.Done()
		return gai.ChatCompleteResponse{}, ctx.Err()
	}

	if r.preStreamErr != nil {
		return gai.ChatCompleteResponse{}, r.preStreamErr
	}

	meta := r.meta
	if meta == nil {
		meta = &gai.ChatCompleteResponseMetadata{}
	}

	partsFunc := func(yield func(gai.Part, error) bool) {
		if r.hangInStream {
			<-ctx.Done()
			yield(gai.Part{}, ctx.Err())
			return
		}
		for i, p := range r.parts {
			// partDelay slows the stream after the first part, modelling a healthy stream that
			// commits quickly then runs longer than the per-attempt timeout. The delay is
			// context-aware: if the attempt context were cancelled mid-stream (e.g. a timer
			// that kept running past commit), the delay yields ctx.Err() instead of the part,
			// so the test observes the cancellation rather than silently completing.
			if r.partDelay > 0 && i > 0 {
				select {
				case <-time.After(r.partDelay):
				case <-ctx.Done():
					yield(gai.Part{}, ctx.Err())
					return
				}
			}
			if !yield(p, nil) {
				return
			}
		}
		if r.iterErr != nil {
			yield(gai.Part{}, r.iterErr)
		}
	}

	res := gai.NewChatCompleteResponse(iter.Seq2[gai.Part, error](partsFunc))
	res.Meta = meta
	return res, nil
}

// collectParts drains the response into a slice plus a terminal error.
func collectParts(t *testing.T, res gai.ChatCompleteResponse) ([]gai.Part, error) {
	t.Helper()
	var parts []gai.Part
	for p, err := range res.Parts() {
		if err != nil {
			return parts, err
		}
		parts = append(parts, p)
	}
	return parts, nil
}

func TestChatCompleter_ChatComplete(t *testing.T) {
	t.Run("succeeds on first try when primary completer returns no errors", func(t *testing.T) {
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{{
			parts: []gai.Part{gai.TextPart("hello, markus")},
		}})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{primary},
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{
			Messages: []gai.Message{gai.NewUserTextMessage("hi")},
		})
		is.NotError(t, err)

		parts, err := collectParts(t, res)
		is.NotError(t, err)
		is.Equal(t, 1, len(parts))
		is.Equal(t, "hello, markus", parts[0].Text())
		is.Equal(t, 1, primary.calls)
	})

	t.Run("retries a pre-stream error and succeeds on the second attempt", func(t *testing.T) {
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{preStreamErr: errors.New("transient glitter storm")},
			{parts: []gai.Part{gai.TextPart("ok")}},
		})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{primary},
			BaseDelay:  time.Nanosecond,
			MaxDelay:   time.Nanosecond,
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)

		parts, err := collectParts(t, res)
		is.NotError(t, err)
		is.Equal(t, 1, len(parts))
		is.Equal(t, "ok", parts[0].Text())
		is.Equal(t, 2, primary.calls)
	})

	t.Run("bubbles up context.Canceled immediately without falling back", func(t *testing.T) {
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{{preStreamErr: context.Canceled}})
		secondary := newFakeChatCompleter(t, "secondary", []fakeResponse{{parts: []gai.Part{gai.TextPart("should not happen")}}})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{primary, secondary},
			BaseDelay:  time.Nanosecond,
			MaxDelay:   time.Nanosecond,
		})

		_, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.Error(t, context.Canceled, err)
		is.Equal(t, 1, primary.calls)
		is.Equal(t, 0, secondary.calls)
	})

	t.Run("interrupts the backoff sleep when the caller cancels the context mid-sleep", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			primary := newFakeChatCompleter(t, "primary", []fakeResponse{
				{preStreamErr: errors.New("first fails")},
				{preStreamErr: errors.New("should not be called")},
			})

			ctx, cancel := context.WithCancel(t.Context())

			cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
				Completers:  []gai.ChatCompleter{primary},
				MaxAttempts: 3,
				BaseDelay:   time.Hour,
				MaxDelay:    time.Hour,
			})

			go func() {
				time.Sleep(10 * time.Millisecond)
				cancel()
			}()

			_, err := cc.ChatComplete(ctx, gai.ChatCompleteRequest{})
			is.Error(t, context.Canceled, err)
			is.Equal(t, 1, primary.calls)
		})
	})

	t.Run("returns the final error when all completers are exhausted", func(t *testing.T) {
		finalErr := errors.New("final failure")
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{preStreamErr: errors.New("p1")},
			{preStreamErr: errors.New("p2")},
		})
		secondary := newFakeChatCompleter(t, "secondary", []fakeResponse{
			{preStreamErr: errors.New("s1")},
			{preStreamErr: finalErr},
		})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:  []gai.ChatCompleter{primary, secondary},
			MaxAttempts: 2,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
		})

		_, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.Error(t, finalErr, err)
		is.Equal(t, 2, primary.calls)
		is.Equal(t, 2, secondary.calls)
	})

	t.Run("defaults MaxAttempts to 3 when zero", func(t *testing.T) {
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{preStreamErr: errors.New("a")},
			{preStreamErr: errors.New("b")},
			{preStreamErr: errors.New("c")},
		})
		secondary := newFakeChatCompleter(t, "secondary", []fakeResponse{{parts: []gai.Part{gai.TextPart("saved")}}})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{primary, secondary},
			BaseDelay:  time.Nanosecond,
			MaxDelay:   time.Nanosecond,
		})

		_, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		is.Equal(t, 3, primary.calls)
		is.Equal(t, 1, secondary.calls)
	})

	t.Run("panics when Completers is empty", func(t *testing.T) {
		defer func() {
			r := recover()
			is.Equal(t, "Completers must not be empty", r)
		}()

		robust.NewChatCompleter(robust.NewChatCompleterOptions{})
	})

	t.Run("uses the default classifier when none is provided", func(t *testing.T) {
		// context.Canceled should bubble up via the default classifier.
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{{preStreamErr: context.DeadlineExceeded}})
		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{primary},
			BaseDelay:  time.Nanosecond,
			MaxDelay:   time.Nanosecond,
		})

		_, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.Error(t, context.DeadlineExceeded, err)
		is.Equal(t, 1, primary.calls)
	})

	t.Run("does not retry when MaxAttempts is 1", func(t *testing.T) {
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{preStreamErr: errors.New("one and done")},
		})
		secondary := newFakeChatCompleter(t, "secondary", []fakeResponse{{parts: []gai.Part{gai.TextPart("saved")}}})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:  []gai.ChatCompleter{primary, secondary},
			MaxAttempts: 1,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		_, err = collectParts(t, res)
		is.NotError(t, err)
		is.Equal(t, 1, primary.calls)
		is.Equal(t, 1, secondary.calls)
	})

	t.Run("retries when the underlying completer yields an empty stream", func(t *testing.T) {
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{}, // empty: no parts, no error
			{parts: []gai.Part{gai.TextPart("recovered")}},
		})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{primary},
			BaseDelay:  time.Nanosecond,
			MaxDelay:   time.Nanosecond,
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		parts, err := collectParts(t, res)
		is.NotError(t, err)
		is.Equal(t, 1, len(parts))
		is.Equal(t, "recovered", parts[0].Text())
		is.Equal(t, 2, primary.calls)
	})

	t.Run("panics when the classifier returns an unknown Action", func(t *testing.T) {
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{preStreamErr: errors.New("anything")},
		})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{primary},
			BaseDelay:  time.Nanosecond,
			MaxDelay:   time.Nanosecond,
			ErrorClassifier: func(error) robust.Action {
				return robust.Action(999)
			},
		})

		defer func() {
			r := recover()
			is.True(t, r != nil, "expected panic from unknown Action")
		}()

		_, _ = cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
	})

	t.Run("panics when MaxAttempts is negative", func(t *testing.T) {
		defer func() {
			r := recover()
			is.Equal(t, "MaxAttempts must not be negative", r)
		}()

		robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:  []gai.ChatCompleter{newFakeChatCompleter(t, "p", nil)},
			MaxAttempts: -1,
		})
	})

	t.Run("panics when BaseDelay exceeds MaxDelay", func(t *testing.T) {
		defer func() {
			r := recover()
			is.Equal(t, "BaseDelay must not exceed MaxDelay", r)
		}()

		robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{newFakeChatCompleter(t, "p", nil)},
			BaseDelay:  10 * time.Second,
			MaxDelay:   time.Second,
		})
	})

	t.Run("forwards the Meta pointer from the succeeding completer", func(t *testing.T) {
		finishReason := gai.ChatCompleteFinishReasonStop
		meta := &gai.ChatCompleteResponseMetadata{
			Usage:        gai.ChatCompleteResponseUsage{PromptTokens: 42},
			FinishReason: &finishReason,
		}
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{{
			parts: []gai.Part{gai.TextPart("ok")},
			meta:  meta,
		}})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{primary},
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		is.True(t, res.Meta == meta, "Meta pointer should be the underlying one")
		_, _ = collectParts(t, res)
	})

	t.Run("exhausts MaxAttempts retries then falls back to the next completer", func(t *testing.T) {
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{preStreamErr: errors.New("flake 1")},
			{preStreamErr: errors.New("flake 2")},
		})
		secondary := newFakeChatCompleter(t, "secondary", []fakeResponse{{parts: []gai.Part{gai.TextPart("saved")}}})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:  []gai.ChatCompleter{primary, secondary},
			MaxAttempts: 2,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		parts, err := collectParts(t, res)
		is.NotError(t, err)
		is.Equal(t, 1, len(parts))
		is.Equal(t, "saved", parts[0].Text())
		is.Equal(t, 2, primary.calls)
		is.Equal(t, 1, secondary.calls)
	})

	t.Run("skips remaining retries and falls back when classifier returns ActionFallback", func(t *testing.T) {
		fallbackErr := errors.New("permanent disco failure")
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{{preStreamErr: fallbackErr}})
		secondary := newFakeChatCompleter(t, "secondary", []fakeResponse{{parts: []gai.Part{gai.TextPart("from secondary")}}})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{primary, secondary},
			BaseDelay:  time.Nanosecond,
			MaxDelay:   time.Nanosecond,
			ErrorClassifier: func(err error) robust.Action {
				if errors.Is(err, fallbackErr) {
					return robust.ActionFallback
				}
				return robust.ActionRetry
			},
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		parts, err := collectParts(t, res)
		is.NotError(t, err)
		is.Equal(t, 1, len(parts))
		is.Equal(t, "from secondary", parts[0].Text())
		is.Equal(t, 1, primary.calls)
		is.Equal(t, 1, secondary.calls)
	})

	t.Run("passes a mid-stream error through to the caller without retrying", func(t *testing.T) {
		midStreamErr := errors.New("glitter ran out mid-sentence")
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{{
			parts:   []gai.Part{gai.TextPart("hello, ")},
			iterErr: midStreamErr,
		}})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{primary},
			BaseDelay:  time.Nanosecond,
			MaxDelay:   time.Nanosecond,
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)

		parts, err := collectParts(t, res)
		is.Error(t, midStreamErr, err)
		is.Equal(t, 1, len(parts))
		is.Equal(t, "hello, ", parts[0].Text())
		is.Equal(t, 1, primary.calls)
	})

	t.Run("retries when the iterator yields an error before the first part is emitted", func(t *testing.T) {
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{iterErr: errors.New("early stream failure")},
			{parts: []gai.Part{gai.TextPart("recovered")}},
		})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{primary},
			BaseDelay:  time.Nanosecond,
			MaxDelay:   time.Nanosecond,
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)

		parts, err := collectParts(t, res)
		is.NotError(t, err)
		is.Equal(t, 1, len(parts))
		is.Equal(t, "recovered", parts[0].Text())
		is.Equal(t, 2, primary.calls)
	})

	t.Run("emits a root span with config and one success attempt span on first-try success", func(t *testing.T) {
		sr := oteltest.NewSpanRecorder(t)

		primary := newFakeChatCompleter(t, "primary", []fakeResponse{{
			parts: []gai.Part{gai.TextPart("ok")},
		}})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:  []gai.ChatCompleter{primary},
			MaxAttempts: 2,
			BaseDelay:   time.Millisecond,
			MaxDelay:    time.Second,
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		_, err = collectParts(t, res)
		is.NotError(t, err)

		root := oteltest.FindSpan(t, sr.Ended(), "robust.chat_complete")
		is.True(t, oteltest.HasAttribute(root.Attributes(), attribute.Int("ai.robust.completer_count", 1)))
		is.True(t, oteltest.HasAttribute(root.Attributes(), attribute.Int("ai.robust.max_attempts", 2)))
		is.True(t, oteltest.HasAttribute(root.Attributes(), attribute.Int64("ai.robust.base_delay_ms", 1)))
		is.True(t, oteltest.HasAttribute(root.Attributes(), attribute.Int64("ai.robust.max_delay_ms", 1000)))

		attempts := oteltest.SpansByName(sr.Ended(), "robust.chat_complete_attempt")
		is.Equal(t, 1, len(attempts))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.Int("ai.robust.completer_index", 0)))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.Int("ai.robust.attempt_number", 1)))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.String("ai.robust.action", "success")))
	})

	t.Run("records a retry action on the first attempt and success on the second after a transient error", func(t *testing.T) {
		sr := oteltest.NewSpanRecorder(t)

		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{preStreamErr: errors.New("transient glitter storm")},
			{parts: []gai.Part{gai.TextPart("ok")}},
		})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{primary},
			BaseDelay:  time.Nanosecond,
			MaxDelay:   time.Nanosecond,
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		_, err = collectParts(t, res)
		is.NotError(t, err)

		attempts := oteltest.SpansByName(sr.Ended(), "robust.chat_complete_attempt")
		is.Equal(t, 2, len(attempts))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.Int("ai.robust.attempt_number", 1)))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.String("ai.robust.action", "retry")))
		is.True(t, oteltest.HasAttribute(attempts[1].Attributes(), attribute.Int("ai.robust.attempt_number", 2)))
		is.True(t, oteltest.HasAttribute(attempts[1].Attributes(), attribute.String("ai.robust.action", "success")))
	})

	t.Run("records a fallback action on the primary attempt and success on the secondary", func(t *testing.T) {
		sr := oteltest.NewSpanRecorder(t)

		fallbackErr := errors.New("permanent disco failure")
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{{preStreamErr: fallbackErr}})
		secondary := newFakeChatCompleter(t, "secondary", []fakeResponse{{parts: []gai.Part{gai.TextPart("saved")}}})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{primary, secondary},
			BaseDelay:  time.Nanosecond,
			MaxDelay:   time.Nanosecond,
			ErrorClassifier: func(err error) robust.Action {
				if errors.Is(err, fallbackErr) {
					return robust.ActionFallback
				}
				return robust.ActionRetry
			},
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		_, err = collectParts(t, res)
		is.NotError(t, err)

		attempts := oteltest.SpansByName(sr.Ended(), "robust.chat_complete_attempt")
		is.Equal(t, 2, len(attempts))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.Int("ai.robust.completer_index", 0)))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.String("ai.robust.action", "fallback")))
		is.True(t, oteltest.HasAttribute(attempts[1].Attributes(), attribute.Int("ai.robust.completer_index", 1)))
		is.True(t, oteltest.HasAttribute(attempts[1].Attributes(), attribute.String("ai.robust.action", "success")))
	})

	t.Run("marks the root span as an error when all completers are exhausted", func(t *testing.T) {
		sr := oteltest.NewSpanRecorder(t)

		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{preStreamErr: errors.New("p1")},
			{preStreamErr: errors.New("p2")},
		})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:  []gai.ChatCompleter{primary},
			MaxAttempts: 2,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
		})

		_, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.True(t, err != nil)

		root := oteltest.FindSpan(t, sr.Ended(), "robust.chat_complete")
		is.Equal(t, "all completers exhausted", root.Status().Description)

		attempts := oteltest.SpansByName(sr.Ended(), "robust.chat_complete_attempt")
		is.Equal(t, 2, len(attempts))
		for _, a := range attempts {
			is.True(t, oteltest.HasAttribute(a.Attributes(), attribute.String("ai.robust.action", "retry")))
		}
	})

	t.Run("retries a primary that hangs before the first part under AttemptTimeout then falls over", func(t *testing.T) {
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{hangBeforeStream: true},
			{hangBeforeStream: true},
		})
		secondary := newFakeChatCompleter(t, "secondary", []fakeResponse{
			{parts: []gai.Part{gai.TextPart("saved")}},
		})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:     []gai.ChatCompleter{primary, secondary},
			MaxAttempts:    2,
			BaseDelay:      time.Millisecond,
			MaxDelay:       time.Millisecond,
			AttemptTimeout: 10 * time.Millisecond,
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		parts, err := collectParts(t, res)
		is.NotError(t, err)
		is.Equal(t, 1, len(parts))
		is.Equal(t, "saved", parts[0].Text())
		is.Equal(t, 2, primary.calls)
		is.Equal(t, 1, secondary.calls)
	})

	t.Run("retries a hung backend up to MaxAttempts with a fresh clock each time then falls over", func(t *testing.T) {
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{hangBeforeStream: true},
			{hangBeforeStream: true},
			{hangBeforeStream: true},
		})
		secondary := newFakeChatCompleter(t, "secondary", []fakeResponse{
			{parts: []gai.Part{gai.TextPart("saved")}},
		})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:     []gai.ChatCompleter{primary, secondary},
			MaxAttempts:    3,
			BaseDelay:      time.Millisecond,
			MaxDelay:       time.Millisecond,
			AttemptTimeout: 10 * time.Millisecond,
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		parts, err := collectParts(t, res)
		is.NotError(t, err)
		is.Equal(t, "saved", parts[0].Text())
		is.Equal(t, 3, primary.calls)
		is.Equal(t, 1, secondary.calls)
	})

	t.Run("retries a primary that stalls in the stream before the first part then falls over", func(t *testing.T) {
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{hangInStream: true},
		})
		secondary := newFakeChatCompleter(t, "secondary", []fakeResponse{
			{parts: []gai.Part{gai.TextPart("saved")}},
		})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:     []gai.ChatCompleter{primary, secondary},
			MaxAttempts:    1,
			BaseDelay:      time.Millisecond,
			MaxDelay:       time.Millisecond,
			AttemptTimeout: 10 * time.Millisecond,
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		parts, err := collectParts(t, res)
		is.NotError(t, err)
		is.Equal(t, "saved", parts[0].Text())
		is.Equal(t, 1, primary.calls)
		is.Equal(t, 1, secondary.calls)
	})

	t.Run("does not kill a healthy backend that delivers the first part quickly then streams slowly past AttemptTimeout", func(t *testing.T) {
		// The first part arrives immediately (no delay), committing the stream and stopping
		// the per-attempt timer. The remaining parts each take longer than AttemptTimeout; the
		// stream must complete because the timer no longer bounds it after commit. The margin
		// (20ms timeout vs 50ms part delay) is wide enough that scheduling jitter at commit
		// can't fire the timer before it is stopped.
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{{
			parts:     []gai.Part{gai.TextPart("first"), gai.TextPart("second"), gai.TextPart("third")},
			partDelay: 50 * time.Millisecond,
		}})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:     []gai.ChatCompleter{primary},
			MaxAttempts:    1,
			BaseDelay:      time.Millisecond,
			MaxDelay:       time.Millisecond,
			AttemptTimeout: 20 * time.Millisecond,
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		parts, err := collectParts(t, res)
		is.NotError(t, err)
		is.Equal(t, 3, len(parts))
		is.Equal(t, "first", parts[0].Text())
		is.Equal(t, "second", parts[1].Text())
		is.Equal(t, "third", parts[2].Text())
		is.Equal(t, 1, primary.calls)
	})

	t.Run("treats the caller's own deadline as fatal even when AttemptTimeout is set", func(t *testing.T) {
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{hangBeforeStream: true},
		})
		secondary := newFakeChatCompleter(t, "secondary", []fakeResponse{
			{parts: []gai.Part{gai.TextPart("should not happen")}},
		})

		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		defer cancel()

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:     []gai.ChatCompleter{primary, secondary},
			MaxAttempts:    3,
			BaseDelay:      time.Nanosecond,
			MaxDelay:       time.Nanosecond,
			AttemptTimeout: time.Hour,
		})

		_, err := cc.ChatComplete(ctx, gai.ChatCompleteRequest{})
		is.Error(t, context.DeadlineExceeded, err)
		is.Equal(t, 1, primary.calls)
		is.Equal(t, 0, secondary.calls)
	})

	t.Run("retries a per-attempt timeout without invoking the custom classifier", func(t *testing.T) {
		var classifierCalls int
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{hangBeforeStream: true},
		})
		secondary := newFakeChatCompleter(t, "secondary", []fakeResponse{
			{parts: []gai.Part{gai.TextPart("saved")}},
		})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:     []gai.ChatCompleter{primary, secondary},
			MaxAttempts:    1,
			BaseDelay:      time.Millisecond,
			MaxDelay:       time.Millisecond,
			AttemptTimeout: 10 * time.Millisecond,
			ErrorClassifier: func(error) robust.Action {
				classifierCalls++
				return robust.ActionFail
			},
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		parts, err := collectParts(t, res)
		is.NotError(t, err)
		is.Equal(t, "saved", parts[0].Text())
		is.Equal(t, 0, classifierCalls)
		is.Equal(t, 1, primary.calls)
		is.Equal(t, 1, secondary.calls)
	})

	t.Run("records a retry action and attempt_timed_out when the call hangs before the stream", func(t *testing.T) {
		sr := oteltest.NewSpanRecorder(t)

		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{hangBeforeStream: true},
		})
		secondary := newFakeChatCompleter(t, "secondary", []fakeResponse{
			{parts: []gai.Part{gai.TextPart("saved")}},
		})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:     []gai.ChatCompleter{primary, secondary},
			MaxAttempts:    1,
			BaseDelay:      time.Millisecond,
			MaxDelay:       time.Millisecond,
			AttemptTimeout: 10 * time.Millisecond,
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		_, err = collectParts(t, res)
		is.NotError(t, err)

		attempts := oteltest.SpansByName(sr.Ended(), "robust.chat_complete_attempt")
		is.Equal(t, 2, len(attempts))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.String("ai.robust.action", "retry")))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.Bool("ai.robust.attempt_timed_out", true)))
		is.True(t, oteltest.HasAttribute(attempts[1].Attributes(), attribute.String("ai.robust.action", "success")))
	})

	t.Run("records a retry action and attempt_timed_out when the stream stalls before the first part", func(t *testing.T) {
		// This path runs through commitOnFirstPart, which must not end the attempt span on its
		// failure branches — otherwise the timeout-path attributes set afterwards are dropped.
		sr := oteltest.NewSpanRecorder(t)

		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{hangInStream: true},
		})
		secondary := newFakeChatCompleter(t, "secondary", []fakeResponse{
			{parts: []gai.Part{gai.TextPart("saved")}},
		})

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:     []gai.ChatCompleter{primary, secondary},
			MaxAttempts:    1,
			BaseDelay:      time.Millisecond,
			MaxDelay:       time.Millisecond,
			AttemptTimeout: 10 * time.Millisecond,
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{})
		is.NotError(t, err)
		_, err = collectParts(t, res)
		is.NotError(t, err)

		attempts := oteltest.SpansByName(sr.Ended(), "robust.chat_complete_attempt")
		is.Equal(t, 2, len(attempts))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.String("ai.robust.action", "retry")))
		is.True(t, oteltest.HasAttribute(attempts[0].Attributes(), attribute.Bool("ai.robust.attempt_timed_out", true)))
		is.True(t, oteltest.HasAttribute(attempts[1].Attributes(), attribute.String("ai.robust.action", "success")))
	})

	t.Run("panics when AttemptTimeout is negative", func(t *testing.T) {
		defer func() {
			r := recover()
			is.Equal(t, "AttemptTimeout must not be negative", r)
		}()

		robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:     []gai.ChatCompleter{newFakeChatCompleter(t, "p", nil)},
			AttemptTimeout: -1,
		})
	})
}
