package google_test

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/google"
	"maragu.dev/gai/internal/oteltest"
)

func TestChatCompleter_Spans(t *testing.T) {
	t.Run("records standard attributes on the chat-complete span for a simple text prompt", func(t *testing.T) {
		sr := oteltest.NewSpanRecorder(t)
		cc := newChatCompleter(t)

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{
			Messages: []gai.Message{gai.NewUserTextMessage("Reply with a single word.")},
		})
		is.NotError(t, err)
		for _, err := range res.Parts() {
			is.NotError(t, err)
		}

		span := oteltest.FindSpan(t, sr.Ended(), "google.chat_complete")
		is.True(t, oteltest.HasAttribute(span.Attributes(), attribute.String("ai.model", string(google.ChatCompleteModelGemini2_5Flash))))
		oteltest.RequireAttributePresent(t, span.Attributes(), "ai.time_to_first_token_ms")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.prompt_tokens")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.completion_tokens")
		oteltest.RequireCacheReadSubsetOfPromptTokens(t, span.Attributes())
	})
}

func TestEmbedder_Spans(t *testing.T) {
	t.Run("records standard attributes on the embed span", func(t *testing.T) {
		sr := oteltest.NewSpanRecorder(t)
		c := newClient(t)
		e := google.NewEmbedder[float32](c, google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding001,
			Dimensions: 768,
		})

		_, err := e.Embed(t.Context(), gai.NewTextEmbedRequest("Embed this, please."))
		is.NotError(t, err)

		span := oteltest.FindSpan(t, sr.Ended(), "google.embed")
		is.True(t, oteltest.HasAttribute(span.Attributes(), attribute.String("ai.model", string(google.EmbedModelGeminiEmbedding001))))
		is.True(t, oteltest.HasAttribute(span.Attributes(), attribute.Int("ai.dimensions", 768)))
		is.True(t, oteltest.HasAttribute(span.Attributes(), attribute.Int("ai.input_length", len("Embed this, please."))))
	})
}
