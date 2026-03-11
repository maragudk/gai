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

		req := gai.EmbedRequest{
			Parts: []gai.MessagePart{gai.TextMessagePart("Embed this, please.")},
		}

		res, err := e.Embed(t.Context(), req)
		is.NotError(t, err)

		is.Equal(t, 768, len(res.Embedding))
	})

	t.Run("can embed an image", func(t *testing.T) {
		c := newClient(t)

		e := c.NewEmbedder(google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding2Preview,
			Dimensions: 768,
		})

		req := gai.EmbedRequest{
			Parts: []gai.MessagePart{gai.DataMessagePart("image/jpeg", bytes.NewReader(image))},
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
			Parts: []gai.MessagePart{gai.DataMessagePart("audio/mp4", bytes.NewReader(audio))},
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
			Parts: []gai.MessagePart{gai.DataMessagePart("video/quicktime", bytes.NewReader(video))},
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
			Parts: []gai.MessagePart{
				gai.TextMessagePart("A multimedia embedding test."),
				gai.DataMessagePart("image/jpeg", bytes.NewReader(image)),
				gai.DataMessagePart("audio/mp4", bytes.NewReader(audio)),
				gai.DataMessagePart("video/quicktime", bytes.NewReader(video)),
			},
		}

		res, err := e.Embed(t.Context(), req)
		is.NotError(t, err)

		is.Equal(t, 768, len(res.Embedding))
	})
}
