package llm

import (
	"io"
	"log/slog"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type OpenAIClient struct {
	Client *openai.Client
	log    *slog.Logger
}

type NewOpenAIClientOptions struct {
	BaseURL string
	Log     *slog.Logger
	Token   string
}

func NewOpenAIClient(opts NewOpenAIClientOptions) *OpenAIClient {
	if opts.Log == nil {
		opts.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	var clientOpts []option.RequestOption

	if opts.BaseURL != "" {
		if !strings.HasSuffix(opts.BaseURL, "/") {
			opts.BaseURL += "/"
		}
		clientOpts = append(clientOpts, option.WithBaseURL(opts.BaseURL))
	}

	if opts.Token != "" {
		clientOpts = append(clientOpts, option.WithAPIKey(opts.Token))
	}

	return &OpenAIClient{
		Client: openai.NewClient(clientOpts...),
		log:    opts.Log,
	}
}
