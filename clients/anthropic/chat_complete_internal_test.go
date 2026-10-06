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

func TestMapChatUsage(t *testing.T) {
	tests := []struct {
		name     string
		usage    anthropic.Usage
		expected gai.ChatCompleteResponseUsage
	}{
		{
			name:     "maps zero usage to zero",
			usage:    anthropic.Usage{},
			expected: gai.ChatCompleteResponseUsage{},
		},
		{
			name: "maps input and output tokens",
			usage: anthropic.Usage{
				InputTokens:  10,
				OutputTokens: 20,
			},
			expected: gai.ChatCompleteResponseUsage{
				PromptTokens:     10,
				CompletionTokens: 20,
			},
		},
		{
			name: "maps thinking tokens to thoughts tokens, keeping them inside completion tokens",
			usage: anthropic.Usage{
				InputTokens:  10,
				OutputTokens: 120,
				OutputTokensDetails: anthropic.OutputTokensDetails{
					ThinkingTokens: 100,
				},
			},
			expected: gai.ChatCompleteResponseUsage{
				PromptTokens:     10,
				CompletionTokens: 120,
				ThoughtsTokens:   100,
			},
		},
		{
			name: "adds cache read and cache creation tokens to prompt tokens, because input tokens exclude them",
			usage: anthropic.Usage{
				InputTokens:              10,
				CacheReadInputTokens:     1024,
				CacheCreationInputTokens: 512,
				OutputTokens:             5,
			},
			expected: gai.ChatCompleteResponseUsage{
				PromptTokens:     1546,
				CacheReadTokens:  1024,
				CacheWriteTokens: 512,
				CompletionTokens: 5,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			is.Equal(t, test.expected, mapChatUsage(test.usage))
		})
	}
}
