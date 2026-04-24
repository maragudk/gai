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
	EmbedModelGeminiEmbedding001 = EmbedModel("gemini-embedding-001")
	EmbedModelGeminiEmbedding2   = EmbedModel("gemini-embedding-2")
)

// EmbedTaskAsymmetric is a task-type prefix for asymmetric retrieval tasks with the [EmbedModelGeminiEmbedding2] model, where queries and documents use different formats.
// Use [FormatEmbedTaskQuery] on the query side and [FormatEmbedTaskDocument] on the document side.
type EmbedTaskAsymmetric string

const (
	// EmbedTaskSearchResult is the retrieval task for general search, where queries are matched against documents.
	EmbedTaskSearchResult = EmbedTaskAsymmetric("search result")
	// EmbedTaskQuestionAnswering is the retrieval task for question-answering systems, where questions are matched against answer passages.
	EmbedTaskQuestionAnswering = EmbedTaskAsymmetric("question answering")
	// EmbedTaskFactChecking is the retrieval task for fact verification, where statements are matched against supporting or refuting evidence.
	EmbedTaskFactChecking = EmbedTaskAsymmetric("fact checking")
	// EmbedTaskCodeRetrieval is the retrieval task for matching natural-language queries against code blocks.
	EmbedTaskCodeRetrieval = EmbedTaskAsymmetric("code retrieval")
)

// EmbedTaskSymmetric is a task-type prefix for symmetric tasks with the [EmbedModelGeminiEmbedding2] model, where both sides use the same format.
// Use [FormatEmbedTask] on all inputs.
type EmbedTaskSymmetric string

const (
	// EmbedTaskClassification is the task for classifying texts according to preset labels.
	EmbedTaskClassification = EmbedTaskSymmetric("classification")
	// EmbedTaskClustering is the task for clustering texts based on their similarities.
	EmbedTaskClustering = EmbedTaskSymmetric("clustering")
	// EmbedTaskSentenceSimilarity is the task for assessing similarity between sentences.
	EmbedTaskSentenceSimilarity = EmbedTaskSymmetric("sentence similarity")
)

// FormatEmbedTask formats content with the given symmetric task-type prefix for the [EmbedModelGeminiEmbedding2] model, as "task: {task} | query: {content}".
func FormatEmbedTask(task EmbedTaskSymmetric, content string) string {
	return "task: " + string(task) + " | query: " + content
}

// FormatEmbedTaskQuery formats a query with the given asymmetric task-type prefix for the [EmbedModelGeminiEmbedding2] model, as "task: {task} | query: {query}".
func FormatEmbedTaskQuery(task EmbedTaskAsymmetric, query string) string {
	return "task: " + string(task) + " | query: " + query
}

// FormatEmbedTaskDocument formats a document with the given title for the [EmbedModelGeminiEmbedding2] model, as "title: {title} | text: {content}".
// Use this for the document side of asymmetric retrieval tasks. If title is empty, it is set to "none".
// The task is not included in the output but is accepted so the document helper mirrors the shape of [FormatEmbedTaskQuery].
func FormatEmbedTaskDocument(_ EmbedTaskAsymmetric, title, content string) string {
	if title == "" {
		title = "none"
	}
	return "title: " + title + " | text: " + content
}

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
