package evals

import (
	"os"
	"strings"
	"testing"
)

// SkipIfNotEval skips the test if "go test" is not being run with "-test.run=TestEval*".
func SkipIfNotEval(t *testing.T) {
	t.Helper()

	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.run=TestEval") {
			return
		}
	}

	t.Skip("skipping eval")
}
