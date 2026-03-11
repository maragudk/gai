package eval_test

import (
	"testing"

	"maragu.dev/gai/eval"
)

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("skips if called like a regular test", func(t *testing.T) {
		t.Parallel()

		eval.Run(t, "some eval", func(t *testing.T, e *eval.E) {
			// This will not be reached because the test is skipped
			t.FailNow()
		})
	})
}
