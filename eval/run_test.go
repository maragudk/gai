package eval_test

import (
	"testing"

	"maragu.dev/gai/eval"
)

func TestRun(t *testing.T) {
	t.Run("skips if called like a regular test", func(t *testing.T) {
		eval.Run(t, "some eval", func(e *eval.E) {
			// This will not be reached because the test is skipped
			e.T.FailNow()
		})
	})
}
