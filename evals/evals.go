package evals

import (
	"os"
	"strings"
	"testing"
)

func SkipIfNotEval(t *testing.T) {
	t.Helper()

	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.run=TestEval") {
			return
		}
	}

	t.Skip("skipping eval")
}
