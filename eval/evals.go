package eval

import (
	"os"
	"strings"

	"github.com/agnivade/levenshtein"
)

type helper interface {
	Helper()
}

type skipper interface {
	helper
	Skip(args ...any)
}

// SkipIfNotEvaluating skips the test if "go test" is not being run with "-test.run=TestEval".
func SkipIfNotEvaluating(t skipper) {
	t.Helper()

	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.run=TestEval") {
			return
		}
	}

	t.Skip("skipping eval")
}

type errorer interface {
	helper
	Errorf(format string, args ...any)
}

// SimilarityScoreFunc is a function that computes the similarity score between two strings.
// The score is between 0 and 1, where 0 means the strings are completely different
// and 1 means the strings are identical.
type SimilarityScoreFunc = func(s1, s2 string) float64

// Similarity compares two strings according to the given similarity score function,
// and fails the test if the score is lower than the threshold.
func Similarity(t errorer, s1, s2 string, threshold float64, fn SimilarityScoreFunc) {
	t.Helper()

	if threshold < 0 || threshold > 1 {
		panic("similarity score threshold should be between 0 and 1")
	}

	score := fn(s1, s2)
	if score < threshold {
		t.Errorf(`Similarity between "%v" and "%v" is %v < %v`, s1, s2, score, threshold)
	}
}

// LevenshteinSimilarityScore computes the similarity score between two strings using the levenshtein distance.
// Uses https://github.com/agnivade/levenshtein
func LevenshteinSimilarityScore(s1, s2 string) float64 {
	if s1 == s2 {
		return 1
	}
	return 1 - float64(levenshtein.ComputeDistance(s1, s2))/float64(max(len(s1), len(s2)))
}
