package service

import (
	"fmt"
	"sort"
	"strings"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

const attendanceCompatibilityDefaultReason = "未填寫"

// CreateLeaveRequest delegates the compatibility endpoint to the canonical workflow submission runtime.
func (c AttendanceService) CreateLeaveRequest(ctx RequestContext, input CreateLeaveRequestInput) (LeaveRequest, error) {
	account, employeeID, err := c.authorizeLeaveRequestEmployee(ctx, input.EmployeeID)
	if err != nil {
		return LeaveRequest{}, err
	}
	if employeeID != strings.TrimSpace(account.EmployeeID) {
		return LeaveRequest{}, Forbidden("proxy attendance submission requires a dedicated authorization path").WithReasonCode("attendance_proxy_submission_not_supported")
	}
	if _, err := c.requireAttendanceEmployeeActive(ctx, employeeID); err != nil {
		return LeaveRequest{}, err
	}
	instance, err := c.submitAttendanceCompatibilityForm(ctx, account, "leave-request", map[string]any{
		"leave_type": input.LeaveType,
		"start_at":   input.StartAt,
		"end_at":     input.EndAt,
		"hours":      input.Hours,
		"reason":     attendanceCompatibilityReason(input.Reason),
	})
	if err != nil {
		return LeaveRequest{}, err
	}
	request, ok, err := c.store.GetLeaveRequestByFormInstanceID(goContext(ctx), ctx.TenantID, instance.ID)
	if err != nil {
		return LeaveRequest{}, err
	}
	if !ok {
		return LeaveRequest{}, domain.E(500, "attendance_projection_missing", "workflow submission did not create a leave request")
	}
	return request, nil
}

// submitAttendanceCompatibilityForm preserves the old API shape while using the canonical workflow engine.
func (c AttendanceService) submitAttendanceCompatibilityForm(ctx RequestContext, account Account, templateKey string, payload map[string]any) (FormInstance, error) {
	workflow := c.Service.Workflow()
	instance, err := workflow.submitNewFormForApplicant(ctx, account, templateKey, "", payload)
	if err != nil {
		return FormInstance{}, err
	}
	if workflow.workflowStartOutboxEnabled {
		return instance, nil
	}
	if startErr := workflow.startTemporalFormApprovalWorkflow(ctx, instance); startErr != nil {
		return FormInstance{}, workflow.compensateFormApprovalWorkflowStartFailure(ctx, instance, startErr)
	}
	return instance, nil
}

// applyLeaveWorkflowReview 處理 apply 請假流程審核的服務流程。
func (c AttendanceService) applyLeaveWorkflowReview(ctx RequestContext, instance FormInstance, kind string, status string) error {
	request, ok, err := c.store.GetLeaveRequestByFormInstanceID(goContext(ctx), ctx.TenantID, instance.ID)
	if err != nil {
		return err
	}
	if !ok {
		leaveRequestID := workflowLinkedLeaveRequestID(instance)
		if leaveRequestID != "" {
			candidate, found, lookupErr := c.store.GetLeaveRequest(goContext(ctx), ctx.TenantID, leaveRequestID)
			if lookupErr != nil {
				return lookupErr
			}
			if found && strings.TrimSpace(candidate.FormInstanceID) == strings.TrimSpace(instance.ID) {
				request, ok = candidate, true
			}
		}
		if !ok {
			return nil
		}
	}
	applicantEmployeeID, err := c.workflowApplicantEmployeeID(ctx, instance)
	if err != nil {
		return err
	}
	if strings.TrimSpace(request.EmployeeID) != applicantEmployeeID {
		return Conflict("linked leave request does not belong to the form applicant")
	}
	nextStatus := leaveRequestStatusForWorkflow(kind, status)
	if nextStatus == "" {
		return nil
	}
	if nextStatus == "approved" && request.EffectStatus == "applied" {
		return nil
	}
	if (nextStatus == "rejected" || nextStatus == "cancelled") && request.EffectStatus == "compensated" {
		return nil
	}
	leaveTypeID := strings.TrimSpace(request.LeaveTypeID)
	if leaveTypeID == "" {
		leaveTypeID = domain.StableLeaveTypeID(request.LeaveType)
		request.LeaveTypeID = leaveTypeID
	}
	cycle := leaveRequestBalanceCycle(request)
	leaveRecord, found, err := c.store.GetLeaveRecord(goContext(ctx), ctx.TenantID, request.ID)
	if err != nil {
		return err
	}
	if !found {
		return Conflict("leave request has no annual leave record")
	}
	reserved, _ := request.EvaluationSnapshot["balance_required"].(bool)
	if nextStatus == "approved" || nextStatus == "rejected" || nextStatus == "cancelled" {
		if reserved {
			if err := c.appendLeaveBalanceEntry(ctx, request, leaveRecord, leaveBalanceEntryRelease, request.RequestedMinutes, cycle); err != nil {
				return err
			}
		}
	}
	if nextStatus == "approved" {
		if reserved {
			if err := c.appendLeaveBalanceEntry(ctx, request, leaveRecord, leaveBalanceEntryLocalConsume, -request.RequestedMinutes, cycle); err != nil {
				return err
			}
		}
		leaveRecord.Status = "active"
		request.ReconciliationStatus = "nexus_only"
	} else {
		leaveRecord.Status = "cancelled"
	}
	leaveRecord.UpdatedAt = c.Now()
	if err := c.store.UpsertLeaveRecord(goContext(ctx), leaveRecord); err != nil {
		return err
	}
	request.Status = nextStatus
	request.UpdatedAt = c.Now()
	request.EffectResult = utils.CopyStringMap(request.EffectResult)
	if request.EffectResult == nil {
		request.EffectResult = map[string]any{}
	}
	request.EffectResult["workflow_action"] = strings.ToLower(strings.TrimSpace(kind))
	request.EffectResult["balance_cycle"] = cycle
	if nextStatus == "approved" {
		request.EffectStatus = "applied"
		request.EffectAppliedAt = &request.UpdatedAt
		request.EffectResult["leave_record_applied"] = true
	} else {
		request.EffectStatus = "compensated"
		request.EffectAppliedAt = nil
		request.EffectResult["reservation_released"] = true
	}
	return c.store.UpsertLeaveRequest(goContext(ctx), request)
}

// filterLeaveBalancesByEmployees 處理篩選請假 balances by 員工。
func filterLeaveBalancesByEmployees(items []LeaveBalance, allowed map[string]struct{}) []LeaveBalance {
	out := make([]LeaveBalance, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.EmployeeID]; ok {
			out = append(out, item)
		}
	}
	return out
}

// normalizeLeaveRequestQuery 正規化請假請求查詢。
func normalizeLeaveRequestQuery(query LeaveRequestQuery) LeaveRequestQuery {
	query.Status = strings.TrimSpace(strings.ToLower(query.Status))
	query.FromDate = strings.TrimSpace(query.FromDate)
	query.ToDate = strings.TrimSpace(query.ToDate)
	query.EmployeeIDs = employeeIDsFromSlice(query.EmployeeIDs)
	return query
}

// employeeIDsFromSet 處理員工 IDs 來源 集合。
func employeeIDsFromSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for id := range values {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	sort.Strings(out)
	return out
}

// employeeIDsFromSlice 處理員工 IDs 來源 slice。
func employeeIDsFromSlice(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

// intersectEmployeeIDs preserves an explicit employee filter without widening the authorized scope.
func intersectEmployeeIDs(requested []string, allowed map[string]struct{}) []string {
	requested = employeeIDsFromSlice(requested)
	if len(requested) == 0 {
		return employeeIDsFromSet(allowed)
	}
	out := make([]string, 0, len(requested))
	for _, employeeID := range requested {
		if _, ok := allowed[employeeID]; ok {
			out = append(out, employeeID)
		}
	}
	return out
}

// workflowLinkedLeaveRequestID 處理流程 linked 請假請求 ID。
func workflowLinkedLeaveRequestID(instance FormInstance) string {
	if !strings.EqualFold(stringFromAny(instance.Payload["linked_resource_type"]), "attendance.leave_request") {
		if stringFromAny(instance.Payload["leave_request_id"]) == "" {
			return ""
		}
	}
	if id := strings.TrimSpace(stringFromAny(instance.Payload["leave_request_id"])); id != "" {
		return id
	}
	return strings.TrimSpace(stringFromAny(instance.Payload["linked_resource_id"]))
}

// leaveRequestStatusForWorkflow 處理請假請求狀態 for 流程。
func leaveRequestStatusForWorkflow(kind string, status string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "approve":
		return "approved"
	case "reject", "return":
		return "rejected"
	case "cancel":
		return "cancelled"
	}
	return normalizeLeaveRequestStatus(status)
}

// normalizeLeaveRequestStatus 正規化請假請求狀態。
func normalizeLeaveRequestStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "approved":
		return "approved"
	case "rejected", "reject":
		return "rejected"
	case "cancelled", "canceled", "cancel":
		return "cancelled"
	case "submitted", "pending", "pending_approval", "":
		return "pending_approval"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

// CreateOvertimeRequest delegates the compatibility endpoint to the canonical workflow submission runtime.
func (c AttendanceService) CreateOvertimeRequest(ctx RequestContext, input CreateOvertimeRequestInput) (OvertimeRequest, error) {
	account, employeeID, err := c.authorizeLeaveRequestEmployee(ctx, input.EmployeeID)
	if err != nil {
		return OvertimeRequest{}, err
	}
	if employeeID != strings.TrimSpace(account.EmployeeID) {
		return OvertimeRequest{}, Forbidden("proxy attendance submission requires a dedicated authorization path").WithReasonCode("attendance_proxy_submission_not_supported")
	}
	if _, err := c.requireAttendanceEmployeeActive(ctx, employeeID); err != nil {
		return OvertimeRequest{}, err
	}
	overtimeType, err := normalizeOvertimeType(input.OvertimeType)
	if err != nil {
		return OvertimeRequest{}, err
	}
	compensationType, err := normalizeOvertimeCompensationType(input.CompensationType)
	if err != nil {
		return OvertimeRequest{}, err
	}
	instance, err := c.submitAttendanceCompatibilityForm(ctx, account, "overtime-approval", map[string]any{
		"start_at":          input.StartAt,
		"end_at":            input.EndAt,
		"hours":             input.Hours,
		"overtime_type":     overtimeType,
		"compensation_type": compensationType,
		"reason":            attendanceCompatibilityReason(input.Reason),
	})
	if err != nil {
		return OvertimeRequest{}, err
	}
	request, ok, err := c.store.GetOvertimeRequestByFormInstanceID(goContext(ctx), ctx.TenantID, instance.ID)
	if err != nil {
		return OvertimeRequest{}, err
	}
	if !ok {
		return OvertimeRequest{}, domain.E(500, "attendance_projection_missing", "workflow submission did not create an overtime request")
	}
	return request, nil
}

// attendanceCompatibilityReason keeps optional legacy API requests valid against required form fields.
func attendanceCompatibilityReason(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return attendanceCompatibilityDefaultReason
}

// listOvertimeRequestsByQuery 列出加班申請 by 查詢的服務流程。
func (c AttendanceService) listOvertimeRequestsByQuery(ctx RequestContext, query OvertimeRequestQuery) ([]OvertimeRequest, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppAttendance, ResourceType: ResourceLeave, Action: ActionRead})
	if err != nil {
		return nil, err
	}
	if !decision.Allowed {
		return nil, forbiddenAuthz(decision)
	}
	allowed, all, err := c.attendanceEmployeeScope(ctx, account, decision)
	if err != nil {
		return nil, err
	}
	if !all {
		query.EmployeeIDs = intersectEmployeeIDs(query.EmployeeIDs, allowed)
		if len(query.EmployeeIDs) == 0 {
			return []OvertimeRequest{}, nil
		}
	}
	return c.store.ListOvertimeRequestsByQuery(goContext(ctx), ctx.TenantID, normalizeOvertimeRequestQuery(query))
}

// ListOvertimeRequestPage 列出加班申請分頁的服務流程。
func (c AttendanceService) ListOvertimeRequestPage(ctx RequestContext, page PageRequest) (PageResponse[OvertimeRequest], error) {
	items, err := c.listOvertimeRequestsByQuery(ctx, OvertimeRequestQuery{})
	if err != nil {
		return PageResponse[OvertimeRequest]{}, err
	}
	return utils.PageResponse(items, page), nil
}

// normalizeOvertimeRequestQuery 正規化加班申請查詢。
func normalizeOvertimeRequestQuery(query OvertimeRequestQuery) OvertimeRequestQuery {
	query.Status = strings.TrimSpace(strings.ToLower(query.Status))
	query.FromDate = strings.TrimSpace(query.FromDate)
	query.ToDate = strings.TrimSpace(query.ToDate)
	query.EmployeeIDs = employeeIDsFromSlice(query.EmployeeIDs)
	return query
}

// normalizeOvertimeType 正規化加班類型。
func normalizeOvertimeType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", overtimeTypeWeekday:
		return overtimeTypeWeekday, nil
	case overtimeTypeHoliday, "weekend":
		return overtimeTypeHoliday, nil
	default:
		return "", BadRequest("overtime_type must be weekday or holiday")
	}
}

// normalizeOvertimeCompensationType 正規化加班補償類型。
func normalizeOvertimeCompensationType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", overtimeCompensationLeave:
		return overtimeCompensationLeave, nil
	case overtimeCompensationPay:
		return overtimeCompensationPay, nil
	default:
		return "", BadRequest("compensation_type must be leave or pay")
	}
}

// applyAttendanceWorkflowReview 處理考勤相關表單流程審核的統一入口。
func (c AttendanceService) applyAttendanceWorkflowReview(ctx RequestContext, instance FormInstance, kind string, status string) error {
	if err := c.applyLeaveWorkflowReview(ctx, instance, kind, status); err != nil {
		return err
	}
	if err := c.applyCorrectionWorkflowReview(ctx, instance, kind); err != nil {
		return err
	}
	return c.applyOvertimeWorkflowReview(ctx, instance, kind, status)
}

// applyCorrectionWorkflowReview 處理補卡表單流程審核的服務流程。
func (c AttendanceService) applyCorrectionWorkflowReview(ctx RequestContext, instance FormInstance, kind string) error {
	nextStatus := ""
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "approve":
		nextStatus = correctionStatusApproved
	case "reject", "return", "cancel":
		nextStatus = correctionStatusRejected
	default:
		return nil
	}
	now := c.Now()
	current, claimed, err := c.store.ClaimAttendanceCorrectionReview(
		goContext(ctx), ctx.TenantID, instance.ID, strings.TrimSpace(ctx.AccountID), now,
	)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	current.Status = nextStatus
	current.ReviewedByAccountID = strings.TrimSpace(ctx.AccountID)
	current.ReviewedAt = &now
	current.UpdatedAt = now
	if nextStatus == correctionStatusApproved {
		if err := c.applyApprovedAttendanceCorrection(ctx, &current, strings.TrimSpace(ctx.AccountID), current.ReviewReason, now); err != nil {
			return err
		}
		current.EffectStatus = "applied"
		current.EffectAppliedAt = &now
	} else {
		current.EffectStatus = "compensated"
		current.EffectAppliedAt = nil
	}
	if err := c.store.UpsertAttendanceCorrectionRequest(goContext(ctx), current); err != nil {
		return err
	}
	event := "attendance.correction.reject"
	if nextStatus == correctionStatusApproved {
		event = "attendance.correction.approve"
	}
	return c.audit(ctx, event, string(ResourceAttendanceCorrection), current.ID, string(SeverityHigh), map[string]any{
		"employee_id": current.EmployeeID,
		"direction":   current.Direction,
		"status":      current.Status,
		"via":         "workflow",
	})
}

// applyOvertimeWorkflowReview 處理加班表單流程審核的服務流程。
func (c AttendanceService) applyOvertimeWorkflowReview(ctx RequestContext, instance FormInstance, kind string, status string) error {
	request, ok, err := c.store.GetOvertimeRequestByFormInstanceID(goContext(ctx), ctx.TenantID, instance.ID)
	if err != nil {
		return err
	}
	if !ok {
		overtimeRequestID := workflowLinkedOvertimeRequestID(instance)
		if overtimeRequestID != "" {
			candidate, found, lookupErr := c.store.GetOvertimeRequest(goContext(ctx), ctx.TenantID, overtimeRequestID)
			if lookupErr != nil {
				return lookupErr
			}
			if found && strings.TrimSpace(candidate.FormInstanceID) == strings.TrimSpace(instance.ID) {
				request, ok = candidate, true
			}
		}
		if !ok {
			return nil
		}
	}
	applicantEmployeeID, err := c.workflowApplicantEmployeeID(ctx, instance)
	if err != nil {
		return err
	}
	if strings.TrimSpace(request.EmployeeID) != applicantEmployeeID {
		return Conflict("linked overtime request does not belong to the form applicant")
	}
	nextStatus := leaveRequestStatusForWorkflow(kind, status)
	if nextStatus == "" {
		return nil
	}
	if nextStatus == "approved" && request.EffectStatus == "applied" {
		return nil
	}
	if (nextStatus == "rejected" || nextStatus == "cancelled") && request.EffectStatus == "compensated" {
		return nil
	}
	if nextStatus == "approved" && strings.EqualFold(request.CompensationType, overtimeCompensationLeave) {
		if err := c.creditCompensatoryLeaveBalance(ctx, request); err != nil {
			return err
		}
	}
	request.Status = nextStatus
	request.UpdatedAt = c.Now()
	request.EffectResult = utils.CopyStringMap(request.EffectResult)
	if request.EffectResult == nil {
		request.EffectResult = map[string]any{}
	}
	request.EffectResult["workflow_action"] = strings.ToLower(strings.TrimSpace(kind))
	if nextStatus == "approved" {
		request.EffectStatus = "applied"
		request.EffectAppliedAt = &request.UpdatedAt
		request.EffectResult["overtime_applied"] = true
	} else {
		request.EffectStatus = "compensated"
		request.EffectAppliedAt = nil
	}
	return c.store.UpsertOvertimeRequest(goContext(ctx), request)
}

// creditCompensatoryLeaveBalance writes one idempotent credit bound to the
// approved overtime request. The deterministic annual Nexus balance carries no
// snapshot amount of its own.
func (c AttendanceService) creditCompensatoryLeaveBalance(ctx RequestContext, request OvertimeRequest) error {
	minutes := leaveMinutes(request.Hours)
	if minutes <= 0 {
		return nil
	}
	leaveType := leaveTypeCodeCompensatory
	leaveTypeID := domain.StableLeaveTypeID(leaveType)
	now := c.Now()
	year := request.StartAt.In(attendanceClockLocation).Year()
	anchor, err := c.store.EnsureLocalLeaveBalanceAnchor(goContext(ctx), LeaveBalance{
		ID:       ehrmsStableID("lb-nexus", ctx.TenantID, request.EmployeeID, leaveTypeID, fmt.Sprint(year)),
		TenantID: ctx.TenantID, EmployeeID: request.EmployeeID,
		LeaveType: leaveType, LeaveTypeID: leaveTypeID, EntitlementYear: year, Source: "nexus", UpdatedAt: now,
	})
	if err != nil {
		return err
	}
	_, err = c.store.AppendStandaloneLeaveBalanceEntry(goContext(ctx), domain.LeaveBalanceEntry{
		ID: utils.NewID("lbe"), TenantID: ctx.TenantID,
		EmployeeID: request.EmployeeID, LeaveTypeID: leaveTypeID, BalanceID: anchor.ID,
		EntitlementYear: year, EntryType: leaveBalanceEntryOvertimeCredit,
		AmountMinutes: minutes, IdempotencyKey: "overtime-request:" + request.ID + ":credit",
		OccurredAt: now, CreatedAt: now,
	})
	return err
}

// workflowLinkedOvertimeRequestID 處理流程 linked 加班申請 ID。
func workflowLinkedOvertimeRequestID(instance FormInstance) string {
	if !strings.EqualFold(stringFromAny(instance.Payload["linked_resource_type"]), "attendance.overtime_request") {
		if stringFromAny(instance.Payload["overtime_request_id"]) == "" {
			return ""
		}
	}
	if id := strings.TrimSpace(stringFromAny(instance.Payload["overtime_request_id"])); id != "" {
		return id
	}
	return strings.TrimSpace(stringFromAny(instance.Payload["linked_resource_id"]))
}
