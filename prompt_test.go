package gai_test

import (
	"testing"

	"maragu.dev/env"
	"maragu.dev/gai"
	"maragu.dev/is"
)

func TestChatCompleter(t *testing.T) {
	_ = env.Load(".env.test.local")

	t.Run("can send a streaming chat completion request", func(t *testing.T) {
		tests := []struct {
			name string
			cc   gai.Completer
		}{
			{"openai", newOpenAIClient()},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				prompt := gai.Prompt{
					Model: gai.ModelGPT4oMini,
					Messages: []gai.Message{
						{Role: gai.MessageRoleUser, Parts: []gai.MessagePart{gai.TextMessagePart("Hi!")}},
					},
					Temperature: gai.Ptr(0.0),
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

func newOpenAIClient() *gai.OpenAIClient {
	_ = env.Load(".env.test.local")

	return gai.NewOpenAIClient(gai.NewOpenAIClientOptions{Key: env.GetStringOrDefault("OPENAI_KEY", "")})
}
