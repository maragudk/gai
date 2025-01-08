package eval_test

import (
	"math"
	"testing"

	"maragu.dev/is"

	"maragu.dev/llm/eval"
)

func TestLevenshteinDistanceScorer(t *testing.T) {
	tests := []struct {
		s1, s2 string
		score  eval.Score
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
		t.Run(test.s1+" "+test.s2, func(t *testing.T) {
			scorer := eval.LevenshteinDistanceScorer()
			result := scorer(eval.Sample{Expected: test.s1, Output: test.s2})
			is.True(t, math.Abs(float64(test.score-result.Score)) < 0.01)
		})
	}
}

func TestExactMatchScorer(t *testing.T) {
	tests := []struct {
		s1, s2 string
		score  eval.Score
	}{
		{"", "", 1},
		{"a", "", 0},
		{"", "a", 0},
		{"a", "a", 1},
	}
	for _, test := range tests {
		t.Run(test.s1+" "+test.s2, func(t *testing.T) {
			scorer := eval.ExactMatchScorer()
			result := scorer(eval.Sample{Expected: test.s1, Output: test.s2})
			is.Equal(t, test.score, result.Score)
		})
	}
}
