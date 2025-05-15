package tools_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"maragu.dev/gai/tools"
	"maragu.dev/is"
)

func TestNewFetch(t *testing.T) {
	t.Run("successfully fetches content from a URL", func(t *testing.T) {
		// Create a test server that serves a simple response
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Hello, World!"))
		}))
		defer server.Close()

		client := &http.Client{Timeout: 5 * time.Second}
		tool := tools.NewFetch(client)

		// Check tool name
		is.Equal(t, "fetch", tool.Name)

		// Execute the tool with the test server URL
		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.FetchArgs{
			URL: server.URL,
		}))

		is.NotError(t, err)
		is.Equal(t, "Hello, World!", result)
	})

	t.Run("follows redirects correctly", func(t *testing.T) {
		// Create a mux to handle both routes
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/destination", http.StatusFound)
		})
		mux.HandleFunc("/destination", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Redirected successfully"))
		})

		// Create a server with the mux
		server := httptest.NewServer(mux)
		defer server.Close()

		client := &http.Client{Timeout: 5 * time.Second}
		tool := tools.NewFetch(client)

		// Execute the tool with the root URL, which should redirect
		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.FetchArgs{
			URL: server.URL,
		}))

		is.NotError(t, err)
		is.Equal(t, "Redirected successfully", result)
	})

	t.Run("returns error for client-side HTTP errors (4xx)", func(t *testing.T) {
		// Create a server that returns a 404 Not Found error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Not Found"))
		}))
		defer server.Close()

		client := &http.Client{Timeout: 5 * time.Second}
		tool := tools.NewFetch(client)

		// Execute the tool with the test server URL
		_, err := tool.Function(t.Context(), mustMarshalJSON(tools.FetchArgs{
			URL: server.URL,
		}))

		is.True(t, err != nil)
		is.Equal(t, "received HTTP error: 404 Not Found (status code: 404)", err.Error())
	})

	t.Run("retries and returns error for server-side HTTP errors (5xx)", func(t *testing.T) {
		// Track the number of request attempts
		attempts := 0

		// Create a server that always returns a 500 Internal Server Error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Internal Server Error"))
		}))
		defer server.Close()

		// Use a custom client with a very short timeout to speed up the test
		client := &http.Client{Timeout: 1 * time.Second}
		tool := tools.NewFetch(client)

		// Execute the tool with the test server URL
		_, err := tool.Function(t.Context(), mustMarshalJSON(tools.FetchArgs{
			URL: server.URL,
		}))

		is.True(t, err != nil)
		is.Equal(t, "received HTTP error: 500 Internal Server Error (status code: 500)", err.Error())
		// Should have tried multiple times due to retry logic
		is.True(t, attempts > 1)
	})

	t.Run("returns error for empty URL", func(t *testing.T) {
		client := &http.Client{Timeout: 5 * time.Second}
		tool := tools.NewFetch(client)

		// Execute the tool with an empty URL
		_, err := tool.Function(t.Context(), mustMarshalJSON(tools.FetchArgs{
			URL: "",
		}))

		is.True(t, err != nil)
		is.Equal(t, "url cannot be empty", err.Error())
	})

	t.Run("returns error for invalid URL", func(t *testing.T) {
		client := &http.Client{Timeout: 5 * time.Second}
		tool := tools.NewFetch(client)

		// Execute the tool with an invalid URL
		_, err := tool.Function(t.Context(), mustMarshalJSON(tools.FetchArgs{
			URL: "http://invalid-url-that-does-not-exist.example",
		}))

		is.True(t, err != nil)
		is.True(t, strings.Contains(err.Error(), `error fetching URL after 3 attempts: Get "http://invalid-url-that-does-not-exist.example"`))
	})

	t.Run("works with nil http client (creates default client)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Default client works!"))
		}))
		defer server.Close()

		// Pass nil as the client
		tool := tools.NewFetch(nil)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.FetchArgs{
			URL: server.URL,
		}))

		is.NotError(t, err)
		is.Equal(t, "Default client works!", result)
	})
}
