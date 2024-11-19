# llm

<img src="logo.png" alt="Logo" width="300" align="right">

[![GoDoc](https://pkg.go.dev/badge/maragu.dev/llm)](https://pkg.go.dev/maragu.dev/llm)
[![Go](https://github.com/maragudk/llm/actions/workflows/ci.yml/badge.svg)](https://github.com/maragudk/llm/actions/workflows/ci.yml)

LLM tools and helpers in Go.

⚠️ **This library is in development**. Things will probably break, but existing functionality is usable. ⚠️

```shell
go get maragu.dev/llm
```

Made with ✨sparkles✨ by [maragu](https://www.maragu.dev/).

Does your company depend on this project? [Contact me at markus@maragu.dk](mailto:markus@maragu.dk?Subject=Supporting%20your%20project) to discuss options for a one-time or recurring invoice to ensure its continued thriving.

## Usage

```go
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
```
