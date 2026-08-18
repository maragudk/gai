package anthropic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"maragu.dev/gai"
)

// PartMetadata carried on [gai.Part] values produced by [ChatCompleter.ChatComplete],
// implementing [gai.PartMetadata]. When such a part is passed back as message history,
// the client re-emits the metadata so the API accepts the follow-up turn.
//
// The client streams a thinking block as its text deltas first, each a plain
// [gai.PartTypeThought] part, followed by one final empty [gai.PartTypeThought] part
// whose metadata carries the block's Signature. A redacted thinking block is a single
// empty [gai.PartTypeThought] part whose metadata carries RedactedThinkingData. Passing
// all streamed parts back as history in order reassembles the original blocks. Thought
// parts without usable metadata of this package — from other providers, or built by
// hand — are omitted from requests entirely, because the API rejects unsigned thinking
// blocks.
type PartMetadata struct {
	// Signature of a thinking block, which the API requires back verbatim with the
	// block's full text on the next turn of a tool-use flow. See
	// https://docs.claude.com/en/docs/build-with-claude/extended-thinking.
	Signature string
	// RedactedThinkingData is the opaque payload of a redacted thinking block, which the
	// API likewise requires back verbatim on the next turn.
	RedactedThinkingData string
}

// PartMetadata satisfies [gai.PartMetadata].
func (PartMetadata) PartMetadata() {}

// errLastMessageEmpty is returned when the last message of a request has no parts the
// client can send — for example only thought parts without usable [PartMetadata], which
// are dropped. Sending the request anyway would make the previous message the final
// turn, silently changing what the model responds to, so the client rejects it instead.
var errLastMessageEmpty = errors.New("last message has no sendable parts")

// ChatCompleteModel is an Anthropic Claude model identifier accepted by the
// chat-completions surface. See https://platform.claude.com/docs/en/about-claude/models/overview
// for the full list and the current availability and capability matrix of each model.
type ChatCompleteModel string

// The model constants below are hand-curated: stable, generally-available models of the
// current and recent generations, with previews included case-by-case. Dated snapshots,
// modality variants, and models that cannot work through the client's implemented API
// surface (e.g. Responses-API-only) are excluded, and models killed server-side are
// removed immediately. The set is enforced by TestModelConformance and its ignore list.
const (
	ChatCompleteModelClaudeHaiku4_5Latest  = ChatCompleteModel(anthropic.ModelClaudeHaiku4_5)
	ChatCompleteModelClaudeSonnet4_5Latest = ChatCompleteModel(anthropic.ModelClaudeSonnet4_5)
	ChatCompleteModelClaudeOpus4_5Latest   = ChatCompleteModel(anthropic.ModelClaudeOpus4_5)
	ChatCompleteModelClaudeSonnet4_6Latest = ChatCompleteModel(anthropic.ModelClaudeSonnet4_6)
	ChatCompleteModelClaudeOpus4_6Latest   = ChatCompleteModel(anthropic.ModelClaudeOpus4_6)
	ChatCompleteModelClaudeOpus4_7Latest   = ChatCompleteModel(anthropic.ModelClaudeOpus4_7)
	ChatCompleteModelClaudeOpus4_8Latest   = ChatCompleteModel(anthropic.ModelClaudeOpus4_8)
	ChatCompleteModelClaudeFable5Latest    = ChatCompleteModel(anthropic.ModelClaudeFable5)
	ChatCompleteModelClaudeSonnet5Latest   = ChatCompleteModel(anthropic.ModelClaudeSonnet5)
	ChatCompleteModelClaudeOpus5Latest     = ChatCompleteModel(anthropic.ModelClaudeOpus5)
)

// Per-client [gai.ThinkingLevel] constants. These map onto the `output_config.effort` enum
// used by Sonnet 4.6 / Opus 4.6 / Opus 4.7. The API expects two coupled fields for adaptive
// thinking — `thinking.type=adaptive` enables thinking, `output_config.effort` sets the
// level — so non-`None` levels populate both. There is no Minimal: the Anthropic enum starts
// at Low. XHigh is currently Opus-4.7-only; Sonnet 4.6 and Opus 4.6 reject it with a 400.
// Pass [gai.ThinkingLevelNone] to opt out of thinking entirely (no fields set). Levels not
// in this list panic at the client boundary.
const (
	// ThinkingLevelLow applies low reasoning effort.
	ThinkingLevelLow gai.ThinkingLevel = "low"
	// ThinkingLevelMedium applies medium reasoning effort.
	ThinkingLevelMedium gai.ThinkingLevel = "medium"
	// ThinkingLevelHigh applies high reasoning effort.
	ThinkingLevelHigh gai.ThinkingLevel = "high"
	// ThinkingLevelXHigh applies extra-high reasoning effort. Opus 4.7+ only.
	ThinkingLevelXHigh gai.ThinkingLevel = "xhigh"
	// ThinkingLevelMax applies maximum reasoning effort.
	ThinkingLevelMax gai.ThinkingLevel = "max"
)

type ChatCompleter struct {
	Client anthropic.Client
	log    *slog.Logger
	model  ChatCompleteModel
	tracer trace.Tracer
}

type NewChatCompleterOptions struct {
	Model ChatCompleteModel
}

func (c *Client) NewChatCompleter(opts NewChatCompleterOptions) *ChatCompleter {
	return &ChatCompleter{
		Client: c.Client,
		log:    c.log,
		model:  opts.Model,
		tracer: otel.Tracer("maragu.dev/gai/clients/anthropic"),
	}
}

// ChatComplete satisfies [gai.ChatCompleter].
func (c *ChatCompleter) ChatComplete(ctx context.Context, req gai.ChatCompleteRequest) (gai.ChatCompleteResponse, error) {
	ctx, span := c.tracer.Start(ctx, "anthropic.chat_complete",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("ai.model", string(c.model)),
			attribute.Int("ai.message_count", len(req.Messages)),
		),
	)

	if len(req.Messages) == 0 {
		panic("no messages")
	}

	if err := req.ToolChoice.Validate(req.Tools); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid tool choice")
		span.End()
		return gai.ChatCompleteResponse{}, err
	}

	var messages []anthropic.MessageParam
	var lastMessageSent bool
	for _, m := range req.Messages {
		var parts []anthropic.ContentBlockParamUnion

		// Thought parts stream as plain text fragments terminated by a part whose
		// [PartMetadata] carries the block signature (see [PartMetadata]); buffer the
		// fragments and emit one signed thinking block per group. Fragment runs never
		// terminated by usable metadata — thoughts from other providers, or hand-built
		// ones — are dropped silently: the API rejects unsigned thinking blocks outright,
		// so a history replayed across providers would otherwise always error over
		// context the API refuses anyway.
		var pendingThinking strings.Builder

		for _, part := range m.Parts {
			if part.Type != gai.PartTypeThought {
				// A thinking block is a contiguous run of thought parts; any other part
				// type ends the run, discarding unsigned fragments.
				pendingThinking.Reset()
			}

			switch part.Type {
			case gai.PartTypeText:
				parts = append(parts, anthropic.ContentBlockParamUnion{
					OfText: &anthropic.TextBlockParam{
						Text: part.Text(),
					},
				})

			case gai.PartTypeThought:
				pendingThinking.WriteString(part.Thought())
				if md, ok := part.Metadata.(PartMetadata); ok {
					switch {
					case md.RedactedThinkingData != "":
						parts = append(parts, anthropic.NewRedactedThinkingBlock(md.RedactedThinkingData))
					case md.Signature != "":
						parts = append(parts, anthropic.NewThinkingBlock(md.Signature, pendingThinking.String()))
					}
					// Any metadata of this package ends the run, so a zero value drops
					// its unsigned text rather than bleeding it into the next block.
					pendingThinking.Reset()
				}

			case gai.PartTypeToolCall:
				toolCall := part.ToolCall()
				parts = append(parts, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    toolCall.ID,
						Name:  toolCall.Name,
						Input: toolCall.Args,
					},
				})

			case gai.PartTypeToolResult:
				toolResult := part.ToolResult()
				content := toolResult.Content
				var isError bool
				if toolResult.Err != nil {
					isError = true
					content = toolResult.Err.Error()
				}
				parts = append(parts, anthropic.ContentBlockParamUnion{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: toolResult.ID,
						Content: []anthropic.ToolResultBlockParamContentUnion{
							{
								OfText: &anthropic.TextBlockParam{
									Text: content,
								},
							},
						},
						IsError: anthropic.Bool(isError),
					},
				})

			case gai.PartTypeData:
				if part.MIMEType == "" {
					panic("data part has empty MIME type")
				}
				if len(part.Data) == 0 {
					panic("data part has empty data")
				}
				encoded := base64.StdEncoding.EncodeToString(part.Data)

				switch {
				case strings.HasPrefix(part.MIMEType, "image/"):
					parts = append(parts, anthropic.ContentBlockParamUnion{
						OfImage: &anthropic.ImageBlockParam{
							Source: anthropic.ImageBlockParamSourceUnion{
								OfBase64: &anthropic.Base64ImageSourceParam{
									Data:      encoded,
									MediaType: anthropic.Base64ImageSourceMediaType(part.MIMEType),
								},
							},
						},
					})

				case part.MIMEType == "application/pdf":
					parts = append(parts, anthropic.ContentBlockParamUnion{
						OfDocument: &anthropic.DocumentBlockParam{
							Source: anthropic.DocumentBlockParamSourceUnion{
								OfBase64: &anthropic.Base64PDFSourceParam{
									Data: encoded,
								},
							},
						},
					})

				default:
					panic("unsupported MIME type for Anthropic: " + part.MIMEType)
				}

			default:
				panic("unknown part type " + string(part.Type))
			}
		}

		var role anthropic.MessageParamRole
		switch m.Role {
		case gai.MessageRoleUser:
			role = anthropic.MessageParamRoleUser
		case gai.MessageRoleModel:
			role = anthropic.MessageParamRoleAssistant
		default:
			panic("unknown role " + m.Role)
		}

		// A message whose parts were all dropped would reach the API as empty content
		// and be rejected, so skip the whole message instead.
		lastMessageSent = len(parts) > 0
		if lastMessageSent {
			messages = append(messages, anthropic.MessageParam{
				Content: parts,
				Role:    role,
			})
		}
	}

	// If the final message lost all its parts to dropping, the previous message would
	// silently become the final turn, so reject the request instead.
	if !lastMessageSent {
		err := fmt.Errorf("anthropic: %w", errLastMessageEmpty)
		span.RecordError(err)
		span.SetStatus(codes.Error, "last message empty")
		span.End()
		return gai.ChatCompleteResponse{}, err
	}

	var tools []anthropic.ToolUnionParam
	var toolNames []string
	for _, tool := range req.Tools {
		tools = append(tools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        tool.Name,
				Description: anthropic.String(tool.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: tool.Schema.Properties,
				},
			},
		})
		toolNames = append(toolNames, tool.Name)
	}
	sort.Strings(toolNames)
	span.SetAttributes(
		attribute.Int("ai.tool_count", len(req.Tools)),
		attribute.StringSlice("ai.tools", toolNames),
	)

	var temperature param.Opt[float64]
	if req.Temperature != nil {
		temperature = param.NewOpt(req.Temperature.Float64())
		span.SetAttributes(attribute.Float64("ai.temperature", req.Temperature.Float64()))
	}

	var system []anthropic.TextBlockParam
	if req.System != nil {
		system = []anthropic.TextBlockParam{
			{
				Text: *req.System,
			},
		}
		span.SetAttributes(attribute.Bool("ai.has_system_prompt", true))
	}

	maxTokens := 16_384
	if req.MaxCompletionTokens != nil {
		maxTokens = *req.MaxCompletionTokens
	}
	span.SetAttributes(attribute.Int("ai.max_completion_tokens", maxTokens))

	params := anthropic.MessageNewParams{
		MaxTokens:   int64(maxTokens),
		Messages:    messages,
		Model:       anthropic.Model(c.model),
		System:      system,
		Temperature: temperature,
		Tools:       tools,
	}

	switch req.ToolChoice.Mode {
	case gai.ToolChoiceModeAny:
		params.ToolChoice = anthropic.ToolChoiceUnionParam{OfAny: &anthropic.ToolChoiceAnyParam{}}
		span.SetAttributes(attribute.String("ai.tool_choice", string(req.ToolChoice.Mode)))
	case gai.ToolChoiceModeTool:
		params.ToolChoice = anthropic.ToolChoiceParamOfTool(req.ToolChoice.Name)
		span.SetAttributes(attribute.String("ai.tool_choice", string(req.ToolChoice.Mode)))
	}

	if req.ResponseSchema != nil {
		params.OutputConfig.Format = anthropic.JSONOutputFormatParam{
			Schema: schemaToMap(req.ResponseSchema),
		}
		span.SetAttributes(attribute.Bool("ai.has_response_schema", true))
	}

	if req.ThinkingLevel != nil {
		switch *req.ThinkingLevel {
		case gai.ThinkingLevelNone:
			// Off: no Thinking field, no Effort. Adaptive thinking is opt-in.
		case ThinkingLevelLow:
			params.Thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}
			params.OutputConfig.Effort = anthropic.OutputConfigEffortLow
		case ThinkingLevelMedium:
			params.Thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}
			params.OutputConfig.Effort = anthropic.OutputConfigEffortMedium
		case ThinkingLevelHigh:
			params.Thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}
			params.OutputConfig.Effort = anthropic.OutputConfigEffortHigh
		case ThinkingLevelXHigh:
			params.Thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}
			params.OutputConfig.Effort = anthropic.OutputConfigEffortXhigh
		case ThinkingLevelMax:
			params.Thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}
			params.OutputConfig.Effort = anthropic.OutputConfigEffortMax
		default:
			panic("unsupported thinking level: " + string(*req.ThinkingLevel))
		}
		span.SetAttributes(attribute.String("ai.thinking_level", string(*req.ThinkingLevel)))
	}

	stream := c.Client.Messages.NewStreaming(ctx, params)

	streamStart := time.Now()
	var firstTokenRecorded bool
	recordFirstToken := func() {
		if firstTokenRecorded {
			return
		}
		firstTokenRecorded = true
		span.SetAttributes(attribute.Int64("ai.time_to_first_token_ms", time.Since(streamStart).Milliseconds()))
	}

	return gai.NewChatCompleteResponse(func(yield func(gai.Part, error) bool) {
		defer span.End()

		defer func() {
			if err := stream.Close(); err != nil {
				c.log.Info("Error closing stream", "error", err)
			}
		}()

		var message anthropic.Message
		defer func() {
			// ai.prompt_tokens is normalised to include cache tokens, matching
			// OpenAI's PromptTokens and Google's PromptTokenCount semantics, so
			// ai.cache_read_tokens is always a subset of ai.prompt_tokens.
			span.SetAttributes(
				attribute.Int("ai.prompt_tokens", int(message.Usage.InputTokens+message.Usage.CacheReadInputTokens+message.Usage.CacheCreationInputTokens)),
				attribute.Int("ai.completion_tokens", int(message.Usage.OutputTokens)),
				attribute.Int("ai.cache_read_tokens", int(message.Usage.CacheReadInputTokens)),
				attribute.Int("ai.cache_creation_tokens", int(message.Usage.CacheCreationInputTokens)),
			)
		}()

		for stream.Next() {
			event := stream.Current()

			if err := message.Accumulate(event); err != nil {
				// A hack to circumvent a bug, see https://github.com/anthropics/anthropic-sdk-go/issues/164
				if !strings.Contains(err.Error(), "unexpected end of JSON input") {
					span.RecordError(err)
					span.SetStatus(codes.Error, "message accumulation failed")
					yield(gai.Part{}, fmt.Errorf("error accumulating message: %w", err))
					return
				}
			}

			switch event := event.AsAny().(type) {
			case anthropic.ContentBlockStartEvent:
				recordFirstToken()

			case anthropic.ContentBlockDeltaEvent:
				switch delta := event.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					if !yield(gai.TextPart(delta.Text), nil) {
						return
					}
				case anthropic.ThinkingDelta:
					if !yield(gai.ThoughtPart(delta.Thinking), nil) {
						return
					}
				}

			case anthropic.ContentBlockStopEvent:
				// Yield the tool call for the block that just stopped. Index into
				// message.Content directly rather than scanning the whole slice, so
				// each block is yielded exactly once and the accumulator's content
				// slice stays intact: the SDK's Accumulate requires each
				// ContentBlockStartEvent index to equal len(message.Content), so
				// clearing the slice mid-stream breaks the next start event.
				if event.Index < 0 || int(event.Index) >= len(message.Content) {
					continue
				}
				switch block := message.Content[event.Index].AsAny().(type) {
				case anthropic.ThinkingBlock:
					// The block's signature only arrives in the trailing signature delta,
					// after the text deltas above have already been yielded, so it rides
					// on a final empty thought part. See [PartMetadata] for how the
					// request builder reassembles the block from these parts.
					thoughtPart := gai.ThoughtPart("")
					thoughtPart.Metadata = PartMetadata{Signature: block.Signature}
					if !yield(thoughtPart, nil) {
						return
					}

				case anthropic.RedactedThinkingBlock:
					// Redacted thinking has no readable text, only an opaque payload the
					// API requires back verbatim, so it surfaces as a single empty thought
					// part carrying the payload in its metadata.
					thoughtPart := gai.ThoughtPart("")
					thoughtPart.Metadata = PartMetadata{RedactedThinkingData: block.Data}
					if !yield(thoughtPart, nil) {
						return
					}

				case anthropic.ToolUseBlock:
					c.log.Debug("Tool call", "id", block.ID, "name", block.Name, "input", block.Input)
					var found bool
					for _, tool := range req.Tools {
						if tool.Name == block.Name {
							found = true
							if !yield(gai.ToolCallPart(block.ID, block.Name, block.Input), nil) {
								return
							}
						}
					}
					if !found {
						span.RecordError(fmt.Errorf("tool not found: %s", block.Name))
						span.SetStatus(codes.Error, "tool not found")
						yield(gai.Part{}, fmt.Errorf("tool not found: %s", block.Name))
						return
					}
				}
			}
		}

		if stream.Err() != nil {
			span.RecordError(stream.Err())
			span.SetStatus(codes.Error, "stream error")
			yield(gai.Part{}, stream.Err())
		}
	}), nil
}

// schemaToMap converts a gai.Schema to a map[string]any for the Anthropic API.
func schemaToMap(schema *gai.Schema) map[string]any {
	if schema == nil {
		return nil
	}

	data, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}

	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		panic(err)
	}

	ensureAdditionalPropertiesFalse(obj)
	removeUnsupportedFields(obj)
	return obj
}

// ensureAdditionalPropertiesFalse recursively sets additionalProperties to false
// on all object-type schemas, as required by the Anthropic structured output API.
func ensureAdditionalPropertiesFalse(obj map[string]any) {
	if obj == nil {
		return
	}

	if t, ok := obj["type"].(string); ok && t == "object" {
		if _, ok := obj["additionalProperties"]; !ok {
			obj["additionalProperties"] = false
		}
		if props, ok := obj["properties"].(map[string]any); ok {
			for _, v := range props {
				if child, ok := v.(map[string]any); ok {
					ensureAdditionalPropertiesFalse(child)
				}
			}
		}
	}

	if items, ok := obj["items"].(map[string]any); ok {
		ensureAdditionalPropertiesFalse(items)
	}

	if anyOf, ok := obj["anyOf"].([]any); ok {
		for _, v := range anyOf {
			if child, ok := v.(map[string]any); ok {
				ensureAdditionalPropertiesFalse(child)
			}
		}
	}
}

// removeUnsupportedFields recursively removes fields not supported by the Anthropic schema API.
func removeUnsupportedFields(obj map[string]any) {
	if obj == nil {
		return
	}

	delete(obj, "propertyOrdering")

	if props, ok := obj["properties"].(map[string]any); ok {
		for _, v := range props {
			if child, ok := v.(map[string]any); ok {
				removeUnsupportedFields(child)
			}
		}
	}

	if items, ok := obj["items"].(map[string]any); ok {
		removeUnsupportedFields(items)
	}

	if anyOf, ok := obj["anyOf"].([]any); ok {
		for _, v := range anyOf {
			if child, ok := v.(map[string]any); ok {
				removeUnsupportedFields(child)
			}
		}
	}
}

var _ gai.ChatCompleter = (*ChatCompleter)(nil)
