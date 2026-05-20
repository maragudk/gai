package anthropic_test

import (
	_ "embed"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/anthropic"
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

		is.True(t, strings.Contains(strings.ToLower(output), "hi") || strings.Contains(strings.ToLower(output), "hello"), output)
		is.True(t, strings.Contains(strings.ToLower(output), "help") || strings.Contains(strings.ToLower(output), "assist"), output)

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
		is.True(t, strings.Contains(strings.ToLower(output), "artificial intelligence"), output)
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

		// Anthropic may provide explanatory text before tool calls
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

		is.True(t, strings.Contains(strings.ToLower(output), "readme") && strings.Contains(output, "Hi!"), output)
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

		// Anthropic may provide explanatory text before tool calls
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

		is.True(t, strings.Contains(strings.ToLower(output), "bonjour"), output)
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

	t.Run("can describe a PDF", func(t *testing.T) {
		cc := newChatCompleter(t)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserDataMessage("application/pdf", pdf),
			},
			System:      gai.Ptr("Describe the contents of this PDF concisely."),
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

	t.Run("panics on unsupported MIME type", func(t *testing.T) {
		cc := newChatCompleter(t)

		defer func() {
			r := recover()
			is.True(t, r != nil)
			is.Equal(t, "unsupported MIME type for Anthropic: audio/wav", r)
		}()

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserDataMessage("audio/wav", []byte("fake audio")),
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

	// Thinking-level matrix. Each row exercises a real (model, level) combination so the
	// per-client `ThinkingLevel` mapping is grounded in live API behaviour. The rows below
	// are pinned to empirical probe results: which models support adaptive thinking, which
	// effort levels each accepts, and (where stable across multiple runs) whether the model
	// emits `PartTypeThought` blocks at that level.
	t.Run("adaptive thinking matrix", func(t *testing.T) {
		tests := []struct {
			name            string
			model           anthropic.ChatCompleteModel
			level           gai.ThinkingLevel
			wantErr         bool
			requireThoughts bool // strict: assert thoughtParts > 0
		}{
			// Sonnet 4.6 supports adaptive thinking up to "high"; xhigh is rejected.
			// "low" is non-deterministic — the model decides whether to surface a thinking
			// block; we don't strictly assert. Medium/high/max are reliably thoughtful.
			{name: "sonnet 4.6 + low", model: anthropic.ChatCompleteModelClaudeSonnet4_6Latest, level: anthropic.ThinkingLevelLow},
			{name: "sonnet 4.6 + medium", model: anthropic.ChatCompleteModelClaudeSonnet4_6Latest, level: anthropic.ThinkingLevelMedium, requireThoughts: true},
			{name: "sonnet 4.6 + high", model: anthropic.ChatCompleteModelClaudeSonnet4_6Latest, level: anthropic.ThinkingLevelHigh, requireThoughts: true},
			{name: "sonnet 4.6 + xhigh rejected", model: anthropic.ChatCompleteModelClaudeSonnet4_6Latest, level: anthropic.ThinkingLevelXHigh, wantErr: true},
			{name: "sonnet 4.6 + max", model: anthropic.ChatCompleteModelClaudeSonnet4_6Latest, level: anthropic.ThinkingLevelMax, requireThoughts: true},

			// Opus 4.7 accepts every level including xhigh, but `ThinkingDelta` events
			// don't reliably arrive on the streaming path at any level: the non-streaming
			// `Messages.New` API returns `thinking` blocks at max, yet the equivalent
			// streaming run yields zero `ThinkingDelta`s. Treated as "no strict assertion"
			// here — the rows still confirm the call doesn't error.
			{name: "opus 4.7 + low", model: anthropic.ChatCompleteModelClaudeOpus4_7Latest, level: anthropic.ThinkingLevelLow},
			{name: "opus 4.7 + medium", model: anthropic.ChatCompleteModelClaudeOpus4_7Latest, level: anthropic.ThinkingLevelMedium},
			{name: "opus 4.7 + high", model: anthropic.ChatCompleteModelClaudeOpus4_7Latest, level: anthropic.ThinkingLevelHigh},
			{name: "opus 4.7 + xhigh", model: anthropic.ChatCompleteModelClaudeOpus4_7Latest, level: anthropic.ThinkingLevelXHigh},
			{name: "opus 4.7 + max", model: anthropic.ChatCompleteModelClaudeOpus4_7Latest, level: anthropic.ThinkingLevelMax},

			// Older 4.x models: adaptive thinking is not supported. The API returns
			// `400 adaptive thinking is not supported on this model` for all levels.
			// Haiku 4.5 confirms the rejection; Sonnet 4.5 confirms it on the older mid-tier.
			{name: "haiku 4.5 rejects adaptive", model: anthropic.ChatCompleteModelClaudeHaiku4_5Latest, level: anthropic.ThinkingLevelMedium, wantErr: true},
			{name: "sonnet 4.5 rejects adaptive", model: anthropic.ChatCompleteModelClaudeSonnet4_5Latest, level: anthropic.ThinkingLevelMedium, wantErr: true},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				cc := newChatCompleter(t, test.model)

				req := gai.ChatCompleteRequest{
					Messages: []gai.Message{
						gai.NewUserTextMessage("Solve step by step: a farmer has 17 sheep, all but 9 die. How many remain?"),
					},
					MaxCompletionTokens: gai.Ptr(4096),
					ThinkingLevel:       gai.Ptr(test.level),
				}

				res, err := cc.ChatComplete(t.Context(), req)
				if test.wantErr {
					// Anthropic surfaces level/capability rejections during the streaming
					// pass: the constructor returns nil error but the Parts iterator yields
					// the API error on the first read.
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
				// Anthropic does not separately count thinking tokens in the SDK Usage
				// struct; they're bundled into OutputTokens. So we don't assert on
				// res.Meta.Usage.ThoughtsTokens here.
				t.Logf("thoughtParts=%d textParts=%d", thoughtParts, textParts)
			})
		}
	})

	t.Run("rejects inbound PartTypeThought as deferred", func(t *testing.T) {
		// Multi-turn round-trip of the per-block signature is tracked by
		// https://github.com/maragudk/gai/issues/250. Until that lands, the client
		// returns a typed error rather than silently dropping the part.
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

	t.Run("panics on unsupported thinking level", func(t *testing.T) {
		// The Anthropic client publishes Low/Medium/High/XHigh/Max. Anything outside
		// that set must panic at the boundary, not silently round-trip to the API.
		tests := []struct {
			name  string
			level gai.ThinkingLevel
		}{
			{name: "minimal not published", level: gai.ThinkingLevel("minimal")},
			{name: "arbitrary string", level: gai.ThinkingLevel("none-i-mean-nothing")},
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

// newChatCompleter builds a [anthropic.ChatCompleter] for tests. With no model argument,
// the default is `claude-haiku-4-5` — the cheapest current model, which keeps the bulk of
// the integration tests fast and inexpensive. Tests that need a specific capability
// (Sonnet 4.6 adaptive thinking, Opus 4.7 xhigh effort, etc.) pass the model explicitly.
func newChatCompleter(t *testing.T, model ...anthropic.ChatCompleteModel) *anthropic.ChatCompleter {
	t.Helper()
	m := anthropic.ChatCompleteModelClaudeHaiku4_5Latest
	if len(model) > 0 {
		m = model[0]
	}
	c := newClient(t)
	return c.NewChatCompleter(anthropic.NewChatCompleterOptions{Model: m})
}
