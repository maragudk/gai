package eval

import (
	"fmt"
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

// SimilarityEvaluator is a function that evaluates the similarity between two strings,
// returning true if they are similar enough, and false otherwise.
// It's typically used in a closure.
type SimilarityEvaluator = func(s1, s2 string) bool

// Similarity compares two strings according to the given [SimilarityEvaluator],
// and fails the test if the evaluator returns false.
func Similarity(t errorer, s1, s2 string, se SimilarityEvaluator) {
	t.Helper()

	if !se(s1, s2) {
		t.Errorf(`"%v" and "%v" are dissimilar`, s1, s2)
	}
}

// Score between 0 and 1.
type Score float64

func (s Score) IsValid() {
	if s < 0 || s > 1 {
		panic(fmt.Sprintf("score is %v, must be between 0 and 1", s))
	}
}

func (s Score) String() string {
	// floating point with two decimals
	return fmt.Sprintf("%.2f", float64(s))
}

// LevenshteinEvaluator returns a [SimilarityEvaluator] that uses the [LevenshteinDistanceScore] to compare strings,
// and returns true if the similarity [Score] is greater than or equal to the given threshold.
func LevenshteinEvaluator(threshold Score) SimilarityEvaluator {
	threshold.IsValid()

	return func(s1, s2 string) bool {
		return LevenshteinDistanceScore(s1, s2) >= threshold
	}
}

// LevenshteinDistanceScore computes a similarity [Score] between two strings using the levenshtein distance.
// Uses https://github.com/agnivade/levenshtein
func LevenshteinDistanceScore(s1, s2 string) Score {
	if s1 == s2 {
		return 1
	}
	return Score(1 - float64(levenshtein.ComputeDistance(s1, s2))/float64(max(len(s1), len(s2))))
}
