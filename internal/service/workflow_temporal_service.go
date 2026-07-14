package service

import (
	"errors"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

func (c WorkflowService) startTemporalFormApprovalWorkflow(ctx RequestContext, instance domain.FormInstance) error {
	if c.formApprovalWorkflows == nil {
		return domain.E(503, "temporal_workflow_unavailable", "temporal form approval workflow client is required").WithReasonCode("temporal_workflow_unavailable")
	}
	if strings.TrimSpace(instance.ID) == "" {
		return BadRequest("form_instance_id is required")
	}
	run, ok, err := c.store.GetWorkflowRunByFormInstance(goContext(ctx), ctx.TenantID, instance.ID)
	if err != nil {
		return err
	}
	start := domain.FormApprovalWorkflowStart{
		TenantID:                ctx.TenantID,
		FormInstanceID:          instance.ID,
		DefaultRemindAfterHours: domain.DefaultFormApprovalRemindAfterHours,
	}
	if ok {
		start.RunID = run.ID
		start.StageDefinitionsJSON = run.StageDefinitionsJSON
	}
	return c.formApprovalWorkflows.StartFormApprovalWorkflow(goContext(ctx), start)
}

// markFormApprovalWorkflowStartFailed compensates a committed submit when Temporal start fails.
// The instance is marked workflow_start_failed so operators can backfill instead of leaving a silent in_review orphan.
func (c WorkflowService) markFormApprovalWorkflowStartFailed(ctx RequestContext, instance domain.FormInstance, startErr error) error {
	return c.withTransaction(ctx, func(tx WorkflowService) error {
		current, ok, err := tx.store.GetFormInstance(goContext(ctx), ctx.TenantID, instance.ID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("form instance", instance.ID)
		}
		now := tx.Now()
		current.Status = domain.WorkflowFormStatusWorkflowStartFailed
		current.UpdatedAt = now
		if err := tx.store.UpsertFormInstance(goContext(ctx), current); err != nil {
			return err
		}
		if run, ok, err := tx.store.GetWorkflowRunByFormInstance(goContext(ctx), ctx.TenantID, instance.ID); err != nil {
			return err
		} else if ok {
			run.Status = domain.WorkflowRunStatusStartFailed
			run.UpdatedAt = now
			if err := tx.store.UpsertWorkflowRun(goContext(ctx), run); err != nil {
				return err
			}
		}
		details := map[string]any{
			"template_id": current.TemplateID,
			"temporal":    true,
		}
		if startErr != nil {
			details["error"] = startErr.Error()
		}
		return tx.audit(ctx, "workflow.form.temporal_start_failed", string(ResourceFormInstance), current.ID, string(SeverityHigh), details)
	})
}

func (c WorkflowService) signalTemporalFormApprovalWorkflow(ctx RequestContext, id, action, expectedStatus, reason string) (domain.FormInstance, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.FormInstance{}, BadRequest("id is required")
	}
	if _, _, err := c.requireWorkflowAuthz(ctx, ResourceFormInstance, workflowSignalAuthzAction(action), workflowSignalAuthzResourceID(action, id)); err != nil {
		return domain.FormInstance{}, err
	}
	if c.formApprovalWorkflows == nil {
		return domain.FormInstance{}, domain.E(503, "temporal_workflow_unavailable", "temporal form approval workflow client is required").WithReasonCode("temporal_workflow_unavailable")
	}
	before, err := c.LoadTemporalFormApprovalProjection(ctx, id)
	if err != nil {
		return domain.FormInstance{}, err
	}
	signal := domain.FormApprovalWorkflowSignal{
		TenantID:       ctx.TenantID,
		FormInstanceID: id,
		AccountID:      ctx.AccountID,
		Action:         strings.TrimSpace(strings.ToLower(action)),
		Reason:         strings.TrimSpace(reason),
		RequestID:      ctx.RequestID,
		TraceID:        ctx.TraceID,
	}
	if err := c.formApprovalWorkflows.SignalFormApprovalWorkflow(goContext(ctx), signal); err != nil {
		if errors.Is(err, domain.ErrFormApprovalWorkflowNotFound) {
			return domain.FormInstance{}, domain.E(404, "workflow_not_found", "form approval workflow is not available for this form instance").WithReasonCode("workflow_not_found")
		}
		return domain.FormInstance{}, err
	}
	projection, err := c.waitTemporalFormApprovalProjection(ctx, before, expectedStatus)
	if err != nil {
		return domain.FormInstance{}, err
	}
	instance, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, projection.FormInstanceID)
	if err != nil {
		return domain.FormInstance{}, err
	}
	if !ok {
		return domain.FormInstance{}, NotFound("form instance", projection.FormInstanceID)
	}
	return instance, nil
}

// LoadTemporalFormApprovalProjection reads the current DB projection for a Temporal workflow.
func (c WorkflowService) LoadTemporalFormApprovalProjection(ctx RequestContext, formInstanceID string) (domain.FormApprovalProjection, error) {
	instance, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, strings.TrimSpace(formInstanceID))
	if err != nil {
		return domain.FormApprovalProjection{}, err
	}
	if !ok {
		return domain.FormApprovalProjection{}, NotFound("form instance", formInstanceID)
	}
	projection := domain.FormApprovalProjection{
		TenantID:       ctx.TenantID,
		FormInstanceID: instance.ID,
		FormStatus:     instance.Status,
		UpdatedAt:      instance.UpdatedAt,
	}
	run, ok, err := c.store.GetWorkflowRunByFormInstance(goContext(ctx), ctx.TenantID, instance.ID)
	if err != nil {
		return domain.FormApprovalProjection{}, err
	}
	if !ok {
		return projection, nil
	}
	projection.RunID = run.ID
	projection.RunStatus = run.Status
	projection.CurrentStageInstanceID = run.CurrentStageInstanceID
	if !run.UpdatedAt.IsZero() && run.UpdatedAt.After(projection.UpdatedAt) {
		projection.UpdatedAt = run.UpdatedAt
	}
	if run.CurrentStageInstanceID == "" {
		return projection, nil
	}
	stageInstance, ok, err := c.store.GetWorkflowStageInstance(goContext(ctx), ctx.TenantID, run.CurrentStageInstanceID)
	if err != nil {
		return domain.FormApprovalProjection{}, err
	}
	if !ok {
		return projection, nil
	}
	projection.CurrentStageID = stageInstance.StageID
	projection.CurrentStageLabel = stageInstance.Label
	stage := workflowStageByID(DeserializeWorkflowStages(run.StageDefinitionsJSON), stageInstance.StageID)
	projection.RemindAfterHours = stage.Config.RemindAfterHours
	if projection.RemindAfterHours <= 0 {
		projection.RemindAfterHours = domain.DefaultFormApprovalRemindAfterHours
	}
	return projection, nil
}

// ApplyTemporalFormApprovalSignal updates query projections for one Temporal signal.
func (c WorkflowService) ApplyTemporalFormApprovalSignal(ctx RequestContext, signal domain.FormApprovalWorkflowSignal) (domain.FormApprovalProjection, error) {
	action := strings.TrimSpace(strings.ToLower(signal.Action))
	var instance domain.FormInstance
	var err error
	switch action {
	case domain.FormApprovalWorkflowActionApprove:
		instance, err = c.ActOnWorkflowStage(ctx, signal.FormInstanceID, "approve", signal.Reason)
	case domain.FormApprovalWorkflowActionReject:
		instance, err = c.ActOnWorkflowStage(ctx, signal.FormInstanceID, "reject", signal.Reason)
	case domain.FormApprovalWorkflowActionReturn:
		instance, err = c.ActOnWorkflowStage(ctx, signal.FormInstanceID, "return", signal.Reason)
	case domain.FormApprovalWorkflowActionWithdraw:
		instance, err = c.withdrawTemporalFormApproval(ctx, signal.FormInstanceID, signal.Reason)
	default:
		return c.LoadTemporalFormApprovalProjection(ctx, signal.FormInstanceID)
	}
	if err != nil {
		return domain.FormApprovalProjection{}, err
	}
	severity := string(SeverityHigh)
	if action == domain.FormApprovalWorkflowActionWithdraw {
		severity = string(SeverityMedium)
	}
	if auditErr := c.audit(ctx, "workflow.form."+action, string(ResourceFormInstance), instance.ID, severity, map[string]any{
		"template_id":      instance.TemplateID,
		"reviewed_by":      ctx.AccountID,
		"review_action":    action,
		"review_comment":   strings.TrimSpace(signal.Reason),
		"resulting_status": instance.Status,
		"temporal":         true,
	}); auditErr != nil {
		return domain.FormApprovalProjection{}, auditErr
	}
	return c.LoadTemporalFormApprovalProjection(ctx, instance.ID)
}

// RecordTemporalFormApprovalReminder writes one reminder notification and audit log.
func (c WorkflowService) RecordTemporalFormApprovalReminder(ctx RequestContext, reminder domain.FormApprovalReminder) error {
	instance, run, stageInstance, _, err := c.loadActiveWorkflowStage(ctx, reminder.FormInstanceID)
	if err != nil {
		return err
	}
	assignees, err := c.store.ListWorkflowStageAssignees(goContext(ctx), ctx.TenantID, stageInstance.ID)
	if err != nil {
		return err
	}
	recipients := make([]string, 0, len(assignees))
	for _, assignee := range assignees {
		if assignee.Status == domain.WorkflowAssigneeStatusPending {
			recipients = append(recipients, assignee.AccountID)
		}
	}
	template, ok, err := c.store.GetFormTemplate(goContext(ctx), ctx.TenantID, instance.TemplateID)
	if err != nil {
		return err
	}
	if !ok {
		template = domain.FormTemplate{ID: instance.TemplateID}
	}
	title := workflowNotificationTemplateTitle(template, instance)
	body := "「" + title + "」在「" + utils.FirstNonEmpty(stageInstance.Label, reminder.CurrentStageLabel, stageInstance.StageID) + "」已等待超過設定時間，請盡快處理。"
	if err := c.deliverWorkflowNotification(ctx, domain.Notification{
		ID:                 workflowNotificationID("reminder-"+stageInstance.ID, instance.ID),
		TenantID:           ctx.TenantID,
		Tone:               "warning",
		Category:           "workflow",
		Title:              "催辦：" + title,
		Body:               body,
		StatusText:         "待處理",
		LinkURL:            "/notifications?reviewId=" + instance.ID,
		SourceType:         "workflow.form.reminder",
		SourceID:           instance.ID + ":" + stageInstance.ID,
		CreatedByAccountID: "system",
		CreatedAt:          c.Now(),
	}, recipients); err != nil {
		return err
	}
	return c.audit(ctx, "workflow.form.reminder", string(ResourceFormInstance), instance.ID, string(SeverityMedium), map[string]any{
		"run_id":            run.ID,
		"stage_id":          stageInstance.StageID,
		"stage_instance_id": stageInstance.ID,
		"recipient_count":   len(uniqueWorkflowRecipientIDs(recipients)),
		"temporal":          true,
	})
}

func (c WorkflowService) withdrawTemporalFormApproval(ctx RequestContext, formInstanceID, reason string) (domain.FormInstance, error) {
	account, decision, err := c.requireWorkflowAuthz(ctx, ResourceFormInstance, ActionUpdate, "")
	if err != nil {
		return domain.FormInstance{}, err
	}
	var instance domain.FormInstance
	if err := c.withTransaction(ctx, func(tx WorkflowService) error {
		current, run, stageInstance, _, err := tx.loadActiveWorkflowStage(ctx, formInstanceID)
		if err != nil {
			return err
		}
		if err := requireFormInstanceVisible(current, account, decision); err != nil {
			return err
		}
		now := tx.Now()
		if stageInstance.ID != "" {
			if err := tx.recordWorkflowAction(ctx, run, stageInstance, domain.FormApprovalWorkflowActionWithdraw, reason, now); err != nil {
				return err
			}
			stageInstance.Status = domain.WorkflowStageStatusSkipped
			stageInstance.CompletedAt = &now
			if err := tx.store.UpsertWorkflowStageInstance(goContext(ctx), stageInstance); err != nil {
				return err
			}
		}
		run.Status = domain.WorkflowRunStatusCancelled
		run.CurrentStageInstanceID = ""
		run.UpdatedAt = now
		current.Status = workflowFormStatusCancelled
		current.ApprovedBy = ctx.AccountID
		current.Payload = withWorkflowReview(current.Payload, domain.FormApprovalWorkflowActionWithdraw, ctx.AccountID, reason, now)
		current.UpdatedAt = now
		if err := tx.store.UpsertWorkflowRun(goContext(ctx), run); err != nil {
			return err
		}
		if err := tx.store.UpsertFormInstance(goContext(ctx), current); err != nil {
			return err
		}
		if err := tx.Service.Attendance().applyAttendanceWorkflowReview(ctx, current, "cancel", workflowFormStatusCancelled); err != nil {
			return err
		}
		instance = current
		return nil
	}); err != nil {
		return domain.FormInstance{}, err
	}
	return instance, nil
}

// workflowSignalAuthzAction maps review signals to their policy action before side effects run.
func workflowSignalAuthzAction(action string) Action {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case domain.FormApprovalWorkflowActionApprove, domain.FormApprovalWorkflowActionReject, domain.FormApprovalWorkflowActionReturn:
		return ActionApprove
	default:
		return ActionUpdate
	}
}

// workflowSignalAuthzResourceID defers withdraw ownership checks until the form projection is loaded.
func workflowSignalAuthzResourceID(action, id string) string {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case domain.FormApprovalWorkflowActionWithdraw:
		return ""
	default:
		return id
	}
}

func (c WorkflowService) waitTemporalFormApprovalProjection(ctx RequestContext, before domain.FormApprovalProjection, expectedStatus string) (domain.FormApprovalProjection, error) {
	execCtx := goContext(ctx)
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		projection, err := c.LoadTemporalFormApprovalProjection(ctx, before.FormInstanceID)
		if err != nil {
			return domain.FormApprovalProjection{}, err
		}
		if temporalProjectionReached(projection, before, expectedStatus) {
			return projection, nil
		}
		select {
		case <-execCtx.Done():
			return domain.FormApprovalProjection{}, execCtx.Err()
		case <-deadline.C:
			return domain.FormApprovalProjection{}, domain.E(503, "temporal_projection_timeout", "temporal workflow projection did not update before timeout")
		case <-ticker.C:
		}
	}
}

func temporalProjectionReached(projection, before domain.FormApprovalProjection, expectedStatus string) bool {
	if strings.EqualFold(projection.FormStatus, strings.TrimSpace(expectedStatus)) {
		return true
	}
	if !projection.UpdatedAt.IsZero() && projection.UpdatedAt.After(before.UpdatedAt) {
		return true
	}
	return projection.CurrentStageInstanceID != "" && projection.CurrentStageInstanceID != before.CurrentStageInstanceID
}
