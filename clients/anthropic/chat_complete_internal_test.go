package anthropic

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"maragu.dev/is"

	"maragu.dev/gai"
)

func TestMapChatFinishReason(t *testing.T) {
	tests := []struct {
		name     string
		reason   anthropic.StopReason
		expected gai.ChatCompleteFinishReason
	}{
		{name: "maps end_turn to stop", reason: anthropic.StopReasonEndTurn, expected: gai.ChatCompleteFinishReasonStop},
		{name: "maps stop_sequence to stop", reason: anthropic.StopReasonStopSequence, expected: gai.ChatCompleteFinishReasonStop},
		{name: "maps max_tokens to length", reason: anthropic.StopReasonMaxTokens, expected: gai.ChatCompleteFinishReasonLength},
		{name: "maps model_context_window_exceeded to length", reason: anthropic.StopReasonModelContextWindowExceeded, expected: gai.ChatCompleteFinishReasonLength},
		{name: "maps tool_use to tool_calls", reason: anthropic.StopReasonToolUse, expected: gai.ChatCompleteFinishReasonToolCalls},
		{name: "maps refusal to refusal", reason: anthropic.StopReasonRefusal, expected: gai.ChatCompleteFinishReasonRefusal},
		{name: "maps pause_turn to unknown", reason: anthropic.StopReasonPauseTurn, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps an unrecognised value to unknown", reason: "something_new", expected: gai.ChatCompleteFinishReasonUnknown},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			is.Equal(t, test.expected, mapChatFinishReason(test.reason))
		})
	}
}
