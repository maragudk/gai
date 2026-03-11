package google_test

import (
	"bytes"
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

		res, err := e.Embed(t.Context(), gai.NewTextEmbedRequest("Embed this, please."))
		is.NotError(t, err)

		is.Equal(t, 768, len(res.Embedding))
	})

	t.Run("panics with no parts", func(t *testing.T) {
		c := newClient(t)

		e := c.NewEmbedder(google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding001,
			Dimensions: 768,
		})

		defer func() {
			r := recover()
			is.Equal(t, "no parts", r)
		}()

		_, _ = e.Embed(t.Context(), gai.EmbedRequest{})
	})

	t.Run("panics with unsupported part type", func(t *testing.T) {
		c := newClient(t)

		e := c.NewEmbedder(google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding001,
			Dimensions: 768,
		})

		defer func() {
			r := recover()
			is.Equal(t, "unsupported part type for embedding: tool_call", r)
		}()

		_, _ = e.Embed(t.Context(), gai.EmbedRequest{
			Parts: []gai.Part{gai.ToolCallPart("id", "name", nil)},
		})
	})

	t.Run("can embed an image", func(t *testing.T) {
		c := newClient(t)

		e := c.NewEmbedder(google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding2Preview,
			Dimensions: 768,
		})

		req := gai.EmbedRequest{
			Parts: []gai.Part{gai.DataPart("image/jpeg", bytes.NewReader(image))},
		}

		res, err := e.Embed(t.Context(), req)
		is.NotError(t, err)

		is.Equal(t, 768, len(res.Embedding))
	})

	t.Run("can embed audio", func(t *testing.T) {
		c := newClient(t)

		e := c.NewEmbedder(google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding2Preview,
			Dimensions: 768,
		})

		req := gai.EmbedRequest{
			Parts: []gai.Part{gai.DataPart("audio/mp4", bytes.NewReader(audio))},
		}

		res, err := e.Embed(t.Context(), req)
		is.NotError(t, err)

		is.Equal(t, 768, len(res.Embedding))
	})

	t.Run("can embed video", func(t *testing.T) {
		c := newClient(t)

		e := c.NewEmbedder(google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding2Preview,
			Dimensions: 768,
		})

		req := gai.EmbedRequest{
			Parts: []gai.Part{gai.DataPart("video/quicktime", bytes.NewReader(video))},
		}

		res, err := e.Embed(t.Context(), req)
		is.NotError(t, err)

		is.Equal(t, 768, len(res.Embedding))
	})

	t.Run("can embed a mixture of text, image, audio, and video", func(t *testing.T) {
		c := newClient(t)

		e := c.NewEmbedder(google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding2Preview,
			Dimensions: 768,
		})

		req := gai.EmbedRequest{
			Parts: []gai.Part{
				gai.TextPart("A multimedia embedding test."),
				gai.DataPart("image/jpeg", bytes.NewReader(image)),
				gai.DataPart("audio/mp4", bytes.NewReader(audio)),
				gai.DataPart("video/quicktime", bytes.NewReader(video)),
			},
		}

		res, err := e.Embed(t.Context(), req)
		is.NotError(t, err)

		is.Equal(t, 768, len(res.Embedding))
	})
}
