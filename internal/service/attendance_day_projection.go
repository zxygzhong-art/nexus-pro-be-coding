package service

import (
	"crypto/sha256"
	"fmt"
	"nexus-pro-api/internal/domain"
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

	attendanceLeaveFactApproved  = "approved"
	attendanceLeaveFactPending   = "pending"
	attendanceNextActionComplete = "complete"
)

// attendanceTimeInterval represents a half-open attendance interval.
type attendanceTimeInterval struct {
	Start time.Time
	End   time.Time
}

// attendanceEffectiveLeave is the source-independent interval consumed by all
// attendance projections. Approved facts originate only from confirmed active
// leave cases; pending facts originate only from requests awaiting approval.
type attendanceEffectiveLeave struct {
	EmployeeID   string
	LeaveTypeID  string
	LeaveType    string
	StartAt      time.Time
	EndAt        time.Time
	NetMinutes   int
	FactStatus   string
	SourceFactID string
}

// loadEffectiveAttendanceLeaves is the only service loader allowed to combine
// approved and pending leave. Approved requests are intentionally never read:
// reconciliation must first materialize an active, confirmed leave case.
func (c AttendanceService) loadEffectiveAttendanceLeaves(ctx RequestContext, employeeIDs []string, fromDate, toDate string) ([]attendanceEffectiveLeave, error) {
	employeeIDs = employeeIDsFromSlice(employeeIDs)
	if len(employeeIDs) == 0 {
		return nil, nil
	}
	from, toExclusive, err := attendanceProjectionDateRange(fromDate, toDate)
	if err != nil {
		return nil, err
	}
	// Extend one calendar day on both sides so overnight schedules can consume a
	// leave interval that starts after midnight but belongs to the prior work date.
	queryFrom := from.AddDate(0, 0, -1)
	queryToExclusive := toExclusive.AddDate(0, 0, 1)
	records, err := c.store.ListActiveLeaveRecordsByQuery(goContext(ctx), ctx.TenantID, employeeIDs, queryFrom, queryToExclusive)
	if err != nil {
		return nil, err
	}
	pending, err := c.store.ListLeaveRequestsByQuery(goContext(ctx), ctx.TenantID, LeaveRequestQuery{
		EmployeeIDs: employeeIDs,
		Status:      "pending_approval",
		FromDate:    queryFrom.Format(time.DateOnly),
		ToDate:      queryToExclusive.AddDate(0, 0, -1).Format(time.DateOnly),
	})
	if err != nil {
		return nil, err
	}
	return attendanceEffectiveLeaves(records, pending), nil
}

// attendanceProjectionDateRange converts inclusive work-date filters to the
// half-open timestamp range used by leave case overlap queries.
func attendanceProjectionDateRange(fromDate, toDate string) (time.Time, time.Time, error) {
	from, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(fromDate), attendanceClockLocation)
	if err != nil {
		return time.Time{}, time.Time{}, BadRequest("from_date must be YYYY-MM-DD")
	}
	to, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(toDate), attendanceClockLocation)
	if err != nil {
		return time.Time{}, time.Time{}, BadRequest("to_date must be YYYY-MM-DD")
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, BadRequest("to_date must not be before from_date")
	}
	return from, to.AddDate(0, 0, 1), nil
}

// attendanceClockStatusFromProjection exposes the stable daily projection to clients.
func attendanceClockStatusFromProjection(employeeID, workDate string, projection domain.AttendanceDayProjection) AttendanceClockStatus {
	nextAction := attendanceNextActionComplete
	if projection.CanClockIn {
		nextAction = clockDirectionIn
	} else if projection.CanClockOut {
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

// loadAttendanceDayProjection loads every raw punch and overlapping leave before deriving the day state.
func (c AttendanceService) loadAttendanceDayProjection(ctx RequestContext, employeeID, workDate string, asOf time.Time) (domain.AttendanceDayProjection, error) {
	records, err := c.store.ListAttendanceClockRecords(goContext(ctx), ctx.TenantID, AttendanceClockRecordQuery{
		EmployeeID: employeeID,
		FromDate:   workDate,
		ToDate:     workDate,
	})
	if err != nil {
		return domain.AttendanceDayProjection{}, err
	}
	leaves, err := c.loadEffectiveAttendanceLeaves(ctx, []string{employeeID}, workDate, workDate)
	if err != nil {
		return domain.AttendanceDayProjection{}, err
	}
	policy, err := c.loadAttendancePolicyResponseForWorkDate(ctx, workDate)
	if err != nil {
		return domain.AttendanceDayProjection{}, err
	}
	return c.projectAndPersistAttendanceDay(ctx, employeeID, records, leaves, workDate, policy, asOf)
}

// projectAndPersistAttendanceDay implements a fingerprint-validated read-through
// projection. The persisted row is a rebuildable cache; raw clocks, canonical
// leave facts, and the policy version remain authoritative.
func (c AttendanceService) projectAndPersistAttendanceDay(ctx RequestContext, employeeID string, records []AttendanceClockRecord, leaves []attendanceEffectiveLeave, workDate string, policy AttendancePolicyResponse, asOf time.Time) (domain.AttendanceDayProjection, error) {
	leaves = attendanceEffectiveLeavesForProjection(leaves, employeeID, workDate, policy.WorkTime)
	fingerprint := attendanceProjectionFingerprint(records, leaves, workDate, policy, asOf)
	stored, ok, err := c.store.GetAttendanceDayProjection(goContext(ctx), ctx.TenantID, employeeID, workDate)
	if err != nil {
		return domain.AttendanceDayProjection{}, err
	}
	if ok && stored.InputFingerprint == fingerprint {
		materializeAttendanceProjectionRecords(&stored, records)
		return stored, nil
	}
	projection, err := c.projectAttendanceDayWithCanonicalLeave(ctx, records, leaves, workDate, policy.WorkTime, asOf)
	if err != nil {
		return domain.AttendanceDayProjection{}, err
	}
	projection.TenantID = ctx.TenantID
	projection.EmployeeID = employeeID
	projection.WorkDate = workDate
	projection.PolicyVersion = policy.Version
	projection.InputFingerprint = fingerprint
	projection.ComputedAt = asOf
	projection.UpdatedAt = asOf
	if err := c.store.UpsertAttendanceDayProjection(goContext(ctx), projection); err != nil {
		return domain.AttendanceDayProjection{}, err
	}
	return projection, nil
}

func attendanceEffectiveLeavesForProjection(leaves []attendanceEffectiveLeave, employeeID, workDate string, workTime AttendancePolicyWorkTime) []attendanceEffectiveLeave {
	schedule, _ := attendanceScheduleIntervals(workDate, workTime)
	if len(schedule) == 0 {
		return nil
	}
	start, end := schedule[0].Start, schedule[len(schedule)-1].End
	out := make([]attendanceEffectiveLeave, 0, len(leaves))
	for _, leave := range leaves {
		if leave.EmployeeID == employeeID && leave.StartAt.Before(end) && leave.EndAt.After(start) {
			out = append(out, leave)
		}
	}
	return out
}

func materializeAttendanceProjectionRecords(projection *domain.AttendanceDayProjection, records []AttendanceClockRecord) {
	byID := make(map[string]AttendanceClockRecord, len(records))
	for _, record := range records {
		byID[record.ID] = record
	}
	if record, ok := byID[projection.ClockInRecordID]; ok {
		current := record
		projection.ClockIn = &current
	}
	if record, ok := byID[projection.ClockOutRecordID]; ok {
		current := record
		projection.ClockOut = &current
	}
	if record, ok := byID[projection.LastPunchRecordID]; ok {
		current := record
		projection.LastPunch = &current
	}
	projection.CanClockIn = projection.PunchCount == 0
	projection.CanClockOut = projection.ClockInRecordID != "" && projection.LastPunch != nil && projection.LastPunch.Direction == clockDirectionIn
}

func attendanceProjectionFingerprint(records []AttendanceClockRecord, leaves []attendanceEffectiveLeave, workDate string, policy AttendancePolicyResponse, asOf time.Time) string {
	records = append([]AttendanceClockRecord(nil), records...)
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].ClockedAt.Equal(records[j].ClockedAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].ClockedAt.Before(records[j].ClockedAt)
	})
	leaves = append([]attendanceEffectiveLeave(nil), leaves...)
	sort.SliceStable(leaves, func(i, j int) bool {
		if leaves[i].StartAt.Equal(leaves[j].StartAt) {
			return leaves[i].SourceFactID < leaves[j].SourceFactID
		}
		return leaves[i].StartAt.Before(leaves[j].StartAt)
	})
	var source strings.Builder
	fmt.Fprintf(&source, "date=%s|policy=%d|work_time=%#v", workDate, policy.Version, policy.WorkTime)
	for _, record := range records {
		fmt.Fprintf(&source, "|clock=%s,%s,%s,%t,%s,%s", record.ID, record.Direction, record.RecordStatus, record.Voided, record.ClockedAt.UTC().Format(time.RFC3339Nano), record.CreatedAt.UTC().Format(time.RFC3339Nano))
	}
	for _, leave := range leaves {
		fmt.Fprintf(&source, "|leave=%s,%s,%s,%s,%s,%d", leave.SourceFactID, leave.FactStatus, leave.LeaveTypeID, leave.StartAt.UTC().Format(time.RFC3339Nano), leave.EndAt.UTC().Format(time.RFC3339Nano), leave.NetMinutes)
	}
	if attendanceProjectionHasOpenClock(records, workDate) {
		fmt.Fprintf(&source, "|open_as_of=%s", asOf.UTC().Truncate(time.Minute).Format(time.RFC3339))
	}
	sum := sha256.Sum256([]byte(source.String()))
	return fmt.Sprintf("%x", sum[:])
}

func attendanceProjectionHasOpenClock(records []AttendanceClockRecord, workDate string) bool {
	effective := make([]AttendanceClockRecord, 0, len(records))
	for _, record := range records {
		if record.WorkDate == workDate && strings.EqualFold(record.RecordStatus, clockRecordStatusAccepted) && !record.Voided {
			effective = append(effective, record)
		}
	}
	if len(effective) == 0 {
		return false
	}
	sort.SliceStable(effective, func(i, j int) bool {
		if effective[i].ClockedAt.Equal(effective[j].ClockedAt) {
			return effective[i].ID < effective[j].ID
		}
		return effective[i].ClockedAt.Before(effective[j].ClockedAt)
	})
	return effective[len(effective)-1].Direction == clockDirectionIn
}

// projectAttendanceDays refreshes every employee-day that currently has a raw
// source or an existing persisted row, then returns the read-model view used by
// workspace reporting. Existing rows are candidates too so void/cancellation
// can actively replace stale non-zero projections.
func (c AttendanceService) projectAttendanceDays(ctx RequestContext, employeeIDs []string, records []AttendanceClockRecord, leaves []attendanceEffectiveLeave, policies map[string]AttendancePolicyResponse, fromDate, toDate string, asOf time.Time) (map[string]map[string]domain.AttendanceDayProjection, error) {
	employeeIDs = employeeIDsFromSlice(employeeIDs)
	out := map[string]map[string]domain.AttendanceDayProjection{}
	if len(employeeIDs) == 0 {
		return out, nil
	}
	from, toExclusive, err := attendanceProjectionDateRange(fromDate, toDate)
	if err != nil {
		return nil, err
	}
	existing, err := c.store.ListAttendanceDayProjections(goContext(ctx), ctx.TenantID, employeeIDs, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	candidates := map[string]map[string]struct{}{}
	recordsByEmployeeDate := map[string]map[string][]AttendanceClockRecord{}
	addCandidate := func(employeeID, workDate string) {
		if employeeID == "" || workDate == "" {
			return
		}
		if candidates[employeeID] == nil {
			candidates[employeeID] = map[string]struct{}{}
		}
		candidates[employeeID][workDate] = struct{}{}
	}
	for _, stored := range existing {
		addCandidate(stored.EmployeeID, stored.WorkDate)
	}
	for _, record := range records {
		if recordsByEmployeeDate[record.EmployeeID] == nil {
			recordsByEmployeeDate[record.EmployeeID] = map[string][]AttendanceClockRecord{}
		}
		recordsByEmployeeDate[record.EmployeeID][record.WorkDate] = append(recordsByEmployeeDate[record.EmployeeID][record.WorkDate], record)
		addCandidate(record.EmployeeID, record.WorkDate)
	}
	for day := from; day.Before(toExclusive); day = day.AddDate(0, 0, 1) {
		if !attendanceEffectiveLeaveOverlapsDay(leaves, day) {
			continue
		}
		workDate := day.Format(time.DateOnly)
		dayEnd := day.AddDate(0, 0, 2)
		for _, leave := range leaves {
			if leave.StartAt.Before(dayEnd) && leave.EndAt.After(day) {
				addCandidate(leave.EmployeeID, workDate)
			}
		}
	}
	employees := make([]string, 0, len(candidates))
	for employeeID := range candidates {
		employees = append(employees, employeeID)
	}
	sort.Strings(employees)
	for _, employeeID := range employees {
		workDates := make([]string, 0, len(candidates[employeeID]))
		for workDate := range candidates[employeeID] {
			if workDate >= fromDate && workDate <= toDate {
				workDates = append(workDates, workDate)
			}
		}
		sort.Strings(workDates)
		for _, workDate := range workDates {
			policy, ok := policies[workDate]
			if !ok {
				return nil, fmt.Errorf("attendance policy missing for work date %s", workDate)
			}
			projection, err := c.projectAndPersistAttendanceDay(ctx, employeeID, recordsByEmployeeDate[employeeID][workDate], leaves, workDate, policy, asOf)
			if err != nil {
				return nil, err
			}
			if out[employeeID] == nil {
				out[employeeID] = map[string]domain.AttendanceDayProjection{}
			}
			out[employeeID][workDate] = projection
		}
	}
	return out, nil
}

// ProjectAttendanceDay is the compatibility entrypoint used by the legacy
// platform projection. New attendance paths must use
// ProjectAttendanceDayWithEffectiveLeave so approved requests cannot bypass
// the canonical leave-case reconciliation boundary.
func ProjectAttendanceDay(records []AttendanceClockRecord, leaves []LeaveRequest, workDate string, workTime AttendancePolicyWorkTime, asOf time.Time) domain.AttendanceDayProjection {
	return projectAttendanceDayWithEffectiveLeaves(records, attendanceEffectiveLeavesFromLegacyRequests(leaves), workDate, workTime, asOf, attendanceOpenClockDeadlineForPolicy(workDate, workTime))
}

// ProjectAttendanceDayWithEffectiveLeave is the canonical projection entrypoint:
// confirmed active cases supply approved time and pending requests supply only
// non-credited pending time.
func ProjectAttendanceDayWithEffectiveLeave(records []AttendanceClockRecord, approvedRecords []LeaveRecord, pendingRequests []LeaveRequest, workDate string, workTime AttendancePolicyWorkTime, asOf time.Time) domain.AttendanceDayProjection {
	leaves := attendanceEffectiveLeaves(approvedRecords, pendingRequests)
	return projectAttendanceDayWithEffectiveLeaves(records, leaves, workDate, workTime, asOf, attendanceOpenClockDeadlineForPolicy(workDate, workTime))
}

// projectAttendanceDayWithEffectiveLeaves allows service paths to provide a
// policy-based open-clock deadline without weakening the leave source contract.
func projectAttendanceDayWithEffectiveLeaves(records []AttendanceClockRecord, leaves []attendanceEffectiveLeave, workDate string, workTime AttendancePolicyWorkTime, asOf, openClockDeadline time.Time) domain.AttendanceDayProjection {
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

	projection := domain.AttendanceDayProjection{
		WorkDate:        workDate,
		PunchCount:      len(effective),
		RequiredMinutes: int(standardDayHours(workTime)*60 + 0.5),
		DayStatus:       attendanceDayStatusNotStarted,
	}
	if len(effective) > 0 {
		last := effective[len(effective)-1]
		projection.LastPunch = &last
		projection.LastPunchRecordID = last.ID
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
	projection.CanClockIn = len(effective) == 0
	projection.CanClockOut = projection.ClockIn != nil && projection.LastPunch != nil && projection.LastPunch.Direction == clockDirectionIn
	if projection.ClockIn != nil {
		projection.ClockInRecordID = projection.ClockIn.ID
	}
	if projection.ClockOut != nil {
		projection.ClockOutRecordID = projection.ClockOut.ID
	}

	schedule, breaks := attendanceScheduleIntervals(workDate, workTime)
	if len(schedule) > 0 {
		start, end := schedule[0].Start, schedule[len(schedule)-1].End
		projection.ScheduledStartAt = &start
		projection.ScheduledEndAt = &end
	}
	approvedLeave, pendingLeave := attendanceEffectiveLeaveIntervals(leaves, schedule, breaks)
	projection.ApprovedLeaveMinutes = intervalMinutes(approvedLeave)
	projection.PendingLeaveMinutes = intervalMinutes(pendingLeave)
	workIntervals := attendanceWorkIntervals(effective, asOf)
	workIntervals = subtractIntervals(workIntervals, append(append([]attendanceTimeInterval{}, breaks...), approvedLeave...))
	projection.WorkedMinutes = intervalMinutes(workIntervals)
	projection.AnomalyReasons = attendanceDayAnomalies(projection, workTime)

	switch {
	case projection.ClockIn == nil && projection.PendingLeaveMinutes > 0:
		projection.DayStatus = attendanceDayStatusPendingLeave
	case projection.ClockIn == nil && projection.RequiredMinutes > 0 && projection.ApprovedLeaveMinutes >= projection.RequiredMinutes:
		projection.DayStatus = attendanceDayStatusComplete
	case projection.ClockIn == nil && projection.ApprovedLeaveMinutes > 0:
		projection.AnomalyReasons = appendUniqueString(projection.AnomalyReasons, clockRejectionInsufficientWorkHours)
		projection.DayStatus = attendanceDayStatusAbnormal
	case projection.ClockIn == nil:
		projection.DayStatus = attendanceDayStatusNotStarted
	case projection.ClockOut == nil && attendanceOpenClockHasExpired(openClockDeadline, asOf):
		projection.AnomalyReasons = appendUniqueString(projection.AnomalyReasons, attendanceAnomalyMissingClockOut)
		projection.DayStatus = attendanceDayStatusAbnormal
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

// projectAttendanceDay derives day state using the policy open-clock deadline.
func (c AttendanceService) projectAttendanceDay(ctx RequestContext, records []AttendanceClockRecord, leaves []LeaveRequest, workDate string, workTime AttendancePolicyWorkTime, asOf time.Time) (domain.AttendanceDayProjection, error) {
	deadline := attendanceOpenClockDeadlineForPolicy(workDate, workTime)
	return projectAttendanceDayWithEffectiveLeaves(records, attendanceEffectiveLeavesFromLegacyRequests(leaves), workDate, workTime, asOf, deadline), nil
}

// projectAttendanceDayWithCanonicalLeave is the service-level canonical path.
func (c AttendanceService) projectAttendanceDayWithCanonicalLeave(ctx RequestContext, records []AttendanceClockRecord, leaves []attendanceEffectiveLeave, workDate string, workTime AttendancePolicyWorkTime, asOf time.Time) (domain.AttendanceDayProjection, error) {
	deadline := attendanceOpenClockDeadlineForPolicy(workDate, workTime)
	return projectAttendanceDayWithEffectiveLeaves(records, leaves, workDate, workTime, asOf, deadline), nil
}

// attendanceOpenClockDeadlineForPolicy keeps an open punch active through its business day and overnight schedule.
func attendanceOpenClockDeadlineForPolicy(workDate string, workTime AttendancePolicyWorkTime) time.Time {
	day, err := time.ParseInLocation(time.DateOnly, workDate, attendanceClockLocation)
	if err != nil {
		return time.Time{}
	}
	cutoff := day.AddDate(0, 0, 1)
	schedule, _ := attendanceScheduleIntervals(workDate, workTime)
	if len(schedule) > 0 && schedule[len(schedule)-1].End.After(cutoff) {
		cutoff = schedule[len(schedule)-1].End.Add(time.Minute)
	}
	return cutoff
}

// attendanceOpenClockHasExpired compares the current business time with the exclusive open-clock deadline.
func attendanceOpenClockHasExpired(deadline, asOf time.Time) bool {
	return !deadline.IsZero() && !asOf.In(attendanceClockLocation).Before(deadline)
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

// attendanceLeaveIntervals remains a compatibility adapter for callers that
// have not crossed the canonical case boundary yet.
func attendanceLeaveIntervals(leaves []LeaveRequest, schedule, breaks []attendanceTimeInterval) ([]attendanceTimeInterval, []attendanceTimeInterval) {
	return attendanceEffectiveLeaveIntervals(attendanceEffectiveLeavesFromLegacyRequests(leaves), schedule, breaks)
}

// attendanceEffectiveLeaveIntervals clips canonical facts to the work schedule,
// removes breaks, and unions overlaps so two sources can never double-credit a
// minute. NetMinutes caps exact single-day facts when the source interval is
// broader than its authoritative credited duration.
func attendanceEffectiveLeaveIntervals(leaves []attendanceEffectiveLeave, schedule, breaks []attendanceTimeInterval) ([]attendanceTimeInterval, []attendanceTimeInterval) {
	approved := make([]attendanceTimeInterval, 0)
	pending := make([]attendanceTimeInterval, 0)
	for _, leave := range leaves {
		if !leave.EndAt.After(leave.StartAt) {
			continue
		}
		clipped := intersectIntervals([]attendanceTimeInterval{{Start: leave.StartAt, End: leave.EndAt}}, schedule)
		clipped = subtractIntervals(clipped, breaks)
		if leave.NetMinutes > 0 && leave.NetMinutes < intervalMinutes(clipped) {
			clipped = capIntervals(clipped, leave.NetMinutes)
		}
		switch leave.FactStatus {
		case attendanceLeaveFactApproved:
			approved = append(approved, clipped...)
		case attendanceLeaveFactPending:
			pending = append(pending, clipped...)
		}
	}
	return mergeIntervals(approved), mergeIntervals(pending)
}

// attendanceEffectiveLeaves enforces the canonical source contract at the
// projection boundary. Confirmation is enforced by the repository query; the
// status checks here protect pure callers and tests from stale rows.
func attendanceEffectiveLeaves(approvedRecords []LeaveRecord, pendingRequests []LeaveRequest) []attendanceEffectiveLeave {
	out := make([]attendanceEffectiveLeave, 0, len(approvedRecords)+len(pendingRequests))
	for _, leaveRecord := range approvedRecords {
		if !strings.EqualFold(strings.TrimSpace(leaveRecord.Status), "active") || !leaveRecord.EndAt.After(leaveRecord.StartAt) {
			continue
		}
		out = append(out, attendanceEffectiveLeave{
			EmployeeID:   leaveRecord.EmployeeID,
			LeaveTypeID:  leaveRecord.LeaveTypeID,
			StartAt:      leaveRecord.StartAt,
			EndAt:        leaveRecord.EndAt,
			NetMinutes:   leaveRecord.NetMinutes,
			FactStatus:   attendanceLeaveFactApproved,
			SourceFactID: leaveRecord.ID,
		})
	}
	for _, request := range pendingRequests {
		if normalizeLeaveRequestStatus(request.Status) != "pending_approval" || !request.EndAt.After(request.StartAt) {
			continue
		}
		out = append(out, attendanceEffectiveLeave{
			EmployeeID:   request.EmployeeID,
			LeaveTypeID:  request.LeaveTypeID,
			LeaveType:    request.LeaveType,
			StartAt:      request.StartAt,
			EndAt:        request.EndAt,
			NetMinutes:   request.RequestedMinutes,
			FactStatus:   attendanceLeaveFactPending,
			SourceFactID: request.ID,
		})
	}
	return out
}

func attendanceEffectiveLeavesFromLegacyRequests(leaves []LeaveRequest) []attendanceEffectiveLeave {
	out := make([]attendanceEffectiveLeave, 0, len(leaves))
	for _, leave := range leaves {
		status := normalizeLeaveRequestStatus(leave.Status)
		factStatus := ""
		switch status {
		case "approved":
			factStatus = attendanceLeaveFactApproved
		case "pending_approval":
			factStatus = attendanceLeaveFactPending
		default:
			continue
		}
		out = append(out, attendanceEffectiveLeave{
			EmployeeID:   leave.EmployeeID,
			LeaveTypeID:  leave.LeaveTypeID,
			LeaveType:    leave.LeaveType,
			StartAt:      leave.StartAt,
			EndAt:        leave.EndAt,
			NetMinutes:   leave.RequestedMinutes,
			FactStatus:   factStatus,
			SourceFactID: leave.ID,
		})
	}
	return out
}

// capIntervals keeps the earliest credited minutes while preserving half-open
// interval semantics. It is used only when a source provides an explicit net
// duration smaller than its timestamp envelope.
func capIntervals(values []attendanceTimeInterval, minutes int) []attendanceTimeInterval {
	if minutes <= 0 {
		return nil
	}
	remaining := minutes
	out := make([]attendanceTimeInterval, 0, len(values))
	for _, value := range mergeIntervals(values) {
		if remaining <= 0 {
			break
		}
		valueMinutes := int(value.End.Sub(value.Start) / time.Minute)
		if valueMinutes <= remaining {
			out = append(out, value)
			remaining -= valueMinutes
			continue
		}
		out = append(out, attendanceTimeInterval{Start: value.Start, End: value.Start.Add(time.Duration(remaining) * time.Minute)})
		remaining = 0
	}
	return out
}

// attendanceDayAnomalies calculates soft anomalies from the final daily projection.
func attendanceDayAnomalies(projection domain.AttendanceDayProjection, workTime AttendancePolicyWorkTime) []string {
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
