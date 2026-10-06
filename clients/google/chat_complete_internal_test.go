package google

import (
	"testing"

	"google.golang.org/genai"
	"maragu.dev/is"

	"maragu.dev/gai"
)

func TestMapChatFinishReason(t *testing.T) {
	tests := []struct {
		name     string
		reason   genai.FinishReason
		expected gai.ChatCompleteFinishReason
	}{
		{name: "maps STOP to stop", reason: genai.FinishReasonStop, expected: gai.ChatCompleteFinishReasonStop},
		{name: "maps MAX_TOKENS to length", reason: genai.FinishReasonMaxTokens, expected: gai.ChatCompleteFinishReasonLength},
		{name: "maps SAFETY to content_filter", reason: genai.FinishReasonSafety, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps RECITATION to content_filter", reason: genai.FinishReasonRecitation, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps BLOCKLIST to content_filter", reason: genai.FinishReasonBlocklist, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps PROHIBITED_CONTENT to content_filter", reason: genai.FinishReasonProhibitedContent, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps SPII to content_filter", reason: genai.FinishReasonSPII, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps IMAGE_SAFETY to content_filter", reason: genai.FinishReasonImageSafety, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps IMAGE_PROHIBITED_CONTENT to content_filter", reason: genai.FinishReasonImageProhibitedContent, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps IMAGE_RECITATION to content_filter", reason: genai.FinishReasonImageRecitation, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps FINISH_REASON_UNSPECIFIED to unknown", reason: genai.FinishReasonUnspecified, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps OTHER to unknown", reason: genai.FinishReasonOther, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps LANGUAGE to unknown", reason: genai.FinishReasonLanguage, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps MALFORMED_FUNCTION_CALL to unknown", reason: genai.FinishReasonMalformedFunctionCall, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps UNEXPECTED_TOOL_CALL to unknown", reason: genai.FinishReasonUnexpectedToolCall, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps TOO_MANY_TOOL_CALLS to unknown", reason: genai.FinishReasonTooManyToolCalls, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps NO_IMAGE to unknown", reason: genai.FinishReasonNoImage, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps IMAGE_OTHER to unknown", reason: genai.FinishReasonImageOther, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps an unrecognised value to unknown", reason: "SOMETHING_NEW", expected: gai.ChatCompleteFinishReasonUnknown},
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
		usage    genai.GenerateContentResponseUsageMetadata
		expected gai.ChatCompleteResponseUsage
	}{
		{
			name:     "maps zero usage to zero",
			usage:    genai.GenerateContentResponseUsageMetadata{},
			expected: gai.ChatCompleteResponseUsage{},
		},
		{
			name: "maps prompt and candidates tokens",
			usage: genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 20,
				TotalTokenCount:      30,
			},
			expected: gai.ChatCompleteResponseUsage{
				PromptTokens:     10,
				CompletionTokens: 20,
			},
		},
		{
			name: "adds thoughts tokens to completion tokens, because candidates tokens exclude them",
			usage: genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 20,
				ThoughtsTokenCount:   100,
				TotalTokenCount:      130,
			},
			expected: gai.ChatCompleteResponseUsage{
				PromptTokens:     10,
				CompletionTokens: 120,
				ThoughtsTokens:   100,
			},
		},
		{
			name: "maps cached content tokens to cache read tokens, keeping them inside prompt tokens",
			usage: genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:        2000,
				CachedContentTokenCount: 1024,
				CandidatesTokenCount:    5,
				TotalTokenCount:         2005,
			},
			expected: gai.ChatCompleteResponseUsage{
				PromptTokens:     2000,
				CacheReadTokens:  1024,
				CompletionTokens: 5,
			},
		},
		{
			name: "leaves tool use prompt tokens out of prompt tokens",
			usage: genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:        10,
				ToolUsePromptTokenCount: 50,
				CandidatesTokenCount:    5,
				TotalTokenCount:         65,
			},
			expected: gai.ChatCompleteResponseUsage{
				PromptTokens:     10,
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
