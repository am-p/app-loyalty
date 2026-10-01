package repository

import (
	"testing"
	"time"
)

func TestTrialEndCalendarMonth(t *testing.T) {
	for _, tt := range []struct{ start, end string }{
		{"2026-01-31T12:34:56-03:00", "2026-02-28T12:34:56-03:00"},
		{"2028-01-31T12:34:56-03:00", "2028-02-29T12:34:56-03:00"},
		{"2026-08-31T12:34:56-03:00", "2026-09-30T12:34:56-03:00"},
		{"2026-12-31T12:34:56-03:00", "2027-01-31T12:34:56-03:00"},
		{"2026-10-01T01:00:00Z", "2026-10-30T22:00:00-03:00"},
	} {
		start, _ := time.Parse(time.RFC3339, tt.start)
		want, _ := time.Parse(time.RFC3339, tt.end)
		if got := TrialEnd(start); !got.Equal(want) {
			t.Fatalf("%s: got %s want %s", tt.start, got, want)
		}
	}
}
