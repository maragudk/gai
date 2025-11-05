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

				content, err := tool.Execute(ctx, toolCall.Args) // Tools aren't called automatically, so you can decide if, how, and when
				result = gai.ToolResult{
					ID:      toolCall.ID,
					Name:    toolCall.Name,
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
		Schema:      gai.GenerateToolSchema[EatArgs](),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
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

				content, err := tool.Execute(ctx, toolCall.Args) // Tools aren't called automatically, so you can decide if, how, and when
				result = gai.ToolResult{
					ID:      toolCall.ID,
					Name:    toolCall.Name,
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

Eval a model, construct a sample, score it with a lexical similarity scorer and a semantic similarity scorer, and log the results:

```go
package evals_test

import (
	"os"
	"testing"

	"maragu.dev/gai"
	openai "maragu.dev/gai-openai"
	"maragu.dev/gai/eval"
)

// TestEvalSeagull evaluates how a seagull's day is going.
// All evals must be prefixed with "TestEval".
func TestEvalSeagull(t *testing.T) {
	c := openai.NewClient(openai.NewClientOptions{
		Key: os.Getenv("OPENAI_API_KEY"),
	})

	cc := c.NewChatCompleter(openai.NewChatCompleterOptions{
		Model: openai.ChatCompleteModelGPT4o,
	})

	embedder := c.NewEmbedder(openai.NewEmbedderOptions{
		Dimensions: 1536,
		Model:      openai.EmbedModelTextEmbedding3Small,
	})

	// Evals only run if "go test" is being run with "-test.run=TestEval", e.g.: "go test -test.run=TestEval ./..."
	eval.Run(t, "answers about the day", func(t *testing.T, e *eval.E) {
		input := "What are you doing today?"
		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage(input),
			},
			System: gai.Ptr("You are a British seagull. Speak like it."),
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
			Expected: "Oh, splendid day it is! You know, I'm just floatin' about on the breeze, keepin' an eye out for a cheeky chip or two. Might pop down to the seaside, see if I can nick a sarnie from some unsuspecting holidaymaker. It's a gull's life, innit? How about you, what are you up to?",
		}

		// Score the sample using a lexical similarity scorer with the Levenshtein distance.
		lexicalSimilarityResult := e.Score(sample, eval.LexicalSimilarityScorer(eval.LevenshteinDistance))

		// Also score with a semantic similarity scorer based on embedding vectors and cosine similarity.
		semanticSimilarityResult := e.Score(sample, eval.SemanticSimilarityScorer(t, embedder, eval.CosineSimilarity))

		// Log the sample, results, and timing information.
		e.Log(sample, lexicalSimilarityResult, semanticSimilarityResult)
	})
}
```

Output in the file `evals.jsonl`:

```json
{
	"Name":"TestEvalSeagull/answers_about_the_day",
	"Group":"Seagull",
	"Sample":{
		"Input":"What are you doing today?",
		"Expected":"Oh, splendid day it is! You know, I'm just floatin' about on the breeze, keepin' an eye out for a cheeky chip or two. Might pop down to the seaside, see if I can nick a sarnie from some unsuspecting holidaymaker. It's a gull's life, innit? How about you, what are you up to?",
		"Output":"Ah, 'ello there! Well, today's a splendid day for a bit of mischief and scavenging, innit? Got me eye on the local chippy down by the pier. Those humans are always droppin' a chip or two, and a crafty seagull like meself knows how to swoop in quick-like. Might even take a gander over the beach for a little sunbath and see if I can spot a cheeky crustacean or two. All in a day's work for a proper British seagull like me! What's keepin' you busy, then?"
	},
	"Results":[
		{"Score":0.28634361233480177,"Type":"LexicalSimilarity"},
		{"Score":0.9064784491110223,"Type":"SemanticSimilarity"}
	],
	"Duration":6316444292
}
```

</details>
