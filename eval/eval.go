// Package eval lets you evaluate LLM output by scoring it with various scoring methods, and logging the result.
// It provides a convenient way to run evaluations as part of the standard Go tests using the [Run] function.
package eval

import (
	"fmt"

	"github.com/agnivade/levenshtein"
)

type Sample struct {
	Expected string
	Input    string
	Output   string
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

// Result of an evaluation with a [Score] and the type of the [Score].
type Result struct {
	Score Score
	Type  string
}

// Scorer produces a [Result] with a [Score] for a [Sample].
type Scorer = func(s Sample) Result

// LevenshteinDistanceScorer returns a [Scorer] that uses the [LevenshteinDistanceScore] to compare strings.
func LevenshteinDistanceScorer() Scorer {
	return func(sample Sample) Result {
		score := LevenshteinDistanceScore(sample.Expected, sample.Output)
		return Result{Score: score, Type: "LevenshteinDistance"}
	}
}

// LevenshteinDistanceScore computes a [Score] between two strings using the levenshtein distance.
// A score of 1 means the strings are equal, and 0 means they are completely different.
// Uses https://github.com/agnivade/levenshtein
func LevenshteinDistanceScore(s1, s2 string) Score {
	if s1 == s2 {
		return 1
	}
	return Score(1 - float64(levenshtein.ComputeDistance(s1, s2))/float64(max(len(s1), len(s2))))
}

// ExactMatchScorer returns a [Scorer] that scores 1 if the expected and output strings are equal, and 0 otherwise.
func ExactMatchScorer() Scorer {
	return func(sample Sample) Result {
		if sample.Expected == sample.Output {
			return Result{Score: 1, Type: "ExactMatch"}
		}
		return Result{Score: 0, Type: "ExactMatch"}
	}
}
