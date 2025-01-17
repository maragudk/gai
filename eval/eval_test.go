package eval_test

import (
	"math"
	"testing"

	"maragu.dev/is"

	"maragu.dev/llm/eval"
)

func TestLevenshteinDistanceScorer(t *testing.T) {
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
			scorer := eval.LevenshteinDistanceScorer()
			result := scorer(eval.Sample{Expected: test.expected, Output: test.output})
			is.True(t, math.Abs(float64(test.score-result.Score)) < 0.01)
		})
	}
}

func TestExactMatchScorer(t *testing.T) {
	tests := []struct {
		expected, output string
		score            eval.Score
	}{
		{"", "", 1},
		{"a", "", 0},
		{"", "a", 0},
		{"a", "a", 1},
	}
	for _, test := range tests {
		t.Run(test.expected+" "+test.output, func(t *testing.T) {
			scorer := eval.ExactMatchScorer()
			result := scorer(eval.Sample{Expected: test.expected, Output: test.output})
			is.Equal(t, test.score, result.Score)
		})
	}
}

func TestSemanticMatchScorer(t *testing.T) {
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

			eg := &mockEmbeddingGetter{
				embeddings: map[string][]float64{
					test.expected: test.expectedEmbedding,
					test.output:   test.outputEmbedding,
				},
			}

			scorer := eval.SemanticMatchScorer(eg, eval.CosineSimilarity)
			result := scorer(eval.Sample{Expected: test.expected, Output: test.output})
			is.True(t, math.Abs(float64(test.score-result.Score)) < 0.01)
		})
	}
}

type mockEmbeddingGetter struct {
	embeddings map[string][]float64
}

func (m *mockEmbeddingGetter) GetEmbedding(v string) ([]float64, error) {
	return m.embeddings[v], nil
}
