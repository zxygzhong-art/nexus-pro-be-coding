package service_test

import (
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

// TestProjectAttendanceDayUsesEarliestInAndLatestOut verifies repeated and accidental punches do not replace stable boundaries.
func TestProjectAttendanceDayUsesEarliestInAndLatestOut(t *testing.T) {
	workDate := "2026-07-13"
	records := []domain.AttendanceClockRecord{
		attendanceProjectionRecord("in-first", workDate, "clock_in", "09:00", false),
		attendanceProjectionRecord("in-duplicate", workDate, "clock_in", "09:05", false),
		attendanceProjectionRecord("out-accidental", workDate, "clock_out", "10:00", false),
		attendanceProjectionRecord("out-final", workDate, "clock_out", "18:00", false),
	}
	projection := service.ProjectAttendanceDay(records, nil, workDate, attendanceProjectionWorkTime(), attendanceProjectionTime(workDate, "18:00"))
	if projection.ClockIn == nil || projection.ClockIn.ID != "in-first" {
		t.Fatalf("clock in = %#v, want earliest in-first", projection.ClockIn)
	}
	if projection.ClockOut == nil || projection.ClockOut.ID != "out-final" {
		t.Fatalf("clock out = %#v, want latest out-final", projection.ClockOut)
	}
	if projection.PunchCount != 4 || projection.WorkedMinutes != 480 || projection.DayStatus != "complete" {
		t.Fatalf("projection = %#v, want 4 punches, 480 minutes, complete", projection)
	}
}

// TestProjectAttendanceDayCreditsApprovedLeave verifies a one-hour workday can close normally when approved leave covers the remainder.
func TestProjectAttendanceDayCreditsApprovedLeave(t *testing.T) {
	workDate := "2026-07-13"
	records := []domain.AttendanceClockRecord{
		attendanceProjectionRecord("in", workDate, "clock_in", "09:00", false),
		attendanceProjectionRecord("out", workDate, "clock_out", "10:00", false),
	}
	leaves := []domain.LeaveRequest{{
		EmployeeID: "emp-1",
		StartAt:    attendanceProjectionTime(workDate, "10:00"),
		EndAt:      attendanceProjectionTime(workDate, "18:00"),
		Status:     "approved",
	}}
	projection := service.ProjectAttendanceDay(records, leaves, workDate, attendanceProjectionWorkTime(), attendanceProjectionTime(workDate, "18:00"))
	if projection.WorkedMinutes != 60 || projection.ApprovedLeaveMinutes != 420 {
		t.Fatalf("minutes = worked %d leave %d, want 60 and 420", projection.WorkedMinutes, projection.ApprovedLeaveMinutes)
	}
	if projection.DayStatus != "complete" || len(projection.AnomalyReasons) != 0 {
		t.Fatalf("projection = %#v, want approved leave to complete the day", projection)
	}
}

// TestProjectAttendanceDayKeepsPendingLeaveAbnormal verifies unapproved leave does not offset required time.
func TestProjectAttendanceDayKeepsPendingLeaveAbnormal(t *testing.T) {
	workDate := "2026-07-13"
	records := []domain.AttendanceClockRecord{
		attendanceProjectionRecord("in", workDate, "clock_in", "09:00", false),
		attendanceProjectionRecord("out", workDate, "clock_out", "10:00", false),
	}
	leaves := []domain.LeaveRequest{{
		EmployeeID: "emp-1",
		StartAt:    attendanceProjectionTime(workDate, "10:00"),
		EndAt:      attendanceProjectionTime(workDate, "18:00"),
		Status:     "pending_approval",
	}}
	projection := service.ProjectAttendanceDay(records, leaves, workDate, attendanceProjectionWorkTime(), attendanceProjectionTime(workDate, "18:00"))
	if projection.ApprovedLeaveMinutes != 0 || projection.PendingLeaveMinutes != 420 || projection.DayStatus != "pending_leave" {
		t.Fatalf("projection = %#v, want pending leave without credited time", projection)
	}
}

// TestProjectAttendanceDayExcludesVoidedPunch verifies an audited mistake never becomes the last clock-out.
func TestProjectAttendanceDayExcludesVoidedPunch(t *testing.T) {
	workDate := "2026-07-13"
	records := []domain.AttendanceClockRecord{
		attendanceProjectionRecord("in", workDate, "clock_in", "09:00", false),
		attendanceProjectionRecord("out-real", workDate, "clock_out", "18:00", false),
		attendanceProjectionRecord("out-mistake", workDate, "clock_out", "22:00", true),
	}
	projection := service.ProjectAttendanceDay(records, nil, workDate, attendanceProjectionWorkTime(), attendanceProjectionTime(workDate, "22:00"))
	if projection.ClockOut == nil || projection.ClockOut.ID != "out-real" || projection.PunchCount != 2 {
		t.Fatalf("projection = %#v, want voided punch excluded", projection)
	}
}

// TestProjectAttendanceDayMarksExpiredOpenClockAbnormal verifies a past workday cannot remain "working" forever.
func TestProjectAttendanceDayMarksExpiredOpenClockAbnormal(t *testing.T) {
	workDate := "2026-07-13"
	records := []domain.AttendanceClockRecord{
		attendanceProjectionRecord("in", workDate, "clock_in", "09:00", false),
	}

	projection := service.ProjectAttendanceDay(records, nil, workDate, attendanceProjectionWorkTime(), attendanceProjectionTime("2026-07-14", "10:00"))

	if projection.DayStatus != "abnormal" {
		t.Fatalf("day status = %q, want abnormal for a past open clock", projection.DayStatus)
	}
	if len(projection.AnomalyReasons) != 1 || projection.AnomalyReasons[0] != "missing_clock_out" {
		t.Fatalf("anomaly reasons = %#v, want missing_clock_out", projection.AnomalyReasons)
	}
}

// TestProjectAttendanceDayKeepsOvernightClockWorkingUntilScheduleEnds protects cross-midnight shifts.
func TestProjectAttendanceDayKeepsOvernightClockWorkingUntilScheduleEnds(t *testing.T) {
	workDate := "2026-07-13"
	records := []domain.AttendanceClockRecord{
		attendanceProjectionRecord("in", workDate, "clock_in", "22:00", false),
	}
	workTime := attendanceProjectionWorkTime()
	workTime.StandardStart = "22:00"
	workTime.StandardEnd = "06:00"

	working := service.ProjectAttendanceDay(records, nil, workDate, workTime, attendanceProjectionTime("2026-07-14", "01:00"))
	if working.DayStatus != "working" {
		t.Fatalf("day status before overnight schedule end = %q, want working", working.DayStatus)
	}

	expired := service.ProjectAttendanceDay(records, nil, workDate, workTime, attendanceProjectionTime("2026-07-14", "07:00"))
	if expired.DayStatus != "abnormal" {
		t.Fatalf("day status after overnight schedule end = %q, want abnormal", expired.DayStatus)
	}
}

// attendanceProjectionRecord builds one accepted raw punch in the business timezone.
func attendanceProjectionRecord(id, workDate, direction, hhmm string, voided bool) domain.AttendanceClockRecord {
	at := attendanceProjectionTime(workDate, hhmm)
	return domain.AttendanceClockRecord{
		ID:           id,
		EmployeeID:   "emp-1",
		WorkDate:     workDate,
		Direction:    direction,
		ClockedAt:    at,
		RecordStatus: "accepted",
		Voided:       voided,
		CreatedAt:    at,
	}
}

// attendanceProjectionTime parses a stable local business time for projection tests.
func attendanceProjectionTime(workDate, hhmm string) time.Time {
	value, err := time.ParseInLocation("2006-01-02 15:04", workDate+" "+hhmm, time.FixedZone("UTC+8", 8*60*60))
	if err != nil {
		panic(err)
	}
	return value
}

// attendanceProjectionWorkTime returns the standard eight-hour fixed schedule.
func attendanceProjectionWorkTime() domain.AttendancePolicyWorkTime {
	return domain.AttendancePolicyWorkTime{
		ClockMode:     "fixed",
		StandardStart: "09:00",
		StandardEnd:   "18:00",
		BreakStart:    "12:00",
		BreakEnd:      "13:00",
	}
}
