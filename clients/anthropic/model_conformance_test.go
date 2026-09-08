package anthropic_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"maragu.dev/is"

	"maragu.dev/gai/clients/anthropic"
)

// exportedModels are the values of all exported chat model constants,
// checked against the live API by [TestModelConformance].
var exportedModels = []string{
	string(anthropic.ChatCompleteModelClaudeHaiku4_5Latest),
	string(anthropic.ChatCompleteModelClaudeSonnet4_5Latest),
	string(anthropic.ChatCompleteModelClaudeOpus4_5Latest),
	string(anthropic.ChatCompleteModelClaudeSonnet4_6Latest),
	string(anthropic.ChatCompleteModelClaudeOpus4_6Latest),
	string(anthropic.ChatCompleteModelClaudeOpus4_7Latest),
	string(anthropic.ChatCompleteModelClaudeOpus4_8Latest),
	string(anthropic.ChatCompleteModelClaudeFable5Latest),
	string(anthropic.ChatCompleteModelClaudeFable5_1Latest),
	string(anthropic.ChatCompleteModelClaudeSonnet5Latest),
	string(anthropic.ChatCompleteModelClaudeOpus5Latest),
}

// ignoredModels are provider model IDs deliberately not exported as constants,
// per the curation policy above the model const block. An entry ending in "*"
// matches every model ID with that prefix; any other entry matches exactly.
var ignoredModels = []string{
	// Dated snapshots of exported aliases; "-2*" matches the date suffix.
	"claude-haiku-4-5-2*",
	"claude-opus-4-5-2*",
	"claude-sonnet-4-5-2*",
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
				_, err := client.Client.Models.Get(t.Context(), model, anthropicsdk.ModelGetParams{})
				is.NotError(t, err)
			})
		}
	})

	t.Run("every listed model ID is exported or ignored", func(t *testing.T) {
		var unmatched []string
		iter := client.Client.Models.ListAutoPaging(t.Context(), anthropicsdk.ModelListParams{})
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
