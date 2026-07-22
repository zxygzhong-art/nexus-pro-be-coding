package service

import (
	"strings"
	"time"

	"nexus-pro-api/internal/utils"
)

var leaveLinkedTemplateKeys = map[string]struct{}{
	"leave-request": {},
	"field-leave":   {},
}

// preflightWorkflowAttendanceSubmission rejects inactive applicants before workflow projection or stage resolution.
func (c AttendanceService) preflightWorkflowAttendanceSubmission(ctx RequestContext, account Account, templateKey string) error {
	templateKey = strings.TrimSpace(templateKey)
	_, isLeave := leaveLinkedTemplateKeys[templateKey]
	_, isOvertime := overtimeLinkedTemplateKeys[templateKey]
	if !isLeave && !isOvertime {
		return nil
	}
	employeeID := strings.TrimSpace(account.EmployeeID)
	if employeeID == "" {
		return BadRequest("form applicant account is not linked to an employee")
	}
	_, err := c.requireAttendanceEmployeeActive(ctx, employeeID)
	return err
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
	employeeID, err := c.workflowApplicantEmployeeID(ctx, instance)
	if err != nil {
		return LeaveRequest{}, err
	}
	if _, err := c.requireAttendanceEmployeeActive(ctx, employeeID); err != nil {
		return LeaveRequest{}, err
	}
	existing, resubmitting, err := c.store.GetLeaveRequestByFormInstanceID(goContext(ctx), ctx.TenantID, instance.ID)
	if err != nil {
		return LeaveRequest{}, err
	}
	if resubmitting {
		if strings.TrimSpace(existing.EmployeeID) != employeeID {
			return LeaveRequest{}, Conflict("linked leave request does not belong to the form applicant")
		}
		switch normalizeLeaveRequestStatus(existing.Status) {
		case "pending_approval":
			return existing, nil
		case "rejected", "cancelled":
			// Returned submissions reuse their stable request identity and reserve entitlement again below.
		case "approved":
			return LeaveRequest{}, Conflict("approved linked leave request cannot be resubmitted")
		default:
			return LeaveRequest{}, Conflict("linked leave request is not eligible for resubmission")
		}
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

	evaluation, err := c.EvaluateLeaveRequestRules(ctx, employeeID, leaveTypeRaw, startAt, endAt, 0)
	if err != nil {
		return LeaveRequest{}, err
	}
	if !evaluation.Eligible {
		return LeaveRequest{}, leaveEvaluationError(evaluation)
	}
	leaveTypeCode := evaluation.LeaveType
	hours := evaluation.Hours

	reason := utils.FirstNonEmpty(stringFromAny(payload["reason"]), stringFromAny(payload["description"]))
	requestID := utils.NewID("lr")
	createdAt := c.Now()
	if resubmitting {
		requestID = existing.ID
		createdAt = existing.CreatedAt
	}
	balanceCycle := nextLeaveRequestBalanceCycle(existing, resubmitting)
	leaveBalanceID := ""
	if evaluation.BalanceRequired {
		balance, fallbackReason, err := c.reserveLeaveBalanceIfAvailable(ctx, employeeID, evaluation.LeaveTypeID, hours, startAt)
		if err != nil {
			return LeaveRequest{}, err
		}
		if fallbackReason != "" {
			evaluation.BalanceInitialized = fallbackReason != leaveEvaluationBalanceMissing
			evaluation.AvailableHours = balance.RemainingHours
			evaluation = applyLeaveBalanceFallback(evaluation, fallbackReason)
		} else {
			leaveBalanceID = balance.ID
		}
	}
	evaluationSnapshot := leaveEvaluationSnapshotMap(evaluation)
	evaluationSnapshot["balance_cycle"] = balanceCycle
	req := LeaveRequest{
		ID:                   requestID,
		TenantID:             ctx.TenantID,
		EmployeeID:           employeeID,
		LeaveType:            leaveTypeCode,
		LeaveTypeID:          evaluation.LeaveTypeID,
		PolicyVersion:        evaluation.PolicyVersion,
		RuleSnapshot:         leaveRuleSnapshotMap(evaluation.Rule),
		EvaluationSnapshot:   evaluationSnapshot,
		StartAt:              startAt,
		EndAt:                endAt,
		Hours:                hours,
		Reason:               strings.TrimSpace(reason),
		Status:               "pending_approval",
		FormInstanceID:       instance.ID,
		LeaveBalanceID:       leaveBalanceID,
		ReconciliationStatus: "not_required",
		CreatedAt:            createdAt,
		UpdatedAt:            c.Now(),
	}
	if err := c.store.UpsertLeaveRequest(goContext(ctx), req); err != nil {
		return LeaveRequest{}, err
	}
	if leaveBalanceID != "" {
		if err := c.store.UpsertLeaveRequestAllocation(goContext(ctx), LeaveRequestAllocation{
			TenantID: ctx.TenantID, LeaveRequestID: req.ID, LeaveBalanceID: leaveBalanceID,
			ReservedHours: req.Hours, CreatedAt: c.Now(),
		}); err != nil {
			return LeaveRequest{}, err
		}
		if err := c.appendLeaveBalanceEntry(ctx, req, leaveBalanceID, "", leaveBalanceEntryReserve, -leaveMinutes(req.Hours), balanceCycle); err != nil {
			return LeaveRequest{}, err
		}
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
	if leaveBalanceID != "" {
		if err := c.audit(ctx, "attendance.leave_balance.reserve", "leave_balance", employeeID+"|"+leaveTypeCode, "medium", map[string]any{
			"employee_id":    employeeID,
			"leave_type":     leaveTypeCode,
			"reserved_hours": hours,
		}); err != nil {
			return LeaveRequest{}, err
		}
	}
	event := "attendance.leave_request.create"
	if resubmitting {
		event = "attendance.leave_request.resubmit"
	}
	if err := c.audit(ctx, event, "leave_request", req.ID, "medium", map[string]any{
		"leave_type":       req.LeaveType,
		"hours":            req.Hours,
		"form_instance_id": instance.ID,
		"source":           "workflow.form.submit",
		"resubmit":         resubmitting,
	}); err != nil {
		return LeaveRequest{}, err
	}
	return req, nil
}

// workflowApplicantEmployeeID derives leave ownership from the immutable form applicant.
func (c AttendanceService) workflowApplicantEmployeeID(ctx RequestContext, instance FormInstance) (string, error) {
	accountID := strings.TrimSpace(instance.ApplicantAccountID)
	if accountID == "" {
		return "", BadRequest("form applicant account is required")
	}
	account, ok, err := c.Service.store.GetAccount(goContext(ctx), ctx.TenantID, accountID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", NotFound("account", accountID)
	}
	employeeID := strings.TrimSpace(account.EmployeeID)
	if employeeID == "" {
		return "", BadRequest("form applicant account is not linked to an employee")
	}
	return employeeID, nil
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
