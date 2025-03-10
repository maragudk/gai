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

### Simple example

Eval a mocked model, construct a sample, score it with a lexical similarity scorer and a semantic similarity scorer, and log the results.

```go
package examples_test

import (
	"context"
	"io"
	"testing"

	"maragu.dev/gai"
	"maragu.dev/gai/eval"
)

// TestEvalPing evaluates the Ping method.
// All evals must be prefixed with "TestEval".
func TestEvalPing(t *testing.T) {
	// Evals only run if "go test" is being run with "-test.run=TestEval", e.g.: "go test -test.run=TestEval ./..."
	eval.Run(t, "answers with a pong", func(e *eval.E) {
		// Initialize our intensely powerful in-memory foundation model.
		model := &powerfulModel{response: "plong"}

		// Send our input to the model and get an output back.
		input := "ping"
		output := model.Prompt(input)

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
	response string
}

func (m *powerfulModel) Prompt(request string) string {
	return m.response
}

func (m *powerfulModel) Embed(ctx context.Context, r io.Reader) (gai.EmbedResponse[int], error) {
	return gai.EmbedResponse[int]{Embedding: []int{1, 2, 3}}, nil
}
```

## Evals

![Evals](https://api.evals.fun/evals.svg?key=p_public_key_3cce2e69199da00dc5ae46643b42a001&branch=main)
