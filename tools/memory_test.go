package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"maragu.dev/is"

	"maragu.dev/gai/tools"
)

// mockMemoryStore implements all the memory-related interfaces
type mockMemoryStore struct {
	memories []string
	failMode bool
}

func (m *mockMemoryStore) SaveMemory(_ context.Context, memory string) error {
	if m.failMode {
		return errors.New("mock memory store fail")
	}
	m.memories = append(m.memories, memory)
	return nil
}

func (m *mockMemoryStore) GetMemories(_ context.Context) ([]string, error) {
	if m.failMode {
		return nil, errors.New("mock memory store fail")
	}
	return m.memories, nil
}

func (m *mockMemoryStore) SearchMemories(_ context.Context, query string) ([]string, error) {
	if m.failMode {
		return nil, errors.New("mock memory store fail")
	}

	var results []string
	queryLower := strings.ToLower(query)
	for _, memory := range m.memories {
		if strings.Contains(strings.ToLower(memory), queryLower) {
			results = append(results, memory)
		}
	}
	return results, nil
}

func TestNewSaveMemory(t *testing.T) {
	t.Run("saves a memory successfully", func(t *testing.T) {
		store := &mockMemoryStore{}
		tool := tools.NewSaveMemory(store)

		is.Equal(t, "save_memory", tool.Name)

		memory := "Remember to buy milk"
		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.SaveMemoryArgs{
			Memory: memory,
		}))

		is.NotError(t, err)
		is.Equal(t, "OK", result)
		is.Equal(t, 1, len(store.memories))
		is.Equal(t, memory, store.memories[0])
	})

	t.Run("handles error when saving memory fails", func(t *testing.T) {
		store := &mockMemoryStore{failMode: true}
		tool := tools.NewSaveMemory(store)

		_, err := tool.Function(t.Context(), mustMarshalJSON(tools.SaveMemoryArgs{
			Memory: "This will fail",
		}))

		is.True(t, err != nil)
		is.True(t, strings.Contains(err.Error(), "error saving memory"))
	})

	t.Run("handles invalid JSON input", func(t *testing.T) {
		store := &mockMemoryStore{}
		tool := tools.NewSaveMemory(store)

		_, err := tool.Function(t.Context(), json.RawMessage(`{invalid json`))

		is.True(t, err != nil)
		is.True(t, strings.Contains(err.Error(), "error unmarshaling"))
	})

	t.Run("summarize save_memory with short memory", func(t *testing.T) {
		store := &mockMemoryStore{}
		tool := tools.NewSaveMemory(store)

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.SaveMemoryArgs{
			Memory: "Buy milk",
		}))

		is.NotError(t, err)
		is.Equal(t, `memory="Buy milk"`, summary)
	})

	t.Run("summarize save_memory with long memory", func(t *testing.T) {
		store := &mockMemoryStore{}
		tool := tools.NewSaveMemory(store)

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.SaveMemoryArgs{
			Memory: "This is a very long memory that should be truncated after 30 characters",
		}))

		is.NotError(t, err)
		is.Equal(t, `memory="This is a very long memory tha..."`, summary)
	})

	t.Run("summarize save_memory with invalid JSON", func(t *testing.T) {
		store := &mockMemoryStore{}
		tool := tools.NewSaveMemory(store)

		summary, err := tool.Summarize(t.Context(), []byte(`{invalid json`))

		is.NotError(t, err)
		is.Equal(t, "error parsing arguments", summary)
	})

}

func TestNewGetMemories(t *testing.T) {
	t.Run("returns all memories", func(t *testing.T) {
		store := &mockMemoryStore{
			memories: []string{"Memory 1", "Memory 2", "Memory 3"},
		}
		tool := tools.NewGetMemories(store)

		is.Equal(t, "get_memories", tool.Name)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.GetMemoryArgs{}))

		is.NotError(t, err)
		is.Equal(t, "Memories: [Memory 1 Memory 2 Memory 3]", result)
	})

	t.Run("returns empty list when no memories exist", func(t *testing.T) {
		store := &mockMemoryStore{}
		tool := tools.NewGetMemories(store)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.GetMemoryArgs{}))

		is.NotError(t, err)
		is.Equal(t, "Memories: []", result)
	})

	t.Run("handles error when retrieving memories fails", func(t *testing.T) {
		store := &mockMemoryStore{failMode: true}
		tool := tools.NewGetMemories(store)

		_, err := tool.Function(t.Context(), mustMarshalJSON(tools.GetMemoryArgs{}))

		is.True(t, err != nil)
		is.True(t, strings.Contains(err.Error(), "error getting memories"))
	})

	t.Run("summarize get_memories", func(t *testing.T) {
		store := &mockMemoryStore{}
		tool := tools.NewGetMemories(store)

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.GetMemoryArgs{}))

		is.NotError(t, err)
		is.Equal(t, "", summary)
	})
}

func TestNewSearchMemories(t *testing.T) {
	t.Run("returns matching memories", func(t *testing.T) {
		store := &mockMemoryStore{
			memories: []string{
				"My pet rock needs a bath but hates getting wet",
				"Aliens probably think humans are weird for keeping plants as pets",
				"If I could teleport, I'd still be late to meetings",
			},
		}
		tool := tools.NewSearchMemories(store)

		is.Equal(t, "search_memories", tool.Name)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.SearchMemoriesArgs{
			Query: "pet",
		}))

		is.NotError(t, err)
		is.Equal(t, "Found memories: [My pet rock needs a bath but hates getting wet Aliens probably think humans are weird for keeping plants as pets]", result)
	})

	t.Run("returns multiple matching memories", func(t *testing.T) {
		store := &mockMemoryStore{
			memories: []string{
				"I dreamt my code compiled on the first try",
				"My rubber duck debugger judged me today",
				"Conspiracy theory: semicolons are secretly plotting against me",
			},
		}
		tool := tools.NewSearchMemories(store)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.SearchMemoriesArgs{
			Query: "my",
		}))

		is.NotError(t, err)
		is.Equal(t, "Found memories: [I dreamt my code compiled on the first try My rubber duck debugger judged me today]", result)
	})

	t.Run("returns no memories when no matches found", func(t *testing.T) {
		store := &mockMemoryStore{
			memories: []string{
				"My houseplant is plotting world domination",
				"Time is just spicy space and no one can convince me otherwise",
				"According to quantum physics, I might be a waffle in another universe",
			},
		}
		tool := tools.NewSearchMemories(store)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.SearchMemoriesArgs{
			Query: "coffee",
		}))

		is.NotError(t, err)
		is.Equal(t, "No memories found matching the query.", result)
	})

	t.Run("handles error when search fails", func(t *testing.T) {
		store := &mockMemoryStore{failMode: true}
		tool := tools.NewSearchMemories(store)

		_, err := tool.Function(t.Context(), mustMarshalJSON(tools.SearchMemoriesArgs{
			Query: "anything",
		}))

		is.True(t, err != nil)
		is.True(t, strings.Contains(err.Error(), "error searching memories"))
	})

	t.Run("handles invalid JSON input", func(t *testing.T) {
		store := &mockMemoryStore{}
		tool := tools.NewSearchMemories(store)

		_, err := tool.Function(t.Context(), json.RawMessage(`{invalid json`))

		is.True(t, err != nil)
		is.True(t, strings.Contains(err.Error(), "error unmarshaling"))
	})

	t.Run("summarize search_memories", func(t *testing.T) {
		store := &mockMemoryStore{}
		tool := tools.NewSearchMemories(store)

		summary, err := tool.Summarize(t.Context(), mustMarshalJSON(tools.SearchMemoriesArgs{
			Query: "coffee",
		}))

		is.NotError(t, err)
		is.Equal(t, `query="coffee"`, summary)
	})

	t.Run("summarize search_memories with invalid JSON", func(t *testing.T) {
		store := &mockMemoryStore{}
		tool := tools.NewSearchMemories(store)

		summary, err := tool.Summarize(t.Context(), []byte(`{invalid json`))

		is.NotError(t, err)
		is.Equal(t, "error parsing arguments", summary)
	})
}
