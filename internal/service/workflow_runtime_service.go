package service

import (
	"strconv"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

// StartWorkflowRun 依 template stages 啟動流程運行。
func (c WorkflowService) StartWorkflowRun(ctx RequestContext, instance domain.FormInstance, template domain.FormTemplate, applicant domain.Account) error {
	return c.withTransaction(ctx, func(tx WorkflowService) error {
		_, err := tx.initWorkflowRun(ctx, instance, template, applicant)
		return err
	})
}

func (c WorkflowService) initWorkflowRun(ctx RequestContext, instance domain.FormInstance, template domain.FormTemplate, applicant domain.Account) (domain.FormInstance, error) {
	stages := ParseWorkflowStagesFromTemplate(template)
	if len(stages) == 0 {
		return domain.FormInstance{}, BadRequest("form template has no workflow stages")
	}
	version := 1
	if runs, err := c.store.ListWorkflowRunsByFormInstance(goContext(ctx), ctx.TenantID, instance.ID); err != nil {
		return domain.FormInstance{}, err
	} else if len(runs) > 0 {
		version = runs[len(runs)-1].Version + 1
	}
	now := c.Now()
	temporalStartStatus := domain.WorkflowTemporalStartStarted
	temporalWorkflowID := domain.FormApprovalWorkflowID(ctx.TenantID, instance.ID)
	temporalStartEventID := ""
	var temporalStartedAt *time.Time
	if c.workflowStartOutboxEnabled {
		temporalStartStatus = domain.WorkflowTemporalStartPending
		temporalWorkflowID = ""
		temporalStartEventID = utils.NewID("outbox")
	} else {
		temporalStartedAt = &now
	}
	run := domain.WorkflowRun{
		ID:                   utils.NewID("wfr"),
		TenantID:             ctx.TenantID,
		FormInstanceID:       instance.ID,
		TemplateID:           template.ID,
		Version:              version,
		Status:               domain.WorkflowRunStatusRunning,
		StageDefinitionsJSON: SerializeWorkflowStages(stages),
		TemporalStartStatus:  temporalStartStatus,
		TemporalWorkflowID:   temporalWorkflowID,
		TemporalStartEventID: temporalStartEventID,
		TemporalStartedAt:    temporalStartedAt,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if c.workflowStartOutboxEnabled {
		// The run ID is part of the Temporal identity so each resubmission is isolated.
		run.TemporalWorkflowID = domain.FormApprovalWorkflowIDForRun(ctx.TenantID, instance.ID, run.ID)
	}
	// 呼叫端在同一交易內剛寫入過此表單,重讀以取得最新 version 供樂觀鎖檢查。
	if current, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, instance.ID); err != nil {
		return domain.FormInstance{}, err
	} else if ok {
		instance = current
	}
	instance.Status = domain.WorkflowFormStatusInReview
	instance.CurrentRunID = run.ID
	instance.UpdatedAt = now
	if err := c.store.UpsertWorkflowRun(goContext(ctx), run); err != nil {
		return domain.FormInstance{}, err
	}
	if err := c.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
		return domain.FormInstance{}, err
	}
	if err := c.advanceWorkflowAt(ctx, run, stages, stages[0].ID, applicant, instance.Payload, ""); err != nil {
		return domain.FormInstance{}, err
	}
	if c.workflowStartOutboxEnabled {
		if err := c.store.AppendOutboxEvent(goContext(ctx), workflowStartOutboxEvent(run, now)); err != nil {
			return domain.FormInstance{}, err
		}
	}
	updated, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, instance.ID)
	if err != nil {
		return domain.FormInstance{}, err
	}
	if !ok {
		return domain.FormInstance{}, NotFound("form instance", instance.ID)
	}
	return updated, nil
}

// ActOnWorkflowStage 對當前流程節點執行審批動作。
func (c WorkflowService) ActOnWorkflowStage(ctx RequestContext, formInstanceID, action, comment string) (domain.FormInstance, error) {
	return c.actOnWorkflowStage(ctx, formInstanceID, action, comment, false)
}

// actOnWorkflowStage keeps the Temporal command receipt, projection mutation,
// and its audit record in one transaction when temporalAudit is enabled.
func (c WorkflowService) actOnWorkflowStage(ctx RequestContext, formInstanceID, action, comment string, temporalAudit bool) (domain.FormInstance, error) {
	action = strings.TrimSpace(strings.ToLower(action))
	if action == "" {
		return domain.FormInstance{}, BadRequest("action is required")
	}
	if _, _, err := c.RequireWorkflowAuthz(ctx, ResourceFormInstance, ActionRead, ""); err != nil {
		return domain.FormInstance{}, err
	}
	latestRun, runOK, err := c.store.GetWorkflowRunByFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
	if err != nil {
		return domain.FormInstance{}, err
	}
	if !runOK {
		return domain.FormInstance{}, NotFound("workflow run", formInstanceID)
	}
	if latestRun.TemporalStartStatus == domain.WorkflowTemporalStartPending || latestRun.TemporalStartStatus == domain.WorkflowTemporalStartStarting {
		return domain.FormInstance{}, Conflict("workflow is still starting").WithReasonCode("workflow_start_pending")
	}
	if latestRun.TemporalStartStatus == domain.WorkflowTemporalStartAbandoned {
		return domain.FormInstance{}, Conflict("workflow start was abandoned").WithReasonCode("workflow_start_abandoned")
	}
	if key := strings.TrimSpace(ctx.IdempotencyKey); key != "" {
		fingerprint := workflowCommandFingerprint(latestRun.ID, ctx.AccountID, action, comment)
		existing, found, lookupErr := c.store.GetWorkflowActionByIdempotencyKey(goContext(ctx), ctx.TenantID, latestRun.ID, key)
		if lookupErr != nil {
			return domain.FormInstance{}, lookupErr
		}
		if found {
			if existing.CommandFingerprint != fingerprint {
				return domain.FormInstance{}, Conflict("idempotency key was already used for a different workflow command").WithReasonCode("idempotency_key_reused")
			}
			current, ok, loadErr := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
			if loadErr != nil {
				return domain.FormInstance{}, loadErr
			}
			if !ok {
				return domain.FormInstance{}, NotFound("form instance", formInstanceID)
			}
			return current, nil
		}
	}
	stageInstanceID := strings.TrimSpace(latestRun.CurrentStageInstanceID)
	if stageInstanceID == "" {
		return domain.FormInstance{}, Conflict("form instance has no active workflow stage").WithReasonCode("workflow_stage_unavailable")
	}
	now := c.Now()
	err = c.withTransaction(ctx, func(tx WorkflowService) error {
		// Every decision on a stage serializes on the same row. After acquiring
		// the lock, reload all mutable workflow state so an earlier decision can
		// never be replayed from the stale pre-transaction snapshot.
		stageInstance, ok, err := tx.store.GetWorkflowStageInstanceForUpdate(goContext(ctx), ctx.TenantID, stageInstanceID)
		if err != nil {
			return err
		}
		if key := strings.TrimSpace(ctx.IdempotencyKey); key != "" {
			fingerprint := workflowCommandFingerprint(latestRun.ID, ctx.AccountID, action, comment)
			existing, found, lookupErr := tx.store.GetWorkflowActionByIdempotencyKey(goContext(ctx), ctx.TenantID, latestRun.ID, key)
			if lookupErr != nil {
				return lookupErr
			}
			if found {
				if existing.CommandFingerprint != fingerprint {
					return Conflict("idempotency key was already used for a different workflow command").WithReasonCode("idempotency_key_reused")
				}
				return nil
			}
		}
		if !ok || stageInstance.Status != domain.WorkflowStageStatusActive {
			return Conflict("workflow stage is no longer active").WithReasonCode("workflow_stage_unavailable")
		}
		run, ok, err := tx.store.GetWorkflowRun(goContext(ctx), ctx.TenantID, latestRun.ID)
		if err != nil {
			return err
		}
		if !ok || run.Status != domain.WorkflowRunStatusRunning || run.CurrentStageInstanceID != stageInstance.ID {
			return Conflict("workflow stage is no longer current").WithReasonCode("workflow_stage_unavailable")
		}
		instance, ok, err := tx.store.GetFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("form instance", formInstanceID)
		}
		if instance.CurrentRunID != run.ID {
			return Conflict("workflow run is no longer current").WithReasonCode("workflow_stage_unavailable")
		}
		stages := DeserializeWorkflowStages(run.StageDefinitionsJSON)
		stageDefinition := workflowStageByID(stages, stageInstance.StageID)
		assignees, err := tx.store.ListWorkflowStageAssignees(goContext(ctx), ctx.TenantID, stageInstance.ID)
		if err != nil {
			return err
		}
		if !workflowAssigneeCanAct(assignees, ctx.AccountID) {
			return Forbidden("current account is not an active assignee for this stage").WithReasonCode("workflow_not_assignee")
		}
		if err := tx.validateWorkflowActorEligibility(ctx, instance, run, stageDefinition, action); err != nil {
			return err
		}
		if err := tx.recordWorkflowAction(ctx, run, stageInstance, action, comment, now); err != nil {
			return err
		}
		var mutationErr error
		switch action {
		case "approve":
			mutationErr = tx.handleWorkflowApprove(ctx, instance, run, stageInstance, stages, assignees, comment, now)
		case "reject":
			mutationErr = tx.completeWorkflowDecision(ctx, instance, run, stageInstance, workflowFormStatusRejected, domain.WorkflowRunStatusCompleted, "reject", comment, now)
		case "return":
			mutationErr = tx.completeWorkflowDecision(ctx, instance, run, stageInstance, domain.WorkflowFormStatusReturned, domain.WorkflowRunStatusReturned, "return", comment, now)
		default:
			return BadRequest("unsupported workflow action: " + action)
		}
		if mutationErr != nil {
			return mutationErr
		}
		if !temporalAudit {
			return nil
		}
		result, ok, err := tx.store.GetFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("form instance", formInstanceID)
		}
		return tx.auditTemporalWorkflowAction(ctx, result, action, comment, string(SeverityHigh))
	})
	if err != nil {
		return domain.FormInstance{}, err
	}
	updated, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
	if err != nil {
		return domain.FormInstance{}, err
	}
	if !ok {
		return domain.FormInstance{}, NotFound("form instance", formInstanceID)
	}
	return updated, nil
}

// loadActiveWorkflowStageForAssignee combines the baseline read grant with the runtime assignee relation.
func (c WorkflowService) loadActiveWorkflowStageForAssignee(ctx RequestContext, formInstanceID string) (domain.FormInstance, domain.WorkflowRun, domain.WorkflowStageInstance, []domain.WorkflowStageDefinition, []domain.WorkflowStageAssignee, error) {
	if _, _, err := c.RequireWorkflowAuthz(ctx, ResourceFormInstance, ActionRead, ""); err != nil {
		return domain.FormInstance{}, domain.WorkflowRun{}, domain.WorkflowStageInstance{}, nil, nil, err
	}
	instance, run, stageInstance, stages, err := c.loadActiveWorkflowStage(ctx, formInstanceID)
	if err != nil {
		return domain.FormInstance{}, domain.WorkflowRun{}, domain.WorkflowStageInstance{}, nil, nil, err
	}
	assignees, err := c.store.ListWorkflowStageAssignees(goContext(ctx), ctx.TenantID, stageInstance.ID)
	if err != nil {
		return domain.FormInstance{}, domain.WorkflowRun{}, domain.WorkflowStageInstance{}, nil, nil, err
	}
	if !workflowAssigneeCanAct(assignees, ctx.AccountID) {
		return domain.FormInstance{}, domain.WorkflowRun{}, domain.WorkflowStageInstance{}, nil, nil, Forbidden("current account is not an active assignee for this stage").WithReasonCode("workflow_not_assignee")
	}
	return instance, run, stageInstance, stages, assignees, nil
}

// GetWorkflowFormState 回傳單據流程運行狀態。
func (c WorkflowService) GetWorkflowFormState(ctx RequestContext, formInstanceID string) (domain.WorkflowFormStateResponse, error) {
	account, decision, err := c.RequireWorkflowAuthz(ctx, ResourceFormInstance, ActionRead, "")
	if err != nil {
		return domain.WorkflowFormStateResponse{}, err
	}
	instance, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
	if err != nil {
		return domain.WorkflowFormStateResponse{}, err
	}
	if !ok {
		return domain.WorkflowFormStateResponse{}, NotFound("form instance", formInstanceID)
	}
	if err := requireFormInstanceVisible(instance, account, decision); err != nil {
		pending, pendingErr := workflowFormInstancePendingForAccount(ctx, c.store, ctx.TenantID, account.ID)
		if pendingErr != nil {
			return domain.WorkflowFormStateResponse{}, pendingErr
		}
		if _, assigned := pending[instance.ID]; !assigned {
			return domain.WorkflowFormStateResponse{}, err
		}
	}
	run, ok, err := c.store.GetWorkflowRunByFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
	if err != nil {
		return domain.WorkflowFormStateResponse{}, err
	}
	if !ok {
		return domain.WorkflowFormStateResponse{
			FormInstanceID: formInstanceID,
			FormStatus:     instance.Status,
			Steps:          []domain.WorkflowFormStep{},
		}, nil
	}
	stages := DeserializeWorkflowStages(run.StageDefinitionsJSON)
	stageInstances, err := c.store.ListWorkflowStageInstancesByRun(goContext(ctx), ctx.TenantID, run.ID)
	if err != nil {
		return domain.WorkflowFormStateResponse{}, err
	}
	actions, err := c.store.ListWorkflowActionsByRun(goContext(ctx), ctx.TenantID, run.ID)
	if err != nil {
		return domain.WorkflowFormStateResponse{}, err
	}
	steps := make([]domain.WorkflowFormStep, 0, len(stages))
	stageInstanceByStageID := map[string]domain.WorkflowStageInstance{}
	for _, item := range stageInstances {
		stageInstanceByStageID[item.StageID] = item
	}
	for _, stage := range stages {
		step := domain.WorkflowFormStep{
			StageID: stage.ID,
			Label:   stage.Label,
			Detail:  stage.Detail,
			State:   workflowStepState(stage.ID, stageInstanceByStageID[stage.ID], run),
		}
		if stageInstance, ok := stageInstanceByStageID[stage.ID]; ok {
			assignees, err := c.store.ListWorkflowStageAssignees(goContext(ctx), ctx.TenantID, stageInstance.ID)
			if err != nil {
				return domain.WorkflowFormStateResponse{}, err
			}
			for _, assignee := range assignees {
				account, found, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, assignee.AccountID)
				if err != nil {
					return domain.WorkflowFormStateResponse{}, err
				}
				name := assignee.AccountID
				if found {
					name = workflowAccountLabel(account)
				}
				step.Assignees = append(step.Assignees, domain.WorkflowFormStepAssignee{
					AccountID: assignee.AccountID,
					Name:      name,
					Status:    assignee.Status,
				})
			}
		}
		steps = append(steps, step)
	}
	currentStageID := ""
	currentStageLabel := ""
	canAct := false
	allowed := []string{}
	if run.Status == domain.WorkflowRunStatusRunning && run.CurrentStageInstanceID != "" {
		current, ok, err := c.store.GetWorkflowStageInstance(goContext(ctx), ctx.TenantID, run.CurrentStageInstanceID)
		if err != nil {
			return domain.WorkflowFormStateResponse{}, err
		}
		if ok {
			currentStageID = current.StageID
			currentStageLabel = current.Label
			assignees, err := c.store.ListWorkflowStageAssignees(goContext(ctx), ctx.TenantID, current.ID)
			if err != nil {
				return domain.WorkflowFormStateResponse{}, err
			}
			if run.TemporalStartStatus == domain.WorkflowTemporalStartStarted && workflowAssigneeCanAct(assignees, ctx.AccountID) && current.StageType == "approver" {
				canAct = true
				allowed = []string{"approve", "reject", "return"}
			}
		}
	}
	reviewLog := make([]domain.WorkflowReviewLogItem, 0, len(actions))
	for _, item := range actions {
		account, found, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, item.AccountID)
		if err != nil {
			return domain.WorkflowFormStateResponse{}, err
		}
		name := item.AccountID
		if found {
			name = workflowAccountLabel(account)
		}
		reviewLog = append(reviewLog, domain.WorkflowReviewLogItem{
			Type:    item.Action,
			Name:    name,
			Time:    platformTime(item.CreatedAt),
			Comment: item.Comment,
		})
	}
	return domain.WorkflowFormStateResponse{
		FormInstanceID:      formInstanceID,
		FormStatus:          instance.Status,
		RunID:               run.ID,
		RunStatus:           run.Status,
		TemporalStartStatus: run.TemporalStartStatus,
		TemporalWorkflowID:  run.TemporalWorkflowID,
		TemporalRunID:       run.TemporalRunID,
		TemporalStartedAt:   run.TemporalStartedAt,
		CurrentStageID:      currentStageID,
		CurrentStageLabel:   currentStageLabel,
		CanAct:              canAct,
		AllowedActions:      allowed,
		Steps:               steps,
		Actions:             reviewLog,
	}, nil
}

func (c WorkflowService) loadActiveWorkflowStage(ctx RequestContext, formInstanceID string) (domain.FormInstance, domain.WorkflowRun, domain.WorkflowStageInstance, []domain.WorkflowStageDefinition, error) {
	instance, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
	if err != nil {
		return domain.FormInstance{}, domain.WorkflowRun{}, domain.WorkflowStageInstance{}, nil, err
	}
	if !ok {
		return domain.FormInstance{}, domain.WorkflowRun{}, domain.WorkflowStageInstance{}, nil, NotFound("form instance", formInstanceID)
	}
	run, ok, err := c.store.GetWorkflowRunByFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
	if err != nil {
		return domain.FormInstance{}, domain.WorkflowRun{}, domain.WorkflowStageInstance{}, nil, err
	}
	if !ok || run.Status != domain.WorkflowRunStatusRunning || run.CurrentStageInstanceID == "" {
		return domain.FormInstance{}, domain.WorkflowRun{}, domain.WorkflowStageInstance{}, nil, BadRequest("form instance has no active workflow stage").WithReasonCode("workflow_stage_unavailable")
	}
	if run.TemporalStartStatus == domain.WorkflowTemporalStartPending || run.TemporalStartStatus == domain.WorkflowTemporalStartStarting {
		return domain.FormInstance{}, domain.WorkflowRun{}, domain.WorkflowStageInstance{}, nil, Conflict("workflow is still starting").WithReasonCode("workflow_start_pending")
	}
	if run.TemporalStartStatus == domain.WorkflowTemporalStartAbandoned {
		return domain.FormInstance{}, domain.WorkflowRun{}, domain.WorkflowStageInstance{}, nil, Conflict("workflow start was abandoned").WithReasonCode("workflow_start_abandoned")
	}
	stageInstance, ok, err := c.store.GetWorkflowStageInstance(goContext(ctx), ctx.TenantID, run.CurrentStageInstanceID)
	if err != nil {
		return domain.FormInstance{}, domain.WorkflowRun{}, domain.WorkflowStageInstance{}, nil, err
	}
	if !ok || stageInstance.Status != domain.WorkflowStageStatusActive {
		return domain.FormInstance{}, domain.WorkflowRun{}, domain.WorkflowStageInstance{}, nil, BadRequest("workflow stage is not active").WithReasonCode("workflow_stage_unavailable")
	}
	return instance, run, stageInstance, DeserializeWorkflowStages(run.StageDefinitionsJSON), nil
}

func (c WorkflowService) advanceWorkflowFrom(ctx RequestContext, run domain.WorkflowRun, stages []domain.WorkflowStageDefinition, startIndex int, applicant domain.Account, payload map[string]any, approvalComment string) error {
	if startIndex < 0 || startIndex >= len(stages) {
		return c.completeWorkflowApproved(ctx, run, applicant, payload, approvalComment)
	}
	return c.advanceWorkflowAt(ctx, run, stages, stages[startIndex].ID, applicant, payload, approvalComment)
}

func (c WorkflowService) advanceWorkflowAt(ctx RequestContext, run domain.WorkflowRun, stages []domain.WorkflowStageDefinition, stageID string, applicant domain.Account, payload map[string]any, approvalComment string) error {
	stageID = strings.TrimSpace(stageID)
	for hop := 0; hop < len(stages)+2; hop++ {
		if stageID == "" || stageID == domain.WorkflowStageEndID {
			return c.completeWorkflowApproved(ctx, run, applicant, payload, approvalComment)
		}
		stage := workflowStageByID(stages, stageID)
		if strings.TrimSpace(stage.ID) == "" {
			return BadRequest("workflow stage not found: " + stageID).WithReasonCode("workflow_stage_unavailable")
		}
		sequence := workflowStageIndexByID(stages, stage.ID)
		if sequence < 0 {
			sequence = hop
		}
		switch strings.TrimSpace(stage.Type) {
		case "notify":
			if err := c.completeAutomaticStage(ctx, run, stage, sequence, "notify", applicant, payload); err != nil {
				return err
			}
			stageID = strings.TrimSpace(stage.Config.NextStageID)
			if stageID == "" {
				stageID = domain.WorkflowStageEndID
			}
			continue
		case "condition":
			nextID, err := c.completeConditionStage(ctx, run, stage, sequence, applicant, payload, stages)
			if err != nil {
				return err
			}
			stageID = nextID
			continue
		case "approver":
			return c.activateApprovalStage(ctx, run, stage, sequence, applicant, payload)
		default:
			return BadRequest("unsupported workflow stage type: " + stage.Type)
		}
	}
	return BadRequest("workflow graph appears to cycle").WithReasonCode("workflow_stage_unavailable")
}

func (c WorkflowService) completeAutomaticStage(ctx RequestContext, run domain.WorkflowRun, stage domain.WorkflowStageDefinition, sequence int, action string, applicant domain.Account, payload map[string]any) error {
	now := c.Now()
	stageInstance := domain.WorkflowStageInstance{
		ID:          utils.NewID("wfs"),
		TenantID:    ctx.TenantID,
		RunID:       run.ID,
		StageID:     stage.ID,
		StageType:   stage.Type,
		Label:       stage.Label,
		Status:      domain.WorkflowStageStatusCompleted,
		Sequence:    sequence,
		StartedAt:   &now,
		CompletedAt: &now,
	}
	assigneeIDs, err := c.resolveWorkflowAssignees(ctx, applicant, stage, payload)
	if err != nil {
		return err
	}
	if stage.Type == "notify" && len(assigneeIDs) > 0 {
		instance, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, run.FormInstanceID)
		if err != nil {
			return err
		}
		if ok {
			template, templateOK, err := c.store.GetFormTemplate(goContext(ctx), ctx.TenantID, run.TemplateID)
			if err != nil {
				return err
			}
			if !templateOK {
				template = domain.FormTemplate{ID: run.TemplateID}
			}
			title := workflowNotificationTemplateTitle(template, instance)
			body := workflowAccountLabel(applicant) + " 提交了「" + title + "」，請知悉。"
			_ = c.deliverWorkflowNotification(ctx, domain.Notification{
				ID:                 workflowNotificationID("notify-"+stage.ID, run.FormInstanceID),
				TenantID:           ctx.TenantID,
				Tone:               "info",
				Category:           "workflow",
				Title:              "知會：" + title,
				Body:               body,
				StatusText:         "知會",
				LinkURL:            "/notifications?reviewId=" + run.FormInstanceID,
				SourceType:         "workflow.form.notify",
				SourceID:           run.FormInstanceID + ":" + stage.ID,
				CreatedByAccountID: applicant.ID,
				CreatedAt:          now,
			}, assigneeIDs)
		}
	}
	return c.persistAutomaticStage(ctx, run, stageInstance, assigneeIDs, action, now)
}

func (c WorkflowService) persistAutomaticStage(ctx RequestContext, run domain.WorkflowRun, stageInstance domain.WorkflowStageInstance, assigneeIDs []string, action string, now time.Time) error {
	if err := c.store.UpsertWorkflowStageInstance(goContext(ctx), stageInstance); err != nil {
		return err
	}
	for _, accountID := range assigneeIDs {
		if err := c.store.UpsertWorkflowStageAssignee(goContext(ctx), domain.WorkflowStageAssignee{
			TenantID:        ctx.TenantID,
			StageInstanceID: stageInstance.ID,
			AccountID:       accountID,
			Status:          domain.WorkflowAssigneeStatusApproved,
		}); err != nil {
			return err
		}
	}
	return c.recordWorkflowAction(ctx, run, stageInstance, action, "", now)
}

func (c WorkflowService) completeConditionStage(ctx RequestContext, run domain.WorkflowRun, stage domain.WorkflowStageDefinition, sequence int, applicant domain.Account, payload map[string]any, stages []domain.WorkflowStageDefinition) (string, error) {
	employee, err := c.resolveWorkflowApplicantEmployee(ctx, applicant)
	if err != nil {
		return "", err
	}
	now := c.Now()
	matched := evaluateWorkflowCondition(stage.Config, employee, payload)
	targetStageID := strings.TrimSpace(stage.Config.FalseNextStageID)
	if matched {
		targetStageID = strings.TrimSpace(stage.Config.TrueNextStageID)
	}
	if targetStageID == "" {
		return "", BadRequest("condition stage is missing routing targets: " + stage.ID).WithReasonCode("workflow_stage_unavailable")
	}
	if targetStageID != domain.WorkflowStageEndID && workflowStageByID(stages, targetStageID).ID == "" {
		return "", BadRequest("condition stage routes to unknown stage: " + targetStageID).WithReasonCode("workflow_stage_unavailable")
	}
	stageInstance := domain.WorkflowStageInstance{
		ID:        utils.NewID("wfs"),
		TenantID:  ctx.TenantID,
		RunID:     run.ID,
		StageID:   stage.ID,
		StageType: stage.Type,
		Label:     stage.Label,
		Status:    domain.WorkflowStageStatusCompleted,
		Sequence:  sequence,
		Result: map[string]any{
			"matched":         matched,
			"target_stage_id": targetStageID,
		},
		StartedAt:   &now,
		CompletedAt: &now,
	}
	if err := c.store.UpsertWorkflowStageInstance(goContext(ctx), stageInstance); err != nil {
		return "", err
	}
	if err := c.recordWorkflowAction(ctx, run, stageInstance, "auto_condition", "", now); err != nil {
		return "", err
	}
	return targetStageID, nil
}

func (c WorkflowService) activateApprovalStage(ctx RequestContext, run domain.WorkflowRun, stage domain.WorkflowStageDefinition, sequence int, applicant domain.Account, payload map[string]any) error {
	assigneeIDs, err := c.resolveWorkflowAssignees(ctx, applicant, stage, payload)
	if err != nil {
		return err
	}
	if stage.Config.RequireDistinctApprover {
		assigneeIDs, err = c.filterPreviouslyApprovedAssignees(ctx, run.ID, assigneeIDs)
		if err != nil {
			return err
		}
	}
	if len(assigneeIDs) == 0 {
		return BadRequest("workflow stage has no resolvable assignees: " + stage.Label).WithReasonCode("workflow_stage_unavailable")
	}
	now := c.Now()
	stageInstance := domain.WorkflowStageInstance{
		ID:        utils.NewID("wfs"),
		TenantID:  ctx.TenantID,
		RunID:     run.ID,
		StageID:   stage.ID,
		StageType: stage.Type,
		Label:     stage.Label,
		Status:    domain.WorkflowStageStatusActive,
		Sequence:  sequence,
		StartedAt: &now,
	}
	run.CurrentStageInstanceID = stageInstance.ID
	run.UpdatedAt = now
	if err := c.store.UpsertWorkflowStageInstance(goContext(ctx), stageInstance); err != nil {
		return err
	}
	for _, accountID := range assigneeIDs {
		if err := c.store.UpsertWorkflowStageAssignee(goContext(ctx), domain.WorkflowStageAssignee{
			TenantID:        ctx.TenantID,
			StageInstanceID: stageInstance.ID,
			AccountID:       accountID,
			Status:          domain.WorkflowAssigneeStatusPending,
		}); err != nil {
			return err
		}
	}
	if err := c.store.UpsertWorkflowRun(goContext(ctx), run); err != nil {
		return err
	}
	instance, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, run.FormInstanceID)
	if err != nil {
		return err
	}
	if !ok {
		return NotFound("form instance", run.FormInstanceID)
	}
	template, templateOK, err := c.store.GetFormTemplate(goContext(ctx), ctx.TenantID, run.TemplateID)
	if err != nil {
		return err
	}
	if !templateOK {
		template = domain.FormTemplate{ID: run.TemplateID}
	}
	if run.TemporalStartStatus != domain.WorkflowTemporalStartStarted {
		return nil
	}
	return c.notifyWorkflowPendingApprovers(ctx, instance, template, applicant, stage, assigneeIDs)
}

// filterPreviouslyApprovedAssignees prevents all-mode stages from waiting on ineligible prior reviewers.
func (c WorkflowService) filterPreviouslyApprovedAssignees(ctx RequestContext, runID string, accountIDs []string) ([]string, error) {
	actions, err := c.store.ListWorkflowActionsByRun(goContext(ctx), ctx.TenantID, runID)
	if err != nil {
		return nil, err
	}
	previousApprovers := map[string]struct{}{}
	for _, action := range actions {
		if action.Action == "approve" {
			previousApprovers[action.AccountID] = struct{}{}
		}
	}
	filtered := make([]string, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		if _, alreadyApproved := previousApprovers[accountID]; !alreadyApproved {
			filtered = append(filtered, accountID)
		}
	}
	return uniqueWorkflowRecipientIDs(filtered), nil
}

func (c WorkflowService) handleWorkflowApprove(ctx RequestContext, instance domain.FormInstance, run domain.WorkflowRun, stageInstance domain.WorkflowStageInstance, stages []domain.WorkflowStageDefinition, assignees []domain.WorkflowStageAssignee, comment string, now time.Time) error {
	for index, assignee := range assignees {
		if assignee.AccountID != ctx.AccountID {
			continue
		}
		assignees[index].Status = domain.WorkflowAssigneeStatusApproved
		if err := c.store.UpsertWorkflowStageAssignee(goContext(ctx), assignees[index]); err != nil {
			return err
		}
	}
	stageDef := workflowStageByID(stages, stageInstance.StageID)
	mode := strings.TrimSpace(stageDef.Config.Mode)
	if mode == "all" {
		for _, assignee := range assignees {
			if assignee.Status != domain.WorkflowAssigneeStatusApproved {
				return nil
			}
		}
	}
	stageInstance.Status = domain.WorkflowStageStatusCompleted
	stageInstance.CompletedAt = &now
	if err := c.store.UpsertWorkflowStageInstance(goContext(ctx), stageInstance); err != nil {
		return err
	}
	run.CurrentStageInstanceID = ""
	run.UpdatedAt = now
	if err := c.store.UpsertWorkflowRun(goContext(ctx), run); err != nil {
		return err
	}
	applicant, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, instance.ApplicantAccountID)
	if err != nil {
		return err
	}
	if !ok {
		return NotFound("account", instance.ApplicantAccountID)
	}
	nextID := strings.TrimSpace(stageDef.Config.NextStageID)
	if nextID == "" {
		nextID = domain.WorkflowStageEndID
	}
	return c.advanceWorkflowAt(ctx, run, stages, nextID, applicant, instance.Payload, comment)
}

func (c WorkflowService) completeWorkflowDecision(ctx RequestContext, instance domain.FormInstance, run domain.WorkflowRun, stageInstance domain.WorkflowStageInstance, formStatus, runStatus, action, comment string, now time.Time) error {
	assignees, err := c.store.ListWorkflowStageAssignees(goContext(ctx), ctx.TenantID, stageInstance.ID)
	if err != nil {
		return err
	}
	for _, assignee := range assignees {
		if assignee.AccountID == ctx.AccountID {
			if action == "return" {
				assignee.Status = domain.WorkflowAssigneeStatusReturned
			} else {
				assignee.Status = domain.WorkflowAssigneeStatusRejected
			}
			if err := c.store.UpsertWorkflowStageAssignee(goContext(ctx), assignee); err != nil {
				return err
			}
		}
	}
	stageInstance.Status = domain.WorkflowStageStatusRejected
	stageInstance.CompletedAt = &now
	run.Status = runStatus
	run.CurrentStageInstanceID = ""
	run.UpdatedAt = now
	instance.Status = formStatus
	instance.ApprovedBy = ctx.AccountID
	instance.Payload = withWorkflowReview(instance.Payload, action, ctx.AccountID, comment, now)
	instance.UpdatedAt = now
	template, templateOK, err := c.store.GetFormTemplate(goContext(ctx), ctx.TenantID, instance.TemplateID)
	if err != nil {
		return err
	}
	if !templateOK {
		template = domain.FormTemplate{ID: instance.TemplateID}
	}
	reviewer, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, ctx.AccountID)
	if err != nil {
		return err
	}
	if !ok {
		reviewer = domain.Account{ID: ctx.AccountID}
	}
	if err := c.store.UpsertWorkflowStageInstance(goContext(ctx), stageInstance); err != nil {
		return err
	}
	if err := c.store.UpsertWorkflowRun(goContext(ctx), run); err != nil {
		return err
	}
	if err := c.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
		return err
	}
	if err := c.Service.Attendance().applyAttendanceWorkflowReview(ctx, instance, action, formStatus); err != nil {
		return err
	}
	return c.notifyWorkflowFormReviewed(ctx, instance, template, reviewer, action, comment)
}

func (c WorkflowService) completeWorkflowApproved(ctx RequestContext, run domain.WorkflowRun, applicant domain.Account, payload map[string]any, comment string) error {
	instance, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, run.FormInstanceID)
	if err != nil {
		return err
	}
	if !ok {
		return NotFound("form instance", run.FormInstanceID)
	}
	now := c.Now()
	run.Status = domain.WorkflowRunStatusCompleted
	run.CurrentStageInstanceID = ""
	run.UpdatedAt = now
	instance.Status = workflowFormStatusApproved
	instance.ApprovedBy = ctx.AccountID
	reason := strings.TrimSpace(comment)
	instance.Payload = withWorkflowReview(instance.Payload, "approve", ctx.AccountID, reason, now)
	instance.UpdatedAt = now
	if err := c.store.UpsertWorkflowRun(goContext(ctx), run); err != nil {
		return err
	}
	if err := c.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
		return err
	}
	template, templateOK, err := c.store.GetFormTemplate(goContext(ctx), ctx.TenantID, run.TemplateID)
	if err != nil {
		return err
	}
	if !templateOK {
		template = domain.FormTemplate{ID: run.TemplateID}
	}
	if err := c.Service.Attendance().applyAttendanceWorkflowReview(ctx, instance, "approve", workflowFormStatusApproved); err != nil {
		return err
	}
	reviewer, reviewerOK, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, ctx.AccountID)
	if err != nil {
		return err
	}
	if !reviewerOK {
		reviewer = domain.Account{ID: ctx.AccountID}
	}
	return c.notifyWorkflowFormReviewed(ctx, instance, template, reviewer, "approve", reason)
}

func (c WorkflowService) recordWorkflowAction(ctx RequestContext, run domain.WorkflowRun, stageInstance domain.WorkflowStageInstance, action, comment string, at time.Time) error {
	accountID := strings.TrimSpace(ctx.AccountID)
	if accountID == "" && (action == "notify" || action == "auto_condition") {
		accountID = "system"
	}
	idempotencyKey := ""
	commandFingerprint := ""
	switch strings.ToLower(strings.TrimSpace(action)) {
	case domain.FormApprovalWorkflowActionApprove, domain.FormApprovalWorkflowActionReject, domain.FormApprovalWorkflowActionReturn, domain.FormApprovalWorkflowActionWithdraw:
		idempotencyKey = strings.TrimSpace(ctx.IdempotencyKey)
		if idempotencyKey != "" {
			commandFingerprint = workflowCommandFingerprint(run.ID, accountID, action, comment)
		}
	}
	return c.store.InsertWorkflowAction(goContext(ctx), domain.WorkflowAction{
		ID:                 utils.NewID("wfa"),
		TenantID:           ctx.TenantID,
		RunID:              run.ID,
		StageInstanceID:    stageInstance.ID,
		AccountID:          accountID,
		Action:             action,
		Comment:            strings.TrimSpace(comment),
		IdempotencyKey:     idempotencyKey,
		CommandFingerprint: commandFingerprint,
		RequestID:          strings.TrimSpace(ctx.RequestID),
		TraceID:            strings.TrimSpace(ctx.TraceID),
		CreatedAt:          at,
	})
}

func (c WorkflowService) notifyWorkflowPendingApprovers(ctx RequestContext, instance domain.FormInstance, template domain.FormTemplate, applicant domain.Account, stage domain.WorkflowStageDefinition, assigneeIDs []string) error {
	title := workflowNotificationTemplateTitle(template, instance)
	body := workflowAccountLabel(applicant) + " 提交了「" + title + "」，目前在「" + stage.Label + "」待您審核。"
	return c.deliverWorkflowNotification(ctx, domain.Notification{
		ID:                 workflowNotificationID("pending-"+stage.ID, instance.ID),
		TenantID:           ctx.TenantID,
		Tone:               "warning",
		Category:           "workflow",
		Title:              "待審核：" + title,
		Body:               body,
		StatusText:         "待處理",
		LinkURL:            "/notifications?reviewId=" + instance.ID,
		SourceType:         "workflow.form.pending",
		SourceID:           instance.ID + ":" + stage.ID,
		CreatedByAccountID: applicant.ID,
		CreatedAt:          c.Now(),
	}, assigneeIDs)
}

func (c WorkflowService) resolveWorkflowAssignees(ctx RequestContext, applicant domain.Account, stage domain.WorkflowStageDefinition, payload map[string]any) ([]string, error) {
	if len(stage.Config.AccountIDs) > 0 {
		return uniqueWorkflowRecipientIDs(stage.Config.AccountIDs), nil
	}
	// Migration: reject legacy group targeting so admins must reconfigure templates with approval roles.
	if len(stage.Config.UserGroupIDs) > 0 {
		return nil, BadRequest("workflow user_group_ids targeting is retired; assign an approval role instead").WithReasonCode("workflow_user_group_retired")
	}
	employee, err := c.resolveWorkflowApplicantEmployee(ctx, applicant)
	if err != nil {
		return nil, err
	}
	role := strings.TrimSpace(stage.Config.Role)
	if role == "" {
		role = "manager"
	}
	switch role {
	case "applicant":
		return []string{applicant.ID}, nil
	case "manager":
		managerID, err := c.effectiveManagerEmployeeID(ctx, employee, 1)
		if err != nil {
			return nil, err
		}
		return c.accountIDsForEmployeeIDs(ctx, []string{managerID})
	case "relative":
		level := stage.Config.RelativeLevel
		if level <= 0 {
			level = 1
		}
		managerID, err := c.effectiveManagerEmployeeID(ctx, employee, level)
		if err != nil {
			return nil, err
		}
		if managerID == "" {
			return nil, BadRequest("unable to resolve relative approver")
		}
		return c.accountIDsForEmployeeIDs(ctx, []string{managerID})
	case "dept-head":
		return c.resolveRoleAssignees(ctx, employee, []string{"主管", "部門主管", "head"})
	case "hr":
		return c.resolveRoleAssignees(ctx, employee, []string{"hr", "human resource", "人資"})
	case "finance":
		return c.resolveRoleAssignees(ctx, employee, []string{"finance", "財務"})
	case "ceo":
		return c.resolveRoleAssignees(ctx, employee, []string{"ceo", "總經理", "general manager"})
	default:
		managerID, err := c.effectiveManagerEmployeeID(ctx, employee, 1)
		if err != nil {
			return nil, err
		}
		return c.accountIDsForEmployeeIDs(ctx, []string{managerID})
	}
}

func (c WorkflowService) effectiveManagerEmployeeID(ctx RequestContext, employee Employee, levels int) (string, error) {
	employees, err := c.Service.listBusinessEmployees(ctx)
	if err != nil {
		return "", err
	}
	units, err := c.Service.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return "", err
	}
	resolved := ResolveEffectiveManagerChain(employee, employees, units, c.Now(), levels)
	return strings.TrimSpace(resolved.ManagerEmployeeID), nil
}

// validateWorkflowActorEligibility rechecks segregation-of-duties before every action.
// Legacy user_group_ids stages no longer revalidate membership; in-flight assignee snapshots remain authoritative.
func (c WorkflowService) validateWorkflowActorEligibility(ctx RequestContext, instance domain.FormInstance, run domain.WorkflowRun, stage domain.WorkflowStageDefinition, action string) error {
	if action != "approve" || !stage.Config.RequireDistinctApprover {
		return nil
	}
	actions, err := c.store.ListWorkflowActionsByRun(goContext(ctx), ctx.TenantID, run.ID)
	if err != nil {
		return err
	}
	for _, prior := range actions {
		if prior.AccountID == ctx.AccountID && prior.Action == "approve" && prior.StageInstanceID != run.CurrentStageInstanceID {
			return Forbidden("current account already approved an earlier workflow stage").WithReasonCode("workflow_not_assignee")
		}
	}
	return nil
}

func (c WorkflowService) resolveWorkflowApplicantEmployee(ctx RequestContext, applicant domain.Account) (domain.Employee, error) {
	if strings.TrimSpace(applicant.EmployeeID) == "" {
		return domain.Employee{}, BadRequest("applicant employee is required for workflow routing")
	}
	employee, ok, err := c.Service.getBusinessEmployee(ctx, applicant.EmployeeID)
	if err != nil {
		return domain.Employee{}, err
	}
	if !ok {
		return domain.Employee{}, NotFound("employee", applicant.EmployeeID)
	}
	return employee, nil
}

func (c WorkflowService) accountIDsForEmployeeIDs(ctx RequestContext, employeeIDs []string) ([]string, error) {
	out := make([]string, 0, len(employeeIDs))
	for _, employeeID := range uniqueWorkflowRecipientIDs(employeeIDs) {
		employee, ok, err := c.Service.getBusinessEmployee(ctx, employeeID)
		if err != nil {
			return nil, err
		}
		if !ok || strings.TrimSpace(employee.AccountID) == "" {
			continue
		}
		out = append(out, employee.AccountID)
	}
	return uniqueWorkflowRecipientIDs(out), nil
}

func (c WorkflowService) resolveRoleAssignees(ctx RequestContext, applicant domain.Employee, keywords []string) ([]string, error) {
	employees, err := c.Service.listBusinessEmployees(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0)
	for _, employee := range employees {
		position := strings.ToLower(strings.TrimSpace(employee.Position))
		for _, keyword := range keywords {
			if strings.Contains(position, strings.ToLower(keyword)) {
				if employee.AccountID != "" {
					out = append(out, employee.AccountID)
				}
				break
			}
		}
	}
	if len(out) == 0 && applicant.ManagerEmployeeID != "" {
		return c.accountIDsForEmployeeIDs(ctx, []string{applicant.ManagerEmployeeID})
	}
	return uniqueWorkflowRecipientIDs(out), nil
}

func evaluateWorkflowCondition(config domain.WorkflowStageConfig, applicant domain.Employee, payload map[string]any) bool {
	field := strings.TrimSpace(config.Field)
	if field == "" {
		field = "hours"
	}
	switch field {
	case "level":
		levelText := strings.TrimSpace(applicant.Position)
		for _, level := range config.Levels {
			if strings.Contains(levelText, strconv.Itoa(level)) {
				return true
			}
		}
		return false
	case "amount":
		left := workflowPayloadNumber(payload, "amount", "total_amount", "totalAmount")
		right, err := strconv.ParseFloat(strings.TrimSpace(config.Value), 64)
		if err != nil {
			return false
		}
		return compareWorkflowNumbers(left, right, config.Operator)
	default:
		left := workflowPayloadNumber(payload, "hours", "leave_hours", "leaveHours")
		right, err := strconv.ParseFloat(strings.TrimSpace(config.Value), 64)
		if err != nil {
			return false
		}
		return compareWorkflowNumbers(left, right, config.Operator)
	}
}

func workflowPayloadNumber(payload map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			switch typed := value.(type) {
			case float64:
				return typed
			case float32:
				return float64(typed)
			case int:
				return float64(typed)
			case int64:
				return float64(typed)
			case string:
				parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
				if err == nil {
					return parsed
				}
			}
		}
	}
	return 0
}

func compareWorkflowNumbers(left, right float64, operator string) bool {
	switch strings.TrimSpace(operator) {
	case "<":
		return left < right
	case "<=":
		return left <= right
	case "==":
		return left == right
	case ">":
		return left > right
	default:
		return left >= right
	}
}

func workflowAssigneeCanAct(assignees []domain.WorkflowStageAssignee, accountID string) bool {
	for _, assignee := range assignees {
		if assignee.AccountID == accountID && assignee.Status == domain.WorkflowAssigneeStatusPending {
			return true
		}
	}
	return false
}

func workflowStepState(stageID string, stageInstance domain.WorkflowStageInstance, run domain.WorkflowRun) string {
	if stageInstance.ID == "" {
		return "pending"
	}
	switch stageInstance.Status {
	case domain.WorkflowStageStatusCompleted:
		return "completed"
	case domain.WorkflowStageStatusActive:
		if run.CurrentStageInstanceID == stageInstance.ID {
			return "current"
		}
		return "pending"
	case domain.WorkflowStageStatusRejected:
		return "rejected"
	default:
		return "pending"
	}
}

func workflowStageByID(stages []domain.WorkflowStageDefinition, stageID string) domain.WorkflowStageDefinition {
	for _, stage := range stages {
		if stage.ID == stageID {
			return stage
		}
	}
	return domain.WorkflowStageDefinition{}
}

func workflowStageIndexByID(stages []domain.WorkflowStageDefinition, stageID string) int {
	for index, stage := range stages {
		if stage.ID == stageID {
			return index
		}
	}
	return -1
}

func workflowNextStageID(stages []domain.WorkflowStageDefinition, currentID string) string {
	for index, stage := range stages {
		if stage.ID == currentID && index+1 < len(stages) {
			return stages[index+1].ID
		}
	}
	return ""
}

func workflowSkipToNextApproverStageID(stages []domain.WorkflowStageDefinition, currentID string) string {
	for index, stage := range stages {
		if stage.ID != currentID {
			continue
		}
		for offset := index + 1; offset < len(stages); offset++ {
			if strings.TrimSpace(stages[offset].Type) == "approver" {
				return stages[offset].ID
			}
		}
	}
	return ""
}

func workflowFormInstancePendingForAccount(ctx RequestContext, store workflowStore, tenantID, accountID string) (map[string]struct{}, error) {
	stageIDs, err := store.ListPendingAssigneeStageInstanceIDs(goContext(ctx), tenantID, accountID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(stageIDs))
	for _, stageInstanceID := range stageIDs {
		stageInstance, ok, err := store.GetWorkflowStageInstance(goContext(ctx), tenantID, stageInstanceID)
		if err != nil {
			return nil, err
		}
		if !ok || stageInstance.Status != domain.WorkflowStageStatusActive {
			continue
		}
		run, ok, err := store.GetWorkflowRun(goContext(ctx), tenantID, stageInstance.RunID)
		if err != nil {
			return nil, err
		}
		if !ok || run.Status != domain.WorkflowRunStatusRunning || run.TemporalStartStatus != domain.WorkflowTemporalStartStarted {
			continue
		}
		out[run.FormInstanceID] = struct{}{}
	}
	return out, nil
}
