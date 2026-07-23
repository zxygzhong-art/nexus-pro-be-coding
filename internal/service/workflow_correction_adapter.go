package service

import (
	"strings"
	"time"
	"unicode/utf8"

	"nexus-pro-api/internal/utils"
)

const correctionLinkedTemplateKey = "punch-fix"

// createCorrectionFromSubmittedForm projects a punch-fix form into the generic
// business-record store inside the canonical workflow submission transaction.
func (c AttendanceService) createCorrectionFromSubmittedForm(ctx RequestContext, instance FormInstance, templateKey string, payload map[string]any) (AttendanceCorrectionRequest, error) {
	if strings.TrimSpace(templateKey) != correctionLinkedTemplateKey || !workflowCorrectionPayloadHasLinkedFields(payload) {
		return AttendanceCorrectionRequest{}, nil
	}
	employeeID, err := c.workflowApplicantEmployeeID(ctx, instance)
	if err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	if _, err := c.requireAttendanceEmployeeActive(ctx, employeeID); err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	existing, resubmitting, err := c.store.GetAttendanceCorrectionRequestByFormInstanceID(goContext(ctx), ctx.TenantID, instance.ID)
	if err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	if resubmitting {
		if strings.TrimSpace(existing.EmployeeID) != employeeID {
			return AttendanceCorrectionRequest{}, Conflict("linked attendance correction does not belong to the form applicant")
		}
		if existing.EffectStatus == "applied" {
			return AttendanceCorrectionRequest{}, Conflict("applied attendance correction cannot be resubmitted")
		}
	}

	correctionType, err := normalizeAttendanceCorrectionType(stringFromAny(payload["correction_type"]))
	if err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	targetRecordID := strings.TrimSpace(stringFromAny(payload["target_clock_record_id"]))
	var targetRecord AttendanceClockRecord
	if correctionType != correctionTypeAddRecord {
		if targetRecordID == "" {
			return AttendanceCorrectionRequest{}, BadRequest("target_clock_record_id is required")
		}
		targetRecord, err = c.findAttendanceClockRecord(ctx, employeeID, targetRecordID)
		if err != nil {
			return AttendanceCorrectionRequest{}, err
		}
		if targetRecord.Voided {
			return AttendanceCorrectionRequest{}, BadRequest("target clock record is already voided").WithReasonCode("attendance_correction_invalid_state")
		}
	}
	direction := targetRecord.Direction
	requestedAt := targetRecord.ClockedAt
	workDate := targetRecord.WorkDate
	if correctionType != correctionTypeVoidRecord {
		direction, err = normalizeClockDirection(stringFromAny(payload["direction"]))
		if err != nil {
			return AttendanceCorrectionRequest{}, err
		}
		requestedAt, err = utils.ParseDateTime(stringFromAny(payload["requested_clocked_at"]))
		if err != nil {
			return AttendanceCorrectionRequest{}, BadRequest("requested_clocked_at must be RFC3339 or YYYY-MM-DD")
		}
		if correctionType == correctionTypeAddRecord {
			workDate = attendanceWorkDate(requestedAt)
		}
	}
	reason := strings.TrimSpace(stringFromAny(payload["reason"]))
	if n := utf8.RuneCountInString(reason); n < attendanceCorrectionReasonMinLength {
		return AttendanceCorrectionRequest{}, BadRequest("reason must be at least 4 characters").WithReasonCode("attendance_correction_reason_too_short")
	} else if n > attendanceCorrectionReasonMaxLength {
		return AttendanceCorrectionRequest{}, BadRequest("reason must be at most 200 characters").WithReasonCode("attendance_correction_reason_too_long")
	}

	now := c.Now()
	requestID := utils.NewID("acorr")
	createdAt := now
	if resubmitting {
		requestID = existing.ID
		createdAt = existing.CreatedAt
	}
	request := AttendanceCorrectionRequest{
		ID: requestID, TenantID: ctx.TenantID, EmployeeID: employeeID,
		Direction: direction, RequestedClockedAt: requestedAt, WorkDate: workDate,
		CorrectionType: correctionType, TargetClockRecordID: targetRecordID, Reason: reason,
		Status: correctionStatusPending, FormInstanceID: instance.ID,
		EffectStatus: "not_applied", CreatedAt: createdAt, UpdatedAt: now,
	}
	if err := c.store.UpsertAttendanceCorrectionRequest(goContext(ctx), request); err != nil {
		return AttendanceCorrectionRequest{}, err
	}

	nextPayload := utils.CopyStringMap(instance.Payload)
	nextPayload["employee_id"] = employeeID
	nextPayload["correction_request_id"] = requestID
	nextPayload["linked_resource_id"] = requestID
	nextPayload["linked_resource_type"] = "attendance.clock_correction"
	nextPayload["correction_type"] = correctionType
	nextPayload["target_clock_record_id"] = targetRecordID
	nextPayload["direction"] = direction
	nextPayload["requested_clocked_at"] = requestedAt.Format(time.RFC3339)
	nextPayload["reason"] = reason
	instance.Payload = nextPayload
	instance.UpdatedAt = now
	if err := c.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	event := "attendance.correction.create"
	if resubmitting {
		event = "attendance.correction.resubmit"
	}
	if err := c.audit(ctx, event, string(ResourceAttendanceCorrection), request.ID, string(SeverityMedium), map[string]any{
		"employee_id": employeeID, "direction": direction, "form_instance_id": instance.ID,
		"source": "workflow.form.submit", "resubmit": resubmitting,
	}); err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	return request, nil
}

func workflowCorrectionPayloadHasLinkedFields(payload map[string]any) bool {
	for _, key := range []string{"correction_type", "target_clock_record_id", "direction", "requested_clocked_at", "reason"} {
		if value, ok := payload[key]; ok && !isEmptyFormPayloadValue(value) {
			return true
		}
	}
	return false
}
