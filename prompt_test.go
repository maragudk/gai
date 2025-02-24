package llm_test

import (
	"testing"

	"maragu.dev/env"
	"maragu.dev/is"
	"maragu.dev/llm"
)

func TestChatCompleter(t *testing.T) {
	_ = env.Load(".env.test.local")

	t.Run("can send a streaming chat completion request", func(t *testing.T) {
		tests := []struct {
			name string
			cc   llm.Completer
		}{
			{"openai", newOpenAIClient()},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				prompt := llm.Prompt{
					Model: llm.ModelGPT4oMini,
					Messages: []llm.Message{
						{Role: llm.MessageRoleUser, Parts: []llm.MessagePart{llm.TextMessagePart("Hi!")}},
					},
					Temperature: llm.Ptr(0.0),
				}

				res := test.cc.Complete(t.Context(), prompt)

				var text string
				for part, err := range res.Parts() {
					text += part.Text()
					is.NotError(t, err)
				}
				is.Equal(t, text, "Hello! How can I assist you today?")
			})
		}
	})
}

func newOpenAIClient() *llm.OpenAIClient {
	_ = env.Load(".env.test.local")

	return llm.NewOpenAIClient(llm.NewOpenAIClientOptions{Key: env.GetStringOrDefault("OPENAI_KEY", "")})
}
