// Example of using robust.ChatCompleter with OpenAI (primary) + Anthropic (fallback).
//
// Running this example hits the APIs with empty keys on purpose — it exercises the
// 401 → retry → fallback → exhaustion path end to end so it's a useful smoke test
// of the failover behavior without any setup.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/anthropic"
	"maragu.dev/gai/clients/openai"
	"maragu.dev/gai/robust"
)

func main() {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Key intentionally empty — see top-of-file comment.
	primary := openai.NewClient(openai.NewClientOptions{
		Log: log,
	}).NewChatCompleter(openai.NewChatCompleterOptions{
		Model: openai.ChatCompleteModelGPT5_1Mini,
	})

	// Key intentionally empty — see top-of-file comment.
	secondary := anthropic.NewClient(anthropic.NewClientOptions{
		Log: log,
	}).NewChatCompleter(anthropic.NewChatCompleterOptions{
		Model: anthropic.ChatCompleteModelClaudeSonnet4_6Latest,
	})

	cc := robust.NewChatCompleter(robust.NewChatCompleterOptions{
		Completers:  []gai.ChatCompleter{primary, secondary},
		MaxAttempts: 3,
		BaseDelay:   500 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		Log:         log,
	})

	res, err := cc.ChatComplete(ctx, gai.ChatCompleteRequest{
		Messages: []gai.Message{
			gai.NewUserTextMessage("Write a one-line haiku about resilience."),
		},
	})
	if err != nil {
		log.Error("chat-completing", "error", err)
		return
	}

	for part, err := range res.Parts() {
		if err != nil {
			log.Error("streaming", "error", err)
			return
		}
		if part.Type == gai.PartTypeText {
			fmt.Print(part.Text())
		}
	}
	fmt.Println()
}
