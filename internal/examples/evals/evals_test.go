package evals_test

import (
	_ "embed"
	"os"
	"testing"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/google"
	"maragu.dev/gai/clients/openai"
	"maragu.dev/gai/eval"
)

//go:embed testdata/logo.jpg
var logo []byte

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
		sample := eval.NewTextSample(input, "Oh, splendid day it is! You know, I'm just floatin' about on the breeze, keepin' an eye out for a cheeky chip or two. Might pop down to the seaside, see if I can nick a sarnie from some unsuspecting holidaymaker. It's a gull's life, innit? How about you, what are you up to?", output)

		// Score the sample using a lexical similarity scorer with the Levenshtein distance.
		lexicalSimilarityResult := e.Score(sample, eval.LexicalSimilarityScorer(eval.LevenshteinDistance))

		// Also score with a semantic similarity scorer based on embedding vectors and cosine similarity.
		semanticSimilarityResult := e.Score(sample, eval.SemanticSimilarityScorer(t, embedder, eval.CosineSimilarity))

		// Log the sample, results, and timing information.
		e.Log(sample, lexicalSimilarityResult, semanticSimilarityResult)
	})
}

// TestEvalImageDescription evaluates how well a model describes an image.
// This demonstrates multimodal evaluation using image input and semantic similarity scoring.
func TestEvalImageDescription(t *testing.T) {
	eval.Run(t, "describes the logo", func(t *testing.T, e *eval.E) {
		gc := google.NewClient(google.NewClientOptions{
			Key: os.Getenv("GOOGLE_API_KEY"),
		})

		cc := gc.NewChatCompleter(google.NewChatCompleterOptions{
			Model: google.ChatCompleteModelGemini2_5Flash,
		})

		embedder := gc.NewEmbedder(google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding2Preview,
			Dimensions: 768,
		})
		// Send the image to the model and ask it to describe what it sees.
		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{
			Messages: []gai.Message{
				{
					Role: gai.MessageRoleUser,
					Parts: []gai.Part{
						gai.DataPart("image/jpeg", logo),
						gai.TextPart("Describe this image in one sentence."),
					},
				},
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		var output string
		for part, err := range res.Parts() {
			if err != nil {
				t.Fatal(err)
			}
			output += part.Text()
		}

		// Create a multimodal sample: input is the image, output and expected are text descriptions.
		sample := eval.Sample{
			Input:    []gai.Part{gai.DataPart("image/jpeg", logo)},
			Output:   []gai.Part{gai.TextPart(output)},
			Expected: []gai.Part{gai.TextPart("A cute cartoon turquoise gopher character on a pink background.")},
		}

		// Score with semantic similarity using the multimodal embedder.
		semanticResult := e.Score(sample, eval.SemanticSimilarityScorer(t, embedder, eval.CosineSimilarity))

		e.Log(sample, semanticResult)
	})
}
