package anthropic

import (
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"

	"maragu.dev/gai"
)

// TestChatCompleter_AcceptsInboundThoughtParts asserts that a [gai.PartTypeThought]
// part in [gai.ChatCompleteRequest.Messages] is silently dropped on the request
// side — the natural multi-turn pattern (echo back streamed parts as history)
// must not panic.
func TestChatCompleter_AcceptsInboundThoughtParts(t *testing.T) {
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

	var panicValue any
	func() {
		defer func() { panicValue = recover() }()
		_, _ = cc.ChatComplete(t.Context(), req)
	}()

	if msg, ok := panicValue.(string); ok && strings.HasPrefix(msg, "unknown part type") {
		t.Fatalf("expected thought part to be silently dropped, got panic: %v", panicValue)
	}
}
