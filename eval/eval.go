// Package eval lets you evaluate models with various [Scorer] functions.
// It also provides a convenient way to run evaluations as part of the standard Go tests using the [Run] function.
package eval

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/agnivade/levenshtein"

	"maragu.dev/gai"
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

// LexicalSimilarityScorer returns a [Scorer] which uses a lexical similarity metric to compare
// expected and output strings from a [Sample].
// This is a common way to score texts if you have a reference text.
// You can choose which similarity function to use, such as [LevenshteinDistance], [ExactMatch], or [Contains].
func LexicalSimilarityScorer(similarityFunc func(a, b string) Score) Scorer {
	return func(sample Sample) Result {
		score := similarityFunc(sample.Output, sample.Expected)
		return Result{Score: score, Type: "LexicalSimilarity"}
	}
}

// LevenshteinDistance computes a [Score] between two strings using the levenshtein distance,
// and is useful as a lexical similarity metric together with [LexicalSimilarityScorer].
// A score of 1 means the strings are equal, and 0 means they are completely different.
// The score is normalized to the length of the longest string.
// Uses https://github.com/agnivade/levenshtein internally.
func LevenshteinDistance(a, b string) Score {
	if a == b {
		return 1
	}
	return Score(1 - float64(levenshtein.ComputeDistance(a, b))/float64(max(len(a), len(b))))
}

// ExactMatch computes a [Score] between two strings, returning 1 if they are equal and 0 otherwise.
// Useful as a simple [Scorer] for exact string matching together with [LexicalSimilarityScorer].
func ExactMatch(a, b string) Score {
	if a == b {
		return 1
	}
	return 0
}

// Contains computes a [Score] between two strings, returning 1 if the first string contains the second string, and 0 otherwise.
// Useful as a simple [Scorer] for string containment together with [LexicalSimilarityScorer].
func Contains(a, b string) Score {
	if strings.Contains(a, b) {
		return 1
	}
	return 0
}

type fataler interface {
	helper
	Context() context.Context
	Fatal(args ...any)
}

// SemanticSimilarityScorer returns a [Scorer] which uses embedding vectors to compare expected and output strings from a [Sample].
// You can choose which vector similarity function to use. If in doubt, use [CosineSimilarity].
func SemanticSimilarityScorer[T gai.VectorComponent](t fataler, e gai.Embedder[T], similarityFunc func(a, b []T) Score) Scorer {
	return func(sample Sample) Result {
		expected, err := e.Embed(t.Context(), gai.EmbedRequest{Input: strings.NewReader(sample.Expected)})
		if err != nil {
			t.Fatal("could not get embedding for expected string:", err)
		}
		output, err := e.Embed(t.Context(), gai.EmbedRequest{Input: strings.NewReader(sample.Output)})
		if err != nil {
			t.Fatal("could not get embedding for output string:", err)
		}

		score := similarityFunc(expected.Embedding, output.Embedding)
		return Result{Score: score, Type: "SemanticSimilarity"}
	}
}

// CosineSimilarity between two embedding vectors a and b, normalized to a [Score].
func CosineSimilarity[T gai.VectorComponent](a, b []T) Score {
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
