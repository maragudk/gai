package google

import (
	"testing"

	"google.golang.org/genai"
	"maragu.dev/is"

	"maragu.dev/gai"
)

func TestMapChatFinishReason(t *testing.T) {
	tests := []struct {
		name     string
		reason   genai.FinishReason
		expected gai.ChatCompleteFinishReason
	}{
		{name: "maps STOP to stop", reason: genai.FinishReasonStop, expected: gai.ChatCompleteFinishReasonStop},
		{name: "maps MAX_TOKENS to length", reason: genai.FinishReasonMaxTokens, expected: gai.ChatCompleteFinishReasonLength},
		{name: "maps SAFETY to content_filter", reason: genai.FinishReasonSafety, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps RECITATION to content_filter", reason: genai.FinishReasonRecitation, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps BLOCKLIST to content_filter", reason: genai.FinishReasonBlocklist, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps PROHIBITED_CONTENT to content_filter", reason: genai.FinishReasonProhibitedContent, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps SPII to content_filter", reason: genai.FinishReasonSPII, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps IMAGE_SAFETY to content_filter", reason: genai.FinishReasonImageSafety, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps IMAGE_PROHIBITED_CONTENT to content_filter", reason: genai.FinishReasonImageProhibitedContent, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps IMAGE_RECITATION to content_filter", reason: genai.FinishReasonImageRecitation, expected: gai.ChatCompleteFinishReasonContentFilter},
		{name: "maps FINISH_REASON_UNSPECIFIED to unknown", reason: genai.FinishReasonUnspecified, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps OTHER to unknown", reason: genai.FinishReasonOther, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps LANGUAGE to unknown", reason: genai.FinishReasonLanguage, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps MALFORMED_FUNCTION_CALL to unknown", reason: genai.FinishReasonMalformedFunctionCall, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps UNEXPECTED_TOOL_CALL to unknown", reason: genai.FinishReasonUnexpectedToolCall, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps TOO_MANY_TOOL_CALLS to unknown", reason: genai.FinishReasonTooManyToolCalls, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps NO_IMAGE to unknown", reason: genai.FinishReasonNoImage, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps IMAGE_OTHER to unknown", reason: genai.FinishReasonImageOther, expected: gai.ChatCompleteFinishReasonUnknown},
		{name: "maps an unrecognised value to unknown", reason: "SOMETHING_NEW", expected: gai.ChatCompleteFinishReasonUnknown},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			is.Equal(t, test.expected, mapChatFinishReason(test.reason))
		})
	}
}
