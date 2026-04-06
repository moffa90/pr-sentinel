package schedule

import (
	"testing"
	"time"
)

// mustTime parses "2006-01-02 15:04" in the given location and panics on error.
func mustTime(s, tz string) time.Time {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		panic(err)
	}
	t, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
	if err != nil {
		panic(err)
	}
	return t
}

func TestIsEmpty(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"zero value", Config{}, true},
		{"days only", Config{Days: []string{"mon"}}, false},
		{"times only", Config{StartTime: "09:00", EndTime: "17:00"}, false},
		{"full config", Config{Days: []string{"mon"}, StartTime: "09:00", EndTime: "17:00"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.IsEmpty(); got != tc.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsActive(t *testing.T) {
	// 2024-01-15 is a Monday; 2024-01-13 is Saturday; 2024-01-14 is Sunday.
	cases := []struct {
		name string
		cfg  Config
		t    time.Time
		want bool
	}{
		{
			name: "empty config always active",
			cfg:  Config{},
			t:    mustTime("2024-01-15 10:00", "UTC"),
			want: true,
		},
		{
			name: "weekday within window",
			cfg:  Config{Days: []string{"mon", "tue", "wed", "thu", "fri"}, StartTime: "09:00", EndTime: "17:00", Timezone: "UTC"},
			t:    mustTime("2024-01-15 10:00", "UTC"), // Monday 10:00
			want: true,
		},
		{
			name: "weekday too early",
			cfg:  Config{Days: []string{"mon", "tue", "wed", "thu", "fri"}, StartTime: "09:00", EndTime: "17:00", Timezone: "UTC"},
			t:    mustTime("2024-01-15 08:59", "UTC"), // Monday 08:59
			want: false,
		},
		{
			name: "weekday too late",
			cfg:  Config{Days: []string{"mon", "tue", "wed", "thu", "fri"}, StartTime: "09:00", EndTime: "17:00", Timezone: "UTC"},
			t:    mustTime("2024-01-15 17:00", "UTC"), // Monday 17:00 — end is exclusive
			want: false,
		},
		{
			name: "weekend excluded",
			cfg:  Config{Days: []string{"mon", "tue", "wed", "thu", "fri"}, StartTime: "09:00", EndTime: "17:00", Timezone: "UTC"},
			t:    mustTime("2024-01-13 10:00", "UTC"), // Saturday
			want: false,
		},
		{
			name: "overnight window before midnight (active)",
			cfg:  Config{Days: []string{"mon", "tue", "wed", "thu", "fri"}, StartTime: "22:00", EndTime: "06:00", Timezone: "UTC"},
			t:    mustTime("2024-01-15 23:00", "UTC"), // Monday 23:00
			want: true,
		},
		{
			name: "overnight window after midnight (active — yesterday was allowed)",
			cfg:  Config{Days: []string{"mon", "tue", "wed", "thu", "fri"}, StartTime: "22:00", EndTime: "06:00", Timezone: "UTC"},
			t:    mustTime("2024-01-16 02:00", "UTC"), // Tuesday 02:00 — window opened Monday
			want: true,
		},
		{
			name: "overnight window outside gap (between end and start)",
			cfg:  Config{Days: []string{"mon", "tue", "wed", "thu", "fri"}, StartTime: "22:00", EndTime: "06:00", Timezone: "UTC"},
			t:    mustTime("2024-01-15 12:00", "UTC"), // Monday 12:00 — gap
			want: false,
		},
		{
			name: "exact start time is active",
			cfg:  Config{Days: []string{"mon"}, StartTime: "09:00", EndTime: "17:00", Timezone: "UTC"},
			t:    mustTime("2024-01-15 09:00", "UTC"),
			want: true,
		},
		{
			name: "exact end time is not active",
			cfg:  Config{Days: []string{"mon"}, StartTime: "09:00", EndTime: "17:00", Timezone: "UTC"},
			t:    mustTime("2024-01-15 17:00", "UTC"),
			want: false,
		},
		{
			name: "all days active",
			cfg:  Config{Days: []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}, StartTime: "00:00", EndTime: "23:59", Timezone: "UTC"},
			t:    mustTime("2024-01-13 12:00", "UTC"), // Saturday
			want: true,
		},
		{
			name: "timezone conversion — in window in local tz",
			cfg: Config{
				Days:      []string{"mon"},
				StartTime: "09:00",
				EndTime:   "17:00",
				Timezone:  "America/New_York",
			},
			// 2024-01-15 14:00 UTC = 09:00 EST (UTC-5) — Monday, exactly at start
			t:    time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "timezone conversion — outside window in local tz",
			cfg: Config{
				Days:      []string{"mon"},
				StartTime: "09:00",
				EndTime:   "17:00",
				Timezone:  "America/New_York",
			},
			// 2024-01-15 13:59 UTC = 08:59 EST — one minute before start
			t:    time.Date(2024, 1, 15, 13, 59, 0, 0, time.UTC),
			want: false,
		},
		{
			name: "default timezone (empty = system local)",
			cfg:  Config{Days: []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}, StartTime: "00:00", EndTime: "23:59"},
			t:    time.Now(),
			want: true,
		},
		{
			name: "invalid timezone fails open",
			cfg: Config{
				Days:      []string{"mon"},
				StartTime: "08:00",
				EndTime:   "17:00",
				Timezone:  "Not/A/Place",
			},
			t:    time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "overnight — window after midnight, yesterday was weekend (not allowed)",
			cfg:  Config{Days: []string{"mon", "tue", "wed", "thu", "fri"}, StartTime: "22:00", EndTime: "06:00", Timezone: "UTC"},
			// Sunday 01:00 — yesterday was Saturday, not in days list
			t:    mustTime("2024-01-14 01:00", "UTC"),
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.cfg.IsActive(tc.t)
			if got != tc.want {
				t.Errorf("IsActive(%v) = %v, want %v", tc.t, got, tc.want)
			}
		})
	}
}

func TestNextWindowOpen(t *testing.T) {
	cases := []struct {
		name     string
		cfg      Config
		t        time.Time
		wantZero bool
		wantTime time.Time
	}{
		{
			name:     "empty config returns zero",
			cfg:      Config{},
			t:        mustTime("2024-01-15 10:00", "UTC"),
			wantZero: true,
		},
		{
			name: "next open is today",
			cfg:  Config{Days: []string{"mon"}, StartTime: "09:00", EndTime: "17:00", Timezone: "UTC"},
			// Monday 08:00 — window opens at 09:00 same day
			t:        mustTime("2024-01-15 08:00", "UTC"),
			wantTime: mustTime("2024-01-15 09:00", "UTC"),
		},
		{
			name: "window already passed today — next is same day next week",
			cfg:  Config{Days: []string{"mon"}, StartTime: "09:00", EndTime: "17:00", Timezone: "UTC"},
			// Monday 18:00 — window closed, next is Monday next week
			t:        mustTime("2024-01-15 18:00", "UTC"),
			wantTime: mustTime("2024-01-22 09:00", "UTC"),
		},
		{
			name: "currently inside window — next open is next occurrence",
			cfg:  Config{Days: []string{"mon"}, StartTime: "09:00", EndTime: "17:00", Timezone: "UTC"},
			// Monday 10:00 — currently active, next open is next Monday
			t:        mustTime("2024-01-15 10:00", "UTC"),
			wantTime: mustTime("2024-01-22 09:00", "UTC"),
		},
		{
			name: "overnight — before window opens today",
			cfg: Config{
				Days:      []string{"mon", "tue", "wed", "thu", "fri"},
				StartTime: "22:00",
				EndTime:   "06:00",
				Timezone:  "UTC",
			},
			t:        time.Date(2026, 4, 6, 20, 0, 0, 0, time.UTC), // Monday 8pm
			wantTime: time.Date(2026, 4, 6, 22, 0, 0, 0, time.UTC), // Monday 10pm same day
		},
		{
			name: "overnight — after window already opened",
			cfg: Config{
				Days:      []string{"mon", "tue", "wed", "thu", "fri"},
				StartTime: "22:00",
				EndTime:   "06:00",
				Timezone:  "UTC",
			},
			t:        time.Date(2026, 4, 6, 23, 30, 0, 0, time.UTC), // Monday 11:30pm
			wantTime: time.Date(2026, 4, 7, 22, 0, 0, 0, time.UTC),  // Tuesday 10pm next day
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.cfg.NextWindowOpen(tc.t)
			if tc.wantZero {
				if !got.IsZero() {
					t.Errorf("NextWindowOpen() = %v, want zero", got)
				}
				return
			}
			if !got.Equal(tc.wantTime) {
				t.Errorf("NextWindowOpen() = %v, want %v", got, tc.wantTime)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "empty config is valid",
			cfg:     Config{},
			wantErr: false,
		},
		{
			name: "valid weekday schedule",
			cfg: Config{
				Days:      []string{"mon", "tue", "wed", "thu", "fri"},
				StartTime: "09:00",
				EndTime:   "17:00",
			},
			wantErr: false,
		},
		{
			name: "valid overnight schedule",
			cfg: Config{
				Days:      []string{"mon", "tue"},
				StartTime: "22:00",
				EndTime:   "06:00",
			},
			wantErr: false,
		},
		{
			name:    "invalid day abbreviation",
			cfg:     Config{Days: []string{"monday"}, StartTime: "09:00", EndTime: "17:00"},
			wantErr: true,
		},
		{
			name:    "invalid start_time format",
			cfg:     Config{Days: []string{"mon"}, StartTime: "9am", EndTime: "17:00"},
			wantErr: true,
		},
		{
			name:    "invalid end_time format",
			cfg:     Config{Days: []string{"mon"}, StartTime: "09:00", EndTime: "5pm"},
			wantErr: true,
		},
		{
			name:    "invalid timezone",
			cfg:     Config{Days: []string{"mon"}, StartTime: "09:00", EndTime: "17:00", Timezone: "NotAPlace/Unknown"},
			wantErr: true,
		},
		{
			name:    "days empty but times set",
			cfg:     Config{Days: []string{}, StartTime: "09:00", EndTime: "17:00"},
			wantErr: true,
		},
		{
			name: "start_time without end_time",
			cfg: Config{
				Days:      []string{"mon"},
				StartTime: "08:00",
			},
			wantErr: true,
		},
		{
			name: "valid with explicit timezone",
			cfg: Config{
				Days:      []string{"mon"},
				StartTime: "09:00",
				EndTime:   "17:00",
				Timezone:  "America/New_York",
			},
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestParseDay(t *testing.T) {
	cases := []struct {
		input   string
		want    time.Weekday
		wantErr bool
	}{
		{"mon", time.Monday, false},
		{"tue", time.Tuesday, false},
		{"wed", time.Wednesday, false},
		{"thu", time.Thursday, false},
		{"fri", time.Friday, false},
		{"sat", time.Saturday, false},
		{"sun", time.Sunday, false},
		{"MON", time.Monday, false}, // case-insensitive
		{"Mon", time.Monday, false},
		{"monday", 0, true},
		{"m", 0, true},
		{"", 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseDay(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("parseDay(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("parseDay(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
