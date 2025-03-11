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

Made with ✨sparkles✨ by [maragu](https://www.maragu.dev/).

Does your company depend on this project? [Contact me at markus@maragu.dk](mailto:markus@maragu.dk?Subject=Supporting%20your%20project) to discuss options for a one-time or recurring invoice to ensure its continued thriving.

## Usage

Evals will only run with `go test -run TestEval ./...` and otherwise be skipped.

### Eval a model with lexical and semantic similarity

Eval a mocked model, construct a sample, score it with a lexical similarity scorer and a semantic similarity scorer, and log the results.

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
	eval.Run(t, "answers with a pong", func(e *eval.E) {
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
		semanticSimilarityResult := e.Score(sample, eval.SemanticSimilarityScorer(e.T, model, eval.CosineSimilarity))

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
func (m *powerfulModel) Embed(ctx context.Context, req gai.EmbedRequest) (gai.EmbedResponse[int], error) {
	var embedding []int
	for range m.dimensions {
		embedding = append(embedding, rand.IntN(5))
	}
	return gai.EmbedResponse[int]{Embedding: embedding}, nil
}

var _ gai.Embedder[int] = (*powerfulModel)(nil)
```

## Evals

![Evals](https://api.evals.fun/evals.svg?key=p_public_key_3cce2e69199da00dc5ae46643b42a001&branch=main)
