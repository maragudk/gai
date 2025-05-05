package tools_test

import (
	"testing"
	"time"

	"maragu.dev/gai/tools"
	"maragu.dev/is"
)

func TestNewGetTime(t *testing.T) {
	t.Run("returns the current time in RFC3339 format", func(t *testing.T) {
		// Create a fixed time for testing
		fixedTime := time.Date(2023, 5, 1, 12, 30, 45, 0, time.UTC)
		
		// Create tool with a function that returns our fixed time
		tool := tools.NewGetTime(func() time.Time {
			return fixedTime
		})

		is.Equal(t, "get_time", tool.Name)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.GetTimeArgs{}))
		is.NotError(t, err)
		is.Equal(t, "2023-05-01T12:30:45Z", result)
	})
}
