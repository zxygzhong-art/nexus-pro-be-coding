package service

import (
	"strings"
	"time"

	"nexus-pro-be/internal/utils"
)

var leaveLinkedTemplateKeys = map[string]struct{}{
	"leave-request": {},
	"field-leave":   {},
}

// createLeaveRequestFromSubmittedForm links leave data inside the caller's submission transaction.
func (c AttendanceService) createLeaveRequestFromSubmittedForm(ctx RequestContext, instance FormInstance, templateKey string, payload map[string]any) (LeaveRequest, error) {
	if _, ok := leaveLinkedTemplateKeys[strings.TrimSpace(templateKey)]; !ok {
		return LeaveRequest{}, nil
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if !workflowLeavePayloadHasLinkedFields(payload) {
		return LeaveRequest{}, nil
	}
	if existingID := strings.TrimSpace(stringFromAny(payload["leave_request_id"])); existingID != "" {
		if existing, ok, err := c.store.GetLeaveRequest(goContext(ctx), ctx.TenantID, existingID); err != nil {
			return LeaveRequest{}, err
		} else if ok {
			return existing, nil
		}
	}
	if existing, ok, err := c.store.GetLeaveRequestByFormInstanceID(goContext(ctx), ctx.TenantID, instance.ID); err != nil {
		return LeaveRequest{}, err
	} else if ok {
		return existing, nil
	}

	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return LeaveRequest{}, err
	}
	employeeID := strings.TrimSpace(stringFromAny(payload["employee_id"]))
	if employeeID == "" {
		employeeID = account.EmployeeID
	}
	if employeeID == "" {
		return LeaveRequest{}, BadRequest("employee_id is required")
	}

	leaveTypeRaw := utils.FirstNonEmpty(
		stringFromAny(payload["leave_type"]),
		stringFromAny(payload["leaveType"]),
		stringFromAny(payload["leave_name"]),
		stringFromAny(payload["leaveName"]),
	)
	if leaveTypeRaw == "" {
		return LeaveRequest{}, BadRequest("leave_type is required")
	}
	startRaw := utils.FirstNonEmpty(stringFromAny(payload["start_at"]), stringFromAny(payload["startAt"]))
	endRaw := utils.FirstNonEmpty(stringFromAny(payload["end_at"]), stringFromAny(payload["endAt"]))
	if startRaw == "" || endRaw == "" {
		return LeaveRequest{}, BadRequest("start_at and end_at are required")
	}
	startAt, err := utils.ParseDateTime(startRaw)
	if err != nil {
		return LeaveRequest{}, BadRequest("start_at must be RFC3339 or YYYY-MM-DD")
	}
	endAt, err := utils.ParseDateTime(endRaw)
	if err != nil {
		return LeaveRequest{}, BadRequest("end_at must be RFC3339 or YYYY-MM-DD")
	}
	if !endAt.After(startAt) {
		return LeaveRequest{}, BadRequest("end_at must be after start_at")
	}

	policy, err := c.loadAttendancePolicyResponse(ctx)
	if err != nil {
		return LeaveRequest{}, err
	}
	hours := calculateLeaveHoursWithinPolicy(startAt, endAt, policy.WorkTime)
	if hours <= 0 {
		return LeaveRequest{}, BadRequest("selected time does not include working hours")
	}
	leaveTypeCode := normalizeLeaveTypeCode(leaveTypeRaw)
	leaveType, ok := findLeaveTypeInPolicy(policy, leaveTypeCode)
	if !ok || !leaveType.Active {
		return LeaveRequest{}, BadRequest("unknown leave type")
	}

	reason := utils.FirstNonEmpty(stringFromAny(payload["reason"]), stringFromAny(payload["description"]))
	requestID := utils.NewID("lr")
	leaveBalanceID := ""
	if leaveType.RequiresBalance {
		balance, err := c.reserveLeaveBalance(ctx, employeeID, leaveTypeCode, hours, startAt)
		if err != nil {
			return LeaveRequest{}, err
		}
		leaveBalanceID = balance.ID
	}
	req := LeaveRequest{
		ID:             requestID,
		TenantID:       ctx.TenantID,
		EmployeeID:     employeeID,
		LeaveType:      leaveTypeCode,
		StartAt:        startAt,
		EndAt:          endAt,
		Hours:          hours,
		Reason:         strings.TrimSpace(reason),
		Status:         "pending_approval",
		FormInstanceID: instance.ID,
		LeaveBalanceID: leaveBalanceID,
		CreatedAt:      c.Now(),
	}
	if err := c.store.UpsertLeaveRequest(goContext(ctx), req); err != nil {
		return LeaveRequest{}, err
	}

	nextPayload := utils.CopyStringMap(instance.Payload)
	if nextPayload == nil {
		nextPayload = map[string]any{}
	}
	nextPayload["employee_id"] = employeeID
	nextPayload["leave_request_id"] = requestID
	nextPayload["leave_type"] = leaveTypeCode
	nextPayload["linked_resource_id"] = requestID
	nextPayload["linked_resource_type"] = "attendance.leave_request"
	nextPayload["start_at"] = startAt.Format(time.RFC3339)
	nextPayload["end_at"] = endAt.Format(time.RFC3339)
	nextPayload["hours"] = hours
	nextPayload["reason"] = strings.TrimSpace(reason)
	instance.Payload = nextPayload
	instance.UpdatedAt = c.Now()
	if err := c.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
		return LeaveRequest{}, err
	}
	if leaveType.RequiresBalance {
		if err := c.audit(ctx, "attendance.leave_balance.reserve", "leave_balance", employeeID+"|"+leaveTypeCode, "medium", map[string]any{
			"employee_id":    employeeID,
			"leave_type":     leaveTypeCode,
			"reserved_hours": hours,
		}); err != nil {
			return LeaveRequest{}, err
		}
	}
	if err := c.audit(ctx, "attendance.leave_request.create", "leave_request", req.ID, "medium", map[string]any{
		"leave_type":       req.LeaveType,
		"hours":            req.Hours,
		"form_instance_id": instance.ID,
		"source":           "workflow.form.submit",
	}); err != nil {
		return LeaveRequest{}, err
	}
	return req, nil
}

// workflowLeavePayloadHasLinkedFields avoids treating generic workflow payloads as attendance leave requests.
func workflowLeavePayloadHasLinkedFields(payload map[string]any) bool {
	for _, key := range []string{
		"leave_request_id", "leaveRequestId",
		"employee_id", "employeeId",
		"leave_type", "leaveType", "leave_name", "leaveName",
		"hours", "leave_hours", "leaveHours", "hours_requested", "hoursRequested",
		"start_at", "startAt", "end_at", "endAt",
	} {
		if strings.TrimSpace(stringFromAny(payload[key])) != "" {
			return true
		}
	}
	return false
}
