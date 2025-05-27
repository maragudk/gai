package tools_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai/tools"
)

func TestNewReadFile(t *testing.T) {
	testdata, err := os.OpenRoot("testdata")
	is.NotError(t, err)

	t.Run("reads the contents of a file", func(t *testing.T) {
		tool := tools.NewReadFile(testdata)

		is.Equal(t, "read_file", tool.Name)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.ReadFileArgs{Path: "readme.txt"}))
		is.NotError(t, err)
		is.Equal(t, "Hi!\n", result)
	})

	t.Run("errors if file does not exist", func(t *testing.T) {
		tool := tools.NewReadFile(testdata)

		_, err := tool.Function(t.Context(), mustMarshalJSON(tools.ReadFileArgs{Path: "nonexistent.txt"}))
		is.Equal(t, "openat nonexistent.txt: no such file or directory", err.Error())
	})
}

func TestNewListDir(t *testing.T) {
	testdata, err := os.OpenRoot("testdata")
	is.NotError(t, err)

	t.Run("lists files in a directory", func(t *testing.T) {
		tool := tools.NewListDir(testdata)

		is.Equal(t, "list_dir", tool.Name)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.ListDirArgs{Path: "."}))
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

		is.True(t, slices.Contains(files, "dir1/"))
		is.True(t, slices.Contains(files, "readme.txt"))
		is.True(t, slices.Contains(files, "dir1/hello.txt"))
	})

	t.Run("errors if directory does not exist", func(t *testing.T) {
		tool := tools.NewListDir(testdata)

		_, err := tool.Function(t.Context(), mustMarshalJSON(tools.ListDirArgs{Path: "nonexistent"}))

		is.Equal(t, "statat nonexistent: no such file or directory", err.Error())
	})
}

func TestNewEditFile(t *testing.T) {
	t.Run("edits the contents of a file", func(t *testing.T) {
		tempDir := t.TempDir()
		root, err := os.OpenRoot(tempDir)
		is.NotError(t, err)

		tool := tools.NewEditFile(root)

		is.Equal(t, "edit_file", tool.Name)

		// First, ensure the file exists with known content
		err = os.WriteFile(filepath.Join(tempDir, "edit_test.txt"), []byte("Original text"), 0644)
		is.NotError(t, err)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.EditFileArgs{
			Path:       "edit_test.txt",
			SearchStr:  "Original",
			ReplaceStr: "Modified",
		}))
		is.NotError(t, err)
		is.Equal(t, "Edited file at edit_test.txt", result)

		// Verify the file was changed
		content, err := os.ReadFile(filepath.Join(tempDir, "edit_test.txt"))
		is.NotError(t, err)
		is.Equal(t, "Modified text", string(content))
	})

	t.Run("creates a new file if it doesn't exist", func(t *testing.T) {
		tempDir := t.TempDir()
		root, err := os.OpenRoot(tempDir)
		is.NotError(t, err)

		tool := tools.NewEditFile(root)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.EditFileArgs{
			Path:       "new_file.txt",
			SearchStr:  "",
			ReplaceStr: "New content",
		}))
		is.NotError(t, err)
		is.Equal(t, "Created new file at new_file.txt", result)

		// Verify the file was created with the correct content
		content, err := os.ReadFile(filepath.Join(tempDir, "new_file.txt"))
		is.NotError(t, err)
		is.Equal(t, "New content", string(content))
	})

	t.Run("creates a new file in subdirectories", func(t *testing.T) {
		tempDir := t.TempDir()
		root, err := os.OpenRoot(tempDir)
		is.NotError(t, err)

		tool := tools.NewEditFile(root)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.EditFileArgs{
			Path:       "dir/subdir/nested_file.txt",
			SearchStr:  "",
			ReplaceStr: "Content in subdirectory",
		}))
		is.NotError(t, err)
		is.Equal(t, "Created new file at dir/subdir/nested_file.txt", result)

		// Verify both directories were created
		dirInfo, err := os.Stat(filepath.Join(tempDir, "dir"))
		is.NotError(t, err)
		is.True(t, dirInfo.IsDir())

		subdirInfo, err := os.Stat(filepath.Join(tempDir, "dir", "subdir"))
		is.NotError(t, err)
		is.True(t, subdirInfo.IsDir())

		// Verify the file was created with the correct content
		content, err := os.ReadFile(filepath.Join(tempDir, "dir/subdir/nested_file.txt"))
		is.NotError(t, err)
		is.Equal(t, "Content in subdirectory", string(content))
	})

	t.Run("errors if search_str and replace_str are the same", func(t *testing.T) {
		tempDir := t.TempDir()
		root, err := os.OpenRoot(tempDir)
		is.NotError(t, err)

		tool := tools.NewEditFile(root)

		_, err = tool.Function(t.Context(), mustMarshalJSON(tools.EditFileArgs{
			Path:       "some_file.txt",
			SearchStr:  "same",
			ReplaceStr: "same",
		}))
		is.Equal(t, "search_str and replace_str cannot be the same", err.Error())
	})

	t.Run("errors if search_str not found in file", func(t *testing.T) {
		tempDir := t.TempDir()
		root, err := os.OpenRoot(tempDir)
		is.NotError(t, err)

		tool := tools.NewEditFile(root)

		// First, ensure the file exists with known content
		err = os.WriteFile(filepath.Join(tempDir, "edit_test.txt"), []byte("Existing content"), 0644)
		is.NotError(t, err)

		_, err = tool.Function(t.Context(), mustMarshalJSON(tools.EditFileArgs{
			Path:       "edit_test.txt",
			SearchStr:  "NotInFile",
			ReplaceStr: "ShouldNotReplace",
		}))
		is.Equal(t, "search_str not found in file", err.Error())
	})

	t.Run("summarize read_file", func(t *testing.T) {
		testdata, err := os.OpenRoot("testdata")
		is.NotError(t, err)

		tool := tools.NewReadFile(testdata)

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.ReadFileArgs{
			Path: "readme.txt",
		}))

		is.NotError(t, err)
		is.Equal(t, `path="readme.txt"`, summary)
	})

	t.Run("summarize read_file with invalid JSON", func(t *testing.T) {
		testdata, err := os.OpenRoot("testdata")
		is.NotError(t, err)

		tool := tools.NewReadFile(testdata)

		summary, err := tool.Summarize(t.Context(), []byte(`{invalid json`))

		is.NotError(t, err)
		is.Equal(t, "error parsing arguments", summary)
	})

	t.Run("summarize list_dir with path", func(t *testing.T) {
		testdata, err := os.OpenRoot("testdata")
		is.NotError(t, err)

		tool := tools.NewListDir(testdata)

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.ListDirArgs{
			Path: "dir1",
		}))

		is.NotError(t, err)
		is.Equal(t, `path="dir1"`, summary)
	})

	t.Run("summarize list_dir with current directory", func(t *testing.T) {
		testdata, err := os.OpenRoot("testdata")
		is.NotError(t, err)

		tool := tools.NewListDir(testdata)

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.ListDirArgs{
			Path: ".",
		}))

		is.NotError(t, err)
		is.Equal(t, "", summary)
	})

	t.Run("summarize list_dir with empty path", func(t *testing.T) {
		testdata, err := os.OpenRoot("testdata")
		is.NotError(t, err)

		tool := tools.NewListDir(testdata)

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.ListDirArgs{}))

		is.NotError(t, err)
		is.Equal(t, "", summary)
	})

	t.Run("summarize edit_file with short strings", func(t *testing.T) {
		tempDir := t.TempDir()
		root, err := os.OpenRoot(tempDir)
		is.NotError(t, err)

		tool := tools.NewEditFile(root)

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.EditFileArgs{
			Path:       "test.txt",
			SearchStr:  "short",
			ReplaceStr: "brief",
		}))

		is.NotError(t, err)
		is.Equal(t, `path="test.txt" search="short" replace="brief"`, summary)
	})

	t.Run("summarize edit_file with long strings", func(t *testing.T) {
		tempDir := t.TempDir()
		root, err := os.OpenRoot(tempDir)
		is.NotError(t, err)

		tool := tools.NewEditFile(root)

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.EditFileArgs{
			Path:       "test.txt",
			SearchStr:  "This is a very long search string that should be truncated",
			ReplaceStr: "This is a very long replacement string that should also be truncated",
		}))

		is.NotError(t, err)
		is.Equal(t, `path="test.txt" search="This is a very long ..." replace="This is a very long ..."`, summary)
	})

	t.Run("summarize edit_file with invalid JSON", func(t *testing.T) {
		tempDir := t.TempDir()
		root, err := os.OpenRoot(tempDir)
		is.NotError(t, err)

		tool := tools.NewEditFile(root)

		summary, err := tool.Summarize(t.Context(), []byte(`{invalid json`))

		is.NotError(t, err)
		is.Equal(t, "error parsing arguments", summary)
	})
}

func mustMarshalJSON(v any) json.RawMessage {
	d, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return d
}
