package service

import (
	"encoding/json"
	"sort"
	"strings"

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
	now := c.Now()
	tpl := FormTemplate{
		ID:             utils.NewID("ft"),
		TenantID:       ctx.TenantID,
		Key:            strings.TrimSpace(input.Key),
		Name:           strings.TrimSpace(input.Name),
		Description:    strings.TrimSpace(input.Description),
		Schema:         utils.CopyStringMap(input.Schema),
		Status:         "published",
		CurrentVersion: 1,
		CreatedAt:      now,
		UpdatedAt:      now,
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
	version, err := c.currentFormTemplateVersion(ctx, template)
	if err != nil {
		return FormInstance{}, err
	}
	template = formTemplateAtVersion(template, version)
	now := c.Now()
	instance := FormInstance{
		ID:                 utils.NewID("fi"),
		TenantID:           ctx.TenantID,
		TemplateID:         template.ID,
		TemplateVersionID:  version.ID,
		ApplicantAccountID: account.ID,
		Status:             workflowFormStatusDraft,
		Payload:            workflowPayload(input.Payload),
		SubmittedAt:        now,
		UpdatedAt:          now,
	}
	if err := c.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
		return FormInstance{}, err
	}
	if err := c.replaceFormInstanceFieldProjection(ctx, template, instance); err != nil {
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
			version, err := tx.currentFormTemplateVersion(ctx, template)
			if err != nil {
				return err
			}
			next.TemplateVersionID = version.ID
		}
		template, ok, err := tx.store.GetFormTemplate(goContext(ctx), ctx.TenantID, next.TemplateID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("form template", next.TemplateID)
		}
		version, err := tx.formTemplateVersionForInstance(ctx, template, next)
		if err != nil {
			return err
		}
		template = formTemplateAtVersion(template, version)
		next.Payload = workflowPayload(input.Payload)
		next.UpdatedAt = tx.Now()
		if err := tx.store.UpsertFormInstance(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.replaceFormInstanceFieldProjection(ctx, template, next); err != nil {
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
	var instance FormInstance
	var err error
	if existing, ok, lookupErr := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, idOrTemplateKey); lookupErr != nil {
		return FormInstance{}, lookupErr
	} else if ok {
		instance, err = c.submitExistingDraft(ctx, existing.ID, input.Payload)
	} else {
		instance, err = c.submitNewForm(ctx, idOrTemplateKey, input.Payload)
	}
	if err != nil {
		return FormInstance{}, err
	}
	if err := c.startTemporalFormApprovalWorkflow(ctx, instance); err != nil {
		_ = c.markFormApprovalWorkflowStartFailed(ctx, instance, err)
		return FormInstance{}, err
	}
	return instance, nil
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
		version, err := tx.currentFormTemplateVersion(ctx, nextTemplate)
		if err != nil {
			return err
		}
		nextTemplate = formTemplateAtVersion(nextTemplate, version)
		normalizedPayload, err := tx.validateFormSubmissionPayload(ctx, nextTemplate, payload)
		if err != nil {
			return err
		}
		now := tx.Now()
		next := FormInstance{
			ID:                 utils.NewID("fi"),
			TenantID:           ctx.TenantID,
			TemplateID:         nextTemplate.ID,
			TemplateVersionID:  version.ID,
			ApplicantAccountID: account.ID,
			Status:             workflowFormStatusDraft,
			Payload:            normalizedPayload,
			SubmittedAt:        now,
			UpdatedAt:          now,
		}
		if err := tx.store.UpsertFormInstance(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.replaceFormInstanceFieldProjection(ctx, nextTemplate, next); err != nil {
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
	if _, err := c.Service.Attendance().createLeaveRequestFromSubmittedForm(ctx, instance, template.Key, instance.Payload); err != nil {
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
		version, err := tx.formTemplateVersionForInstance(ctx, template, next)
		if err != nil {
			return err
		}
		template = formTemplateAtVersion(template, version)
		if err := validateWorkflowTemplateSubmittable(template); err != nil {
			return err
		}
		effectivePayload := next.Payload
		if payload != nil {
			effectivePayload = workflowPayload(payload)
		}
		normalizedPayload, err := tx.validateFormSubmissionPayload(ctx, template, effectivePayload)
		if err != nil {
			return err
		}
		effectivePayload = normalizedPayload
		now := tx.Now()
		next.Payload = effectivePayload
		next.SubmittedAt = now
		next.UpdatedAt = now
		if err := tx.store.UpsertFormInstance(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.replaceFormInstanceFieldProjection(ctx, template, next); err != nil {
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
	if template, ok, err := c.store.GetFormTemplate(goContext(ctx), ctx.TenantID, instance.TemplateID); err != nil {
		return FormInstance{}, err
	} else if ok {
		if _, err := c.Service.Attendance().createLeaveRequestFromSubmittedForm(ctx, instance, template.Key, instance.Payload); err != nil {
			return FormInstance{}, err
		}
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
	current, ok, err := c.store.GetFormInstance(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return FormInstance{}, err
	}
	if !ok {
		return FormInstance{}, NotFound("form instance", id)
	}
	if err := requireFormInstanceVisible(current, account, decision); err != nil {
		return FormInstance{}, err
	}
	instance, err := c.signalTemporalFormApprovalWorkflow(ctx, id, domain.FormApprovalWorkflowActionWithdraw, workflowFormStatusCancelled, input.Reason)
	if err != nil {
		return FormInstance{}, err
	}
	if err := authzAudit.Commit(ctx); err != nil {
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
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case domain.FormApprovalWorkflowActionApprove, domain.FormApprovalWorkflowActionReject, domain.FormApprovalWorkflowActionReturn:
	default:
		return FormInstance{}, BadRequest("action must be approve, reject, or return")
	}
	return c.signalTemporalFormApprovalWorkflow(ctx, id, kind, status, comment)
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
	case "pending", "pending_review", "pending-review", "submitted", "in_review", "in-review":
		return workflowFormStatusInReview, nil
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
	return apiTimestamp(t)
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
