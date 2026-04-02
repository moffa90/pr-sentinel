package retry

import (
	"fmt"
	"log/slog"
	"time"
)

// Do retries fn up to maxAttempts times with exponential backoff starting
// at baseDelay. Returns the last error if all attempts fail.
func Do(maxAttempts int, baseDelay time.Duration, desc string, fn func() error) error {
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
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
