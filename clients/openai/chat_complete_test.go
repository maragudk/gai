package openai_test

import (
	_ "embed"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/openai"
	"maragu.dev/gai/tools"
)

//go:embed testdata/logo.jpg
var image []byte

//go:embed testdata/hello.pdf
var pdf []byte

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
			case gai.PartTypeText:
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
			case gai.PartTypeText:
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

		requireContainsAll(t, output, "hi")
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
		is.Equal(t, `["hello.pdf","logo.jpg","readme.txt"]`, result.Content)
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
			case gai.PartTypeText:
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
			System: gai.Ptr("You always respond in French."),
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

		// Accept either "bonjour" (formal) or "salut" (informal). GPT-5-nano picks either
		// depending on its read of the register, and both satisfy the "respond in French" intent.
		requireContainsAny(t, output, "bonjour", "salut")
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
			if part.Type == gai.PartTypeText {
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

	t.Run("can describe an image", func(t *testing.T) {
		cc := newChatCompleter(t)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserDataMessage("image/jpeg", image),
			},
			System: gai.Ptr("Describe this image concisely."),
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

	t.Run("panics on unsupported MIME type", func(t *testing.T) {
		cc := newChatCompleter(t)

		defer func() {
			r := recover()
			is.True(t, r != nil)
			is.Equal(t, "unsupported MIME type for OpenAI: application/pdf", r)
		}()

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserDataMessage("application/pdf", pdf),
			},
		}
		_, _ = cc.ChatComplete(t.Context(), req)
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

	// Reasoning-effort matrix. Each row exercises a real (model, level) combination so the
	// per-client `ThinkingLevel` mapping is grounded in live API behaviour. The matrix
	// matches the per-model GoDoc bullet list on `clients/openai`'s ThinkingLevel constants:
	// each row either confirms acceptance or confirms the documented 400 from the API.
	//
	// `wantThoughtTokens: true` is asserted only where probe shows the model reliably
	// returns reasoning tokens at that level. OpenAI Chat Completions never streams
	// reasoning text, so `PartTypeThought` parts are never expected.
	t.Run("reasoning effort matrix", func(t *testing.T) {
		tests := []struct {
			name              string
			model             openai.ChatCompleteModel
			level             gai.ThinkingLevel
			wantErr           bool
			wantThoughtTokens bool
		}{
			// gpt-5: minimal/low/medium/high accepted, none and xhigh rejected.
			{name: "gpt-5 + none rejected", model: openai.ChatCompleteModelGPT5, level: gai.ThinkingLevelNone, wantErr: true},
			{name: "gpt-5 + minimal", model: openai.ChatCompleteModelGPT5, level: openai.ThinkingLevelMinimal},
			{name: "gpt-5 + low", model: openai.ChatCompleteModelGPT5, level: openai.ThinkingLevelLow},
			{name: "gpt-5 + medium", model: openai.ChatCompleteModelGPT5, level: openai.ThinkingLevelMedium},
			{name: "gpt-5 + high", model: openai.ChatCompleteModelGPT5, level: openai.ThinkingLevelHigh},
			{name: "gpt-5 + xhigh rejected", model: openai.ChatCompleteModelGPT5, level: openai.ThinkingLevelXHigh, wantErr: true},

			// gpt-5-mini: same matrix as gpt-5.
			{name: "gpt-5-mini + none rejected", model: openai.ChatCompleteModelGPT5Mini, level: gai.ThinkingLevelNone, wantErr: true},
			{name: "gpt-5-mini + minimal", model: openai.ChatCompleteModelGPT5Mini, level: openai.ThinkingLevelMinimal},
			{name: "gpt-5-mini + low", model: openai.ChatCompleteModelGPT5Mini, level: openai.ThinkingLevelLow},
			{name: "gpt-5-mini + medium", model: openai.ChatCompleteModelGPT5Mini, level: openai.ThinkingLevelMedium},
			{name: "gpt-5-mini + high", model: openai.ChatCompleteModelGPT5Mini, level: openai.ThinkingLevelHigh},
			{name: "gpt-5-mini + xhigh rejected", model: openai.ChatCompleteModelGPT5Mini, level: openai.ThinkingLevelXHigh, wantErr: true},

			// gpt-5-nano: same matrix as gpt-5.
			{name: "gpt-5-nano + none rejected", model: openai.ChatCompleteModelGPT5Nano, level: gai.ThinkingLevelNone, wantErr: true},
			{name: "gpt-5-nano + minimal", model: openai.ChatCompleteModelGPT5Nano, level: openai.ThinkingLevelMinimal},
			{name: "gpt-5-nano + low", model: openai.ChatCompleteModelGPT5Nano, level: openai.ThinkingLevelLow},
			{name: "gpt-5-nano + medium", model: openai.ChatCompleteModelGPT5Nano, level: openai.ThinkingLevelMedium},
			{name: "gpt-5-nano + high", model: openai.ChatCompleteModelGPT5Nano, level: openai.ThinkingLevelHigh},
			{name: "gpt-5-nano + xhigh rejected", model: openai.ChatCompleteModelGPT5Nano, level: openai.ThinkingLevelXHigh, wantErr: true},

			// gpt-5.1: none/low/medium/high accepted, minimal and xhigh rejected.
			{name: "gpt-5.1 + none", model: openai.ChatCompleteModelGPT5_1, level: gai.ThinkingLevelNone},
			{name: "gpt-5.1 + minimal rejected", model: openai.ChatCompleteModelGPT5_1, level: openai.ThinkingLevelMinimal, wantErr: true},
			{name: "gpt-5.1 + low", model: openai.ChatCompleteModelGPT5_1, level: openai.ThinkingLevelLow},
			{name: "gpt-5.1 + medium", model: openai.ChatCompleteModelGPT5_1, level: openai.ThinkingLevelMedium},
			{name: "gpt-5.1 + high", model: openai.ChatCompleteModelGPT5_1, level: openai.ThinkingLevelHigh},
			{name: "gpt-5.1 + xhigh rejected", model: openai.ChatCompleteModelGPT5_1, level: openai.ThinkingLevelXHigh, wantErr: true},

			// gpt-5.1-mini intentionally omitted: the model is in the SDK enum but not
			// accessible with our test API key (404 model_not_found). The matrix is the
			// same as gpt-5.1 above per OpenAI's docs; no level-mapping signal is lost
			// by skipping it.

			// gpt-5.2: none/low/medium/high/xhigh accepted, minimal rejected.
			{name: "gpt-5.2 + none", model: openai.ChatCompleteModelGPT5_2, level: gai.ThinkingLevelNone},
			{name: "gpt-5.2 + minimal rejected", model: openai.ChatCompleteModelGPT5_2, level: openai.ThinkingLevelMinimal, wantErr: true},
			{name: "gpt-5.2 + low", model: openai.ChatCompleteModelGPT5_2, level: openai.ThinkingLevelLow},
			{name: "gpt-5.2 + medium", model: openai.ChatCompleteModelGPT5_2, level: openai.ThinkingLevelMedium},
			{name: "gpt-5.2 + high", model: openai.ChatCompleteModelGPT5_2, level: openai.ThinkingLevelHigh},
			{name: "gpt-5.2 + xhigh", model: openai.ChatCompleteModelGPT5_2, level: openai.ThinkingLevelXHigh, wantThoughtTokens: true},

			// gpt-5.3-chat-latest: chat-tuned, only `medium` accepted; every other level
			// rejected with `Supported values are: 'medium'`.
			{name: "gpt-5.3-chat-latest + none rejected", model: openai.ChatCompleteModelGPT5_3ChatLatest, level: gai.ThinkingLevelNone, wantErr: true},
			{name: "gpt-5.3-chat-latest + minimal rejected", model: openai.ChatCompleteModelGPT5_3ChatLatest, level: openai.ThinkingLevelMinimal, wantErr: true},
			{name: "gpt-5.3-chat-latest + low rejected", model: openai.ChatCompleteModelGPT5_3ChatLatest, level: openai.ThinkingLevelLow, wantErr: true},
			{name: "gpt-5.3-chat-latest + medium", model: openai.ChatCompleteModelGPT5_3ChatLatest, level: openai.ThinkingLevelMedium},
			{name: "gpt-5.3-chat-latest + high rejected", model: openai.ChatCompleteModelGPT5_3ChatLatest, level: openai.ThinkingLevelHigh, wantErr: true},
			{name: "gpt-5.3-chat-latest + xhigh rejected", model: openai.ChatCompleteModelGPT5_3ChatLatest, level: openai.ThinkingLevelXHigh, wantErr: true},

			// gpt-5.4: same matrix as gpt-5.2 — none/low/medium/high/xhigh accepted.
			{name: "gpt-5.4 + none", model: openai.ChatCompleteModelGPT5_4, level: gai.ThinkingLevelNone},
			{name: "gpt-5.4 + minimal rejected", model: openai.ChatCompleteModelGPT5_4, level: openai.ThinkingLevelMinimal, wantErr: true},
			{name: "gpt-5.4 + low", model: openai.ChatCompleteModelGPT5_4, level: openai.ThinkingLevelLow},
			{name: "gpt-5.4 + medium", model: openai.ChatCompleteModelGPT5_4, level: openai.ThinkingLevelMedium, wantThoughtTokens: true},
			{name: "gpt-5.4 + high", model: openai.ChatCompleteModelGPT5_4, level: openai.ThinkingLevelHigh, wantThoughtTokens: true},
			{name: "gpt-5.4 + xhigh", model: openai.ChatCompleteModelGPT5_4, level: openai.ThinkingLevelXHigh, wantThoughtTokens: true},

			// gpt-5.4-mini: same matrix as gpt-5.4.
			{name: "gpt-5.4-mini + none", model: openai.ChatCompleteModelGPT5_4Mini, level: gai.ThinkingLevelNone},
			{name: "gpt-5.4-mini + minimal rejected", model: openai.ChatCompleteModelGPT5_4Mini, level: openai.ThinkingLevelMinimal, wantErr: true},
			{name: "gpt-5.4-mini + low", model: openai.ChatCompleteModelGPT5_4Mini, level: openai.ThinkingLevelLow},
			{name: "gpt-5.4-mini + medium", model: openai.ChatCompleteModelGPT5_4Mini, level: openai.ThinkingLevelMedium},
			{name: "gpt-5.4-mini + high", model: openai.ChatCompleteModelGPT5_4Mini, level: openai.ThinkingLevelHigh, wantThoughtTokens: true},
			{name: "gpt-5.4-mini + xhigh", model: openai.ChatCompleteModelGPT5_4Mini, level: openai.ThinkingLevelXHigh, wantThoughtTokens: true},

			// gpt-5.4-nano: same matrix; nano is much more reluctant to reason than the
			// full-size 5.4 model — probe returned 0 reasoning tokens at low/medium and
			// flaky non-zero at high. Only assert thought tokens at xhigh, where reasoning
			// is reliable.
			{name: "gpt-5.4-nano + none", model: openai.ChatCompleteModelGPT5_4Nano, level: gai.ThinkingLevelNone},
			{name: "gpt-5.4-nano + minimal rejected", model: openai.ChatCompleteModelGPT5_4Nano, level: openai.ThinkingLevelMinimal, wantErr: true},
			{name: "gpt-5.4-nano + low", model: openai.ChatCompleteModelGPT5_4Nano, level: openai.ThinkingLevelLow},
			{name: "gpt-5.4-nano + medium", model: openai.ChatCompleteModelGPT5_4Nano, level: openai.ThinkingLevelMedium},
			{name: "gpt-5.4-nano + high", model: openai.ChatCompleteModelGPT5_4Nano, level: openai.ThinkingLevelHigh},
			{name: "gpt-5.4-nano + xhigh", model: openai.ChatCompleteModelGPT5_4Nano, level: openai.ThinkingLevelXHigh, wantThoughtTokens: true},

			// gpt-5.5: frontier model, same matrix as 5.4. Reasons more eagerly than 5.4.
			{name: "gpt-5.5 + none", model: openai.ChatCompleteModelGPT5_5, level: gai.ThinkingLevelNone},
			{name: "gpt-5.5 + minimal rejected", model: openai.ChatCompleteModelGPT5_5, level: openai.ThinkingLevelMinimal, wantErr: true},
			{name: "gpt-5.5 + low", model: openai.ChatCompleteModelGPT5_5, level: openai.ThinkingLevelLow, wantThoughtTokens: true},
			{name: "gpt-5.5 + medium", model: openai.ChatCompleteModelGPT5_5, level: openai.ThinkingLevelMedium, wantThoughtTokens: true},
			{name: "gpt-5.5 + high", model: openai.ChatCompleteModelGPT5_5, level: openai.ThinkingLevelHigh, wantThoughtTokens: true},
			{name: "gpt-5.5 + xhigh", model: openai.ChatCompleteModelGPT5_5, level: openai.ThinkingLevelXHigh, wantThoughtTokens: true},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				cc := newChatCompleter(t, test.model)

				req := gai.ChatCompleteRequest{
					Messages: []gai.Message{
						gai.NewUserTextMessage("Solve step by step: a farmer has 17 sheep, all but 9 die. How many remain?"),
					},
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

				var output string
				for part, partErr := range res.Parts() {
					is.NotError(t, partErr)
					switch part.Type {
					case gai.PartTypeText:
						output += part.Text()
					case gai.PartTypeThought:
						t.Fatal("OpenAI Chat Completions should not stream PartTypeThought")
					default:
						t.Fatalf("unexpected part type %s", part.Type)
					}
				}
				is.True(t, len(output) > 0, "should have output")
				if test.wantThoughtTokens {
					is.True(t, res.Meta.Usage.ThoughtsTokens > 0, "thoughts tokens should be populated")
				}
				t.Logf("thoughtsTokens=%d outputLen=%d", res.Meta.Usage.ThoughtsTokens, len(output))
			})
		}
	})

	t.Run("panics on unsupported thinking level", func(t *testing.T) {
		// The OpenAI client publishes Minimal/Low/Medium/High/XHigh. Anything outside
		// that set must panic at the boundary, not silently round-trip to the API.
		tests := []struct {
			name  string
			level gai.ThinkingLevel
		}{
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
				ToolChoice: &gai.ToolChoice{Mode: gai.ToolChoiceModeAny},
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

			req := gai.ChatCompleteRequest{
				Messages:   []gai.Message{gai.NewUserTextMessage("What is the weather in Paris?")},
				Tools:      []gai.Tool{weather},
				ToolChoice: &gai.ToolChoice{Mode: gai.ToolChoiceModeTool, Name: "get_weather"},
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
				ToolChoice: &gai.ToolChoice{Mode: gai.ToolChoiceModeTool, Name: "missing"},
			}

			_, err := cc.ChatComplete(t.Context(), req)
			is.True(t, err != nil, "expected an error")
			is.Equal(t, `tool choice name "missing" does not match any provided tool`, err.Error())
		})
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

// newChatCompleter builds an [openai.ChatCompleter] for tests. With no model argument,
// the default is `gpt-5-nano` — the cheapest current model, which keeps the bulk of the
// integration tests fast and inexpensive. Tests that need a specific capability (gpt-5.4
// reasoning, gpt-5.5 frontier behaviour, gpt-5.3-chat-latest medium-only quirk, etc.)
// pass the model explicitly.
func newChatCompleter(t *testing.T, model ...openai.ChatCompleteModel) *openai.ChatCompleter {
	t.Helper()
	m := openai.ChatCompleteModelGPT5Nano
	if len(model) > 0 {
		m = model[0]
	}
	c := newClient(t)
	return c.NewChatCompleter(openai.NewChatCompleterOptions{Model: m})
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
