package v1

import (
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
)

func TestDefaultManualEHRMSAttendanceSyncRangeUsesShanghaiToday(t *testing.T) {
	input := domain.EHRMSAttendanceSyncInput{}
	now := time.Date(2026, time.July, 22, 16, 30, 0, 0, time.UTC)

	defaultManualEHRMSAttendanceSyncRange(&input, now)

	if input.Start != "2026-07-23" || input.End != "2026-07-24" {
		t.Fatalf("expected Shanghai today bounds, got start=%q end=%q", input.Start, input.End)
	}
}

func TestDefaultManualEHRMSAttendanceSyncRangePreservesExplicitBounds(t *testing.T) {
	input := domain.EHRMSAttendanceSyncInput{Start: "2026-01-01", End: "2026-02-01"}

	defaultManualEHRMSAttendanceSyncRange(&input, time.Now())

	if input.Start != "2026-01-01" || input.End != "2026-02-01" {
		t.Fatalf("expected explicit bounds to be preserved, got start=%q end=%q", input.Start, input.End)
	}
}
