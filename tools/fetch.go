package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"maragu.dev/gai"
)

// FetchArgs holds the arguments for the Fetch tool
type FetchArgs struct {
	URL string `json:"url" jsonschema_description:"The URL to fetch."`
}

// NewFetch creates a new tool for fetching content from a URL
func NewFetch(client *http.Client) gai.Tool {
	// If no client is provided, create one with default settings
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	return gai.Tool{
		Name:        "fetch",
		Description: "Fetch an HTML site and output the results as a string. Follows redirects automatically.",
		Schema:      gai.GenerateSchema[FetchArgs](),
		Function: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args FetchArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "", fmt.Errorf("error unmarshaling fetch args from JSON: %w", err)
			}

			if args.URL == "" {
				return "", errors.New("url cannot be empty")
			}

			// Maximum number of retries for transient errors
			const maxRetries = 3
			// Base delay in milliseconds before retrying
			const baseDelayMs = 500

			var res *http.Response
			var err error

			// Retry logic for transient errors
			for attempt := range maxRetries {
				// Create a new request for each attempt
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, args.URL, nil)
				if err != nil {
					return "", fmt.Errorf("error creating request: %w", err)
				}

				// Set common headers
				req.Header.Set("User-Agent", "gai-fetch-tool/1.0")

				// Execute the request
				res, err = client.Do(req)

				// Check for errors that might be temporary (connection issues, server errors)
				if err != nil {
					// Network errors are often temporary, retry
					if attempt < maxRetries-1 {
						// Exponential backoff: delay = baseDelay * 2^attempt
						delay := time.Duration(baseDelayMs*(1<<attempt)) * time.Millisecond
						time.Sleep(delay)
						continue
					}
					return "", fmt.Errorf("error fetching URL after %d attempts: %w", maxRetries, err)
				}

				// Check for server error status codes (5xx)
				if res.StatusCode >= 500 && res.StatusCode < 600 && attempt < maxRetries-1 {
					// Close the response body to avoid resource leaks
					_ = res.Body.Close()
					// Exponential backoff
					delay := time.Duration(baseDelayMs*(1<<attempt)) * time.Millisecond
					time.Sleep(delay)
					continue
				}

				// Break the retry loop if we've got a response
				break
			}

			// Ensure we close the response body when done
			if res != nil {
				defer func() {
					_ = res.Body.Close()
				}()
			}

			// Check if we received a successful response
			if res == nil {
				return "", errors.New("no response received after retries")
			}

			// Check for client error status codes (4xx) or server error status codes (5xx)
			// that weren't resolved through retries
			if res.StatusCode >= 400 {
				return "", fmt.Errorf("received HTTP error: %s (status code: %d)", res.Status, res.StatusCode)
			}

			// Read the response body
			body, err := io.ReadAll(res.Body)
			if err != nil {
				return "", fmt.Errorf("error reading response body: %w", err)
			}

			return string(body), nil
		},
	}
}
