package openai

import (
	"context"
	"log/slog"

	"github.com/openai/openai-go/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"maragu.dev/errors"

	"maragu.dev/gai"
)

type EmbedModel string

const (
	EmbedModelTextEmbedding3Large = EmbedModel(openai.EmbeddingModelTextEmbedding3Large)
	EmbedModelTextEmbedding3Small = EmbedModel(openai.EmbeddingModelTextEmbedding3Small)
)

type Embedder struct {
	Client     openai.Client
	dimensions int
	log        *slog.Logger
	model      EmbedModel
	tracer     trace.Tracer
}

type NewEmbedderOptions struct {
	Dimensions int
	Model      EmbedModel
}

func (c *Client) NewEmbedder(opts NewEmbedderOptions) *Embedder {
	if opts.Dimensions <= 0 {
		panic("dimensions must be greater than 0")
	}

	switch opts.Model {
	case EmbedModelTextEmbedding3Large:
		if opts.Dimensions > 3072 {
			panic("dimensions must be less than or equal to 3072")
		}
	case EmbedModelTextEmbedding3Small:
		if opts.Dimensions > 1536 {
			panic("dimensions must be less than or equal to 1536")
		}
	}

	return &Embedder{
		Client:     c.Client,
		dimensions: opts.Dimensions,
		log:        c.log,
		model:      opts.Model,
		tracer:     otel.Tracer("maragu.dev/gai-openai"),
	}
}

// Embed satisfies [gai.Embedder].
func (e *Embedder) Embed(ctx context.Context, req gai.EmbedRequest) (gai.EmbedResponse[float64], error) {
	ctx, span := e.tracer.Start(ctx, "openai.embed",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("ai.model", string(e.model)),
			attribute.Int("ai.dimensions", e.dimensions),
		),
	)
	defer span.End()

	v := gai.ReadAllString(req.Input)
	span.SetAttributes(attribute.Int("ai.input_length", len(v)))

	res, err := e.Client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input:          openai.EmbeddingNewParamsInputUnion{OfString: openai.Opt(v)},
		Model:          openai.EmbeddingModel(e.model),
		EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat,
		Dimensions:     openai.Opt(int64(e.dimensions)),
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "embedding request failed")
		return gai.EmbedResponse[float64]{}, errors.Wrap(err, "error embedding")
	}
	if len(res.Data) == 0 {
		err := errors.New("no embeddings returned")
		span.RecordError(err)
		span.SetStatus(codes.Error, "no embeddings in response")
		return gai.EmbedResponse[float64]{}, err
	}

	// Record token usage if available
	if res.Usage.PromptTokens > 0 {
		span.SetAttributes(
			attribute.Int("ai.prompt_tokens", int(res.Usage.PromptTokens)),
			attribute.Int("ai.total_tokens", int(res.Usage.TotalTokens)),
		)
	}

	return gai.EmbedResponse[float64]{
		Embedding: res.Data[0].Embedding,
	}, nil
}

var _ gai.Embedder[float64] = (*Embedder)(nil)
