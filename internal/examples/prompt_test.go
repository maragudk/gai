package examples_test

import (
	"testing"

	"maragu.dev/llm/evals"
)

func TestEvalPrompt(t *testing.T) {
	evals.SkipIfNotEval(t)

	t.Run("answers with a pong", func(t *testing.T) {
		llm := &llm{response: "plong"}
		response := llm.Prompt("ping")
		evals.Similar(t, "pong", response, 0.8, evals.LevenshteinSimilarityScore)
	})
}

type llm struct {
	response string
}

func (l *llm) Prompt(request string) string {
	return l.response
}
