package llm_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"maragu.dev/env"
	"maragu.dev/is"

	"maragu.dev/llm"
)

func TestNewAnthropicClient(t *testing.T) {
	t.Run("can create a new client with a key", func(t *testing.T) {
		client := llm.NewAnthropicClient(llm.NewAnthropicClientOptions{Key: "123"})
		is.NotNil(t, client)
	})
}

func TestAnthropicClientCompletion(t *testing.T) {
	_ = env.Load(".env.test.local")

	t.Run("can do a basic chat completion", func(t *testing.T) {
		client := llm.NewAnthropicClient(llm.NewAnthropicClientOptions{Key: env.GetStringOrDefault("ANTHROPIC_KEY", "")})
		is.NotNil(t, client)

		res, err := client.Client.Messages.New(context.Background(), anthropic.MessageNewParams{
			System: anthropic.F([]anthropic.TextBlockParam{
				anthropic.NewTextBlock(`Only say the word "Hi", nothing more.`),
			}),
			Messages: anthropic.F([]anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Hi.")),
			}),
			Model:     anthropic.F(anthropic.ModelClaude3_5HaikuLatest),
			MaxTokens: anthropic.F(int64(4)),
		})
		is.NotError(t, err)
		is.True(t, strings.Contains(fmt.Sprint(res.Content), "Hi"))
	})
}
