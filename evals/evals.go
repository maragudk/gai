package evals

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

// SkipIfNotEval skips the test if "go test" is not being run with "-test.run=TestEval*".
func SkipIfNotEval(t skipper) {
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

// Similar compares two strings and fails the test if the similarity score is less than the expected score,
// using the given similarity score function.
func Similar(t errorer, s1, s2 string, expected float64, fn SimilarityScoreFunc) {
	t.Helper()

	if expected < 0 || expected > 1 {
		panic("expected similarity score should be between 0 and 1")
	}

	actual := fn(s1, s2)

	if actual < expected {
		t.Errorf(`Similarity between "%v" and "%v" is %v < %v`, s1, s2, actual, expected)
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
