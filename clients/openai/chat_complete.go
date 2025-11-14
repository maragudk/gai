package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"maragu.dev/gai"
)

type ChatCompleteModel string

const (
	ChatCompleteModelGPT4o     = ChatCompleteModel(openai.ChatModelGPT4o)
	ChatCompleteModelGPT4oMini = ChatCompleteModel(openai.ChatModelGPT4oMini)
)

type ChatCompleter struct {
	Client openai.Client
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
		tracer: otel.Tracer("maragu.dev/gai-openai"),
	}
}

// ChatComplete satisfies [gai.ChatCompleter].
func (c *ChatCompleter) ChatComplete(ctx context.Context, req gai.ChatCompleteRequest) (gai.ChatCompleteResponse, error) {
	ctx, span := c.tracer.Start(ctx, "openai.chat_complete",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("ai.model", string(c.model)),
			attribute.Int("ai.message_count", len(req.Messages)),
		),
	)

	var messages []openai.ChatCompletionMessageParamUnion

	if req.System != nil {
		messages = append(messages, openai.SystemMessage(*req.System))
		span.SetAttributes(
			attribute.Bool("ai.has_system_prompt", true),
			attribute.String("ai.system_prompt", *req.System),
		)
	}

	for _, m := range req.Messages {
		switch m.Role {
		case gai.MessageRoleUser:
			var parts []openai.ChatCompletionContentPartUnionParam

			for _, part := range m.Parts {
				switch part.Type {
				case gai.MessagePartTypeText:
					parts = append(parts, openai.ChatCompletionContentPartUnionParam{
						OfText: &openai.ChatCompletionContentPartTextParam{Text: part.Text()},
					})

				case gai.MessagePartTypeToolResult:
					// Even though this is just a part, we append to messages directly

					// Take existing parts and append to messages first
					if len(parts) > 0 {
						messages = append(messages, openai.UserMessage(parts))
					}
					parts = nil

					toolResult := part.ToolResult()
					content := toolResult.Content
					if toolResult.Err != nil {
						content = fmt.Sprintf("Error: %s", toolResult.Err)
					}
					messages = append(messages, openai.ToolMessage(content, toolResult.ID))
					continue

				default:
					panic("not implemented")
				}
			}

			if len(parts) > 0 {
				messages = append(messages, openai.UserMessage(parts))
			}

		case gai.MessageRoleModel:
			var parts []openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion

			for _, part := range m.Parts {
				switch part.Type {
				case gai.MessagePartTypeText:
					parts = append(parts, openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{
						OfText: &openai.ChatCompletionContentPartTextParam{Text: part.Text()},
					})

				case gai.MessagePartTypeToolCall:
					// Even though this is just a part, we append to messages directly

					// Take existing parts and append to messages first
					if len(parts) > 0 {
						messages = append(messages, openai.AssistantMessage(parts))
					}
					parts = nil

					toolCall := part.ToolCall()
					messages = append(messages, openai.ChatCompletionMessageParamUnion{
						OfAssistant: &openai.ChatCompletionAssistantMessageParam{
							ToolCalls: []openai.ChatCompletionMessageToolCallParam{
								{
									ID: toolCall.ID,
									Function: openai.ChatCompletionMessageToolCallFunctionParam{
										Name:      toolCall.Name,
										Arguments: string(toolCall.Args),
									},
								},
							},
						},
					})
					continue

				default:
					panic("not implemented")
				}
			}

			if len(parts) > 0 {
				messages = append(messages, openai.AssistantMessage(parts))
			}

		default:
			panic("unknown role " + m.Role)
		}
	}

	var tools []openai.ChatCompletionToolParam
	var toolNames []string
	for _, tool := range req.Tools {
		tools = append(tools, openai.ChatCompletionToolParam{
			Function: openai.FunctionDefinitionParam{
				Name:        tool.Name,
				Description: openai.String(tool.Description),
				Parameters: openai.FunctionParameters{
					"type":       "object",
					"properties": normalizeToolSchemaProperties(tool.Schema.Properties),
				},
			},
		})
		toolNames = append(toolNames, tool.Name)
	}
	sort.Strings(toolNames)
	span.SetAttributes(
		attribute.Int("ai.tool_count", len(tools)),
		attribute.StringSlice("ai.tools", toolNames),
	)

	params := openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    openai.ChatModel(c.model),
		Tools:    tools,
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		},
	}

	if req.Temperature != nil {
		params.Temperature = openai.Opt(req.Temperature.Float64())
		span.SetAttributes(attribute.Float64("ai.temperature", req.Temperature.Float64()))
	}

	if req.ResponseSchema != nil {
		normalized := normalizeToolSchema(req.ResponseSchema)
		jsonSchemaObject := schemaToJSONObject(normalized)
		jsonSchema := shared.ResponseFormatJSONSchemaJSONSchemaParam{
			Name:   responseSchemaName(req.ResponseSchema),
			Strict: openai.Bool(true),
			Schema: jsonSchemaObject,
		}
		if normalized.Description != "" {
			jsonSchema.Description = openai.String(normalized.Description)
		}

		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				JSONSchema: jsonSchema,
			},
		}

		span.SetAttributes(attribute.Bool("ai.has_response_schema", true))
	}

	stream := c.Client.Chat.Completions.NewStreaming(ctx, params)

	meta := &gai.ChatCompleteResponseMetadata{}

	res := gai.NewChatCompleteResponse(func(yield func(gai.MessagePart, error) bool) {
		defer span.End()

		defer func() {
			if err := stream.Close(); err != nil {
				c.log.Info("Error closing stream", "error", err)
			}
		}()

		var acc openai.ChatCompletionAccumulator
		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			if len(chunk.Choices) > 0 {
				if reason := chunk.Choices[0].FinishReason; reason != "" {
					mapped := mapChatFinishReason(reason)
					if meta.FinishReason == nil || *meta.FinishReason != mapped {
						meta.FinishReason = gai.Ptr(mapped)
					}
					span.SetAttributes(attribute.String("ai.finish_reason", string(mapped)))
				}
			}

			if _, ok := acc.JustFinishedContent(); !ok {
				if toolCall, ok := acc.JustFinishedToolCall(); ok {
					if !yield(gai.ToolCallPart(toolCall.ID, toolCall.Name, json.RawMessage(toolCall.Arguments)), nil) {
						return
					}
					continue
				}

				if refusal, ok := acc.JustFinishedRefusal(); ok {
					err := fmt.Errorf("refusal: %v", refusal)
					meta.FinishReason = gai.Ptr(gai.ChatCompleteFinishReasonRefusal)
					span.SetAttributes(attribute.String("ai.finish_reason", string(gai.ChatCompleteFinishReasonRefusal)))
					span.RecordError(err)
					span.SetStatus(codes.Error, "model refused request")
					yield(gai.MessagePart{}, err)
					return
				}

				if len(chunk.Choices) > 0 {
					if !yield(gai.TextMessagePart(chunk.Choices[0].Delta.Content), nil) {
						return
					}
				}
			}

			if chunk.Usage.PromptTokens == 0 {
				continue
			}

			meta.Usage = gai.ChatCompleteResponseUsage{
				PromptTokens:     int(chunk.Usage.PromptTokens),
				CompletionTokens: int(chunk.Usage.CompletionTokens),
			}
			span.SetAttributes(
				attribute.Int("ai.prompt_tokens", int(chunk.Usage.PromptTokens)),
				attribute.Int("ai.completion_tokens", int(chunk.Usage.CompletionTokens)),
				attribute.Int("ai.total_tokens", int(chunk.Usage.TotalTokens)),
			)
		}

		if meta.FinishReason == nil && len(acc.Choices) > 0 {
			if reason := acc.Choices[0].FinishReason; reason != "" {
				mapped := mapChatFinishReason(reason)
				meta.FinishReason = gai.Ptr(mapped)
				span.SetAttributes(attribute.String("ai.finish_reason", string(mapped)))
			}
		}

		if err := stream.Err(); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "stream error")
			yield(gai.MessagePart{}, err)
		}
	})

	res.Meta = meta

	return res, nil
}

// normalizeToolSchemaProperties recursively normalizes schema properties for OpenAI compatibility
func normalizeToolSchemaProperties(properties map[string]*gai.Schema) map[string]*gai.Schema {
	if len(properties) == 0 {
		return properties
	}

	result := make(map[string]*gai.Schema)
	for key, schema := range properties {
		result[key] = normalizeToolSchema(schema)
	}
	return result
}

// normalizeToolSchema creates a normalized copy of a gai.Schema with lowercase type names
func normalizeToolSchema(schema *gai.Schema) *gai.Schema {
	if schema == nil {
		return nil
	}

	// Create a copy of the schema
	normalized := &gai.Schema{
		AnyOf:            schema.AnyOf,
		Default:          schema.Default,
		Description:      schema.Description,
		Enum:             schema.Enum,
		Example:          schema.Example,
		Format:           schema.Format,
		Items:            normalizeToolSchema(schema.Items),
		MaxItems:         schema.MaxItems,
		Maximum:          schema.Maximum,
		MinItems:         schema.MinItems,
		Minimum:          schema.Minimum,
		Properties:       normalizeToolSchemaProperties(schema.Properties),
		PropertyOrdering: schema.PropertyOrdering,
		Required:         schema.Required,
		Title:            schema.Title,
		Type:             gai.SchemaType(strings.ToLower(string(schema.Type))),
	}

	// Recursively normalize anyOf schemas
	if len(schema.AnyOf) > 0 {
		normalized.AnyOf = make([]*gai.Schema, len(schema.AnyOf))
		for i, s := range schema.AnyOf {
			normalized.AnyOf[i] = normalizeToolSchema(s)
		}
	}

	return normalized
}

func schemaToJSONObject(schema *gai.Schema) map[string]any {
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

	ensureObjectSchemasDisallowAdditionalProperties(obj)
	return obj
}

func ensureObjectSchemasDisallowAdditionalProperties(obj map[string]any) {
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
					ensureObjectSchemasDisallowAdditionalProperties(child)
				}
			}
		}
	}

	if items, ok := obj["items"].(map[string]any); ok {
		ensureObjectSchemasDisallowAdditionalProperties(items)
	} else if itemsArr, ok := obj["items"].([]any); ok {
		for _, v := range itemsArr {
			if child, ok := v.(map[string]any); ok {
				ensureObjectSchemasDisallowAdditionalProperties(child)
			}
		}
	}

	if anyOf, ok := obj["anyOf"].([]any); ok {
		for _, v := range anyOf {
			if child, ok := v.(map[string]any); ok {
				ensureObjectSchemasDisallowAdditionalProperties(child)
			}
		}
	}
}

func responseSchemaName(schema *gai.Schema) string {
	name := schema.Title
	if name == "" {
		name = "response"
	}

	const maxLen = 64
	var b strings.Builder
	b.Grow(len(name))

	for _, r := range name {
		if b.Len() >= maxLen {
			break
		}

		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('_')
		default:
			// Skip unsupported characters
		}
	}

	if b.Len() == 0 {
		return "response"
	}

	return b.String()
}

func mapChatFinishReason(reason string) gai.ChatCompleteFinishReason {
	switch reason {
	case string(openai.CompletionChoiceFinishReasonStop):
		return gai.ChatCompleteFinishReasonStop
	case string(openai.CompletionChoiceFinishReasonLength):
		return gai.ChatCompleteFinishReasonLength
	case string(openai.CompletionChoiceFinishReasonContentFilter):
		return gai.ChatCompleteFinishReasonContentFilter
	case "tool_calls", "function_call":
		return gai.ChatCompleteFinishReasonToolCalls
	default:
		return gai.ChatCompleteFinishReasonUnknown
	}
}

var _ gai.ChatCompleter = (*ChatCompleter)(nil)
