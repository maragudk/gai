package openai_test

import (
	"strings"
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/openai"
)

func TestEmbedder_Embed(t *testing.T) {
	t.Run("can embed a text", func(t *testing.T) {
		c := newClient(t)

		e := c.NewEmbedder(openai.NewEmbedderOptions{
			Model:      openai.EmbedModelTextEmbedding3Small,
			Dimensions: 1536,
		})

		req := gai.EmbedRequest{
			Input: strings.NewReader("Embed this, please."),
		}

		res, err := e.Embed(t.Context(), req)
		is.NotError(t, err)

		is.Equal(t, 1536, len(res.Embedding))
	})
}
