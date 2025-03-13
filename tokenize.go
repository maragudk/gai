package gai

import "strings"

// Tokenizer converts a string into a list of tokens.
type Tokenizer interface {
	Tokenize(text string) []string
}

// NaiveWordTokenizer is a simple tokenizer that splits the text into words.
// It uses a space-based split and preserves punctuation as separate tokens.
type NaiveWordTokenizer struct{}

// Tokenize splits the input text into words and punctuation.
// It handles basic cases without considering all edge cases of natural language.
// The algorithm:
// 1. Replaces common punctuation with spaces around them to ensure they become separate tokens
// 2. Splits on whitespace
// 3. Removes empty tokens
func (t *NaiveWordTokenizer) Tokenize(text string) []string {
	if text == "" {
		return nil
	}

	// Replace common punctuation with spaced versions to preserve them as separate tokens
	replacer := strings.NewReplacer(
		".", " . ",
		",", " , ",
		";", " ; ",
		":", " : ",
		"!", " ! ",
		"?", " ? ",
		"(", " ( ",
		")", " ) ",
		"[", " [ ",
		"]", " ] ",
		"{", " { ",
		"}", " } ",
		`"`, ` " `,
		"'", " ' ",
	)

	// Apply replacements and split on whitespace
	processed := replacer.Replace(text)
	tokens := strings.Fields(processed)

	return tokens
}

var _ Tokenizer = (*NaiveWordTokenizer)(nil)
