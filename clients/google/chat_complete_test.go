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

	// Thinking-level matrix. Each row exercises a real (model, level) combination so the
	// per-client `ThinkingLevel` mapping is grounded in live API behaviour. Stays
	// single-turn — multi-turn tool flows on Gemini 3.x require thought_signature
	// round-trip (https://github.com/maragudk/gai/issues/256), which is deferred.
	t.Run("thinking level matrix", func(t *testing.T) {
		tests := []struct {
			name              string
			model             google.ChatCompleteModel
			level             gai.ThinkingLevel
			wantErr           bool
			requireThoughts   bool // strict: assert thoughtParts > 0
			wantThoughtTokens bool // assert Usage.ThoughtsTokens > 0
		}{
			// The 2.5 family does not accept the symbolic ThinkingLevel enum at all — the
			// API returns 400 with `Thinking level is not supported for this model`. These
			// rows confirm the rejection surfaces cleanly.
			{name: "2.5 flash rejects symbolic level", model: google.ChatCompleteModelGemini2_5Flash, level: google.ThinkingLevelLow, wantErr: true},
			{name: "2.5 flash-lite rejects symbolic level", model: google.ChatCompleteModelGemini2_5FlashLite, level: google.ThinkingLevelMedium, wantErr: true},
			{name: "2.5 pro rejects symbolic level", model: google.ChatCompleteModelGemini2_5Pro, level: google.ThinkingLevelHigh, wantErr: true},

			// Flash 3 accepts every level including `gai.ThinkingLevelNone` (mapped to
			// ThinkingBudget=0). The streaming API does not surface thought summaries on
			// Flash 3 — those only show up on the batch endpoint — so we don't strictly
			// assert PartTypeThought parts here. ThoughtsTokens are populated from the
			// usage metadata at non-trivial levels.
			{name: "flash 3 + none", model: google.ChatCompleteModelGemini3FlashPreview, level: gai.ThinkingLevelNone},
			{name: "flash 3 + minimal", model: google.ChatCompleteModelGemini3FlashPreview, level: google.ThinkingLevelMinimal},
			{name: "flash 3 + low", model: google.ChatCompleteModelGemini3FlashPreview, level: google.ThinkingLevelLow, wantThoughtTokens: true},
			{name: "flash 3 + medium", model: google.ChatCompleteModelGemini3FlashPreview, level: google.ThinkingLevelMedium, wantThoughtTokens: true},
			{name: "flash 3 + high", model: google.ChatCompleteModelGemini3FlashPreview, level: google.ThinkingLevelHigh, wantThoughtTokens: true},

			// Flash Lite 3.1 accepts every level too. Streaming behaviour: no thought parts
			// at None/Minimal/Low/Medium, but at High the streaming API does emit one
			// PartTypeThought (different from full Flash 3 which never streams thoughts).
			// thoughts_tokens are populated from Low onwards.
			{name: "flash-lite 3.1 + none", model: google.ChatCompleteModelGemini3_1FlashLitePreview, level: gai.ThinkingLevelNone},
			{name: "flash-lite 3.1 + minimal", model: google.ChatCompleteModelGemini3_1FlashLitePreview, level: google.ThinkingLevelMinimal},
			{name: "flash-lite 3.1 + low", model: google.ChatCompleteModelGemini3_1FlashLitePreview, level: google.ThinkingLevelLow, wantThoughtTokens: true},
			{name: "flash-lite 3.1 + medium", model: google.ChatCompleteModelGemini3_1FlashLitePreview, level: google.ThinkingLevelMedium, wantThoughtTokens: true},
			{name: "flash-lite 3.1 + high", model: google.ChatCompleteModelGemini3_1FlashLitePreview, level: google.ThinkingLevelHigh, requireThoughts: true, wantThoughtTokens: true},

			// Pro 3.1 rejects the off path entirely: `This model only works in thinking
			// mode`. It also rejects MINIMAL: `Thinking level MINIMAL is not supported for
			// this model`. Low/Medium/High all populate the thoughts-tokens count. Streamed
			// thought parts are reliable at Medium/High but flaky at Low (probe: ~50%) — at
			// Low we assert thoughts_tokens only. Same shape as the now-shut-down Gemini 3
			// Pro Preview, which we used to target until Google retired it on 2026-03-09.
			{name: "pro 3.1 + none rejected", model: google.ChatCompleteModelGemini3_1ProPreview, level: gai.ThinkingLevelNone, wantErr: true},
			{name: "pro 3.1 + minimal rejected", model: google.ChatCompleteModelGemini3_1ProPreview, level: google.ThinkingLevelMinimal, wantErr: true},
			{name: "pro 3.1 + low", model: google.ChatCompleteModelGemini3_1ProPreview, level: google.ThinkingLevelLow, wantThoughtTokens: true},
			{name: "pro 3.1 + medium", model: google.ChatCompleteModelGemini3_1ProPreview, level: google.ThinkingLevelMedium, requireThoughts: true, wantThoughtTokens: true},
			{name: "pro 3.1 + high", model: google.ChatCompleteModelGemini3_1ProPreview, level: google.ThinkingLevelHigh, requireThoughts: true, wantThoughtTokens: true},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				cc := newChatCompleter(t, test.model)

				req := gai.ChatCompleteRequest{
					Messages: []gai.Message{
						gai.NewUserTextMessage("Solve step by step: a farmer has 17 sheep, all but 9 die. How many remain?"),
					},
					Temperature:   gai.Ptr(gai.Temperature(0)),
					ThinkingLevel: gai.Ptr(test.level),
				}

				res, err := cc.ChatComplete(t.Context(), req)
				if test.wantErr {
					if err != nil {
						return
					}
					streamErr := drainParts(t, res)
					is.True(t, streamErr != nil, "expected an error from the API")
					return
				}
				is.NotError(t, err)

				var thoughtParts, textParts int
				for part, partErr := range res.Parts() {
					is.NotError(t, partErr)
					switch part.Type {
					case gai.PartTypeText:
						textParts++
					case gai.PartTypeThought:
						thoughtParts++
					default:
						t.Fatalf("unexpected part type %s", part.Type)
					}
				}
				is.True(t, textParts > 0, "should produce text parts")
				if test.requireThoughts {
					is.True(t, thoughtParts > 0, "should stream PartTypeThought parts")
				}
				if test.wantThoughtTokens {
					is.True(t, res.Meta.Usage.ThoughtsTokens > 0, "thoughts tokens should be populated")
				}
				t.Logf("thoughtParts=%d textParts=%d thoughtsTokens=%d", thoughtParts, textParts, res.Meta.Usage.ThoughtsTokens)
			})
		}
	})

	t.Run("panics on unsupported thinking level", func(t *testing.T) {
		// The Google client publishes Minimal/Low/Medium/High. Anything outside that set
		// must panic at the boundary, not silently round-trip to the API.
		tests := []struct {
			name  string
			level gai.ThinkingLevel
		}{
			{name: "xhigh not published", level: gai.ThinkingLevel("xhigh")},
			{name: "max not published", level: gai.ThinkingLevel("max")},
			{name: "arbitrary string", level: gai.ThinkingLevel("nonsense")},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				cc := newChatCompleter(t)

				defer func() {
					r := recover()
					is.True(t, r != nil, "expected a panic")
					msg, ok := r.(string)
					is.True(t, ok, "panic value should be a string")
					is.Equal(t, "unsupported thinking level: "+string(test.level), msg)
				}()

				req := gai.ChatCompleteRequest{
					Messages:      []gai.Message{gai.NewUserTextMessage("Hi!")},
					ThinkingLevel: gai.Ptr(test.level),
				}
				_, _ = cc.ChatComplete(t.Context(), req)
			})
		}
	})

	t.Run("can chat-complete via Vertex AI with API key", func(t *testing.T) {
		c := newVertexAIClientWithKey(t)
		assertVertexFlashChatComplete(t, c)
	})

	t.Run("can chat-complete via Vertex AI with service account", func(t *testing.T) {
		c := newVertexAIClientWithCredentials(t)
		assertVertexFlashChatComplete(t, c)
	})

	t.Run("rejects inbound PartTypeThought as deferred", func(t *testing.T) {
		// Multi-turn round-trip of the per-part `thought_signature` is tracked by
		// https://github.com/maragudk/gai/issues/256. Until that lands, the client
		// returns a typed error rather than silently dropping the part. This subtest
		// runs without making a network call — the error path triggers during the
		// request-message conversion, before the API is contacted.
		cc := newChatCompleter(t)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				{Role: gai.MessageRoleUser, Parts: []gai.Part{gai.TextPart("Hi!")}},
				{Role: gai.MessageRoleModel, Parts: []gai.Part{gai.ThoughtPart("the user said hi")}},
				gai.NewUserTextMessage("And again, hi!"),
			},
		}

		_, err := cc.ChatComplete(t.Context(), req)
		is.True(t, err != nil, "expected an error")
		is.True(t, strings.Contains(err.Error(), "PartTypeThought"), err.Error())
	})
}

// drainParts iterates the response stream, returning the first error if any.
func drainParts(t *testing.T, res gai.ChatCompleteResponse) error {
	t.Helper()
	for _, err := range res.Parts() {
		if err != nil {
			return err
		}
	}
	return nil
}

func assertVertexFlashChatComplete(t *testing.T, c *google.Client) {
	t.Helper()

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
}

// newChatCompleter builds a [google.ChatCompleter] for tests. With no model argument,
// the default is `gemini-2.5-flash` — held at 2.5 Flash (rather than the spec's 3.x flash)
// because Gemini 3.x enforces a `thought_signature` round-trip on tool follow-ups that
// `gai.Part` does not yet preserve. Tests that need a 3.x model pass it explicitly. See
// https://github.com/maragudk/gai/issues/256 for the deferred signature plumbing.
func newChatCompleter(t *testing.T, model ...google.ChatCompleteModel) *google.ChatCompleter {
	t.Helper()
	m := google.ChatCompleteModelGemini2_5Flash
	if len(model) > 0 {
		m = model[0]
	}
	c := newClient(t)
	return c.NewChatCompleter(google.NewChatCompleterOptions{Model: m})
}
