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

// Scorer produces a [Score] for a [Sample].
type Scorer = func(s Sample) Score

// LevenshteinDistanceScorer returns a [Scorer] that uses the [LevenshteinDistanceScore] to compare strings.
func LevenshteinDistanceScorer() Scorer {
	return func(s Sample) Score {
		return LevenshteinDistanceScore(s.Expected, s.Output)
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
