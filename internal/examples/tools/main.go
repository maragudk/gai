package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/openai"
	"maragu.dev/gai/tools"
)

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
			gai.NewUserTextMessage("What time is it?"),
		},
		System: gai.Ptr("You are a British seagull. Speak like it."),
		Tools: []gai.Tool{
			tools.NewGetTime(time.Now), // Note that some tools that only require the stdlib are included in GAI
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
