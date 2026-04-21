// Example of using robust.Embedder with OpenAI (primary) + Google (fallback).
//
// Running this example hits the APIs with empty keys on purpose — it exercises the
// 401 → retry → fallback → exhaustion path end to end so it's a useful smoke test
// of the failover behavior without any setup.
//
// Both concrete embedders are generic over the vector component type, so picking the
// same T for both lets them share a [gai.Embedder[T]] list directly, no adapters.
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
	primary := openai.NewEmbedder[float32](openai.NewClient(openai.NewClientOptions{
		Log: log,
	}), openai.NewEmbedderOptions{
		Model:      openai.EmbedModelTextEmbedding3Small,
		Dimensions: 1536,
	})

	// Key is a placeholder on purpose — see top-of-file comment. Google's client panics
	// on an empty key at construction, so we pass a non-empty stub that will fail at
	// the API call.
	secondary := google.NewEmbedder[float32](google.NewClient(google.NewClientOptions{
		Key: "placeholder",
		Log: log,
	}), google.NewEmbedderOptions{
		Model:      google.EmbedModelGeminiEmbedding001,
		Dimensions: 1536,
	})

	e := robust.NewEmbedder[float32](robust.NewEmbedderOptions[float32]{
		Embedders:   []gai.Embedder[float32]{primary, secondary},
		MaxAttempts: 3,
		Log:         log,
	})

	if _, err := e.Embed(ctx, gai.NewTextEmbedRequest("A one-line haiku about resilience.")); err != nil {
		log.Error("embedding", "error", err)
		return
	}
}
