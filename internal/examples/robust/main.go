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

	// Primary: OpenAI. Fallback: Anthropic. If the primary rate-limits or 5xx's,
	// retry up to MaxAttempts times with jittered backoff, then fall over to the secondary.
	primary := openai.NewClient(openai.NewClientOptions{
		Key: os.Getenv("OPENAI_API_KEY"),
		Log: log,
	}).NewChatCompleter(openai.NewChatCompleterOptions{
		Model: openai.ChatCompleteModelGPT5_1Mini,
	})

	secondary := anthropic.NewClient(anthropic.NewClientOptions{
		Key: os.Getenv("ANTHROPIC_KEY"),
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
		// ErrorClassifier left nil to use the built-in default:
		// context errors fail, 429/5xx retry, other 4xx fall back, rest retry.
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
