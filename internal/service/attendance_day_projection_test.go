package service

import (
	"testing"
	"time"
)

// TestProjectAttendanceDayUsesEarliestInAndLatestOut verifies repeated and accidental punches do not replace stable boundaries.
func TestProjectAttendanceDayUsesEarliestInAndLatestOut(t *testing.T) {
	workDate := "2026-07-13"
	records := []AttendanceClockRecord{
		attendanceProjectionRecord("in-first", workDate, clockDirectionIn, "09:00", false),
		attendanceProjectionRecord("in-duplicate", workDate, clockDirectionIn, "09:05", false),
		attendanceProjectionRecord("out-accidental", workDate, clockDirectionOut, "10:00", false),
		attendanceProjectionRecord("out-final", workDate, clockDirectionOut, "18:00", false),
	}
	projection := projectAttendanceDay(records, nil, workDate, attendanceProjectionWorkTime(), attendanceProjectionTime(workDate, "18:00"))
	if projection.ClockIn == nil || projection.ClockIn.ID != "in-first" {
		t.Fatalf("clock in = %#v, want earliest in-first", projection.ClockIn)
	}
	if projection.ClockOut == nil || projection.ClockOut.ID != "out-final" {
		t.Fatalf("clock out = %#v, want latest out-final", projection.ClockOut)
	}
	if projection.PunchCount != 4 || projection.WorkedMinutes != 480 || projection.DayStatus != attendanceDayStatusComplete {
		t.Fatalf("projection = %#v, want 4 punches, 480 minutes, complete", projection)
	}
}

// TestProjectAttendanceDayCreditsApprovedLeave verifies a one-hour workday can close normally when approved leave covers the remainder.
func TestProjectAttendanceDayCreditsApprovedLeave(t *testing.T) {
	workDate := "2026-07-13"
	records := []AttendanceClockRecord{
		attendanceProjectionRecord("in", workDate, clockDirectionIn, "09:00", false),
		attendanceProjectionRecord("out", workDate, clockDirectionOut, "10:00", false),
	}
	leaves := []LeaveRequest{{
		EmployeeID: "emp-1",
		StartAt:    attendanceProjectionTime(workDate, "10:00"),
		EndAt:      attendanceProjectionTime(workDate, "18:00"),
		Status:     "approved",
	}}
	projection := projectAttendanceDay(records, leaves, workDate, attendanceProjectionWorkTime(), attendanceProjectionTime(workDate, "18:00"))
	if projection.WorkedMinutes != 60 || projection.ApprovedLeaveMinutes != 420 {
		t.Fatalf("minutes = worked %d leave %d, want 60 and 420", projection.WorkedMinutes, projection.ApprovedLeaveMinutes)
	}
	if projection.DayStatus != attendanceDayStatusComplete || len(projection.AnomalyReasons) != 0 {
		t.Fatalf("projection = %#v, want approved leave to complete the day", projection)
	}
}

// TestProjectAttendanceDayKeepsPendingLeaveAbnormal verifies unapproved leave does not offset required time.
func TestProjectAttendanceDayKeepsPendingLeaveAbnormal(t *testing.T) {
	workDate := "2026-07-13"
	records := []AttendanceClockRecord{
		attendanceProjectionRecord("in", workDate, clockDirectionIn, "09:00", false),
		attendanceProjectionRecord("out", workDate, clockDirectionOut, "10:00", false),
	}
	leaves := []LeaveRequest{{
		EmployeeID: "emp-1",
		StartAt:    attendanceProjectionTime(workDate, "10:00"),
		EndAt:      attendanceProjectionTime(workDate, "18:00"),
		Status:     "pending_approval",
	}}
	projection := projectAttendanceDay(records, leaves, workDate, attendanceProjectionWorkTime(), attendanceProjectionTime(workDate, "18:00"))
	if projection.ApprovedLeaveMinutes != 0 || projection.PendingLeaveMinutes != 420 || projection.DayStatus != attendanceDayStatusPendingLeave {
		t.Fatalf("projection = %#v, want pending leave without credited time", projection)
	}
}

// TestProjectAttendanceDayExcludesVoidedPunch verifies an audited mistake never becomes the last clock-out.
func TestProjectAttendanceDayExcludesVoidedPunch(t *testing.T) {
	workDate := "2026-07-13"
	records := []AttendanceClockRecord{
		attendanceProjectionRecord("in", workDate, clockDirectionIn, "09:00", false),
		attendanceProjectionRecord("out-real", workDate, clockDirectionOut, "18:00", false),
		attendanceProjectionRecord("out-mistake", workDate, clockDirectionOut, "22:00", true),
	}
	projection := projectAttendanceDay(records, nil, workDate, attendanceProjectionWorkTime(), attendanceProjectionTime(workDate, "22:00"))
	if projection.ClockOut == nil || projection.ClockOut.ID != "out-real" || projection.PunchCount != 2 {
		t.Fatalf("projection = %#v, want voided punch excluded", projection)
	}
}

// attendanceProjectionRecord builds one accepted raw punch in the business timezone.
func attendanceProjectionRecord(id, workDate, direction, hhmm string, voided bool) AttendanceClockRecord {
	at := attendanceProjectionTime(workDate, hhmm)
	return AttendanceClockRecord{
		ID:           id,
		EmployeeID:   "emp-1",
		WorkDate:     workDate,
		Direction:    direction,
		ClockedAt:    at,
		RecordStatus: clockRecordStatusAccepted,
		Voided:       voided,
		CreatedAt:    at,
	}
}

// attendanceProjectionTime parses a stable local business time for projection tests.
func attendanceProjectionTime(workDate, hhmm string) time.Time {
	value, err := time.ParseInLocation("2006-01-02 15:04", workDate+" "+hhmm, attendanceClockLocation)
	if err != nil {
		panic(err)
	}
	return value
}

// attendanceProjectionWorkTime returns the standard eight-hour fixed schedule.
func attendanceProjectionWorkTime() AttendancePolicyWorkTime {
	return AttendancePolicyWorkTime{
		ClockMode:     clockModeFixed,
		StandardStart: "09:00",
		StandardEnd:   "18:00",
		BreakStart:    "12:00",
		BreakEnd:      "13:00",
	}
}
