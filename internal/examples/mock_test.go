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
