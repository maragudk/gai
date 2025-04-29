// Package eval lets you evaluate models with various [Scorer] functions.
// It also provides a convenient way to run evaluations as part of the standard Go tests using the [Run] function.
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
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

// LLMJudge defines the configuration for using a language model to evaluate samples.
// It specifies how to format prompts for the model, and how to extract scores from
// the model's responses.
type LLMJudge struct {
	// Temperature controls the randomness of the LLM's output.
	Temperature gai.Temperature

	// PromptFunc generates a prompt for the LLM based on a Sample.
	// The prompt should instruct the LLM how to evaluate the sample's output
	// against its expected output.
	PromptFunc func(s Sample) string

	// ScoreFunc extracts a normalized Score (between 0 and 1) from the LLM's response.
	// This function should parse the LLM's textual output and convert it to a Score.
	ScoreFunc func(response string) (Score, error)
}

// LLMScorer returns a Scorer that uses a language model to evaluate samples.
// It sends prompts generated by judge.PromptFunc to the provided LLM,
// then uses judge.ScoreFunc to extract a score from the LLM's response.
//
// The returned Scorer can be used to get human-like qualitative judgments on
// the similarity between expected and actual outputs.
//
// The fataler parameter t is typically a testing.T instance, used for test failure
// reporting and providing context for the LLM request.
func LLMScorer(t fataler, judge LLMJudge, llm gai.ChatCompleter) Scorer {
	return func(sample Sample) Result {
		prompt := judge.PromptFunc(sample)
		response, err := llm.ChatComplete(t.Context(), gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage(prompt),
			},
		})
		if err != nil {
			t.Fatal("could not get LLM response:", err)
		}

		var output string
		for part, err := range response.Parts() {
			if err != nil {
				t.Fatal(err)
			}
			output += part.Text()
		}

		score, err := judge.ScoreFunc(output)
		if err != nil {
			t.Fatal("could not get LLM score:", err)
		}
		return Result{Score: score, Type: "LLM"}
	}
}

// DefaultJudge returns a basic LLMJudge configuration with the given temperature.
// It uses DefaultJudgePrompt for generating prompts and DefaultScoreFunc for
// extracting scores from LLM responses. This provides a standard way to
// evaluate how well an AI's output matches expected output.
func DefaultJudge(temperature gai.Temperature) LLMJudge {
	return LLMJudge{
		Temperature: temperature,
		PromptFunc:  DefaultJudgePrompt(),
		ScoreFunc:   DefaultScoreFunc(),
	}
}

// DefaultJudgePrompt returns a function that creates a standard prompt for LLM
// evaluation. The prompt instructs the LLM to act as an impartial judge and rate
// how well the actual output matches the expected output on a scale from 0.0 to 1.0.
// The prompt includes the input, expected output, and actual output from the Sample,
// as well as scoring guidelines. The LLM is instructed to provide its reasoning followed
// by a score in the format "Score: X.XX" on the final line.
func DefaultJudgePrompt() func(s Sample) string {
	return func(s Sample) string {
		return fmt.Sprintf(`You are an impartial judge evaluating the quality of an AI system's response.

INPUT:
%s

EXPECTED OUTPUT:
%s

ACTUAL OUTPUT:
%s

Your task is to rate how well the ACTUAL OUTPUT matches the EXPECTED OUTPUT in addressing the INPUT.

Score the response on a scale from 0.0 to 1.0, where:
- 1.0: Perfect match or superior to expected output
- 0.7-0.9: Minor differences that don't affect quality or completeness
- 0.4-0.6: Contains most key information but has notable omissions or differences
- 0.1-0.3: Significant omissions or differences
- 0.0: Completely incorrect or irrelevant

Provide your assessment and reasoning, then on the final line include ONLY your numerical score in format "Score: X.XX" (a number between 0.0 and 1.0).`,
			s.Input, s.Expected, s.Output)
	}
}

// DefaultScoreFunc returns a scoring function that extracts a Score from an LLM's
// response by looking for "Score: X.XX" in the last line. The function expects
// a score between 0.0 and 1.0 and returns an error if the score format is invalid
// or outside the expected range. This function is designed to work with the prompt
// created by DefaultJudgePrompt.
func DefaultScoreFunc() func(response string) (Score, error) {
	return func(response string) (Score, error) {
		// Split the response by newlines
		lines := strings.Split(strings.TrimSpace(response), "\n")
		if len(lines) == 0 {
			return 0, fmt.Errorf("empty response")
		}

		// Get the last line
		lastLine := strings.TrimSpace(lines[len(lines)-1])

		// Check if it matches the expected format
		if !strings.HasPrefix(lastLine, "Score:") {
			return 0, fmt.Errorf("last line doesn't contain a score: %q", lastLine)
		}

		// Extract the score value
		scoreStr := strings.TrimSpace(strings.TrimPrefix(lastLine, "Score:"))
		scoreVal, err := strconv.ParseFloat(scoreStr, 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse score value %q: %v", scoreStr, err)
		}

		// Validate the score range
		if scoreVal < 0 || scoreVal > 1 {
			return 0, fmt.Errorf("score %f is outside valid range [0, 1]", scoreVal)
		}

		return Score(scoreVal), nil
	}
}

type rubricScores struct {
	ContentAccuracy float64 `json:"contentAccuracy"`
	Relevance       float64 `json:"relevance"`
	Completeness    float64 `json:"completeness"`
	Clarity         float64 `json:"clarity"`
}

// RubricJudge returns an LLMJudge configured for multi-dimensional rubric-based evaluation.
// It uses RubricJudgePrompt for generating prompts and RubricScoreFunc for extracting scores
// from LLM responses. This judge evaluates responses on four dimensions: content accuracy,
// relevance, completeness, and clarity.
//
// The temperature parameter controls the randomness of the LLM's output during evaluation.
// Lower values produce more deterministic evaluations.
func RubricJudge(temperature gai.Temperature) LLMJudge {
	return LLMJudge{
		Temperature: temperature,
		PromptFunc:  RubricJudgePrompt(),
		ScoreFunc:   RubricScoreFunc(),
	}
}

// RubricJudgePrompt returns a function that creates prompts for rubric-based LLM evaluation.
// The prompt instructs the LLM to evaluate the actual response against the expected response
// on four dimensions (content accuracy, relevance, completeness, and clarity) using a 0-10 scale.
//
// The LLM is instructed to provide brief reasoning for each score and output a valid JSON object
// containing the numerical scores. This structured approach enables more detailed analysis
// of response quality across multiple dimensions.
func RubricJudgePrompt() func(s Sample) string {
	return func(s Sample) string {
		return fmt.Sprintf(`You are an expert evaluator assessing an AI response against expectations.

INPUT QUERY:
%s

EXPECTED RESPONSE:
%s

ACTUAL RESPONSE:
%s

Evaluate the ACTUAL RESPONSE against the EXPECTED RESPONSE on these dimensions:
1. Content Accuracy (0-10): How accurately does it include all key information?
2. Relevance (0-10): How well does it directly address the input query?
3. Completeness (0-10): How thoroughly does it cover all necessary points?
4. Clarity (0-10): How clear and understandable is the response?

Provide brief reasoning for each score. Then output a JSON object with your scores in this exact format:

`+"```"+`json
{
  "contentAccuracy": X,
  "relevance": X,
  "completeness": X,
  "clarity": X
}
`+"```"+`

Ensure the JSON is valid and contains only numeric values between 0 and 10 for each dimension.`, s.Input, s.Expected, s.Output)
	}
}

// RubricScoreFunc returns a scoring function that extracts a normalized Score from an LLM's
// rubric-based evaluation response. It searches for a JSON object in the response, parses the
// scores for each dimension (content accuracy, relevance, completeness, and clarity), validates
// that all scores are within the 0-10 range, and calculates a final score normalized to the 0-1 range.
//
// The function returns an error if it cannot find valid JSON, if parsing fails, or if any
// dimension score is outside the valid range. This function is designed to work with responses
// generated from prompts created by RubricJudgePrompt.
func RubricScoreFunc() func(response string) (Score, error) {
	return func(response string) (Score, error) {
		// Extract JSON from the response
		jsonStart := strings.Index(response, "{")
		jsonEnd := strings.LastIndex(response, "}")

		if jsonStart == -1 || jsonEnd == -1 || jsonEnd < jsonStart {
			return 0, fmt.Errorf("could not find valid JSON in response")
		}

		jsonStr := response[jsonStart : jsonEnd+1]

		// Parse the JSON into the rubricScores struct
		var scores rubricScores
		if err := json.Unmarshal([]byte(jsonStr), &scores); err != nil {
			return 0, fmt.Errorf("failed to parse JSON: %v", err)
		}

		// Validate score ranges
		if scores.ContentAccuracy < 0 || scores.ContentAccuracy > 10 ||
			scores.Relevance < 0 || scores.Relevance > 10 ||
			scores.Completeness < 0 || scores.Completeness > 10 ||
			scores.Clarity < 0 || scores.Clarity > 10 {
			return 0, fmt.Errorf("one or more scores are outside the valid range [0, 10]")
		}

		// Calculate the final normalized score
		sum := scores.ContentAccuracy + scores.Relevance + scores.Completeness + scores.Clarity
		finalScore := sum / 40.0

		return Score(finalScore), nil
	}
}
