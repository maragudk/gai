// Package google provides [gai.ChatCompleter] and [gai.Embedder] implementations
// backed by Google Gemini (via the Gemini API or Vertex AI). Construct a [Client]
// with [NewClient], then derive a chat completer or embedder via
// [Client.NewChatCompleter] or [Client.NewEmbedder].
package google

import (
	"context"
	"log/slog"

	"cloud.google.com/go/auth/credentials"
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
	// CredentialsPath is the path to a service account JSON key file used for
	// [BackendVertexAI] when Project is set. If empty, credentials are resolved
	// via Application Default Credentials (e.g. GOOGLE_APPLICATION_CREDENTIALS,
	// gcloud, or attached service accounts on GCP infrastructure).
	CredentialsPath string
	// Key is the API key. For [BackendVertexAI] it is ignored when Project is set,
	// because Vertex AI API keys pin requests to a fixed regional endpoint and
	// cannot reach multi-region-only models such as [EmbedModelGeminiEmbedding2].
	Key string
	// Location is the Vertex AI location (e.g. "global", "us", "eu", "us-central1").
	// Used only when Project is set; ignored on the API-key path because Vertex AI
	// API keys carry their own routing.
	Location string
	Log      *slog.Logger
	// Project is the Google Cloud project ID. When set on [BackendVertexAI], the
	// client authenticates via a service account (CredentialsPath, or Application
	// Default Credentials if CredentialsPath is empty) instead of Key, and Location
	// is required.
	Project string
}

func NewClient(opts NewClientOptions) *Client {
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}

	cfg := &genai.ClientConfig{}
	switch opts.Backend {
	case BackendVertexAI:
		cfg.Backend = genai.BackendVertexAI
		if opts.Project != "" {
			cfg.Project = opts.Project
			cfg.Location = opts.Location
			if opts.CredentialsPath != "" {
				creds, err := credentials.DetectDefault(&credentials.DetectOptions{
					CredentialsFile: opts.CredentialsPath,
					Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform"},
				})
				if err != nil {
					panic(err)
				}
				cfg.Credentials = creds
			}
		} else {
			cfg.APIKey = opts.Key
		}
	default:
		cfg.Backend = genai.BackendGeminiAPI
		cfg.APIKey = opts.Key
	}

	client, err := genai.NewClient(context.Background(), cfg)
	if err != nil {
		panic(err)
	}

	return &Client{
		Client: client,
		log:    opts.Log,
	}
}
