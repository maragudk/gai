package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"maragu.dev/gai"
)

// ExecArgs holds the arguments for the Exec tool
type ExecArgs struct {
	Command string   `json:"command" jsonschema_description:"The command to execute."`
	Args    []string `json:"args,omitempty" jsonschema_description:"Arguments to pass to the command."`
	Input   string   `json:"input,omitempty" jsonschema_description:"Optional input to provide to stdin."`
	Timeout int      `json:"timeout,omitempty" jsonschema_description:"Optional timeout in seconds. Default is 30 seconds."`
}

// NewExec creates a new tool for executing shell commands
func NewExec() gai.Tool {
	return gai.Tool{
		Name: "exec",
		Description: `Execute a shell command and capture its output.

Executes the provided command with the specified arguments and returns the output.
- Stdin can be provided via the input parameter
- Timeout can be specified in seconds (default is 30 seconds)
- Both stdout and stderr are captured and included in the output
- Command arguments are properly escaped`,
		Schema: gai.GenerateSchema[ExecArgs](),
		Summarize: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args ExecArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "error parsing arguments", nil
			}

			// Start with command
			summary := fmt.Sprintf(`command="%s"`, args.Command)

			// Add args if present
			if len(args.Args) > 0 {
				if len(args.Args) <= 3 {
					summary += fmt.Sprintf(` args=%v`, args.Args)
				} else {
					// Show first 3 args and total count
					summary += fmt.Sprintf(` args=[%s,%s,%s,...] (%d total)`,
						args.Args[0], args.Args[1], args.Args[2], len(args.Args))
				}
			}

			// Add input if present (truncated)
			if args.Input != "" {
				truncated := args.Input
				if len(truncated) > 20 {
					truncated = truncated[:20] + "..."
				}
				summary += fmt.Sprintf(` input="%s"`, truncated)
			}

			// Add timeout if different from default
			if args.Timeout > 0 && args.Timeout != 30 {
				summary += fmt.Sprintf(` timeout=%ds`, args.Timeout)
			}

			return summary, nil
		},
		Function: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args ExecArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "", fmt.Errorf("error unmarshaling exec args from JSON: %w", err)
			}

			if args.Command == "" {
				return "", errors.New("command cannot be empty")
			}

			// Set default timeout if not provided
			timeout := 30
			if args.Timeout > 0 {
				timeout = args.Timeout
			}

			// Create a context with timeout
			execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
			defer cancel()

			// Create the command with provided arguments
			cmd := exec.CommandContext(execCtx, args.Command, args.Args...)

			// Buffer for stdout and stderr
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			// If input is provided, set up stdin
			if args.Input != "" {
				cmd.Stdin = strings.NewReader(args.Input)
			}

			// Execute the command
			err := cmd.Run()

			// Check if the error was due to the context being canceled (timeout)
			if err != nil && execCtx.Err() == context.DeadlineExceeded {
				return fmt.Sprintf("Command timed out after %d seconds", timeout),
					fmt.Errorf("command timed out after %d seconds", timeout)
			}

			// Format the output
			var result strings.Builder

			// Add stdout if present
			stdoutStr := stdout.String()
			if len(stdoutStr) > 0 {
				result.WriteString("STDOUT:\n")
				result.WriteString(stdoutStr)
				// Add newline if stdout doesn't end with one
				if !strings.HasSuffix(stdoutStr, "\n") {
					result.WriteString("\n")
				}
			}

			// Add stderr if present
			stderrStr := stderr.String()
			if len(stderrStr) > 0 {
				if result.Len() > 0 {
					result.WriteString("\n")
				}
				result.WriteString("STDERR:\n")
				result.WriteString(stderrStr)
				// Add newline if stderr doesn't end with one
				if !strings.HasSuffix(stderrStr, "\n") {
					result.WriteString("\n")
				}
			}

			// Add error information if command failed
			if err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					if result.Len() > 0 {
						result.WriteString("\n")
					}
					result.WriteString(fmt.Sprintf("Command exited with status %d\n", exitErr.ExitCode()))
				} else {
					if result.Len() > 0 {
						result.WriteString("\n")
					}
					result.WriteString(fmt.Sprintf("ERROR: %s\n", err.Error()))
				}
				return result.String(), err
			}

			// If no output was generated, indicate success
			if result.Len() == 0 {
				result.WriteString("Command executed successfully with no output")
			}

			return result.String(), nil
		},
	}
}
