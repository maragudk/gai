package google

import (
	"context"
	"log/slog"

	"google.golang.org/genai"
)

// Backend is the Google AI backend to use.
type Backend string

const (
	// BackendGeminiAPI is the Google Gemini API backend.
	BackendGeminiAPI Backend = "gemini"
	// BackendVertexAI is the Google Vertex AI backend.
	BackendVertexAI Backend = "vertexai"
)

type Client struct {
	Client *genai.Client
	log    *slog.Logger
}

type NewClientOptions struct {
	Backend Backend
	Key     string
	Log     *slog.Logger
}

func NewClient(opts NewClientOptions) *Client {
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}

	var backend genai.Backend
	switch opts.Backend {
	case BackendVertexAI:
		backend = genai.BackendVertexAI
	default:
		backend = genai.BackendGeminiAPI
	}

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  opts.Key,
		Backend: backend,
	})
	if err != nil {
		panic(err)
	}

	return &Client{
		Client: client,
		log:    opts.Log,
	}
}
