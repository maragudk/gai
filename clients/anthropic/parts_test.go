package anthropic

import (
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"maragu.dev/is"

	"maragu.dev/gai"
)

// TestChatCompleter_RejectsInboundThoughtParts asserts that a [gai.PartTypeThought]
// part in [gai.ChatCompleteRequest.Messages] returns a clear error rather than
// being silently dropped or producing a confusing API 400 downstream. Multi-turn
// round-trip of signed Anthropic thinking blocks is tracked at
// https://github.com/maragudk/gai/issues/250.
func TestChatCompleter_RejectsInboundThoughtParts(t *testing.T) {
	cc := &ChatCompleter{
		log:    slog.New(slog.DiscardHandler),
		model:  ChatCompleteModelClaudeSonnet4Latest,
		tracer: otel.Tracer("test"),
	}

	req := gai.ChatCompleteRequest{
		Messages: []gai.Message{
			{
				Role: gai.MessageRoleModel,
				Parts: []gai.Part{
					gai.ThoughtPart("the duck sits down to think"),
					gai.TextPart("hi"),
				},
			},
			gai.NewUserTextMessage("carry on"),
		},
	}

	_, err := cc.ChatComplete(t.Context(), req)
	is.True(t, err != nil, "expected error for inbound thought part")
	is.True(t,
		strings.Contains(err.Error(), "gai.PartTypeThought is not yet supported"),
		"error message should mention the unsupported part type, got: "+err.Error())
}
