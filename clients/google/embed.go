package google

import (
	"context"
	"fmt"
	"io"
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
	EmbedModelGeminiEmbedding001      = EmbedModel("models/gemini-embedding-001")
	EmbedModelGeminiEmbedding2Preview = EmbedModel("models/gemini-embedding-2-preview")
)

// Embedder satisfies [gai.Embedder] for Google Gemini models.
type Embedder struct {
	Client     *genai.Client
	dimensions int
	log        *slog.Logger
	model      EmbedModel
	tracer     trace.Tracer
}

// NewEmbedderOptions for [Client.NewEmbedder].
type NewEmbedderOptions struct {
	Dimensions int
	Model      EmbedModel
}

// NewEmbedder creates a new [Embedder].
func (c *Client) NewEmbedder(opts NewEmbedderOptions) *Embedder {
	if opts.Dimensions <= 0 {
		panic("dimensions must be greater than 0")
	}

	if opts.Dimensions > 3072 {
		panic("dimensions must be less than or equal to 3072")
	}

	return &Embedder{
		Client:     c.Client,
		dimensions: opts.Dimensions,
		log:        c.log,
		model:      opts.Model,
		tracer:     otel.Tracer("maragu.dev/gai/clients/google"),
	}
}

// Embed satisfies [gai.Embedder].
func (e *Embedder) Embed(ctx context.Context, req gai.EmbedRequest) (gai.EmbedResponse[float32], error) {
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
			content.Parts = append(content.Parts, &genai.Part{Text: part.Text()})
		case gai.PartTypeData:
			data, err := io.ReadAll(part.Data)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "data read failed")
				return gai.EmbedResponse[float32]{}, fmt.Errorf("error reading request data: %w", err)
			}
			content.Parts = append(content.Parts, &genai.Part{
				InlineData: &genai.Blob{
					MIMEType: part.MIMEType,
					Data:     data,
				},
			})
		default:
			panic("unsupported part type for embedding: " + part.Type)
		}
	}

	dims := int32(e.dimensions)
	res, err := e.Client.Models.EmbedContent(ctx, string(e.model), []*genai.Content{&content}, &genai.EmbedContentConfig{
		OutputDimensionality: &dims,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "embedding request failed")
		return gai.EmbedResponse[float32]{}, errors.Wrap(err, "error embedding")
	}
	if len(res.Embeddings) == 0 {
		err := errors.New("no embeddings returned")
		span.RecordError(err)
		span.SetStatus(codes.Error, "no embeddings in response")
		return gai.EmbedResponse[float32]{}, err
	}

	return gai.EmbedResponse[float32]{
		Embedding: res.Embeddings[0].Values,
	}, nil
}

var _ gai.Embedder[float32] = (*Embedder)(nil)
