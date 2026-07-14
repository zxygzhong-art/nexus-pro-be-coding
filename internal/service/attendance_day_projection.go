package service

import (
	"sort"
	"strings"
	"time"
)

const (
	attendanceDayStatusNotStarted   = "not_started"
	attendanceDayStatusWorking      = "working"
	attendanceDayStatusComplete     = "complete"
	attendanceDayStatusPendingLeave = "pending_leave"
	attendanceDayStatusAbnormal     = "abnormal"
)

// attendanceTimeInterval represents a half-open attendance interval.
type attendanceTimeInterval struct {
	Start time.Time
	End   time.Time
}

// attendanceClockStatusFromProjection exposes the stable daily projection to clients.
func attendanceClockStatusFromProjection(employeeID, workDate string, projection attendanceDayProjection) AttendanceClockStatus {
	nextAction := clockDirectionIn
	if projection.CanClockOut {
		nextAction = clockDirectionOut
	}
	return AttendanceClockStatus{
		EmployeeID:           employeeID,
		WorkDate:             workDate,
		ClockIn:              projection.ClockIn,
		ClockOut:             projection.ClockOut,
		LastPunch:            projection.LastPunch,
		PunchCount:           projection.PunchCount,
		NextAction:           nextAction,
		CanClockIn:           projection.CanClockIn,
		CanClockOut:          projection.CanClockOut,
		WorkedMinutes:        projection.WorkedMinutes,
		ApprovedLeaveMinutes: projection.ApprovedLeaveMinutes,
		RequiredMinutes:      projection.RequiredMinutes,
		DayStatus:            projection.DayStatus,
		AnomalyReasons:       projection.AnomalyReasons,
	}
}

// attendanceDayProjection is the single derived view shared by clock status and reporting.
type attendanceDayProjection struct {
	ClockIn              *AttendanceClockRecord
	ClockOut             *AttendanceClockRecord
	LastPunch            *AttendanceClockRecord
	PunchCount           int
	WorkedMinutes        int
	ApprovedLeaveMinutes int
	PendingLeaveMinutes  int
	RequiredMinutes      int
	DayStatus            string
	AnomalyReasons       []string
	CanClockIn           bool
	CanClockOut          bool
}

// loadAttendanceDayProjection loads every raw punch and overlapping leave before deriving the day state.
func (c AttendanceService) loadAttendanceDayProjection(ctx RequestContext, employeeID, workDate string, asOf time.Time) (attendanceDayProjection, error) {
	records, err := c.store.ListAttendanceClockRecords(goContext(ctx), ctx.TenantID, AttendanceClockRecordQuery{
		EmployeeID: employeeID,
		FromDate:   workDate,
		ToDate:     workDate,
	})
	if err != nil {
		return attendanceDayProjection{}, err
	}
	leaves, err := c.store.ListLeaveRequestsByQuery(goContext(ctx), ctx.TenantID, LeaveRequestQuery{
		EmployeeIDs: []string{employeeID},
		FromDate:    workDate,
		ToDate:      workDate,
	})
	if err != nil {
		return attendanceDayProjection{}, err
	}
	policy, err := c.loadAttendancePolicyResponse(ctx)
	if err != nil {
		return attendanceDayProjection{}, err
	}
	return projectAttendanceDay(records, leaves, workDate, policy.WorkTime, asOf), nil
}

// projectAttendanceDay derives stable boundaries and credited time without mutating raw punches.
func projectAttendanceDay(records []AttendanceClockRecord, leaves []LeaveRequest, workDate string, workTime AttendancePolicyWorkTime, asOf time.Time) attendanceDayProjection {
	effective := make([]AttendanceClockRecord, 0, len(records))
	for _, record := range records {
		if record.WorkDate != workDate || !strings.EqualFold(record.RecordStatus, clockRecordStatusAccepted) || record.Voided {
			continue
		}
		effective = append(effective, record)
	}
	sort.SliceStable(effective, func(i, j int) bool {
		if effective[i].ClockedAt.Equal(effective[j].ClockedAt) {
			if effective[i].CreatedAt.Equal(effective[j].CreatedAt) {
				return effective[i].ID < effective[j].ID
			}
			return effective[i].CreatedAt.Before(effective[j].CreatedAt)
		}
		return effective[i].ClockedAt.Before(effective[j].ClockedAt)
	})

	projection := attendanceDayProjection{
		PunchCount:      len(effective),
		RequiredMinutes: int(standardDayHours(workTime)*60 + 0.5),
		DayStatus:       attendanceDayStatusNotStarted,
	}
	if len(effective) > 0 {
		last := effective[len(effective)-1]
		projection.LastPunch = &last
	}
	for i := range effective {
		record := effective[i]
		switch record.Direction {
		case clockDirectionIn:
			if projection.ClockIn == nil || record.ClockedAt.Before(projection.ClockIn.ClockedAt) {
				current := record
				projection.ClockIn = &current
			}
		case clockDirectionOut:
			if projection.ClockOut == nil || record.ClockedAt.After(projection.ClockOut.ClockedAt) {
				current := record
				projection.ClockOut = &current
			}
		}
	}
	projection.CanClockIn = projection.ClockIn == nil
	projection.CanClockOut = projection.ClockIn != nil

	schedule, breaks := attendanceScheduleIntervals(workDate, workTime)
	approvedLeave, pendingLeave := attendanceLeaveIntervals(leaves, schedule, breaks)
	projection.ApprovedLeaveMinutes = intervalMinutes(approvedLeave)
	projection.PendingLeaveMinutes = intervalMinutes(pendingLeave)
	workIntervals := attendanceWorkIntervals(effective, asOf)
	workIntervals = subtractIntervals(workIntervals, append(append([]attendanceTimeInterval{}, breaks...), approvedLeave...))
	projection.WorkedMinutes = intervalMinutes(workIntervals)
	projection.AnomalyReasons = attendanceDayAnomalies(projection, workTime)

	switch {
	case projection.ClockIn == nil:
		projection.DayStatus = attendanceDayStatusNotStarted
	case projection.ClockOut == nil:
		projection.DayStatus = attendanceDayStatusWorking
	case len(projection.AnomalyReasons) == 0:
		projection.DayStatus = attendanceDayStatusComplete
	case projection.PendingLeaveMinutes > 0:
		projection.DayStatus = attendanceDayStatusPendingLeave
	default:
		projection.DayStatus = attendanceDayStatusAbnormal
	}
	return projection
}

// attendanceWorkIntervals pairs punches while letting a later consecutive clock-out replace an earlier candidate.
func attendanceWorkIntervals(records []AttendanceClockRecord, asOf time.Time) []attendanceTimeInterval {
	var open *AttendanceClockRecord
	var closeCandidate *AttendanceClockRecord
	out := make([]attendanceTimeInterval, 0)
	flush := func() {
		if open == nil {
			return
		}
		end := asOf
		if closeCandidate != nil {
			end = closeCandidate.ClockedAt
		}
		if end.After(open.ClockedAt) {
			out = append(out, attendanceTimeInterval{Start: open.ClockedAt, End: end})
		}
	}
	for i := range records {
		record := records[i]
		switch record.Direction {
		case clockDirectionIn:
			if open == nil {
				current := record
				open = &current
				closeCandidate = nil
				continue
			}
			if closeCandidate != nil {
				flush()
				current := record
				open = &current
				closeCandidate = nil
			}
		case clockDirectionOut:
			if open != nil && record.ClockedAt.After(open.ClockedAt) {
				current := record
				closeCandidate = &current
			}
		}
	}
	flush()
	return mergeIntervals(out)
}

// attendanceScheduleIntervals builds the standard work and break intervals for one business day.
func attendanceScheduleIntervals(workDate string, workTime AttendancePolicyWorkTime) ([]attendanceTimeInterval, []attendanceTimeInterval) {
	day, err := time.ParseInLocation(time.DateOnly, workDate, attendanceClockLocation)
	if err != nil {
		return nil, nil
	}
	startMinute := parseHHMMMinutes(workTime.StandardStart)
	endMinute := parseHHMMMinutes(workTime.StandardEnd)
	if startMinute < 0 || endMinute < 0 {
		return nil, nil
	}
	start := day.Add(time.Duration(startMinute) * time.Minute)
	end := day.Add(time.Duration(endMinute) * time.Minute)
	if !end.After(start) {
		end = end.Add(24 * time.Hour)
	}
	schedule := []attendanceTimeInterval{{Start: start, End: end}}
	breakStartMinute := parseHHMMMinutes(workTime.BreakStart)
	breakEndMinute := parseHHMMMinutes(workTime.BreakEnd)
	if breakStartMinute < 0 || breakEndMinute <= breakStartMinute {
		return schedule, nil
	}
	breakStart := day.Add(time.Duration(breakStartMinute) * time.Minute)
	breakEnd := day.Add(time.Duration(breakEndMinute) * time.Minute)
	if !breakEnd.After(breakStart) {
		breakEnd = breakEnd.Add(24 * time.Hour)
	}
	return schedule, intersectIntervals([]attendanceTimeInterval{{Start: breakStart, End: breakEnd}}, schedule)
}

// attendanceLeaveIntervals clips approved and pending leave to the work schedule and removes breaks.
func attendanceLeaveIntervals(leaves []LeaveRequest, schedule, breaks []attendanceTimeInterval) ([]attendanceTimeInterval, []attendanceTimeInterval) {
	approved := make([]attendanceTimeInterval, 0)
	pending := make([]attendanceTimeInterval, 0)
	for _, leave := range leaves {
		if !leave.EndAt.After(leave.StartAt) {
			continue
		}
		clipped := intersectIntervals([]attendanceTimeInterval{{Start: leave.StartAt, End: leave.EndAt}}, schedule)
		clipped = subtractIntervals(clipped, breaks)
		switch normalizeLeaveRequestStatus(leave.Status) {
		case "approved":
			approved = append(approved, clipped...)
		case "pending_approval":
			pending = append(pending, clipped...)
		}
	}
	return mergeIntervals(approved), mergeIntervals(pending)
}

// attendanceDayAnomalies calculates soft anomalies from the final daily projection.
func attendanceDayAnomalies(projection attendanceDayProjection, workTime AttendancePolicyWorkTime) []string {
	reasons := make([]string, 0, 2)
	credited := projection.WorkedMinutes + projection.ApprovedLeaveMinutes
	if projection.ClockOut != nil && credited < projection.RequiredMinutes {
		reasons = append(reasons, clockRejectionInsufficientWorkHours)
	}
	if projection.ClockIn != nil && workTime.ClockMode == clockModeFixed && credited < projection.RequiredMinutes {
		start := parseHHMMMinutes(workTime.StandardStart)
		if start >= 0 && clockMinuteOfDay(projection.ClockIn.ClockedAt) > start {
			reasons = appendUniqueString(reasons, clockRejectionOutsideWindow)
		}
	}
	if projection.ClockOut != nil && workTime.ClockMode == clockModeFixed {
		end := parseHHMMMinutes(workTime.StandardEnd)
		if end >= 0 && clockMinuteOfDay(projection.ClockOut.ClockedAt) < end && credited < projection.RequiredMinutes {
			reasons = appendUniqueString(reasons, clockRejectionOutsideWindow)
		}
	}
	if workTime.ClockMode == clockModeFlexible {
		earliest := parseHHMMMinutes(workTime.FlexibleClockInEarliest)
		latest := parseHHMMMinutes(workTime.FlexibleClockOutLatest)
		if projection.ClockIn != nil && earliest >= 0 && clockMinuteOfDay(projection.ClockIn.ClockedAt) < earliest {
			reasons = appendUniqueString(reasons, clockRejectionOutsideWindow)
		}
		if projection.ClockOut != nil && latest >= 0 && clockMinuteOfDay(projection.ClockOut.ClockedAt) > latest {
			reasons = appendUniqueString(reasons, clockRejectionOutsideWindow)
		}
	}
	return reasons
}

// appendUniqueString appends a non-empty value once while preserving order.
func appendUniqueString(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

// mergeIntervals returns a sorted union of overlapping time intervals.
func mergeIntervals(values []attendanceTimeInterval) []attendanceTimeInterval {
	filtered := make([]attendanceTimeInterval, 0, len(values))
	for _, value := range values {
		if value.End.After(value.Start) {
			filtered = append(filtered, value)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Start.Before(filtered[j].Start) })
	out := make([]attendanceTimeInterval, 0, len(filtered))
	for _, value := range filtered {
		if len(out) == 0 || value.Start.After(out[len(out)-1].End) {
			out = append(out, value)
			continue
		}
		if value.End.After(out[len(out)-1].End) {
			out[len(out)-1].End = value.End
		}
	}
	return out
}

// intersectIntervals returns the overlap between two interval collections.
func intersectIntervals(left, right []attendanceTimeInterval) []attendanceTimeInterval {
	out := make([]attendanceTimeInterval, 0)
	for _, a := range left {
		for _, b := range right {
			start := a.Start
			if b.Start.After(start) {
				start = b.Start
			}
			end := a.End
			if b.End.Before(end) {
				end = b.End
			}
			if end.After(start) {
				out = append(out, attendanceTimeInterval{Start: start, End: end})
			}
		}
	}
	return mergeIntervals(out)
}

// subtractIntervals removes every exclusion interval from the supplied intervals.
func subtractIntervals(values, exclusions []attendanceTimeInterval) []attendanceTimeInterval {
	remaining := mergeIntervals(values)
	for _, exclusion := range mergeIntervals(exclusions) {
		next := make([]attendanceTimeInterval, 0, len(remaining)+1)
		for _, value := range remaining {
			if !exclusion.End.After(value.Start) || !value.End.After(exclusion.Start) {
				next = append(next, value)
				continue
			}
			if exclusion.Start.After(value.Start) {
				next = append(next, attendanceTimeInterval{Start: value.Start, End: exclusion.Start})
			}
			if value.End.After(exclusion.End) {
				next = append(next, attendanceTimeInterval{Start: exclusion.End, End: value.End})
			}
		}
		remaining = next
	}
	return mergeIntervals(remaining)
}

// intervalMinutes returns rounded whole minutes for a union of intervals.
func intervalMinutes(values []attendanceTimeInterval) int {
	minutes := 0.0
	for _, value := range mergeIntervals(values) {
		minutes += value.End.Sub(value.Start).Minutes()
	}
	return int(minutes + 0.5)
}
