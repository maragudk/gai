package examples_test

import (
	"testing"

	"maragu.dev/llm/eval"
)

func TestEvalPrompt(t *testing.T) {
	eval.SkipIfNotEval(t)

	t.Run("answers with a pong", func(t *testing.T) {
		llm := &llm{response: "plong"}
		response := llm.Prompt("ping")
		eval.Similar(t, "pong", response, 0.8, eval.LevenshteinSimilarityScore)
	})
}

type llm struct {
	response string
}

func (l *llm) Prompt(request string) string {
	return l.response
}
