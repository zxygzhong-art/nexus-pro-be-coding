package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

type formApprovalWorkflowEnsurer interface {
	EnsureFormApprovalWorkflow(context.Context, domain.FormApprovalWorkflowStart) (domain.FormApprovalWorkflowExecution, error)
}

const workflowTemporalStartClaimLease = 5 * time.Minute

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
		start.WorkflowID = run.TemporalWorkflowID
		start.StageDefinitionsJSON = run.StageDefinitionsJSON
	}
	return c.formApprovalWorkflows.StartFormApprovalWorkflow(goContext(ctx), start)
}

// HandleWorkflowStartEvent converges one committed workflow run with Temporal.
// It deliberately never compensates committed attendance or balance projections.
func (c WorkflowService) HandleWorkflowStartEvent(execCtx context.Context, event domain.OutboxEvent) error {
	if strings.TrimSpace(event.EventType) != domain.WorkflowStartRequestedEventType {
		return BadRequest("unsupported workflow start event type")
	}
	var payload domain.WorkflowStartRequestedPayload
	raw, err := json.Marshal(event.Payload)
	if err != nil {
		return BadRequest("invalid workflow start payload")
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return BadRequest("invalid workflow start payload")
	}
	payload.RunID = strings.TrimSpace(payload.RunID)
	payload.FormInstanceID = strings.TrimSpace(payload.FormInstanceID)
	payload.WorkflowID = strings.TrimSpace(payload.WorkflowID)
	if payload.RunID == "" || payload.FormInstanceID == "" || payload.WorkflowID == "" {
		return BadRequest("workflow start payload requires run_id, form_instance_id, and temporal_workflow_id")
	}
	if aggregateID := strings.TrimSpace(event.AggregateID); aggregateID != "" && aggregateID != payload.RunID {
		return BadRequest("workflow start aggregate does not match run_id")
	}
	ctx := RequestContext{Context: execCtx, TenantID: event.TenantID, AccountID: "system"}
	run, ok, err := c.store.GetWorkflowRun(goContext(ctx), ctx.TenantID, payload.RunID)
	if err != nil {
		return err
	}
	if !ok {
		return NotFound("workflow run", payload.RunID)
	}
	if run.FormInstanceID != payload.FormInstanceID || run.TemporalWorkflowID != payload.WorkflowID {
		return Conflict("workflow start payload no longer matches the persisted run").WithReasonCode("workflow_start_stale")
	}
	if run.TemporalStartEventID != "" && run.TemporalStartEventID != event.ID {
		return Conflict("workflow start event is stale").WithReasonCode("workflow_start_stale")
	}
	switch run.TemporalStartStatus {
	case domain.WorkflowTemporalStartStarted, domain.WorkflowTemporalStartAbandoned:
		return nil
	case domain.WorkflowTemporalStartPending, domain.WorkflowTemporalStartStarting:
	default:
		return Conflict("workflow start has an invalid delivery state").WithReasonCode("workflow_start_state_invalid")
	}
	claimedAt := c.Now()
	run, claimed, err := c.store.ClaimWorkflowRunTemporalStart(
		goContext(ctx), ctx.TenantID, run.ID, claimedAt, claimedAt.Add(-workflowTemporalStartClaimLease),
	)
	if err != nil {
		return err
	}
	if !claimed {
		current, found, loadErr := c.store.GetWorkflowRun(goContext(ctx), ctx.TenantID, payload.RunID)
		if loadErr != nil {
			return loadErr
		}
		if !found {
			return NotFound("workflow run", payload.RunID)
		}
		if current.TemporalStartStatus == domain.WorkflowTemporalStartStarted || current.TemporalStartStatus == domain.WorkflowTemporalStartAbandoned {
			return nil
		}
		return fmt.Errorf("workflow start %s is already claimed", payload.RunID)
	}
	claimToken := run.UpdatedAt
	releaseClaim := func(cause error) error {
		_, releaseErr := c.store.ReleaseWorkflowRunTemporalStart(goContext(ctx), ctx.TenantID, run.ID, claimToken, c.Now())
		if releaseErr != nil {
			return errors.Join(cause, fmt.Errorf("release workflow start claim: %w", releaseErr))
		}
		return cause
	}
	instance, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, run.FormInstanceID)
	if err != nil {
		return releaseClaim(err)
	}
	if !ok {
		_, _, abandonErr := c.store.AbandonClaimedWorkflowRunTemporalStart(goContext(ctx), ctx.TenantID, run.ID, claimToken, c.Now())
		if abandonErr != nil {
			return errors.Join(NotFound("form instance", run.FormInstanceID), abandonErr)
		}
		return NotFound("form instance", run.FormInstanceID)
	}
	if (instance.CurrentRunID != "" && instance.CurrentRunID != run.ID) || workflowRunTerminalForTemporalStart(instance, run) {
		_, abandoned, abandonErr := c.store.AbandonClaimedWorkflowRunTemporalStart(goContext(ctx), ctx.TenantID, run.ID, claimToken, c.Now())
		if abandonErr != nil {
			return abandonErr
		}
		if !abandoned {
			return fmt.Errorf("workflow start %s lost its claim before abandonment", run.ID)
		}
		return nil
	}
	start := domain.FormApprovalWorkflowStart{
		TenantID:                ctx.TenantID,
		FormInstanceID:          instance.ID,
		RunID:                   run.ID,
		WorkflowID:              run.TemporalWorkflowID,
		StageDefinitionsJSON:    run.StageDefinitionsJSON,
		DefaultRemindAfterHours: domain.DefaultFormApprovalRemindAfterHours,
	}
	execution := domain.FormApprovalWorkflowExecution{WorkflowID: run.TemporalWorkflowID}
	if ensurer, ok := c.formApprovalWorkflows.(formApprovalWorkflowEnsurer); ok {
		execution, err = ensurer.EnsureFormApprovalWorkflow(execCtx, start)
	} else if c.formApprovalWorkflows == nil {
		err = domain.E(503, "temporal_workflow_unavailable", "temporal form approval workflow client is required")
	} else {
		err = c.formApprovalWorkflows.StartFormApprovalWorkflow(execCtx, start)
	}
	if err != nil {
		return releaseClaim(err)
	}
	if err := c.markTemporalStartDelivered(ctx, run.ID, claimToken, execution); err != nil {
		return releaseClaim(err)
	}
	return nil
}

func workflowStartOutboxEvent(run domain.WorkflowRun, now time.Time) domain.OutboxEvent {
	unlimitedAttempts := 0
	return domain.OutboxEvent{
		ID:            run.TemporalStartEventID,
		TenantID:      run.TenantID,
		EventType:     domain.WorkflowStartRequestedEventType,
		AggregateType: domain.WorkflowStartAggregateType,
		AggregateID:   run.ID,
		Payload: map[string]any{
			"run_id":               run.ID,
			"form_instance_id":     run.FormInstanceID,
			"temporal_workflow_id": run.TemporalWorkflowID,
		},
		PayloadVersion: 1,
		IdempotencyKey: run.ID,
		Status:         domain.OutboxStatusPending,
		MaxAttempts:    &unlimitedAttempts,
		NextAttemptAt:  now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// ReconcilePendingWorkflowStarts is a safety net for lost wakeups, stale
// dispatcher claims, and a Temporal success followed by a DB write failure.
func (c WorkflowService) ReconcilePendingWorkflowStarts(execCtx context.Context, tenantID string, limit int) (int, error) {
	ctx := RequestContext{Context: execCtx, TenantID: strings.TrimSpace(tenantID), AccountID: "system"}
	runs, err := c.store.ListPendingWorkflowRuns(execCtx, ctx.TenantID, c.Now().Add(-workflowTemporalStartClaimLease), limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	var errs []error
	for _, candidate := range runs {
		run := candidate
		if strings.TrimSpace(run.TemporalStartEventID) == "" {
			if run.TemporalStartStatus == domain.WorkflowTemporalStartStarting {
				releasedAt := c.Now()
				released, releaseErr := c.store.ReleaseWorkflowRunTemporalStart(execCtx, ctx.TenantID, run.ID, run.UpdatedAt, releasedAt)
				if releaseErr != nil {
					errs = append(errs, fmt.Errorf("release stale run %s without start event: %w", run.ID, releaseErr))
					continue
				}
				if !released {
					continue
				}
				run.TemporalStartStatus = domain.WorkflowTemporalStartPending
				run.UpdatedAt = releasedAt
			}
			stale := false
			err := c.withTransaction(ctx, func(tx WorkflowService) error {
				current, ok, err := tx.store.GetWorkflowRun(goContext(ctx), ctx.TenantID, run.ID)
				if err != nil {
					return err
				}
				if !ok || current.TemporalStartStatus != domain.WorkflowTemporalStartPending {
					stale = true
					return nil
				}
				now := tx.Now()
				existingEvents, err := tx.store.ListOutboxEvents(goContext(ctx), ctx.TenantID)
				if err != nil {
					return err
				}
				createEvent := true
				for _, existing := range existingEvents {
					if existing.EventType == domain.WorkflowStartRequestedEventType && existing.IdempotencyKey == current.ID {
						current.TemporalStartEventID = existing.ID
						createEvent = false
						break
					}
				}
				if createEvent {
					current.TemporalStartEventID = utils.NewID("outbox")
				}
				current.UpdatedAt = now
				if err := tx.store.UpsertWorkflowRun(goContext(ctx), current); err != nil {
					return err
				}
				if createEvent {
					if err := tx.store.AppendOutboxEvent(goContext(ctx), workflowStartOutboxEvent(current, now)); err != nil {
						return err
					}
				}
				run = current
				return nil
			})
			if err != nil {
				errs = append(errs, fmt.Errorf("repair run %s start event: %w", run.ID, err))
				continue
			}
			if stale {
				continue
			}
		}
		event, found, err := c.store.GetOutboxEventByID(execCtx, ctx.TenantID, run.TemporalStartEventID)
		if err != nil {
			errs = append(errs, fmt.Errorf("load run %s start event: %w", run.ID, err))
			continue
		}
		if !found {
			event = workflowStartOutboxEvent(run, c.Now())
			if err := c.store.AppendOutboxEvent(execCtx, event); err != nil {
				errs = append(errs, fmt.Errorf("recreate run %s start event: %w", run.ID, err))
				continue
			}
		}
		if err := c.HandleWorkflowStartEvent(execCtx, event); err != nil {
			if deferErr := c.deferFailedWorkflowStartReconcile(execCtx, ctx.TenantID, run.ID); deferErr != nil {
				err = errors.Join(err, fmt.Errorf("defer retry: %w", deferErr))
			}
			errs = append(errs, fmt.Errorf("converge run %s: %w", run.ID, err))
			continue
		}
		processed++
	}
	if processed > 0 {
		c.wakeOutboxDispatcher()
	}
	return processed, errors.Join(errs...)
}

// deferFailedWorkflowStartReconcile rotates a poison/transient row to the end
// of the pending queue without allowing an older worker to overwrite a newer
// claim. This prevents a fixed-size batch of bad rows from starving later runs.
func (c WorkflowService) deferFailedWorkflowStartReconcile(execCtx context.Context, tenantID, runID string) error {
	now := c.Now()
	claimed, ok, err := c.store.ClaimWorkflowRunTemporalStart(execCtx, tenantID, runID, now, now.Add(-workflowTemporalStartClaimLease))
	if err != nil || !ok {
		return err
	}
	_, err = c.store.ReleaseWorkflowRunTemporalStart(execCtx, tenantID, runID, claimed.UpdatedAt, now)
	return err
}

func workflowRunTerminalForTemporalStart(instance domain.FormInstance, run domain.WorkflowRun) bool {
	switch strings.ToLower(strings.TrimSpace(instance.Status)) {
	case workflowFormStatusApproved, workflowFormStatusRejected, workflowFormStatusReturned, workflowFormStatusCancelled, "canceled":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(run.Status)) {
	case domain.WorkflowRunStatusCompleted, domain.WorkflowRunStatusReturned, domain.WorkflowRunStatusCancelled, domain.WorkflowRunStatusStartFailed:
		return true
	default:
		return false
	}
}

func (c WorkflowService) markTemporalStartDelivered(ctx RequestContext, runID string, claimToken time.Time, execution domain.FormApprovalWorkflowExecution) error {
	return c.withTransaction(ctx, func(tx WorkflowService) error {
		now := tx.Now()
		run, transitioned, err := tx.store.MarkWorkflowRunTemporalStarted(goContext(ctx), ctx.TenantID, runID, claimToken, execution, now)
		if err != nil {
			return err
		}
		if !transitioned {
			current, ok, loadErr := tx.store.GetWorkflowRun(goContext(ctx), ctx.TenantID, runID)
			if loadErr != nil {
				return loadErr
			}
			if !ok {
				return NotFound("workflow run", runID)
			}
			if current.TemporalStartStatus == domain.WorkflowTemporalStartStarted || current.TemporalStartStatus == domain.WorkflowTemporalStartAbandoned {
				return nil
			}
			return fmt.Errorf("workflow start %s lost its delivery claim", runID)
		}
		if err := tx.notifyCurrentWorkflowApproversAfterTemporalStart(ctx, run); err != nil {
			return err
		}
		return tx.audit(ctx, "workflow.form.temporal_started", string(ResourceFormInstance), run.FormInstanceID, string(SeverityMedium), map[string]any{
			"run_id":               run.ID,
			"temporal_workflow_id": run.TemporalWorkflowID,
			"temporal_run_id":      run.TemporalRunID,
		})
	})
}

func (c WorkflowService) notifyCurrentWorkflowApproversAfterTemporalStart(ctx RequestContext, run domain.WorkflowRun) error {
	if run.Status != domain.WorkflowRunStatusRunning || run.CurrentStageInstanceID == "" {
		return nil
	}
	stageInstance, ok, err := c.store.GetWorkflowStageInstance(goContext(ctx), ctx.TenantID, run.CurrentStageInstanceID)
	if err != nil {
		return err
	}
	if !ok {
		return NotFound("workflow stage instance", run.CurrentStageInstanceID)
	}
	if stageInstance.Status != domain.WorkflowStageStatusActive || stageInstance.StageType != "approver" {
		return nil
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
	instance, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, run.FormInstanceID)
	if err != nil {
		return err
	}
	if !ok {
		return NotFound("form instance", run.FormInstanceID)
	}
	template, ok, err := c.store.GetFormTemplate(goContext(ctx), ctx.TenantID, run.TemplateID)
	if err != nil {
		return err
	}
	if !ok {
		return NotFound("form template", run.TemplateID)
	}
	applicant, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, instance.ApplicantAccountID)
	if err != nil {
		return err
	}
	if !ok {
		return NotFound("account", instance.ApplicantAccountID)
	}
	stage := workflowStageByID(DeserializeWorkflowStages(run.StageDefinitionsJSON), stageInstance.StageID)
	if stage.ID == "" {
		stage = domain.WorkflowStageDefinition{ID: stageInstance.StageID, Type: stageInstance.StageType, Label: stageInstance.Label}
	}
	return c.notifyWorkflowPendingApprovers(ctx, instance, template, applicant, stage, recipients)
}

// compensateFormApprovalWorkflowStartFailure closes committed projections before returning a Temporal start error.
func (c WorkflowService) compensateFormApprovalWorkflowStartFailure(ctx RequestContext, instance domain.FormInstance, startErr error) error {
	compensationContext, cancel := context.WithTimeout(context.WithoutCancel(goContext(ctx)), 5*time.Second)
	defer cancel()
	compensationRequestContext := ctx
	compensationRequestContext.Context = compensationContext
	compensationErr := c.withTransaction(compensationRequestContext, func(tx WorkflowService) error {
		attendance := tx.Service.Attendance()
		if _, ok, err := attendance.store.GetLeaveRequestByFormInstanceID(compensationContext, ctx.TenantID, instance.ID); err != nil {
			return err
		} else if ok {
			if err := attendance.applyLeaveWorkflowReview(compensationRequestContext, instance, "cancel", "cancelled"); err != nil {
				return err
			}
		}
		if _, ok, err := attendance.store.GetOvertimeRequestByFormInstanceID(compensationContext, ctx.TenantID, instance.ID); err != nil {
			return err
		} else if ok {
			if err := attendance.applyOvertimeWorkflowReview(compensationRequestContext, instance, "cancel", "cancelled"); err != nil {
				return err
			}
		}
		return tx.markFormApprovalWorkflowStartFailedInTransaction(compensationRequestContext, instance, startErr)
	})
	if compensationErr != nil {
		return errors.Join(startErr, compensationErr)
	}
	return startErr
}

// markFormApprovalWorkflowStartFailedInTransaction persists start-failure state inside the caller's transaction.
func (c WorkflowService) markFormApprovalWorkflowStartFailedInTransaction(ctx RequestContext, instance domain.FormInstance, startErr error) error {
	current, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, instance.ID)
	if err != nil {
		return err
	}
	if !ok {
		return NotFound("form instance", instance.ID)
	}
	now := c.Now()
	current.Status = domain.WorkflowFormStatusWorkflowStartFailed
	current.UpdatedAt = now
	if err := c.store.UpsertFormInstance(goContext(ctx), current); err != nil {
		return err
	}
	if run, ok, err := c.store.GetWorkflowRunByFormInstance(goContext(ctx), ctx.TenantID, instance.ID); err != nil {
		return err
	} else if ok {
		run.Status = domain.WorkflowRunStatusStartFailed
		run.TemporalStartStatus = domain.WorkflowTemporalStartAbandoned
		run.TemporalStartedAt = nil
		run.UpdatedAt = now
		if err := c.store.UpsertWorkflowRun(goContext(ctx), run); err != nil {
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
	return c.audit(ctx, "workflow.form.temporal_start_failed", string(ResourceFormInstance), current.ID, string(SeverityHigh), details)
}

func (c WorkflowService) signalTemporalFormApprovalWorkflow(ctx RequestContext, id, action, expectedStatus, reason string) (domain.FormInstance, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.FormInstance{}, BadRequest("id is required")
	}
	action = strings.TrimSpace(strings.ToLower(action))
	switch action {
	case domain.FormApprovalWorkflowActionApprove, domain.FormApprovalWorkflowActionReject, domain.FormApprovalWorkflowActionReturn:
		if _, _, err := c.RequireWorkflowAuthz(ctx, ResourceFormInstance, ActionRead, ""); err != nil {
			return domain.FormInstance{}, err
		}
	default:
		if _, _, err := c.RequireWorkflowAuthz(ctx, ResourceFormInstance, ActionUpdate, ""); err != nil {
			return domain.FormInstance{}, err
		}
	}
	before, err := c.LoadTemporalFormApprovalProjection(ctx, id)
	if err != nil {
		return domain.FormInstance{}, err
	}
	run, runOK, err := c.store.GetWorkflowRunByFormInstance(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.FormInstance{}, err
	}
	if !runOK {
		return domain.FormInstance{}, NotFound("workflow run", id)
	}
	if run.TemporalStartStatus == domain.WorkflowTemporalStartPending || run.TemporalStartStatus == domain.WorkflowTemporalStartStarting {
		return domain.FormInstance{}, Conflict("workflow is still starting").WithReasonCode("workflow_start_pending")
	}
	if run.TemporalStartStatus == domain.WorkflowTemporalStartAbandoned {
		return domain.FormInstance{}, Conflict("workflow start was abandoned").WithReasonCode("workflow_start_abandoned")
	}
	idempotencyKey := strings.TrimSpace(ctx.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = strings.TrimSpace(ctx.RequestID)
	}
	fingerprint := workflowCommandFingerprint(run.ID, ctx.AccountID, action, reason)
	if idempotencyKey != "" {
		if existing, ok, lookupErr := c.store.GetWorkflowActionByIdempotencyKey(goContext(ctx), ctx.TenantID, run.ID, idempotencyKey); lookupErr != nil {
			return domain.FormInstance{}, lookupErr
		} else if ok {
			if existing.CommandFingerprint != fingerprint {
				return domain.FormInstance{}, Conflict("idempotency key was already used for a different workflow command").WithReasonCode("idempotency_key_reused")
			}
			current, found, loadErr := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, id)
			if loadErr != nil {
				return domain.FormInstance{}, loadErr
			}
			if !found {
				return domain.FormInstance{}, NotFound("form instance", id)
			}
			return current, nil
		}
	}
	if action == domain.FormApprovalWorkflowActionApprove || action == domain.FormApprovalWorkflowActionReject || action == domain.FormApprovalWorkflowActionReturn {
		if _, _, _, _, _, err := c.loadActiveWorkflowStageForAssignee(ctx, id); err != nil {
			return domain.FormInstance{}, err
		}
	}
	if c.formApprovalWorkflows == nil {
		return domain.FormInstance{}, domain.E(503, "temporal_workflow_unavailable", "temporal form approval workflow client is required").WithReasonCode("temporal_workflow_unavailable")
	}
	signal := domain.FormApprovalWorkflowSignal{
		TenantID:           ctx.TenantID,
		FormInstanceID:     id,
		RunID:              run.ID,
		WorkflowID:         run.TemporalWorkflowID,
		AccountID:          ctx.AccountID,
		Action:             action,
		Reason:             strings.TrimSpace(reason),
		RequestID:          ctx.RequestID,
		TraceID:            ctx.TraceID,
		IdempotencyKey:     idempotencyKey,
		CommandFingerprint: fingerprint,
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
	ctx.IdempotencyKey = strings.TrimSpace(signal.IdempotencyKey)
	action := strings.TrimSpace(strings.ToLower(signal.Action))
	activeRun, activeRunFound, err := c.store.GetWorkflowRunByFormInstance(goContext(ctx), ctx.TenantID, signal.FormInstanceID)
	if err != nil {
		return domain.FormApprovalProjection{}, err
	}
	if !activeRunFound {
		return domain.FormApprovalProjection{}, NotFound("workflow run", signal.FormInstanceID)
	}
	// A delayed signal for an older resubmission must never mutate the latest
	// run merely because both executions share the same form instance ID.
	if signalRunID := strings.TrimSpace(signal.RunID); signalRunID != "" && signalRunID != activeRun.ID {
		return c.LoadTemporalFormApprovalProjection(ctx, signal.FormInstanceID)
	}
	if signalWorkflowID := strings.TrimSpace(signal.WorkflowID); signalWorkflowID != "" && activeRun.TemporalWorkflowID != "" && signalWorkflowID != activeRun.TemporalWorkflowID {
		return c.LoadTemporalFormApprovalProjection(ctx, signal.FormInstanceID)
	}
	if ctx.IdempotencyKey != "" {
		runID := activeRun.ID
		if runID != "" {
			existing, found, err := c.store.GetWorkflowActionByIdempotencyKey(goContext(ctx), ctx.TenantID, runID, ctx.IdempotencyKey)
			if err != nil {
				return domain.FormApprovalProjection{}, err
			}
			if found {
				expected := workflowCommandFingerprint(runID, signal.AccountID, action, signal.Reason)
				if existing.CommandFingerprint != expected {
					return domain.FormApprovalProjection{}, Conflict("idempotency key was already used for a different workflow command").WithReasonCode("idempotency_key_reused")
				}
				return c.LoadTemporalFormApprovalProjection(ctx, signal.FormInstanceID)
			}
		}
	}
	var instance domain.FormInstance
	var actionErr error
	switch action {
	case domain.FormApprovalWorkflowActionApprove:
		instance, actionErr = c.actOnWorkflowStage(ctx, signal.FormInstanceID, "approve", signal.Reason, true)
	case domain.FormApprovalWorkflowActionReject:
		instance, actionErr = c.actOnWorkflowStage(ctx, signal.FormInstanceID, "reject", signal.Reason, true)
	case domain.FormApprovalWorkflowActionReturn:
		instance, actionErr = c.actOnWorkflowStage(ctx, signal.FormInstanceID, "return", signal.Reason, true)
	case domain.FormApprovalWorkflowActionWithdraw:
		instance, actionErr = c.withdrawTemporalFormApproval(ctx, signal.FormInstanceID, signal.Reason)
	default:
		return c.LoadTemporalFormApprovalProjection(ctx, signal.FormInstanceID)
	}
	if actionErr != nil {
		return domain.FormApprovalProjection{}, actionErr
	}
	return c.LoadTemporalFormApprovalProjection(ctx, instance.ID)
}

func (c WorkflowService) auditTemporalWorkflowAction(ctx RequestContext, instance domain.FormInstance, action, reason, severity string) error {
	return c.audit(ctx, "workflow.form."+action, string(ResourceFormInstance), instance.ID, severity, map[string]any{
		"template_id":      instance.TemplateID,
		"reviewed_by":      ctx.AccountID,
		"review_action":    action,
		"review_comment":   strings.TrimSpace(reason),
		"resulting_status": instance.Status,
		"temporal":         true,
	})
}

func workflowCommandFingerprint(runID, accountID, action, reason string) string {
	canonical := strings.Join([]string{
		strings.TrimSpace(runID),
		strings.TrimSpace(accountID),
		strings.ToLower(strings.TrimSpace(action)),
		strings.TrimSpace(reason),
	}, "\x00")
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(canonical)))
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
	account, decision, err := c.RequireWorkflowAuthz(ctx, ResourceFormInstance, ActionUpdate, "")
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
		return tx.auditTemporalWorkflowAction(ctx, current, domain.FormApprovalWorkflowActionWithdraw, reason, string(SeverityMedium))
	}); err != nil {
		return domain.FormInstance{}, err
	}
	return instance, nil
}

func (c WorkflowService) cancelPendingTemporalFormApproval(ctx RequestContext, formInstanceID, reason string) (domain.FormInstance, error) {
	var cancelled domain.FormInstance
	err := c.withTransaction(ctx, func(tx WorkflowService) error {
		current, ok, err := tx.store.GetFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("form instance", formInstanceID)
		}
		run, ok, err := tx.store.GetWorkflowRunByFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("workflow run", formInstanceID)
		}
		if run.TemporalStartStatus != domain.WorkflowTemporalStartPending {
			return Conflict("workflow start completed while cancellation was being requested").WithReasonCode("workflow_start_raced")
		}
		now := tx.Now()
		run, abandoned, err := tx.store.AbandonPendingWorkflowRunTemporalStart(goContext(ctx), ctx.TenantID, run.ID, now)
		if err != nil {
			return err
		}
		if !abandoned {
			return Conflict("workflow start completed while cancellation was being requested").WithReasonCode("workflow_start_raced")
		}
		if run.CurrentStageInstanceID != "" {
			stage, found, err := tx.store.GetWorkflowStageInstance(goContext(ctx), ctx.TenantID, run.CurrentStageInstanceID)
			if err != nil {
				return err
			}
			if found {
				if err := tx.recordWorkflowAction(ctx, run, stage, domain.FormApprovalWorkflowActionWithdraw, reason, now); err != nil {
					return err
				}
				stage.Status = domain.WorkflowStageStatusSkipped
				stage.CompletedAt = &now
				if err := tx.store.UpsertWorkflowStageInstance(goContext(ctx), stage); err != nil {
					return err
				}
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
		cancelled = current
		return tx.audit(ctx, "workflow.form.cancel_pending_start", string(ResourceFormInstance), current.ID, string(SeverityMedium), map[string]any{
			"run_id":                run.ID,
			"temporal_start_status": run.TemporalStartStatus,
		})
	})
	return cancelled, err
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
