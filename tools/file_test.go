package tools_test

import (
	"embed"
	"encoding/json"
	"testing"

	"maragu.dev/gai/tools"
	"maragu.dev/is"
)

//go:embed testdata
var testdata embed.FS

func TestNewReadFile(t *testing.T) {
	t.Run("reads the contents of a file", func(t *testing.T) {
		tool := tools.NewReadFile(testdata)

		is.Equal(t, "read_file", tool.Name)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.ReadFileArgs{Path: "testdata/readme.txt"}))
		is.NotError(t, err)
		is.Equal(t, "Hi!\n", result)
	})

	t.Run("errors if file does not exist", func(t *testing.T) {
		tool := tools.NewReadFile(testdata)

		_, err := tool.Function(t.Context(), mustMarshalJSON(tools.ReadFileArgs{Path: "testdata/nonexistent.txt"}))
		is.Equal(t, "open testdata/nonexistent.txt: file does not exist", err.Error())
	})
}

func mustMarshalJSON(v any) json.RawMessage {
	d, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return d
}
