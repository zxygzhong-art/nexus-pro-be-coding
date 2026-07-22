package service

import (
	"strings"
	"time"
)

// AttendanceMonthlySummary derives one employee's monthly totals from the same daily projection used by clock status.
func (c AttendanceService) AttendanceMonthlySummary(ctx RequestContext, month string) (AttendanceMonthlySummary, error) {
	account, decision, err := c.requireAttendanceAuthz(ctx, ResourceAttendanceClock, ActionRead, "")
	if err != nil {
		return AttendanceMonthlySummary{}, err
	}
	employeeID := strings.TrimSpace(account.EmployeeID)
	if employeeID == "" {
		return AttendanceMonthlySummary{}, BadRequest("employee_id is required")
	}
	if err := c.ensureAttendanceEmployeeAllowed(ctx, account, decision, employeeID); err != nil {
		return AttendanceMonthlySummary{}, err
	}

	monthKey, start, end, err := attendanceMonthRange(month, c.Now())
	if err != nil {
		return AttendanceMonthlySummary{}, err
	}
	records, err := c.store.ListAttendanceClockRecords(goContext(ctx), ctx.TenantID, AttendanceClockRecordQuery{
		EmployeeID: employeeID,
		FromDate:   start.Format(time.DateOnly),
		ToDate:     end.Format(time.DateOnly),
	})
	if err != nil {
		return AttendanceMonthlySummary{}, err
	}
	leaves, err := c.loadEffectiveAttendanceLeaves(ctx, []string{employeeID}, start.Format(time.DateOnly), end.Format(time.DateOnly))
	if err != nil {
		return AttendanceMonthlySummary{}, err
	}
	storedProjections, err := c.store.ListAttendanceDayProjections(goContext(ctx), ctx.TenantID, []string{employeeID}, start.Format(time.DateOnly), end.Format(time.DateOnly))
	if err != nil {
		return AttendanceMonthlySummary{}, err
	}
	persistedDates := make(map[string]struct{}, len(storedProjections))
	for _, projection := range storedProjections {
		persistedDates[projection.WorkDate] = struct{}{}
	}

	result := AttendanceMonthlySummary{
		EmployeeID:  employeeID,
		Month:       monthKey,
		RecordCount: len(records),
		Days:        make([]AttendanceMonthlyDaySummary, 0),
	}
	recordsByDate := make(map[string][]AttendanceClockRecord)
	attendanceDates := make(map[string]struct{})
	for _, record := range records {
		recordsByDate[record.WorkDate] = append(recordsByDate[record.WorkDate], record)
		if record.RecordStatus == clockRecordStatusAccepted && !record.Voided && record.Direction == clockDirectionIn {
			attendanceDates[record.WorkDate] = struct{}{}
		}
	}
	result.AttendanceDays = len(attendanceDates)
	workDates := attendanceMonthlyProjectionDates(recordsByDate, leaves, persistedDates, start, end)
	now := c.Now()
	for _, workDate := range workDates {
		dayRecords := recordsByDate[workDate]
		policy, err := c.loadAttendancePolicyResponseForWorkDate(ctx, workDate)
		if err != nil {
			return AttendanceMonthlySummary{}, err
		}
		projection, err := c.projectAndPersistAttendanceDay(ctx, employeeID, dayRecords, leaves, workDate, policy, now)
		if err != nil {
			return AttendanceMonthlySummary{}, err
		}
		if len(dayRecords) == 0 && projection.ApprovedLeaveMinutes == 0 && projection.PendingLeaveMinutes == 0 {
			continue
		}
		workedMinutes := 0
		if projection.ClockIn != nil && projection.ClockOut != nil {
			workedMinutes = projection.WorkedMinutes
			result.WorkedMinutes += workedMinutes
		}
		if projection.DayStatus == attendanceDayStatusAbnormal {
			result.AbnormalDays++
		}
		result.Days = append(result.Days, AttendanceMonthlyDaySummary{
			WorkDate:       workDate,
			WorkedMinutes:  workedMinutes,
			RecordCount:    len(dayRecords),
			DayStatus:      projection.DayStatus,
			AnomalyReasons: projection.AnomalyReasons,
		})
	}
	return result, nil
}

// attendanceMonthlyProjectionDates returns the union of raw-punch dates and
// leave-only dates. A day is retained only as a candidate here; the policy-aware
// projection performs the final schedule clipping before it enters the report.
func attendanceMonthlyProjectionDates(recordsByDate map[string][]AttendanceClockRecord, leaves []attendanceEffectiveLeave, persistedDates map[string]struct{}, start, end time.Time) []string {
	out := make([]string, 0, len(recordsByDate))
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		workDate := day.Format(time.DateOnly)
		_, persisted := persistedDates[workDate]
		if len(recordsByDate[workDate]) > 0 || persisted || attendanceEffectiveLeaveOverlapsDay(leaves, day) {
			out = append(out, workDate)
		}
	}
	return out
}

func attendanceEffectiveLeaveOverlapsDay(leaves []attendanceEffectiveLeave, day time.Time) bool {
	// A work date may own an overnight schedule ending on the next calendar day.
	end := day.AddDate(0, 0, 2)
	for _, leave := range leaves {
		if leave.StartAt.Before(end) && leave.EndAt.After(day) {
			return true
		}
	}
	return false
}

// attendanceMonthRange validates YYYY-MM and returns inclusive business-time-zone boundaries.
func attendanceMonthRange(raw string, now time.Time) (string, time.Time, time.Time, error) {
	month := strings.TrimSpace(raw)
	if month == "" {
		month = now.In(attendanceClockLocation).Format("2006-01")
	}
	start, err := time.ParseInLocation("2006-01", month, attendanceClockLocation)
	if err != nil || start.Format("2006-01") != month {
		return "", time.Time{}, time.Time{}, BadRequest("month must be YYYY-MM")
	}
	return month, start, start.AddDate(0, 1, -1), nil
}
