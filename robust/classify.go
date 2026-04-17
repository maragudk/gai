package robust

import (
	"context"
	"errors"
	"regexp"
	"strconv"
)

// statusCoder matches any error in the tree that exposes an HTTP status code.
// Provider SDKs currently expose StatusCode as a field rather than a method, so this
// interface catches caller-wrapped errors; the regex fallback in [DefaultErrorClassifier]
// handles SDK errors by inspecting the error string.
type statusCoder interface {
	error
	StatusCode() int
}

// DefaultErrorClassifier is the classifier used when [NewChatCompleterOptions.ErrorClassifier] is nil.
// It applies the following rules in order:
//  1. [context.Canceled] and [context.DeadlineExceeded] → [ActionFail].
//  2. Any error in the tree that implements StatusCode() int is classified by status.
//  3. HTTP status codes (4xx/5xx) found in the error string are classified by status.
//  4. Anything else → [ActionRetry] (optimistic default).
//
// Callers who need tighter behavior should supply their own [ErrorClassifierFunc].
var DefaultErrorClassifier ErrorClassifierFunc = func(err error) Action {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ActionFail
	}

	var sc statusCoder
	if errors.As(err, &sc) {
		return classifyStatus(sc.StatusCode())
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
	case code == 429:
		return ActionRetry
	case code >= 500 && code < 600:
		return ActionRetry
	case code >= 400 && code < 500:
		return ActionFallback
	default:
		return ActionRetry
	}
}

// statusCodeRe matches a bare 4xx or 5xx HTTP status code as a word.
// This is best-effort string inspection; callers with provider-specific needs
// should supply a custom [ErrorClassifierFunc].
var statusCodeRe = regexp.MustCompile(`\b([45]\d{2})\b`)

// findStatusCode returns the first 4xx/5xx integer appearing as a whole word in s.
func findStatusCode(s string) (int, bool) {
	m := statusCodeRe.FindString(s)
	if m == "" {
		return 0, false
	}
	code, err := strconv.Atoi(m)
	if err != nil {
		return 0, false
	}
	return code, true
}
