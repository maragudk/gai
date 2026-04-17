package robust

import (
	"context"
	"math/rand/v2"
	"time"
)

// sleep waits for the full-jitter backoff duration for the given retry number (1-indexed),
// or returns the context error if the context is cancelled first.
func sleep(ctx context.Context, baseDelay, maxDelay time.Duration, retryNumber int) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(nextDelay(baseDelay, maxDelay, retryNumber)):
		return nil
	}
}

// nextDelay returns a full-jitter backoff duration for the given retry number (1-indexed).
// The ceiling at retry n is min(maxDelay, baseDelay*2^(n-1)), so the first retry draws
// from [0, baseDelay].
func nextDelay(baseDelay, maxDelay time.Duration, retryNumber int) time.Duration {
	shift := retryNumber - 1
	exp := baseDelay << shift
	if exp <= 0 || exp > maxDelay {
		exp = maxDelay
	}
	return time.Duration(rand.Int64N(int64(exp) + 1))
}
