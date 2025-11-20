package google

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/genai"

	"maragu.dev/gai"
	"maragu.dev/gai/clients/google/internal/schema"
)

type ChatCompleteModel string

const (
	ChatCompleteModelGemini2_0Flash     = ChatCompleteModel("models/gemini-2.0-flash")
	ChatCompleteModelGemini2_5Flash     = ChatCompleteModel("models/gemini-2.5-flash")
	ChatCompleteModelGemini2_5FlashLite = ChatCompleteModel("models/gemini-2.5-flash-lite")
	ChatCompleteModelGemini2_5Pro       = ChatCompleteModel("models/gemini-2.5-pro")
)

type ChatCompleter struct {
	Client *genai.Client
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
		tracer: otel.Tracer("maragu.dev/gai-google"),
	}
}

func (c *ChatCompleter) ChatComplete(ctx context.Context, req gai.ChatCompleteRequest) (gai.ChatCompleteResponse, error) {
	ctx, span := c.tracer.Start(ctx, "google.chat_complete",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("ai.model", string(c.model)),
			attribute.Int("ai.message_count", len(req.Messages)),
		),
	)

	if len(req.Messages) == 0 {
		panic("no messages")
	}

	if req.Messages[len(req.Messages)-1].Role != gai.MessageRoleUser {
		panic("last message must have user role")
	}

	var config genai.GenerateContentConfig
	if req.Temperature != nil {
		config.Temperature = gai.Ptr(float32(*req.Temperature))
		span.SetAttributes(attribute.Float64("ai.temperature", float64(*req.Temperature)))
	}
	if req.System != nil {
		config.SystemInstruction = genai.NewContentFromText(*req.System, genai.RoleUser)
		span.SetAttributes(attribute.Bool("ai.has_system_prompt", true))
		span.SetAttributes(attribute.String("ai.system_prompt", *req.System))
	}
	if req.MaxCompletionTokens != nil {
		config.MaxOutputTokens = int32(*req.MaxCompletionTokens)
		span.SetAttributes(attribute.Int("ai.max_completion_tokens", *req.MaxCompletionTokens))
	}

	if len(req.Tools) > 0 {
		tools, err := schema.ConvertTools(req.Tools)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "tool conversion failed")
			return gai.ChatCompleteResponse{}, fmt.Errorf("error converting tools: %w", err)
		}
		config.Tools = tools

		// Extract and sort tool names for tracing
		var toolNames []string
		for _, tool := range req.Tools {
			toolNames = append(toolNames, tool.Name)
		}
		sort.Strings(toolNames)
		span.SetAttributes(
			attribute.Int("ai.tool_count", len(req.Tools)),
			attribute.StringSlice("ai.tools", toolNames),
		)
	}

	if req.ResponseSchema != nil {
		responseSchema, err := schema.ConvertResponseSchema(*req.ResponseSchema)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "response schema conversion failed")
			return gai.ChatCompleteResponse{}, fmt.Errorf("error converting response schema: %w", err)
		}
		config.ResponseMIMEType = "application/json"
		config.ResponseSchema = responseSchema
		span.SetAttributes(attribute.Bool("ai.has_response_schema", true))
	}

	var history []*genai.Content
	for _, m := range req.Messages {
		var content genai.Content

		switch m.Role {
		case gai.MessageRoleUser:
			content.Role = genai.RoleUser
		case gai.MessageRoleModel:
			content.Role = genai.RoleModel
		default:
			panic("unknown role " + m.Role)
		}

		for _, part := range m.Parts {
			switch part.Type {
			case gai.MessagePartTypeText:
				content.Parts = append(content.Parts, &genai.Part{Text: part.Text()})

			case gai.MessagePartTypeToolCall:
				toolCall := part.ToolCall()
				args := make(map[string]any)
				if err := json.Unmarshal(toolCall.Args, &args); err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, "request tool call args unmarshal failed")
					return gai.ChatCompleteResponse{}, fmt.Errorf("error unmarshaling request tool call args: %w", err)
				}
				part := genai.NewPartFromFunctionCall(toolCall.Name, args)
				part.FunctionCall.ID = toolCall.ID
				content.Parts = append(content.Parts, part)

			case gai.MessagePartTypeToolResult:
				toolResult := part.ToolResult()
				res := map[string]any{"output": toolResult.Content}
				if toolResult.Err != nil {
					res = map[string]any{"error": toolResult.Err.Error()}
				}
				part := genai.NewPartFromFunctionResponse(toolResult.Name, res)
				part.FunctionResponse.ID = toolResult.ID
				content.Parts = append(content.Parts, part)

			case gai.MessagePartTypeData:
				data, err := io.ReadAll(part.Data)
				if err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, "data read failed")
					return gai.ChatCompleteResponse{}, fmt.Errorf("error reading request data: %w", err)
				}

				part := &genai.Part{
					InlineData: &genai.Blob{
						MIMEType: part.MIMEType,
						Data:     data,
					},
				}
				content.Parts = append(content.Parts, part)

			default:
				panic("unknown part type " + part.Type)
			}
		}

		history = append(history, &content)
	}

	// Delete the last content from the history, because SendMessageStream expects it as varargs
	lastContent := history[len(history)-1]
	history = history[:len(history)-1]

	chat, err := c.Client.Chats.Create(ctx, string(c.model), &config, history)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "chat session creation failed")
		return gai.ChatCompleteResponse{}, err
	}

	meta := &gai.ChatCompleteResponseMetadata{}

	res := gai.NewChatCompleteResponse(func(yield func(gai.MessagePart, error) bool) {
		defer span.End()

		for chunk, err := range chat.SendStream(ctx, lastContent.Parts...) {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "chat stream send failed")
				yield(gai.MessagePart{}, err)
				return
			}

			// Extract token usage from the response
			// Google GenAI sends usage metadata with every chunk during streaming:
			// - Early chunks show prompt tokens only (with minor variations between chunks)
			// - The final chunk contains complete counts including completion tokens
			// We update on each chunk, so the final values will be correct
			if chunk.UsageMetadata != nil {
				meta.Usage = gai.ChatCompleteResponseUsage{
					PromptTokens:     int(chunk.UsageMetadata.PromptTokenCount),
					ThoughtsTokens:   int(chunk.UsageMetadata.ThoughtsTokenCount),
					CompletionTokens: int(chunk.UsageMetadata.CandidatesTokenCount),
				}
				span.SetAttributes(
					attribute.Int("ai.prompt_tokens", int(chunk.UsageMetadata.PromptTokenCount)),
					attribute.Int("ai.thoughts_tokens", int(chunk.UsageMetadata.ThoughtsTokenCount)),
					attribute.Int("ai.completion_tokens", int(chunk.UsageMetadata.CandidatesTokenCount)),
				)
			}

			if len(chunk.Candidates) == 0 || chunk.Candidates[0].Content == nil {
				continue
			}

			for _, part := range chunk.Candidates[0].Content.Parts {
				if part.Text != "" {
					if !yield(gai.TextMessagePart(part.Text), nil) {
						return
					}
				}

				if part.FunctionCall != nil {
					args, err := json.Marshal(part.FunctionCall.Args)
					if err != nil {
						span.RecordError(err)
						span.SetStatus(codes.Error, "response tool call args marshal failed")
						yield(gai.MessagePart{}, fmt.Errorf("error marshaling response tool call args: %w", err))
						return
					}
					id := part.FunctionCall.ID
					if id == "" {
						id = createRandomID()
					}
					if !yield(gai.ToolCallPart(id, part.FunctionCall.Name, args), nil) {
						return
					}
				}
			}
		}
	})

	res.Meta = meta

	return res, nil
}

var _ gai.ChatCompleter = (*ChatCompleter)(nil)

func createRandomID() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(time.Now().Format(time.RFC3339Nano))))
}
