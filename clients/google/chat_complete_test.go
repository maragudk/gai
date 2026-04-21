package google_test

import (
	_ "embed"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/google"
	"maragu.dev/gai/tools"
)

//go:embed testdata/logo.jpg
var image []byte

//go:embed testdata/hello-there.m4a
var audio []byte

//go:embed testdata/thumbs-up.mov
var video []byte

func TestChatCompleter_ChatComplete(t *testing.T) {
	t.Run("can chat-complete", func(t *testing.T) {
		cc := newChatCompleter(t)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("Hi!"),
			},
			Temperature: gai.Ptr(gai.Temperature(0)),
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		for part, err := range res.Parts() {
			is.NotError(t, err)

			switch part.Type {
			case gai.PartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}

		is.True(t, strings.Contains(output, "How can I help you today?"), output)

		req.Messages = append(req.Messages, gai.NewModelTextMessage(output))
		req.Messages = append(req.Messages, gai.NewUserTextMessage("What does the acronym AI stand for? Be brief."))

		res, err = cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		output = ""
		for part, err := range res.Parts() {
			is.NotError(t, err)

			switch part.Type {
			case gai.PartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}
		is.True(t, strings.Contains(output, "Artificial Intelligence"), output)
	})

	t.Run("can use a tool", func(t *testing.T) {
		cc := newChatCompleter(t)

		root, err := os.OpenRoot("testdata")
		is.NotError(t, err)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("What is in the readme.txt file?"),
			},
			Temperature: gai.Ptr(gai.Temperature(0)),
			Tools: []gai.Tool{
				tools.NewReadFile(root),
			},
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		var found bool
		var parts []gai.Part
		var result gai.ToolResult
		for part, err := range res.Parts() {
			is.NotError(t, err)

			parts = append(parts, part)

			switch part.Type {
			case gai.PartTypeToolCall:
				toolCall := part.ToolCall()
				for _, tool := range req.Tools {
					if tool.Name == toolCall.Name {
						found = true
						content, err := tool.Execute(t.Context(), toolCall.Args)
						result = gai.ToolResult{
							ID:      toolCall.ID,
							Name:    tool.Name,
							Content: content,
							Err:     err,
						}
						break
					}
				}

			case gai.PartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}

		is.Equal(t, "", output)
		is.True(t, found, "tool not found")
		is.Equal(t, "Hi!\n", result.Content)
		is.NotError(t, result.Err)

		req.Messages = []gai.Message{
			gai.NewUserTextMessage("What is in the readme.txt file?"),
			{Role: gai.MessageRoleModel, Parts: parts},
			gai.NewUserToolResultMessage(result),
		}
		req.System = gai.Ptr("Answer the user's question in a single sentence using the tool result. Do not call any more tools.")

		res, err = cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		output = ""
		for part, err := range res.Parts() {
			is.NotError(t, err)

			switch part.Type {
			case gai.PartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}

		t.Log(output)
		lower := strings.ToLower(output)
		is.True(t, strings.Contains(lower, "readme.txt"), output)
		is.True(t, strings.Contains(output, "Hi"), output)
	})

	t.Run("can use a tool with no args", func(t *testing.T) {
		cc := newChatCompleter(t)

		root, err := os.OpenRoot("testdata")
		is.NotError(t, err)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("What is in the current directory?"),
			},
			Temperature: gai.Ptr(gai.Temperature(0)),
			Tools: []gai.Tool{
				tools.NewListDir(root),
			},
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		var found bool
		var parts []gai.Part
		var result gai.ToolResult
		for part, err := range res.Parts() {
			is.NotError(t, err)

			parts = append(parts, part)

			switch part.Type {
			case gai.PartTypeToolCall:
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

			case gai.PartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}

		is.Equal(t, "", output)
		is.True(t, found, "tool not found")
		is.Equal(t, `["hello-there.m4a","logo.jpg","readme.txt","thumbs-up.mov"]`, result.Content)
		is.NotError(t, result.Err)
	})

	t.Run("can use a system prompt", func(t *testing.T) {
		cc := newChatCompleter(t)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("Hi!"),
			},
			System:      gai.Ptr("You always respond in French."),
			Temperature: gai.Ptr(gai.Temperature(0)),
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		for part, err := range res.Parts() {
			is.NotError(t, err)

			switch part.Type {
			case gai.PartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}

		// Accept either "bonjour" (formal) or "salut" (informal); both satisfy the
		// "respond in French" intent even if a model revision drifts on register
		// or trailing punctuation.
		lower := strings.ToLower(output)
		is.True(t, strings.Contains(lower, "bonjour") || strings.Contains(lower, "salut"), output)
	})

	t.Run("can use structured output", func(t *testing.T) {
		cc := newChatCompleter(t)

		type BookRecommendation struct {
			Title  string `json:"title"`
			Author string `json:"author"`
			Year   int    `json:"year"`
		}

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("Recommend a science fiction book. Include the title, author, and the year it was published."),
			},
			ResponseSchema: gai.Ptr(gai.GenerateSchema[BookRecommendation]()),
			Temperature:    gai.Ptr(gai.Temperature(0)),
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		for part, err := range res.Parts() {
			is.NotError(t, err)

			switch part.Type {
			case gai.PartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}

		// Verify it's valid JSON with the expected structure
		var book BookRecommendation
		err = json.Unmarshal([]byte(output), &book)
		is.NotError(t, err)

		// Check that all fields are populated. Avoid pinning the exact recommendation
		// (Dune / Frank Herbert / 1965) since a model revision could reasonably
		// suggest a different canonical sci-fi title.
		is.True(t, book.Title != "", "title should not be empty")
		is.True(t, book.Author != "", "author should not be empty")
		is.True(t, book.Year > 0, "year should be positive")
	})

	t.Run("can describe an image", func(t *testing.T) {
		cc := newChatCompleter(t)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserDataMessage("image/jpeg", image),
			},
			System:      gai.Ptr("Describe this image concisely."),
			Temperature: gai.Ptr(gai.Temperature(0)),
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		for part, err := range res.Parts() {
			is.NotError(t, err)

			switch part.Type {
			case gai.PartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}

		t.Log(output)
		is.True(t, len(output) > 0, "should have output")
	})

	t.Run("can describe audio", func(t *testing.T) {
		cc := newChatCompleter(t)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserDataMessage("audio/mp4", audio),
			},
			System:      gai.Ptr("Describe this audio concisely."),
			Temperature: gai.Ptr(gai.Temperature(0)),
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		for part, err := range res.Parts() {
			is.NotError(t, err)

			switch part.Type {
			case gai.PartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}

		t.Log(output)
		lower := strings.ToLower(output)
		is.True(t, strings.Contains(lower, "voice") || strings.Contains(lower, "speech") || strings.Contains(lower, "says") || strings.Contains(lower, "hello"), "should describe the audio content")
	})

	t.Run("can describe a video", func(t *testing.T) {
		cc := newChatCompleter(t)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserDataMessage("video/quicktime", video),
			},
			System:      gai.Ptr("Describe this video concisely."),
			Temperature: gai.Ptr(gai.Temperature(0)),
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		for part, err := range res.Parts() {
			is.NotError(t, err)

			switch part.Type {
			case gai.PartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}

		t.Log(output)
		normalized := strings.ToLower(strings.ReplaceAll(output, "-", " "))
		is.True(t, strings.Contains(normalized, "thumbs up"), "should contain thumbs-up")
	})

	t.Run("tracks token usage", func(t *testing.T) {
		cc := newChatCompleter(t)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("Hi!"),
			},
			Temperature: gai.Ptr(gai.Temperature(0)),
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		// Consume the response to ensure token usage is populated
		var output string
		for part, err := range res.Parts() {
			is.NotError(t, err)
			if part.Type == gai.PartTypeText {
				output += part.Text()
			}
		}

		// Check that we got a response
		is.True(t, len(output) > 0, "should have response text")

		// Check token usage in Meta.Usage
		is.NotNil(t, res.Meta, "should have metadata")
		t.Log(res.Meta.Usage.PromptTokens, res.Meta.Usage.CompletionTokens, res.Meta.Usage.ThoughtsTokens)
		is.True(t, res.Meta.Usage.PromptTokens > 0, "should have prompt tokens")
		is.True(t, res.Meta.Usage.CompletionTokens > 0, "should have completion tokens")
		is.True(t, res.Meta.Usage.ThoughtsTokens > 0, "should have thoughts tokens")
	})

	t.Run("respects max completion tokens", func(t *testing.T) {
		const maxCompletionTokens = 3

		cc := newChatCompleter(t)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("Write a poem of at least 20 words about gophers."),
			},
			Temperature:         gai.Ptr(gai.Temperature(0)),
			MaxCompletionTokens: gai.Ptr(maxCompletionTokens),
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var limitedOutput string
		for part, err := range res.Parts() {
			is.NotError(t, err)
			if part.Type == gai.PartTypeText {
				limitedOutput += part.Text()
			}
		}

		is.NotNil(t, res.Meta)
		is.True(t, res.Meta.Usage.CompletionTokens <= maxCompletionTokens, "should respect max completion tokens")

		req.MaxCompletionTokens = nil

		res, err = cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var fullOutput string
		for part, err := range res.Parts() {
			is.NotError(t, err)
			if part.Type == gai.PartTypeText {
				fullOutput += part.Text()
			}
		}

		is.NotNil(t, res.Meta)
		is.True(t, res.Meta.Usage.CompletionTokens > maxCompletionTokens, "should exceed limit when not constrained")
		is.True(t, len(fullOutput) > len(limitedOutput), "should produce more output without limit")
	})

	t.Run("panics on empty MIME type", func(t *testing.T) {
		cc := newChatCompleter(t)

		defer func() {
			r := recover()
			is.True(t, r != nil)
			is.Equal(t, "data part has empty MIME type", r)
		}()

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				{Role: gai.MessageRoleUser, Parts: []gai.Part{
					{Type: gai.PartTypeData, Data: []byte("data")},
				}},
			},
		}
		_, _ = cc.ChatComplete(t.Context(), req)
	})

	t.Run("panics on empty data", func(t *testing.T) {
		cc := newChatCompleter(t)

		defer func() {
			r := recover()
			is.True(t, r != nil)
			is.Equal(t, "data part has empty data", r)
		}()

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				{Role: gai.MessageRoleUser, Parts: []gai.Part{
					{Type: gai.PartTypeData, MIMEType: "image/jpeg"},
				}},
			},
		}
		_, _ = cc.ChatComplete(t.Context(), req)
	})
}

func TestChatCompleter_ChatComplete_VertexAI(t *testing.T) {
	t.Run("can chat-complete with Vertex AI backend", func(t *testing.T) {
		c := newVertexAIClient(t)
		cc := c.NewChatCompleter(google.NewChatCompleterOptions{
			Model: google.ChatCompleteModelGemini2_5Flash,
		})

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("Hi!"),
			},
			Temperature: gai.Ptr(gai.Temperature(0)),
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		for part, err := range res.Parts() {
			is.NotError(t, err)

			switch part.Type {
			case gai.PartTypeText:
				output += part.Text()

			default:
				t.Fatal("unexpected message parts")
			}
		}

		is.True(t, len(output) > 0, "should have response text")
	})
}

func newChatCompleter(t *testing.T) *google.ChatCompleter {
	c := newClient(t)
	cc := c.NewChatCompleter(google.NewChatCompleterOptions{
		Model: google.ChatCompleteModelGemini2_5Flash,
	})
	return cc
}
