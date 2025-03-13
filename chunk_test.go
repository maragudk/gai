package gai_test

import (
	"context"
	"testing"

	"maragu.dev/gai"
	"maragu.dev/is"
)

func TestFixedSizeChunker(t *testing.T) {
	t.Run("Chunk", func(t *testing.T) {
		tests := []struct {
			name     string
			size     int
			overlap  float64
			text     string
			expected []string
		}{
			{
				name:     "empty text",
				size:     5,
				overlap:  0,
				text:     "",
				expected: nil,
			},
			{
				name:     "text shorter than chunk size",
				size:     10,
				overlap:  0,
				text:     "This is a short text.",
				expected: []string{"This is a short text ."},
			},
			{
				name:    "simple chunks no overlap",
				size:    5,
				overlap: 0,
				text:    "The quick brown fox jumps over the lazy dog. The fox was very quick indeed.",
				expected: []string{
					"The quick brown fox jumps",
					"over the lazy dog .",
					"The fox was very quick",
					"indeed .",
				},
			},
			{
				name:    "chunks with 0.2 overlap",
				size:    5,
				overlap: 0.2,
				text:    "The quick brown fox jumps over the lazy dog. The fox was very quick indeed.",
				expected: []string{
					"The quick brown fox jumps",
					"jumps over the lazy dog",
					"dog . The fox was",
					"was very quick indeed .",
				},
			},
			{
				name:    "chunks with 0.5 overlap",
				size:    6,
				overlap: 0.5,
				text:    "Machine learning models can process natural language to perform various tasks such as translation sentiment analysis and text generation.",
				expected: []string{
					"Machine learning models can process natural",
					"can process natural language to perform",
					"language to perform various tasks such",
					"various tasks such as translation sentiment",
					"as translation sentiment analysis and text",
					"analysis and text generation .",
				},
			},
			{
				name:    "longer text realistic case",
				size:    10,
				overlap: 0.3,
				text:    "Effective chunking strategies are crucial for processing large documents in natural language processing applications. When working with language models that have token limits, proper text segmentation ensures that context is preserved across segments. Overlapping chunks can help maintain coherence between segments, avoiding information loss at chunk boundaries. The ideal chunk size and overlap ratio depend on the specific use case and the characteristics of the text being processed.",
				expected: []string{
					"Effective chunking strategies are crucial for processing large documents in",
					"large documents in natural language processing applications . When working",
					". When working with language models that have token limits",
					"have token limits , proper text segmentation ensures that context",
					"ensures that context is preserved across segments . Overlapping chunks",
					". Overlapping chunks can help maintain coherence between segments ,",
					"between segments , avoiding information loss at chunk boundaries .",
					"chunk boundaries . The ideal chunk size and overlap ratio",
					"and overlap ratio depend on the specific use case and",
					"use case and the characteristics of the text being processed",
					"text being processed .",
				},
			},
			{
				name:    "high overlap (0.9)",
				size:    5,
				overlap: 0.9,
				text:    "One two three four five six seven eight nine ten eleven twelve.",
				expected: []string{
					"One two three four five",
					"two three four five six",
					"three four five six seven",
					"four five six seven eight",
					"five six seven eight nine",
					"six seven eight nine ten",
					"seven eight nine ten eleven",
					"eight nine ten eleven twelve",
					"nine ten eleven twelve .",
				},
			},
			{
				name:    "full overlap (1.0)",
				size:    3,
				overlap: 1.0,
				text:    "Alpha beta gamma delta epsilon zeta eta.",
				expected: []string{
					"Alpha beta gamma",
					"beta gamma delta",
					"gamma delta epsilon",
					"delta epsilon zeta",
					"epsilon zeta eta",
					"zeta eta .",
				},
			},
		}

		// Run tests
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Create a new chunker with the test parameters
				chunker := gai.NewFixedSizeChunker(gai.NewFixedSizeChunkerOptions{
					Tokenizer: &gai.NaiveWordTokenizer{},
					Size:      tt.size,
					Overlap:   tt.overlap,
				})

				// Get chunks
				got := chunker.Chunk(context.Background(), tt.text)

				// Verify results
				t.Logf("Got %d chunks:", len(got))
				for i, chunk := range got {
					t.Logf("  %d: %q", i, chunk)
				}

				t.Logf("Expected %d chunks:", len(tt.expected))
				for i, chunk := range tt.expected {
					t.Logf("  %d: %q", i, chunk)
				}

				is.Equal(t, len(tt.expected), len(got))
				for i, chunk := range got {
					if i < len(tt.expected) {
						is.Equal(t, tt.expected[i], chunk)
					}
				}
			})
		}
	})
}
