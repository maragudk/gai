package google_test

import (
	"strings"
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/google"
)

func TestEmbedder_Embed(t *testing.T) {
	t.Run("can embed a text", func(t *testing.T) {
		c := newClient(t)

		e := c.NewEmbedder(google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding001,
			Dimensions: 768,
		})

		req := gai.EmbedRequest{
			Input: strings.NewReader("Embed this, please."),
		}

		res, err := e.Embed(t.Context(), req)
		is.NotError(t, err)

		is.Equal(t, 768, len(res.Embedding))
	})
}
