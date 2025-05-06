package gai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"strings"

	"github.com/invopop/jsonschema"
)

type Temperature float64

// String satisfies [fmt.Stringer].
func (t Temperature) String() string {
	return fmt.Sprintf("%.2f", t)
}

func (t Temperature) Float64() float64 {
	return float64(t)
}

// ChatCompleteRequest for a chat model.
type ChatCompleteRequest struct {
	Messages    []Message
	System      *string
	Temperature *Temperature
	Tools       []Tool
}

type Message struct {
	Role  MessageRole
	Parts []MessagePart
}

// NewUserTextMessage is a convenience function to create a new user text message.
func NewUserTextMessage(text string) Message {
	return Message{
		Role: MessageRoleUser,
		Parts: []MessagePart{
			TextMessagePart(text),
		},
	}
}

// NewAssistantTextMessage is a convenience function to create a new assistant text message.
func NewAssistantTextMessage(text string) Message {
	return Message{
		Role: MessageRoleAssistant,
		Parts: []MessagePart{
			TextMessagePart(text),
		},
	}
}

func NewUserToolResultMessage(result ToolResult) Message {
	return Message{
		Role: MessageRoleUser,
		Parts: []MessagePart{
			{
				Type:       MessagePartTypeToolResult,
				toolResult: &result,
			},
		},
	}
}

// MessageRole for [Message].
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant" // TODO make this "model", like at Google?
)

type MessagePart struct {
	Type       MessagePartType
	Data       io.Reader
	text       *string
	toolCall   *ToolCall
	toolResult *ToolResult
}

func (m MessagePart) Text() string {
	if m.Type != MessagePartTypeText {
		panic("not text type")
	}
	if m.text != nil {
		return *m.text
	}
	text, err := io.ReadAll(m.Data)
	if err != nil {
		panic("error reading text: " + err.Error())
	}
	return string(text)
}

func (m MessagePart) ToolCall() ToolCall {
	if m.Type != MessagePartTypeToolCall {
		panic("not tool call type")
	}
	return *m.toolCall
}

func (m MessagePart) ToolResult() ToolResult {
	if m.Type != MessagePartTypeToolResult {
		panic("not tool result type")
	}
	return *m.toolResult
}

// MessagePartType for [MessagePart].
type MessagePartType string

const (
	MessagePartTypeText       MessagePartType = "text"
	MessagePartTypeToolCall   MessagePartType = "tool_call"
	MessagePartTypeToolResult MessagePartType = "tool_result"
)

func TextMessagePart(text string) MessagePart {
	return MessagePart{
		Type: MessagePartTypeText,
		Data: strings.NewReader(text),
		text: &text,
	}
}

func ToolCallPart(id, name string, args json.RawMessage) MessagePart {
	return MessagePart{
		Type: MessagePartTypeToolCall,
		toolCall: &ToolCall{
			ID:   id,
			Name: name,
			Args: args,
		},
	}
}

// ChatCompleteResponse for [ChatCompleter].
// Construct with [NewChatCompleteResponse].
type ChatCompleteResponse struct {
	partsFunc iter.Seq2[MessagePart, error]
}

func NewChatCompleteResponse(partsFunc iter.Seq2[MessagePart, error]) ChatCompleteResponse {
	return ChatCompleteResponse{
		partsFunc: partsFunc,
	}
}

func (c ChatCompleteResponse) Parts() iter.Seq2[MessagePart, error] {
	return c.partsFunc
}

// ChatCompleter is satisfied by models supporting chat completion.
// Streaming chat completion is preferred where possible, so that methods on [ChatCompleteResponse],
// like [ChatCompleteResponse.Parts], can be used to stream the response.
type ChatCompleter interface {
	ChatComplete(ctx context.Context, req ChatCompleteRequest) (ChatCompleteResponse, error)
}

func Ptr[T any](v T) *T {
	return &v
}

// Tool definition.
type Tool struct {
	Name        string
	Description string
	Schema      ToolSchema
	Function    ToolFunction
}

// ToolSchema in JSON Schema format of the arguments the tool accepts.
type ToolSchema struct {
	Properties any
}

func GenerateSchema[T any]() ToolSchema {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}

	var v T
	schema := reflector.Reflect(v)

	return ToolSchema{
		Properties: schema.Properties,
	}
}

type ToolFunction func(ctx context.Context, rawArgs json.RawMessage) (string, error)

type ToolCall struct {
	ID   string
	Name string
	Args json.RawMessage
}

// TODO tool result can be string but also other types, such as image!
type ToolResult struct {
	ID      string
	Content string
	Err     error
}
