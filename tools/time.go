package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"maragu.dev/gai"
)

type GetTimeArgs struct{}

// NewGetTime creates a new tool that returns the current date and time, given the time function.
func NewGetTime(now func() time.Time) gai.Tool {
	return gai.Tool{
		Name:        "get_time",
		Description: "Get the current date and time, in the format YYYY-MM-DDTHH:MM:SSZ (RFC3339).",
		Schema:      gai.GenerateToolSchema[GetTimeArgs](),
		Summarize: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			return "", nil
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var getTimeArgs GetTimeArgs
			if err := json.Unmarshal(args, &getTimeArgs); err != nil {
				return "", fmt.Errorf("error unmarshaling get_time args from JSON: %w", err)
			}

			return now().Format(time.RFC3339), nil
		},
	}
}
