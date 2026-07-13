package service

import (
	"testing"
	"time"
)

// TestCalculateLeaveHoursWithinPolicy verifies full, partial, and multi-day break deductions.
func TestCalculateLeaveHoursWithinPolicy(t *testing.T) {
	work := AttendancePolicyWorkTime{
		StandardStart: "09:00",
		StandardEnd:   "17:00",
		BreakStart:    "12:00",
		BreakEnd:      "13:00",
	}
	parse := func(value string) time.Time {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatalf("parse test time %q: %v", value, err)
		}
		return parsed
	}

	tests := []struct {
		name  string
		start string
		end   string
		want  float64
	}{
		{name: "full day is seven hours", start: "2026-07-09T09:00:00+08:00", end: "2026-07-09T17:00:00+08:00", want: 7},
		{name: "partial day subtracts overlapping break", start: "2026-07-09T09:00:00+08:00", end: "2026-07-09T14:00:00+08:00", want: 4},
		{name: "outside hours are clipped", start: "2026-07-09T08:00:00+08:00", end: "2026-07-09T18:00:00+08:00", want: 7},
		{name: "two days use policy per day", start: "2026-07-09T09:00:00+08:00", end: "2026-07-10T17:00:00+08:00", want: 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calculateLeaveHoursWithinPolicy(parse(tt.start), parse(tt.end), work); got != tt.want {
				t.Fatalf("calculateLeaveHoursWithinPolicy() = %v, want %v", got, tt.want)
			}
		})
	}
}
