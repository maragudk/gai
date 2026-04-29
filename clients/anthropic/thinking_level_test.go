package anthropic

import (
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel"

	"maragu.dev/gai"
)

func TestChatCompleter_ThinkingLevel(t *testing.T) {
	tests := []struct {
		name        string
		level       gai.ThinkingLevel
		shouldPanic bool
	}{
		{name: "accepts gai.ThinkingLevelNone", level: gai.ThinkingLevelNone, shouldPanic: false},
		{name: "accepts ThinkingLevelLow", level: ThinkingLevelLow, shouldPanic: false},
		{name: "accepts ThinkingLevelMedium", level: ThinkingLevelMedium, shouldPanic: false},
		{name: "accepts ThinkingLevelHigh", level: ThinkingLevelHigh, shouldPanic: false},
		{name: "accepts ThinkingLevelXHigh", level: ThinkingLevelXHigh, shouldPanic: false},
		{name: "accepts ThinkingLevelMax", level: ThinkingLevelMax, shouldPanic: false},
		{name: "panics on a level the client does not publish (minimal)", level: gai.ThinkingLevel("minimal"), shouldPanic: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cc := &ChatCompleter{
				log:    slog.New(slog.DiscardHandler),
				model:  ChatCompleteModelClaudeSonnet4_6Latest,
				tracer: otel.Tracer("test"),
			}

			req := gai.ChatCompleteRequest{
				Messages:      []gai.Message{gai.NewUserTextMessage("Hi!")},
				ThinkingLevel: gai.Ptr(test.level),
			}

			var panicValue any
			func() {
				defer func() { panicValue = recover() }()
				_, _ = cc.ChatComplete(t.Context(), req)
			}()

			if test.shouldPanic {
				msg, ok := panicValue.(string)
				if !ok || msg != "unsupported thinking level: "+string(test.level) {
					t.Fatalf("expected panic with unsupported thinking level message, got %v", panicValue)
				}
			} else {
				if panicValue != nil {
					msg, ok := panicValue.(string)
					if ok && msg == "unsupported thinking level: "+string(test.level) {
						t.Fatalf("unexpected panic on supported thinking level: %v", panicValue)
					}
				}
			}
		})
	}
}
