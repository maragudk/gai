package gai

import (
	"io"
	"log/slog"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type AnthropicClient struct {
	Client *anthropic.Client
	log    *slog.Logger
}

type NewAnthropicClientOptions struct {
	Key string
	Log *slog.Logger
}

func NewAnthropicClient(opts NewAnthropicClientOptions) *AnthropicClient {
	if opts.Log == nil {
		opts.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &AnthropicClient{
		Client: anthropic.NewClient(option.WithAPIKey(opts.Key)),
		log:    opts.Log,
	}
}
