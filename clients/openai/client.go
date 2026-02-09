package openai

import (
	"log/slog"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type Client struct {
	Client openai.Client
	log    *slog.Logger
}

type NewClientOptions struct {
	BaseURL string
	Key     string
	Log     *slog.Logger
}

func NewClient(opts NewClientOptions) *Client {
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}

	var clientOpts []option.RequestOption

	if opts.BaseURL != "" {
		if !strings.HasSuffix(opts.BaseURL, "/") {
			opts.BaseURL += "/"
		}
		clientOpts = append(clientOpts, option.WithBaseURL(opts.BaseURL))
	}

	if opts.Key != "" {
		clientOpts = append(clientOpts, option.WithAPIKey(opts.Key))
	}

	return &Client{
		Client: openai.NewClient(clientOpts...),
		log:    opts.Log,
	}
}
