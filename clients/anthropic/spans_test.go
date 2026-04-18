package anthropic_test

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/anthropic"
	"maragu.dev/gai/internal/oteltest"
)

func TestChatCompleter_Spans(t *testing.T) {
	// This also serves as a regression test for a previous bug where
	// message = anthropic.Message{} on every ContentBlockStopEvent wiped
	// message.Usage mid-stream, leaving ai.prompt_tokens at zero.
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

		span := oteltest.FindSpan(t, sr.Ended(), "anthropic.chat_complete")
		is.True(t, oteltest.HasAttribute(span.Attributes(), attribute.String("ai.model", string(anthropic.ChatCompleteModelClaudeHaiku4_5Latest))))
		oteltest.RequireAttributePresent(t, span.Attributes(), "ai.time_to_first_token_ms")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.prompt_tokens")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.completion_tokens")
		oteltest.RequireAttributePresent(t, span.Attributes(), "ai.cache_creation_tokens")
		oteltest.RequireCacheReadSubsetOfPromptTokens(t, span.Attributes())
	})
}
