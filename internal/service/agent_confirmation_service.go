package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

const (
	agentConfirmationTTL           = 10 * time.Minute
	agentConfirmationSettleTimeout = 5 * time.Second
	agentBulkReviewLimit           = 20
	agentConfirmationFormSubmit    = "form_submit"
	agentConfirmationBulkReview    = "workflow_bulk_review"
	agentConfirmationExternalTool  = "external_tool_call"
	agentConfirmationStatusDone    = "completed"
	agentConfirmationStatusPartial = "partial"
	AgentConfirmationMemoryKey     = "__agent_confirmation__"
)

type agentConfirmationExpectedReview struct {
	FormInstanceID      string `json:"form_instance_id"`
	FormInstanceVersion int64  `json:"form_instance_version"`
	WorkflowRunID       string `json:"workflow_run_id"`
	StageInstanceID     string `json:"stage_instance_id"`
}

type agentConfirmationAction struct {
	Public                 domain.AgentConfirmation          `json:"-"`
	DraftID                string                            `json:"draft_id,omitempty"`
	ExpectedDraftVersion   int64                             `json:"expected_draft_version,omitempty"`
	Payload                map[string]any                    `json:"payload,omitempty"`
	ReviewAction           string                            `json:"review_action,omitempty"`
	ReviewReason           string                            `json:"review_reason,omitempty"`
	Reviews                []agentConfirmationExpectedReview `json:"reviews,omitempty"`
	ExternalConnectionID   string                            `json:"connection_id,omitempty"`
	ExternalCapabilityID   string                            `json:"capability_id,omitempty"`
	ExternalSchemaChecksum string                            `json:"schema_checksum,omitempty"`
	ExternalArguments      map[string]any                    `json:"arguments,omitempty"`
}

// agentConfirmationStore is the confirmation-only subset of repository.AgentV2Store.
// Keeping it structural lets the legacy repository.Store remain unchanged during the v2 cutover.
type agentConfirmationStore interface {
	UpsertAgentConfirmation(context.Context, domain.AgentConfirmationRecord) error
	ListPendingAgentConfirmations(ctx context.Context, tenantID, accountID, conversationID, segmentID string, now time.Time) ([]domain.AgentConfirmationRecord, error)
	ClaimAgentConfirmation(ctx context.Context, tenantID, accountID, id string, now time.Time) (domain.AgentConfirmationRecord, bool, error)
	UpdateAgentConfirmation(ctx context.Context, confirmation domain.AgentConfirmationRecord) (domain.AgentConfirmationRecord, bool, error)
}

// ExecuteConfirmation 在重新驗證資源版本與操作者後執行一次性 Agent 操作。
func (c *Service) ExecuteAgentConfirmation(ctx RequestContext, id string, _ domain.ExecuteAgentConfirmationInput) (domain.AgentConfirmationExecution, error) {
	if _, _, err := c.requireServiceAuthz(ctx, AppAgent, ResourceType("run"), ActionCreate, ""); err != nil {
		return domain.AgentConfirmationExecution{}, err
	}
	action, record, err := c.takeAgentConfirmation(ctx, id)
	if err != nil {
		return domain.AgentConfirmationExecution{}, err
	}

	var result domain.AgentConfirmationExecution
	switch action.Public.Kind {
	case agentConfirmationFormSubmit:
		result, err = c.executeConfirmedFormSubmission(ctx, action)
	case agentConfirmationBulkReview:
		result, err = c.executeConfirmedBulkReview(ctx, action)
	case agentConfirmationExternalTool:
		result, err = c.executeConfirmedExternalToolCall(ctx, action, record)
	default:
		err = BadRequest("unsupported agent confirmation kind")
	}
	if err != nil {
		c.settleFailedAgentConfirmation(ctx, record, err)
		return domain.AgentConfirmationExecution{}, err
	}
	if err := c.completeAgentConfirmation(ctx, record, result); err != nil {
		return domain.AgentConfirmationExecution{}, err
	}
	auditDetails := map[string]any{
		"kind":   action.Public.Kind,
		"action": action.Public.Action,
		"count":  len(action.Reviews),
	}
	if action.Public.Kind == agentConfirmationExternalTool {
		auditDetails["connection_id"] = action.ExternalConnectionID
		auditDetails["capability_id"] = action.ExternalCapabilityID
	}
	if err := c.RecordAudit(ctx, "agent.confirmation.execute", "agent_confirmation", action.Public.ID, "high", auditDetails); err != nil {
		return domain.AgentConfirmationExecution{}, err
	}
	return result, nil
}

// settleFailedAgentConfirmation moves a claimed record to pending only for retryable failures.
// Cancellation, expiry, and deterministic failures are terminal and cannot be replayed.
func (c *Service) settleFailedAgentConfirmation(ctx RequestContext, record domain.AgentConfirmationRecord, executionErr error) {
	store, ok := c.agentConfirmationPersistence()
	if !ok {
		c.LogWarn(ctx, "agent confirmation settlement unavailable", "confirmation_id", record.ID)
		return
	}
	now := c.Now()
	record.LastError = agentConfirmationStoredError(executionErr)
	record.UpdatedAt = now
	record.ResultPayload = map[string]any{}
	switch {
	case !record.ExpiresAt.After(now):
		record.Status = domain.AgentConfirmationStatusExpired
		record.ConsumedAt = &now
	case errors.Is(executionErr, context.Canceled):
		record.Status = domain.AgentConfirmationStatusCancelled
		record.ConsumedAt = &now
	case agentConfirmationExecutionRetryable(executionErr):
		record.Status = domain.AgentConfirmationStatusPending
		record.ConsumedAt = nil
	default:
		record.Status = domain.AgentConfirmationStatusFailed
		record.ConsumedAt = &now
	}
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(goContext(ctx)), agentConfirmationSettleTimeout)
	defer cancel()
	if _, updated, err := store.UpdateAgentConfirmation(settleCtx, record); err != nil || !updated {
		c.LogWarn(ctx, "agent confirmation settlement failed",
			"confirmation_id", record.ID,
			"status", record.Status,
			"updated", updated,
			"error_type", fmt.Sprintf("%T", err),
		)
	}
}

// agentConfirmationExecutionRetryable 僅接受明確的伺服器端、逾時或暫時性基礎設施錯誤。
func agentConfirmationExecutionRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errAgentConfirmationNonRetryable) {
		return false
	}
	if appErr, ok := domain.AsAppError(err); ok {
		return appErr.Status >= 500
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return true
	}
	var temporary interface{ Temporary() bool }
	return errors.As(err, &temporary) && temporary.Temporary()
}

// saveAgentConfirmation persists display and protected action payloads in the dedicated confirmation store.
func (c *Service) saveAgentConfirmation(ctx RequestContext, action agentConfirmationAction) error {
	execution, ok := AgentChatExecutionContextFromContext(ctx.Context)
	if !ok || strings.TrimSpace(execution.SessionID) == "" || strings.TrimSpace(execution.SegmentID) == "" {
		return domain.E(500, "agent_confirmation_context_missing", "agent confirmation context is unavailable")
	}
	store, ok := c.agentConfirmationPersistence()
	if !ok {
		return domain.E(500, "agent_confirmation_store_unavailable", "agent confirmation storage is unavailable")
	}
	publicPayload, err := agentConfirmationJSONMap(action.Public)
	if err != nil {
		return err
	}
	actionPayload, err := agentConfirmationActionPayload(action)
	if err != nil {
		return err
	}
	now := c.Now()
	return store.UpsertAgentConfirmation(goContext(ctx), domain.AgentConfirmationRecord{
		ID: action.Public.ID, TenantID: ctx.TenantID, AccountID: ctx.AccountID,
		ConversationID: execution.SessionID, SegmentID: execution.SegmentID,
		ExecutionID: execution.RunID, SourceMessageID: execution.InputMessageID,
		Kind: action.Public.Kind, Title: action.Public.Title, Action: action.Public.Action,
		PublicPayload: publicPayload, ActionPayload: actionPayload, ResultPayload: map[string]any{},
		Status: domain.AgentConfirmationStatusPending, ExpiresAt: action.Public.ExpiresAt,
		CreatedAt: now, UpdatedAt: now,
	})
}

func agentConfirmationActionPayload(action agentConfirmationAction) (map[string]any, error) {
	if action.Public.Kind != agentConfirmationExternalTool {
		return agentConfirmationJSONMap(action)
	}
	arguments, err := agentConfirmationJSONMap(action.ExternalArguments)
	if err != nil {
		return nil, err
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	return map[string]any{
		"connection_id":   action.ExternalConnectionID,
		"capability_id":   action.ExternalCapabilityID,
		"schema_checksum": action.ExternalSchemaChecksum,
		"arguments":       arguments,
	}, nil
}

// takeAgentConfirmation atomically claims a current-segment record and reconstructs the legacy public DTO.
func (c *Service) takeAgentConfirmation(ctx RequestContext, id string) (agentConfirmationAction, domain.AgentConfirmationRecord, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return agentConfirmationAction{}, domain.AgentConfirmationRecord{}, NotFound("agent confirmation", id)
	}
	store, ok := c.agentConfirmationPersistence()
	if !ok {
		return agentConfirmationAction{}, domain.AgentConfirmationRecord{}, domain.E(500, "agent_confirmation_store_unavailable", "agent confirmation storage is unavailable")
	}
	record, claimed, err := store.ClaimAgentConfirmation(goContext(ctx), ctx.TenantID, ctx.AccountID, id, c.Now())
	if err != nil {
		return agentConfirmationAction{}, domain.AgentConfirmationRecord{}, err
	}
	if !claimed {
		return agentConfirmationAction{}, domain.AgentConfirmationRecord{}, Conflict("agent confirmation is invalid or was already used").WithReasonCode("agent_confirmation_invalid")
	}
	if record.Status == domain.AgentConfirmationStatusExpired {
		return agentConfirmationAction{}, record, Conflict("agent confirmation has expired").WithReasonCode("agent_confirmation_expired")
	}
	if record.Status != domain.AgentConfirmationStatusExecuting {
		return agentConfirmationAction{}, record, Conflict("agent confirmation is invalid").WithReasonCode("agent_confirmation_invalid")
	}
	action, err := decodeAgentConfirmationRecord(record)
	if err != nil {
		c.settleFailedAgentConfirmation(ctx, record, err)
		return agentConfirmationAction{}, record, Conflict("agent confirmation is invalid").WithReasonCode("agent_confirmation_invalid")
	}
	return action, record, nil
}

// PendingAgentConfirmationMessages restores only unexpired, unconsumed confirmations owned by this session context.
func (c *Service) PendingAgentConfirmationMessages(ctx RequestContext, accountID string, session domain.AgentSession) ([]domain.AgentSessionMessage, error) {
	if strings.TrimSpace(session.ID) == "" || strings.TrimSpace(session.SegmentID) == "" {
		return []domain.AgentSessionMessage{}, nil
	}
	store, ok := c.agentConfirmationPersistence()
	if !ok {
		// During the v2 cutover legacy/in-memory stores may not implement AgentV2Store.
		return []domain.AgentSessionMessage{}, nil
	}
	records, err := store.ListPendingAgentConfirmations(
		goContext(ctx), ctx.TenantID, accountID, session.ID, session.SegmentID, c.Now(),
	)
	if err != nil {
		return nil, err
	}
	messages := make([]domain.AgentSessionMessage, 0, len(records))
	for _, record := range records {
		action, err := decodeAgentConfirmationRecord(record)
		if err != nil {
			continue
		}
		data := map[string]any{}
		if action.DraftID != "" {
			data["form_draft_id"] = action.DraftID
		}
		metadata, err := domain.EncodeAgentArtifactMetadata(map[string]any{
			"event":        domain.AgentChatEventConfirmation,
			"confirmation": action.Public,
			"data":         data,
		})
		if err != nil {
			return nil, err
		}
		messages = append(messages, domain.AgentSessionMessage{
			ID:             "pending-" + action.Public.ID,
			TenantID:       ctx.TenantID,
			SessionID:      session.ID,
			SegmentID:      session.SegmentID,
			Role:           domain.AgentMessageRoleTool,
			RunID:          record.ExecutionID,
			ContextVersion: session.ContextVersion,
			Metadata:       metadata,
			CreatedAt:      record.CreatedAt,
		})
	}
	return messages, nil
}

func (c *Service) completeAgentConfirmation(ctx RequestContext, record domain.AgentConfirmationRecord, result domain.AgentConfirmationExecution) error {
	store, ok := c.agentConfirmationPersistence()
	if !ok {
		return domain.E(500, "agent_confirmation_store_unavailable", "agent confirmation storage is unavailable")
	}
	resultPayload, err := agentConfirmationJSONMap(result)
	if err != nil {
		return err
	}
	now := c.Now()
	record.Status = domain.AgentConfirmationStatusCompleted
	record.ResultPayload = resultPayload
	record.LastError = ""
	record.ConsumedAt = &now
	record.UpdatedAt = now
	// The protected action has already succeeded. Persist its terminal state even
	// when the client disconnects immediately after the side effect commits.
	completeCtx, cancel := context.WithTimeout(context.WithoutCancel(goContext(ctx)), agentConfirmationSettleTimeout)
	defer cancel()
	_, updated, err := store.UpdateAgentConfirmation(completeCtx, record)
	if err != nil {
		return err
	}
	if !updated {
		return Conflict("agent confirmation state changed during execution").WithReasonCode("agent_confirmation_invalid")
	}
	return nil
}

func (c *Service) agentConfirmationPersistence() (agentConfirmationStore, bool) {
	store, ok := c.store.(agentConfirmationStore)
	return store, ok
}

func agentConfirmationJSONMap(value any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func decodeAgentConfirmationRecord(record domain.AgentConfirmationRecord) (agentConfirmationAction, error) {
	publicRaw, err := json.Marshal(record.PublicPayload)
	if err != nil {
		return agentConfirmationAction{}, err
	}
	var public domain.AgentConfirmation
	if err := json.Unmarshal(publicRaw, &public); err != nil {
		return agentConfirmationAction{}, err
	}
	actionRaw, err := json.Marshal(record.ActionPayload)
	if err != nil {
		return agentConfirmationAction{}, err
	}
	var action agentConfirmationAction
	if err := json.Unmarshal(actionRaw, &action); err != nil {
		return agentConfirmationAction{}, err
	}
	public.ID = record.ID
	public.Kind = record.Kind
	public.Title = record.Title
	public.Action = record.Action
	public.ExpiresAt = record.ExpiresAt
	action.Public = public
	return action, nil
}

func agentConfirmationStoredError(err error) string {
	if err == nil {
		return ""
	}
	if appErr, ok := domain.AsAppError(err); ok {
		return appErr.Code
	}
	return fmt.Sprintf("%T", err)
}

// executeConfirmedFormSubmission 防止草稿在預覽後被修改或換人提交。
func (c *Service) executeConfirmedFormSubmission(ctx RequestContext, action agentConfirmationAction) (domain.AgentConfirmationExecution, error) {
	current, ok, err := c.Store().GetFormInstance(goContext(ctx), ctx.TenantID, action.DraftID)
	if err != nil {
		return domain.AgentConfirmationExecution{}, err
	}
	if !ok || current.ApplicantAccountID != ctx.AccountID {
		return domain.AgentConfirmationExecution{}, NotFound("form instance", action.DraftID)
	}
	if current.Version != action.ExpectedDraftVersion || !strings.EqualFold(current.Status, WorkflowFormStatusDraft) {
		return domain.AgentConfirmationExecution{}, Conflict("form draft changed after confirmation preview")
	}
	instance, err := c.Workflow().SubmitForm(ctx, domain.SubmitFormInput{TemplateKey: current.ID, Payload: action.Payload})
	if err != nil {
		return domain.AgentConfirmationExecution{}, err
	}
	return domain.AgentConfirmationExecution{
		ConfirmationID: action.Public.ID,
		Kind:           action.Public.Kind,
		Status:         agentConfirmationStatusDone,
		FormInstance:   &instance,
	}, nil
}

// executeConfirmedBulkReview 固定批次與節點版本，任一單據過期時整批不執行。
func (c *Service) executeConfirmedBulkReview(ctx RequestContext, action agentConfirmationAction) (domain.AgentConfirmationExecution, error) {
	ids := make([]string, 0, len(action.Reviews))
	for _, expected := range action.Reviews {
		instance, ok, err := c.Store().GetFormInstance(goContext(ctx), ctx.TenantID, expected.FormInstanceID)
		if err != nil {
			return domain.AgentConfirmationExecution{}, err
		}
		if !ok || instance.Version != expected.FormInstanceVersion || instance.CurrentRunID != expected.WorkflowRunID {
			return domain.AgentConfirmationExecution{}, Conflict("review item changed after confirmation preview: " + expected.FormInstanceID)
		}
		run, ok, err := c.Store().GetWorkflowRunByFormInstance(goContext(ctx), ctx.TenantID, expected.FormInstanceID)
		if err != nil {
			return domain.AgentConfirmationExecution{}, err
		}
		if !ok || run.ID != expected.WorkflowRunID || run.CurrentStageInstanceID != expected.StageInstanceID {
			return domain.AgentConfirmationExecution{}, Conflict("review stage changed after confirmation preview: " + expected.FormInstanceID)
		}
		ids = append(ids, expected.FormInstanceID)
	}
	bulk, err := c.Workflow().BulkReviewForms(ctx, domain.BulkReviewFormsInput{
		FormInstanceIDs: ids,
		Action:          action.ReviewAction,
		Reason:          action.ReviewReason,
	})
	if err != nil {
		return domain.AgentConfirmationExecution{}, err
	}
	status := agentConfirmationStatusDone
	for _, item := range bulk.Results {
		if !item.Success {
			status = agentConfirmationStatusPartial
			break
		}
	}
	return domain.AgentConfirmationExecution{
		ConfirmationID: action.Public.ID,
		Kind:           action.Public.Kind,
		Status:         status,
		BulkReview:     &bulk,
	}, nil
}

// ToolListPublishedFormTemplates 列出 Agent 可用且可提交的已發佈表單。
func (c *Service) ToolListPublishedFormTemplates(ctx domain.RequestContext, _ map[string]any) (map[string]any, error) {
	templates, err := c.Workflow().ListFormTemplates(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(templates))
	for _, template := range templates {
		if !agentTemplatePublished(template) {
			continue
		}
		version, versionErr := c.Workflow().currentFormTemplateVersion(ctx, template)
		if versionErr != nil {
			continue
		}
		publishedTemplate := FormTemplateAtVersion(template, version)
		if ValidateWorkflowTemplateSubmittable(publishedTemplate) != nil {
			continue
		}
		items = append(items, map[string]any{
			"id": template.ID, "key": template.Key, "name": template.Name,
			"description": template.Description, "current_version": version.Version,
		})
	}
	return map[string]any{"items": items, "total": len(items)}, nil
}

// ToolGetPublishedFormTemplate 回傳 Agent 可填欄位、選項資料源與審批路徑。
func (c *Service) ToolGetPublishedFormTemplate(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	template, err := c.agentPublishedFormTemplate(ctx, stringFromAny(args["template_key"]))
	if err != nil {
		return nil, err
	}
	fields := agentAccessibleFormFields(template)
	dataSources, err := c.agentFormDataSources(ctx, fields)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"template": map[string]any{
			"id": template.ID, "key": template.Key, "name": template.Name,
			"description": template.Description, "version": template.CurrentVersion,
			"fields": fields, "approval_path": agentApprovalPath(template),
		},
		"data_sources": dataSources,
	}, nil
}

// ToolCreateFormDraft 建立可撤銷草稿，且只接受 schema 明確允許 Agent 存取的欄位。
func (c *Service) ToolCreateFormDraft(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	template, err := c.agentPublishedFormTemplate(ctx, stringFromAny(args["template_key"]))
	if err != nil {
		return nil, err
	}
	payload, err := toolPayload(args)
	if err != nil {
		return nil, err
	}
	payload, normalizedFields, err := c.prepareAgentFormDraftPayload(ctx, template, payload)
	if err != nil {
		return nil, err
	}
	instance, err := c.Workflow().SaveFormDraft(ctx, domain.SaveFormDraftInput{
		TemplateKey: template.Key,
		Payload:     payload,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"draft":             instance,
		"normalized_fields": normalizedFields,
		"next_step":         "tell the user about any defaulted or recalculated fields, then call preview_form_submission after all required fields are filled",
	}, nil
}

// ToolUpdateFormDraft 更新本人草稿，避免 Agent 覆寫未授權欄位。
func (c *Service) ToolUpdateFormDraft(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	draftID := strings.TrimSpace(stringFromAny(args["draft_id"]))
	if draftID == "" {
		return nil, BadRequest("draft_id is required")
	}
	current, ok, err := c.Store().GetFormInstance(goContext(ctx), ctx.TenantID, draftID)
	if err != nil {
		return nil, err
	}
	if !ok || current.ApplicantAccountID != ctx.AccountID {
		return nil, NotFound("form instance", draftID)
	}
	template, ok, err := c.Store().GetFormTemplate(goContext(ctx), ctx.TenantID, current.TemplateID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, NotFound("form template", current.TemplateID)
	}
	payload, err := toolPayload(args)
	if err != nil {
		return nil, err
	}
	payload, normalizedFields, err := c.prepareAgentFormDraftPayload(ctx, template, payload)
	if err != nil {
		return nil, err
	}
	instance, err := c.Workflow().UpdateFormDraft(ctx, draftID, domain.UpdateFormDraftInput{Payload: payload})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"draft":             instance,
		"normalized_fields": normalizedFields,
		"next_step":         "tell the user about any defaulted or recalculated fields, then call preview_form_submission after all required fields are filled",
	}, nil
}

// ToolPreviewFormSubmission 驗證草稿並產生一次性提交確認，不直接啟動流程。
func (c *Service) ToolPreviewFormSubmission(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	draftID := strings.TrimSpace(stringFromAny(args["draft_id"]))
	if draftID == "" {
		return nil, BadRequest("draft_id is required")
	}
	workflow := c.Workflow()
	account, _, err := workflow.RequireWorkflowAuthz(ctx, ResourceFormInstance, ActionSubmit, "")
	if err != nil {
		return nil, err
	}
	instance, ok, err := c.Store().GetFormInstance(goContext(ctx), ctx.TenantID, draftID)
	if err != nil {
		return nil, err
	}
	if !ok || instance.ApplicantAccountID != account.ID {
		return nil, NotFound("form instance", draftID)
	}
	if !strings.EqualFold(instance.Status, WorkflowFormStatusDraft) {
		return nil, BadRequest("only draft form instances can be submitted")
	}
	template, ok, err := c.Store().GetFormTemplate(goContext(ctx), ctx.TenantID, instance.TemplateID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, NotFound("form template", instance.TemplateID)
	}
	version, err := workflow.FormTemplateVersionForInstance(ctx, template, instance)
	if err != nil {
		return nil, err
	}
	template = FormTemplateAtVersion(template, version)
	if err := ValidateWorkflowTemplateSubmittable(template); err != nil {
		return nil, err
	}
	normalized, err := workflow.validateFormSubmissionPayload(ctx, template, instance.Payload)
	if err != nil {
		return nil, err
	}
	confirmation, err := c.newFormSubmissionConfirmation(ctx, template, instance, normalized)
	if err != nil {
		return nil, err
	}
	return map[string]any{"confirmation": confirmation, "normalized_payload": normalized}, nil
}

// ToolPrepareBulkReview 固定待審單據、動作與當前節點，等待主管明確確認。
func (c *Service) ToolPrepareBulkReview(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	action, _, err := normalizeBulkReviewAction(stringFromAny(args["action"]))
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(stringFromAny(args["reason"]))
	if action != domain.FormApprovalWorkflowActionApprove && reason == "" {
		return nil, BadRequest("reason is required for reject or return")
	}
	queue, err := c.Workflow().ReviewQueue(ctx)
	if err != nil {
		return nil, err
	}
	pending := make(map[string]domain.WorkflowReviewItem, len(queue.PendingReview))
	for _, item := range queue.PendingReview {
		pending[item.ID] = item
	}
	ids := uniqueStrings(stringSliceFromAny(args["form_instance_ids"]))
	if len(ids) == 0 {
		for _, item := range queue.PendingReview {
			ids = append(ids, item.ID)
			if len(ids) == agentBulkReviewLimit {
				break
			}
		}
	}
	if len(ids) == 0 {
		return nil, BadRequest("there are no pending review items")
	}
	if len(ids) > agentBulkReviewLimit {
		return nil, BadRequest(fmt.Sprintf("at most %d review items can be confirmed at once", agentBulkReviewLimit))
	}
	confirmation, err := c.newBulkReviewConfirmation(ctx, pending, ids, action, reason)
	if err != nil {
		return nil, err
	}
	return map[string]any{"confirmation": confirmation, "count": len(ids)}, nil
}

// newFormSubmissionConfirmation 建立與草稿版本綁定的提交確認卡。
func (c *Service) newFormSubmissionConfirmation(ctx domain.RequestContext, template domain.FormTemplate, instance domain.FormInstance, payload map[string]any) (*domain.AgentConfirmation, error) {
	id, err := utils.NewSecretID("aconf")
	if err != nil {
		return nil, err
	}
	rows, err := c.agentPayloadRows(ctx, template, payload)
	if err != nil {
		return nil, err
	}
	confirmation := domain.AgentConfirmation{
		ID: id, Kind: agentConfirmationFormSubmit, Title: "確認提交「" + template.Name + "」",
		Description: "提交後將啟動審批流程；送出前請確認以下資料。",
		Action:      "submit", ActionLabel: "確認並提交",
		Rows: append(rows, domain.AgentAnalysisRow{
			Label: "審批流程", Value: strings.Join(agentApprovalPath(template), " → "),
		}),
		ExpiresAt: c.Now().Add(agentConfirmationTTL),
	}
	if err := c.saveAgentConfirmation(ctx, agentConfirmationAction{
		Public:  confirmation,
		DraftID: instance.ID, ExpectedDraftVersion: instance.Version, Payload: utils.CopyStringMap(payload),
	}); err != nil {
		return nil, err
	}
	return &confirmation, nil
}

// newBulkReviewConfirmation 建立固定 ID 與節點版本的批次審批確認卡。
func (c *Service) newBulkReviewConfirmation(ctx domain.RequestContext, pending map[string]domain.WorkflowReviewItem, ids []string, action, reason string) (*domain.AgentConfirmation, error) {
	id, err := utils.NewSecretID("aconf")
	if err != nil {
		return nil, err
	}
	items := make([]domain.AgentConfirmationItem, 0, len(ids))
	expected := make([]agentConfirmationExpectedReview, 0, len(ids))
	for _, formID := range ids {
		item, ok := pending[formID]
		if !ok {
			return nil, BadRequest("form instance is not pending for current reviewer: " + formID)
		}
		run, ok, err := c.Store().GetWorkflowRunByFormInstance(goContext(ctx), ctx.TenantID, formID)
		if err != nil {
			return nil, err
		}
		if !ok || strings.TrimSpace(run.CurrentStageInstanceID) == "" {
			return nil, Conflict("form instance has no active workflow stage: " + formID)
		}
		items = append(items, domain.AgentConfirmationItem{
			ID: formID, Title: item.Title, Subtitle: item.Who, Status: item.StatusText,
			Rows: []domain.AgentAnalysisRow{{Label: "摘要", Value: item.Desc}, {Label: "申請時間", Value: item.Time}},
		})
		expected = append(expected, agentConfirmationExpectedReview{
			FormInstanceID: formID, FormInstanceVersion: item.Instance.Version,
			WorkflowRunID: run.ID, StageInstanceID: run.CurrentStageInstanceID,
		})
	}
	label := map[string]string{"approve": "批准", "reject": "拒絕", "return": "退回"}[action]
	rows := []domain.AgentAnalysisRow{{Label: "操作", Value: label}, {Label: "單據數量", Value: fmt.Sprintf("%d", len(items))}}
	if reason != "" {
		rows = append(rows, domain.AgentAnalysisRow{Label: "意見", Value: reason})
	}
	confirmation := domain.AgentConfirmation{
		ID: id, Kind: agentConfirmationBulkReview, Title: "確認批量" + label,
		Description: "系統將在執行前重新檢查每筆單據的當前審批人與節點版本。",
		Action:      action, ActionLabel: fmt.Sprintf("確認%s %d 筆", label, len(items)),
		Rows: rows, Items: items, ExpiresAt: c.Now().Add(agentConfirmationTTL),
	}
	if err := c.saveAgentConfirmation(ctx, agentConfirmationAction{
		Public:       confirmation,
		ReviewAction: action, ReviewReason: reason, Reviews: expected,
	}); err != nil {
		return nil, err
	}
	return &confirmation, nil
}

// agentPublishedFormTemplate 以 key 或 ID 尋找可提交的已發佈表單。
func (c *Service) agentPublishedFormTemplate(ctx domain.RequestContext, keyOrID string) (domain.FormTemplate, error) {
	keyOrID = strings.TrimSpace(keyOrID)
	if keyOrID == "" {
		return domain.FormTemplate{}, BadRequest("template_key is required")
	}
	templates, err := c.Workflow().ListFormTemplates(ctx)
	if err != nil {
		return domain.FormTemplate{}, err
	}
	for _, template := range templates {
		if template.Key != keyOrID && template.ID != keyOrID {
			continue
		}
		if !agentTemplatePublished(template) {
			return domain.FormTemplate{}, BadRequest("form template is not published")
		}
		version, err := c.Workflow().currentFormTemplateVersion(ctx, template)
		if err != nil {
			return domain.FormTemplate{}, err
		}
		publishedTemplate := FormTemplateAtVersion(template, version)
		if err := ValidateWorkflowTemplateSubmittable(publishedTemplate); err != nil {
			return domain.FormTemplate{}, err
		}
		return publishedTemplate, nil
	}
	return domain.FormTemplate{}, NotFound("form template", keyOrID)
}

// agentTemplatePublished 相容舊資料空狀態，並拒絕明確未發佈的模板。
func agentTemplatePublished(template domain.FormTemplate) bool {
	status := strings.TrimSpace(strings.ToLower(template.Status))
	return status != "archived" && workspaceFormPublishedVersion(template) > 0
}

// agentAccessibleFormFields 排除佈局欄位與 schema 明確禁止 Agent 存取的欄位。
func agentAccessibleFormFields(template domain.FormTemplate) []domain.PlatformFormBuilderField {
	fields := platformTemplateFields(template.Key, template.Schema)
	out := make([]domain.PlatformFormBuilderField, 0, len(fields))
	for _, field := range fields {
		kind := strings.TrimSpace(strings.ToLower(field.Type))
		if kind == "layout" || kind == "section-title" || strings.TrimSpace(field.ID) == "" {
			continue
		}
		if field.Security != nil && !field.Security.AgentAccess {
			continue
		}
		out = append(out, field)
	}
	return out
}

// sanitizeAgentFormPayload 防止模型注入 schema 外或禁止 Agent 存取的欄位。
func sanitizeAgentFormPayload(template domain.FormTemplate, payload map[string]any) map[string]any {
	allowed := make(map[string]struct{})
	for _, field := range agentAccessibleFormFields(template) {
		allowed[field.ID] = struct{}{}
	}
	out := make(map[string]any, len(allowed))
	for key, value := range payload {
		if _, ok := allowed[key]; ok {
			out[key] = value
		}
	}
	return out
}

// prepareAgentFormDraftPayload 保留欄位白名單，解析資料源標籤，並依租戶考勤政策補齊缺失的請假時間。
func (c *Service) prepareAgentFormDraftPayload(ctx domain.RequestContext, template domain.FormTemplate, payload map[string]any) (map[string]any, []string, error) {
	filtered := sanitizeAgentFormPayload(template, payload)
	filtered, normalizedFields, err := c.normalizeAgentBoundPayloadValues(ctx, template, filtered)
	if err != nil {
		return nil, nil, err
	}
	if _, linked := leaveLinkedTemplateKeys[strings.TrimSpace(template.Key)]; !linked {
		return filtered, normalizedFields, nil
	}

	startRaw := strings.TrimSpace(stringFromAny(filtered["start_at"]))
	endRaw := strings.TrimSpace(stringFromAny(filtered["end_at"]))
	if startRaw != "" && endRaw != "" {
		normalized, err := c.Workflow().normalizeLeaveSubmissionHours(ctx, template.Key, filtered)
		if err != nil {
			return nil, nil, err
		}
		return normalized, append(normalizedFields, "hours"), nil
	}

	workDate := c.Now().In(attendanceClockLocation).Format(time.DateOnly)
	providedRaw := utils.FirstNonEmpty(startRaw, endRaw)
	if providedRaw != "" {
		providedAt, err := utils.ParseDateTime(providedRaw)
		if err != nil {
			return filtered, nil, nil
		}
		workDate = providedAt.In(attendanceClockLocation).Format(time.DateOnly)
	}

	policy, err := c.Attendance().loadAttendancePolicyResponse(ctx)
	if err != nil {
		return nil, nil, err
	}
	schedule, _ := attendanceScheduleIntervals(workDate, policy.WorkTime)
	if len(schedule) == 0 {
		return nil, nil, ValidationFailed("leave time defaulting failed", []domain.FieldError{{
			Field: "start_at", Code: "invalid_work_schedule", Message: "attendance standard work time is invalid",
		}})
	}

	if startRaw == "" {
		filtered["start_at"] = schedule[0].Start.Format(time.RFC3339)
		normalizedFields = append(normalizedFields, "start_at")
	}
	if endRaw == "" {
		filtered["end_at"] = schedule[0].End.Format(time.RFC3339)
		normalizedFields = append(normalizedFields, "end_at")
	}
	filtered, err = c.Workflow().normalizeLeaveSubmissionHours(ctx, template.Key, filtered)
	if err != nil {
		return nil, nil, err
	}
	normalizedFields = append(normalizedFields, "hours")
	return filtered, normalizedFields, nil
}

// normalizeAgentBoundPayloadValues resolves one unambiguous data-source label to its persisted value.
func (c *Service) normalizeAgentBoundPayloadValues(ctx domain.RequestContext, template domain.FormTemplate, payload map[string]any) (map[string]any, []string, error) {
	fields := agentAccessibleFormFields(template)
	sources, err := c.agentFormDataSources(ctx, fields)
	if err != nil {
		return nil, nil, err
	}
	sourceByID := make(map[string]domain.FormDataSource, len(sources))
	for _, source := range sources {
		sourceByID[source.ID] = source
	}
	normalizedFields := make([]string, 0, 2)
	for _, field := range fields {
		binding := field.Binding
		if binding == nil || strings.TrimSpace(binding.LabelField) == "" {
			continue
		}
		current, exists := payload[field.ID]
		if !exists || isEmptyFormPayloadValue(current) {
			continue
		}
		source, ok := sourceByID[strings.TrimSpace(binding.SourceID)]
		if !ok {
			continue
		}
		currentText := strings.TrimSpace(agentDisplayValue(current))
		var matchedValue any
		labelMatches := 0
		valueExists := false
		for _, record := range source.Records {
			value := record[binding.ValueField]
			if strings.TrimSpace(agentDisplayValue(value)) == currentText {
				valueExists = true
				break
			}
			label := strings.TrimSpace(agentDisplayValue(record[binding.LabelField]))
			if label != "" && strings.EqualFold(label, currentText) {
				matchedValue = value
				labelMatches++
			}
		}
		if !valueExists && labelMatches == 1 {
			payload[field.ID] = matchedValue
			normalizedFields = append(normalizedFields, field.ID)
		}
	}
	return payload, normalizedFields, nil
}

// toolPayload 僅接受結構化 payload object，避免鬆散字串被當成表單內容。
func toolPayload(args map[string]any) (map[string]any, error) {
	if args == nil {
		return nil, BadRequest("payload is required")
	}
	payload, ok := args["payload"].(map[string]any)
	if !ok {
		return nil, BadRequest("payload must be an object")
	}
	return payload, nil
}

func (c *Service) agentFormDataSources(ctx domain.RequestContext, fields []domain.PlatformFormBuilderField) ([]domain.FormDataSource, error) {
	used := map[string]map[string]struct{}{}
	for _, field := range fields {
		if field.Binding != nil {
			sourceID := strings.TrimSpace(field.Binding.SourceID)
			if used[sourceID] == nil {
				used[sourceID] = map[string]struct{}{}
			}
			used[sourceID][strings.TrimSpace(field.Binding.ValueField)] = struct{}{}
			if labelField := strings.TrimSpace(field.Binding.LabelField); labelField != "" {
				used[sourceID][labelField] = struct{}{}
			}
		}
	}
	if len(used) == 0 {
		return []domain.FormDataSource{}, nil
	}
	catalog, err := c.Workflow().FormDataSources(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.FormDataSource, 0, len(used))
	for _, source := range catalog.DataSources {
		allowed, ok := used[source.ID]
		if !ok {
			continue
		}
		filtered := domain.FormDataSource{ID: source.ID, Label: source.Label, Kind: source.Kind}
		for _, field := range source.Fields {
			if _, ok := allowed[field.Key]; ok {
				filtered.Fields = append(filtered.Fields, field)
			}
		}
		for _, record := range source.Records {
			values := make(map[string]interface{}, len(allowed))
			for key := range allowed {
				if value, ok := record[key]; ok {
					values[key] = value
				}
			}
			filtered.Records = append(filtered.Records, values)
		}
		out = append(out, filtered)
	}
	return out, nil
}

// agentApprovalPath 將模板節點投影成確認卡可讀的審批路徑。
func agentApprovalPath(template domain.FormTemplate) []string {
	stages := ParseWorkflowStagesFromTemplate(template)
	out := make([]string, 0, len(stages))
	for _, stage := range stages {
		if label := strings.TrimSpace(stage.Label); label != "" {
			out = append(out, label)
		}
	}
	return out
}

// agentPayloadRows 依 schema 順序產生確認摘要，並將受控選項值解析成人類可讀標籤。
func (c *Service) agentPayloadRows(ctx domain.RequestContext, template domain.FormTemplate, payload map[string]any) ([]domain.AgentAnalysisRow, error) {
	fields := agentAccessibleFormFields(template)
	sources, err := c.agentFormDataSources(ctx, fields)
	if err != nil {
		return nil, err
	}
	sourceByID := make(map[string]domain.FormDataSource, len(sources))
	for _, source := range sources {
		sourceByID[source.ID] = source
	}
	rows := make([]domain.AgentAnalysisRow, 0, len(payload))
	for _, field := range fields {
		value, ok := payload[field.ID]
		if !ok || isEmptyFormPayloadValue(value) {
			continue
		}
		display := agentDisplayValue(value)
		for _, option := range field.Options {
			if option.Value == display {
				display = utils.FirstNonEmpty(option.Label, display)
				break
			}
		}
		if field.Binding != nil && strings.TrimSpace(field.Binding.LabelField) != "" {
			if source, ok := sourceByID[strings.TrimSpace(field.Binding.SourceID)]; ok {
				for _, record := range source.Records {
					if agentDisplayValue(record[field.Binding.ValueField]) == agentDisplayValue(value) {
						display = utils.FirstNonEmpty(agentDisplayValue(record[field.Binding.LabelField]), display)
						break
					}
				}
			}
		}
		rows = append(rows, domain.AgentAnalysisRow{Label: utils.FirstNonEmpty(field.Label, field.ID), Value: display})
	}
	return rows, nil
}

// agentDisplayValue 將結構化欄位值穩定轉成確認卡文字。
func agentDisplayValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	raw, err := json.Marshal(value)
	if err == nil {
		return string(raw)
	}
	return fmt.Sprint(value)
}
