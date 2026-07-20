package service_test

import (
	"testing"
	"time"

	"nexus-pro-api/internal/service"
)

func TestNormalizeEHRMSAttendanceSinceDefaultsToOneMonthWindow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	since, err := service.NormalizeEHRMSAttendanceSince("", now, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if since != "2026-06-08" {
		t.Fatalf("expected default since 2026-06-08, got %q", since)
	}
}

func TestNormalizeEHRMSAttendanceSinceClampsOlderExplicitSince(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	since, err := service.NormalizeEHRMSAttendanceSince("2026-01-01", now, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if since != "2026-06-08" {
		t.Fatalf("expected clamped since 2026-06-08, got %q", since)
	}
}

func TestNormalizeEHRMSAttendanceSinceKeepsRecentExplicitSince(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	since, err := service.NormalizeEHRMSAttendanceSince("2026-07-01", now, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if since != "2026-07-01" {
		t.Fatalf("expected explicit since 2026-07-01, got %q", since)
	}
}
