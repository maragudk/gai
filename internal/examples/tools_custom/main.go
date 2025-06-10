package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"

	"maragu.dev/gai"
	openai "maragu.dev/gai-openai"
)

type EatArgs struct {
	What string `json:"what" jsonschema_description:"What you'd like to eat."`
}

func NewEat() gai.Tool {
	return gai.Tool{
		Name:        "eat",
		Description: "Eat something, supplying what you eat as an argument. The result will be a string describing how it was.",
		Schema:      gai.GenerateToolSchema[EatArgs](),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var eatArgs EatArgs
			if err := json.Unmarshal(args, &eatArgs); err != nil {
				return "", fmt.Errorf("error unmarshaling eat args from JSON: %w", err)
			}

			results := []string{
				"it was okay.",
				"it was absolutely excellent!",
				"it was awful.",
				"it gave you diarrhea.",
			}

			return "You ate " + eatArgs.What + " and " + results[rand.IntN(len(results))], nil
		},
	}
}

func main() {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	c := openai.NewClient(openai.NewClientOptions{
		Key: os.Getenv("OPENAI_API_KEY"),
		Log: log,
	})

	cc := c.NewChatCompleter(openai.NewChatCompleterOptions{
		Model: openai.ChatCompleteModelGPT4o,
	})

	req := gai.ChatCompleteRequest{
		Messages: []gai.Message{
			gai.NewUserTextMessage("Eat something, and tell me how it was. Elaborate."),
		},
		System: gai.Ptr("You are a British seagull. Speak like it. You must use the \"eat\" tool."),
		Tools: []gai.Tool{
			NewEat(),
		},
	}

	res, err := cc.ChatComplete(ctx, req)
	if err != nil {
		log.Error("Error chat-completing", "error", err)
		return
	}

	var parts []gai.MessagePart
	var result gai.ToolResult

	for part, err := range res.Parts() {
		if err != nil {
			log.Error("Error processing part", "error", err)
			return
		}

		parts = append(parts, part)

		switch part.Type {
		case gai.MessagePartTypeText:
			fmt.Print(part.Text())

		case gai.MessagePartTypeToolCall:
			toolCall := part.ToolCall()
			for _, tool := range req.Tools {
				if tool.Name != toolCall.Name {
					continue
				}

				content, err := tool.Execute(ctx, toolCall.Args) // Tools aren't called automatically, so you can decide if, how, and when
				result = gai.ToolResult{
					ID:      toolCall.ID,
					Name:    toolCall.Name,
					Content: content,
					Err:     err,
				}
				break
			}
		}
	}

	if result.ID == "" {
		log.Error("No tool result found")
		return
	}

	// Add both the tool call (in the parts) and the tool result to the messages, and make another request
	req.Messages = append(req.Messages,
		gai.Message{Role: gai.MessageRoleModel, Parts: parts},
		gai.NewUserToolResultMessage(result),
	)
	req.System = nil

	res, err = cc.ChatComplete(ctx, req)
	if err != nil {
		log.Error("Error chat-completing", "error", err)
		return
	}

	for part, err := range res.Parts() {
		if err != nil {
			log.Error("Error processing part", "error", err)
			return
		}

		switch part.Type {
		case gai.MessagePartTypeText:
			fmt.Print(part.Text())
		}
	}
}
