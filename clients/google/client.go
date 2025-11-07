package google

import (
	"context"
	"log/slog"

	"google.golang.org/genai"
)

type Client struct {
	Client *genai.Client
	log    *slog.Logger
}

type NewClientOptions struct {
	Key string
	Log *slog.Logger
}

func NewClient(opts NewClientOptions) *Client {
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  opts.Key,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		panic(err)
	}

	return &Client{
		Client: client,
		log:    opts.Log,
	}
}
