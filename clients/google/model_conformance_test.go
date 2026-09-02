package google_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai/clients/google"
)

// exportedModels are the values of all exported chat and embed model constants,
// checked against the live API by [TestModelConformance].
var exportedModels = []string{
	string(google.ChatCompleteModelGemini2_0Flash),
	string(google.ChatCompleteModelGemini2_5Flash),
	string(google.ChatCompleteModelGemini2_5FlashLite),
	string(google.ChatCompleteModelGemini2_5Pro),
	string(google.ChatCompleteModelGemini3FlashPreview),
	string(google.ChatCompleteModelGemini3_1ProPreview),
	string(google.ChatCompleteModelGemini3_1FlashLite),
	string(google.ChatCompleteModelGemini3_5Flash),
	string(google.ChatCompleteModelGemini3_5FlashLite),
	string(google.ChatCompleteModelGemini3_6Flash),
	string(google.ChatCompleteModelGemini3_7Flash),
	string(google.EmbedModelGeminiEmbedding001),
	string(google.EmbedModelGeminiEmbedding2),
}

// ignoredModels are provider model IDs deliberately not exported as constants,
// per the curation policy above the model const blocks. An entry ending in "*"
// matches every model ID with that prefix; any other entry matches exactly.
var ignoredModels = []string{
	// Floating aliases that track the newest model; the exported constants pin versions instead.
	"gemini-flash-latest",
	"gemini-flash-lite-latest",
	"gemini-pro-latest",
	// Previews without an exported stable counterpart, or superseded by one.
	"gemini-3.1-flash-lite-preview",
	"gemini-3.1-pro-preview-customtools",
	"gemini-embedding-2-preview",
	// Omni models: only usable via the Interactions API, rejected by the generateContent
	// endpoint this package targets. Both the preview and its stable 1.1 counterpart list
	// generateContent as a supported action, yet answer streaming and non-streaming calls
	// alike with 400 INVALID_ARGUMENT, `This model only supports Interactions API.`.
	"gemini-omni-1.1-flash",
	"gemini-omni-flash-preview",
	// Modality variants: image generation, TTS, transcription, computer use, robotics, music.
	"gemini-2.5-computer-use-*",
	"gemini-2.5-flash-image*",
	"gemini-2.5-flash-preview-tts",
	"gemini-2.5-pro-preview-tts",
	"gemini-3-pro-image*",
	"gemini-3.1-flash-image*",
	"gemini-3.1-flash-lite-image*",
	"gemini-3.1-flash-tts-preview",
	"gemini-3.5-transcribe",
	"gemini-robotics-*",
	"lyria-*",
	"nano-banana-*",
	// Non-Gemini artifacts: open models and agentic research/IDE products.
	"antigravity-*",
	"deep-research-*",
	"gemma-*",
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
				_, err := client.Client.Models.Get(t.Context(), model, nil)
				is.NotError(t, err)
			})
		}
	})

	t.Run("every listed model ID is exported or ignored", func(t *testing.T) {
		var unmatched []string
		for model, err := range client.Client.Models.All(t.Context()) {
			is.NotError(t, err)

			// The list endpoint also returns image, video, TTS, and other artifacts gai has no
			// surface for; scope the check to models that can chat-complete or embed.
			if !slices.Contains(model.SupportedActions, "generateContent") && !slices.Contains(model.SupportedActions, "embedContent") {
				continue
			}

			id := strings.TrimPrefix(model.Name, "models/")
			if !slices.Contains(exportedModels, id) && !isIgnoredModel(id) {
				unmatched = append(unmatched, id)
			}
		}

		slices.Sort(unmatched)
		is.Equal(t, 0, len(unmatched), "export or ignore these model IDs: "+strings.Join(unmatched, ", "))
	})
}
