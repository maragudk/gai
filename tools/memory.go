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
		Summarize: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args SaveMemoryArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "error parsing arguments", nil
			}

			// Truncate memory content
			memory := args.Memory
			if len(memory) > 30 {
				memory = memory[:30] + "..."
			}

			return fmt.Sprintf(`memory="%s"`, memory), nil
		},
		Execute: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
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
		Summarize: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			return "", nil
		},
		Execute: func(ctx context.Context, _ json.RawMessage) (string, error) {
			memories, err := mg.GetMemories(ctx)
			if err != nil {
				return "", fmt.Errorf("error getting memories: %w", err)
			}

			return fmt.Sprintf("Memories: %v", memories), nil
		},
	}
}

type SearchMemoriesArgs struct {
	Query string `json:"query"`
}

type memorySearcher interface {
	SearchMemories(ctx context.Context, query string) ([]string, error)
}

func NewSearchMemories(ms memorySearcher) gai.Tool {
	return gai.Tool{
		Name:        "search_memories",
		Description: "Search saved memories using a query string.",
		Schema:      gai.GenerateSchema[SearchMemoriesArgs](),
		Summarize: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args SearchMemoriesArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "error parsing arguments", nil
			}
			return fmt.Sprintf(`query="%s"`, args.Query), nil
		},
		Execute: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args SearchMemoriesArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "", fmt.Errorf("error unmarshaling search_memories args from JSON: %w", err)
			}

			memories, err := ms.SearchMemories(ctx, args.Query)
			if err != nil {
				return "", fmt.Errorf("error searching memories: %w", err)
			}

			if len(memories) == 0 {
				return "No memories found matching the query.", nil
			}

			return fmt.Sprintf("Found memories: %v", memories), nil
		},
	}
}
