package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

const (
	ehrmsAttendanceFieldEmployeeNo = "員工編號"
	ehrmsAttendanceFieldDate       = "日期"
	ehrmsAttendanceFieldShiftStart = "班別開始"
	ehrmsAttendanceFieldShiftEnd   = "班別結束"
	ehrmsAttendanceFieldShiftHours = "班別工時"
	ehrmsAttendanceFieldDailyHours = "應出勤工時"
	ehrmsAttendanceFieldClockHours = "刷卡工時"
	ehrmsAttendanceSource          = "ehrms"
)

// SyncEHRMSAttendance 同步 eHRMS 考勤日彙總。
func (c AttendanceService) SyncEHRMSAttendance(ctx RequestContext, input EHRMSAttendanceSyncInput) (EHRMSAttendanceSyncResponse, error) {
	if c.ehrmsClient == nil {
		return EHRMSAttendanceSyncResponse{}, BadRequest("eHRMS is not configured")
	}
	mode, err := normalizeEHRMSSyncMode(input.Mode)
	if err != nil {
		return EHRMSAttendanceSyncResponse{}, err
	}
	since, err := normalizeEHRMSAttendanceSince(input.Since)
	if err != nil {
		return EHRMSAttendanceSyncResponse{}, err
	}
	_, decision, authzAudit, err := c.Service.Authorize(ctx,
		CheckRequest{ApplicationCode: AppAttendance, ResourceType: ResourceAttendanceClock, Action: ActionImport},
		AuditTarget{Event: "attendance.ehrms.sync", Resource: string(ResourceAttendanceClock)},
	)
	if err != nil {
		return EHRMSAttendanceSyncResponse{}, err
	}
	records, err := c.ehrmsClient.ListAttendance(goContext(ctx))
	if err != nil {
		c.logWarn(ctx, "eHRMS attendance fetch failed", "error", err)
		return EHRMSAttendanceSyncResponse{}, BadRequest("fetch eHRMS attendance failed")
	}
	response := EHRMSAttendanceSyncResponse{Fetched: len(records), Mode: mode, Since: since}
	if err := c.withTransaction(ctx, func(tx AttendanceService) error {
		for idx, record := range records {
			result := tx.syncEHRMSAttendanceRecord(ctx, record, idx+1, mode, since)
			response.Results = append(response.Results, result.result)
			response.RowErrors = append(response.RowErrors, result.rowErrors...)
			switch result.action {
			case "created":
				response.Created++
			case "updated":
				response.Updated++
			case "skipped":
				response.Skipped++
			case "failed":
				response.Failed++
			}
		}
		if err := tx.audit(ctx, "attendance.ehrms.sync", string(ResourceAttendanceClock), "ehrms", string(SeverityHigh), map[string]any{
			"source":  ehrmsAttendanceSource,
			"fetched": response.Fetched,
			"created": response.Created,
			"updated": response.Updated,
			"skipped": response.Skipped,
			"failed":  response.Failed,
			"mode":    mode,
			"since":   since,
		}); err != nil {
			return err
		}
		_ = decision
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		return EHRMSAttendanceSyncResponse{}, err
	}
	c.logInfo(ctx, "eHRMS attendance sync completed",
		"fetched", response.Fetched,
		"created", response.Created,
		"updated", response.Updated,
		"skipped", response.Skipped,
		"failed", response.Failed,
		"mode", mode,
		"since", since,
	)
	return response, nil
}

type ehrmsAttendanceSyncResult struct {
	action    string
	result    BatchEmployeeResult
	rowErrors []RowError
}

func (c AttendanceService) syncEHRMSAttendanceRecord(ctx RequestContext, record domain.EHRMSAttendanceRecord, rowNumber int, mode string, since string) ehrmsAttendanceSyncResult {
	summary, employeeNo, errors := c.ehrmsAttendanceSummaryCandidate(ctx, record, rowNumber)
	if len(errors) > 0 {
		return ehrmsAttendanceFailed(rowNumber, errors)
	}
	if since != "" && summary.WorkDate < since {
		return ehrmsAttendanceSkipped(rowNumber, "", "before_since", "attendance summary is before since date")
	}
	employee, ok, err := c.store.GetEmployeeByEmployeeNo(goContext(ctx), ctx.TenantID, employeeNo)
	if err != nil {
		return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "employee_no", Code: "store_error", Message: err.Error()}})
	}
	if !ok {
		return ehrmsAttendanceSkipped(rowNumber, "", "employee_not_found", "employee_no was not found for eHRMS attendance sync")
	}
	summary.EmployeeID = employee.ID
	existing, ok, err := c.store.GetAttendanceDailySummaryByExternalRef(goContext(ctx), ctx.TenantID, summary.ExternalRef)
	if err != nil {
		return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "external_ref", Code: "store_error", Message: err.Error()}})
	}
	if !ok {
		existing, ok, err = c.store.GetAttendanceDailySummaryByEmployeeDate(goContext(ctx), ctx.TenantID, employee.ID, summary.WorkDate)
		if err != nil {
			return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "work_date", Code: "store_error", Message: err.Error()}})
		}
	}
	update := ok
	switch mode {
	case employeeImportModeCreate:
		if update {
			return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "work_date", Code: "unique", Message: "attendance daily summary already exists"}})
		}
	case employeeImportModeUpdate:
		if !update {
			return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "work_date", Code: "not_found", Message: "attendance daily summary was not found for eHRMS sync"}})
		}
	}
	if update {
		summary.ID = existing.ID
		summary.CreatedAt = existing.CreatedAt
	}
	if err := c.store.UpsertAttendanceDailySummary(goContext(ctx), summary); err != nil {
		return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "work_date", Code: "store_error", Message: err.Error()}})
	}
	action := "created"
	if update {
		action = "updated"
	}
	return ehrmsAttendanceSyncResult{action: action, result: BatchEmployeeResult{RowNumber: rowNumber, EmployeeID: employee.ID, Success: true, Action: action, Message: action}}
}

func (c AttendanceService) ehrmsAttendanceSummaryCandidate(ctx RequestContext, record domain.EHRMSAttendanceRecord, rowNumber int) (AttendanceDailySummary, string, []RowError) {
	errors := make([]RowError, 0)
	employeeNo := ehrmsAttendanceValue(record, ehrmsAttendanceFieldEmployeeNo)
	workDate := normalizeEHRMSAttendanceDate(ehrmsAttendanceValue(record, ehrmsAttendanceFieldDate))
	if employeeNo == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "employee_no", Code: "required", Message: "employee_no is required"})
	}
	if workDate == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "date", Code: "invalid", Message: "date must be YYYY-MM-DD"})
	}
	shiftStart := normalizeEHRMSAttendanceTime(ehrmsAttendanceValue(record, ehrmsAttendanceFieldShiftStart))
	if ehrmsAttendanceValue(record, ehrmsAttendanceFieldShiftStart) != "" && shiftStart == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "shift_start", Code: "invalid", Message: "shift_start must be HH:MM"})
	}
	shiftEnd := normalizeEHRMSAttendanceTime(ehrmsAttendanceValue(record, ehrmsAttendanceFieldShiftEnd))
	if ehrmsAttendanceValue(record, ehrmsAttendanceFieldShiftEnd) != "" && shiftEnd == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "shift_end", Code: "invalid", Message: "shift_end must be HH:MM"})
	}
	shiftHours, ok := parseEHRMSAttendanceHours(ehrmsAttendanceValue(record, ehrmsAttendanceFieldShiftHours))
	if !ok {
		errors = append(errors, RowError{Row: rowNumber, Field: "shift_hours", Code: "invalid", Message: "shift_hours must be a number"})
	}
	dailyHours, ok := parseEHRMSAttendanceHours(ehrmsAttendanceValue(record, ehrmsAttendanceFieldDailyHours))
	if !ok {
		errors = append(errors, RowError{Row: rowNumber, Field: "daily_hours", Code: "invalid", Message: "daily_hours must be a number"})
	}
	clockHours, ok := parseEHRMSAttendanceHours(ehrmsAttendanceValue(record, ehrmsAttendanceFieldClockHours))
	if !ok {
		errors = append(errors, RowError{Row: rowNumber, Field: "clock_hours", Code: "invalid", Message: "clock_hours must be a number"})
	}
	now := c.Now()
	return AttendanceDailySummary{
		ID:          utils.NewID("ads"),
		TenantID:    ctx.TenantID,
		WorkDate:    workDate,
		ShiftStart:  shiftStart,
		ShiftEnd:    shiftEnd,
		ShiftHours:  shiftHours,
		DailyHours:  dailyHours,
		ClockHours:  clockHours,
		Source:      ehrmsAttendanceSource,
		ExternalRef: fmt.Sprintf("%s:%s", employeeNo, workDate),
		CreatedAt:   now,
		UpdatedAt:   now,
	}, employeeNo, errors
}

func ehrmsAttendanceFailed(rowNumber int, errors []RowError) ehrmsAttendanceSyncResult {
	return ehrmsAttendanceSyncResult{
		action:    "failed",
		rowErrors: errors,
		result:    BatchEmployeeResult{RowNumber: rowNumber, Success: false, Code: "import_validation_failed", Message: firstRowErrorMessage(errors)},
	}
}

func ehrmsAttendanceSkipped(rowNumber int, employeeID string, code string, message string) ehrmsAttendanceSyncResult {
	return ehrmsAttendanceSyncResult{
		action: "skipped",
		result: BatchEmployeeResult{RowNumber: rowNumber, EmployeeID: employeeID, Success: true, Action: "skipped", Code: code, Message: message},
	}
}

func normalizeEHRMSAttendanceSince(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if parsed, err := time.Parse(time.DateOnly, value); err == nil {
		return parsed.Format(time.DateOnly), nil
	}
	return "", BadRequest("since must be YYYY-MM-DD")
}

func normalizeEHRMSAttendanceDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := time.Parse(time.DateOnly, value); err == nil {
		return parsed.Format(time.DateOnly)
	}
	if parsed, err := utils.ParseDate(value); err == nil {
		return parsed.UTC().Format(time.DateOnly)
	}
	return ""
}

func normalizeEHRMSAttendanceTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, layout := range []string{"15:04", "15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("15:04")
		}
	}
	return ""
}

func parseEHRMSAttendanceHours(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, true
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func ehrmsAttendanceValue(record domain.EHRMSAttendanceRecord, key string) string {
	if len(record) == 0 {
		return ""
	}
	return strings.TrimSpace(record[key])
}
