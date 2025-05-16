package tools_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"maragu.dev/gai"
	"maragu.dev/gai/tools"
	"maragu.dev/is"
)


// mockChatCompleter implements gai.ChatCompleter for testing
type mockChatCompleter struct{}

func (m *mockChatCompleter) ChatComplete(ctx context.Context, req gai.ChatCompleteRequest) (gai.ChatCompleteResponse, error) {
	// Extract the HTML from the user message
	html := req.Messages[0].Parts[0].Text()
	
	// Add "MARKDOWN:" prefix for simple testing
	markdownText := "MARKDOWN: " + html
	
	// Create a sequence function that yields a single markdown part
	partsFunc := func(yield func(gai.MessagePart, error) bool) {
		part := gai.TextMessagePart(markdownText)
		yield(part, nil)
		// No need to signal end with EOF
	}
	
	return gai.NewChatCompleteResponse(partsFunc), nil
}

func TestNewFetch(t *testing.T) {
	t.Run("successfully fetches content from a URL as HTML", func(t *testing.T) {
		// Create a test server that serves a simple response
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<p>Hello, World!</p>"))
		}))
		defer server.Close()

		client := &http.Client{Timeout: 5 * time.Second}
		// Pass nil for the completer
		tool := tools.NewFetch(client, nil)

		// Check tool name
		is.Equal(t, "fetch", tool.Name)

		// Execute the tool with the test server URL and HTML output format
		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.FetchArgs{
			URL:          server.URL,
			OutputFormat: "html",
		}))

		is.NotError(t, err)
		is.Equal(t, "<p>Hello, World!</p>", result)
	})
	
	t.Run("successfully fetches content and converts to Markdown", func(t *testing.T) {
		// Create a test server that serves HTML content
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<p>Hello, World!</p>"))
		}))
		defer server.Close()

		client := &http.Client{Timeout: 5 * time.Second}
		completer := &mockChatCompleter{}
		tool := tools.NewFetch(client, completer)

		// Execute the tool with the test server URL and Markdown output format
		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.FetchArgs{
			URL:          server.URL,
			OutputFormat: "markdown",
		}))

		is.NotError(t, err)
		is.Equal(t, "MARKDOWN: <p>Hello, World!</p>", result)
	})
	
	t.Run("uses Markdown as default output format when converter is available", func(t *testing.T) {
		// Create a test server that serves HTML content
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<p>Hello, World!</p>"))
		}))
		defer server.Close()

		client := &http.Client{Timeout: 5 * time.Second}
		completer := &mockChatCompleter{}
		tool := tools.NewFetch(client, completer)

		// Execute the tool with the test server URL without specifying format
		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.FetchArgs{
			URL: server.URL,
		}))

		is.NotError(t, err)
		is.Equal(t, "MARKDOWN: <p>Hello, World!</p>", result)
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
		tool := tools.NewFetch(client, nil)

		// Execute the tool with the root URL, which should redirect
		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.FetchArgs{
			URL:          server.URL,
			OutputFormat: "html", // Set HTML format to skip conversion
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
		tool := tools.NewFetch(client, nil)

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
		tool := tools.NewFetch(client, nil)

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
		tool := tools.NewFetch(client, nil)

		// Execute the tool with an empty URL
		_, err := tool.Function(t.Context(), mustMarshalJSON(tools.FetchArgs{
			URL: "",
		}))

		is.True(t, err != nil)
		is.Equal(t, "url cannot be empty", err.Error())
	})

	t.Run("returns error for invalid URL", func(t *testing.T) {
		client := &http.Client{Timeout: 5 * time.Second}
		tool := tools.NewFetch(client, nil)

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

		// Pass nil as the client and completer
		tool := tools.NewFetch(nil, nil)

		result, err := tool.Function(t.Context(), mustMarshalJSON(tools.FetchArgs{
			URL:          server.URL,
			OutputFormat: "html", // Set HTML format to skip conversion
		}))

		is.NotError(t, err)
		is.Equal(t, "Default client works!", result)
	})
	
	t.Run("returns error when markdown is requested but no converter is available", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<p>Hello, no converter!</p>"))
		}))
		defer server.Close()

		client := &http.Client{Timeout: 5 * time.Second}
		// Pass nil for the completer
		tool := tools.NewFetch(client, nil)

		// Request Markdown but with no converter available
		_, err := tool.Function(t.Context(), mustMarshalJSON(tools.FetchArgs{
			URL:          server.URL,
			OutputFormat: "markdown",
		}))

		is.True(t, err != nil)
		is.Equal(t, "markdown output requested but no converter is available", err.Error())
	})
}