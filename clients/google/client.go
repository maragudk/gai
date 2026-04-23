// Package google provides [gai.ChatCompleter] and [gai.Embedder] implementations
// backed by Google Gemini (via the Gemini API or Vertex AI). Construct a [Client]
// with [NewClient], then derive a chat completer or embedder via
// [Client.NewChatCompleter] or [Client.NewEmbedder].
package google

import (
	"context"
	"log/slog"

	"google.golang.org/genai"
)

// Backend is the Google AI backend to use.
type Backend string

const (
	// BackendGemini is the Google Gemini API backend.
	BackendGemini Backend = "gemini"
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
