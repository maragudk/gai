// Package anthropic provides a [gai.ChatCompleter] implementation backed by
// the Anthropic Messages API. Construct a [Client] with [NewClient], then
// derive a chat completer via [Client.NewChatCompleter]. Anthropic does not
// expose embeddings, so this package has no Embedder.
package anthropic

import (
	"log/slog"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type Client struct {
	Client anthropic.Client
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

	return &Client{
		Client: anthropic.NewClient(option.WithAPIKey(opts.Key)),
		log:    opts.Log,
	}
}
