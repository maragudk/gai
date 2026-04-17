// Example of using robust.Embedder with OpenAI (primary) + Google (fallback).
//
// Running this example hits the APIs with empty keys on purpose — it exercises the
// 401 → retry → fallback → exhaustion path end to end so it's a useful smoke test
// of the failover behavior without any setup.
//
// Google's embedder returns float32 and OpenAI returns float64, so the two clients
// can't share a [gai.Embedder[T]] list directly. The googleFloat64 adapter below
// shows how to bridge the gap by converting float32 components to float64.
package main

import (
	"context"
	"log/slog"
	"os"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/google"
	"maragu.dev/gai/clients/openai"
	"maragu.dev/gai/robust"
)

func main() {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Key intentionally empty — see top-of-file comment.
	primary := openai.NewClient(openai.NewClientOptions{
		Log: log,
	}).NewEmbedder(openai.NewEmbedderOptions{
		Model:      openai.EmbedModelTextEmbedding3Small,
		Dimensions: 1536,
	})

	// Key is a placeholder on purpose — see top-of-file comment. Google's client panics
	// on an empty key at construction, so we pass a non-empty stub that will fail at
	// the API call.
	secondary := google.NewClient(google.NewClientOptions{
		Key: "placeholder",
		Log: log,
	}).NewEmbedder(google.NewEmbedderOptions{
		Model:      google.EmbedModelGeminiEmbedding001,
		Dimensions: 1536,
	})

	e := robust.NewEmbedder[float64](robust.NewEmbedderOptions[float64]{
		Embedders:   []gai.Embedder[float64]{primary, googleFloat64{secondary}},
		MaxAttempts: 3,
		Log:         log,
	})

	if _, err := e.Embed(ctx, gai.NewTextEmbedRequest("A one-line haiku about resilience.")); err != nil {
		log.Error("embedding", "error", err)
		return
	}
}

// googleFloat64 adapts a gai.Embedder[float32] to a gai.Embedder[float64] by converting
// each component. Use when you need cross-provider fallback and the providers disagree
// on vector component type.
type googleFloat64 struct {
	inner gai.Embedder[float32]
}

func (g googleFloat64) Embed(ctx context.Context, req gai.EmbedRequest) (gai.EmbedResponse[float64], error) {
	res, err := g.inner.Embed(ctx, req)
	if err != nil {
		return gai.EmbedResponse[float64]{}, err
	}
	embedding := make([]float64, len(res.Embedding))
	for i, v := range res.Embedding {
		embedding[i] = float64(v)
	}
	return gai.EmbedResponse[float64]{Embedding: embedding}, nil
}
