package retry

import (
	"errors"
	"testing"
	"time"
)

func TestDoRetriesThenSucceeds(t *testing.T) {
	calls := 0
	err := Do(3, time.Millisecond, "op", func() error {
		calls++
		if calls < 2 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Errorf("err=%v calls=%d, want nil/2", err, calls)
	}
}

func TestDoStopsOnStop(t *testing.T) {
	permanent := errors.New("permanent")
	calls := 0
	err := Do(3, time.Hour, "op", func() error {
		calls++
		return Stop(permanent)
	})
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry, no sleep)", calls)
	}
	if !errors.Is(err, permanent) {
		t.Errorf("err = %v, want it to wrap the permanent error", err)
	}
	if Stop(nil) != nil {
		t.Error("Stop(nil) should be nil")
	}
}
