package llm_test

import (
	"context"
	"testing"

	"github.com/openai/openai-go"
	"maragu.dev/is"

	"maragu.dev/llm"
)

func TestNewOpenAIClient(t *testing.T) {
	t.Run("can create a new client with a token", func(t *testing.T) {
		client := llm.NewOpenAIClient(llm.NewOpenAIClientOptions{Token: "123"})
		is.NotNil(t, client)
	})
}

func TestOpenAIClientPrompt(t *testing.T) {
	t.Run("can do a basic chat completion", func(t *testing.T) {
		client := llm.NewOpenAIClient(llm.NewOpenAIClientOptions{BaseURL: "http://localhost:8090/v1/"})
		is.NotNil(t, client)

		res, err := client.Client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
			Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("Just say hi."),
			}),
			Model: openai.F("llama"),
		})
		is.NotError(t, err)
		is.True(t, len(res.Choices) > 0)
		is.Equal(t, "foo", res.Choices[0].Message.Content)
	})
}
