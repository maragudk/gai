package eval

import (
	"strings"
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai"
)

func TestSampleText(t *testing.T) {
	t.Run("returns empty string for nil parts", func(t *testing.T) {
		is.Equal(t, "", sampleText(nil))
	})

	t.Run("returns empty string for empty parts", func(t *testing.T) {
		is.Equal(t, "", sampleText([]gai.Part{}))
	})

	t.Run("returns text from a single text part", func(t *testing.T) {
		parts := []gai.Part{gai.TextPart("hello")}
		is.Equal(t, "hello", sampleText(parts))
	})

	t.Run("concatenates multiple text parts", func(t *testing.T) {
		parts := []gai.Part{gai.TextPart("hello"), gai.TextPart(" world")}
		is.Equal(t, "hello world", sampleText(parts))
	})

	t.Run("skips data parts", func(t *testing.T) {
		parts := []gai.Part{
			gai.DataPart("image/jpeg", strings.NewReader("not real image data")),
		}
		is.Equal(t, "", sampleText(parts))
	})

	t.Run("extracts text from mixed parts", func(t *testing.T) {
		parts := []gai.Part{
			gai.TextPart("before"),
			gai.DataPart("image/jpeg", strings.NewReader("not real image data")),
			gai.TextPart(" after"),
		}
		is.Equal(t, "before after", sampleText(parts))
	})
}
