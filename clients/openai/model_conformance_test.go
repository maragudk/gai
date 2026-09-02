package openai_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai/clients/openai"
)

// exportedModels are the values of all exported chat and embed model constants,
// checked against the live API by [TestModelConformance].
var exportedModels = []string{
	string(openai.ChatCompleteModelGPT5),
	string(openai.ChatCompleteModelGPT5Mini),
	string(openai.ChatCompleteModelGPT5Nano),
	string(openai.ChatCompleteModelGPT5_1),
	string(openai.ChatCompleteModelGPT5_2),
	string(openai.ChatCompleteModelGPT5_4),
	string(openai.ChatCompleteModelGPT5_4Mini),
	string(openai.ChatCompleteModelGPT5_4Nano),
	string(openai.ChatCompleteModelGPT5_5),
	string(openai.ChatCompleteModelGPT5_6Luna),
	string(openai.ChatCompleteModelGPT5_6Sol),
	string(openai.ChatCompleteModelGPT5_6Terra),
	string(openai.EmbedModelTextEmbedding3Large),
	string(openai.EmbedModelTextEmbedding3Small),
}

// ignoredModels are provider model IDs deliberately not exported as constants,
// per the curation policy above the model const blocks. An entry ending in "*"
// matches every model ID with that prefix; any other entry matches exactly.
var ignoredModels = []string{
	// Dated snapshots of exported aliases; "-2*" matches the date suffix.
	"gpt-5-2*",
	"gpt-5-mini-2*",
	"gpt-5-nano-2*",
	"gpt-5.1-2*",
	"gpt-5.2-2*",
	"gpt-5.4-2*",
	"gpt-5.4-mini-2*",
	"gpt-5.4-nano-2*",
	"gpt-5.5-2*",
	// Floating aliases that track ChatGPT; the exported constants pin versions instead.
	"chat-latest",
	"gpt-5-chat-latest",
	"gpt-5.1-chat-latest",
	"gpt-5.2-chat-latest",
	"gpt-5.3-chat-latest",
	// Pro reasoning models: only usable via the Responses API, rejected by the
	// chat-completions endpoint this package targets.
	"gpt-5-pro*",
	"gpt-5.2-pro*",
	"gpt-5.4-pro*",
	"gpt-5.5-pro*",
	// Coding-agent (codex) and search variants.
	"gpt-5-codex",
	"gpt-5.1-codex*",
	"gpt-5.2-codex",
	"gpt-5.3-codex",
	"gpt-5-search-api*",
	// Audio, realtime, transcription, TTS, image, video, and moderation surfaces.
	"chatgpt-image-latest",
	"gpt-audio*",
	"gpt-image*",
	"gpt-live-transcribe",
	"gpt-realtime*",
	"gpt-transcribe",
	"omni-moderation-*",
	"sora-*",
	"tts-1*",
	"whisper-*",
	// Legacy generations and legacy embeddings.
	"babbage-002",
	"davinci-002",
	"gpt-3.5-*",
	"gpt-4*",
	"o1*",
	"o3*",
	"o4-mini*",
	"text-embedding-ada-002",
}

// isIgnoredModel reports whether the given model ID matches an entry in [ignoredModels].
func isIgnoredModel(id string) bool {
	for _, pattern := range ignoredModels {
		if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
			if strings.HasPrefix(id, prefix) {
				return true
			}
			continue
		}
		if id == pattern {
			return true
		}
	}
	return false
}

func TestModelConformance(t *testing.T) {
	if os.Getenv("GAI_MODEL_CONFORMANCE") == "" {
		t.Skip("set GAI_MODEL_CONFORMANCE=1 to run the live model conformance test")
	}

	client := newClient(t)

	t.Run("every exported model constant resolves via get-by-ID", func(t *testing.T) {
		for _, model := range exportedModels {
			t.Run(model, func(t *testing.T) {
				_, err := client.Client.Models.Get(t.Context(), model)
				is.NotError(t, err)
			})
		}
	})

	t.Run("every listed model ID is exported or ignored", func(t *testing.T) {
		var unmatched []string
		iter := client.Client.Models.ListAutoPaging(t.Context())
		for iter.Next() {
			id := iter.Current().ID
			if !slices.Contains(exportedModels, id) && !isIgnoredModel(id) {
				unmatched = append(unmatched, id)
			}
		}
		is.NotError(t, iter.Err())

		slices.Sort(unmatched)
		is.Equal(t, 0, len(unmatched), "export or ignore these model IDs: "+strings.Join(unmatched, ", "))
	})
}
