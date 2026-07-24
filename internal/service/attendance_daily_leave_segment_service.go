package service

import (
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
)

type ehrmsAttendanceLeaveFields struct {
	leaveType string
	start     string
	end       string
	hours     string
	counted   string
}

var ehrmsAttendanceLeaveFieldSets = []ehrmsAttendanceLeaveFields{
	{
		leaveType: ehrmsAttendanceFieldLeaveType,
		start:     ehrmsAttendanceFieldLeaveStart,
		end:       ehrmsAttendanceFieldLeaveEnd,
		hours:     ehrmsAttendanceFieldLeaveHours,
		counted:   ehrmsAttendanceFieldLeaveCounted,
	},
	{
		leaveType: ehrmsAttendanceFieldLeave2Type,
		start:     ehrmsAttendanceFieldLeave2Start,
		end:       ehrmsAttendanceFieldLeave2End,
		hours:     ehrmsAttendanceFieldLeave2Hours,
		counted:   ehrmsAttendanceFieldLeave2Counted,
	},
}

// syncEHRMSAttendanceLeaveSegments runs after leave-details have been persisted
// and reconciled. The daily slice points to the eHRMS leave fact; that fact's
// matched_record_id remains the single route to the Nexus leave request.
func (c AttendanceService) syncEHRMSAttendanceLeaveSegments(ctx RequestContext, record domain.EHRMSAttendanceRecord, rowNumber int) error {
	employeeNo := ehrmsAttendanceValue(record, ehrmsAttendanceFieldEmployeeNo)
	workDate := normalizeEHRMSAttendanceDate(ehrmsAttendanceValue(record, ehrmsAttendanceFieldDate))
	if employeeNo == "" || workDate == "" {
		return nil
	}
	employee, ok, err := c.store.GetEmployeeByEmployeeNo(goContext(ctx), ctx.TenantID, employeeNo)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	summary, ok, err := c.store.GetAttendanceDailySummaryByEmployeeDate(goContext(ctx), ctx.TenantID, employee.ID, workDate)
	if err != nil {
		return err
	}
	if !ok || summary.Source != ehrmsAttendanceSource {
		return nil
	}
	if err := c.store.DeleteAttendanceDailyLeaveSegments(goContext(ctx), ctx.TenantID, employee.ID, workDate); err != nil {
		return err
	}

	for index, fields := range ehrmsAttendanceLeaveFieldSets {
		segment, present, buildErr := c.buildEHRMSAttendanceLeaveSegment(ctx, summary, record, fields, index+1, rowNumber)
		if buildErr != nil {
			return buildErr
		}
		if !present {
			continue
		}
		if err := c.store.UpsertAttendanceDailyLeaveSegment(goContext(ctx), segment); err != nil {
			return err
		}
	}
	return nil
}

func (c AttendanceService) buildEHRMSAttendanceLeaveSegment(
	ctx RequestContext,
	summary AttendanceDailySummary,
	record domain.EHRMSAttendanceRecord,
	fields ehrmsAttendanceLeaveFields,
	segmentNo int,
	rowNumber int,
) (domain.AttendanceDailyLeaveSegment, bool, error) {
	rawType := ehrmsAttendanceValue(record, fields.leaveType)
	rawStart := ehrmsAttendanceValue(record, fields.start)
	rawEnd := ehrmsAttendanceValue(record, fields.end)
	rawHours := ehrmsAttendanceValue(record, fields.hours)
	rawCounted := ehrmsAttendanceValue(record, fields.counted)
	if rawType == "" && rawStart == "" && rawEnd == "" && rawHours == "" && rawCounted == "" {
		return domain.AttendanceDailyLeaveSegment{}, false, nil
	}

	now := c.Now()
	segment := domain.AttendanceDailyLeaveSegment{
		TenantID: summary.TenantID, EmployeeID: summary.EmployeeID, WorkDate: summary.WorkDate,
		DailySource: attendanceDailySourceEHRMS,
		SegmentNo:   segmentNo, SourceLeaveType: rawType, Counted: parseEHRMSCounted(rawCounted),
		LinkStatus: "mismatch", MatchBasis: "invalid_segment",
		Payload: map[string]any{
			"row_number": rowNumber, "leave_type": rawType, "start": rawStart,
			"end": rawEnd, "hours": rawHours, "counted": rawCounted,
		},
		CreatedAt: now, UpdatedAt: now,
	}
	hours, hoursOK := parseEHRMSAttendanceHours(rawHours)
	if hoursOK {
		segment.Minutes = leaveMinutes(hours)
	}

	windowStart, windowEnd, windowOK := attendanceSummaryScheduleWindow(summary)
	rawStartAt, startOK := parseEHRMSLeaveDetailDateTime(summary.WorkDate, rawStart)
	rawEndAt, endOK := parseEHRMSLeaveDetailDateTime(summary.WorkDate, rawEnd)
	if windowOK && startOK && endOK && rawEndAt.After(rawStartAt) {
		startAt := latestTime(rawStartAt, windowStart)
		endAt := earliestTime(rawEndAt, windowEnd)
		if endAt.After(startAt) {
			segment.StartAt = &startAt
			segment.EndAt = &endAt
			segment.TimeInferred = !rawStartAt.Equal(startAt) || !rawEndAt.Equal(endAt) ||
				!ehrmsValueContainsDate(rawStart) || !ehrmsValueContainsDate(rawEnd)
		}
	}

	asOf, _ := time.ParseInLocation(time.DateOnly, summary.WorkDate, attendanceClockLocation)
	_, leaveTypeID, typeFound, err := c.resolveEHRMSLeaveType(ctx, "", "", rawType, asOf)
	if err != nil {
		return domain.AttendanceDailyLeaveSegment{}, false, err
	}
	if !typeFound {
		segment.MatchBasis = "unknown_leave_type"
		return segment, true, nil
	}
	segment.LeaveTypeID = leaveTypeID
	if segment.Minutes <= 0 || segment.StartAt == nil || segment.EndAt == nil || !windowOK {
		return segment, true, nil
	}

	candidates, err := c.store.ListEHRMSLeaveRecordCandidates(
		goContext(ctx), ctx.TenantID, summary.EmployeeID, leaveTypeID, windowStart, windowEnd,
	)
	if err != nil {
		return domain.AttendanceDailyLeaveSegment{}, false, err
	}
	segment.CandidateRecordIDs = leaveRecordIDs(candidates)
	if len(candidates) == 0 {
		segment.LinkStatus = "unmatched"
		segment.MatchBasis = "employee+type+interval:no_candidate"
		return segment, true, nil
	}

	exact := exactLeaveIntervalCandidates(candidates, rawStartAt, rawEndAt, startOK && endOK)
	switch {
	case len(exact) == 1:
		segment.LeaveRecordID = exact[0].ID
		segment.LinkStatus = "matched"
		segment.MatchBasis = "employee+type+exact_interval"
	case len(candidates) == 1:
		segment.LeaveRecordID = candidates[0].ID
		segment.LinkStatus = "matched"
		segment.MatchBasis = "employee+type+daily_overlap"
	default:
		segment.LinkStatus = "ambiguous"
		segment.MatchBasis = "employee+type+interval:multiple_candidates"
	}
	return segment, true, nil
}

func attendanceSummaryScheduleWindow(summary AttendanceDailySummary) (time.Time, time.Time, bool) {
	day, err := time.ParseInLocation(time.DateOnly, summary.WorkDate, attendanceClockLocation)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	start, startOK := parseEHRMSLeaveDetailDateTime(summary.WorkDate, summary.ShiftStart)
	end, endOK := parseEHRMSLeaveDetailDateTime(summary.WorkDate, summary.ShiftEnd)
	if !startOK || !endOK {
		return day, day.AddDate(0, 0, 1), true
	}
	if !end.After(start) {
		end = end.AddDate(0, 0, 1)
	}
	return start, end, true
}

func parseEHRMSCounted(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "v", "true", "1", "y", "yes", "是", "有":
		return true
	default:
		return false
	}
}

func ehrmsValueContainsDate(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= len(time.DateOnly) &&
		((value[4] == '-' && value[7] == '-') || (value[4] == '/' && value[7] == '/'))
}

func latestTime(left, right time.Time) time.Time {
	if left.After(right) {
		return left
	}
	return right
}

func earliestTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

func leaveRecordIDs(items []LeaveRecord) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func exactLeaveIntervalCandidates(items []LeaveRecord, startAt, endAt time.Time, valid bool) []LeaveRecord {
	if !valid {
		return nil
	}
	out := make([]LeaveRecord, 0, 1)
	for _, item := range items {
		if item.StartAt.Equal(startAt) && item.EndAt.Equal(endAt) {
			out = append(out, item)
		}
	}
	return out
}
