package gai

import "context"

// EmbedRequest for [Embedder].
type EmbedRequest struct {
	Parts []Part
}

// NewTextEmbedRequest is a convenience function to create an [EmbedRequest] with a single text part.
func NewTextEmbedRequest(text string) EmbedRequest {
	return EmbedRequest{
		Parts: []Part{TextPart(text)},
	}
}

// VectorComponent is a single component of a vector.
type VectorComponent interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~float32 | ~float64
}

// EmbedResponse for [Embedder].
type EmbedResponse[T VectorComponent] struct {
	Embedding []T
}

// Embedder is satisfied by models supporting embedding.
type Embedder[T VectorComponent] interface {
	Embed(ctx context.Context, p EmbedRequest) (EmbedResponse[T], error)
}
