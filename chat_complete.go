package gai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"

	"github.com/invopop/jsonschema"
)

// ThinkingLevel controls how much reasoning effort the model applies.
type ThinkingLevel string

const (
	// ThinkingLevelNone disables thinking entirely.
	ThinkingLevelNone ThinkingLevel = "none"
	// ThinkingLevelMinimal applies minimal thinking.
	ThinkingLevelMinimal ThinkingLevel = "minimal"
	// ThinkingLevelLow applies low thinking effort.
	ThinkingLevelLow ThinkingLevel = "low"
	// ThinkingLevelMedium applies medium thinking effort.
	ThinkingLevelMedium ThinkingLevel = "medium"
	// ThinkingLevelHigh applies high thinking effort.
	ThinkingLevelHigh ThinkingLevel = "high"
	// ThinkingLevelXHigh applies extra-high thinking effort.
	ThinkingLevelXHigh ThinkingLevel = "xhigh"
	// ThinkingLevelMax applies maximum thinking effort.
	ThinkingLevelMax ThinkingLevel = "max"
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
	MaxCompletionTokens *int
	Messages            []Message
	ResponseSchema      *Schema
	System              *string
	Temperature         *Temperature
	ThinkingLevel       *ThinkingLevel
	Tools               []Tool
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

// NewUserDataMessage is a convenience function to create a new user data message.
func NewUserDataMessage(mimeType string, data io.Reader) Message {
	return Message{
		Role: MessageRoleUser,
		Parts: []MessagePart{
			DataMessagePart(mimeType, data),
		},
	}
}

// NewModelTextMessage is a convenience function to create a new model text message.
func NewModelTextMessage(text string) Message {
	return Message{
		Role: MessageRoleModel,
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
	MessageRoleUser  MessageRole = "user"
	MessageRoleModel MessageRole = "model"
)

type MessagePart struct {
	Type       MessagePartType
	Data       io.Reader
	MIMEType   string
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
	MessagePartTypeData       MessagePartType = "data"
	MessagePartTypeText       MessagePartType = "text"
	MessagePartTypeToolCall   MessagePartType = "tool_call"
	MessagePartTypeToolResult MessagePartType = "tool_result"
)

func TextMessagePart(text string) MessagePart {
	return MessagePart{
		Type: MessagePartTypeText,
		text: &text,
	}
}

func DataMessagePart(mimeType string, data io.Reader) MessagePart {
	return MessagePart{
		Type:     MessagePartTypeData,
		Data:     data,
		MIMEType: mimeType,
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

type ChatCompleteResponseUsage struct {
	PromptTokens     int
	ThoughtsTokens   int
	CompletionTokens int
}

// ChatCompleteFinishReason describes why the model stopped generating tokens.
type ChatCompleteFinishReason string

const (
	// ChatCompleteFinishReasonUnknown indicates that the provider did not supply a recognised termination code.
	ChatCompleteFinishReasonUnknown ChatCompleteFinishReason = "unknown"
	// ChatCompleteFinishReasonStop indicates that generation stopped naturally or due to a configured stop sequence.
	ChatCompleteFinishReasonStop ChatCompleteFinishReason = "stop"
	// ChatCompleteFinishReasonLength indicates that generation hit the configured token limit.
	ChatCompleteFinishReasonLength ChatCompleteFinishReason = "length"
	// ChatCompleteFinishReasonContentFilter indicates that a platform-level moderation filter blocked the content.
	ChatCompleteFinishReasonContentFilter ChatCompleteFinishReason = "content_filter"
	// ChatCompleteFinishReasonToolCalls indicates that the model requested a tool invocation mid-response.
	ChatCompleteFinishReasonToolCalls ChatCompleteFinishReason = "tool_calls"
	// ChatCompleteFinishReasonRefusal indicates that the model produced a refusal message of its own accord.
	ChatCompleteFinishReasonRefusal ChatCompleteFinishReason = "refusal"
)

// ChatCompleteResponseMetadata contains metadata about the request and response, for example, token usage.
type ChatCompleteResponseMetadata struct {
	Usage ChatCompleteResponseUsage
	// FinishReason is optional; nil indicates the provider omitted a finish signal entirely.
	FinishReason *ChatCompleteFinishReason
}

// ChatCompleteResponse for [ChatCompleter].
// Construct with [NewChatCompleteResponse].
// Note that the [ChatCompleteResponse.Meta] field is a pointer, because it's updated continuously
// until the streaming response with [ChatCompleteResponse.Parts] is complete.
type ChatCompleteResponse struct {
	Meta      *ChatCompleteResponseMetadata
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
	Execute     ToolFunction
	Summarize   ToolFunction
}

// ToolSchema in JSON Schema format of the arguments the tool accepts.
type ToolSchema struct {
	Properties map[string]*Schema
}

func GenerateToolSchema[T any]() ToolSchema {
	schema := GenerateSchema[T]()

	return ToolSchema{
		Properties: schema.Properties,
	}
}

type SchemaType string

const (
	// OpenAPI string type
	SchemaTypeString SchemaType = "string"
	// OpenAPI number type
	SchemaTypeNumber SchemaType = "number"
	// OpenAPI integer type
	SchemaTypeInteger SchemaType = "integer"
	// OpenAPI boolean type
	SchemaTypeBoolean SchemaType = "boolean"
	// OpenAPI array type
	SchemaTypeArray SchemaType = "array"
	// OpenAPI object type
	SchemaTypeObject SchemaType = "object"
)

type Schema struct {
	// Optional. The value should be validated against any (one or more) of the subschemas
	// in the list.
	AnyOf []*Schema `json:"anyOf,omitempty"`

	// Optional. Default value of the data.
	Default any `json:"default,omitempty"`

	// Optional. The description of the data.
	Description string `json:"description,omitempty"`

	// Optional. Possible values of the element of primitive type with enum format. Examples:
	// 1. We can define direction as : {type:STRING, format:enum, enum:["EAST", NORTH",
	// "SOUTH", "WEST"]} 2. We can define apartment number as : {type:INTEGER, format:enum,
	// enum:["101", "201", "301"]}
	Enum []string `json:"enum,omitempty"`

	// Optional. Example of the object. Will only populated when the object is the root.
	Example any `json:"example,omitempty"`

	// Optional. The format of the data. Supported formats: for NUMBER type: "float", "double"
	// for INTEGER type: "int32", "int64" for STRING type: "email", "byte", etc
	Format string `json:"format,omitempty"`

	// Optional. SCHEMA FIELDS FOR TYPE ARRAY Schema of the elements of Type.ARRAY.
	Items *Schema `json:"items,omitempty"`

	// Optional. Maximum number of the elements for Type.ARRAY.
	MaxItems *int64 `json:"maxItems,omitempty,string"`

	// Optional. Maximum value of the Type.INTEGER and Type.NUMBER
	Maximum *float64 `json:"maximum,omitempty"`

	// Optional. Minimum number of the elements for Type.ARRAY.
	MinItems *int64 `json:"minItems,omitempty,string"`

	// Optional. Minimum value of the Type.INTEGER and Type.NUMBER.
	Minimum *float64 `json:"minimum,omitempty"`

	// Optional. SCHEMA FIELDS FOR TYPE OBJECT Properties of Type.OBJECT.
	Properties map[string]*Schema `json:"properties,omitempty"`

	// Optional. The order of the properties. Not a standard field in open API spec. Only
	// used to support the order of the properties.
	PropertyOrdering []string `json:"propertyOrdering,omitempty"`

	// Optional. Required properties of Type.OBJECT.
	Required []string `json:"required,omitempty"`

	// Optional. The title of the Schema.
	Title string `json:"title,omitempty"`

	// Optional. The type of the data.
	Type SchemaType `json:"type,omitempty"`
}

// GenerateSchema from any type.
// See github.com/invopop/jsonschema for struct tags etc.
func GenerateSchema[T any]() Schema {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}

	var v T
	schema := reflector.Reflect(v)

	return convertJSONSchemaToSchema(schema)
}

func convertJSONSchemaToSchema(js *jsonschema.Schema) Schema {
	s := Schema{
		Description: js.Description,
		Title:       js.Title,
		Default:     js.Default,
		Format:      js.Format,
	}

	// Convert example (Examples is a slice, use first one if available)
	if len(js.Examples) > 0 {
		s.Example = js.Examples[0]
	}

	// Convert type
	if js.Type != "" {
		switch js.Type {
		case "string", "number", "integer", "boolean", "array", "object":
			s.Type = SchemaType(js.Type)
		default:
			panic("unsupported schema type " + js.Type)
		}
	}

	// Convert enum
	if len(js.Enum) > 0 {
		s.Enum = make([]string, len(js.Enum))
		for i, v := range js.Enum {
			s.Enum[i] = fmt.Sprint(v)
		}
	}

	// Convert numeric constraints (json.Number is a string)
	if js.Minimum != "" {
		if min, err := js.Minimum.Float64(); err == nil {
			s.Minimum = &min
		}
	}
	if js.Maximum != "" {
		if max, err := js.Maximum.Float64(); err == nil {
			s.Maximum = &max
		}
	}

	// Convert array constraints
	if js.MinItems != nil {
		minItems := int64(*js.MinItems)
		s.MinItems = &minItems
	}
	if js.MaxItems != nil {
		maxItems := int64(*js.MaxItems)
		s.MaxItems = &maxItems
	}
	if js.Items != nil {
		converted := convertJSONSchemaToSchema(js.Items)
		s.Items = &converted
	}

	// Convert object constraints
	if js.Properties != nil && js.Properties.Len() > 0 {
		s.Properties = make(map[string]*Schema)
		s.PropertyOrdering = make([]string, 0, js.Properties.Len())

		// Iterate through ordered map
		for pair := js.Properties.Oldest(); pair != nil; pair = pair.Next() {
			converted := convertJSONSchemaToSchema(pair.Value)
			s.Properties[pair.Key] = &converted
			s.PropertyOrdering = append(s.PropertyOrdering, pair.Key)
		}
	}
	s.Required = js.Required

	// Convert anyOf
	if len(js.AnyOf) > 0 {
		s.AnyOf = make([]*Schema, len(js.AnyOf))
		for i, v := range js.AnyOf {
			converted := convertJSONSchemaToSchema(v)
			s.AnyOf[i] = &converted
		}
	}

	return s
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
	Name    string
	Content string
	Err     error
}
