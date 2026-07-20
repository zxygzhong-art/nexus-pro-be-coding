package service_test

import (
	"testing"
	"time"

	"nexus-pro-api/internal/service"
)

// TestCalculateLeaveHoursWithinPolicy verifies full, partial, and multi-day break deductions.
func TestCalculateLeaveHoursWithinPolicy(t *testing.T) {
	work := service.AttendancePolicyWorkTime{
		StandardStart: "09:00",
		StandardEnd:   "17:00",
		BreakStart:    "12:00",
		BreakEnd:      "13:00",
		Weekend:       "週六、週日",
	}
	parse := func(value string) time.Time {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatalf("parse test time %q: %v", value, err)
		}
		return parsed
	}

	tests := []struct {
		name    string
		start   string
		end     string
		weekend string
		want    float64
	}{
		{name: "full day is seven hours", start: "2026-07-09T09:00:00+08:00", end: "2026-07-09T17:00:00+08:00", want: 7},
		{name: "partial day subtracts overlapping break", start: "2026-07-09T09:00:00+08:00", end: "2026-07-09T14:00:00+08:00", want: 4},
		{name: "outside hours are clipped", start: "2026-07-09T08:00:00+08:00", end: "2026-07-09T18:00:00+08:00", want: 7},
		{name: "two days use policy per day", start: "2026-07-09T09:00:00+08:00", end: "2026-07-10T17:00:00+08:00", want: 14},
		{name: "cross weekend skips configured weekend", start: "2026-07-10T09:00:00+08:00", end: "2026-07-13T17:00:00+08:00", want: 14},
		{name: "sunday-only weekend counts saturday", start: "2026-07-11T09:00:00+08:00", end: "2026-07-13T17:00:00+08:00", weekend: "週日", want: 14},
		{name: "no weekend counts saturday and sunday", start: "2026-07-11T09:00:00+08:00", end: "2026-07-12T17:00:00+08:00", weekend: "無", want: 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effectiveWork := work
			if tt.weekend != "" {
				effectiveWork.Weekend = tt.weekend
			}
			if got := service.CalculateLeaveHoursWithinPolicy(parse(tt.start), parse(tt.end), effectiveWork); got != tt.want {
				t.Fatalf("CalculateLeaveHoursWithinPolicy() = %v, want %v", got, tt.want)
			}
		})
	}
}
