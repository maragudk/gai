package openai_test

import (
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
	string(openai.ChatCompleteModelGPT5_1Mini),
	string(openai.ChatCompleteModelGPT5_2),
	string(openai.ChatCompleteModelGPT5_2Pro),
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
var ignoredModels = []string{}

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
