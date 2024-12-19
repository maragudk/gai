package eval_test

import (
	"math"
	"testing"

	"maragu.dev/is"

	"maragu.dev/llm/eval"
)

func TestLevenshteinDistanceScore(t *testing.T) {
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
	for _, tt := range tests {
		t.Run(tt.s1+" "+tt.s2, func(t *testing.T) {
			score := eval.LevenshteinDistanceScore(tt.s1, tt.s2)
			is.True(t, math.Abs(float64(tt.score-score)) < 0.01)
		})
	}
}
