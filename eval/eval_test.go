package eval_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/eval"
)

func TestSample_MarshalJSON(t *testing.T) {
	t.Run("renders parts as summaries instead of spelling out data", func(t *testing.T) {
		s := eval.Sample{
			Input:    []gai.Part{gai.TextPart("what is this?"), gai.DataPart("image/jpeg", []byte("fake image"))},
			Expected: []gai.Part{gai.TextPart("a logo")},
			Output:   []gai.Part{gai.TextPart("a logo")},
		}

		data, err := json.Marshal(s)
		is.NotError(t, err)

		is.Equal(t, `{"Input":["what is this?","[data: image/jpeg, 10 bytes]"],"Expected":["a logo"],"Output":["a logo"]}`, string(data))
	})
}

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
				result := scorer(eval.NewTextSample("", test.expected, test.output))
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
				result := scorer(eval.NewTextSample("", test.expected, test.output))
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
				result := scorer(eval.NewTextSample("", test.expected, test.output))
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
			result := scorer(eval.NewTextSample("", test.expected, test.output))
			is.True(t, math.Abs(float64(test.score-result.Score)) < 0.01)
		})
	}
}

type embedder struct {
	embeddings map[string][]float64
}

func (m *embedder) Embed(ctx context.Context, req gai.EmbedRequest) (gai.EmbedResponse[float64], error) {
	v := req.Parts[0].Text()
	return gai.EmbedResponse[float64]{Embedding: m.embeddings[v]}, nil
}
