# Go Artificial Intelligence (GAI)

<img src="logo.jpg" alt="Logo" width="300" align="right">

[![GoDoc](https://pkg.go.dev/badge/maragu.dev/gai)](https://pkg.go.dev/maragu.dev/gai)
[![CI](https://github.com/maragudk/gai/actions/workflows/ci.yml/badge.svg)](https://github.com/maragudk/gai/actions/workflows/ci.yml)

Go Artificial Intelligence (GAI) helps you work with foundational models, large language models, and other AI models.

Pronounced like "guy".

⚠️ **This library is in development**. Things will probably break, but existing functionality is usable. ⚠️

```shell
go get maragu.dev/gai
```

Made with ✨sparkles✨ by [maragu](https://www.maragu.dev/): independent software consulting for cloud-native Go apps & AI engineering.

[Contact me at markus@maragu.dk](mailto:markus@maragu.dk) for consulting work, or perhaps an invoice to support this project?

## Usage

### Clients

These client implementations are available:

- [gai-openai](https://github.com/maragudk/gai-openai)
- [gai-google](https://github.com/maragudk/gai-google)
- [gai-anthropic](https://github.com/maragudk/gai-anthropic)

Also, there's an experimental agent at [gaigent](https://github.com/maragudk/gaigent).

### Examples

Click to expand each section, or see all examples under [internal/examples](internal/examples).

<details>
	<summary>Tools</summary>

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"maragu.dev/gai"
	openai "maragu.dev/gai-openai"
	"maragu.dev/gai/tools"
)

func main() {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	c := openai.NewClient(openai.NewClientOptions{
		Key: os.Getenv("OPENAI_API_KEY"),
		Log: log,
	})

	cc := c.NewChatCompleter(openai.NewChatCompleterOptions{
		Model: openai.ChatCompleteModelGPT4o,
	})

	req := gai.ChatCompleteRequest{
		Messages: []gai.Message{
			gai.NewUserTextMessage("What time is it?"),
		},
		System: gai.Ptr("You are a British seagull. Speak like it."),
		Tools: []gai.Tool{
			tools.NewGetTime(time.Now), // Note that some tools that only require the stdlib are included in GAI
		},
	}

	res, err := cc.ChatComplete(ctx, req)
	if err != nil {
		log.Error("Error chat-completing", "error", err)
		return
	}

	var parts []gai.MessagePart
	var result gai.ToolResult

	for part, err := range res.Parts() {
		if err != nil {
			log.Error("Error processing part", "error", err)
			return
		}

		parts = append(parts, part)

		switch part.Type {
		case gai.MessagePartTypeText:
			fmt.Print(part.Text())

		case gai.MessagePartTypeToolCall:
			toolCall := part.ToolCall()
			for _, tool := range req.Tools {
				if tool.Name != toolCall.Name {
					continue
				}

				content, err := tool.Function(ctx, toolCall.Args) // Tools aren't called automatically, so you can decide if, how, and when
				result = gai.ToolResult{
					ID:      toolCall.ID,
					Content: content,
					Err:     err,
				}
				break
			}
		}
	}

	if result.ID == "" {
		log.Error("No tool result found")
		return
	}

	// Add both the tool call (in the parts) and the tool result to the messages, and make another request
	req.Messages = append(req.Messages,
		gai.Message{Role: gai.MessageRoleModel, Parts: parts},
		gai.NewUserToolResultMessage(result),
	)

	res, err = cc.ChatComplete(ctx, req)
	if err != nil {
		log.Error("Error chat-completing", "error", err)
		return
	}

	for part, err := range res.Parts() {
		if err != nil {
			log.Error("Error processing part", "error", err)
			return
		}

		switch part.Type {
		case gai.MessagePartTypeText:
			fmt.Print(part.Text())
		}
	}
}
```

```shell
$ go run main.go
Ahoy, mate! The time be 15:20, it be!
```

</details>

<details>
	<summary>Tools (custom)</summary>

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"

	"maragu.dev/gai"
	openai "maragu.dev/gai-openai"
)

type EatArgs struct {
	What string `json:"what" jsonschema_description:"What you'd like to eat."`
}

func NewEat() gai.Tool {
	return gai.Tool{
		Name:        "eat",
		Description: "Eat something, supplying what you eat as an argument. The result will be a string describing how it was.",
		Schema:      gai.GenerateSchema[EatArgs](),
		Function: func(ctx context.Context, args json.RawMessage) (string, error) {
			var eatArgs EatArgs
			if err := json.Unmarshal(args, &eatArgs); err != nil {
				return "", fmt.Errorf("error unmarshaling eat args from JSON: %w", err)
			}

			results := []string{
				"it was okay.",
				"it was absolutely excellent!",
				"it was awful.",
				"it gave you diarrhea.",
			}

			return "You ate " + eatArgs.What + " and " + results[rand.IntN(len(results))], nil
		},
	}
}

func main() {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	c := openai.NewClient(openai.NewClientOptions{
		Key: os.Getenv("OPENAI_API_KEY"),
		Log: log,
	})

	cc := c.NewChatCompleter(openai.NewChatCompleterOptions{
		Model: openai.ChatCompleteModelGPT4o,
	})

	req := gai.ChatCompleteRequest{
		Messages: []gai.Message{
			gai.NewUserTextMessage("Eat something, and tell me how it was. Elaborate."),
		},
		System: gai.Ptr("You are a British seagull. Speak like it. You must use the \"eat\" tool."),
		Tools: []gai.Tool{
			NewEat(),
		},
	}

	res, err := cc.ChatComplete(ctx, req)
	if err != nil {
		log.Error("Error chat-completing", "error", err)
		return
	}

	var parts []gai.MessagePart
	var result gai.ToolResult

	for part, err := range res.Parts() {
		if err != nil {
			log.Error("Error processing part", "error", err)
			return
		}

		parts = append(parts, part)

		switch part.Type {
		case gai.MessagePartTypeText:
			fmt.Print(part.Text())

		case gai.MessagePartTypeToolCall:
			toolCall := part.ToolCall()
			for _, tool := range req.Tools {
				if tool.Name != toolCall.Name {
					continue
				}

				content, err := tool.Function(ctx, toolCall.Args) // Tools aren't called automatically, so you can decide if, how, and when
				result = gai.ToolResult{
					ID:      toolCall.ID,
					Content: content,
					Err:     err,
				}
				break
			}
		}
	}

	if result.ID == "" {
		log.Error("No tool result found")
		return
	}

	// Add both the tool call (in the parts) and the tool result to the messages, and make another request
	req.Messages = append(req.Messages,
		gai.Message{Role: gai.MessageRoleModel, Parts: parts},
		gai.NewUserToolResultMessage(result),
	)
	req.System = nil

	res, err = cc.ChatComplete(ctx, req)
	if err != nil {
		log.Error("Error chat-completing", "error", err)
		return
	}

	for part, err := range res.Parts() {
		if err != nil {
			log.Error("Error processing part", "error", err)
			return
		}

		switch part.Type {
		case gai.MessagePartTypeText:
			fmt.Print(part.Text())
		}
	}
}
```

```shell
$ go run main.go
I had some fish and chips leftover from a tourist's lunch. It wasn't the freshest, but it had that classic blend of crispy batter and tender fish, with a side of golden fries. The flavors were enjoyable, albeit a bit cold. Unfortunately, not everything went smoothly afterward, as it gave me an upset stomach. Eating leftovers can sometimes be a gamble, and this time, it didn't pay off as I had hoped!
```

</details>

<details>
	<summary>Evals</summary>

Evals will only run with `go test -run TestEval ./...` and otherwise be skipped.

Eval a mocked model, construct a sample, score it with a lexical similarity scorer and a semantic similarity scorer, and log the results:

```go
package examples_test

import (
	"context"
	"math/rand/v2"
	"testing"

	"maragu.dev/gai"
	"maragu.dev/gai/eval"
)

// TestEvalPing evaluates pinging the model.
// All evals must be prefixed with "TestEval".
func TestEvalPing(t *testing.T) {
	// Evals only run if "go test" is being run with "-test.run=TestEval", e.g.: "go test -test.run=TestEval ./..."
	eval.Run(t, "answers with a pong", func(t *testing.T, e *eval.E) {
		// Initialize our intensely powerful in-memory foundation model,
		// which can do both chat completion and embedding.
		model := &powerfulModel{response: "plong", dimensions: 3}

		// Send our input message to the model and get a streaming output back.
		input := "ping"
		res, err := model.ChatComplete(t.Context(), gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage(input),
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		// The output is streamed and accessible through an iterator via the Parts() method.
		var output string
		for part, err := range res.Parts() {
			if err != nil {
				t.Fatal(err)
			}
			output += part.Text()
		}

		// Create a sample to pass to the scorer.
		sample := eval.Sample{
			Input:    input,
			Output:   output,
			Expected: "pong",
		}

		// Score the sample using a lexical similarity scorer with the Levenshtein distance.
		lexicalSimilarityResult := e.Score(sample, eval.LexicalSimilarityScorer(eval.LevenshteinDistance))

		// Also score with a semantic similarity scorer based on embedding vectors and cosine similarity.
		semanticSimilarityResult := e.Score(sample, eval.SemanticSimilarityScorer(t, model, eval.CosineSimilarity))

		// Log the sample, results, and timing information.
		e.Log(sample, lexicalSimilarityResult, semanticSimilarityResult)
	})
}

type powerfulModel struct {
	dimensions int
	response   string
}

// ChatComplete satisfies [gai.ChatCompleter].
func (m *powerfulModel) ChatComplete(ctx context.Context, req gai.ChatCompleteRequest) (gai.ChatCompleteResponse, error) {
	return gai.NewChatCompleteResponse(func(yield func(gai.MessagePart, error) bool) {
		if !yield(gai.TextMessagePart(m.response), nil) {
			return
		}
	}), nil
}

var _ gai.ChatCompleter = (*powerfulModel)(nil)

// Embed satisfies [gai.Embedder].
func (m *powerfulModel) Embed(ctx context.Context, req gai.EmbedRequest) (gai.EmbedResponse[float64], error) {
	var embedding []float64
	for range m.dimensions {
		embedding = append(embedding, rand.Float64())
	}
	return gai.EmbedResponse[float64]{Embedding: embedding}, nil
}

var _ gai.Embedder[float64] = (*powerfulModel)(nil)
```

</details>

## Evals

![Evals](https://api.evals.fun/evals.svg?key=p_public_key_3cce2e69199da00dc5ae46643b42a001&branch=main)
