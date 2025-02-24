package gai

import (
	"context"
	"io"
	"log/slog"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type GoogleClient struct {
	Client *genai.Client
	log    *slog.Logger
}

type NewGoogleClientOptions struct {
	Key string
	Log *slog.Logger
}

func NewGoogleClient(opts NewGoogleClientOptions) *GoogleClient {
	if opts.Log == nil {
		opts.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	client, err := genai.NewClient(context.Background(), option.WithAPIKey(opts.Key))
	if err != nil {
		panic(err)
	}

	return &GoogleClient{
		Client: client,
		log:    opts.Log,
	}
}
