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

		res, err := e.Embed(t.Context(), gai.NewTextEmbedRequest("Embed this, please."))
		is.NotError(t, err)

		is.Equal(t, 1536, len(res.Embedding))
	})

	t.Run("panics with no parts", func(t *testing.T) {
		c := newClient(t)

		e := c.NewEmbedder(openai.NewEmbedderOptions{
			Model:      openai.EmbedModelTextEmbedding3Small,
			Dimensions: 1536,
		})

		defer func() {
			r := recover()
			is.Equal(t, "no parts", r)
		}()

		_, _ = e.Embed(t.Context(), gai.EmbedRequest{})
	})

	t.Run("panics with a non-text part", func(t *testing.T) {
		c := newClient(t)

		e := c.NewEmbedder(openai.NewEmbedderOptions{
			Model:      openai.EmbedModelTextEmbedding3Small,
			Dimensions: 1536,
		})

		defer func() {
			r := recover()
			is.Equal(t, "OpenAI embeddings only support a single text part", r)
		}()

		_, _ = e.Embed(t.Context(), gai.EmbedRequest{
			Parts: []gai.Part{gai.DataPart("image/jpeg", strings.NewReader("not an image"))},
		})
	})

	t.Run("panics with multiple parts", func(t *testing.T) {
		c := newClient(t)

		e := c.NewEmbedder(openai.NewEmbedderOptions{
			Model:      openai.EmbedModelTextEmbedding3Small,
			Dimensions: 1536,
		})

		defer func() {
			r := recover()
			is.Equal(t, "OpenAI embeddings only support a single text part", r)
		}()

		_, _ = e.Embed(t.Context(), gai.EmbedRequest{
			Parts: []gai.Part{gai.TextPart("one"), gai.TextPart("two")},
		})
	})
}
