package retry

import (
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// stopError marks an error that retrying cannot fix.
type stopError struct{ err error }

func (e stopError) Error() string { return e.err.Error() }
func (e stopError) Unwrap() error { return e.err }

// Stop wraps err so Do returns it at once instead of retrying. errors.Is and
// errors.As still see the wrapped error.
func Stop(err error) error {
	if err == nil {
		return nil
	}
	return stopError{err}
}

// Do retries fn up to maxAttempts times with exponential backoff starting
// at baseDelay. Returns the last error if all attempts fail.
func Do(maxAttempts int, baseDelay time.Duration, desc string, fn func() error) error {
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		var stop stopError
		if errors.As(err, &stop) {
			return fmt.Errorf("%s failed: %w", desc, stop.err)
		}
		if attempt < maxAttempts {
			delay := baseDelay * (1 << (attempt - 1))
			slog.Warn("retrying after failure",
				"operation", desc,
				"attempt", attempt,
				"max", maxAttempts,
				"delay", delay,
				"error", err,
			)
			time.Sleep(delay)
		}
	}
	return fmt.Errorf("%s failed after %d attempts: %w", desc, maxAttempts, err)
}
