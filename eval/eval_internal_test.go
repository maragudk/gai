package eval

import (
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

	t.Run("panics on data parts", func(t *testing.T) {
		defer func() {
			r := recover()
			is.Equal(t, "sampleText: all parts must be text, got data", r)
		}()

		sampleText([]gai.Part{
			gai.DataPart("image/jpeg", []byte("not real image data")),
		})
	})

	t.Run("panics on mixed parts", func(t *testing.T) {
		defer func() {
			r := recover()
			is.Equal(t, "sampleText: all parts must be text, got data", r)
		}()

		sampleText([]gai.Part{
			gai.TextPart("before"),
			gai.DataPart("image/jpeg", []byte("not real image data")),
		})
	})
}
