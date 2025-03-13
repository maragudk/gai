package gai_test

import (
	"testing"

	"maragu.dev/gai"
	"maragu.dev/is"
)

func TestNaiveWordTokenizer(t *testing.T) {
	t.Run("Tokenize", func(t *testing.T) {
		tests := []struct {
			name     string
			text     string
			expected []string
		}{
			{
				name:     "empty text",
				text:     "",
				expected: nil,
			},
			{
				name:     "simple words",
				text:     "hello world",
				expected: []string{"hello", "world"},
			},
			{
				name:     "with punctuation",
				text:     "Hello, world!",
				expected: []string{"Hello", ",", "world", "!"},
			},
			{
				name:     "mixed punctuation",
				text:     "The quick (brown) fox; jumps over the lazy dog.",
				expected: []string{"The", "quick", "(", "brown", ")", "fox", ";", "jumps", "over", "the", "lazy", "dog", "."},
			},
			{
				name:     "with quotes",
				text:     "She said, \"This is a test.\"",
				expected: []string{"She", "said", ",", "\"", "This", "is", "a", "test", ".", "\""},
			},
			{
				name:     "with newlines and tabs",
				text:     "Line one\nLine\ttwo",
				expected: []string{"Line", "one", "Line", "two"},
			},
			{
				name:     "numbers and special characters",
				text:     "The price is $5.99 for 3 items.",
				expected: []string{"The", "price", "is", "$5", ".", "99", "for", "3", "items", "."},
			},
			{
				name:     "nested brackets",
				text:     "The function f(x) = {sin(x), cos(x)}",
				expected: []string{"The", "function", "f", "(", "x", ")", "=", "{", "sin", "(", "x", ")", ",", "cos", "(", "x", ")", "}"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tokenizer := &gai.NaiveWordTokenizer{}
				got := tokenizer.Tokenize(tt.text)
				
				// Compare length first
				is.Equal(t, len(tt.expected), len(got))
				
				// Then compare each element
				for i, token := range got {
					is.Equal(t, tt.expected[i], token)
				}
			})
		}
	})
}