package openai

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"maragu.dev/is"

	"maragu.dev/gai"
)

func TestMapChatUsage(t *testing.T) {
	tests := []struct {
		name     string
		usage    openai.CompletionUsage
		expected gai.ChatCompleteResponseUsage
	}{
		{
			name:     "maps zero usage to zero",
			usage:    openai.CompletionUsage{},
			expected: gai.ChatCompleteResponseUsage{},
		},
		{
			name: "maps prompt and completion tokens",
			usage: openai.CompletionUsage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
			expected: gai.ChatCompleteResponseUsage{
				PromptTokens:     10,
				CompletionTokens: 20,
			},
		},
		{
			name: "maps reasoning tokens to thoughts tokens, keeping them inside completion tokens",
			usage: openai.CompletionUsage{
				PromptTokens:     10,
				CompletionTokens: 120,
				TotalTokens:      130,
				CompletionTokensDetails: openai.CompletionUsageCompletionTokensDetails{
					ReasoningTokens: 100,
				},
			},
			expected: gai.ChatCompleteResponseUsage{
				PromptTokens:     10,
				CompletionTokens: 120,
				ThoughtsTokens:   100,
			},
		},
		{
			name: "maps cached and cache write tokens, keeping them inside prompt tokens",
			usage: openai.CompletionUsage{
				PromptTokens:     2000,
				CompletionTokens: 5,
				TotalTokens:      2005,
				PromptTokensDetails: openai.CompletionUsagePromptTokensDetails{
					CachedTokens:     1024,
					CacheWriteTokens: 512,
				},
			},
			expected: gai.ChatCompleteResponseUsage{
				PromptTokens:     2000,
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
