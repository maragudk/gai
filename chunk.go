package gai

import (
	"context"
	"strings"
)

// Chunker can split text into chunks according to a given strategy.
type Chunker interface {
	Chunk(ctx context.Context, text string) []string
}

// FixedSizeChunker chunks text into a fixed number of tokens with optional overlap (in a sliding window).
type FixedSizeChunker struct {
	tokenizer Tokenizer
	size      int
	overlap   float64
}

type NewFixedSizeChunkerOptions struct {
	Tokenizer Tokenizer
	Size      int     // Size of each chunk in tokens
	Overlap   float64 // Overlap in percentage (0.0 - 1.0)
}

func NewFixedSizeChunker(opts NewFixedSizeChunkerOptions) *FixedSizeChunker {
	return &FixedSizeChunker{
		tokenizer: opts.Tokenizer,
		size:      opts.Size,
		overlap:   opts.Overlap,
	}
}

// Chunk splits the given text into chunks of the configured size with the configured overlap.
// Each chunk contains at most f.size tokens. The last chunk may contain fewer tokens.
// If the overlap is > 0, each chunk (except the first) will start with some tokens from the previous chunk.
func (f *FixedSizeChunker) Chunk(ctx context.Context, text string) []string {
	if text == "" {
		return nil
	}

	tokens := f.tokenizer.Tokenize(text)
	if len(tokens) == 0 {
		return nil
	}

	if len(tokens) <= f.size {
		// For short text, tokenize and join to ensure consistent token handling
		return []string{joinTokens(tokens)}
	}

	// Calculate the step size based on the overlap percentage
	// Step = ChunkSize * (1 - Overlap)
	step := int(float64(f.size) * (1.0 - f.overlap))
	if step <= 0 {
		step = 1 // Ensure at least one new token per chunk
	}

	var chunks []string
	for startIdx := 0; startIdx < len(tokens); startIdx += step {
		// Calculate chunk end position
		endIdx := startIdx + f.size
		if endIdx > len(tokens) {
			endIdx = len(tokens)
		}

		// Extract the tokens for this chunk and join them
		chunkTokens := tokens[startIdx:endIdx]
		chunk := joinTokens(chunkTokens)
		chunks = append(chunks, chunk)

		// If we've reached the end of the tokens, we're done
		if endIdx >= len(tokens) {
			break
		}
	}

	return chunks
}

// joinTokens reconstructs text from tokens
// This is a simple implementation that just concatenates tokens with spaces
// A real implementation would depend on the tokenizer's behavior
func joinTokens(tokens []string) string {
	return strings.Join(tokens, " ")
}

var _ Chunker = (*FixedSizeChunker)(nil)
