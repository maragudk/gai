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
		{name: "panics on none", level: gai.ThinkingLevelNone, shouldPanic: true},
		{name: "panics on minimal", level: gai.ThinkingLevelMinimal, shouldPanic: true},
		{name: "panics on low", level: gai.ThinkingLevelLow, shouldPanic: true},
		{name: "panics on medium", level: gai.ThinkingLevelMedium, shouldPanic: true},
		{name: "panics on high", level: gai.ThinkingLevelHigh, shouldPanic: true},
		{name: "panics on xhigh", level: gai.ThinkingLevelXHigh, shouldPanic: true},
		{name: "panics on max", level: gai.ThinkingLevelMax, shouldPanic: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cc := &ChatCompleter{
				log:    slog.New(slog.DiscardHandler),
				model:  ChatCompleteModelClaude4SonnetLatest,
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
			}
		})
	}
}
