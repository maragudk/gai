package gai

import (
	"io"
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
		text: Ptr(text),
	}
}

func Ptr[T any](v T) *T {
	return &v
}
