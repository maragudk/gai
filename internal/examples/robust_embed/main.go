package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/openai"
	"maragu.dev/gai/robust"
)

func main() {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Embedder-typed fallback is limited: the generic T must match across the list.
	// OpenAI returns float64, Google returns float32, so they can't share one Embedder[T].
	// A single provider with retries is the realistic use case for now.
	embedder := openai.NewClient(openai.NewClientOptions{
		Key: os.Getenv("OPENAI_API_KEY"),
		Log: log,
	}).NewEmbedder(openai.NewEmbedderOptions{
		Model:      openai.EmbedModelTextEmbedding3Small,
		Dimensions: 1536,
	})

	e := robust.NewEmbedder[float64](robust.NewEmbedderOptions[float64]{
		Embedders:   []gai.Embedder[float64]{embedder},
		MaxAttempts: 3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		Log:         log,
	})

	res, err := e.Embed(ctx, gai.NewTextEmbedRequest("A one-line haiku about resilience."))
	if err != nil {
		log.Error("embedding", "error", err)
		return
	}

	fmt.Printf("got embedding of length %d, first components: %v\n", len(res.Embedding), res.Embedding[:min(5, len(res.Embedding))])
}
