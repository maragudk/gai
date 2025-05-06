package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"maragu.dev/gai"
)

type SaveMemoryArgs struct {
	Memory string `json:"memory"`
}

type memorySaver interface {
	SaveMemory(ctx context.Context, memory string) error
}

func NewSaveMemory(ms memorySaver) gai.Tool {
	return gai.Tool{
		Name:        "save_memory",
		Description: "Save a memory, something you would like to remember for later conversations.",
		Schema:      gai.GenerateSchema[SaveMemoryArgs](),
		Function: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args SaveMemoryArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "", fmt.Errorf("error unmarshaling save_memory args from JSON: %w", err)
			}

			if err := ms.SaveMemory(ctx, args.Memory); err != nil {
				return "", fmt.Errorf("error saving memory: %w", err)
			}

			return "OK", nil
		},
	}
}

type GetMemoryArgs struct{}

type memoryGetter interface {
	GetMemories(ctx context.Context) ([]string, error)
}

func NewGetMemories(mg memoryGetter) gai.Tool {
	return gai.Tool{
		Name:        "get_memories",
		Description: "Get all saved memories.",
		Schema:      gai.GenerateSchema[GetMemoryArgs](),
		Function: func(ctx context.Context, _ json.RawMessage) (string, error) {
			memories, err := mg.GetMemories(ctx)
			if err != nil {
				return "", fmt.Errorf("error getting memories: %w", err)
			}

			return fmt.Sprintf("Memories: %v", memories), nil
		},
	}
}
