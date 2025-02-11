# llm

<img src="logo.png" alt="Logo" width="300" align="right">

[![GoDoc](https://pkg.go.dev/badge/maragu.dev/llm)](https://pkg.go.dev/maragu.dev/llm)
[![Go](https://github.com/maragudk/llm/actions/workflows/ci.yml/badge.svg)](https://github.com/maragudk/llm/actions/workflows/ci.yml)

LLM tools and helpers in Go.

⚠️ **This library is in development**. Things will probably break, but existing functionality is usable. ⚠️

```shell
go get maragu.dev/llm
```

Made with ✨sparkles✨ by [maragu](https://www.maragu.dev/).

Does your company depend on this project? [Contact me at markus@maragu.dk](mailto:markus@maragu.dk?Subject=Supporting%20your%20project) to discuss options for a one-time or recurring invoice to ensure its continued thriving.

## Usage

Evals will only run with `go test -run TestEval ./...` and otherwise be skipped.

### Simple example

Eval a mocked LLM, construct a sample, score it with a lexical similarity scorer, and log the result.

```go
package examples_test

import (
	"testing"

	"maragu.dev/llm/eval"
)

// TestEvalPrompt evaluates the Prompt method.
// All evals must be prefixed with "TestEval".
func TestEvalPrompt(t *testing.T) {
	// Evals only run if "go test" is being run with "-test.run=TestEval", e.g.: "go test -test.run=TestEval ./..."
	eval.Run(t, "answers with a pong", func(e *eval.E) {
		// Initialize our intensely powerful LLM.
		llm := &powerfulLLM{response: "plong"}

		// Send our input to the LLM and get an output back.
		input := "ping"
		output := llm.Prompt(input)

		// Create a sample to pass to the scorer.
		sample := eval.Sample{
			Input:    input,
			Output:   output,
			Expected: "pong",
		}

		// Score the sample using the Levenshtein distance scorer.
		// The scorer is created inline, but for scorers that need more setup, this can be done elsewhere.
		result := e.Score(sample, eval.LexicalSimilarityScorer(eval.LevenshteinDistance))

		// Log the sample, result, and timing information.
		e.Log(sample, result)
	})
}

type powerfulLLM struct {
	response string
}

func (l *powerfulLLM) Prompt(request string) string {
	return l.response
}
```

### Advanced example

This eval uses real LLMs (OpenAI GPT4o mini, Google Gemini 1.5 Flash, Anthropic 3.5 Haiku)
and compares the response to an expected response using both lexical similarity (with Levenshtein distance)
and semantic similarity (with an OpenAI embedding model and cosine similarity comparison).

```go
package examples_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/generative-ai-go/genai"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
	"maragu.dev/env"

	"maragu.dev/llm"
	"maragu.dev/llm/eval"
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
			semanticSimilarityResult := e.Score(sample, eval.SemanticSimilarityScorer(&embeddingGetter{}, eval.CosineSimilarity))
			e.Log(sample, lexicalSimilarityResult, semanticSimilarityResult)
		})
	}
}

func gpt4oMini(prompt string) string {
	client := llm.NewOpenAIClient(llm.NewOpenAIClientOptions{Key: env.GetStringOrDefault("OPENAI_KEY", "")})
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
	client := llm.NewGoogleClient(llm.NewGoogleClientOptions{Key: env.GetStringOrDefault("GOOGLE_KEY", "")})
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
	client := llm.NewAnthropicClient(llm.NewAnthropicClientOptions{Key: env.GetStringOrDefault("ANTHROPIC_KEY", "")})
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

type embeddingGetter struct{}

func (e *embeddingGetter) GetEmbedding(v string) ([]float64, error) {
	client := llm.NewOpenAIClient(llm.NewOpenAIClientOptions{Key: env.GetStringOrDefault("OPENAI_KEY", "")})
	res, err := client.Client.Embeddings.New(context.Background(), openai.EmbeddingNewParams{
		Input:          openai.F[openai.EmbeddingNewParamsInputUnion](shared.UnionString(v)),
		Model:          openai.F(openai.EmbeddingModelTextEmbedding3Small),
		EncodingFormat: openai.F(openai.EmbeddingNewParamsEncodingFormatFloat),
		Dimensions:     openai.F(int64(128)),
	})
	if err != nil {
		return nil, err
	}
	if len(res.Data) == 0 {
		return nil, errors.New("no embeddings returned")
	}
	return res.Data[0].Embedding, nil
}
```
