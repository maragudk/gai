package anthropic_test

import (
	"os"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/anthropic"
	"maragu.dev/gai/internal/oteltest"
	"maragu.dev/gai/tools"
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

		span := oteltest.FindSpan(t, sr.Ended(), "anthropic.chat_complete")
		is.True(t, oteltest.HasAttribute(span.Attributes(), attribute.String("ai.model", string(anthropic.ChatCompleteModelClaudeHaiku4_5Latest))))
		oteltest.RequireNonNegativeInt64Attribute(t, span.Attributes(), "ai.time_to_first_token_ms")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.prompt_tokens")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.completion_tokens")
		oteltest.RequireAttributePresent(t, span.Attributes(), "ai.cache_read_tokens")
		oteltest.RequireAttributePresent(t, span.Attributes(), "ai.cache_creation_tokens")
	})

	t.Run("still records prompt_tokens after multiple content blocks (regression for Usage wipe bug)", func(t *testing.T) {
		// This exercises the ContentBlockStopEvent path that previously reset
		// message = anthropic.Message{}, wiping Usage mid-stream. A tool-call
		// response typically produces text + tool_use blocks, so more than one
		// ContentBlockStopEvent fires before MessageDeltaEvent / MessageStopEvent.
		sr := oteltest.NewSpanRecorder(t)
		cc := newChatCompleter(t)

		root, err := os.OpenRoot("testdata")
		is.NotError(t, err)

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{
			Messages:    []gai.Message{gai.NewUserTextMessage("What is in the readme.txt file?")},
			Temperature: gai.Ptr(gai.Temperature(0)),
			Tools:       []gai.Tool{tools.NewReadFile(root)},
		})
		is.NotError(t, err)
		for _, err := range res.Parts() {
			is.NotError(t, err)
		}

		span := oteltest.FindSpan(t, sr.Ended(), "anthropic.chat_complete")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.prompt_tokens")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.completion_tokens")
	})
}
