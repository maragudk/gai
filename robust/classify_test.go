package robust

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"maragu.dev/is"

	"maragu.dev/gai"
)

// stubCompleter is an inert completer used only to satisfy NewChatCompleter for helper tests
// that don't actually call ChatComplete.
type stubCompleter struct{}

func (stubCompleter) ChatComplete(context.Context, gai.ChatCompleteRequest) (gai.ChatCompleteResponse, error) {
	return gai.ChatCompleteResponse{}, nil
}

func TestDefaultErrorClassifier(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected Action
	}{
		{"context.Canceled fails", context.Canceled, ActionFail},
		{"context.DeadlineExceeded fails", context.DeadlineExceeded, ActionFail},
		{"wrapped context.Canceled fails", fmt.Errorf("outer: %w", context.Canceled), ActionFail},
		{"string with 429 retries", errors.New("got HTTP 429 from provider"), ActionRetry},
		{"string with 503 retries", errors.New("status 503 service unavailable"), ActionRetry},
		{"string with 401 falls back", errors.New("401 unauthorized: bad key"), ActionFallback},
		{"string with 400 falls back", errors.New("400 Bad Request"), ActionFallback},
		{"unknown error retries optimistically", errors.New("mystery disco glitch"), ActionRetry},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			is.Equal(t, test.expected, defaultErrorClassifier(test.err))
		})
	}
}

func TestFindStatusCode(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		wantCode int
		wantOK   bool
	}{
		// Positive cases.
		{"bare 429", "429", 429, true},
		{"429 with status text", "429 Too Many Requests", 429, true},
		{"400 at start", "400 Bad Request", 400, true},
		{"503 with preceding context", "got HTTP 503 from provider", 503, true},
		{"401 with trailing colon", "401 unauthorized: bad key", 401, true},
		{"503 with status prefix", "status 503 service unavailable", 503, true},
		{"parenthesized 500", "error (500): server down", 500, true},
		{"bracketed 429", "error [429] retry later", 429, true},
		{"429 at end", "provider returned 429", 429, true},

		// Negative: adjacent to colon, dot, slash, or digit.
		{"port number with colon", "dial tcp 10.0.0.1:443: refused", 0, false},
		{"ip octet with dot", "invalid address 10.0.0.500", 0, false},
		{"path segment with slashes", "POST https://api.example.com/v1/503/check", 0, false},
		{"four digit number trailing", "5003 is not 5xx", 0, false},
		{"four digit number leading", "44321 nonsense", 0, false},
		{"digits around word", "x429y", 0, false},
		{"no status at all", "no numbers here", 0, false},
		{"dev server port with colon", "connect :5001/api failed", 0, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, ok := findStatusCode(test.s)
			is.Equal(t, test.wantOK, ok)
			is.Equal(t, test.wantCode, code)
		})
	}
}

func TestChatCompleter_nextDelay(t *testing.T) {
	t.Run("stays within [0, min(MaxDelay, BaseDelay<<(n-1))] across retries", func(t *testing.T) {
		c := NewChatCompleter(NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{stubCompleter{}},
			BaseDelay:  100 * time.Millisecond,
			MaxDelay:   10 * time.Second,
		})

		for retry := 1; retry <= 10; retry++ {
			shift := retry - 1
			want := c.baseDelay << shift
			if want <= 0 || want > c.maxDelay {
				want = c.maxDelay
			}
			for range 50 {
				d := c.nextDelay(retry)
				is.True(t, d >= 0, "delay must not be negative")
				is.True(t, d <= want, fmt.Sprintf("retry %d: delay %v exceeds cap %v", retry, d, want))
			}
		}
	})

	t.Run("first retry caps at BaseDelay, not 2*BaseDelay", func(t *testing.T) {
		c := NewChatCompleter(NewChatCompleterOptions{
			Completers: []gai.ChatCompleter{stubCompleter{}},
			BaseDelay:  50 * time.Millisecond,
			MaxDelay:   10 * time.Second,
		})
		for range 100 {
			d := c.nextDelay(1)
			is.True(t, d <= 50*time.Millisecond, fmt.Sprintf("first retry delay %v exceeds BaseDelay", d))
		}
	})
}
