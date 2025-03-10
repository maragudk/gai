package examples_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/generative-ai-go/genai"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
	"maragu.dev/env"

	"maragu.dev/gai"
	"maragu.dev/gai/eval"
)

// TestEvalLLMs evaluates different LLMs with the same prompts.
func TestEvalLLMs(t *testing.T) {
	_ = env.Load("../../.env.test.local")

	tests := []struct {
		name     string
		prompt   func(prompt string) string
		expected string
	}{
		{
			name:     "gpt-4o-mini",
			prompt:   gpt4oMini,
			expected: "Hello! How can I assist you today?",
		},
		{
			name:     "gemini-1.5-flash",
			prompt:   gemini15Flash,
			expected: "Hi there! How can I help you today?",
		},
		{
			name:     "claude-3.5-haiku",
			prompt:   claude35Haiku,
			expected: "Hello! How are you doing today? Is there anything I can help you with?",
		},
	}

	for _, test := range tests {
		eval.Run(t, test.name, func(e *eval.E) {
			input := "Hi!"
			output := test.prompt(input)

			sample := eval.Sample{
				Input:    input,
				Output:   output,
				Expected: test.expected,
			}

			lexicalSimilarityResult := e.Score(sample, eval.LexicalSimilarityScorer(eval.LevenshteinDistance))
			semanticSimilarityResult := e.Score(sample, eval.SemanticSimilarityScorer(&embedder{}, eval.CosineSimilarity))
			e.Log(sample, lexicalSimilarityResult, semanticSimilarityResult)
		})
	}
}

func gpt4oMini(prompt string) string {
	client := gai.NewOpenAIClient(gai.NewOpenAIClientOptions{Key: env.GetStringOrDefault("OPENAI_KEY", "")})
	res, err := client.Client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		}),
		Model:       openai.F(openai.ChatModelGPT4oMini),
		Temperature: openai.F(0.0),
	})
	if err != nil {
		panic(err)
	}
	return res.Choices[0].Message.Content
}

func gemini15Flash(prompt string) string {
	client := gai.NewGoogleClient(gai.NewGoogleClientOptions{Key: env.GetStringOrDefault("GOOGLE_KEY", "")})
	model := client.Client.GenerativeModel("models/gemini-1.5-flash-latest")
	var temperature float32 = 0
	model.Temperature = &temperature
	res, err := model.GenerateContent(context.Background(), genai.Text(prompt))
	if err != nil {
		panic(err)
	}
	return strings.TrimSpace(fmt.Sprint(res.Candidates[0].Content.Parts[0]))
}

func claude35Haiku(prompt string) string {
	client := gai.NewAnthropicClient(gai.NewAnthropicClientOptions{Key: env.GetStringOrDefault("ANTHROPIC_KEY", "")})
	res, err := client.Client.Messages.New(context.Background(), anthropic.MessageNewParams{
		Messages: anthropic.F([]anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		}),
		Model:       anthropic.F(anthropic.ModelClaude3_5HaikuLatest),
		MaxTokens:   anthropic.F(int64(1024)),
		Temperature: anthropic.F(0.0),
	})
	if err != nil {
		panic(err)
	}
	return fmt.Sprint(res.Content[0].Text)
}

type embedder struct{}

func (e *embedder) Embed(ctx context.Context, r io.Reader) (gai.EmbedResponse[float64], error) {
	v := gai.ReadAllString(r)

	client := gai.NewOpenAIClient(gai.NewOpenAIClientOptions{Key: env.GetStringOrDefault("OPENAI_KEY", "")})
	res, err := client.Client.Embeddings.New(context.Background(), openai.EmbeddingNewParams{
		Input:          openai.F[openai.EmbeddingNewParamsInputUnion](shared.UnionString(v)),
		Model:          openai.F(openai.EmbeddingModelTextEmbedding3Small),
		EncodingFormat: openai.F(openai.EmbeddingNewParamsEncodingFormatFloat),
		Dimensions:     openai.F(int64(128)),
	})
	if err != nil {
		return gai.EmbedResponse[float64]{}, err
	}
	if len(res.Data) == 0 {
		return gai.EmbedResponse[float64]{}, errors.New("no embeddings returned")
	}
	return gai.EmbedResponse[float64]{Embedding: res.Data[0].Embedding}, nil
}
