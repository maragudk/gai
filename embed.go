package gai

import (
	"context"
	"io"
)

// EmbedRequest for [Embedder].
type EmbedRequest struct {
	Input io.Reader
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

// ReadAllString is like [io.ReadAll], but returns a string, and panics on errors.
// Useful for situations where the read cannot error.
func ReadAllString(r io.Reader) string {
	d, err := io.ReadAll(r)
	if err != nil {
		panic(err)
	}
	return string(d)
}
