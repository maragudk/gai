package gai

import (
	"context"
	"iter"
)

// ChatCompleter is satisfied by clients supporting chat completion.
// Streaming chat completion is preferred where possible, so that methods on [ChatCompleteResponse],
// like [ChatCompleteResponse.Parts], can be used to stream the response.
type ChatCompleter interface {
	ChatComplete(ctx context.Context, p Prompt) (ChatCompleteResponse, error)
}

type ChatCompleteResponse struct {
	partsFunc iter.Seq2[MessagePart, error]
}

func (c ChatCompleteResponse) Parts() iter.Seq2[MessagePart, error] {
	return c.partsFunc
}

func NewChatCompleteResponse(partsFunc iter.Seq2[MessagePart, error]) ChatCompleteResponse {
	return ChatCompleteResponse{
		partsFunc: partsFunc,
	}
}
