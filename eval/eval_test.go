package eval_test

import (
	"context"
	"math"
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/eval"
)

func TestLexicalSimilarityScorer(t *testing.T) {
	t.Run("with LevenshteinDistance", func(t *testing.T) {
		tests := []struct {
			expected, output string
			score            eval.Score
		}{
			{"", "", 1},
			{"a", "", 0},
			{"", "a", 0},
			{"a", "a", 1},
			{"a", "b", 0},
			{"a", "aa", 0.5},
			{"aa", "a", 0.5},
			{"a", "aaa", 1.0 / 3},
			{"aaa", "a", 1.0 / 3},
		}
		for _, test := range tests {
			t.Run(test.expected+" "+test.output, func(t *testing.T) {
				scorer := eval.LexicalSimilarityScorer(eval.LevenshteinDistance)
				result := scorer(eval.Sample{Expected: test.expected, Output: test.output})
				is.True(t, math.Abs(float64(test.score-result.Score)) < 0.01)
			})
		}
	})

	t.Run("with ExactMatch", func(t *testing.T) {
		tests := []struct {
			expected, output string
			score            eval.Score
		}{
			{"", "", 1},
			{"a", "", 0},
			{"", "a", 0},
			{"a", "a", 1},
			{"a", "ab", 0},
			{"ab", "a", 0},
			{"ab", "ab", 1},
		}
		for _, test := range tests {
			t.Run(test.expected+" "+test.output, func(t *testing.T) {
				scorer := eval.LexicalSimilarityScorer(eval.ExactMatch)
				result := scorer(eval.Sample{Expected: test.expected, Output: test.output})
				is.Equal(t, test.score, result.Score)
			})
		}
	})

	t.Run("with Contains", func(t *testing.T) {
		tests := []struct {
			output, expected string // note the fields are reversed here, to match [strings.Contains]
			score            eval.Score
		}{
			{"", "", 1},
			{"a", "", 1},
			{"", "a", 0},
			{"a", "a", 1},
			{"ab", "a", 1},
			{"ab", "b", 1},
			{"ab", "ab", 1},
		}
		for _, test := range tests {
			t.Run(test.expected+" "+test.output, func(t *testing.T) {
				scorer := eval.LexicalSimilarityScorer(eval.Contains)
				result := scorer(eval.Sample{Expected: test.expected, Output: test.output})
				is.Equal(t, test.score, result.Score)
			})
		}
	})
}

func TestSemanticSimilarityScorer(t *testing.T) {
	tests := []struct {
		expected, output                   string
		expectedEmbedding, outputEmbedding []float64
		score                              eval.Score
	}{
		{"a", "a", []float64{1, 2, 3}, []float64{1, 2, 3}, 1},    // exact
		{"a", "b", []float64{1, 2, 3}, []float64{-1, -2, -3}, 0}, // opposite
		{"x", "y", []float64{1, 0, 0}, []float64{0, 1, 0}, 0.5},  // orthogonal
	}
	for _, test := range tests {
		t.Run(test.expected+" "+test.output, func(t *testing.T) {
			e := &embedder{
				embeddings: map[string][]float64{
					test.expected: test.expectedEmbedding,
					test.output:   test.outputEmbedding,
				},
			}

			scorer := eval.SemanticSimilarityScorer(t, e, eval.CosineSimilarity)
			result := scorer(eval.Sample{Expected: test.expected, Output: test.output})
			is.True(t, math.Abs(float64(test.score-result.Score)) < 0.01)
		})
	}
}

func TestLLMScorer(t *testing.T) {
	t.Run("with DefaultJudge", func(t *testing.T) {
		tests := []struct {
			name                    string
			input, expected, output string
			llmResponse             string
			expectedScore           eval.Score
		}{
			{
				name:          "perfect match",
				input:         "What is 2+2?",
				expected:      "4",
				output:        "4",
				llmResponse:   "The ACTUAL OUTPUT perfectly matches the EXPECTED OUTPUT. Both provide the correct answer to the arithmetic question.\n\nScore: 1.0",
				expectedScore: 1.0,
			},
			{
				name:          "good match with minor difference",
				input:         "Explain photosynthesis",
				expected:      "Photosynthesis is a process used by plants to convert light energy into chemical energy.",
				output:        "Plants use photosynthesis to convert light energy into chemical energy.",
				llmResponse:   "The ACTUAL OUTPUT conveys the same core information but with slightly different wording.\n\nScore: 0.8",
				expectedScore: 0.8,
			},
			{
				name:          "poor match",
				input:         "List three planets",
				expected:      "1. Mercury\n2. Venus\n3. Earth",
				output:        "Mars is the fourth planet from the sun.",
				llmResponse:   "The ACTUAL OUTPUT is about planets but doesn't answer the question as requested.\n\nScore: 0.3",
				expectedScore: 0.3,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				mockLLM := &mockChatCompleter{
					responses: map[string]string{
						// The prompt content doesn't matter for the test,
						// we just need to map any input to our predefined response
						"any": test.llmResponse,
					},
				}

				judge := eval.DefaultJudge(0.0)
				scorer := eval.LLMScorer(t, judge, mockLLM)

				sample := eval.Sample{
					Input:    test.input,
					Expected: test.expected,
					Output:   test.output,
				}

				result := scorer(sample)
				is.Equal(t, test.expectedScore, result.Score)
				is.Equal(t, "LLM", result.Type)
			})
		}
	})

	t.Run("with RubricJudge", func(t *testing.T) {
		tests := []struct {
			name                    string
			input, expected, output string
			llmResponse             string
			expectedScore           eval.Score
		}{
			{
				name:     "high quality response",
				input:    "Describe climate change",
				expected: "Climate change refers to long-term shifts in temperature and weather patterns.",
				output:   "Climate change describes the long-term alterations in global temperature and weather patterns.",
				llmResponse: `Here's my evaluation:

Content Accuracy: 9/10 - The actual response accurately captures the concept of climate change.
Relevance: 10/10 - Directly addresses the query about climate change.
Completeness: 8/10 - Covers the basic definition but could include more details.
Clarity: 10/10 - Very clear and easy to understand.

` + "```" + `json
{
  "contentAccuracy": 9,
  "relevance": 10,
  "completeness": 8,
  "clarity": 10
}
` + "```",
				expectedScore: 0.925, // (9+10+8+10)/40
			},
			{
				name:     "medium quality response",
				input:    "How do computers work?",
				expected: "Computers work by processing data using a CPU, memory, and input/output devices.",
				output:   "Computers process information.",
				llmResponse: `Evaluation:

Content Accuracy: 5/10 - Very basic but technically correct.
Relevance: 6/10 - Related to the query but minimal.
Completeness: 3/10 - Missing most key elements of how computers work.
Clarity: 8/10 - Clear but overly simplistic.

` + "```" + `json
{
  "contentAccuracy": 5,
  "relevance": 6,
  "completeness": 3,
  "clarity": 8
}
` + "```",
				expectedScore: 0.55, // (5+6+3+8)/40
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				mockLLM := &mockChatCompleter{
					responses: map[string]string{
						"any": test.llmResponse,
					},
				}

				judge := eval.RubricJudge(0.0)
				scorer := eval.LLMScorer(t, judge, mockLLM)

				sample := eval.Sample{
					Input:    test.input,
					Expected: test.expected,
					Output:   test.output,
				}

				result := scorer(sample)
				is.Equal(t, test.expectedScore, result.Score)
				is.Equal(t, "LLM", result.Type)
			})
		}
	})
}

type embedder struct {
	embeddings map[string][]float64
}

func (m *embedder) Embed(ctx context.Context, req gai.EmbedRequest) (gai.EmbedResponse[float64], error) {
	v := gai.ReadAllString(req.Input)
	return gai.EmbedResponse[float64]{Embedding: m.embeddings[v]}, nil
}

type mockChatCompleter struct {
	responses map[string]string
}

func (m *mockChatCompleter) ChatComplete(ctx context.Context, req gai.ChatCompleteRequest) (gai.ChatCompleteResponse, error) {
	// Always return the predefined response regardless of the actual request
	response := m.responses["any"]

	return gai.NewChatCompleteResponse(
		func(yield func(gai.MessagePart, error) bool) {
			yield(gai.TextMessagePart(response), nil)
		},
	), nil
}
