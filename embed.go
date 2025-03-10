package gai

import (
	"context"
	"io"
)

// VectorComponent is a single component of a vector.
type VectorComponent interface {
	~int | ~float32 | ~float64
}

type Embedder[T VectorComponent] interface {
	Embed(ctx context.Context, r io.Reader) (EmbedResponse[T], error)
}

type EmbedResponse[T VectorComponent] struct {
	Embedding []T
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
