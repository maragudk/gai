package llm_test

import (
	"testing"

	"maragu.dev/is"

	"maragu.dev/llm"
)

func TestNewAnthropicClient(t *testing.T) {
	t.Run("can create a new client with a token", func(t *testing.T) {
		client := llm.NewAnthropicClient(llm.NewAnthropicClientOptions{Token: "123"})
		is.NotNil(t, client)
	})
}
