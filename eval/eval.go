// Package eval lets you evaluate LLM output with various [Scorer] functions.
// It also provides a convenient way to run evaluations as part of the standard Go tests using the [Run] function.
package eval

import (
	"fmt"
	"math"

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

// String satisfies [fmt.Stringer].
func (s Score) String() string {
	// floating point with two decimals
	return fmt.Sprintf("%.2f", float64(s))
}

// Result of an evaluation with a [Score] and the type of the [Score].
type Result struct {
	Score Score
	Type  string
}

// Scorer produces a [Result] (including a [Score]) for the given [Sample].
type Scorer = func(s Sample) Result

// LevenshteinDistanceScorer returns a [Scorer] that uses the Levenshtein distance to compare strings.
// This is a common lexical similarity metric which is useful if you have a reference text.
// The scorer computes the distance between the expected (reference) and output strings of the [Sample],
// and then normalizes it to a [Score] between 0 and 1 using the max length of the two strings.
func LevenshteinDistanceScorer() Scorer {
	return func(sample Sample) Result {
		score := levenshteinDistanceScore(sample.Expected, sample.Output)
		return Result{Score: score, Type: "LevenshteinDistance"}
	}
}

// levenshteinDistanceScore computes a [Score] between two strings using the levenshtein distance.
// A score of 1 means the strings are equal, and 0 means they are completely different.
// Uses https://github.com/agnivade/levenshtein
func levenshteinDistanceScore(s1, s2 string) Score {
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

// VectorComponent is a single component of a vector.
type VectorComponent interface {
	~float32 | ~float64
}

type embeddingGetter[T VectorComponent] interface {
	GetEmbedding(v string) ([]T, error)
}

// SemanticMatchScorer returns a [Scorer] which uses embedding vectors to compare expected and output strings from a [Sample].
// You can choose which vector similarity function to use. If in doubt, use [CosineSimilarity].
func SemanticMatchScorer[T VectorComponent](eg embeddingGetter[T], similarityFunc func(a, b []T) Score) Scorer {
	return func(sample Sample) Result {
		expected, err := eg.GetEmbedding(sample.Expected)
		if err != nil {
			panic("could not get embedding for expected string: " + err.Error())
		}
		output, err := eg.GetEmbedding(sample.Output)
		if err != nil {
			panic("could not get embedding for output string: " + err.Error())
		}

		score := similarityFunc(expected, output)
		return Result{Score: score, Type: "SemanticMatch"}
	}
}

// CosineSimilarity between two embedding vectors a and b, normalized to a [Score].
func CosineSimilarity[T VectorComponent](a, b []T) Score {
	if len(a) != len(b) {
		panic(fmt.Sprintf("vectors must have equal length, but are lengths %v and %v", len(a), len(b)))
	}

	if len(a) == 0 {
		panic("vectors cannot be empty")
	}

	// Compute dot product and Euclidean norm (L2 norm)
	var dotProduct, normA, normB T
	for i := range len(a) {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	normA = T(math.Sqrt(float64(normA)))
	normB = T(math.Sqrt(float64(normB)))

	if normA == 0 || normB == 0 {
		panic("norm of a or b is zero and cosine similarity is undefined")
	}

	similarity := dotProduct / (normA * normB)

	// Normalize from [-1, 1] to [0, 1] range
	normalizedSimilarity := (similarity + 1) / 2

	// Clamp to [0, 1] range, may be necessary because of floating point rounding errors
	if normalizedSimilarity < 0 {
		return 0
	}
	if normalizedSimilarity > 1 {
		return 1
	}

	return Score(normalizedSimilarity)
}
