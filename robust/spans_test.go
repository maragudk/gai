package robust_test

import (
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/internal/oteltest"
	"maragu.dev/gai/robust"
)

func TestChatCompleter_Spans(t *testing.T) {
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
}

func TestEmbedder_Spans(t *testing.T) {
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
