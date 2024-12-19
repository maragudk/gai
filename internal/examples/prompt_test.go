package examples_test

import (
	"testing"

	"maragu.dev/llm/eval"
)

// TestEvalPrompt evaluates the Prompt method.
// All evals must be prefixed with "TestEval".
func TestEvalPrompt(t *testing.T) {
	// Evals only run if "go test" is being run with "-test.run=TestEval", e.g.: "go test -test.run=TestEval ./..."
	eval.Run(t, "answers with a pong", func(e *eval.E) {
		// Initialize our intensely powerful LLM.
		llm := &llm{response: "plong"}

		// Send our input to the LLM and get an output back.
		input := "ping"
		output := llm.Prompt(input)

		// Create a sample to pass to the scorer.
		sample := eval.Sample{
			Expected: "pong",
			Input:    input,
			Output:   output,
		}

		// Score the sample using the Levenshtein distance scorer.
		// The scorer is created inline, but for scorers that need more setup, this can be done elsewhere.
		score := e.Score(sample, eval.LevenshteinDistanceScorer())

		// Log the score to stdout.
		e.Log(score)
	})
}

type llm struct {
	response string
}

func (l *llm) Prompt(request string) string {
	return l.response
}
