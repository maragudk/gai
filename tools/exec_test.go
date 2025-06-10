package tools_test

import (
	"strings"
	"testing"
	"time"

	"maragu.dev/is"

	"maragu.dev/gai/tools"
)

func TestNewExec(t *testing.T) {
	t.Run("successfully executes a command", func(t *testing.T) {
		tool := tools.NewExec()

		// Check tool name
		is.Equal(t, "exec", tool.Name)

		// Execute a simple echo command
		result, err := tool.Execute(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "echo",
			Args:    []string{"Hello, World!"},
		}))

		is.NotError(t, err)
		is.True(t, strings.Contains(result, "Hello, World!"))
	})

	t.Run("handles command with stdin input", func(t *testing.T) {
		tool := tools.NewExec()

		// Execute the cat command, which reads from stdin
		result, err := tool.Execute(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "cat",
			Input:   "Input from stdin",
		}))

		is.NotError(t, err)
		is.True(t, strings.Contains(result, "Input from stdin"))
	})

	t.Run("handles command failure", func(t *testing.T) {
		tool := tools.NewExec()

		// Execute a command that will fail
		result, err := tool.Execute(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "ls",
			Args:    []string{"/nonexistent/directory"},
		}))

		is.True(t, err != nil)
		is.True(t, strings.Contains(result, "STDERR:"))
		is.True(t, strings.Contains(result, "Command exited with status"))
	})

	t.Run("properly escapes arguments", func(t *testing.T) {
		tool := tools.NewExec()

		// Execute echo with arguments that need escaping
		result, err := tool.Execute(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "echo",
			Args:    []string{"Hello", "Special$Characters", "\"Quotes\"", "`Backticks`"},
		}))

		is.NotError(t, err)
		is.True(t, strings.Contains(result, "Hello Special$Characters \"Quotes\" `Backticks`"))
	})

	t.Run("returns error for empty command", func(t *testing.T) {
		tool := tools.NewExec()

		// Execute with an empty command
		_, err := tool.Execute(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "",
		}))

		is.True(t, err != nil)
		is.Equal(t, "command cannot be empty", err.Error())
	})

	t.Run("handles command with multiple arguments", func(t *testing.T) {
		tool := tools.NewExec()

		// Execute a command with multiple arguments
		result, err := tool.Execute(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "echo",
			Args:    []string{"arg1", "arg2", "arg3"},
		}))

		is.NotError(t, err)
		is.True(t, strings.Contains(result, "arg1 arg2 arg3"))
	})

	t.Run("handles binary data in stdin", func(t *testing.T) {
		tool := tools.NewExec()

		// Create binary data with null bytes
		binaryInput := "Binary\x00Data\x00With\x00Nulls"

		// Use hexdump to see the binary data
		result, err := tool.Execute(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "hexdump",
			Args:    []string{"-C"},
			Input:   binaryInput,
		}))

		is.NotError(t, err)
		// The hexdump output should contain "00" representing null bytes
		is.True(t, strings.Contains(result, "00"))
	})

	t.Run("captures stderr output", func(t *testing.T) {
		tool := tools.NewExec()

		// Run a command that writes to stderr
		result, err := tool.Execute(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "sh",
			Args:    []string{"-c", "echo 'Standard output'; echo 'Error output' >&2"},
		}))

		is.NotError(t, err)
		is.True(t, strings.Contains(result, "Standard output"))
		is.True(t, strings.Contains(result, "Error output"))
	})

	t.Run("handles nonexistent command", func(t *testing.T) {
		tool := tools.NewExec()

		// Run a command that doesn't exist
		result, err := tool.Execute(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "nonexistentcommand",
		}))

		is.True(t, err != nil)
		is.True(t, strings.Contains(result, "ERROR:"))
		is.True(t, strings.Contains(result, "executable file not found"))
	})

	t.Run("respects custom timeout", func(t *testing.T) {
		tool := tools.NewExec()

		// Run a command with a short timeout that will exceed the timeout
		start := time.Now()
		result, err := tool.Execute(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "sleep",
			Args:    []string{"5"},
			Timeout: 1, // 1 second timeout
		}))
		elapsed := time.Since(start)

		is.True(t, err != nil)
		is.True(t, strings.Contains(result, "timed out"))
		is.True(t, strings.Contains(err.Error(), "timed out"))
		// Ensure it didn't wait the full 5 seconds
		is.True(t, elapsed < 3*time.Second)
	})

	t.Run("handles command with no output", func(t *testing.T) {
		tool := tools.NewExec()

		// Execute a command that produces no output
		result, err := tool.Execute(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "true",
		}))

		is.NotError(t, err)
		is.True(t, strings.Contains(result, "Command executed successfully with no output"))
	})

	t.Run("summarize with basic command", func(t *testing.T) {
		tool := tools.NewExec()

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "echo",
		}))

		is.NotError(t, err)
		is.Equal(t, `command="echo"`, summary)
	})

	t.Run("summarize with command and args", func(t *testing.T) {
		tool := tools.NewExec()

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "ls",
			Args:    []string{"-la", "/tmp"},
		}))

		is.NotError(t, err)
		is.Equal(t, `command="ls" args=[-la /tmp]`, summary)
	})

	t.Run("summarize with many args", func(t *testing.T) {
		tool := tools.NewExec()

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "echo",
			Args:    []string{"arg1", "arg2", "arg3", "arg4", "arg5"},
		}))

		is.NotError(t, err)
		is.Equal(t, `command="echo" args=[arg1,arg2,arg3,...] (5 total)`, summary)
	})

	t.Run("summarize with input", func(t *testing.T) {
		tool := tools.NewExec()

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "cat",
			Input:   "Short input",
		}))

		is.NotError(t, err)
		is.Equal(t, `command="cat" input="Short input"`, summary)
	})

	t.Run("summarize with long input", func(t *testing.T) {
		tool := tools.NewExec()

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "cat",
			Input:   "This is a very long input that should be truncated",
		}))

		is.NotError(t, err)
		is.Equal(t, `command="cat" input="This is a very long ..."`, summary)
	})

	t.Run("summarize with custom timeout", func(t *testing.T) {
		tool := tools.NewExec()

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "sleep",
			Args:    []string{"5"},
			Timeout: 10,
		}))

		is.NotError(t, err)
		is.Equal(t, `command="sleep" args=[5] timeout=10s`, summary)
	})

	t.Run("summarize with default timeout", func(t *testing.T) {
		tool := tools.NewExec()

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "sleep",
			Args:    []string{"5"},
			Timeout: 30, // Default timeout
		}))

		is.NotError(t, err)
		is.Equal(t, `command="sleep" args=[5]`, summary)
	})

	t.Run("summarize with all options", func(t *testing.T) {
		tool := tools.NewExec()

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.ExecArgs{
			Command: "grep",
			Args:    []string{"-r", "pattern", "/path/to/search"},
			Input:   "Some input text that will be truncated",
			Timeout: 60,
		}))

		is.NotError(t, err)
		is.Equal(t, `command="grep" args=[-r pattern /path/to/search] input="Some input text that..." timeout=60s`, summary)
	})

	t.Run("summarize with invalid JSON", func(t *testing.T) {
		tool := tools.NewExec()

		summary, err := tool.Summarize(t.Context(), []byte(`{invalid json`))

		is.NotError(t, err)
		is.Equal(t, "error parsing arguments", summary)
	})

}
