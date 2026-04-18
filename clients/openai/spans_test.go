package openai_test

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/openai"
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

		span := oteltest.FindSpan(t, sr.Ended(), "openai.chat_complete")
		is.True(t, oteltest.HasAttribute(span.Attributes(), attribute.String("ai.model", string(openai.ChatCompleteModelGPT5Nano))))
		oteltest.RequireNonNegativeInt64Attribute(t, span.Attributes(), "ai.time_to_first_token_ms")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.prompt_tokens")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.completion_tokens")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.total_tokens")
		oteltest.RequireAttributePresent(t, span.Attributes(), "ai.cache_read_tokens")
	})
}

func TestEmbedder_Spans(t *testing.T) {
	t.Run("records standard attributes on the embed span", func(t *testing.T) {
		sr := oteltest.NewSpanRecorder(t)
		c := newClient(t)
		e := c.NewEmbedder(openai.NewEmbedderOptions{
			Model:      openai.EmbedModelTextEmbedding3Small,
			Dimensions: 1536,
		})

		_, err := e.Embed(t.Context(), gai.NewTextEmbedRequest("Embed this, please."))
		is.NotError(t, err)

		span := oteltest.FindSpan(t, sr.Ended(), "openai.embed")
		is.True(t, oteltest.HasAttribute(span.Attributes(), attribute.String("ai.model", string(openai.EmbedModelTextEmbedding3Small))))
		is.True(t, oteltest.HasAttribute(span.Attributes(), attribute.Int("ai.dimensions", 1536)))
		is.True(t, oteltest.HasAttribute(span.Attributes(), attribute.Int("ai.input_length", len("Embed this, please."))))
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.prompt_tokens")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.total_tokens")
	})
}
