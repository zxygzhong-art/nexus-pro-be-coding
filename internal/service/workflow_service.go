package service

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

const (
	workflowFormStatusDraft     = "draft"
	workflowFormStatusSubmitted = "submitted"
	workflowFormStatusInReview  = "in_review"
	workflowFormStatusApproved  = "approved"
	workflowFormStatusRejected  = "rejected"
	workflowFormStatusReturned  = "returned"
	workflowFormStatusCancelled = "cancelled"
)

var workflowNotificationRecipientPayloadKeys = []string{
	"notify_account_ids",
	"notified_account_ids",
	"cc_account_ids",
	"approver_account_ids",
	"reviewer_account_ids",
	"notifyAccountIds",
	"notifiedAccountIds",
	"ccAccountIds",
	"approverAccountIds",
	"reviewerAccountIds",
}

// WorkflowService 定義流程服務的資料結構。
type WorkflowService struct {
	*Service
	store workflowStore
}

// Workflow 處理流程的服務流程。
func (c *Service) Workflow() WorkflowService {
	return WorkflowService{Service: c, store: c.store}
}

// ListFormTemplates 列出表單範本的服務流程。
func (c WorkflowService) ListFormTemplates(ctx RequestContext) ([]FormTemplate, error) {
	if _, _, err := c.requireWorkflowAuthz(ctx, ResourceType("form_template"), ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListFormTemplates(goContext(ctx), ctx.TenantID)
}

// ListFormTemplatePage 列出表單範本分頁的服務流程。
func (c WorkflowService) ListFormTemplatePage(ctx RequestContext, page PageRequest) (PageResponse[FormTemplate], error) {
	items, err := c.ListFormTemplates(ctx)
	if err != nil {
		return PageResponse[FormTemplate]{}, err
	}
	items = utils.SortFormTemplates(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreateFormTemplate 建立表單範本的服務流程。
func (c WorkflowService) CreateFormTemplate(ctx RequestContext, input CreateFormTemplateInput) (FormTemplate, error) {
	if _, _, err := c.requireWorkflowAuthz(ctx, ResourceType("form_template"), ActionCreate, ""); err != nil {
		return FormTemplate{}, err
	}
	if strings.TrimSpace(input.Key) == "" || strings.TrimSpace(input.Name) == "" {
		return FormTemplate{}, BadRequest("template key and name are required")
	}
	tpl := FormTemplate{
		ID:          utils.NewID("ft"),
		TenantID:    ctx.TenantID,
		Key:         strings.TrimSpace(input.Key),
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		Schema:      utils.CopyStringMap(input.Schema),
		CreatedAt:   c.Now(),
	}
	if err := c.store.UpsertFormTemplate(goContext(ctx), tpl); err != nil {
		return FormTemplate{}, err
	}
	if err := c.audit(ctx, "workflow.form_template.create", "form_template", tpl.ID, "medium", map[string]any{"key": tpl.Key}); err != nil {
		return FormTemplate{}, err
	}
	c.logInfo(ctx, "form template created",
		"form_template_id", tpl.ID,
		"template_key", tpl.Key,
	)
	return tpl, nil
}

// ListFormInstancePage 列出表單實例分頁的服務流程。
func (c WorkflowService) ListFormInstancePage(ctx RequestContext, query FormInstanceQuery, page PageRequest) (PageResponse[FormInstance], error) {
	account, decision, err := c.requireWorkflowAuthz(ctx, ResourceFormInstance, ActionRead, "")
	if err != nil {
		return PageResponse[FormInstance]{}, err
	}
	query, err = normalizeFormInstanceQuery(query, account, decision)
	if err != nil {
		return PageResponse[FormInstance]{}, err
	}
	if page.Sort == "" {
		page.Sort = "submitted_at_desc"
	}
	page = utils.NormalizePageRequest(page)
	items, total, err := c.store.ListFormInstancePageByQuery(goContext(ctx), ctx.TenantID, query, page)
	if err != nil {
		return PageResponse[FormInstance]{}, err
	}
	return utils.PageResponseFromStore(items, total, page), nil
}

// ReviewQueue 處理審核佇列的服務流程。
func (c WorkflowService) ReviewQueue(ctx RequestContext) (WorkflowReviewQueueResponse, error) {
	items, account, err := c.listFormInstances(ctx, FormInstanceQuery{})
	if err != nil {
		return WorkflowReviewQueueResponse{}, err
	}
	pendingForAccount, err := workflowFormInstancePendingForAccount(ctx, c.store, ctx.TenantID, account.ID)
	if err != nil {
		return WorkflowReviewQueueResponse{}, err
	}
	templates, err := c.formTemplateMap(ctx)
	if err != nil {
		return WorkflowReviewQueueResponse{}, err
	}
	accounts, err := c.accountMap(ctx)
	if err != nil {
		return WorkflowReviewQueueResponse{}, err
	}
	employees, err := c.employeeByAccountMap(ctx)
	if err != nil {
		return WorkflowReviewQueueResponse{}, err
	}
	sortFormInstances(items, "submitted_at_desc")

	out := WorkflowReviewQueueResponse{
		PendingReview:   []WorkflowReviewItem{},
		AlreadyReviewed: []WorkflowReviewItem{},
		Notified:        []WorkflowReviewItem{},
	}
	for _, item := range items {
		if strings.EqualFold(item.Status, workflowFormStatusDraft) {
			continue
		}
		projected := c.reviewItem(item, templates[item.TemplateID], accounts[item.ApplicantAccountID], employees[item.ApplicantAccountID])
		status := normalizeWorkflowStatus(item.Status)
		switch status {
		case "approved", "rejected", "cancelled", "canceled":
			out.AlreadyReviewed = append(out.AlreadyReviewed, projected)
		case "returned":
			if item.ApplicantAccountID == account.ID {
				out.AlreadyReviewed = append(out.AlreadyReviewed, projected)
			}
		default:
			if _, ok := pendingForAccount[item.ID]; ok {
				out.PendingReview = append(out.PendingReview, projected)
			}
		}
		if workflowPayloadMentionsAccount(item.Payload, account.ID) {
			out.Notified = append(out.Notified, projected)
		}
	}
	return out, nil
}

// SaveFormDraft 儲存表單草稿的服務流程。
func (c WorkflowService) SaveFormDraft(ctx RequestContext, input SaveFormDraftInput) (FormInstance, error) {
	templateKey := strings.TrimSpace(input.TemplateKey)
	account, _, err := c.requireWorkflowAuthz(ctx, ResourceFormInstance, ActionSubmit, "")
	if err != nil {
		return FormInstance{}, err
	}
	if templateKey == "" {
		return FormInstance{}, BadRequest("template_key is required")
	}
	template, ok, err := c.store.GetFormTemplateByKey(goContext(ctx), ctx.TenantID, templateKey)
	if err != nil {
		return FormInstance{}, err
	}
	if !ok {
		return FormInstance{}, NotFound("form template", templateKey)
	}
	now := c.Now()
	instance := FormInstance{
		ID:                 utils.NewID("fi"),
		TenantID:           ctx.TenantID,
		TemplateID:         template.ID,
		ApplicantAccountID: account.ID,
		Status:             workflowFormStatusDraft,
		Payload:            workflowPayload(input.Payload),
		SubmittedAt:        now,
		UpdatedAt:          now,
	}
	if err := c.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
		return FormInstance{}, err
	}
	if err := c.audit(ctx, "workflow.form.draft.create", "form_instance", instance.ID, "low", map[string]any{"template_key": template.Key}); err != nil {
		return FormInstance{}, err
	}
	c.logInfo(ctx, "form draft saved",
		"form_instance_id", instance.ID,
		"form_template_id", template.ID,
		"template_key", template.Key,
		"status", instance.Status,
	)
	return instance, nil
}

// UpdateFormDraft 更新表單草稿的服務流程。
func (c WorkflowService) UpdateFormDraft(ctx RequestContext, id string, input UpdateFormDraftInput) (FormInstance, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return FormInstance{}, BadRequest("id is required")
	}
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppWorkflow, ResourceType: ResourceFormInstance, Action: ActionUpdate},
		AuditTarget{Event: "workflow.form.draft.update", Resource: string(ResourceFormInstance), Target: id},
	)
	if err != nil {
		return FormInstance{}, err
	}
	var instance FormInstance
	if err := c.withTransaction(ctx, func(tx WorkflowService) error {
		next, ok, err := tx.store.GetFormInstance(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("form instance", id)
		}
		if err := requireFormInstanceVisible(next, account, decision); err != nil {
			return err
		}
		if !strings.EqualFold(next.Status, workflowFormStatusDraft) {
			return BadRequest("only draft form instances can be updated")
		}
		if templateKey := strings.TrimSpace(input.TemplateKey); templateKey != "" {
			template, ok, err := tx.store.GetFormTemplateByKey(goContext(ctx), ctx.TenantID, templateKey)
			if err != nil {
				return err
			}
			if !ok {
				return NotFound("form template", templateKey)
			}
			next.TemplateID = template.ID
		}
		next.Payload = workflowPayload(input.Payload)
		next.UpdatedAt = tx.Now()
		if err := tx.store.UpsertFormInstance(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.audit(ctx, "workflow.form.draft.update", string(ResourceFormInstance), next.ID, string(SeverityLow), map[string]any{
			"template_id": next.TemplateID,
		}); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		instance = next
		return nil
	}); err != nil {
		return FormInstance{}, err
	}
	return instance, nil
}

// DeleteFormDraft 刪除表單草稿的服務流程。
func (c WorkflowService) DeleteFormDraft(ctx RequestContext, id string) (FormInstance, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return FormInstance{}, BadRequest("id is required")
	}
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppWorkflow, ResourceType: ResourceFormInstance, Action: ActionDelete},
		AuditTarget{Event: "workflow.form.draft.delete", Resource: string(ResourceFormInstance), Target: id},
	)
	if err != nil {
		return FormInstance{}, err
	}
	var deleted FormInstance
	if err := c.withTransaction(ctx, func(tx WorkflowService) error {
		current, ok, err := tx.store.GetFormInstance(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("form instance", id)
		}
		if err := requireFormInstanceVisible(current, account, decision); err != nil {
			return err
		}
		if !strings.EqualFold(current.Status, workflowFormStatusDraft) {
			return BadRequest("only draft form instances can be deleted")
		}
		if err := tx.store.DeleteFormInstance(goContext(ctx), ctx.TenantID, id); err != nil {
			return err
		}
		if err := tx.audit(ctx, "workflow.form.draft.delete", string(ResourceFormInstance), current.ID, string(SeverityLow), map[string]any{
			"template_id": current.TemplateID,
		}); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		deleted = current
		return nil
	}); err != nil {
		return FormInstance{}, err
	}
	return deleted, nil
}

// SubmitForm 提交表單的服務流程。
func (c WorkflowService) SubmitForm(ctx RequestContext, input SubmitFormInput) (FormInstance, error) {
	idOrTemplateKey := strings.TrimSpace(input.TemplateKey)
	if idOrTemplateKey == "" {
		return FormInstance{}, BadRequest("template_key is required")
	}
	if existing, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, idOrTemplateKey); err != nil {
		return FormInstance{}, err
	} else if ok {
		return c.submitExistingDraft(ctx, existing.ID, input.Payload)
	}
	return c.submitNewForm(ctx, idOrTemplateKey, input.Payload)
}

// submitNewForm 提交 new 表單的服務流程。
func (c WorkflowService) submitNewForm(ctx RequestContext, templateKey string, payload map[string]any) (FormInstance, error) {
	account, _, err := c.requireWorkflowAuthz(ctx, ResourceFormInstance, ActionSubmit, "")
	if err != nil {
		return FormInstance{}, err
	}
	var instance FormInstance
	var template FormTemplate
	if err := c.withTransaction(ctx, func(tx WorkflowService) error {
		nextTemplate, ok, err := tx.store.GetFormTemplateByKey(goContext(ctx), ctx.TenantID, templateKey)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("form template", templateKey)
		}
		if err := validateWorkflowTemplateSubmittable(nextTemplate); err != nil {
			return err
		}
		now := tx.Now()
		next := FormInstance{
			ID:                 utils.NewID("fi"),
			TenantID:           ctx.TenantID,
			TemplateID:         nextTemplate.ID,
			ApplicantAccountID: account.ID,
			Status:             workflowFormStatusDraft,
			Payload:            workflowPayload(payload),
			SubmittedAt:        now,
			UpdatedAt:          now,
		}
		if err := tx.store.UpsertFormInstance(goContext(ctx), next); err != nil {
			return err
		}
		started, err := tx.initWorkflowRun(ctx, next, nextTemplate, account)
		if err != nil {
			return err
		}
		instance = started
		template = nextTemplate
		if err := tx.audit(ctx, "workflow.form.submit", "form_instance", instance.ID, "medium", map[string]any{"template_key": nextTemplate.Key}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return FormInstance{}, err
	}
	c.logInfo(ctx, "form submitted",
		"form_instance_id", instance.ID,
		"form_template_id", template.ID,
		"template_key", template.Key,
		"status", instance.Status,
	)
	return instance, nil
}

// submitExistingDraft 提交 existing 草稿的服務流程。
func (c WorkflowService) submitExistingDraft(ctx RequestContext, id string, payload map[string]any) (FormInstance, error) {
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppWorkflow, ResourceType: ResourceFormInstance, Action: ActionSubmit},
		AuditTarget{Event: "workflow.form.submit", Resource: string(ResourceFormInstance), Target: id},
	)
	if err != nil {
		return FormInstance{}, err
	}
	var instance FormInstance
	if err := c.withTransaction(ctx, func(tx WorkflowService) error {
		next, ok, err := tx.store.GetFormInstance(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("form instance", id)
		}
		if err := requireFormInstanceVisible(next, account, decision); err != nil {
			return err
		}
		status := normalizeWorkflowStatus(next.Status)
		if status != workflowFormStatusDraft && status != workflowFormStatusReturned {
			return BadRequest("only draft or returned form instances can be submitted by id")
		}
		template, ok, err := tx.store.GetFormTemplate(goContext(ctx), ctx.TenantID, next.TemplateID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("form template", next.TemplateID)
		}
		if err := validateWorkflowTemplateSubmittable(template); err != nil {
			return err
		}
		now := tx.Now()
		if payload != nil {
			next.Payload = workflowPayload(payload)
		}
		next.SubmittedAt = now
		next.UpdatedAt = now
		if err := tx.store.UpsertFormInstance(goContext(ctx), next); err != nil {
			return err
		}
		started, err := tx.initWorkflowRun(ctx, next, template, account)
		if err != nil {
			return err
		}
		if err := tx.audit(ctx, "workflow.form.submit", string(ResourceFormInstance), started.ID, string(SeverityMedium), map[string]any{
			"template_id": next.TemplateID,
			"from_draft":  status == workflowFormStatusDraft,
			"resubmit":    status == workflowFormStatusReturned,
		}); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		instance = started
		return nil
	}); err != nil {
		return FormInstance{}, err
	}
	return instance, nil
}

// ApproveForm 核准表單的服務流程。
func (c WorkflowService) ApproveForm(ctx RequestContext, id string, input ApproveFormInput) (FormInstance, error) {
	instance, err := c.reviewForm(ctx, id, "approve", "approved", input.Reason)
	if err != nil {
		return FormInstance{}, err
	}
	c.logInfo(ctx, "form approved",
		"form_instance_id", instance.ID,
		"approved_by", instance.ApprovedBy,
	)
	return instance, nil
}

// RejectForm 駁回表單的服務流程。
func (c WorkflowService) RejectForm(ctx RequestContext, id string, input RejectFormInput) (FormInstance, error) {
	instance, err := c.reviewForm(ctx, id, "reject", "rejected", input.Reason)
	if err != nil {
		return FormInstance{}, err
	}
	c.logInfo(ctx, "form rejected",
		"form_instance_id", instance.ID,
		"rejected_by", instance.ApprovedBy,
	)
	return instance, nil
}

// ReturnForm 退回表單的服務流程。
func (c WorkflowService) ReturnForm(ctx RequestContext, id string, input ReturnFormInput) (FormInstance, error) {
	instance, err := c.reviewForm(ctx, id, "return", workflowFormStatusReturned, input.Reason)
	if err != nil {
		return FormInstance{}, err
	}
	c.logInfo(ctx, "form returned",
		"form_instance_id", instance.ID,
		"returned_by", instance.ApprovedBy,
	)
	return instance, nil
}

// CancelForm 取消表單的服務流程。
func (c WorkflowService) CancelForm(ctx RequestContext, id string, input CancelFormInput) (FormInstance, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return FormInstance{}, BadRequest("id is required")
	}
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppWorkflow, ResourceType: ResourceFormInstance, Action: ActionUpdate},
		AuditTarget{Event: "workflow.form.cancel", Resource: string(ResourceFormInstance), Target: id},
	)
	if err != nil {
		return FormInstance{}, err
	}
	var instance FormInstance
	if err := c.withTransaction(ctx, func(tx WorkflowService) error {
		next, ok, err := tx.store.GetFormInstance(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("form instance", id)
		}
		if err := requireFormInstanceVisible(next, account, decision); err != nil {
			return err
		}
		switch normalizeWorkflowStatus(next.Status) {
		case workflowFormStatusDraft:
			return BadRequest("draft form instances should be deleted instead of cancelled")
		case workflowFormStatusApproved:
			return BadRequest("approved form instances cannot be cancelled")
		case workflowFormStatusCancelled, "canceled":
			if err := tx.Service.Attendance().applyAttendanceWorkflowReview(ctx, next, "cancel", workflowFormStatusCancelled); err != nil {
				return err
			}
			instance = next
			return nil
		}
		now := tx.Now()
		next.Status = workflowFormStatusCancelled
		next.ApprovedBy = account.ID
		next.Payload = withWorkflowReview(next.Payload, "cancel", account.ID, input.Reason, now)
		next.UpdatedAt = now
		if err := tx.store.UpsertFormInstance(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.Service.Attendance().applyAttendanceWorkflowReview(ctx, next, "cancel", workflowFormStatusCancelled); err != nil {
			return err
		}
		if err := tx.audit(ctx, "workflow.form.cancel", string(ResourceFormInstance), next.ID, string(SeverityMedium), map[string]any{
			"template_id": next.TemplateID,
			"reason":      strings.TrimSpace(input.Reason),
		}); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		instance = next
		return nil
	}); err != nil {
		return FormInstance{}, err
	}
	return instance, nil
}

// DuplicateForm 處理 duplicate 表單的服務流程。
func (c WorkflowService) DuplicateForm(ctx RequestContext, id string) (FormInstance, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return FormInstance{}, BadRequest("id is required")
	}
	reader, readDecision, err := c.requireWorkflowAuthz(ctx, ResourceFormInstance, ActionRead, "")
	if err != nil {
		return FormInstance{}, err
	}
	if _, _, err := c.requireWorkflowAuthz(ctx, ResourceFormInstance, ActionSubmit, ""); err != nil {
		return FormInstance{}, err
	}
	current, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return FormInstance{}, err
	}
	if !ok {
		return FormInstance{}, NotFound("form instance", id)
	}
	if err := requireFormInstanceVisible(current, reader, readDecision); err != nil {
		return FormInstance{}, err
	}
	now := c.Now()
	next := FormInstance{
		ID:                 utils.NewID("fi"),
		TenantID:           ctx.TenantID,
		TemplateID:         current.TemplateID,
		ApplicantAccountID: reader.ID,
		Status:             workflowFormStatusDraft,
		Payload:            workflowPayload(current.Payload),
		SubmittedAt:        now,
		UpdatedAt:          now,
	}
	if err := c.store.UpsertFormInstance(goContext(ctx), next); err != nil {
		return FormInstance{}, err
	}
	if err := c.audit(ctx, "workflow.form.duplicate", "form_instance", next.ID, "low", map[string]any{
		"source_form_instance_id": current.ID,
		"template_id":             current.TemplateID,
	}); err != nil {
		return FormInstance{}, err
	}
	return next, nil
}

// ExportForm 匯出表單的服務流程。
func (c WorkflowService) ExportForm(ctx RequestContext, id string) (ExportedFormFile, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ExportedFormFile{}, BadRequest("id is required")
	}
	account, decision, err := c.requireWorkflowAuthz(ctx, ResourceFormInstance, ActionRead, "")
	if err != nil {
		return ExportedFormFile{}, err
	}
	instance, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return ExportedFormFile{}, err
	}
	if !ok {
		return ExportedFormFile{}, NotFound("form instance", id)
	}
	if err := requireFormInstanceVisible(instance, account, decision); err != nil {
		return ExportedFormFile{}, err
	}
	template, _, err := c.store.GetFormTemplate(goContext(ctx), ctx.TenantID, instance.TemplateID)
	if err != nil {
		return ExportedFormFile{}, err
	}
	body, err := json.MarshalIndent(map[string]any{
		"id":                   instance.ID,
		"tenant_id":            instance.TenantID,
		"template_id":          instance.TemplateID,
		"template_key":         template.Key,
		"template_name":        template.Name,
		"applicant_account_id": instance.ApplicantAccountID,
		"status":               instance.Status,
		"payload":              instance.Payload,
		"submitted_at":         instance.SubmittedAt,
		"updated_at":           instance.UpdatedAt,
	}, "", "  ")
	if err != nil {
		return ExportedFormFile{}, err
	}
	return ExportedFormFile{
		FileName:    safeWorkflowFileName(utils.FirstNonEmpty(template.Key, template.Name, instance.ID)) + "-" + instance.ID + ".json",
		ContentType: "application/octet-stream",
		Body:        append(body, '\n'),
	}, nil
}

// BulkReviewForms 處理批次審核表單的服務流程。
func (c WorkflowService) BulkReviewForms(ctx RequestContext, input BulkReviewFormsInput) (BulkReviewFormsResponse, error) {
	action, status, err := normalizeBulkReviewAction(input.Action)
	if err != nil {
		return BulkReviewFormsResponse{}, err
	}
	ids := uniqueStrings(input.FormInstanceIDs)
	if len(ids) == 0 {
		return BulkReviewFormsResponse{}, BadRequest("form_instance_ids is required")
	}
	results := make([]BulkReviewFormResult, 0, len(ids))
	for _, id := range ids {
		instance, err := c.reviewForm(ctx, id, action, status, input.Reason)
		if err != nil {
			results = append(results, BulkReviewFormResult{
				FormInstanceID: id,
				Success:        false,
				Action:         action,
				Code:           errorCode(err),
				Message:        err.Error(),
			})
			continue
		}
		next := instance
		results = append(results, BulkReviewFormResult{
			FormInstanceID: id,
			Success:        true,
			Action:         action,
			Instance:       &next,
		})
	}
	return BulkReviewFormsResponse{Results: results}, nil
}

// reviewForm 處理審核表單的服務流程。
func (c WorkflowService) reviewForm(ctx RequestContext, id string, kind string, status string, comment string) (FormInstance, error) {
	if run, ok, err := c.store.GetWorkflowRunByFormInstance(goContext(ctx), ctx.TenantID, id); err != nil {
		return FormInstance{}, err
	} else if ok && strings.EqualFold(run.Status, domain.WorkflowRunStatusRunning) {
		return c.ActOnWorkflowStage(ctx, id, kind, comment)
	}
	action := ActionUpdate
	if kind == "approve" {
		action = ActionApprove
	}
	reviewer, _, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppWorkflow, ResourceType: ResourceFormInstance, ResourceID: id, Action: action},
		AuditTarget{Event: "workflow.form." + kind, Resource: string(ResourceFormInstance), Target: id},
	)
	if err != nil {
		return FormInstance{}, err
	}
	var instance FormInstance
	if err := c.withTransaction(ctx, func(tx WorkflowService) error {
		next, ok, err := tx.store.GetFormInstance(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("form instance", id)
		}
		if kind != "approve" && strings.EqualFold(next.Status, "approved") {
			return BadRequest("approved form instance cannot be " + kind + "ed")
		}
		if strings.EqualFold(next.Status, status) {
			if err := tx.Service.Attendance().applyAttendanceWorkflowReview(ctx, next, kind, status); err != nil {
				return err
			}
			instance = next
			return nil
		}
		previousStatus := next.Status
		reason := strings.TrimSpace(comment)
		now := tx.Now()
		next.Status = status
		next.ApprovedBy = ctx.AccountID
		next.Payload = withWorkflowReview(next.Payload, kind, ctx.AccountID, reason, now)
		next.UpdatedAt = now
		if err := tx.store.UpsertFormInstance(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.Service.Attendance().applyAttendanceWorkflowReview(ctx, next, kind, status); err != nil {
			return err
		}
		template, ok, err := tx.store.GetFormTemplate(goContext(ctx), ctx.TenantID, next.TemplateID)
		if err != nil {
			return err
		}
		if !ok {
			template = FormTemplate{ID: next.TemplateID, Name: next.TemplateID}
		}
		if err := tx.audit(ctx, "workflow.form."+kind, string(ResourceFormInstance), next.ID, string(SeverityHigh), map[string]any{
			"template_id":      next.TemplateID,
			"reviewed_by":      ctx.AccountID,
			"review_action":    kind,
			"review_comment":   reason,
			"previous_status":  previousStatus,
			"resulting_status": status,
		}); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		if err := tx.notifyWorkflowFormReviewed(ctx, next, template, reviewer, kind, reason); err != nil {
			return err
		}
		instance = next
		return nil
	}); err != nil {
		return FormInstance{}, err
	}
	return instance, nil
}

// normalizeBulkReviewAction 正規化批次審核 action。
func normalizeBulkReviewAction(action string) (string, string, error) {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "approve", "approved":
		return "approve", "approved", nil
	case "reject", "rejected", "deny", "denied", "disapprove", "not_approve":
		return "reject", "rejected", nil
	case "return", "returned":
		return "return", workflowFormStatusReturned, nil
	default:
		return "", "", BadRequest("action must be approve, reject, or return")
	}
}

// listFormInstances 列出表單實例的服務流程。
func (c WorkflowService) listFormInstances(ctx RequestContext, query FormInstanceQuery) ([]FormInstance, Account, error) {
	account, decision, err := c.requireWorkflowAuthz(ctx, ResourceFormInstance, ActionRead, "")
	if err != nil {
		return nil, Account{}, err
	}
	query, err = normalizeFormInstanceQuery(query, account, decision)
	if err != nil {
		return nil, Account{}, err
	}
	items, err := c.store.ListFormInstancesByQuery(goContext(ctx), ctx.TenantID, query)
	if err != nil {
		return nil, Account{}, err
	}
	return items, account, nil
}

// normalizeFormInstanceQuery 正規化表單實例查詢。
func normalizeFormInstanceQuery(query FormInstanceQuery, account Account, decision CheckResult) (FormInstanceQuery, error) {
	query.Status = strings.TrimSpace(strings.ToLower(query.Status))
	query.TemplateID = strings.TrimSpace(query.TemplateID)
	query.TemplateKey = strings.TrimSpace(query.TemplateKey)
	query.ApplicantAccountID = strings.TrimSpace(query.ApplicantAccountID)
	if query.Status != "" {
		status, err := normalizeFormInstanceStatusFilter(query.Status)
		if err != nil {
			return FormInstanceQuery{}, err
		}
		query.Status = status
	}
	switch decision.Scope {
	case ScopeSelf, ScopeOwn:
		query.ApplicantAccountID = account.ID
	default:
		if query.Mine {
			query.ApplicantAccountID = account.ID
		}
	}
	return query, nil
}

// normalizeFormInstanceStatusFilter 正規化表單實例狀態篩選。
func normalizeFormInstanceStatusFilter(status string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "", "all":
		return "", nil
	case "draft", "drafts":
		return workflowFormStatusDraft, nil
	case "pending", "pending_review", "pending-review", "submitted":
		return workflowFormStatusSubmitted, nil
	case "approved":
		return workflowFormStatusApproved, nil
	case "rejected", "reject":
		return workflowFormStatusRejected, nil
	case "cancelled", "canceled":
		return workflowFormStatusCancelled, nil
	default:
		return "", BadRequest("status must be draft, submitted, approved, rejected, cancelled, or all")
	}
}

// sortFormInstances 排序表單實例。
func sortFormInstances(items []FormInstance, sortKey string) {
	if strings.TrimSpace(sortKey) == "" {
		sortKey = "submitted_at_desc"
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch sortKey {
		case "submitted_at_asc", "created_at_asc":
			return a.SubmittedAt.Before(b.SubmittedAt)
		case "updated_at_asc":
			return a.UpdatedAt.Before(b.UpdatedAt)
		case "updated_at_desc":
			return a.UpdatedAt.After(b.UpdatedAt)
		case "status_asc":
			return a.Status < b.Status
		default:
			return a.SubmittedAt.After(b.SubmittedAt)
		}
	})
}

// formTemplateMap 處理表單範本 map 的服務流程。
func (c WorkflowService) formTemplateMap(ctx RequestContext) (map[string]FormTemplate, error) {
	templates, err := c.store.ListFormTemplates(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]FormTemplate, len(templates))
	for _, item := range templates {
		out[item.ID] = item
	}
	return out, nil
}

// accountMap 處理帳號 map 的服務流程。
func (c WorkflowService) accountMap(ctx RequestContext) (map[string]Account, error) {
	accounts, err := c.store.ListAccounts(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Account, len(accounts))
	for _, item := range accounts {
		out[item.ID] = item
	}
	return out, nil
}

// employeeByAccountMap 處理員工 by 帳號 map 的服務流程。
func (c WorkflowService) employeeByAccountMap(ctx RequestContext) (map[string]Employee, error) {
	employees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Employee, len(employees))
	for _, item := range employees {
		if item.AccountID != "" {
			out[item.AccountID] = item
		}
	}
	return out, nil
}

// reviewItem 處理審核項目的服務流程。
func (c WorkflowService) reviewItem(item FormInstance, template FormTemplate, account Account, employee Employee) WorkflowReviewItem {
	status := normalizeWorkflowStatus(item.Status)
	title := utils.FirstNonEmpty(template.Name, template.Key, item.TemplateID, "审批申请")
	return WorkflowReviewItem{
		ID:         item.ID,
		Status:     workflowReviewStatus(status),
		StatusText: workflowReviewStatusText(status),
		Title:      title,
		Who:        workflowApplicantLabel(account, employee),
		Desc:       workflowReviewDescription(item.Payload),
		Time:       workflowReviewTime(item),
		ReviewLog:  workflowReviewLog(item.Payload),
		Instance:   item,
	}
}

// normalizeWorkflowStatus 正規化流程狀態。
func normalizeWorkflowStatus(status string) string {
	return strings.TrimSpace(strings.ToLower(status))
}

// workflowReviewStatus 處理流程審核狀態。
func workflowReviewStatus(status string) string {
	switch status {
	case "approved":
		return "success"
	case "rejected":
		return "destructive"
	case "returned":
		return "warning"
	case "cancelled", "canceled":
		return "secondary"
	default:
		return "warning"
	}
}

// workflowReviewStatusText 處理流程審核狀態 text。
func workflowReviewStatusText(status string) string {
	switch status {
	case "approved":
		return "已核准"
	case "rejected":
		return "已駁回"
	case "returned":
		return "已退回"
	case "cancelled", "canceled":
		return "已取消"
	default:
		return "审核中"
	}
}

// workflowApplicantLabel 處理流程 applicant label。
func workflowApplicantLabel(account Account, employee Employee) string {
	name := utils.FirstNonEmpty(employee.Name, account.DisplayName, account.Email, account.ID)
	if employee.OrgUnitID != "" {
		return name + "（" + employee.OrgUnitID + "）"
	}
	return name
}

// workflowReviewDescription 處理流程審核 description。
func workflowReviewDescription(payload map[string]any) string {
	if text := strings.TrimSpace(stringFromAny(payload["desc"])); text != "" {
		return text
	}
	if text := strings.TrimSpace(stringFromAny(payload["description"])); text != "" {
		return text
	}
	if text := strings.TrimSpace(stringFromAny(payload["reason"])); text != "" {
		return text
	}
	app := strings.TrimSpace(stringFromAny(payload["application_code"]))
	resource := strings.TrimSpace(stringFromAny(payload["resource_type"]))
	action := strings.TrimSpace(stringFromAny(payload["action"]))
	if app != "" || resource != "" || action != "" {
		return strings.TrimSpace(strings.Join([]string{app, resource, action}, " "))
	}
	return "表单已提交，等待审批处理。"
}

// workflowReviewTime 處理流程審核時間。
func workflowReviewTime(item FormInstance) string {
	t := item.SubmittedAt
	if !item.UpdatedAt.IsZero() && !normalizeWorkflowPendingStatus(item.Status) {
		t = item.UpdatedAt
	}
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006/01/02 15:04")
}

// normalizeWorkflowPendingStatus 正規化流程 pending 狀態。
func normalizeWorkflowPendingStatus(status string) bool {
	switch normalizeWorkflowStatus(status) {
	case "approved", "rejected", "cancelled", "canceled":
		return false
	default:
		return true
	}
}

// workflowReviewLog 處理流程審核 log。
func workflowReviewLog(payload map[string]any) []WorkflowReviewLogItem {
	review, _ := payload["_review"].(map[string]any)
	if len(review) == 0 {
		return nil
	}
	kind := strings.TrimSpace(stringFromAny(review["type"]))
	if kind == "" {
		return nil
	}
	return []WorkflowReviewLogItem{{
		Type:    kind,
		Name:    strings.TrimSpace(stringFromAny(review["account_id"])),
		Role:    "审批人",
		Time:    strings.TrimSpace(stringFromAny(review["time"])),
		Comment: strings.TrimSpace(stringFromAny(review["comment"])),
	}}
}

// notifyWorkflowFormSubmitted 投遞表單提交後的明確知會通知。
func (c WorkflowService) notifyWorkflowFormSubmitted(ctx RequestContext, instance FormInstance, template FormTemplate, applicant Account) error {
	recipients := workflowNotificationRecipients(instance.Payload, instance.ApplicantAccountID)
	if len(recipients) == 0 {
		return nil
	}
	title := workflowNotificationTemplateTitle(template, instance)
	body := workflowAccountLabel(applicant) + "提交了「" + title + "」。"
	if desc := strings.TrimSpace(workflowReviewDescription(instance.Payload)); desc != "" {
		body += " " + desc
	}
	return c.deliverWorkflowNotification(ctx, Notification{
		ID:                 workflowNotificationID("submitted", instance.ID),
		TenantID:           ctx.TenantID,
		Tone:               "warning",
		Category:           "workflow",
		Title:              "有新的「" + title + "」表單待查看",
		Body:               body,
		StatusText:         "待處理",
		LinkURL:            "/notifications?reviewId=" + instance.ID,
		SourceType:         "workflow.form.submitted",
		SourceID:           instance.ID,
		CreatedByAccountID: applicant.ID,
		CreatedAt:          workflowNotificationTime(instance, c.Now()),
	}, recipients)
}

// notifyWorkflowFormReviewed 投遞表單審核結果給申請人。
func (c WorkflowService) notifyWorkflowFormReviewed(ctx RequestContext, instance FormInstance, template FormTemplate, reviewer Account, kind, reason string) error {
	if strings.TrimSpace(instance.ApplicantAccountID) == "" {
		return nil
	}
	title := workflowNotificationTemplateTitle(template, instance)
	tone, statusText, notificationTitle, actionText := workflowReviewNotificationCopy(kind, title)
	body := "由 " + workflowAccountLabel(reviewer) + actionText + "。"
	if reason = strings.TrimSpace(reason); reason != "" {
		body += " 審核意見：" + reason
	}
	return c.deliverWorkflowNotification(ctx, Notification{
		ID:                 workflowNotificationID("review-"+kind, instance.ID),
		TenantID:           ctx.TenantID,
		Tone:               tone,
		Category:           "workflow",
		Title:              notificationTitle,
		Body:               body,
		StatusText:         statusText,
		LinkURL:            "/forms?applicationId=" + instance.ID,
		SourceType:         "workflow.form.review_result",
		SourceID:           instance.ID + ":" + kind,
		CreatedByAccountID: reviewer.ID,
		CreatedAt:          workflowNotificationTime(instance, c.Now()),
	}, []string{instance.ApplicantAccountID})
}

// deliverWorkflowNotification 將一筆工作流通知寫入內容與收件者狀態。
func (c WorkflowService) deliverWorkflowNotification(ctx RequestContext, notification Notification, recipientIDs []string) error {
	recipients, err := c.validWorkflowNotificationRecipients(ctx, recipientIDs)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return nil
	}
	if notification.ID == "" {
		notification.ID = utils.NewID("notif")
	}
	if notification.TenantID == "" {
		notification.TenantID = ctx.TenantID
	}
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = c.Now()
	}
	notification.CreatedAt = notification.CreatedAt.UTC()
	if err := c.store.UpsertNotification(goContext(ctx), notification); err != nil {
		return err
	}
	for _, accountID := range recipients {
		if err := c.store.UpsertNotificationRecipient(goContext(ctx), NotificationRecipient{
			NotificationID: notification.ID,
			TenantID:       notification.TenantID,
			AccountID:      accountID,
			CreatedAt:      notification.CreatedAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

// validWorkflowNotificationRecipients 過濾不存在或停用的通知收件帳號。
func (c WorkflowService) validWorkflowNotificationRecipients(ctx RequestContext, recipientIDs []string) ([]string, error) {
	ids := uniqueWorkflowRecipientIDs(recipientIDs)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		account, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return nil, err
		}
		if !ok || account.Status == string(AccountStatusDisabled) || account.Status == string(AccountStatusPendingInvite) {
			continue
		}
		out = append(out, id)
	}
	return out, nil
}

// workflowReviewNotificationCopy 產生審核結果通知文案。
func workflowReviewNotificationCopy(kind, title string) (tone, statusText, notificationTitle, actionText string) {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case "approve":
		return "success", "已核准", "你的「" + title + "」已核准", "已核准這筆申請"
	case "return":
		return "warning", "已退回", "你的「" + title + "」已退回補件", "已退回這筆申請"
	default:
		return "warning", "不通過", "你的「" + title + "」未通過", "未通過這筆申請"
	}
}

// workflowNotificationRecipients 從 payload 中擷取明確列出的通知收件者。
func workflowNotificationRecipients(payload map[string]any, excluded ...string) []string {
	values := make([]string, 0)
	for _, key := range workflowNotificationRecipientPayloadKeys {
		values = append(values, workflowPayloadAccountIDs(payload[key])...)
	}
	return uniqueWorkflowRecipientIDs(values, excluded...)
}

// workflowPayloadAccountIDs 將 payload 欄位正規化為帳號 ID 清單。
func workflowPayloadAccountIDs(value any) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, stringFromAny(item))
		}
		return out
	default:
		return nil
	}
}

// uniqueWorkflowRecipientIDs 正規化並去重通知收件帳號。
func uniqueWorkflowRecipientIDs(values []string, excluded ...string) []string {
	excludedSet := map[string]struct{}{}
	for _, id := range excluded {
		if id = strings.TrimSpace(id); id != "" {
			excludedSet[id] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, id := range values {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, skip := excludedSet[id]; skip {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// workflowNotificationTemplateTitle 取通知中使用的表單名稱。
func workflowNotificationTemplateTitle(template FormTemplate, instance FormInstance) string {
	return utils.FirstNonEmpty(template.Name, template.Key, template.ID, instance.TemplateID, "表單申請")
}

// workflowNotificationID 建立可重試的工作流通知 ID。
func workflowNotificationID(kind, instanceID string) string {
	return safeWorkflowFileName("notif-workflow-" + kind + "-" + instanceID)
}

// workflowNotificationTime 取通知時間並保證 UTC。
func workflowNotificationTime(instance FormInstance, fallback time.Time) time.Time {
	if !instance.UpdatedAt.IsZero() {
		return instance.UpdatedAt.UTC()
	}
	if !instance.SubmittedAt.IsZero() {
		return instance.SubmittedAt.UTC()
	}
	return fallback.UTC()
}

// workflowAccountLabel 取通知文案中的帳號顯示名稱。
func workflowAccountLabel(account Account) string {
	return utils.FirstNonEmpty(account.DisplayName, account.Email, account.ID, "系統")
}

// workflowPayloadMentionsAccount 處理流程 payload mentions 帳號。
func workflowPayloadMentionsAccount(payload map[string]any, accountID string) bool {
	if accountID == "" {
		return false
	}
	for _, key := range workflowNotificationRecipientPayloadKeys {
		if stringSliceContains(payload[key], accountID) {
			return true
		}
	}
	return false
}

// stringSliceContains 處理字串 slice contains。
func stringSliceContains(value any, target string) bool {
	switch v := value.(type) {
	case []string:
		for _, item := range v {
			if item == target {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if stringFromAny(item) == target {
				return true
			}
		}
	}
	return false
}

// withWorkflowReview 附加流程審核。
func withWorkflowReview(payload map[string]any, kind, accountID, comment string, at time.Time) map[string]any {
	next := utils.CopyStringMap(payload)
	if next == nil {
		next = map[string]any{}
	}
	next["_review"] = map[string]any{
		"type":       kind,
		"account_id": accountID,
		"comment":    strings.TrimSpace(comment),
		"time":       at.UTC().Format("2006/01/02 15:04"),
	}
	return next
}

// workflowPayload 處理流程 payload。
func workflowPayload(payload map[string]any) map[string]any {
	next := utils.CopyStringMap(payload)
	if next == nil {
		return map[string]any{}
	}
	return next
}

// requireFormInstanceVisible 處理 require 表單實例可見。
func requireFormInstanceVisible(instance FormInstance, account Account, decision CheckResult) error {
	switch decision.Scope {
	case ScopeSelf, ScopeOwn:
		if instance.ApplicantAccountID != account.ID {
			return NotFound("form instance", instance.ID)
		}
	}
	return nil
}

// safeWorkflowFileName 處理 safe 流程檔案名稱。
func safeWorkflowFileName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "workflow-form"
	}
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
		" ", "-",
	)
	value = replacer.Replace(value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "workflow-form"
	}
	return value
}
var workflowConditionNumberPattern = regexp.MustCompile(`(?:≥|>|=|<|≤|>=|<=)\s*([0-9]+)`)

// ParseWorkflowStagesFromTemplate 從 template schema 解析可執行流程節點。
func ParseWorkflowStagesFromTemplate(template domain.FormTemplate) []domain.WorkflowStageDefinition {
	stages := platformTemplateStages(template.Schema)
	out := make([]domain.WorkflowStageDefinition, 0, len(stages))
	for _, stage := range stages {
		if strings.TrimSpace(stage.ID) == "" || strings.TrimSpace(stage.Type) == "" {
			continue
		}
		out = append(out, normalizeWorkflowStageDefinition(stage))
	}
	return out
}

// SerializeWorkflowStages 序列化流程節點快照。
func SerializeWorkflowStages(stages []domain.WorkflowStageDefinition) string {
	raw, err := json.Marshal(stages)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// DeserializeWorkflowStages 還原流程節點快照。
func DeserializeWorkflowStages(raw string) []domain.WorkflowStageDefinition {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []domain.WorkflowStageDefinition
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func normalizeWorkflowStageDefinition(stage domain.PlatformFormBuilderStage) domain.WorkflowStageDefinition {
	config := workflowStageConfigFromMap(stage.Config)
	if config.Role == "" && len(config.AccountIDs) == 0 && config.Field == "" {
		config = inferWorkflowStageConfig(stage)
	}
	return domain.WorkflowStageDefinition{
		ID:     strings.TrimSpace(stage.ID),
		Type:   strings.TrimSpace(stage.Type),
		Label:  strings.TrimSpace(stage.Label),
		Detail: strings.TrimSpace(stage.Detail),
		Config: config,
	}
}

func workflowStageConfigFromMap(values map[string]any) domain.WorkflowStageConfig {
	if len(values) == 0 {
		return domain.WorkflowStageConfig{}
	}
	config := domain.WorkflowStageConfig{
		Role:            stringFromAny(values["role"]),
		Mode:            stringFromAny(values["mode"]),
		Field:           stringFromAny(values["field"]),
		Operator:        stringFromAny(values["operator"]),
		Value:           stringFromAny(values["value"]),
		TrueNextStageID: stringFromAny(values["true_next_stage_id"]),
		FalseNextStageID: stringFromAny(values["false_next_stage_id"]),
	}
	if level := intFromAny(values["relative_level"]); level > 0 {
		config.RelativeLevel = level
	}
	if ids, ok := values["account_ids"].([]any); ok {
		for _, item := range ids {
			if id := strings.TrimSpace(stringFromAny(item)); id != "" {
				config.AccountIDs = append(config.AccountIDs, id)
			}
		}
	}
	if levels, ok := values["levels"].([]any); ok {
		for _, item := range levels {
			if level := intFromAny(item); level > 0 {
				config.Levels = append(config.Levels, level)
			}
		}
	}
	return config
}

func inferWorkflowStageConfig(stage domain.PlatformFormBuilderStage) domain.WorkflowStageConfig {
	text := strings.TrimSpace(stage.Label + " " + stage.Detail)
	stageType := strings.TrimSpace(stage.Type)
	switch stageType {
	case "notify":
		return domain.WorkflowStageConfig{Role: inferWorkflowRole(text)}
	case "parallel":
		return domain.WorkflowStageConfig{Role: inferWorkflowRole(text), Mode: "all"}
	case "condition":
		return inferWorkflowConditionConfig(stage)
	default:
		role := inferWorkflowRole(text)
		config := domain.WorkflowStageConfig{Role: role}
		if strings.Contains(text, "+2") {
			config.Role = "relative"
			config.RelativeLevel = 2
		} else if strings.Contains(text, "+1") || strings.Contains(text, "+N") {
			config.Role = "relative"
			config.RelativeLevel = 1
		}
		return config
	}
}

func inferWorkflowRole(text string) string {
	switch {
	case strings.Contains(text, "部門主管"):
		return "dept-head"
	case strings.Contains(text, "HR"):
		return "hr"
	case strings.Contains(text, "財務"):
		return "finance"
	case strings.Contains(text, "總經理"):
		return "ceo"
	case strings.Contains(text, "申請者本人"):
		return "applicant"
	case strings.Contains(text, "+2"):
		return "relative"
	case strings.Contains(text, "+1") || strings.Contains(text, "+N"):
		return "relative"
	default:
		return "manager"
	}
}

func inferWorkflowConditionConfig(stage domain.PlatformFormBuilderStage) domain.WorkflowStageConfig {
	label := strings.TrimSpace(stage.Label)
	field := "hours"
	switch {
	case strings.Contains(label, "金額"):
		field = "amount"
	case strings.Contains(label, "職等"):
		field = "level"
	}
	operator := ">="
	switch {
	case strings.Contains(label, "≤") || strings.Contains(label, "<="):
		operator = "<="
	case strings.Contains(label, "<"):
		operator = "<"
	case strings.Contains(label, ">") && !strings.Contains(label, "≥"):
		operator = ">"
	case strings.Contains(label, "="):
		operator = "=="
	}
	value := ""
	if match := workflowConditionNumberPattern.FindStringSubmatch(label); len(match) > 1 {
		value = match[1]
	}
	levels := make([]int, 0)
	if field == "level" {
		for _, token := range regexp.MustCompile(`[0-9]+`).FindAllString(label, -1) {
			if level, err := strconv.Atoi(token); err == nil {
				levels = append(levels, level)
			}
		}
	}
	return domain.WorkflowStageConfig{
		Field:    field,
		Operator: operator,
		Value:    value,
		Levels:   levels,
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func validateWorkflowTemplateSubmittable(template domain.FormTemplate) error {
	if platformTemplateDeleted(template.Schema) {
		return BadRequest("form template is deleted")
	}
	if !platformTemplateEnabled(template.Schema) {
		return BadRequest("form template is disabled")
	}
	if len(ParseWorkflowStagesFromTemplate(template)) == 0 {
		return BadRequest("form template has no workflow stages")
	}
	return nil
}

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
	run := domain.WorkflowRun{
		ID:                   utils.NewID("wfr"),
		TenantID:             ctx.TenantID,
		FormInstanceID:       instance.ID,
		TemplateID:           template.ID,
		Version:              version,
		Status:               domain.WorkflowRunStatusRunning,
		StageDefinitionsJSON: SerializeWorkflowStages(stages),
		CreatedAt:            now,
		UpdatedAt:            now,
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
	if err := c.advanceWorkflowFrom(ctx, run, stages, 0, applicant, instance.Payload, ""); err != nil {
		return domain.FormInstance{}, err
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
	action = strings.TrimSpace(strings.ToLower(action))
	if action == "" {
		return domain.FormInstance{}, BadRequest("action is required")
	}
	instance, run, stageInstance, stages, err := c.loadActiveWorkflowStage(ctx, formInstanceID)
	if err != nil {
		return domain.FormInstance{}, err
	}
	assignees, err := c.store.ListWorkflowStageAssignees(goContext(ctx), ctx.TenantID, stageInstance.ID)
	if err != nil {
		return domain.FormInstance{}, err
	}
	if !workflowAssigneeCanAct(assignees, ctx.AccountID) {
		return domain.FormInstance{}, Forbidden("current account is not an active assignee for this stage")
	}
	now := c.Now()
	err = c.withTransaction(ctx, func(tx WorkflowService) error {
		if err := tx.recordWorkflowAction(ctx, run, stageInstance, action, comment, now); err != nil {
			return err
		}
		switch action {
		case "approve":
			return tx.handleWorkflowApprove(ctx, instance, run, stageInstance, stages, assignees, comment, now)
		case "reject":
			return tx.completeWorkflowDecision(ctx, instance, run, stageInstance, workflowFormStatusRejected, domain.WorkflowRunStatusCompleted, "reject", comment, now)
		case "return":
			return tx.completeWorkflowDecision(ctx, instance, run, stageInstance, domain.WorkflowFormStatusReturned, domain.WorkflowRunStatusReturned, "return", comment, now)
		default:
			return BadRequest("unsupported workflow action: " + action)
		}
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

// GetWorkflowFormState 回傳單據流程運行狀態。
func (c WorkflowService) GetWorkflowFormState(ctx RequestContext, formInstanceID string) (domain.WorkflowFormStateResponse, error) {
	_, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
	if err != nil {
		return domain.WorkflowFormStateResponse{}, err
	}
	if !ok {
		return domain.WorkflowFormStateResponse{}, NotFound("form instance", formInstanceID)
	}
	run, ok, err := c.store.GetWorkflowRunByFormInstance(goContext(ctx), ctx.TenantID, formInstanceID)
	if err != nil {
		return domain.WorkflowFormStateResponse{}, err
	}
	if !ok {
		return domain.WorkflowFormStateResponse{FormInstanceID: formInstanceID, Steps: []domain.WorkflowFormStep{}}, nil
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
			if workflowAssigneeCanAct(assignees, ctx.AccountID) && (current.StageType == "approver" || current.StageType == "parallel") {
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
		FormInstanceID:    formInstanceID,
		RunID:             run.ID,
		RunStatus:         run.Status,
		CurrentStageID:    currentStageID,
		CurrentStageLabel: currentStageLabel,
		CanAct:            canAct,
		AllowedActions:    allowed,
		Steps:             steps,
		Actions:           reviewLog,
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
		return domain.FormInstance{}, domain.WorkflowRun{}, domain.WorkflowStageInstance{}, nil, BadRequest("form instance has no active workflow stage")
	}
	stageInstance, ok, err := c.store.GetWorkflowStageInstance(goContext(ctx), ctx.TenantID, run.CurrentStageInstanceID)
	if err != nil {
		return domain.FormInstance{}, domain.WorkflowRun{}, domain.WorkflowStageInstance{}, nil, err
	}
	if !ok || stageInstance.Status != domain.WorkflowStageStatusActive {
		return domain.FormInstance{}, domain.WorkflowRun{}, domain.WorkflowStageInstance{}, nil, BadRequest("workflow stage is not active")
	}
	return instance, run, stageInstance, DeserializeWorkflowStages(run.StageDefinitionsJSON), nil
}

func (c WorkflowService) advanceWorkflowFrom(ctx RequestContext, run domain.WorkflowRun, stages []domain.WorkflowStageDefinition, startIndex int, applicant domain.Account, payload map[string]any, approvalComment string) error {
	for index := startIndex; index < len(stages); index++ {
		stage := stages[index]
		switch strings.TrimSpace(stage.Type) {
		case "notify":
			if err := c.completeAutomaticStage(ctx, run, stage, index, "notify", applicant, payload); err != nil {
				return err
			}
			continue
		case "condition":
			nextIndex, err := c.completeConditionStage(ctx, run, stage, index, applicant, payload, stages)
			if err != nil {
				return err
			}
			if nextIndex < 0 || nextIndex >= len(stages) {
				return c.completeWorkflowApproved(ctx, run, applicant, payload, approvalComment)
			}
			index = nextIndex - 1
			continue
		case "approver", "parallel":
			return c.activateApprovalStage(ctx, run, stage, index, applicant, payload)
		default:
			return BadRequest("unsupported workflow stage type: " + stage.Type)
		}
	}
	return c.completeWorkflowApproved(ctx, run, applicant, payload, approvalComment)
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
			body := workflowAccountLabel(applicant) + "提交了「" + title + "」，請知悉。"
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

func (c WorkflowService) completeConditionStage(ctx RequestContext, run domain.WorkflowRun, stage domain.WorkflowStageDefinition, sequence int, applicant domain.Account, payload map[string]any, stages []domain.WorkflowStageDefinition) (int, error) {
	employee, err := c.resolveWorkflowApplicantEmployee(ctx, applicant)
	if err != nil {
		return -1, err
	}
	now := c.Now()
	matched := evaluateWorkflowCondition(stage.Config, employee, payload)
	targetStageID := stage.Config.FalseNextStageID
	if matched {
		targetStageID = stage.Config.TrueNextStageID
	}
	if targetStageID == "" {
		if matched {
			targetStageID = workflowNextStageID(stages, stage.ID)
		} else {
			targetStageID = workflowSkipToNextApproverStageID(stages, stage.ID)
		}
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
		return -1, err
	}
	if err := c.recordWorkflowAction(ctx, run, stageInstance, "auto_condition", "", now); err != nil {
		return -1, err
	}
	return workflowStageIndexByID(stages, targetStageID), nil
}

func (c WorkflowService) activateApprovalStage(ctx RequestContext, run domain.WorkflowRun, stage domain.WorkflowStageDefinition, sequence int, applicant domain.Account, payload map[string]any) error {
	assigneeIDs, err := c.resolveWorkflowAssignees(ctx, applicant, stage, payload)
	if err != nil {
		return err
	}
	if len(assigneeIDs) == 0 {
		return BadRequest("workflow stage has no resolvable assignees: " + stage.Label)
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
	return c.notifyWorkflowPendingApprovers(ctx, instance, template, applicant, stage, assigneeIDs)
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
	if stageInstance.StageType == "parallel" && mode == "" {
		mode = "all"
	}
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
	return c.advanceWorkflowFrom(ctx, run, stages, stageInstance.Sequence+1, applicant, instance.Payload, comment)
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
	return c.store.InsertWorkflowAction(goContext(ctx), domain.WorkflowAction{
		ID:              utils.NewID("wfa"),
		TenantID:        ctx.TenantID,
		RunID:           run.ID,
		StageInstanceID: stageInstance.ID,
		AccountID:       accountID,
		Action:          action,
		Comment:         strings.TrimSpace(comment),
		CreatedAt:       at,
	})
}

func (c WorkflowService) notifyWorkflowPendingApprovers(ctx RequestContext, instance domain.FormInstance, template domain.FormTemplate, applicant domain.Account, stage domain.WorkflowStageDefinition, assigneeIDs []string) error {
	title := workflowNotificationTemplateTitle(template, instance)
	body := workflowAccountLabel(applicant) + "提交了「" + title + "」，目前在「" + stage.Label + "」待您審核。"
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
		return c.accountIDsForEmployeeIDs(ctx, []string{employee.ManagerEmployeeID})
	case "relative":
		level := stage.Config.RelativeLevel
		if level <= 0 {
			level = 1
		}
		managerID := employee.ManagerEmployeeID
		for i := 0; i < level && managerID != ""; i++ {
			if i == level-1 {
				return c.accountIDsForEmployeeIDs(ctx, []string{managerID})
			}
			manager, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, managerID)
			if err != nil {
				return nil, err
			}
			if !ok {
				break
			}
			managerID = manager.ManagerEmployeeID
		}
		return nil, BadRequest("unable to resolve relative approver")
	case "dept-head":
		return c.resolveRoleAssignees(ctx, employee, []string{"主管", "部門主管", "head"})
	case "hr":
		return c.resolveRoleAssignees(ctx, employee, []string{"hr", "human resource", "人資"})
	case "finance":
		return c.resolveRoleAssignees(ctx, employee, []string{"finance", "財務"})
	case "ceo":
		return c.resolveRoleAssignees(ctx, employee, []string{"ceo", "總經理", "general manager"})
	default:
		return c.accountIDsForEmployeeIDs(ctx, []string{employee.ManagerEmployeeID})
	}
}

func (c WorkflowService) resolveWorkflowApplicantEmployee(ctx RequestContext, applicant domain.Account) (domain.Employee, error) {
	if strings.TrimSpace(applicant.EmployeeID) == "" {
		return domain.Employee{}, BadRequest("applicant employee is required for workflow routing")
	}
	employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, applicant.EmployeeID)
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
		employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, employeeID)
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
	employees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
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
		right, _ := strconv.ParseFloat(strings.TrimSpace(config.Value), 64)
		return compareWorkflowNumbers(left, right, config.Operator)
	default:
		left := workflowPayloadNumber(payload, "hours", "leave_hours", "leaveHours")
		right, _ := strconv.ParseFloat(strings.TrimSpace(config.Value), 64)
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
			nextType := strings.TrimSpace(stages[offset].Type)
			if nextType == "approver" || nextType == "parallel" {
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
		if !ok || run.Status != domain.WorkflowRunStatusRunning {
			continue
		}
		out[run.FormInstanceID] = struct{}{}
	}
	return out, nil
}
