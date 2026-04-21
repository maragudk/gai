package google_test

import (
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/google"
)

func TestEmbedder_Embed(t *testing.T) {
	t.Run("can embed a text as float32", func(t *testing.T) {
		c := newClient(t)

		e := google.NewEmbedder[float32](c, google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding001,
			Dimensions: 768,
		})

		res, err := e.Embed(t.Context(), gai.NewTextEmbedRequest("Embed this, please."))
		is.NotError(t, err)

		is.Equal(t, 768, len(res.Embedding))
	})

	t.Run("can embed a text as float64 for callers that want wider components", func(t *testing.T) {
		c := newClient(t)

		e := google.NewEmbedder[float64](c, google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding001,
			Dimensions: 768,
		})

		res, err := e.Embed(t.Context(), gai.NewTextEmbedRequest("Embed this, please."))
		is.NotError(t, err)

		is.Equal(t, 768, len(res.Embedding))
	})

	t.Run("panics with no parts", func(t *testing.T) {
		c := newClient(t)

		e := google.NewEmbedder[float32](c, google.NewEmbedderOptions{
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

		e := google.NewEmbedder[float32](c, google.NewEmbedderOptions{
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

		e := google.NewEmbedder[float32](c, google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding2Preview,
			Dimensions: 768,
		})

		req := gai.EmbedRequest{
			Parts: []gai.Part{gai.DataPart("image/jpeg", image)},
		}

		res, err := e.Embed(t.Context(), req)
		is.NotError(t, err)

		is.Equal(t, 768, len(res.Embedding))
	})

	t.Run("can embed audio", func(t *testing.T) {
		c := newClient(t)

		e := google.NewEmbedder[float32](c, google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding2Preview,
			Dimensions: 768,
		})

		req := gai.EmbedRequest{
			Parts: []gai.Part{gai.DataPart("audio/mp4", audio)},
		}

		res, err := e.Embed(t.Context(), req)
		is.NotError(t, err)

		is.Equal(t, 768, len(res.Embedding))
	})

	t.Run("can embed video", func(t *testing.T) {
		c := newClient(t)

		e := google.NewEmbedder[float32](c, google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding2Preview,
			Dimensions: 768,
		})

		req := gai.EmbedRequest{
			Parts: []gai.Part{gai.DataPart("video/quicktime", video)},
		}

		res, err := e.Embed(t.Context(), req)
		is.NotError(t, err)

		is.Equal(t, 768, len(res.Embedding))
	})

	t.Run("can embed a text with Vertex AI backend", func(t *testing.T) {
		c := newVertexAIClient(t)

		e := google.NewEmbedder[float32](c, google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding001,
			Dimensions: 768,
		})

		res, err := e.Embed(t.Context(), gai.NewTextEmbedRequest("Embed this, please."))
		is.NotError(t, err)

		is.Equal(t, 768, len(res.Embedding))
	})

	t.Run("can embed a mixture of text, image, audio, and video", func(t *testing.T) {
		c := newClient(t)

		e := google.NewEmbedder[float32](c, google.NewEmbedderOptions{
			Model:      google.EmbedModelGeminiEmbedding2Preview,
			Dimensions: 768,
		})

		req := gai.EmbedRequest{
			Parts: []gai.Part{
				gai.TextPart("A multimedia embedding test."),
				gai.DataPart("image/jpeg", image),
				gai.DataPart("audio/mp4", audio),
				gai.DataPart("video/quicktime", video),
			},
		}

		res, err := e.Embed(t.Context(), req)
		is.NotError(t, err)

		is.Equal(t, 768, len(res.Embedding))
	})
}
