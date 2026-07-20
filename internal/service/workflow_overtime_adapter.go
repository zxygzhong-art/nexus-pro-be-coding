package service

import (
	"strconv"
	"strings"
	"time"

	"nexus-pro-api/internal/utils"
)

var overtimeLinkedTemplateKeys = map[string]struct{}{
	"overtime-approval": {},
}

// createOvertimeRequestFromSubmittedForm links overtime data inside the caller's submission transaction.
func (c AttendanceService) createOvertimeRequestFromSubmittedForm(ctx RequestContext, instance FormInstance, templateKey string, payload map[string]any) (OvertimeRequest, error) {
	if _, ok := overtimeLinkedTemplateKeys[strings.TrimSpace(templateKey)]; !ok {
		return OvertimeRequest{}, nil
	}
	if payload == nil || !workflowOvertimePayloadHasLinkedFields(payload) {
		return OvertimeRequest{}, nil
	}
	employeeID, err := c.workflowApplicantEmployeeID(ctx, instance)
	if err != nil {
		return OvertimeRequest{}, err
	}
	if _, err := c.requireAttendanceEmployeeActive(ctx, employeeID); err != nil {
		return OvertimeRequest{}, err
	}
	existing, resubmitting, err := c.store.GetOvertimeRequestByFormInstanceID(goContext(ctx), ctx.TenantID, instance.ID)
	if err != nil {
		return OvertimeRequest{}, err
	}
	if resubmitting {
		if strings.TrimSpace(existing.EmployeeID) != employeeID {
			return OvertimeRequest{}, Conflict("linked overtime request does not belong to the form applicant")
		}
		switch normalizeLeaveRequestStatus(existing.Status) {
		case "pending_approval":
			return existing, nil
		case "rejected", "cancelled":
			// Returned submissions reuse their stable request identity and refresh the requested values below.
		case "approved":
			return OvertimeRequest{}, Conflict("approved linked overtime request cannot be resubmitted")
		default:
			return OvertimeRequest{}, Conflict("linked overtime request is not eligible for resubmission")
		}
	}

	startRaw := utils.FirstNonEmpty(stringFromAny(payload["start_at"]), stringFromAny(payload["startAt"]))
	endRaw := utils.FirstNonEmpty(stringFromAny(payload["end_at"]), stringFromAny(payload["endAt"]))
	startAt, err := utils.ParseDateTime(startRaw)
	if err != nil {
		return OvertimeRequest{}, BadRequest("start_at must be RFC3339 or YYYY-MM-DD")
	}
	endAt, err := utils.ParseDateTime(endRaw)
	if err != nil {
		return OvertimeRequest{}, BadRequest("end_at must be RFC3339 or YYYY-MM-DD")
	}
	if !endAt.After(startAt) {
		return OvertimeRequest{}, BadRequest("end_at must be after start_at")
	}
	hoursText, ok := formProjectionNumber(payload["hours"])
	if !ok {
		return OvertimeRequest{}, BadRequest("hours must be greater than zero")
	}
	hours, err := strconv.ParseFloat(hoursText, 64)
	if err != nil || hours <= 0 {
		return OvertimeRequest{}, BadRequest("hours must be greater than zero")
	}
	overtimeType, err := normalizeOvertimeType(stringFromAny(payload["overtime_type"]))
	if err != nil {
		return OvertimeRequest{}, err
	}
	compensationType, err := normalizeOvertimeCompensationType(stringFromAny(payload["compensation_type"]))
	if err != nil {
		return OvertimeRequest{}, err
	}
	reason := strings.TrimSpace(utils.FirstNonEmpty(stringFromAny(payload["reason"]), stringFromAny(payload["description"])))
	now := c.Now()
	requestID := utils.NewID("ot")
	createdAt := now
	if resubmitting {
		requestID = existing.ID
		createdAt = existing.CreatedAt
	}
	req := OvertimeRequest{
		ID: requestID, TenantID: ctx.TenantID, EmployeeID: employeeID,
		WorkDate: attendanceWorkDate(startAt), StartAt: startAt, EndAt: endAt, Hours: hours,
		OvertimeType: overtimeType, CompensationType: compensationType, Reason: reason,
		Status: "pending_approval", FormInstanceID: instance.ID, CreatedAt: createdAt, UpdatedAt: now,
	}
	if err := c.store.UpsertOvertimeRequest(goContext(ctx), req); err != nil {
		return OvertimeRequest{}, err
	}

	nextPayload := utils.CopyStringMap(instance.Payload)
	if nextPayload == nil {
		nextPayload = map[string]any{}
	}
	nextPayload["employee_id"] = employeeID
	nextPayload["overtime_request_id"] = requestID
	nextPayload["linked_resource_id"] = requestID
	nextPayload["linked_resource_type"] = "attendance.overtime_request"
	nextPayload["start_at"] = startAt.Format(time.RFC3339)
	nextPayload["end_at"] = endAt.Format(time.RFC3339)
	nextPayload["hours"] = hours
	nextPayload["overtime_type"] = overtimeType
	nextPayload["compensation_type"] = compensationType
	nextPayload["reason"] = reason
	instance.Payload = nextPayload
	instance.UpdatedAt = now
	if err := c.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
		return OvertimeRequest{}, err
	}
	event := "attendance.overtime_request.create"
	if resubmitting {
		event = "attendance.overtime_request.resubmit"
	}
	if err := c.audit(ctx, event, "overtime_request", req.ID, "medium", map[string]any{
		"employee_id": employeeID, "hours": hours, "overtime_type": overtimeType,
		"compensation_type": compensationType, "form_instance_id": instance.ID, "source": "workflow.form.submit", "resubmit": resubmitting,
	}); err != nil {
		return OvertimeRequest{}, err
	}
	return req, nil
}

// workflowOvertimePayloadHasLinkedFields avoids creating an empty overtime projection.
func workflowOvertimePayloadHasLinkedFields(payload map[string]any) bool {
	for _, key := range []string{"start_at", "startAt", "end_at", "endAt", "hours", "overtime_type", "compensation_type"} {
		if value, ok := payload[key]; ok && !isEmptyFormPayloadValue(value) {
			return true
		}
	}
	return false
}
