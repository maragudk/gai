package examples_test

import (
	"testing"

	"maragu.dev/llm/eval"
)

// TestEvalPrompt evaluates the Prompt method.
// All evals must be prefixed with "TestEval".
func TestEvalPrompt(t *testing.T) {
	// Skip the test if not evaluating, by running the test suite with "go test -run TestEval".
	eval.SkipIfNotEvaluating(t)

	t.Run("answers with a pong", func(t *testing.T) {
		llm := &llm{response: "plong"}
		response := llm.Prompt("ping")
		eval.Similarity(t, "pong", response, eval.LevenshteinEvaluator(0.8))
	})
}

type llm struct {
	response string
}

func (l *llm) Prompt(request string) string {
	return l.response
}
