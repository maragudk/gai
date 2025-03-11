package gai

import (
	"context"
	"fmt"
	"io"
	"iter"
	"strings"
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
	Temperature *Temperature
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

// MessageRole for [Message].
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

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

// MessagePartType for [MessagePart].
type MessagePartType string

const (
	MessagePartTypeText MessagePartType = "text"
)

func TextMessagePart(text string) MessagePart {
	return MessagePart{
		Type: MessagePartTypeText,
		Data: strings.NewReader(text),
		text: &text,
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
	ChatComplete(ctx context.Context, p ChatCompleteRequest) (ChatCompleteResponse, error)
}

func Ptr[T any](v T) *T {
	return &v
}
