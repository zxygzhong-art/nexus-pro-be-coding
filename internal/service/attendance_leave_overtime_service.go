package service

import (
	"sort"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
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
	if startErr := workflow.startTemporalFormApprovalWorkflow(ctx, instance); startErr != nil {
		return FormInstance{}, workflow.compensateFormApprovalWorkflowStartFailure(ctx, instance, startErr)
	}
	return instance, nil
}

// reserveLeaveBalanceIfAvailable degrades a missing or insufficient balance to a no-balance request.
func (c AttendanceService) reserveLeaveBalanceIfAvailable(ctx RequestContext, employeeID, leaveType string, hours float64, asOf time.Time) (LeaveBalance, string, error) {
	balance, reserved, found, err := c.store.ReserveLeaveBalance(goContext(ctx), ctx.TenantID, employeeID, leaveType, hours, asOf, c.Now())
	if err != nil {
		return LeaveBalance{}, "", err
	}
	if !found {
		return LeaveBalance{}, leaveEvaluationBalanceMissing, nil
	}
	if !reserved {
		return balance, leaveEvaluationBalanceInsufficient, nil
	}
	return balance, "", nil
}

// releaseLeaveBalance restores one exact balance bucket, resolving legacy requests by their start date.
func (c AttendanceService) releaseLeaveBalance(ctx RequestContext, balanceID, employeeID, leaveType string, hours float64, asOf time.Time) (LeaveBalance, error) {
	resolvedID := strings.TrimSpace(balanceID)
	if resolvedID == "" {
		balances, err := c.store.ListLeaveBalances(goContext(ctx), ctx.TenantID)
		if err != nil {
			return LeaveBalance{}, err
		}
		matches := make([]LeaveBalance, 0, 1)
		for _, balance := range balances {
			if balance.EmployeeID == employeeID && strings.EqualFold(strings.TrimSpace(balance.LeaveType), strings.TrimSpace(leaveType)) && leaveBalanceCoversDate(balance, asOf) {
				matches = append(matches, balance)
			}
		}
		switch len(matches) {
		case 0:
			return LeaveBalance{}, Conflict("legacy leave request has no balance covering its start date")
		case 1:
			resolvedID = strings.TrimSpace(matches[0].ID)
		default:
			return LeaveBalance{}, Conflict("legacy leave request matches multiple balances for its start date")
		}
		if resolvedID == "" {
			return LeaveBalance{}, Conflict("legacy leave request resolved to an invalid balance")
		}
	}
	balance, found, err := c.store.ReleaseLeaveBalanceByID(goContext(ctx), ctx.TenantID, resolvedID, hours, c.Now())
	if err != nil {
		return LeaveBalance{}, err
	}
	if !found {
		return LeaveBalance{}, Conflict("linked leave balance was not found")
	}
	return balance, nil
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
	previousStatus := normalizeLeaveRequestStatus(request.Status)
	nextStatus := leaveRequestStatusForWorkflow(kind, status)
	if nextStatus == "" || previousStatus == nextStatus {
		return nil
	}
	if previousStatus == "approved" && nextStatus != "approved" {
		return BadRequest("approved leave request cannot be changed by workflow")
	}
	if leaveRequestStatusReleasesBalance(previousStatus, nextStatus) {
		requiresBalance := strings.TrimSpace(request.LeaveBalanceID) != ""
		if snapshotValue, ok := request.EvaluationSnapshot["balance_required"].(bool); ok {
			requiresBalance = snapshotValue
		}
		if len(request.EvaluationSnapshot) == 0 && len(request.RuleSnapshot) == 0 {
			policy, err := c.loadAttendancePolicyResponse(ctx)
			if err != nil {
				return err
			}
			if leaveType, ok := findLeaveTypeInPolicy(policy, request.LeaveType); ok {
				requiresBalance = leaveType.RequiresBalance
			}
		}
		if requiresBalance {
			if _, err := c.releaseLeaveBalance(ctx, request.LeaveBalanceID, request.EmployeeID, request.LeaveType, request.Hours, request.StartAt); err != nil {
				return err
			}
		}
	}
	request.Status = nextStatus
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

// leaveRequestStatusReleasesBalance 處理請假請求狀態 releases balance。
func leaveRequestStatusReleasesBalance(previousStatus string, nextStatus string) bool {
	switch previousStatus {
	case "rejected", "cancelled":
		return false
	}
	return nextStatus == "rejected" || nextStatus == "cancelled"
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
		query.EmployeeIDs = employeeIDsFromSet(allowed)
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
	current, ok, err := c.store.GetAttendanceCorrectionRequestByFormInstanceID(goContext(ctx), ctx.TenantID, instance.ID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if !strings.EqualFold(current.Status, correctionStatusPending) {
		return nil
	}
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
	current.Status = nextStatus
	current.ReviewedByAccountID = strings.TrimSpace(ctx.AccountID)
	current.ReviewedAt = &now
	current.UpdatedAt = now
	if nextStatus == correctionStatusApproved {
		if err := c.applyApprovedAttendanceCorrection(ctx, &current, strings.TrimSpace(ctx.AccountID), current.ReviewReason, now); err != nil {
			return err
		}
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
	previousStatus := normalizeLeaveRequestStatus(request.Status)
	nextStatus := leaveRequestStatusForWorkflow(kind, status)
	if nextStatus == "" || previousStatus == nextStatus {
		return nil
	}
	if previousStatus == "approved" && nextStatus != "approved" {
		return BadRequest("approved overtime request cannot be changed by workflow")
	}
	if nextStatus == "approved" && strings.EqualFold(request.CompensationType, overtimeCompensationLeave) {
		if err := c.creditCompensatoryLeaveBalance(ctx, request.EmployeeID, request.Hours); err != nil {
			return err
		}
	}
	request.Status = nextStatus
	request.UpdatedAt = c.Now()
	return c.store.UpsertOvertimeRequest(goContext(ctx), request)
}

// creditCompensatoryLeaveBalance 依核准加班時數累積補休假餘額。
func (c AttendanceService) creditCompensatoryLeaveBalance(ctx RequestContext, employeeID string, hours float64) error {
	if hours <= 0 {
		return nil
	}
	policy, err := c.loadAttendancePolicyResponse(ctx)
	if err != nil {
		return err
	}
	leaveType := compensatoryLeaveTypeCode(policy)
	if _, found, err := c.store.ReleaseLeaveBalance(goContext(ctx), ctx.TenantID, employeeID, leaveType, hours, c.Now()); err != nil {
		return err
	} else if found {
		return nil
	}
	return c.store.UpsertLeaveBalance(goContext(ctx), LeaveBalance{
		ID:             utils.NewID("lb"),
		TenantID:       ctx.TenantID,
		EmployeeID:     employeeID,
		LeaveType:      leaveType,
		RemainingHours: hours,
		GrantedHours:   hours,
		Source:         "overtime",
		UpdatedAt:      c.Now(),
	})
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
