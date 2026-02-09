package openai_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/openai"
	"maragu.dev/gai/tools"
)

func TestChatCompleter_ChatComplete(t *testing.T) {
	t.Run("can chat-complete", func(t *testing.T) {
		cc := newChatCompleter(t)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("Hi!"),
			},

		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		for part, err := range res.Parts() {
			is.NotError(t, err)

			switch part.Type {
			case gai.MessagePartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}
		requireContainsAny(t, output, "hello", "hi")
		requireContainsAny(t, output, "assist", "help")

		req.Messages = append(req.Messages, gai.NewModelTextMessage("Hello! How can I assist you today?"))
		req.Messages = append(req.Messages, gai.NewUserTextMessage("What does the acronym AI stand for? Be brief."))

		res, err = cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		output = ""
		for part, err := range res.Parts() {
			is.NotError(t, err)

			switch part.Type {
			case gai.MessagePartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}
		requireContainsAll(t, output, "artificial intelligence")
	})

	t.Run("can use a tool with args", func(t *testing.T) {
		cc := newChatCompleter(t)

		root, err := os.OpenRoot("testdata")
		is.NotError(t, err)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("What is in the readme.txt file?"),
			},

			Tools: []gai.Tool{
				tools.NewReadFile(root),
			},
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		var found bool
		var parts []gai.MessagePart
		var result gai.ToolResult
		for part, err := range res.Parts() {
			is.NotError(t, err)

			parts = append(parts, part)

			switch part.Type {
			case gai.MessagePartTypeToolCall:
				toolCall := part.ToolCall()
				for _, tool := range req.Tools {
					if tool.Name == toolCall.Name {
						found = true
						content, err := tool.Execute(t.Context(), toolCall.Args)
						result = gai.ToolResult{
							ID:      toolCall.ID,
							Name:    toolCall.Name,
							Content: content,
							Err:     err,
						}
						break
					}
				}

			case gai.MessagePartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}

		is.Equal(t, "", output)
		is.True(t, found, "tool not found")
		is.Equal(t, "Hi!\n", result.Content)
		is.NotError(t, result.Err)
		is.NotNil(t, res.Meta, "metadata should be populated")
		is.NotNil(t, res.Meta.FinishReason, "finish reason should be set")
		is.Equal(t, gai.ChatCompleteFinishReasonToolCalls, *res.Meta.FinishReason)

		req.Messages = []gai.Message{
			gai.NewUserTextMessage("What is in the readme.txt file?"),
			{Role: gai.MessageRoleModel, Parts: parts},
			gai.NewUserToolResultMessage(result),
		}

		res, err = cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		output = ""
		for part, err := range res.Parts() {
			is.NotError(t, err)

			switch part.Type {
			case gai.MessagePartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}

		requireContainsAll(t, output, "readme.txt", "hi")
		is.NotNil(t, res.Meta.FinishReason)
		is.Equal(t, gai.ChatCompleteFinishReasonStop, *res.Meta.FinishReason)
	})

	t.Run("can use a tool with no args", func(t *testing.T) {
		cc := newChatCompleter(t)

		root, err := os.OpenRoot("testdata")
		is.NotError(t, err)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("What is in the current directory?"),
			},

			Tools: []gai.Tool{
				tools.NewListDir(root),
			},
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		var found bool
		var parts []gai.MessagePart
		var result gai.ToolResult
		for part, err := range res.Parts() {
			is.NotError(t, err)

			parts = append(parts, part)

			switch part.Type {
			case gai.MessagePartTypeToolCall:
				toolCall := part.ToolCall()
				for _, tool := range req.Tools {
					if tool.Name == toolCall.Name {
						found = true
						content, err := tool.Execute(t.Context(), toolCall.Args)
						result = gai.ToolResult{
							ID:      toolCall.ID,
							Name:    toolCall.Name,
							Content: content,
							Err:     err,
						}
						break
					}
				}

			case gai.MessagePartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}

		is.Equal(t, "", output)
		is.True(t, found, "tool not found")
		is.Equal(t, `["readme.txt"]`, result.Content)
		is.NotError(t, result.Err)
	})

	t.Run("can use structured output", func(t *testing.T) {
		cc := newChatCompleter(t)

		type Recommendation struct {
			Title  string `json:"title"`
			Author string `json:"author"`
			Year   int    `json:"year"`
		}

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("Recommend a science fiction book as JSON with title, author, and year."),
			},
			ResponseSchema: gai.Ptr(gai.GenerateSchema[Recommendation]()),
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		for part, err := range res.Parts() {
			is.NotError(t, err)

			switch part.Type {
			case gai.MessagePartTypeText:
				output += part.Text()

			default:
				t.Fatalf("unexpected message part type: %s", part.Type)
			}
		}

		var rec Recommendation
		is.NotError(t, json.Unmarshal([]byte(output), &rec))
		is.True(t, rec.Title != "", "title should not be empty")
		is.True(t, rec.Author != "", "author should not be empty")
		is.True(t, rec.Year > 0, "year should be positive")
	})

	t.Run("can use a system prompt", func(t *testing.T) {
		cc := newChatCompleter(t)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("Hi!"),
			},
			System:      gai.Ptr("You always respond in French."),

		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		for part, err := range res.Parts() {
			is.NotError(t, err)

			switch part.Type {
			case gai.MessagePartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}

		requireContainsAll(t, output, "bonjour")
	})

	t.Run("tracks token usage", func(t *testing.T) {
		cc := newChatCompleter(t)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("Hi!"),
			},

		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		// Consume the response to ensure token usage is populated
		var output string
		for part, err := range res.Parts() {
			is.NotError(t, err)
			if part.Type == gai.MessagePartTypeText {
				output += part.Text()
			}
		}

		// Check that we got a response
		is.True(t, len(output) > 0, "should have response text")

		// Check token usage in Meta.Usage
		is.NotNil(t, res.Meta, "should have metadata")
		t.Log(res.Meta.Usage.PromptTokens, res.Meta.Usage.CompletionTokens)
		is.True(t, res.Meta.Usage.PromptTokens > 0, "should have prompt tokens")
		is.True(t, res.Meta.Usage.CompletionTokens > 0, "should have completion tokens")
	})
}

func newChatCompleter(t *testing.T) *openai.ChatCompleter {
	c := newClient(t)
	cc := c.NewChatCompleter(openai.NewChatCompleterOptions{
		Model: openai.ChatCompleteModelGPT5Nano,
	})
	return cc
}

func requireContainsAll(t *testing.T, got string, want ...string) {
	t.Helper()

	lower := strings.ToLower(got)

	for _, w := range want {
		if !strings.Contains(lower, strings.ToLower(w)) {
			t.Fatalf("expected output %q to contain %q", got, w)
		}
	}
}

func requireContainsAny(t *testing.T, got string, want ...string) {
	t.Helper()

	lower := strings.ToLower(got)

	for _, w := range want {
		if strings.Contains(lower, strings.ToLower(w)) {
			return
		}
	}

	t.Fatalf("expected output %q to contain one of %v", got, want)
}
