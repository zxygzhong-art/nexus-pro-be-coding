package service

import (
	"sort"
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
	leaves, err := c.store.ListLeaveRequestsByQuery(goContext(ctx), ctx.TenantID, LeaveRequestQuery{
		EmployeeIDs: []string{employeeID},
		Status:      "approved",
		FromDate:    start.Format(time.DateOnly),
		ToDate:      end.Format(time.DateOnly),
	})
	if err != nil {
		return AttendanceMonthlySummary{}, err
	}
	policy, err := c.loadAttendancePolicyResponse(ctx)
	if err != nil {
		return AttendanceMonthlySummary{}, err
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
	workDates := make([]string, 0, len(recordsByDate))
	for workDate := range recordsByDate {
		workDates = append(workDates, workDate)
	}
	sort.Strings(workDates)
	for _, workDate := range workDates {
		dayRecords := recordsByDate[workDate]
		projection, err := c.projectAttendanceDay(ctx, dayRecords, leaves, workDate, policy.WorkTime, c.Now())
		if err != nil {
			return AttendanceMonthlySummary{}, err
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
