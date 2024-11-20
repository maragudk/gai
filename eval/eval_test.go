package eval_test

import (
	"fmt"
	"math"
	"testing"

	"maragu.dev/is"
	"maragu.dev/llm/eval"
)

func TestSkipIfNotEvaluating(t *testing.T) {
	t.Run("skips if called like a regular test", func(t *testing.T) {
		mt := &mockT{}
		eval.SkipIfNotEvaluating(mt)
		is.True(t, mt.skipped)
	})
}

func TestSimilarity(t *testing.T) {
	t.Run("fails the test if the score is lower than a threshold", func(t *testing.T) {
		mt := &mockT{}
		eval.Similarity(mt, "a", "b", eval.LevenshteinEvaluator(0.5))
		is.True(t, mt.failed)
		is.Equal(t, `"a" and "b" are dissimilar`, mt.message)
	})

	t.Run("does not fail the test if the score is equal to the expected", func(t *testing.T) {
		mt := &mockT{}
		eval.Similarity(mt, "a", "a", eval.LevenshteinEvaluator(1))
		is.True(t, !mt.failed)
	})
}

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

type mockT struct {
	failed  bool
	message string
	skipped bool
}

func (t *mockT) Helper() {}

func (t *mockT) Errorf(format string, args ...any) {
	t.failed = true
	t.message = fmt.Sprintf(format, args...)
}

func (t *mockT) Skip(args ...any) {
	t.skipped = true
}
