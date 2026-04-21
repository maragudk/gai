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

// Embedder satisfies [gai.Embedder] for OpenAI embedding models.
// T picks the vector component type; typical choices are float32 or float64.
type Embedder[T gai.VectorComponent] struct {
	Client     openai.Client
	dimensions int
	log        *slog.Logger
	model      EmbedModel
	tracer     trace.Tracer
}

// NewEmbedderOptions for [NewEmbedder].
type NewEmbedderOptions struct {
	Dimensions int
	Model      EmbedModel
}

// NewEmbedder returns a new [Embedder] with the given vector component type T.
// This is a package-level function rather than a method on [Client] because Go
// does not permit type parameters on methods. Example: openai.NewEmbedder[float32](c, opts).
func NewEmbedder[T gai.VectorComponent](c *Client, opts NewEmbedderOptions) *Embedder[T] {
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

	return &Embedder[T]{
		Client:     c.Client,
		dimensions: opts.Dimensions,
		log:        c.log,
		model:      opts.Model,
		tracer:     otel.Tracer("maragu.dev/gai/clients/openai"),
	}
}

// Embed satisfies [gai.Embedder].
func (e *Embedder[T]) Embed(ctx context.Context, req gai.EmbedRequest) (gai.EmbedResponse[T], error) {
	ctx, span := e.tracer.Start(ctx, "openai.embed",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("ai.model", string(e.model)),
			attribute.Int("ai.dimensions", e.dimensions),
		),
	)
	defer span.End()

	if len(req.Parts) == 0 {
		panic("no parts")
	}
	if len(req.Parts) != 1 || req.Parts[0].Type != gai.PartTypeText {
		panic("OpenAI embeddings only support a single text part")
	}

	v := req.Parts[0].Text()
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
		return gai.EmbedResponse[T]{}, errors.Wrap(err, "error embedding")
	}
	if len(res.Data) == 0 {
		err := errors.New("no embeddings returned")
		span.RecordError(err)
		span.SetStatus(codes.Error, "no embeddings in response")
		return gai.EmbedResponse[T]{}, err
	}

	// Record token usage if available
	if res.Usage.PromptTokens > 0 {
		span.SetAttributes(
			attribute.Int("ai.prompt_tokens", int(res.Usage.PromptTokens)),
			attribute.Int("ai.total_tokens", int(res.Usage.TotalTokens)),
		)
	}

	embedding := make([]T, len(res.Data[0].Embedding))
	for i, c := range res.Data[0].Embedding {
		embedding[i] = T(c)
	}
	return gai.EmbedResponse[T]{
		Embedding: embedding,
	}, nil
}
