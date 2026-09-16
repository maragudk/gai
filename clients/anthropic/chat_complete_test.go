package anthropic_test

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

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"go.opentelemetry.io/otel/attribute"
	"maragu.dev/is"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/anthropic"
	"maragu.dev/gai/internal/oteltest"
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

	t.Run("can round-trip signed thinking blocks on multi-turn tool use with thinking enabled", func(t *testing.T) {
		// With thinking enabled and tools in play, the API requires the signed thinking
		// blocks from the previous assistant turn back verbatim. Sonnet 4.6 is pinned
		// because it reliably streams thinking blocks at high effort — but only when the
		// request actually needs reasoning: adaptive thinking skips thought entirely on
		// a plain "read this file" ask, so the prompt includes a puzzle to reason about
		// before the tool call. See https://github.com/maragudk/gai/issues/250.
		cc := newChatCompleter(t, anthropic.ChatCompleteModelClaudeSonnet4_6Latest)

		root, err := os.OpenRoot("testdata")
		is.NotError(t, err)

		question := "Solve step by step: a farmer has 17 sheep, all but 9 die. How many remain? Then read the readme.txt file and report its content."

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage(question),
			},
			ThinkingLevel: gai.Ptr(anthropic.ThinkingLevelHigh),
			Tools: []gai.Tool{
				tools.NewReadFile(root),
			},
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		parts, result, foundTool, foundSignature := collectToolUseParts(t, res, req.Tools)

		var thoughtParts int
		for _, part := range parts {
			if part.Type == gai.PartTypeThought {
				thoughtParts++
			}
		}
		t.Logf("thoughtParts=%d foundSignature=%v", thoughtParts, foundSignature)

		is.True(t, foundTool, "tool not found")
		is.True(t, foundSignature, "should surface a thinking-block signature in part metadata")
		is.Equal(t, "Hi!\n", result.Content)
		is.NotError(t, result.Err)

		req.Messages = []gai.Message{
			gai.NewUserTextMessage(question),
			{Role: gai.MessageRoleModel, Parts: parts},
			gai.NewUserToolResultMessage(result),
		}
		req.System = gai.Ptr("Answer the user's question briefly using the tool result. Do not call any more tools.")

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
	})

	t.Run("can resend streamed parts on a model that thinks unprompted", func(t *testing.T) {
		// Sonnet 5 streams thinking blocks with no thinking level requested at all, so the
		// plain collect-streamed-parts-and-resend flow must round-trip them. The model is
		// pinned because the default test model does not think unprompted. Note: no
		// Temperature here; it is deprecated on the Claude 5 line.
		assertUnpromptedThinkingResend(t, nil)
	})

	t.Run("can resend streamed parts through a serialized history", func(t *testing.T) {
		// The same flow with the history persisted between turns, which is what a caller
		// storing a conversation does. The signed thinking blocks only survive if
		// [gai.PartMetadata] survives the JSON round-trip.
		assertUnpromptedThinkingResend(t, marshalAndUnmarshalMessages)
	})

	t.Run("can round-trip redacted thinking blocks", func(t *testing.T) {
		// This magic string is documented by Anthropic to force a redacted thinking
		// block, so the round-trip of the opaque payload can be tested deliberately. See
		// https://docs.claude.com/en/docs/build-with-claude/extended-thinking.
		const trigger = "ANTHROPIC_MAGIC_STRING_TRIGGER_REDACTED_THINKING_46C9A13E193C177646C7398A98432ECCCE4C1253D5E2D82641AC0E52CC2876CB"

		cc := newChatCompleter(t, anthropic.ChatCompleteModelClaudeSonnet4_6Latest)

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage(trigger),
			},
			ThinkingLevel: gai.Ptr(anthropic.ThinkingLevelHigh),
		}

		res, err := cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var parts []gai.Part
		var foundRedacted bool
		for part, err := range res.Parts() {
			is.NotError(t, err)
			parts = append(parts, part)
			if decodeMetadata(t, part.Metadata).RedactedThinkingData != "" {
				foundRedacted = true
			}
		}
		is.True(t, foundRedacted, "should surface redacted thinking data in part metadata")

		req.Messages = append(req.Messages,
			gai.Message{Role: gai.MessageRoleModel, Parts: parts},
			gai.NewUserTextMessage("What does the acronym AI stand for? Be brief."),
		)

		res, err = cc.ChatComplete(t.Context(), req)
		is.NotError(t, err)

		var output string
		for part, err := range res.Parts() {
			is.NotError(t, err)
			if part.Type == gai.PartTypeText {
				output += part.Text()
			}
		}
		is.True(t, strings.Contains(strings.ToLower(output), "artificial intelligence"), output)
	})

	t.Run("ignores thought parts with foreign or absent metadata in history", func(t *testing.T) {
		// Message history recorded from another implementation can contain thought parts
		// with that implementation's metadata, or none at all. The API rejects unsigned
		// thinking blocks, so the client drops such parts silently rather than erroring.
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
		is.True(t, strings.Contains(strings.ToLower(output), "artificial intelligence"), output)
	})

	t.Run("sends a signature from a serialized history to the API", func(t *testing.T) {
		// A history persisted as JSON and read back must still carry its thinking-block
		// signature and redacted-thinking payload to the wire, since that is the point of
		// the metadata envelope. Served from a local transport, so no API call.
		var body []byte
		cc := newStubChatCompleter(t, func(w http.ResponseWriter, r *http.Request) {
			var err error
			body, err = io.ReadAll(r.Body)
			is.NotError(t, err)
			writeMessageStream(t, w)
		})

		signedThought := gai.ThoughtPart("")
		signedThought.Metadata = newMetadata(t, "sig-123", "")
		redactedThought := gai.ThoughtPart("")
		redactedThought.Metadata = newMetadata(t, "", "redacted-123")

		messages := marshalAndUnmarshalMessages(t, []gai.Message{
			gai.NewUserTextMessage("Hi!"),
			{Role: gai.MessageRoleModel, Parts: []gai.Part{
				gai.ThoughtPart("the user said hi"),
				signedThought,
				redactedThought,
				gai.TextPart("Hello! How can I help you today?"),
			}},
			gai.NewUserTextMessage("And now?"),
		})

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{Messages: messages})
		is.NotError(t, err)
		is.NotError(t, drainParts(t, res))

		sent := unmarshalSentMessages(t, body)

		is.Equal(t, 3, len(sent.Messages))
		is.Equal(t, 3, len(sent.Messages[1].Content))
		is.Equal(t, "thinking", sent.Messages[1].Content[0].Type)
		is.Equal(t, "the user said hi", sent.Messages[1].Content[0].Thinking)
		is.Equal(t, "sig-123", sent.Messages[1].Content[0].Signature)
		is.Equal(t, "redacted_thinking", sent.Messages[1].Content[1].Type)
		is.Equal(t, "redacted-123", sent.Messages[1].Content[1].Data)
		is.Equal(t, "text", sent.Messages[1].Content[2].Type)
	})

	t.Run("ignores metadata from another implementation", func(t *testing.T) {
		// Metadata this client did not produce is opaque to it: the thought part it sits
		// on is dropped like any unsigned one, and nothing of the foreign envelope reaches
		// the wire. Served from a local transport, so no API call.
		var body []byte
		cc := newStubChatCompleter(t, func(w http.ResponseWriter, r *http.Request) {
			var err error
			body, err = io.ReadAll(r.Body)
			is.NotError(t, err)
			writeMessageStream(t, w)
		})

		foreignThought := gai.ThoughtPart("the user said hi")
		foreignThought.Metadata = foreignMetadata
		// Metadata tagged as this client's own but carrying nothing is an absence, not a
		// corruption, so it is ignored too.
		emptyThought := gai.ThoughtPart("and then")
		emptyThought.Metadata = gai.PartMetadata{Source: metadataSource}

		res, err := cc.ChatComplete(t.Context(), gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("Hi!"),
				{Role: gai.MessageRoleModel, Parts: []gai.Part{
					foreignThought,
					emptyThought,
					gai.TextPart("Hello! How can I help you today?"),
				}},
				gai.NewUserTextMessage("And now?"),
			},
		})
		is.NotError(t, err)
		is.NotError(t, drainParts(t, res))

		sent := unmarshalSentMessages(t, body)

		is.Equal(t, 3, len(sent.Messages))
		is.Equal(t, 1, len(sent.Messages[1].Content))
		is.Equal(t, "text", sent.Messages[1].Content[0].Type)
		is.True(t, !strings.Contains(string(body), "example.com/other"), string(body))
		is.True(t, !strings.Contains(string(body), "the user said hi"), string(body))
	})

	t.Run("errors on its own metadata that cannot be decoded", func(t *testing.T) {
		// Metadata tagged as this client's own but with corrupt bytes — a hand-edited or
		// truncated history — is caller data, so it fails at the boundary with a typed
		// error instead of silently dropping the thinking block. This subtest runs
		// without making a network call.
		cc := newChatCompleter(t)

		corruptThought := gai.ThoughtPart("the user said hi")
		corruptThought.Metadata = gai.PartMetadata{Source: metadataSource, Data: []byte("not json")}

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				{Role: gai.MessageRoleUser, Parts: []gai.Part{corruptThought}},
			},
		}

		_, err := cc.ChatComplete(t.Context(), req)
		is.True(t, err != nil, "expected an error")
		is.True(t, strings.HasPrefix(err.Error(), "anthropic: part metadata is not decodable: "), err.Error())
	})

	t.Run("errors when the only message has no sendable parts", func(t *testing.T) {
		// A thought part with foreign metadata is dropped entirely; a request left with
		// no sendable messages must error cleanly, not send empty content. This subtest
		// runs without making a network call.
		cc := newChatCompleter(t)

		foreignThought := gai.ThoughtPart("the user said hi")
		foreignThought.Metadata = foreignMetadata

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				{Role: gai.MessageRoleUser, Parts: []gai.Part{foreignThought}},
			},
		}

		_, err := cc.ChatComplete(t.Context(), req)
		is.True(t, err != nil, "expected an error")
		is.Equal(t, "anthropic: last message has no sendable parts", err.Error())
	})

	t.Run("errors when the last message has no sendable parts", func(t *testing.T) {
		// If only the final message is dropped, sending anyway would make the previous
		// model message the final turn — a silent prefill continuation — so the client
		// must error instead. This subtest runs without making a network call.
		cc := newChatCompleter(t)

		foreignThought := gai.ThoughtPart("the user said hi")
		foreignThought.Metadata = foreignMetadata

		req := gai.ChatCompleteRequest{
			Messages: []gai.Message{
				gai.NewUserTextMessage("Hi!"),
				gai.NewModelTextMessage("Hello! How can I help you today?"),
				{Role: gai.MessageRoleUser, Parts: []gai.Part{foreignThought}},
			},
		}

		_, err := cc.ChatComplete(t.Context(), req)
		is.True(t, err != nil, "expected an error")
		is.Equal(t, "anthropic: last message has no sendable parts", err.Error())
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

	// This also serves as a regression test for a previous bug where
	// message = anthropic.Message{} on every ContentBlockStopEvent wiped
	// message.Usage mid-stream, leaving ai.prompt_tokens at zero.
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

		span := oteltest.FindSpan(t, sr.Ended(), "anthropic.chat_complete")
		is.True(t, oteltest.HasAttribute(span.Attributes(), attribute.String("ai.model", string(anthropic.ChatCompleteModelClaudeHaiku4_5Latest))))
		is.True(t, oteltest.HasAttribute(span.Attributes(), attribute.Bool("ai.has_system_prompt", true)))
		oteltest.RequireAttributePresent(t, span.Attributes(), "ai.time_to_first_token_ms")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.prompt_tokens")
		oteltest.RequirePositiveIntAttribute(t, span.Attributes(), "ai.completion_tokens")
		oteltest.RequireAttributePresent(t, span.Attributes(), "ai.cache_creation_tokens")
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

// collectToolUseParts consumes the response stream of a tool-use turn, executing each
// matching tool call against tools. It returns all streamed parts, the last tool result,
// whether a tool call was found, and whether any part carried metadata with a
// thinking-block signature.
func collectToolUseParts(t *testing.T, res gai.ChatCompleteResponse, tools []gai.Tool) (parts []gai.Part, result gai.ToolResult, foundTool, foundSignature bool) {
	t.Helper()

	for part, err := range res.Parts() {
		is.NotError(t, err)

		parts = append(parts, part)

		if decodeMetadata(t, part.Metadata).Signature != "" {
			foundSignature = true
		}

		if part.Type != gai.PartTypeToolCall {
			continue
		}
		toolCall := part.ToolCall()
		for _, tool := range tools {
			if tool.Name == toolCall.Name {
				foundTool = true
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

	return parts, result, foundTool, foundSignature
}

// metadataSource is the [gai.PartMetadata] source the client tags its metadata with, and
// metadata mirrors the JSON it encodes into the envelope. Both are spelled out here
// rather than imported, because they are persisted in message history: a change to either
// must break these tests.
const metadataSource = "maragu.dev/gai/clients/anthropic"

type metadata struct {
	Signature            string `json:"signature,omitempty"`
	RedactedThinkingData string `json:"redactedThinkingData,omitempty"`
}

// foreignMetadata stands in for [gai.PartMetadata] set by another implementation.
var foreignMetadata = gai.PartMetadata{Source: "example.com/other", Data: []byte("opaque")}

// newMetadata builds the envelope the client would put on a thought part.
func newMetadata(t *testing.T, signature, redactedThinkingData string) gai.PartMetadata {
	t.Helper()

	data, err := json.Marshal(metadata{Signature: signature, RedactedThinkingData: redactedThinkingData})
	is.NotError(t, err)

	return gai.PartMetadata{Source: metadataSource, Data: data}
}

// decodeMetadata returns the metadata the client encoded into m, or the zero value if m
// holds none or holds another implementation's.
func decodeMetadata(t *testing.T, m gai.PartMetadata) metadata {
	t.Helper()

	if m.Source != metadataSource {
		return metadata{}
	}

	var decoded metadata
	is.NotError(t, json.Unmarshal(m.Data, &decoded))

	return decoded
}

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

// sentMessages is the part of an outgoing request body these tests assert on.
type sentMessages struct {
	Messages []struct {
		Role    string `json:"role"`
		Content []struct {
			Type      string `json:"type"`
			Text      string `json:"text"`
			Thinking  string `json:"thinking"`
			Signature string `json:"signature"`
			Data      string `json:"data"`
		} `json:"content"`
	} `json:"messages"`
}

func unmarshalSentMessages(t *testing.T, body []byte) sentMessages {
	t.Helper()

	var sent sentMessages
	is.NotError(t, json.Unmarshal(body, &sent))

	return sent
}

// newStubChatCompleter builds an [anthropic.ChatCompleter] talking to a local test server
// running handler, so request building can be exercised without calling the API.
func newStubChatCompleter(t *testing.T, handler http.HandlerFunc) *anthropic.ChatCompleter {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c := anthropic.NewClient(anthropic.NewClientOptions{Key: "test"})
	c.Client = anthropicsdk.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL))

	return c.NewChatCompleter(anthropic.NewChatCompleterOptions{Model: anthropic.ChatCompleteModelClaudeSonnet5Latest})
}

// writeMessageStream writes the shortest valid message stream: one text block saying Hi!.
func writeMessageStream(t *testing.T, w http.ResponseWriter) {
	t.Helper()

	w.Header().Set("Content-Type", "text/event-stream")
	for _, event := range []string{
		`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi!"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`,
		`{"type":"message_stop"}`,
	} {
		_, err := fmt.Fprintf(w, "data: %s\n\n", event)
		is.NotError(t, err)
	}
}

// assertUnpromptedThinkingResend runs a two-turn tool-use flow on Sonnet 5, which streams
// thinking blocks with no thinking level requested at all: the model calls a tool, and its
// streamed parts go back as history with the tool result. The API rejects the follow-up
// turn unless each thinking block returns with its signature and full text. When persist
// is non-nil, the history passes through it — a JSON round-trip — on the way back. Note:
// no Temperature anywhere here; it is deprecated on the Claude 5 line.
func assertUnpromptedThinkingResend(t *testing.T, persist func(*testing.T, []gai.Message) []gai.Message) {
	t.Helper()

	cc := newChatCompleter(t, anthropic.ChatCompleteModelClaudeSonnet5Latest)

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

	parts, result, foundTool, foundSignature := collectToolUseParts(t, res, req.Tools)

	is.True(t, foundTool, "tool not found")
	is.Equal(t, "Hi!\n", result.Content)
	is.NotError(t, result.Err)

	// Unprompted thinking is the model's own choice, so don't require it — but any
	// thought parts that did stream must end in a signed one for the resend to work.
	var thoughtParts int
	for _, part := range parts {
		if part.Type == gai.PartTypeThought {
			thoughtParts++
		}
	}
	if thoughtParts > 0 {
		is.True(t, foundSignature, "streamed thoughts should carry a signature in part metadata")
	}
	t.Logf("thoughtParts=%d foundSignature=%v", thoughtParts, foundSignature)

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
