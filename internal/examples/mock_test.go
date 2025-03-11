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
