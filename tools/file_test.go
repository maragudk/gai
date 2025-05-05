package tools_test

import (
	"embed"
	"encoding/json"
	"slices"
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

func TestNewListDir(t *testing.T) {
	t.Run("lists files in a directory", func(t *testing.T) {
		tool := tools.NewListDir(testdata)

		is.Equal(t, "list_dir", tool.Name)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.ListDirArgs{Path: "testdata"}))
		is.NotError(t, err)

		var files []string
		err = json.Unmarshal([]byte(result), &files)
		is.NotError(t, err)

		is.Equal(t, 3, len(files))
		is.Equal(t, "dir1/", files[0])
		is.Equal(t, "dir1/hello.txt", files[1])
		is.Equal(t, "readme.txt", files[2])
	})

	t.Run("uses current directory if no path provided", func(t *testing.T) {
		tool := tools.NewListDir(testdata)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.ListDirArgs{}))
		is.NotError(t, err)

		var files []string
		err = json.Unmarshal([]byte(result), &files)
		is.NotError(t, err)

		// Check testdata directory is included
		found := slices.Contains(files, "testdata/")
		is.True(t, found, "Expected files to include testdata/ directory")
	})

	t.Run("errors if directory does not exist", func(t *testing.T) {
		tool := tools.NewListDir(testdata)

		_, err := tool.Function(t.Context(), mustMarshalJSON(tools.ListDirArgs{Path: "nonexistent"}))

		is.Equal(t, "open nonexistent: file does not exist", err.Error())
	})
}

func mustMarshalJSON(v any) json.RawMessage {
	d, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return d
}
