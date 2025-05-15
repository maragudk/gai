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
