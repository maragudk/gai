package robust

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
)

// defaultErrorClassifier is used when [NewChatCompleterOptions.ErrorClassifier] or
// [NewEmbedderOptions.ErrorClassifier] is nil.
// It applies these rules in order:
//  1. [context.Canceled] and [context.DeadlineExceeded] → [ActionFail].
//  2. A 4xx/5xx HTTP status code found in the error string classifies by status.
//  3. Anything else → [ActionRetry] (optimistic default).
//
// The string-inspection step is best-effort; callers who want precise, SDK-aware behavior
// should supply their own [ErrorClassifierFunc]. See issue #210 for a planned gai-native
// error type that would let this classifier match on interfaces instead.
func defaultErrorClassifier(err error) Action {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ActionFail
	}

	if code, ok := findStatusCode(err.Error()); ok {
		return classifyStatus(code)
	}

	return ActionRetry
}

// classifyStatus maps an HTTP status code to an [Action].
// 429 and 5xx retry; other 4xx fall back; anything else retries optimistically.
func classifyStatus(code int) Action {
	switch {
	case code == http.StatusTooManyRequests:
		return ActionRetry
	case code >= http.StatusInternalServerError && code < 600:
		return ActionRetry
	case code >= http.StatusBadRequest && code < http.StatusInternalServerError:
		return ActionFallback
	default:
		return ActionRetry
	}
}

// statusCodeRe matches a bare 4xx or 5xx HTTP status code that is NOT adjacent to a digit,
// colon, dot, or forward slash, rejecting ports (`:443`), IP octets (`10.0.0.500`), path
// segments (`/503/`), and longer numbers (`5003`). Best-effort — see [defaultErrorClassifier].
var statusCodeRe = regexp.MustCompile(`(?:^|[^\w./:])([45]\d{2})(?:[^\w./:]|$)`)

// findStatusCode returns the first 4xx/5xx integer matched by [statusCodeRe] in s.
func findStatusCode(s string) (int, bool) {
	m := statusCodeRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	code, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return code, true
}
