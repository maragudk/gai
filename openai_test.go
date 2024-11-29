package llm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/openai/openai-go"
	"maragu.dev/env"
	"maragu.dev/is"

	"maragu.dev/llm"
)

func TestNewOpenAIClient(t *testing.T) {
	t.Run("can create a new client with a key", func(t *testing.T) {
		client := llm.NewOpenAIClient(llm.NewOpenAIClientOptions{Key: "123"})
		is.NotNil(t, client)
	})
}

func TestOpenAIClientCompletion(t *testing.T) {
	_ = env.Load(".env.test.local")

	t.Run("can do a basic chat completion", func(t *testing.T) {
		client := llm.NewOpenAIClient(llm.NewOpenAIClientOptions{Key: env.GetStringOrDefault("OPENAI_KEY", "")})
		is.NotNil(t, client)

		res, err := client.Client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
			Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage(`Only say the word "Hi", nothing more.`),
				openai.UserMessage("Hi."),
			}),
			Model: openai.F(openai.ChatModelGPT4oMini),
		})
		is.NotError(t, err)
		is.True(t, len(res.Choices) > 0)
		is.True(t, strings.Contains(res.Choices[0].Message.Content, "Hi"))
	})
}
