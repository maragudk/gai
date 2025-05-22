package tools_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"maragu.dev/gai/tools"
	"maragu.dev/is"
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
	
	t.Run("summarize returns a human-readable description", func(t *testing.T) {
		tool := tools.NewReadFile(testdata)

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.ReadFileArgs{Path: "readme.txt"}))
		is.NotError(t, err)
		is.Equal(t, "Reading file: readme.txt", summary)
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
	
	t.Run("summarize returns human-readable description with specific path", func(t *testing.T) {
		tool := tools.NewListDir(testdata)

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.ListDirArgs{Path: "dir1"}))
		is.NotError(t, err)
		is.Equal(t, "Listing files in: dir1", summary)
	})
	
	t.Run("summarize handles empty path correctly", func(t *testing.T) {
		tool := tools.NewListDir(testdata)

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.ListDirArgs{}))
		is.NotError(t, err)
		is.Equal(t, "Listing files in: current directory", summary)
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

	t.Run("summarize returns a human-readable description for creating a new file", func(t *testing.T) {
		tempDir := t.TempDir()
		root, err := os.OpenRoot(tempDir)
		is.NotError(t, err)

		tool := tools.NewEditFile(root)

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.EditFileArgs{
			Path:       "new_file.txt",
			SearchStr:  "",
			ReplaceStr: "New content",
		}))
		is.NotError(t, err)
		is.Equal(t, "Creating new file at new_file.txt", summary)
	})

	t.Run("summarize returns a human-readable description for editing a file", func(t *testing.T) {
		tempDir := t.TempDir()
		root, err := os.OpenRoot(tempDir)
		is.NotError(t, err)

		tool := tools.NewEditFile(root)

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.EditFileArgs{
			Path:       "edit_test.txt",
			SearchStr:  "Original",
			ReplaceStr: "Modified",
		}))
		is.NotError(t, err)
		is.Equal(t, "Editing file edit_test.txt: Replacing \"Original\" with \"Modified\"", summary)
	})

	t.Run("summarize truncates long search and replace strings", func(t *testing.T) {
		tempDir := t.TempDir()
		root, err := os.OpenRoot(tempDir)
		is.NotError(t, err)

		tool := tools.NewEditFile(root)

		longSearch := "This is a very long search string that should be truncated in the summary"
		longReplace := "This is also a very long replacement string that should be truncated"

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.EditFileArgs{
			Path:       "long_strings.txt",
			SearchStr:  longSearch,
			ReplaceStr: longReplace,
		}))
		is.NotError(t, err)
		is.Equal(t, "Editing file long_strings.txt: Replacing \"This is a very long ...\" with \"This is also a very ...\"", summary)
	})
}

func mustMarshalJSON(v any) json.RawMessage {
	d, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return d
}
