package gai_test

import (
	"context"
	"encoding/json"
	"testing"

	"maragu.dev/gai"
	"maragu.dev/is"
)

type EchoArgs struct {
	Text string `json:"text" jsonschema_description:"Text to echo."`
}

func TestToolSummarize(t *testing.T) {
	ctx := context.Background()

	// Create a test tool with both Function and Summarize
	tool := gai.Tool{
		Name:        "echo",
		Description: "Echo the input text",
		Schema:      gai.GenerateSchema[EchoArgs](),
		Function: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args EchoArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "", err
			}
			return args.Text, nil
		},
		Summarize: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args EchoArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "", err
			}
			return "Echo: " + args.Text, nil
		},
	}

	// Test args
	args := json.RawMessage(`{"text": "Hello, world!"}`)

	// Test Function
	result, err := tool.Function(ctx, args)
	is.NotError(t, err)
	is.Equal(t, "Hello, world!", result)

	// Test Summarize
	summary, err := tool.Summarize(ctx, args)
	is.NotError(t, err)
	is.Equal(t, "Echo: Hello, world!", summary)
}

// Test a tool without a Summarize function
func TestToolWithoutSummarize(t *testing.T) {
	tool := gai.Tool{
		Name:        "echo",
		Description: "Echo the input text",
		Schema:      gai.GenerateSchema[EchoArgs](),
		Function: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args EchoArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "", err
			}
			return args.Text, nil
		},
	}

	// Ensure Summarize is nil
	if tool.Summarize != nil {
		t.Error("Expected Summarize to be nil")
	}
}