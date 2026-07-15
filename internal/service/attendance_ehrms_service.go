package service

import (
	"crypto/sha1"
	"fmt"
	"strconv"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

const (
	ehrmsAttendanceFieldEmployeeNo      = "員工編號"
	ehrmsAttendanceFieldDate            = "日期"
	ehrmsAttendanceFieldShiftStart      = "班別開始"
	ehrmsAttendanceFieldShiftEnd        = "班別結束"
	ehrmsAttendanceFieldShiftHours      = "班別工時"
	ehrmsAttendanceFieldDailyHours      = "應出勤工時"
	ehrmsAttendanceFieldClockHours      = "刷卡工時"
	ehrmsAttendanceFieldClockStart      = "clock_start"
	ehrmsAttendanceFieldClockEnd        = "clock_end"
	ehrmsAttendanceFieldAttendStart     = "attend_start"
	ehrmsAttendanceFieldAttendEnd       = "attend_end"
	ehrmsAttendanceFieldAttendHours     = "attend_hours"
	ehrmsAttendanceFieldAttendCounted   = "attend_counted"
	ehrmsAttendanceFieldLeaveType       = "leave_type"
	ehrmsAttendanceFieldLeaveStart      = "leave_start"
	ehrmsAttendanceFieldLeaveEnd        = "leave_end"
	ehrmsAttendanceFieldLeaveHours      = "leave_hours"
	ehrmsAttendanceFieldLeaveCounted    = "leave_counted"
	ehrmsAttendanceFieldLeave2Type      = "leave2_type"
	ehrmsAttendanceFieldLeave2Start     = "leave2_start"
	ehrmsAttendanceFieldLeave2End       = "leave2_end"
	ehrmsAttendanceFieldLeave2Hours     = "leave2_hours"
	ehrmsAttendanceFieldLeave2Counted   = "leave2_counted"
	ehrmsAttendanceFieldOvertimeStart   = "overtime_start"
	ehrmsAttendanceFieldOvertimeEnd     = "overtime_end"
	ehrmsAttendanceFieldOvertimeHours   = "overtime_hours"
	ehrmsAttendanceFieldOvertimeCounted = "overtime_counted"
	ehrmsAttendanceSource               = "ehrms"
	defaultEHRMSAttendanceSyncWindow    = 30 * 24 * time.Hour

	ehrmsLeaveBalanceFieldEmployeeNo  = "員工編號"
	ehrmsLeaveBalanceFieldYear        = "年度"
	ehrmsLeaveBalanceFieldLeaveType   = "假別"
	ehrmsLeaveBalanceFieldUnit        = "單位"
	ehrmsLeaveBalanceFieldQuota       = "額度"
	ehrmsLeaveBalanceFieldUsed        = "已使用"
	ehrmsLeaveBalanceFieldRemaining   = "餘額"
	ehrmsLeaveBalanceFieldGrantStart  = "發放起始日"
	ehrmsLeaveBalanceFieldExpireDate  = "到期日"
	ehrmsLeaveBalanceFieldCarryIn     = "遞延餘額"
	ehrmsLeaveBalanceFieldCarryExpire = "遞延到期日"

	ehrmsLeaveDetailFieldEmployeeNo = "員工編號"
	ehrmsLeaveDetailFieldDate       = "日期"
	ehrmsLeaveDetailFieldLeaveType  = "假別"
	ehrmsLeaveDetailFieldStart      = "開始時間"
	ehrmsLeaveDetailFieldEnd        = "結束時間"
	ehrmsLeaveDetailFieldHours      = "時數"
)

var ehrmsAttendanceOnlyLeaveTypes = map[string]struct{}{
	"absenteeism":      {},
	"attendance hours": {},
	"holiday overtime": {},
	"missing punch":    {},
	"overtime":         {},
	"例假日加班":            {},
	"出勤時數":             {},
	"加班":               {},
	"忘刷忘帶卡":            {},
}

// SyncEHRMSAttendance synchronizes tenant-wide attendance data only for tenant-wide grants.
func (c AttendanceService) SyncEHRMSAttendance(ctx RequestContext, input EHRMSAttendanceSyncInput) (EHRMSAttendanceSyncResponse, error) {
	if c.ehrmsClient == nil {
		return EHRMSAttendanceSyncResponse{}, BadRequest("eHRMS is not configured")
	}
	mode, err := normalizeEHRMSSyncMode(input.Mode)
	if err != nil {
		return EHRMSAttendanceSyncResponse{}, err
	}
	since, err := c.effectiveEHRMSAttendanceSince(input.Since)
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
	if err := requireTenantWideEHRMSSyncScope(decision); err != nil {
		return EHRMSAttendanceSyncResponse{}, err
	}
	records, err := c.ehrmsClient.ListAttendance(goContext(ctx))
	if err != nil {
		c.logWarn(ctx, "eHRMS attendance fetch failed", "error", err)
		return EHRMSAttendanceSyncResponse{}, ehrmsFetchError("attendance", err)
	}
	leaveBalances, err := c.ehrmsClient.ListLeaveBalances(goContext(ctx))
	if err != nil {
		c.logWarn(ctx, "eHRMS leave balance fetch failed", "error", err)
		return EHRMSAttendanceSyncResponse{}, ehrmsFetchError("leave balances", err)
	}
	leaveDetails, err := c.ehrmsClient.ListLeaveDetails(goContext(ctx))
	if err != nil {
		c.logWarn(ctx, "eHRMS leave detail fetch failed", "error", err)
		return EHRMSAttendanceSyncResponse{}, ehrmsFetchError("leave details", err)
	}
	response := EHRMSAttendanceSyncResponse{
		Fetched:              len(records),
		LeaveBalancesFetched: len(leaveBalances),
		LeaveDetailsFetched:  len(leaveDetails),
		Mode:                 mode,
		Since:                since,
	}
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
		for idx, record := range leaveBalances {
			result := tx.syncEHRMSLeaveBalanceRecord(ctx, record, idx+1)
			response.Results = append(response.Results, result.result)
			response.RowErrors = append(response.RowErrors, result.rowErrors...)
			switch result.action {
			case "upserted":
				response.LeaveBalancesUpserted++
			case "skipped":
				response.LeaveBalancesSkipped++
			case "failed":
				response.LeaveBalancesFailed++
			}
		}
		for idx, record := range leaveDetails {
			result := tx.syncEHRMSLeaveDetailRecord(ctx, record, idx+1, mode, since)
			response.Results = append(response.Results, result.result)
			response.RowErrors = append(response.RowErrors, result.rowErrors...)
			switch result.action {
			case "created":
				response.LeaveDetailsCreated++
			case "updated":
				response.LeaveDetailsUpdated++
			case "skipped":
				response.LeaveDetailsSkipped++
			case "failed":
				response.LeaveDetailsFailed++
			}
		}
		if err := tx.audit(ctx, "attendance.ehrms.sync", string(ResourceAttendanceClock), "ehrms", string(SeverityHigh), map[string]any{
			"source":                  ehrmsAttendanceSource,
			"fetched":                 response.Fetched,
			"created":                 response.Created,
			"updated":                 response.Updated,
			"skipped":                 response.Skipped,
			"failed":                  response.Failed,
			"leave_balances_fetched":  response.LeaveBalancesFetched,
			"leave_balances_upserted": response.LeaveBalancesUpserted,
			"leave_balances_skipped":  response.LeaveBalancesSkipped,
			"leave_balances_failed":   response.LeaveBalancesFailed,
			"leave_details_fetched":   response.LeaveDetailsFetched,
			"leave_details_created":   response.LeaveDetailsCreated,
			"leave_details_updated":   response.LeaveDetailsUpdated,
			"leave_details_skipped":   response.LeaveDetailsSkipped,
			"leave_details_failed":    response.LeaveDetailsFailed,
			"mode":                    mode,
			"since":                   since,
		}); err != nil {
			return err
		}
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
		"leave_balances_fetched", response.LeaveBalancesFetched,
		"leave_balances_upserted", response.LeaveBalancesUpserted,
		"leave_details_fetched", response.LeaveDetailsFetched,
		"leave_details_created", response.LeaveDetailsCreated,
		"leave_details_updated", response.LeaveDetailsUpdated,
		"leave_details_skipped", response.LeaveDetailsSkipped,
		"leave_details_failed", response.LeaveDetailsFailed,
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

// syncEHRMSLeaveBalanceRecord excludes attendance metrics before persisting an employee leave balance.
func (c AttendanceService) syncEHRMSLeaveBalanceRecord(ctx RequestContext, record domain.EHRMSLeaveBalanceRecord, rowNumber int) ehrmsAttendanceSyncResult {
	leaveTypeRaw := ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldLeaveType)
	if isEHRMSAttendanceOnlyLeaveType(leaveTypeRaw) {
		return ehrmsAttendanceSkipped(rowNumber, "", "non_leave_balance_type", "attendance-only type was excluded from leave balance sync")
	}
	balance, employeeNo, errors := c.ehrmsLeaveBalanceCandidate(ctx, record, rowNumber)
	if len(errors) > 0 {
		return ehrmsAttendanceFailed(rowNumber, errors)
	}
	employee, ok, err := c.store.GetEmployeeByEmployeeNo(goContext(ctx), ctx.TenantID, employeeNo)
	if err != nil {
		return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "employee_no", Code: "store_error", Message: err.Error()}})
	}
	if !ok {
		return ehrmsAttendanceSkipped(rowNumber, "", "employee_not_found", "employee_no was not found for eHRMS leave balance sync")
	}
	balance.EmployeeID = employee.ID
	balance.ID = ehrmsStableID("ehrms-lb", ctx.TenantID, employee.EmployeeNo, balance.LeaveType, balance.PeriodStart, balance.PeriodEnd)
	if err := c.store.UpsertLeaveBalance(goContext(ctx), balance); err != nil {
		return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "leave_type", Code: "store_error", Message: err.Error()}})
	}
	return ehrmsAttendanceSyncResult{action: "upserted", result: BatchEmployeeResult{RowNumber: rowNumber, EmployeeID: employee.ID, Success: true, Action: "upserted", Message: "upserted"}}
}

// syncEHRMSLeaveDetailRecord excludes attendance metrics before persisting an approved external leave request.
func (c AttendanceService) syncEHRMSLeaveDetailRecord(ctx RequestContext, record domain.EHRMSLeaveDetailRecord, rowNumber int, mode string, since string) ehrmsAttendanceSyncResult {
	leaveTypeRaw := ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldLeaveType)
	if isEHRMSAttendanceOnlyLeaveType(leaveTypeRaw) {
		return ehrmsAttendanceSkipped(rowNumber, "", "non_leave_detail_type", "attendance-only type was excluded from leave detail sync")
	}
	request, employeeNo, workDate, errors := c.ehrmsLeaveDetailCandidate(ctx, record, rowNumber)
	if len(errors) > 0 {
		return ehrmsAttendanceFailed(rowNumber, errors)
	}
	if since != "" && workDate < since {
		return ehrmsAttendanceSkipped(rowNumber, "", "before_since", "leave detail is before since date")
	}
	employee, ok, err := c.store.GetEmployeeByEmployeeNo(goContext(ctx), ctx.TenantID, employeeNo)
	if err != nil {
		return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "employee_no", Code: "store_error", Message: err.Error()}})
	}
	if !ok {
		return ehrmsAttendanceSkipped(rowNumber, "", "employee_not_found", "employee_no was not found for eHRMS leave detail sync")
	}
	request.EmployeeID = employee.ID
	request.ID = ehrmsStableID("ehrms-lr", ctx.TenantID, employee.EmployeeNo, workDate, request.LeaveType, request.StartAt.Format(time.RFC3339), request.EndAt.Format(time.RFC3339))
	existing, exists, err := c.store.GetLeaveRequest(goContext(ctx), ctx.TenantID, request.ID)
	if err != nil {
		return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "leave_detail", Code: "store_error", Message: err.Error()}})
	}
	switch mode {
	case employeeImportModeCreate:
		if exists {
			return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "leave_detail", Code: "unique", Message: "leave detail already exists"}})
		}
	case employeeImportModeUpdate:
		if !exists {
			return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "leave_detail", Code: "not_found", Message: "leave detail was not found for eHRMS sync"}})
		}
	}
	if exists {
		request.CreatedAt = existing.CreatedAt
	}
	if err := c.store.UpsertLeaveRequest(goContext(ctx), request); err != nil {
		return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "leave_detail", Code: "store_error", Message: err.Error()}})
	}
	action := "created"
	if exists {
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
	asOf := c.Now()
	if parsed, err := time.Parse(time.DateOnly, workDate); err == nil {
		asOf = parsed
	}
	leaveTypeRaw := ehrmsAttendanceValue(record, ehrmsAttendanceFieldLeaveType)
	leaveType, _, leaveTypeFound, mappingErr := c.resolveExternalLeaveTypeCode(ctx, ehrmsAttendanceSource, leaveTypeRaw, asOf)
	if mappingErr != nil {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave_type", Code: "store_error", Message: mappingErr.Error()})
	} else if leaveTypeRaw != "" && !leaveTypeFound {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave_type", Code: leaveSyncIssueUnmapped, Message: "leave_type requires HR mapping"})
	}
	leave2TypeRaw := ehrmsAttendanceValue(record, ehrmsAttendanceFieldLeave2Type)
	leave2Type, _, leave2TypeFound, mappingErr := c.resolveExternalLeaveTypeCode(ctx, ehrmsAttendanceSource, leave2TypeRaw, asOf)
	if mappingErr != nil {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave2_type", Code: "store_error", Message: mappingErr.Error()})
	} else if leave2TypeRaw != "" && !leave2TypeFound {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave2_type", Code: leaveSyncIssueUnmapped, Message: "leave2_type requires HR mapping"})
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
	clockStart := ehrmsAttendanceTimeField(record, ehrmsAttendanceFieldClockStart, rowNumber, &errors)
	clockEnd := ehrmsAttendanceTimeField(record, ehrmsAttendanceFieldClockEnd, rowNumber, &errors)
	attendStart := ehrmsAttendanceTimeField(record, ehrmsAttendanceFieldAttendStart, rowNumber, &errors)
	attendEnd := ehrmsAttendanceTimeField(record, ehrmsAttendanceFieldAttendEnd, rowNumber, &errors)
	leaveStart := ehrmsAttendanceTimeField(record, ehrmsAttendanceFieldLeaveStart, rowNumber, &errors)
	leaveEnd := ehrmsAttendanceTimeField(record, ehrmsAttendanceFieldLeaveEnd, rowNumber, &errors)
	leave2Start := ehrmsAttendanceTimeField(record, ehrmsAttendanceFieldLeave2Start, rowNumber, &errors)
	leave2End := ehrmsAttendanceTimeField(record, ehrmsAttendanceFieldLeave2End, rowNumber, &errors)
	overtimeStart := ehrmsAttendanceTimeField(record, ehrmsAttendanceFieldOvertimeStart, rowNumber, &errors)
	overtimeEnd := ehrmsAttendanceTimeField(record, ehrmsAttendanceFieldOvertimeEnd, rowNumber, &errors)
	attendHours := ehrmsAttendanceHoursField(record, ehrmsAttendanceFieldAttendHours, rowNumber, &errors)
	leaveHours := ehrmsAttendanceHoursField(record, ehrmsAttendanceFieldLeaveHours, rowNumber, &errors)
	leave2Hours := ehrmsAttendanceHoursField(record, ehrmsAttendanceFieldLeave2Hours, rowNumber, &errors)
	overtimeHours := ehrmsAttendanceHoursField(record, ehrmsAttendanceFieldOvertimeHours, rowNumber, &errors)
	now := c.Now()
	return AttendanceDailySummary{
		ID:              utils.NewID("ads"),
		TenantID:        ctx.TenantID,
		WorkDate:        workDate,
		ShiftStart:      shiftStart,
		ShiftEnd:        shiftEnd,
		ShiftHours:      shiftHours,
		DailyHours:      dailyHours,
		ClockHours:      clockHours,
		ClockStart:      clockStart,
		ClockEnd:        clockEnd,
		AttendStart:     attendStart,
		AttendEnd:       attendEnd,
		AttendHours:     attendHours,
		AttendCounted:   ehrmsAttendanceBoolValue(record, ehrmsAttendanceFieldAttendCounted),
		LeaveType:       leaveType,
		LeaveStart:      leaveStart,
		LeaveEnd:        leaveEnd,
		LeaveHours:      leaveHours,
		LeaveCounted:    ehrmsAttendanceBoolValue(record, ehrmsAttendanceFieldLeaveCounted),
		Leave2Type:      leave2Type,
		Leave2Start:     leave2Start,
		Leave2End:       leave2End,
		Leave2Hours:     leave2Hours,
		Leave2Counted:   ehrmsAttendanceBoolValue(record, ehrmsAttendanceFieldLeave2Counted),
		OvertimeStart:   overtimeStart,
		OvertimeEnd:     overtimeEnd,
		OvertimeHours:   overtimeHours,
		OvertimeCounted: ehrmsAttendanceBoolValue(record, ehrmsAttendanceFieldOvertimeCounted),
		Payload:         ehrmsAttendancePayload(record),
		Source:          ehrmsAttendanceSource,
		ExternalRef:     fmt.Sprintf("%s:%s", employeeNo, workDate),
		CreatedAt:       now,
		UpdatedAt:       now,
	}, employeeNo, errors
}

func (c AttendanceService) ehrmsLeaveBalanceCandidate(ctx RequestContext, record domain.EHRMSLeaveBalanceRecord, rowNumber int) (LeaveBalance, string, []RowError) {
	errors := make([]RowError, 0)
	employeeNo := ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldEmployeeNo)
	leaveTypeRaw := ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldLeaveType)
	periodStart := normalizeEHRMSAttendanceDate(ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldGrantStart))
	periodEnd := normalizeEHRMSAttendanceDate(ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldExpireDate))
	asOf := c.Now()
	if parsed, err := time.Parse(time.DateOnly, periodStart); err == nil {
		asOf = parsed
	}
	leaveType, leaveTypeID, leaveTypeFound, mappingErr := c.resolveExternalLeaveTypeCode(ctx, ehrmsAttendanceSource, leaveTypeRaw, asOf)
	if mappingErr != nil {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave_type", Code: "store_error", Message: mappingErr.Error()})
	} else if leaveTypeRaw != "" && !leaveTypeFound {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave_type", Code: leaveSyncIssueUnmapped, Message: "leave_type requires HR mapping"})
	}
	if employeeNo == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "employee_no", Code: "required", Message: "employee_no is required"})
	}
	if leaveTypeRaw == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave_type", Code: "required", Message: "leave_type is required"})
	}
	dayHours := 8.0
	if policy, err := c.loadAttendancePolicyResponse(ctx); err == nil {
		dayHours = standardDayHours(policy.WorkTime)
	}
	unit := strings.ToLower(ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldUnit))
	quota, ok := parseEHRMSLeaveBalanceNumber(ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldQuota), unit, dayHours)
	if !ok {
		errors = append(errors, RowError{Row: rowNumber, Field: "quota", Code: "invalid", Message: "quota must be a number"})
	}
	used, ok := parseEHRMSLeaveBalanceNumber(ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldUsed), unit, dayHours)
	if !ok {
		errors = append(errors, RowError{Row: rowNumber, Field: "used", Code: "invalid", Message: "used must be a number"})
	}
	remainingRaw := ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldRemaining)
	remaining, ok := parseEHRMSLeaveBalanceNumber(remainingRaw, unit, dayHours)
	if !ok {
		errors = append(errors, RowError{Row: rowNumber, Field: "remaining", Code: "invalid", Message: "remaining must be a number"})
	}
	if strings.TrimSpace(remainingRaw) == "" && quota > 0 {
		remaining = quota - used
		if remaining < 0 {
			remaining = 0
		}
	}
	now := c.Now()
	return LeaveBalance{
		TenantID:       ctx.TenantID,
		LeaveType:      leaveType,
		LeaveTypeID:    leaveTypeID,
		RemainingHours: remaining,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		GrantedHours:   quota,
		UsedHours:      used,
		Source:         ehrmsAttendanceSource,
		UpdatedAt:      now,
	}, employeeNo, errors
}

func (c AttendanceService) ehrmsLeaveDetailCandidate(ctx RequestContext, record domain.EHRMSLeaveDetailRecord, rowNumber int) (LeaveRequest, string, string, []RowError) {
	errors := make([]RowError, 0)
	employeeNo := ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldEmployeeNo)
	workDate := normalizeEHRMSAttendanceDate(ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldDate))
	leaveTypeRaw := ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldLeaveType)
	asOf := c.Now()
	if parsed, err := time.Parse(time.DateOnly, workDate); err == nil {
		asOf = parsed
	}
	leaveType, leaveTypeID, leaveTypeFound, mappingErr := c.resolveExternalLeaveTypeCode(ctx, ehrmsAttendanceSource, leaveTypeRaw, asOf)
	if mappingErr != nil {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave_type", Code: "store_error", Message: mappingErr.Error()})
	} else if leaveTypeRaw != "" && !leaveTypeFound {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave_type", Code: leaveSyncIssueUnmapped, Message: "leave_type requires HR mapping"})
	}
	if employeeNo == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "employee_no", Code: "required", Message: "employee_no is required"})
	}
	if workDate == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "date", Code: "invalid", Message: "date must be YYYY-MM-DD"})
	}
	if leaveTypeRaw == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave_type", Code: "required", Message: "leave_type is required"})
	}
	startAt, ok := parseEHRMSLeaveDetailDateTime(workDate, ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldStart))
	if !ok {
		errors = append(errors, RowError{Row: rowNumber, Field: "start", Code: "invalid", Message: "start must be HH:MM or datetime"})
	}
	endAt, ok := parseEHRMSLeaveDetailDateTime(workDate, ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldEnd))
	if !ok {
		errors = append(errors, RowError{Row: rowNumber, Field: "end", Code: "invalid", Message: "end must be HH:MM or datetime"})
	}
	if !startAt.IsZero() && !endAt.IsZero() && !endAt.After(startAt) {
		errors = append(errors, RowError{Row: rowNumber, Field: "end", Code: "invalid", Message: "end must be after start"})
	}
	hours, ok := parseEHRMSAttendanceHours(ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldHours))
	if !ok || hours <= 0 {
		errors = append(errors, RowError{Row: rowNumber, Field: "hours", Code: "invalid", Message: "hours must be greater than zero"})
	}
	now := c.Now()
	return LeaveRequest{
		TenantID: ctx.TenantID, LeaveType: leaveType, LeaveTypeID: leaveTypeID,
		RuleSnapshot:       map[string]any{"source": ehrmsAttendanceSource, "leave_type": leaveType},
		EvaluationSnapshot: map[string]any{"source": ehrmsAttendanceSource, "eligible": true, "status": "approved_external"},
		StartAt:            startAt, EndAt: endAt, Hours: hours, Reason: "eHRMS leave detail", Status: "approved", CreatedAt: now,
	}, employeeNo, workDate, errors
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

// NormalizeEHRMSAttendanceSince bounds an upstream attendance cursor to the configured sync window.
func NormalizeEHRMSAttendanceSince(input string, now time.Time, window time.Duration) (string, error) {
	if window <= 0 {
		window = defaultEHRMSAttendanceSyncWindow
	}
	floor := now.Add(-window).Format(time.DateOnly)
	input = strings.TrimSpace(input)
	if input == "" {
		return floor, nil
	}
	parsed, err := time.Parse(time.DateOnly, input)
	if err != nil {
		return "", BadRequest("since must be YYYY-MM-DD")
	}
	explicit := parsed.Format(time.DateOnly)
	floorTime, err := time.Parse(time.DateOnly, floor)
	if err != nil {
		return explicit, nil
	}
	if parsed.Before(floorTime) {
		return floor, nil
	}
	return explicit, nil
}

func (c AttendanceService) effectiveEHRMSAttendanceSince(input string) (string, error) {
	return NormalizeEHRMSAttendanceSince(input, c.Now(), defaultEHRMSAttendanceSyncWindow)
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
	for _, layout := range []string{"15:04", "15:04:05", "2006-01-02 15:04", "2006-01-02 15:04:05"} {
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

func parseEHRMSLeaveBalanceNumber(value string, unit string, configuredDayHours ...float64) (float64, bool) {
	n, ok := parseEHRMSAttendanceHours(value)
	if !ok {
		return 0, false
	}
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "days", "day":
		dayHours := 8.0
		if len(configuredDayHours) > 0 && configuredDayHours[0] > 0 {
			dayHours = configuredDayHours[0]
		}
		return n * dayHours, true
	default:
		return n, true
	}
}

func parseEHRMSLeaveDetailDateTime(workDate string, value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if workDate == "" || value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006/01/02 15:04:05", "2006/01/02 15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, attendanceClockLocation); err == nil {
			return parsed, true
		}
	}
	for _, layout := range []string{"15:04:05", "15:04"} {
		if parsed, err := time.ParseInLocation(time.DateOnly+" "+layout, workDate+" "+value, attendanceClockLocation); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func ehrmsAttendanceTimeField(record domain.EHRMSAttendanceRecord, key string, rowNumber int, errors *[]RowError) string {
	raw := ehrmsAttendanceValue(record, key)
	value := normalizeEHRMSAttendanceTime(raw)
	if raw != "" && value == "" {
		*errors = append(*errors, RowError{Row: rowNumber, Field: key, Code: "invalid", Message: key + " must be HH:MM"})
	}
	return value
}

func ehrmsAttendanceHoursField(record domain.EHRMSAttendanceRecord, key string, rowNumber int, errors *[]RowError) float64 {
	value, ok := parseEHRMSAttendanceHours(ehrmsAttendanceValue(record, key))
	if !ok {
		*errors = append(*errors, RowError{Row: rowNumber, Field: key, Code: "invalid", Message: key + " must be a number"})
	}
	return value
}

func ehrmsAttendanceBoolValue(record domain.EHRMSAttendanceRecord, key string) bool {
	switch strings.ToLower(strings.TrimSpace(ehrmsAttendanceValue(record, key))) {
	case "v", "true", "1", "是":
		return true
	default:
		return false
	}
}

func ehrmsAttendancePayload(record domain.EHRMSAttendanceRecord) map[string]any {
	if len(record) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(record))
	for key, value := range record {
		out[key] = normalizeEHRMSPlaceholder(value)
	}
	return out
}

func ehrmsAttendanceValue(record domain.EHRMSAttendanceRecord, key string) string {
	if len(record) == 0 {
		return ""
	}
	if value := strings.TrimSpace(record[key]); value != "" {
		return value
	}
	switch key {
	case ehrmsAttendanceFieldEmployeeNo:
		return strings.TrimSpace(record["emp_id"])
	case ehrmsAttendanceFieldDate:
		return strings.TrimSpace(record["date"])
	case ehrmsAttendanceFieldShiftStart:
		return strings.TrimSpace(record["shift_start"])
	case ehrmsAttendanceFieldShiftEnd:
		return strings.TrimSpace(record["shift_end"])
	case ehrmsAttendanceFieldShiftHours:
		return strings.TrimSpace(record["shift_hours"])
	case ehrmsAttendanceFieldDailyHours:
		return strings.TrimSpace(record["daily_hours"])
	case ehrmsAttendanceFieldClockHours:
		return strings.TrimSpace(record["clock_hours"])
	case ehrmsAttendanceFieldClockStart:
		return strings.TrimSpace(record["clock_start"])
	case ehrmsAttendanceFieldClockEnd:
		return strings.TrimSpace(record["clock_end"])
	case ehrmsAttendanceFieldAttendStart:
		return strings.TrimSpace(record["attend_start"])
	case ehrmsAttendanceFieldAttendEnd:
		return strings.TrimSpace(record["attend_end"])
	case ehrmsAttendanceFieldAttendHours:
		return strings.TrimSpace(record["attend_hours"])
	case ehrmsAttendanceFieldAttendCounted:
		return strings.TrimSpace(record["attend_counted"])
	case ehrmsAttendanceFieldLeaveType:
		return strings.TrimSpace(record["leave_type"])
	case ehrmsAttendanceFieldLeaveStart:
		return strings.TrimSpace(record["leave_start"])
	case ehrmsAttendanceFieldLeaveEnd:
		return strings.TrimSpace(record["leave_end"])
	case ehrmsAttendanceFieldLeaveHours:
		return strings.TrimSpace(record["leave_hours"])
	case ehrmsAttendanceFieldLeaveCounted:
		return strings.TrimSpace(record["leave_counted"])
	case ehrmsAttendanceFieldLeave2Type:
		return strings.TrimSpace(record["leave2_type"])
	case ehrmsAttendanceFieldLeave2Start:
		return strings.TrimSpace(record["leave2_start"])
	case ehrmsAttendanceFieldLeave2End:
		return strings.TrimSpace(record["leave2_end"])
	case ehrmsAttendanceFieldLeave2Hours:
		return strings.TrimSpace(record["leave2_hours"])
	case ehrmsAttendanceFieldLeave2Counted:
		return strings.TrimSpace(record["leave2_counted"])
	case ehrmsAttendanceFieldOvertimeStart:
		return strings.TrimSpace(record["overtime_start"])
	case ehrmsAttendanceFieldOvertimeEnd:
		return strings.TrimSpace(record["overtime_end"])
	case ehrmsAttendanceFieldOvertimeHours:
		return strings.TrimSpace(record["overtime_hours"])
	case ehrmsAttendanceFieldOvertimeCounted:
		return strings.TrimSpace(record["overtime_counted"])
	default:
		return ""
	}
}

func ehrmsLeaveBalanceValue(record domain.EHRMSLeaveBalanceRecord, key string) string {
	if len(record) == 0 {
		return ""
	}
	if value := strings.TrimSpace(record[key]); value != "" {
		return value
	}
	switch key {
	case ehrmsLeaveBalanceFieldEmployeeNo:
		return strings.TrimSpace(record["emp_id"])
	case ehrmsLeaveBalanceFieldYear:
		return strings.TrimSpace(record["year"])
	case ehrmsLeaveBalanceFieldLeaveType:
		return strings.TrimSpace(record["leave_type"])
	case ehrmsLeaveBalanceFieldUnit:
		return strings.TrimSpace(record["unit"])
	case ehrmsLeaveBalanceFieldQuota:
		return strings.TrimSpace(record["quota"])
	case ehrmsLeaveBalanceFieldUsed:
		return strings.TrimSpace(record["used"])
	case ehrmsLeaveBalanceFieldRemaining:
		return strings.TrimSpace(record["remaining"])
	case ehrmsLeaveBalanceFieldGrantStart:
		return strings.TrimSpace(record["grant_start"])
	case ehrmsLeaveBalanceFieldExpireDate:
		return strings.TrimSpace(record["expire_date"])
	case ehrmsLeaveBalanceFieldCarryIn:
		return strings.TrimSpace(record["carry_in"])
	case ehrmsLeaveBalanceFieldCarryExpire:
		return strings.TrimSpace(record["carry_expire"])
	default:
		return ""
	}
}

// isEHRMSAttendanceOnlyLeaveType rejects upstream attendance metrics that share the leave feeds.
func isEHRMSAttendanceOnlyLeaveType(value string) bool {
	_, excluded := ehrmsAttendanceOnlyLeaveTypes[strings.ToLower(strings.TrimSpace(value))]
	return excluded
}

func ehrmsLeaveDetailValue(record domain.EHRMSLeaveDetailRecord, key string) string {
	if len(record) == 0 {
		return ""
	}
	if value := strings.TrimSpace(record[key]); value != "" {
		return value
	}
	switch key {
	case ehrmsLeaveDetailFieldEmployeeNo:
		return strings.TrimSpace(record["emp_id"])
	case ehrmsLeaveDetailFieldDate:
		return strings.TrimSpace(record["date"])
	case ehrmsLeaveDetailFieldLeaveType:
		return strings.TrimSpace(record["leave_type"])
	case ehrmsLeaveDetailFieldStart:
		return strings.TrimSpace(record["start"])
	case ehrmsLeaveDetailFieldEnd:
		return strings.TrimSpace(record["end"])
	case ehrmsLeaveDetailFieldHours:
		return strings.TrimSpace(record["hours"])
	default:
		return ""
	}
}

func ehrmsStableID(prefix string, parts ...string) string {
	h := sha1.Sum([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("%s-%x", strings.TrimSpace(prefix), h[:10])
}
