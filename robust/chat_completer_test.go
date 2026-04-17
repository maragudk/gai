package robust_test

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"maragu.dev/is"

	"maragu.dev/gai"
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
}

// newFakeChatCompleter constructs a fakeChatCompleter bound to t.
func newFakeChatCompleter(t *testing.T, name string, responses []fakeResponse) *fakeChatCompleter {
	t.Helper()
	return &fakeChatCompleter{t: t, name: name, responses: responses}
}

func (f *fakeChatCompleter) ChatComplete(_ context.Context, _ gai.ChatCompleteRequest) (gai.ChatCompleteResponse, error) {
	f.t.Helper()
	if f.calls >= len(f.responses) {
		f.t.Fatalf("fakeChatCompleter %s: no more queued responses", f.name)
	}
	r := f.responses[f.calls]
	f.calls++

	if r.preStreamErr != nil {
		return gai.ChatCompleteResponse{}, r.preStreamErr
	}

	meta := r.meta
	if meta == nil {
		meta = &gai.ChatCompleteResponseMetadata{}
	}

	partsFunc := func(yield func(gai.Part, error) bool) {
		for _, p := range r.parts {
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

	t.Run("interrupts the backoff sleep when the caller cancels the context", func(t *testing.T) {
		primary := newFakeChatCompleter(t, "primary", []fakeResponse{
			{preStreamErr: errors.New("first fails")},
			{preStreamErr: errors.New("should not be called")},
		})

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:  []gai.ChatCompleter{primary},
			MaxAttempts: 3,
			BaseDelay:   time.Hour,
			MaxDelay:    time.Hour,
		})

		_, err := cc.ChatComplete(ctx, gai.ChatCompleteRequest{})
		is.Error(t, context.Canceled, err)
		is.Equal(t, 1, primary.calls)
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
			is.Equal(t, "robust: Completers must not be empty", r)
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
			is.Equal(t, "robust: MaxAttempts must not be negative", r)
		}()

		robust.NewChatCompleter(robust.NewChatCompleterOptions{
			Completers:  []gai.ChatCompleter{newFakeChatCompleter(t, "p", nil)},
			MaxAttempts: -1,
		})
	})

	t.Run("panics when BaseDelay exceeds MaxDelay", func(t *testing.T) {
		defer func() {
			r := recover()
			is.Equal(t, "robust: BaseDelay must not exceed MaxDelay", r)
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
}
