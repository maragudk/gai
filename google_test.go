package llm_test

import (
	"testing"

	"maragu.dev/is"

	"maragu.dev/llm"
)

func TestNewGoogleClient(t *testing.T) {
	t.Run("can create a new client with a token", func(t *testing.T) {
		client := llm.NewGoogleClient(llm.NewGoogleClientOptions{Token: "123"})
		is.NotNil(t, client)
	})
}
