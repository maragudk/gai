package google

import (
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"

	"maragu.dev/gai"
)

// TestChatCompleter_AcceptsInboundThoughtParts asserts that a [gai.PartTypeThought]
// part in [gai.ChatCompleteRequest.Messages] does not panic on the request side
// — Google round-trips it as a thought-flagged genai part. The actual
// thought-flag survival to the wire is exercised by the integration tests.
func TestChatCompleter_AcceptsInboundThoughtParts(t *testing.T) {
	cc := &ChatCompleter{
		log:    slog.New(slog.DiscardHandler),
		model:  ChatCompleteModelGemini2_5Flash,
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
		t.Fatalf("expected thought part to round-trip without panic, got: %v", panicValue)
	}
}
