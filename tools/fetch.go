package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"maragu.dev/gai"
)

// Constants for output format values
const (
	outputFormatHTML     = "html"
	outputFormatMarkdown = "markdown"
)

// chatCompleterConverter handles HTML to Markdown conversion using ChatCompleter
type chatCompleterConverter struct {
	completer gai.ChatCompleter
}

// newChatCompleterConverter creates a new converter using the provided ChatCompleter
func newChatCompleterConverter(completer gai.ChatCompleter) *chatCompleterConverter {
	return &chatCompleterConverter{
		completer: completer,
	}
}

// ConvertHTMLToMarkdown converts HTML to Markdown using the ChatCompleter
func (c *chatCompleterConverter) ConvertHTMLToMarkdown(ctx context.Context, html string) (string, error) {
	systemPrompt := "You are a helpful assistant that converts HTML to Markdown. " +
		"Preserve the semantic structure of the document. " +
		"Only respond with the Markdown content, with no additional text."

	// Create chat complete request
	req := gai.ChatCompleteRequest{
		System: &systemPrompt,
		Messages: []gai.Message{
			gai.NewUserTextMessage(html),
		},
	}

	// Send the request to the ChatCompleter
	resp, err := c.completer.ChatComplete(ctx, req)
	if err != nil {
		return "", fmt.Errorf("error converting HTML to Markdown: %w", err)
	}

	// Collect all text parts from the response
	var markdown strings.Builder
	var convErr error

	// Iterate over all parts and collect text parts
	for part, err := range resp.Parts() {
		if err != nil {
			convErr = fmt.Errorf("error reading response parts: %w", err)
			break
		}
		if part.Type == gai.MessagePartTypeText {
			markdown.WriteString(part.Text())
		}
	}

	if convErr != nil {
		return "", convErr
	}

	return markdown.String(), nil
}

// FetchArgs holds the arguments for the Fetch tool
type FetchArgs struct {
	URL          string `json:"url" jsonschema_description:"The URL to fetch."`
	OutputFormat string `json:"output_format,omitempty" jsonschema_description:"Format for the output: 'html' or 'markdown' (default is markdown if a converter is available, otherwise html)."`
}

// NewFetch creates a new tool for fetching content from a URL
// If completer is provided, it will be used to convert HTML to Markdown when requested.
func NewFetch(client *http.Client, completer gai.ChatCompleter) gai.Tool {
	// If no client is provided, create one with default settings
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	// Create a converter from the ChatCompleter if one is provided
	var converter *chatCompleterConverter
	if completer != nil {
		converter = newChatCompleterConverter(completer)
	}

	return gai.Tool{
		Name:        "fetch",
		Description: "Fetch an HTML site and output the results as a string or Markdown. Follows redirects automatically.",
		Schema:      gai.GenerateSchema[FetchArgs](),
		Summarize: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args FetchArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "error parsing arguments", nil
			}

			// Start with URL
			summary := fmt.Sprintf(`url="%s"`, args.URL)

			// Add format if explicitly specified
			if args.OutputFormat != "" {
				summary += fmt.Sprintf(` format="%s"`, args.OutputFormat)
			}

			return summary, nil
		},
		Function: func(ctx context.Context, rawArgs json.RawMessage) (string, error) {
			var args FetchArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return "", fmt.Errorf("error unmarshaling fetch args from JSON: %w", err)
			}

			if args.URL == "" {
				return "", errors.New("url cannot be empty")
			}

			// Set default output format to markdown if a converter is available, otherwise html
			outputFormat := args.OutputFormat
			if outputFormat != "" && outputFormat != outputFormatHTML && outputFormat != outputFormatMarkdown {
				return "", fmt.Errorf("unsupported output format: %s. Supported formats are '%s' and '%s'", outputFormat, outputFormatHTML, outputFormatMarkdown)
			}
			if outputFormat == "" {
				if converter != nil {
					outputFormat = outputFormatMarkdown
				} else {
					outputFormat = outputFormatHTML
				}
			}

			// Error if markdown is requested but no converter is available
			if outputFormat == outputFormatMarkdown && converter == nil {
				return "", errors.New("markdown output requested but no converter is available")
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

			// Get the content as string
			htmlContent := string(body)

			// Convert HTML to Markdown if requested
			if outputFormat == outputFormatMarkdown {
				// We already checked earlier that converter is not nil when markdown is requested
				markdownContent, err := converter.ConvertHTMLToMarkdown(ctx, htmlContent)
				if err != nil {
					return "", fmt.Errorf("error converting HTML to Markdown: %w", err)
				}
				return markdownContent, nil
			}

			// Otherwise return HTML content
			return htmlContent, nil
		},
	}
}
