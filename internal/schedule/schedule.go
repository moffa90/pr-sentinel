package schedule

import (
	"fmt"
	"strings"
	"time"
)

// Config defines a time window during which the daemon is allowed to run.
// An empty Config (all zero values) means always active.
type Config struct {
	Days      []string `yaml:"days"`
	StartTime string   `yaml:"start_time"`
	EndTime   string   `yaml:"end_time"`
	Timezone  string   `yaml:"timezone"`
}

// IsEmpty reports true when no schedule has been configured.
func (c Config) IsEmpty() bool {
	return len(c.Days) == 0 && c.StartTime == "" && c.EndTime == ""
}

// IsActive reports whether the given time falls within the configured schedule
// window. An empty config is always active.
func (c Config) IsActive(t time.Time) bool {
	if c.IsEmpty() {
		return true
	}

	loc, err := c.location()
	if err != nil {
		// Invalid timezone — treat as inactive to be safe.
		return false
	}

	now := t.In(loc)

	startH, startM, err := parseTime(c.StartTime)
	if err != nil {
		return false
	}
	endH, endM, err := parseTime(c.EndTime)
	if err != nil {
		return false
	}

	nowMinutes := now.Hour()*60 + now.Minute()
	startMinutes := startH*60 + startM
	endMinutes := endH*60 + endM

	overnight := endMinutes < startMinutes

	if !overnight {
		// Normal window: start <= now < end, today must be an allowed day.
		if nowMinutes < startMinutes || nowMinutes >= endMinutes {
			return false
		}
		return c.isDayAllowed(now.Weekday())
	}

	// Overnight window (e.g. 22:00–06:00).
	if nowMinutes >= startMinutes {
		// After-start portion — window opened today.
		return c.isDayAllowed(now.Weekday())
	}
	if nowMinutes < endMinutes {
		// Before-end portion — window opened yesterday.
		yesterday := now.AddDate(0, 0, -1)
		return c.isDayAllowed(yesterday.Weekday())
	}
	return false
}

// NextWindowOpen returns the next time (after t) when the schedule window
// opens. Returns the zero time if the config is empty.
func (c Config) NextWindowOpen(t time.Time) time.Time {
	if c.IsEmpty() {
		return time.Time{}
	}

	loc, err := c.location()
	if err != nil {
		return time.Time{}
	}

	startH, startM, err := parseTime(c.StartTime)
	if err != nil {
		return time.Time{}
	}
	endH, endM, err := parseTime(c.EndTime)
	if err != nil {
		return time.Time{}
	}

	startMinutes := startH*60 + startM
	endMinutes := endH*60 + endM
	overnight := endMinutes < startMinutes

	now := t.In(loc)

	// Search up to 8 days out.
	for daysAhead := 0; daysAhead <= 8; daysAhead++ {
		candidate := now.AddDate(0, 0, daysAhead)
		candidateMinutes := candidate.Hour()*60 + candidate.Minute()

		// Build the window-open instant for this calendar day.
		windowOpen := time.Date(
			candidate.Year(), candidate.Month(), candidate.Day(),
			startH, startM, 0, 0, loc,
		)

		if !overnight {
			// Window opens at start on the same day.
			if c.isDayAllowed(windowOpen.Weekday()) && windowOpen.After(t) {
				return windowOpen
			}
			continue
		}

		// Overnight: window opens at start on allowed days.
		// Also check if yesterday's window is still open and would open again.
		if c.isDayAllowed(windowOpen.Weekday()) {
			if daysAhead == 0 && candidateMinutes >= startMinutes {
				// Current day window already opened — next open is tomorrow.
				continue
			}
			if windowOpen.After(t) {
				return windowOpen
			}
		}
	}

	return time.Time{}
}

// Validate checks that the Config fields are well-formed.
func (c Config) Validate() error {
	if c.IsEmpty() {
		return nil
	}

	if c.StartTime == "" && c.EndTime == "" && len(c.Days) > 0 {
		// Days-only schedule without times is acceptable only if both times are
		// unset. But the spec says "days not empty when times are set", which
		// implies times require days. Days alone without times is ambiguous —
		// treat as valid (all-day restriction).
	}

	if (c.StartTime != "" || c.EndTime != "") && len(c.Days) == 0 {
		return fmt.Errorf("schedule: days must not be empty when start_time or end_time is set")
	}

	for _, d := range c.Days {
		if _, err := parseDay(d); err != nil {
			return err
		}
	}

	if c.StartTime != "" {
		if _, _, err := parseTime(c.StartTime); err != nil {
			return fmt.Errorf("schedule: invalid start_time %q: %w", c.StartTime, err)
		}
	}
	if c.EndTime != "" {
		if _, _, err := parseTime(c.EndTime); err != nil {
			return fmt.Errorf("schedule: invalid end_time %q: %w", c.EndTime, err)
		}
	}

	if c.Timezone != "" {
		if _, err := time.LoadLocation(c.Timezone); err != nil {
			return fmt.Errorf("schedule: invalid timezone %q: %w", c.Timezone, err)
		}
	}

	return nil
}

// parseDay converts a 3-letter weekday abbreviation (case-insensitive) to a
// time.Weekday.
func parseDay(s string) (time.Weekday, error) {
	switch strings.ToLower(s) {
	case "sun":
		return time.Sunday, nil
	case "mon":
		return time.Monday, nil
	case "tue":
		return time.Tuesday, nil
	case "wed":
		return time.Wednesday, nil
	case "thu":
		return time.Thursday, nil
	case "fri":
		return time.Friday, nil
	case "sat":
		return time.Saturday, nil
	default:
		return 0, fmt.Errorf("schedule: invalid day %q: must be one of sun, mon, tue, wed, thu, fri, sat", s)
	}
}

// location returns the time.Location for the config's Timezone. Falls back to
// UTC when Timezone is empty.
func (c Config) location() (*time.Location, error) {
	if c.Timezone == "" {
		return time.UTC, nil
	}
	return time.LoadLocation(c.Timezone)
}

// isDayAllowed reports whether wd appears in c.Days.
func (c Config) isDayAllowed(wd time.Weekday) bool {
	for _, d := range c.Days {
		allowed, err := parseDay(d)
		if err != nil {
			continue
		}
		if allowed == wd {
			return true
		}
	}
	return false
}

// parseTime parses a "HH:MM" string and returns (hour, minute, error).
func parseTime(s string) (int, int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, err
	}
	return t.Hour(), t.Minute(), nil
}
