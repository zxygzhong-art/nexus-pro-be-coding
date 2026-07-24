package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"nexus-pro-api/internal/domain"
)

const (
	attendanceDailySourceLocal = "local"
	attendanceDailySourceEHRMS = "ehrms"
)

func (c AttendanceService) upsertEHRMSAttendanceDailyRecord(ctx RequestContext, summary AttendanceDailySummary, creditedLeaveMinutes int) error {
	record := ehrmsAttendanceDailyRecord(summary, creditedLeaveMinutes)
	return c.upsertAttendanceDailyRecordPreservingCreatedAt(ctx, record)
}

func ehrmsAttendanceDailyRecord(summary AttendanceDailySummary, creditedLeaveMinutes int) domain.AttendanceDailyRecord {
	now := summary.UpdatedAt
	record := domain.AttendanceDailyRecord{
		TenantID:             summary.TenantID,
		EmployeeID:           summary.EmployeeID,
		WorkDate:             summary.WorkDate,
		Source:               attendanceDailySourceEHRMS,
		ScheduledMinutes:     attendanceHoursToMinutes(summary.ShiftHours),
		RequiredMinutes:      attendanceHoursToMinutes(summary.DailyHours),
		WorkedMinutes:        attendanceHoursToMinutes(summary.ClockHours),
		CreditedLeaveMinutes: creditedLeaveMinutes,
		ExternalRef:          summary.ExternalRef,
		Payload:              summary.Payload,
		CreatedAt:            summary.CreatedAt,
		UpdatedAt:            now,
	}
	if summary.ShiftStart != "" && summary.ShiftEnd != "" {
		if start, end, ok := attendanceSummaryScheduleWindow(summary); ok {
			record.ScheduledStartAt = &start
			record.ScheduledEndAt = &end
		}
	}
	record.ClockInAt = attendanceDailyTime(summary.WorkDate, summary.ClockStart)
	record.ClockOutAt = attendanceDailyTime(summary.WorkDate, summary.ClockEnd)
	if record.ClockInAt != nil && record.ClockOutAt != nil && record.ClockOutAt.Before(*record.ClockInAt) {
		next := record.ClockOutAt.AddDate(0, 0, 1)
		record.ClockOutAt = &next
	}
	record.DayStatus = ehrmsAttendanceDayStatus(record)
	record.InputFingerprint = attendanceDailyRecordFingerprint(record)
	return record
}

func localAttendanceDailyRecord(projection domain.AttendanceDayProjection) domain.AttendanceDailyRecord {
	record := domain.AttendanceDailyRecord{
		TenantID:             projection.TenantID,
		EmployeeID:           projection.EmployeeID,
		WorkDate:             projection.WorkDate,
		Source:               attendanceDailySourceLocal,
		ScheduledStartAt:     projection.ScheduledStartAt,
		ScheduledEndAt:       projection.ScheduledEndAt,
		ScheduledMinutes:     projection.RequiredMinutes,
		RequiredMinutes:      projection.RequiredMinutes,
		WorkedMinutes:        projection.WorkedMinutes,
		CreditedLeaveMinutes: projection.ApprovedLeaveMinutes,
		OvertimeMinutes:      projection.OvertimeMinutes,
		ClockInRecordID:      projection.ClockInRecordID,
		ClockOutRecordID:     projection.ClockOutRecordID,
		PunchCount:           projection.PunchCount,
		DayStatus:            projection.DayStatus,
		AnomalyReasons:       projection.AnomalyReasons,
		Payload:              projection.Payload,
		CreatedAt:            projection.ComputedAt,
		UpdatedAt:            projection.UpdatedAt,
	}
	if projection.ClockIn != nil {
		at := projection.ClockIn.ClockedAt
		record.ClockInAt = &at
	}
	if projection.ClockOut != nil {
		at := projection.ClockOut.ClockedAt
		record.ClockOutAt = &at
	}
	record.InputFingerprint = attendanceDailyRecordFingerprint(record)
	return record
}

func (c AttendanceService) upsertLocalAttendanceDailyRecord(ctx RequestContext, projection domain.AttendanceDayProjection) error {
	record := localAttendanceDailyRecord(projection)
	if err := c.upsertAttendanceDailyRecordPreservingCreatedAt(ctx, record); err != nil {
		return err
	}
	return c.reconcileAttendanceDailyRecord(ctx, record.EmployeeID, record.WorkDate)
}

func (c AttendanceService) upsertAttendanceDailyRecordPreservingCreatedAt(ctx RequestContext, record domain.AttendanceDailyRecord) error {
	existing, ok, err := c.store.GetAttendanceDailyRecord(
		goContext(ctx), ctx.TenantID, record.EmployeeID, record.WorkDate, record.Source,
	)
	if err != nil {
		return err
	}
	if ok {
		record.CreatedAt = existing.CreatedAt
	}
	return c.store.UpsertAttendanceDailyRecord(goContext(ctx), record)
}

func (c AttendanceService) aggregateEHRMSAttendanceLeaveMinutes(ctx RequestContext, employeeID, workDate string) error {
	summary, ok, err := c.store.GetAttendanceDailySummaryByEmployeeDate(goContext(ctx), ctx.TenantID, employeeID, workDate)
	if err != nil || !ok {
		return err
	}
	segments, err := c.store.ListAttendanceDailyLeaveSegments(goContext(ctx), ctx.TenantID, employeeID, workDate, workDate)
	if err != nil {
		return err
	}
	credited := 0
	for _, segment := range segments {
		if segment.DailySource == attendanceDailySourceEHRMS && segment.Counted && segment.LinkStatus == "matched" {
			credited += segment.Minutes
		}
	}
	return c.upsertEHRMSAttendanceDailyRecord(ctx, summary, credited)
}

func (c AttendanceService) reconcileAttendanceDailyRecord(ctx RequestContext, employeeID, workDate string) error {
	local, hasLocal, err := c.store.GetAttendanceDailyRecord(goContext(ctx), ctx.TenantID, employeeID, workDate, attendanceDailySourceLocal)
	if err != nil {
		return err
	}
	ehrms, hasEHRMS, err := c.store.GetAttendanceDailyRecord(goContext(ctx), ctx.TenantID, employeeID, workDate, attendanceDailySourceEHRMS)
	if err != nil {
		return err
	}
	if !hasLocal && !hasEHRMS {
		return nil
	}
	now := c.Now()
	reconciliation := domain.AttendanceDailyReconciliation{
		TenantID: ctx.TenantID, EmployeeID: employeeID, WorkDate: workDate,
		Status: "matched", Differences: map[string]any{}, ResolutionStatus: "unresolved",
		CreatedAt: now, UpdatedAt: now,
	}
	if hasLocal {
		reconciliation.LocalFingerprint = local.InputFingerprint
	}
	if hasEHRMS {
		reconciliation.EHRMSFingerprint = ehrms.InputFingerprint
	}
	switch {
	case !hasLocal:
		reconciliation.Status = "ehrms_only"
	case !hasEHRMS:
		reconciliation.Status = "local_only"
	default:
		reconciliation.Differences = attendanceDailyDifferences(local, ehrms)
		if len(reconciliation.Differences) > 0 {
			reconciliation.Status = "mismatch"
		}
	}
	if existing, ok, getErr := c.store.GetAttendanceDailyReconciliation(goContext(ctx), ctx.TenantID, employeeID, workDate); getErr != nil {
		return getErr
	} else if ok {
		reconciliation.CreatedAt = existing.CreatedAt
		reconciliation.ResolutionStatus = existing.ResolutionStatus
		reconciliation.ResolvedByAccountID = existing.ResolvedByAccountID
		reconciliation.ResolvedAt = existing.ResolvedAt
	}
	return c.store.UpsertAttendanceDailyReconciliation(goContext(ctx), reconciliation)
}

func attendanceDailyDifferences(local, ehrms domain.AttendanceDailyRecord) map[string]any {
	differences := map[string]any{}
	compareInt := func(field string, left, right int) {
		if left != right {
			differences[field] = map[string]any{"local": left, "ehrms": right, "delta": left - right}
		}
	}
	compareTime := func(field string, left, right *time.Time) {
		leftText, rightText := attendanceMinuteText(left), attendanceMinuteText(right)
		if leftText != rightText {
			differences[field] = map[string]any{"local": leftText, "ehrms": rightText}
		}
	}
	compareInt("scheduled_minutes", local.ScheduledMinutes, ehrms.ScheduledMinutes)
	compareInt("required_minutes", local.RequiredMinutes, ehrms.RequiredMinutes)
	compareInt("worked_minutes", local.WorkedMinutes, ehrms.WorkedMinutes)
	compareInt("credited_leave_minutes", local.CreditedLeaveMinutes, ehrms.CreditedLeaveMinutes)
	compareTime("clock_in_at", local.ClockInAt, ehrms.ClockInAt)
	compareTime("clock_out_at", local.ClockOutAt, ehrms.ClockOutAt)
	return differences
}

func attendanceDailyRecordFingerprint(record domain.AttendanceDailyRecord) string {
	value := struct {
		ScheduledStartAt     string
		ScheduledEndAt       string
		ScheduledMinutes     int
		RequiredMinutes      int
		WorkedMinutes        int
		CreditedLeaveMinutes int
		OvertimeMinutes      int
		ClockInAt            string
		ClockOutAt           string
		PunchCount           int
		DayStatus            string
		AnomalyReasons       []string
	}{
		attendanceMinuteText(record.ScheduledStartAt), attendanceMinuteText(record.ScheduledEndAt),
		record.ScheduledMinutes, record.RequiredMinutes, record.WorkedMinutes,
		record.CreditedLeaveMinutes, record.OvertimeMinutes,
		attendanceMinuteText(record.ClockInAt), attendanceMinuteText(record.ClockOutAt),
		record.PunchCount, record.DayStatus, append([]string(nil), record.AnomalyReasons...),
	}
	sort.Strings(value.AnomalyReasons)
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:])
}

func attendanceHoursToMinutes(hours float64) int {
	return int(math.Round(hours * 60))
}

func attendanceDailyTime(workDate, clock string) *time.Time {
	if clock == "" {
		return nil
	}
	value, ok := parseEHRMSLeaveDetailDateTime(workDate, clock)
	if !ok {
		return nil
	}
	return &value
}

func attendanceMinuteText(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Truncate(time.Minute).Format(time.RFC3339)
}

func ehrmsAttendanceDayStatus(record domain.AttendanceDailyRecord) string {
	switch {
	case record.ScheduledMinutes == 0 && record.RequiredMinutes == 0:
		return "off"
	case record.WorkedMinutes > 0:
		return "complete"
	case record.CreditedLeaveMinutes >= record.ScheduledMinutes && record.CreditedLeaveMinutes > 0:
		return "leave"
	default:
		return "missing_clock"
	}
}
