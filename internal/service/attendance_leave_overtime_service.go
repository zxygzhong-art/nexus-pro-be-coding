package service

import (
	"sort"
	"strings"
	"time"

	"nexus-pro-be/internal/utils"
)

// CreateLeaveRequest 建立請假請求的服務流程。
func (c AttendanceService) CreateLeaveRequest(ctx RequestContext, input CreateLeaveRequestInput) (LeaveRequest, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return LeaveRequest{}, err
	}
	employeeID := strings.TrimSpace(input.EmployeeID)
	if employeeID == "" {
		employeeID = account.EmployeeID
	}
	if employeeID == "" {
		return LeaveRequest{}, BadRequest("employee_id is required")
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{
		ApplicationCode:  AppAttendance,
		ResourceType:     ResourceLeave,
		ResourceID:       employeeID,
		Target:           employeeID,
		TargetEmployeeID: employeeID,
		Action:           ActionCreate,
	})
	if err != nil {
		return LeaveRequest{}, err
	}
	if !decision.Allowed {
		return LeaveRequest{}, Forbidden(decision.Reason)
	}
	allowed, all, err := c.attendanceEmployeeScope(ctx, account, decision)
	if err != nil {
		return LeaveRequest{}, err
	}
	if !all {
		if _, ok := allowed[employeeID]; !ok {
			return LeaveRequest{}, Forbidden("employee is outside data scope")
		}
	}
	if _, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, employeeID); err != nil {
		return LeaveRequest{}, err
	} else if !ok {
		return LeaveRequest{}, NotFound("employee", employeeID)
	}
	if strings.TrimSpace(input.LeaveType) == "" {
		return LeaveRequest{}, BadRequest("leave_type is required")
	}
	if input.Hours <= 0 {
		return LeaveRequest{}, BadRequest("hours must be greater than zero")
	}
	policy, err := c.loadAttendancePolicyResponse(ctx)
	if err != nil {
		return LeaveRequest{}, err
	}
	leaveTypeCode := normalizeLeaveTypeCode(input.LeaveType)
	leaveType, ok := findLeaveTypeInPolicy(policy, leaveTypeCode)
	if !ok || !leaveType.Active {
		return LeaveRequest{}, BadRequest("unknown leave type")
	}
	startAt, err := utils.ParseDateTime(input.StartAt)
	if err != nil {
		return LeaveRequest{}, BadRequest("start_at must be RFC3339 or YYYY-MM-DD")
	}
	endAt, err := utils.ParseDateTime(input.EndAt)
	if err != nil {
		return LeaveRequest{}, BadRequest("end_at must be RFC3339 or YYYY-MM-DD")
	}
	if !endAt.After(startAt) {
		return LeaveRequest{}, BadRequest("end_at must be after start_at")
	}
	var req LeaveRequest
	requestID := utils.NewID("lr")
	if err := c.withTransaction(ctx, func(tx AttendanceService) error {
		if leaveType.RequiresBalance {
			if _, err := tx.reserveLeaveBalance(ctx, employeeID, leaveTypeCode, input.Hours); err != nil {
				return err
			}
		}
		template, ok, err := tx.store.GetFormTemplateByKey(goContext(ctx), ctx.TenantID, "leave-request")
		if err != nil {
			return err
		}
		if !ok {
			template = FormTemplate{
				ID:        utils.NewID("ft"),
				TenantID:  ctx.TenantID,
				Key:       "leave-request",
				Name:      "請假申請單",
				Schema:    map[string]any{"type": "object"},
				CreatedAt: tx.Now(),
			}
			if err := tx.store.UpsertFormTemplate(goContext(ctx), template); err != nil {
				return err
			}
		}
		instance := FormInstance{
			ID:                 utils.NewID("fi"),
			TenantID:           ctx.TenantID,
			TemplateID:         template.ID,
			ApplicantAccountID: account.ID,
			Status:             "submitted",
			Payload: map[string]any{
				"employee_id":          employeeID,
				"leave_request_id":     requestID,
				"leave_type":           leaveTypeCode,
				"linked_resource_id":   requestID,
				"linked_resource_type": "attendance.leave_request",
				"start_at":             startAt.Format(time.RFC3339),
				"end_at":               endAt.Format(time.RFC3339),
				"hours":                input.Hours,
				"reason":               input.Reason,
			},
			SubmittedAt: tx.Now(),
			UpdatedAt:   tx.Now(),
		}
		if err := tx.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
			return err
		}
		req = LeaveRequest{
			ID:             requestID,
			TenantID:       ctx.TenantID,
			EmployeeID:     employeeID,
			LeaveType:      leaveTypeCode,
			StartAt:        startAt,
			EndAt:          endAt,
			Hours:          input.Hours,
			Reason:         strings.TrimSpace(input.Reason),
			Status:         "pending_approval",
			FormInstanceID: instance.ID,
			CreatedAt:      tx.Now(),
		}
		if err := tx.store.UpsertLeaveRequest(goContext(ctx), req); err != nil {
			return err
		}
		if leaveType.RequiresBalance {
			if err := tx.audit(ctx, "attendance.leave_balance.reserve", "leave_balance", employeeID+"|"+leaveTypeCode, "medium", map[string]any{
				"employee_id":    employeeID,
				"leave_type":     leaveTypeCode,
				"reserved_hours": input.Hours,
			}); err != nil {
				return err
			}
		}
		if err := tx.audit(ctx, "attendance.leave_request.create", "leave_request", req.ID, "medium", map[string]any{"leave_type": req.LeaveType, "hours": req.Hours}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return LeaveRequest{}, err
	}
	c.logInfo(ctx, "leave request created",
		"leave_request_id", req.ID,
		"employee_id", req.EmployeeID,
		"leave_type", req.LeaveType,
		"hours", req.Hours,
		"status", req.Status,
		"form_instance_id", req.FormInstanceID,
	)
	return req, nil
}

// reserveLeaveBalance 保留請假 balance 的服務流程。
func (c AttendanceService) reserveLeaveBalance(ctx RequestContext, employeeID, leaveType string, hours float64) (LeaveBalance, error) {
	balance, reserved, found, err := c.store.ReserveLeaveBalance(goContext(ctx), ctx.TenantID, employeeID, leaveType, hours, c.Now())
	if err != nil {
		return LeaveBalance{}, err
	}
	if !found {
		return LeaveBalance{}, BadRequest("leave balance is required for this leave type")
	}
	if !reserved {
		return LeaveBalance{}, BadRequest("leave balance is insufficient")
	}
	return balance, nil
}

// releaseLeaveBalance 釋放請假 balance 的服務流程。
func (c AttendanceService) releaseLeaveBalance(ctx RequestContext, employeeID, leaveType string, hours float64) (LeaveBalance, error) {
	balance, found, err := c.store.ReleaseLeaveBalance(goContext(ctx), ctx.TenantID, employeeID, leaveType, hours, c.Now())
	if err != nil {
		return LeaveBalance{}, err
	}
	if !found {
		return LeaveBalance{}, BadRequest("leave balance is required for this leave type")
	}
	return balance, nil
}

// applyLeaveWorkflowReview 處理 apply 請假流程審核的服務流程。
func (c AttendanceService) applyLeaveWorkflowReview(ctx RequestContext, instance FormInstance, kind string, status string) error {
	leaveRequestID := workflowLinkedLeaveRequestID(instance)
	var request LeaveRequest
	var ok bool
	var err error
	if leaveRequestID != "" {
		request, ok, err = c.store.GetLeaveRequest(goContext(ctx), ctx.TenantID, leaveRequestID)
		if err != nil {
			return err
		}
	}
	if leaveRequestID == "" || !ok {
		request, ok, err = c.store.GetLeaveRequestByFormInstanceID(goContext(ctx), ctx.TenantID, instance.ID)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
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
		policy, err := c.loadAttendancePolicyResponse(ctx)
		if err != nil {
			return err
		}
		if leaveType, ok := findLeaveTypeInPolicy(policy, request.LeaveType); ok && leaveType.RequiresBalance {
			if _, err := c.releaseLeaveBalance(ctx, request.EmployeeID, request.LeaveType, request.Hours); err != nil {
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

// CreateOvertimeRequest 建立加班申請的服務流程。
func (c AttendanceService) CreateOvertimeRequest(ctx RequestContext, input CreateOvertimeRequestInput) (OvertimeRequest, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return OvertimeRequest{}, err
	}
	employeeID := strings.TrimSpace(input.EmployeeID)
	if employeeID == "" {
		employeeID = account.EmployeeID
	}
	if employeeID == "" {
		return OvertimeRequest{}, BadRequest("employee_id is required")
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{
		ApplicationCode:  AppAttendance,
		ResourceType:     ResourceLeave,
		ResourceID:       employeeID,
		Target:           employeeID,
		TargetEmployeeID: employeeID,
		Action:           ActionCreate,
	})
	if err != nil {
		return OvertimeRequest{}, err
	}
	if !decision.Allowed {
		return OvertimeRequest{}, Forbidden(decision.Reason)
	}
	if err := c.ensureAttendanceEmployeeAllowed(ctx, account, decision, employeeID); err != nil {
		return OvertimeRequest{}, err
	}
	if _, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, employeeID); err != nil {
		return OvertimeRequest{}, err
	} else if !ok {
		return OvertimeRequest{}, NotFound("employee", employeeID)
	}
	if input.Hours <= 0 {
		return OvertimeRequest{}, BadRequest("hours must be greater than zero")
	}
	startAt, err := utils.ParseDateTime(input.StartAt)
	if err != nil {
		return OvertimeRequest{}, BadRequest("start_at must be RFC3339 or YYYY-MM-DD")
	}
	endAt, err := utils.ParseDateTime(input.EndAt)
	if err != nil {
		return OvertimeRequest{}, BadRequest("end_at must be RFC3339 or YYYY-MM-DD")
	}
	if !endAt.After(startAt) {
		return OvertimeRequest{}, BadRequest("end_at must be after start_at")
	}
	overtimeType, err := normalizeOvertimeType(input.OvertimeType)
	if err != nil {
		return OvertimeRequest{}, err
	}
	compensationType, err := normalizeOvertimeCompensationType(input.CompensationType)
	if err != nil {
		return OvertimeRequest{}, err
	}
	var req OvertimeRequest
	requestID := utils.NewID("ot")
	if err := c.withTransaction(ctx, func(tx AttendanceService) error {
		template, ok, err := tx.store.GetFormTemplateByKey(goContext(ctx), ctx.TenantID, "overtime-approval")
		if err != nil {
			return err
		}
		if !ok {
			template = FormTemplate{
				ID:        utils.NewID("ft"),
				TenantID:  ctx.TenantID,
				Key:       "overtime-approval",
				Name:      "加班核准申請單",
				Schema:    map[string]any{"type": "object"},
				CreatedAt: tx.Now(),
			}
			if err := tx.store.UpsertFormTemplate(goContext(ctx), template); err != nil {
				return err
			}
		}
		instance := FormInstance{
			ID:                 utils.NewID("fi"),
			TenantID:           ctx.TenantID,
			TemplateID:         template.ID,
			ApplicantAccountID: account.ID,
			Status:             "submitted",
			Payload: map[string]any{
				"employee_id":          employeeID,
				"overtime_request_id":  requestID,
				"linked_resource_id":   requestID,
				"linked_resource_type": "attendance.overtime_request",
				"start_at":             startAt.Format(time.RFC3339),
				"end_at":               endAt.Format(time.RFC3339),
				"hours":                input.Hours,
				"overtime_type":        overtimeType,
				"compensation_type":    compensationType,
				"reason":               input.Reason,
			},
			SubmittedAt: tx.Now(),
			UpdatedAt:   tx.Now(),
		}
		if err := tx.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
			return err
		}
		req = OvertimeRequest{
			ID:               requestID,
			TenantID:         ctx.TenantID,
			EmployeeID:       employeeID,
			WorkDate:         attendanceWorkDate(startAt),
			StartAt:          startAt,
			EndAt:            endAt,
			Hours:            input.Hours,
			OvertimeType:     overtimeType,
			CompensationType: compensationType,
			Reason:           strings.TrimSpace(input.Reason),
			Status:           "pending_approval",
			FormInstanceID:   instance.ID,
			CreatedAt:        tx.Now(),
			UpdatedAt:        tx.Now(),
		}
		if err := tx.store.UpsertOvertimeRequest(goContext(ctx), req); err != nil {
			return err
		}
		return tx.audit(ctx, "attendance.overtime_request.create", "overtime_request", req.ID, "medium", map[string]any{
			"employee_id":       employeeID,
			"hours":             req.Hours,
			"overtime_type":     req.OvertimeType,
			"compensation_type": req.CompensationType,
		})
	}); err != nil {
		return OvertimeRequest{}, err
	}
	c.logInfo(ctx, "overtime request created",
		"overtime_request_id", req.ID,
		"employee_id", req.EmployeeID,
		"hours", req.Hours,
		"status", req.Status,
		"form_instance_id", req.FormInstanceID,
	)
	return req, nil
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
	overtimeRequestID := workflowLinkedOvertimeRequestID(instance)
	var request OvertimeRequest
	var ok bool
	var err error
	if overtimeRequestID != "" {
		request, ok, err = c.store.GetOvertimeRequest(goContext(ctx), ctx.TenantID, overtimeRequestID)
		if err != nil {
			return err
		}
	}
	if overtimeRequestID == "" || !ok {
		request, ok, err = c.store.GetOvertimeRequestByFormInstanceID(goContext(ctx), ctx.TenantID, instance.ID)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
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
