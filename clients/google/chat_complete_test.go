package google_test

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/genai"
	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/google"
	"maragu.dev/gai/internal/oteltest"
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
	// single-turn — multi-turn signature round-trip has its own subtest in this file.
	t.Run("thinking level matrix", func(t *testing.T) {
		tests := []struct {
			name              string
			model             google.ChatCompleteModel
			level             gai.ThinkingLevel
			wantErr           bool
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
			// at None/Minimal/Low/Medium. At High the streaming API usually emits at least
			// one PartTypeThought, but not reliably enough to assert — it flaked twice in
			// two days — so the High row asserts thoughts_tokens only, like the rest.
			// thoughts_tokens are populated from Low onwards.
			{name: "flash-lite 3.1 + none", model: google.ChatCompleteModelGemini3_1FlashLite, level: gai.ThinkingLevelNone},
			{name: "flash-lite 3.1 + minimal", model: google.ChatCompleteModelGemini3_1FlashLite, level: google.ThinkingLevelMinimal},
			{name: "flash-lite 3.1 + low", model: google.ChatCompleteModelGemini3_1FlashLite, level: google.ThinkingLevelLow, wantThoughtTokens: true},
			{name: "flash-lite 3.1 + medium", model: google.ChatCompleteModelGemini3_1FlashLite, level: google.ThinkingLevelMedium, wantThoughtTokens: true},
			{name: "flash-lite 3.1 + high", model: google.ChatCompleteModelGemini3_1FlashLite, level: google.ThinkingLevelHigh, wantThoughtTokens: true},

			// Flash 3.5 is a thinking-capable Flash model (version 3.5-flash-05-2026) and,
			// like the rest of the 3.x Flash line, accepts every level including
			// `gai.ThinkingLevelNone` (mapped to ThinkingBudget=0). thoughts_tokens are
			// populated from the usage metadata at non-trivial levels. The streaming API
			// does surface a thought summary from Low upward (unlike full Flash 3, which
			// never streams thoughts), but we keep the assertion to thoughts_tokens to stay
			// robust against streaming variability.
			{name: "flash 3.5 + none", model: google.ChatCompleteModelGemini3_5Flash, level: gai.ThinkingLevelNone},
			{name: "flash 3.5 + minimal", model: google.ChatCompleteModelGemini3_5Flash, level: google.ThinkingLevelMinimal},
			{name: "flash 3.5 + low", model: google.ChatCompleteModelGemini3_5Flash, level: google.ThinkingLevelLow, wantThoughtTokens: true},
			{name: "flash 3.5 + medium", model: google.ChatCompleteModelGemini3_5Flash, level: google.ThinkingLevelMedium, wantThoughtTokens: true},
			{name: "flash 3.5 + high", model: google.ChatCompleteModelGemini3_5Flash, level: google.ThinkingLevelHigh, wantThoughtTokens: true},

			// Flash Lite 3.5 is the one model in the Flash line that rejects
			// `gai.ThinkingLevelNone` (ThinkingBudget=0), with a generic 400 INVALID_ARGUMENT.
			// Minimal is accepted with zero thoughts_tokens; Low upward populates
			// thoughts_tokens. Streamed thought parts are sporadic (probes: 0-2 per response),
			// so rows assert thoughts_tokens only.
			{name: "flash-lite 3.5 + none rejected", model: google.ChatCompleteModelGemini3_5FlashLite, level: gai.ThinkingLevelNone, wantErr: true},
			{name: "flash-lite 3.5 + minimal", model: google.ChatCompleteModelGemini3_5FlashLite, level: google.ThinkingLevelMinimal},
			{name: "flash-lite 3.5 + low", model: google.ChatCompleteModelGemini3_5FlashLite, level: google.ThinkingLevelLow, wantThoughtTokens: true},
			{name: "flash-lite 3.5 + medium", model: google.ChatCompleteModelGemini3_5FlashLite, level: google.ThinkingLevelMedium, wantThoughtTokens: true},
			{name: "flash-lite 3.5 + high", model: google.ChatCompleteModelGemini3_5FlashLite, level: google.ThinkingLevelHigh, wantThoughtTokens: true},

			// Flash 3.6 accepts every level, including `gai.ThinkingLevelNone`
			// (ThinkingBudget=0), and unlike 3.7 and 3.8 below it honours the zero budget:
			// 16 of 16 None probes returned zero thoughts_tokens and no thought parts, so the
			// None row asserts neither. Minimal is likewise accepted with zero thoughts_tokens;
			// Low upward populates them. Streamed thought parts are sporadic (probes: 0-2 per
			// response), so those rows assert thoughts_tokens only.
			{name: "flash 3.6 + none", model: google.ChatCompleteModelGemini3_6Flash, level: gai.ThinkingLevelNone},
			{name: "flash 3.6 + minimal", model: google.ChatCompleteModelGemini3_6Flash, level: google.ThinkingLevelMinimal},
			{name: "flash 3.6 + low", model: google.ChatCompleteModelGemini3_6Flash, level: google.ThinkingLevelLow, wantThoughtTokens: true},
			{name: "flash 3.6 + medium", model: google.ChatCompleteModelGemini3_6Flash, level: google.ThinkingLevelMedium, wantThoughtTokens: true},
			{name: "flash 3.6 + high", model: google.ChatCompleteModelGemini3_6Flash, level: google.ThinkingLevelHigh, wantThoughtTokens: true},

			// Flash 3.7 flips both edges: `gai.ThinkingLevelNone` is accepted but the model
			// usually thinks anyway, and MINIMAL is rejected like on Pro 3.1. Thoughts tokens
			// are nondeterministic at the bottom of the range, so neither the None nor the Low
			// row asserts them: None comes back at 0 in roughly one run in sixteen, Low in
			// roughly one run in ten (the same rate on genai 1.68 and 1.69, so the source is
			// the server, not the SDK). Medium and High never returned 0 across 50 probes each
			// and keep the assertion.
			{name: "flash 3.7 + none", model: google.ChatCompleteModelGemini3_7Flash, level: gai.ThinkingLevelNone},
			{name: "flash 3.7 + minimal rejected", model: google.ChatCompleteModelGemini3_7Flash, level: google.ThinkingLevelMinimal, wantErr: true},
			{name: "flash 3.7 + low", model: google.ChatCompleteModelGemini3_7Flash, level: google.ThinkingLevelLow},
			{name: "flash 3.7 + medium", model: google.ChatCompleteModelGemini3_7Flash, level: google.ThinkingLevelMedium, wantThoughtTokens: true},
			{name: "flash 3.7 + high", model: google.ChatCompleteModelGemini3_7Flash, level: google.ThinkingLevelHigh, wantThoughtTokens: true},

			// Flash 3.8 sits at the same two edges as 3.7: `gai.ThinkingLevelNone` is accepted
			// and MINIMAL is rejected with `Thinking level MINIMAL is not supported for this
			// model`. Where it differs is how loosely it holds the bottom of the range —
			// thoughts tokens came back at 0 in 10 of 15 None probes and 9 of 15 Low probes,
			// far more often than the corresponding rows on 3.7 — so neither row asserts them.
			// Medium and High never returned 0 across 20 probes each and keep the assertion.
			{name: "flash 3.8 + none", model: google.ChatCompleteModelGemini3_8Flash, level: gai.ThinkingLevelNone},
			{name: "flash 3.8 + minimal rejected", model: google.ChatCompleteModelGemini3_8Flash, level: google.ThinkingLevelMinimal, wantErr: true},
			{name: "flash 3.8 + low", model: google.ChatCompleteModelGemini3_8Flash, level: google.ThinkingLevelLow},
			{name: "flash 3.8 + medium", model: google.ChatCompleteModelGemini3_8Flash, level: google.ThinkingLevelMedium, wantThoughtTokens: true},
			{name: "flash 3.8 + high", model: google.ChatCompleteModelGemini3_8Flash, level: google.ThinkingLevelHigh, wantThoughtTokens: true},

			// Pro 3.1 rejects the off path entirely: `This model only works in thinking
			// mode`. It also rejects MINIMAL: `Thinking level MINIMAL is not supported for
			// this model`. Low/Medium/High all populate the thoughts-tokens count. Streamed
			// thought parts looked reliable at Medium/High in early probes, but that read
			// didn't hold — three flakes across two days on those two rows — so all three
			// rows assert thoughts_tokens only. Same shape as the now-shut-down Gemini 3 Pro
			// Preview, which we used to target until Google retired it on 2026-03-09.
			{name: "pro 3.1 + none rejected", model: google.ChatCompleteModelGemini3_1ProPreview, level: gai.ThinkingLevelNone, wantErr: true},
			{name: "pro 3.1 + minimal rejected", model: google.ChatCompleteModelGemini3_1ProPreview, level: google.ThinkingLevelMinimal, wantErr: true},
			{name: "pro 3.1 + low", model: google.ChatCompleteModelGemini3_1ProPreview, level: google.ThinkingLevelLow, wantThoughtTokens: true},
			{name: "pro 3.1 + medium", model: google.ChatCompleteModelGemini3_1ProPreview, level: google.ThinkingLevelMedium, wantThoughtTokens: true},
			{name: "pro 3.1 + high", model: google.ChatCompleteModelGemini3_1ProPreview, level: google.ThinkingLevelHigh, wantThoughtTokens: true},
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

	t.Run("can round-trip thought signatures on multi-turn tool use with Gemini 3.5 Flash Lite", func(t *testing.T) {
		// Gemini 3.x returns a `thought_signature` on function-call parts and rejects the
		// follow-up turn with a 400 unless the signature is sent back on the same part —
		// even with no thinking level requested. The model is pinned because the default
		// test model does not enforce this. See https://github.com/maragudk/gai/issues/256.
		assertMultiTurnToolSignatureRoundTrip(t, nil)
	})

	t.Run("can round-trip thought signatures through a serialized history", func(t *testing.T) {
		// The same flow with the history persisted between turns, which is what a caller
		// storing a conversation does. The signature only survives if [gai.PartMetadata]
		// survives the JSON round-trip.
		assertMultiTurnToolSignatureRoundTrip(t, marshalAndUnmarshalMessages)
	})

	t.Run("ignores foreign or absent metadata on thought parts in history", func(t *testing.T) {
		// Message history recorded from another implementation can contain thought parts
		// with that implementation's metadata, or none at all. The client replays the
		// thought text unsigned and ignores the metadata; the request must not error.
		cc := newChatCompleter(t)

		foreignThought := gai.ThoughtPart("the user said hi")
		foreignThought.Metadata = foreignMetadata

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("Hi!"),
				{Role: gai.MessageRoleModel, Parts: []gai.Part{
					gai.ThoughtPart("I should greet the user back"),
					foreignThought,
					gai.TextPart("Hello! How can I help you today?"),
				}},
				gai.NewUserTextMessage("What does the acronym AI stand for? Be brief."),
			},
			Temperature: gai.Ptr(gai.Temperature(0)),
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		for part, err := range res.Parts() {
			is.NotError(t, err)
			if part.Type == gai.PartTypeText {
				output += part.Text()
			}
		}
		is.True(t, strings.Contains(output, "Artificial Intelligence"), output)
	})

	t.Run("surfaces a signature-only response part as a thought part", func(t *testing.T) {
		// Gemini sends parts carrying only a `thought_signature`, with no text and no
		// function call. The signature must still reach the caller, or it cannot be
		// echoed back on the next turn. Served from a local transport, so no API call.
		cc := newStubChatCompleter(t, func(w http.ResponseWriter, r *http.Request) {
			writeStreamChunk(t, w, `{"candidates":[{"content":{"role":"model","parts":[
				{"text":"Hello there."},
				{"thoughtSignature":"c2lnLTEyMw=="}
			]}}]}`)
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{
			Messages: []gai.Message{gai.NewUserTextMessage("Hi!")},
		})
		is.NotError(t, err)

		var types []gai.PartType
		var signatures []string
		for part, err := range res.Parts() {
			is.NotError(t, err)
			types = append(types, part.Type)
			if part.Metadata.Source == metadataSource {
				signatures = append(signatures, string(part.Metadata.Data))
			}
		}

		is.EqualSlice(t, []gai.PartType{gai.PartTypeText, gai.PartTypeThought}, types)
		is.EqualSlice(t, []string{"sig-123"}, signatures)
	})

	t.Run("sends a signed empty thought part in history to the API", func(t *testing.T) {
		// A model turn carrying only a signed empty thought part must reach the wire
		// intact. The genai chat-session history curation drops such a turn — and the
		// user message before it — because it does not recognise a part that carries
		// only a signature, so this client sends the contents list itself.
		var body []byte
		cc := newStubChatCompleter(t, func(w http.ResponseWriter, r *http.Request) {
			var err error
			body, err = io.ReadAll(r.Body)
			is.NotError(t, err)
			writeStreamChunk(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hi!"}]}}]}`)
		})

		signedThought := gai.ThoughtPart("")
		signedThought.Metadata = gai.PartMetadata{Source: metadataSource, Data: []byte("sig-123")}

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("What is in the readme.txt file?"),
				{Role: gai.MessageRoleModel, Parts: []gai.Part{signedThought}},
				gai.NewUserTextMessage("And now?"),
			},
		})
		is.NotError(t, err)
		is.NotError(t, drainParts(t, res))

		sent := unmarshalSentContents(t, body)

		// All three turns must survive, with the signature on the model turn.
		is.Equal(t, 3, len(sent.Contents))
		is.Equal(t, "user", sent.Contents[0].Role)
		is.Equal(t, "model", sent.Contents[1].Role)
		is.Equal(t, "user", sent.Contents[2].Role)
		is.Equal(t, 1, len(sent.Contents[1].Parts))
		is.Equal(t, "sig-123", string(sent.Contents[1].Parts[0].ThoughtSignature))
	})

	t.Run("sends a signature from a serialized history to the API", func(t *testing.T) {
		// A history persisted as JSON and read back must still carry its signatures to
		// the wire, since that is the point of the metadata envelope. Served from a local
		// transport, so no API call.
		var body []byte
		cc := newStubChatCompleter(t, func(w http.ResponseWriter, r *http.Request) {
			var err error
			body, err = io.ReadAll(r.Body)
			is.NotError(t, err)
			writeStreamChunk(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hi!"}]}}]}`)
		})

		signedToolCall := gai.ToolCallPart("call-1", "read_file", json.RawMessage(`{"path":"readme.txt"}`))
		signedToolCall.Metadata = gai.PartMetadata{Source: metadataSource, Data: []byte("sig-123")}

		messages := marshalAndUnmarshalMessages(t, []gai.Message{
			gai.NewUserTextMessage("What is in the readme.txt file?"),
			{Role: gai.MessageRoleModel, Parts: []gai.Part{signedToolCall}},
			gai.NewUserToolResultMessage(gai.ToolResult{ID: "call-1", Name: "read_file", Content: "Hi!\n"}),
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{Messages: messages})
		is.NotError(t, err)
		is.NotError(t, drainParts(t, res))

		sent := unmarshalSentContents(t, body)

		is.Equal(t, 3, len(sent.Contents))
		is.Equal(t, 1, len(sent.Contents[1].Parts))
		is.Equal(t, "sig-123", string(sent.Contents[1].Parts[0].ThoughtSignature))
	})

	t.Run("ignores metadata from another implementation", func(t *testing.T) {
		// Metadata this client did not produce is opaque to it: the part is sent, but
		// nothing of the foreign envelope reaches the wire. Served from a local
		// transport, so no API call.
		var body []byte
		cc := newStubChatCompleter(t, func(w http.ResponseWriter, r *http.Request) {
			var err error
			body, err = io.ReadAll(r.Body)
			is.NotError(t, err)
			writeStreamChunk(t, w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hi!"}]}}]}`)
		})

		foreignText := gai.TextPart("Hello! How can I help you today?")
		foreignText.Metadata = foreignMetadata

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("Hi!"),
				{Role: gai.MessageRoleModel, Parts: []gai.Part{foreignText}},
				gai.NewUserTextMessage("And now?"),
			},
		})
		is.NotError(t, err)
		is.NotError(t, drainParts(t, res))

		sent := unmarshalSentContents(t, body)

		is.Equal(t, 3, len(sent.Contents))
		is.Equal(t, 1, len(sent.Contents[1].Parts))
		is.Equal(t, "Hello! How can I help you today?", sent.Contents[1].Parts[0].Text)
		is.Equal(t, 0, len(sent.Contents[1].Parts[0].ThoughtSignature))
		is.True(t, !strings.Contains(string(body), "example.com/other"), string(body))
	})

	t.Run("errors when the only message has no sendable parts", func(t *testing.T) {
		// An empty thought part with foreign metadata is skipped entirely; a request
		// left with no sendable messages must error cleanly, not panic. This subtest
		// runs without making a network call.
		cc := newChatCompleter(t)

		emptyThought := gai.ThoughtPart("")
		emptyThought.Metadata = foreignMetadata

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				{Role: gai.MessageRoleUser, Parts: []gai.Part{emptyThought}},
			},
		}

		_, err := cc.ChatComplete(t.Context(), req)
		is.True(t, err != nil, "expected an error")
		is.Equal(t, "google: last message has no sendable parts", err.Error())
	})

	t.Run("errors when the last message has no sendable parts", func(t *testing.T) {
		// If only the final message is skipped, sending anyway would silently make the
		// previous message the current turn, so the client must error instead. This
		// subtest runs without making a network call.
		cc := newChatCompleter(t)

		emptyThought := gai.ThoughtPart("")
		emptyThought.Metadata = foreignMetadata

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("Hi!"),
				gai.NewModelTextMessage("Hello! How can I help you today?"),
				{Role: gai.MessageRoleUser, Parts: []gai.Part{emptyThought}},
			},
		}

		_, err := cc.ChatComplete(t.Context(), req)
		is.True(t, err != nil, "expected an error")
		is.Equal(t, "google: last message has no sendable parts", err.Error())
	})

	t.Run("tool choice", func(t *testing.T) {
		weather := gai.Tool{
			Name:        "get_weather",
			Description: "Get the current weather for a city.",
			Schema: gai.ToolSchema{
				Properties: map[string]*gai.Schema{
					"city": {Type: gai.SchemaTypeString, Description: "The city to look up."},
				},
			},
		}

		t.Run("any mode forces a tool call", func(t *testing.T) {
			cc := newChatCompleter(t)

			req := gai.ChatCompleteRequest{
				// A greeting wouldn't normally trigger a tool call; ToolChoiceModeAny forces one.
				Messages:   []gai.Message{gai.NewUserTextMessage("Hello there!")},
				Tools:      []gai.Tool{weather},
				ToolChoice: gai.ToolChoice{Mode: gai.ToolChoiceModeAny},
			}

			res, err := cc.ChatComplete(t.Context(), req)
			is.NotError(t, err)

			var calledTool bool
			for part, err := range res.Parts() {
				is.NotError(t, err)
				if part.Type == gai.PartTypeToolCall {
					calledTool = true
				}
			}
			is.True(t, calledTool, "expected a forced tool call")
		})

		t.Run("tool mode forces the named tool", func(t *testing.T) {
			cc := newChatCompleter(t)

			// Without a forced choice Gemini tends to free-text a fenced JSON block instead of
			// emitting a function call; ToolChoiceModeTool pins it to the named tool. See #269.
			req := gai.ChatCompleteRequest{
				Messages:   []gai.Message{gai.NewUserTextMessage("What is the weather in Paris?")},
				Tools:      []gai.Tool{weather},
				ToolChoice: gai.ToolChoice{Mode: gai.ToolChoiceModeTool, Name: "get_weather"},
			}

			res, err := cc.ChatComplete(t.Context(), req)
			is.NotError(t, err)

			var calledName string
			for part, err := range res.Parts() {
				is.NotError(t, err)
				if part.Type == gai.PartTypeToolCall {
					calledName = part.ToolCall().Name
				}
			}
			is.Equal(t, "get_weather", calledName)
		})

		t.Run("invalid tool choice is rejected before the API call", func(t *testing.T) {
			cc := newChatCompleter(t)

			req := gai.ChatCompleteRequest{
				Messages:   []gai.Message{gai.NewUserTextMessage("Hi!")},
				Tools:      []gai.Tool{weather},
				ToolChoice: gai.ToolChoice{Mode: gai.ToolChoiceModeTool, Name: "missing"},
			}

			_, err := cc.ChatComplete(t.Context(), req)
			is.True(t, err != nil, "expected an error")
			is.Equal(t, `tool choice name "missing" does not match any provided tool`, err.Error())
		})
	})

	t.Run("records standard attributes on the chat-complete span", func(t *testing.T) {
		sr := oteltest.NewSpanRecorder(t)
		cc := newChatCompleter(t)

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{
			System:   gai.Ptr("You are a robot of few words."),
			Messages: []gai.Message{gai.NewUserTextMessage("Reply with a single word.")},
		})
		is.NotError(t, err)
		for _, err := range res.Parts() {
			is.NotError(t, err)
		}

		span := oteltest.FindSpan(t, sr.Ended(), "google.chat_complete")
		is.True(t, oteltest.HasAttribute(span.Attributes(), attribute.String("ai.model", string(google.ChatCompleteModelGemini2_5Flash))))
		is.True(t, oteltest.HasAttribute(span.Attributes(), attribute.Bool("ai.has_system_prompt", true)))
		oteltest.RequireAttributePresent(t, span.Attributes(), "ai.time_to_first_token_ms")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.prompt_tokens")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.completion_tokens")
		oteltest.RequireCacheReadSubsetOfPromptTokens(t, span.Attributes())
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

// newStubChatCompleter builds a [google.ChatCompleter] talking to a local test server
// running handler, so request building and response streaming can be exercised without
// calling the API.
func newStubChatCompleter(t *testing.T, handler http.HandlerFunc) *google.ChatCompleter {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	genaiClient, err := genai.NewClient(t.Context(), &genai.ClientConfig{
		APIKey:      "test",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	is.NotError(t, err)

	c := google.NewClient(google.NewClientOptions{Key: "test"})
	c.Client = genaiClient

	return c.NewChatCompleter(google.NewChatCompleterOptions{Model: google.ChatCompleteModelGemini3_5FlashLite})
}

// writeStreamChunk writes one server-sent event carrying chunk as its JSON payload.
func writeStreamChunk(t *testing.T, w http.ResponseWriter, chunk string) {
	t.Helper()

	w.Header().Set("Content-Type", "text/event-stream")
	_, err := fmt.Fprintf(w, "data: %s\n\n", chunk)
	is.NotError(t, err)
}

// metadataSource is the [gai.PartMetadata] source the client tags its metadata with.
// Spelled out here rather than imported, because it is persisted in message history: a
// change to it must break these tests.
const metadataSource = "maragu.dev/gai/clients/google"

// foreignMetadata stands in for [gai.PartMetadata] set by another implementation.
var foreignMetadata = gai.PartMetadata{Source: "example.com/other", Data: []byte("opaque")}

// marshalAndUnmarshalMessages persists messages as JSON and reads them back, the way a
// caller storing a conversation between turns does.
func marshalAndUnmarshalMessages(t *testing.T, messages []gai.Message) []gai.Message {
	t.Helper()

	data, err := json.Marshal(messages)
	is.NotError(t, err)

	var restored []gai.Message
	is.NotError(t, json.Unmarshal(data, &restored))

	return restored
}

// sentContents is the part of an outgoing request body these tests assert on.
type sentContents struct {
	Contents []struct {
		Role  string `json:"role"`
		Parts []struct {
			Text             string `json:"text"`
			ThoughtSignature []byte `json:"thoughtSignature"`
		} `json:"parts"`
	} `json:"contents"`
}

func unmarshalSentContents(t *testing.T, body []byte) sentContents {
	t.Helper()

	var sent sentContents
	is.NotError(t, json.Unmarshal(body, &sent))

	return sent
}

// assertMultiTurnToolSignatureRoundTrip runs a two-turn tool-use flow on Gemini 3.5 Flash
// Lite: the model calls a tool, and its streamed parts go back as history with the tool
// result. The follow-up turn is rejected with a 400 unless every `thought_signature`
// returns on the part that carried it. When persist is non-nil, the history passes
// through it — a JSON round-trip — on the way back.
func assertMultiTurnToolSignatureRoundTrip(t *testing.T, persist func(*testing.T, []gai.Message) []gai.Message) {
	t.Helper()

	cc := newChatCompleter(t, google.ChatCompleteModelGemini3_5FlashLite)

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

	var parts []gai.Part
	var found, foundSignature bool
	var result gai.ToolResult
	for part, err := range res.Parts() {
		is.NotError(t, err)

		parts = append(parts, part)

		if part.Metadata.Source == metadataSource && len(part.Metadata.Data) > 0 {
			foundSignature = true
		}

		if part.Type != gai.PartTypeToolCall {
			continue
		}
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
	}

	is.True(t, found, "tool not found")
	is.True(t, foundSignature, "should surface a thought signature in part metadata")
	is.Equal(t, "Hi!\n", result.Content)
	is.NotError(t, result.Err)

	req.Messages = []gai.Message{
		gai.NewUserTextMessage("What is in the readme.txt file?"),
		{Role: gai.MessageRoleModel, Parts: parts},
		gai.NewUserToolResultMessage(result),
	}
	if persist != nil {
		req.Messages = persist(t, req.Messages)
	}
	req.System = gai.Ptr("Answer the user's question in a single sentence using the tool result. Do not call any more tools.")

	res, err = cc.ChatComplete(t.Context(), req)
	is.NotError(t, err)

	var output string
	for part, err := range res.Parts() {
		is.NotError(t, err)
		if part.Type == gai.PartTypeText {
			output += part.Text()
		}
	}

	t.Log(output)
	is.True(t, strings.Contains(output, "Hi!"), output)
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
// the default is `gemini-2.5-flash`, which keeps the bulk of the integration tests fast
// and inexpensive. Tests that need a 3.x model pass it explicitly.
func newChatCompleter(t *testing.T, model ...google.ChatCompleteModel) *google.ChatCompleter {
	t.Helper()
	m := google.ChatCompleteModelGemini2_5Flash
	if len(model) > 0 {
		m = model[0]
	}
	c := newClient(t)
	return c.NewChatCompleter(google.NewChatCompleterOptions{Model: m})
}
