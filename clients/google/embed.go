package google

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/genai"
	"maragu.dev/errors"

	"maragu.dev/gai"
)

// EmbedModel for use with [Embedder].
type EmbedModel string

const (
	EmbedModelGeminiEmbedding001      = EmbedModel("gemini-embedding-001")
	EmbedModelGeminiEmbedding2Preview = EmbedModel("gemini-embedding-2-preview")
)

// Embedder satisfies [gai.Embedder] for Google Gemini embedding models.
// T picks the vector component type; typical choices are float32 or float64.
type Embedder[T gai.VectorComponent] struct {
	Client     *genai.Client
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
// does not permit type parameters on methods. Example: google.NewEmbedder[float32](c, opts).
func NewEmbedder[T gai.VectorComponent](c *Client, opts NewEmbedderOptions) *Embedder[T] {
	if opts.Dimensions <= 0 {
		panic("dimensions must be greater than 0")
	}

	if opts.Dimensions > 3072 {
		panic("dimensions must be less than or equal to 3072")
	}

	return &Embedder[T]{
		Client:     c.Client,
		dimensions: opts.Dimensions,
		log:        c.log,
		model:      opts.Model,
		tracer:     otel.Tracer("maragu.dev/gai/clients/google"),
	}
}

// Embed satisfies [gai.Embedder].
func (e *Embedder[T]) Embed(ctx context.Context, req gai.EmbedRequest) (gai.EmbedResponse[T], error) {
	ctx, span := e.tracer.Start(ctx, "google.embed",
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

	var content genai.Content
	for _, part := range req.Parts {
		switch part.Type {
		case gai.PartTypeText:
			text := part.Text()
			span.SetAttributes(attribute.Int("ai.input_length", len(text)))
			content.Parts = append(content.Parts, &genai.Part{Text: text})
		case gai.PartTypeData:
			content.Parts = append(content.Parts, &genai.Part{
				InlineData: &genai.Blob{
					MIMEType: part.MIMEType,
					Data:     part.Data,
				},
			})
		default:
			panic("unsupported part type for embedding: " + string(part.Type))
		}
	}

	dims := int32(e.dimensions)
	res, err := e.Client.Models.EmbedContent(ctx, string(e.model), []*genai.Content{&content}, &genai.EmbedContentConfig{
		OutputDimensionality: &dims,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "embedding request failed")
		return gai.EmbedResponse[T]{}, errors.Wrap(err, "error embedding")
	}
	if len(res.Embeddings) == 0 {
		err := errors.New("no embeddings returned")
		span.RecordError(err)
		span.SetStatus(codes.Error, "no embeddings in response")
		return gai.EmbedResponse[T]{}, err
	}

	values := res.Embeddings[0].Values
	embedding := make([]T, len(values))
	for i, c := range values {
		embedding[i] = T(c)
	}
	return gai.EmbedResponse[T]{
		Embedding: embedding,
	}, nil
}
