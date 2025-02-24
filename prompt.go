package gai

import (
	"context"
	"io"
	"iter"
	"strings"
)

// ChatModel identified by name.
type ChatModel string

// Prompt for a chat model.
type Prompt struct {
	Model       ChatModel
	Messages    []Message
	Temperature *float64
}

// MessageRole for [Message].
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

// MessagePartType for [MessagePart].
type MessagePartType string

const (
	MessagePartTypeText MessagePartType = "text"
)

type Message struct {
	Role  MessageRole
	Parts []MessagePart
}

type MessagePart struct {
	Type MessagePartType
	Data io.Reader
	text *string
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

func TextMessagePart(text string) MessagePart {
	return MessagePart{
		Type: MessagePartTypeText,
		Data: strings.NewReader(text),
		text: Ptr(text),
	}
}

// Completer is satisfied by clients supporting chat completion.
// Streaming chat completion is preferred where possible, so that methods on [CompletionResponse] like [CompletionResponse.Parts]
// can be used to stream the response.
type Completer interface {
	Complete(ctx context.Context, p Prompt) CompletionResponse
}

type CompletionResponse struct {
	partsFunc iter.Seq2[MessagePart, error]
}

func (c CompletionResponse) Parts() iter.Seq2[MessagePart, error] {
	return c.partsFunc
}

func NewCompletionResponse(partsFunc iter.Seq2[MessagePart, error]) CompletionResponse {
	return CompletionResponse{
		partsFunc: partsFunc,
	}
}

func Ptr[T any](v T) *T {
	return &v
}
