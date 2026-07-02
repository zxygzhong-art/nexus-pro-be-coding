package service

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"nexus-pro-be/internal/utils"
)

const (
	workflowFormStatusDraft     = "draft"
	workflowFormStatusSubmitted = "submitted"
	workflowFormStatusApproved  = "approved"
	workflowFormStatusRejected  = "rejected"
	workflowFormStatusCancelled = "cancelled"
)

// WorkflowService implements form template and form instance workflows.
type WorkflowService struct {
	*Service
	store workflowStore
}

// Workflow returns the workflow service facade.
func (c *Service) Workflow() WorkflowService {
	return WorkflowService{Service: c, store: c.store}
}

// ListFormTemplates returns form templates visible to the current account.
func (c WorkflowService) ListFormTemplates(ctx RequestContext) ([]FormTemplate, error) {
	if _, _, err := c.requireWorkflowAuthz(ctx, ResourceType("form_template"), ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListFormTemplates(goContext(ctx), ctx.TenantID)
}

// ListFormTemplatePage returns paginated form templates.
func (c WorkflowService) ListFormTemplatePage(ctx RequestContext, page PageRequest) (PageResponse[FormTemplate], error) {
	items, err := c.ListFormTemplates(ctx)
	if err != nil {
		return PageResponse[FormTemplate]{}, err
	}
	items = utils.SortFormTemplates(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreateFormTemplate creates a reusable workflow form template.
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

// ListFormInstancePage returns submitted workflow forms after permission and ownership filters.
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

// ReviewQueue groups workflow forms into the notification page review buckets.
func (c WorkflowService) ReviewQueue(ctx RequestContext) (WorkflowReviewQueueResponse, error) {
	items, account, err := c.listFormInstances(ctx, FormInstanceQuery{})
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
		switch normalizeWorkflowStatus(item.Status) {
		case "approved", "rejected", "cancelled", "canceled":
			out.AlreadyReviewed = append(out.AlreadyReviewed, projected)
		default:
			out.PendingReview = append(out.PendingReview, projected)
		}
		if workflowPayloadMentionsAccount(item.Payload, account.ID) {
			out.Notified = append(out.Notified, projected)
		}
	}
	return out, nil
}

// SaveFormDraft creates a draft form instance for the current account.
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

// UpdateFormDraft updates an existing draft owned by the current account or visible to an admin.
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

// DeleteFormDraft removes an existing draft after ownership and status checks.
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

// SubmitForm creates a submitted instance from a template key or submits an existing draft id.
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

// submitNewForm creates a submitted form instance for the current account.
func (c WorkflowService) submitNewForm(ctx RequestContext, templateKey string, payload map[string]any) (FormInstance, error) {
	account, _, err := c.requireWorkflowAuthz(ctx, ResourceFormInstance, ActionSubmit, "")
	if err != nil {
		return FormInstance{}, err
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
		Status:             workflowFormStatusSubmitted,
		Payload:            workflowPayload(payload),
		SubmittedAt:        now,
		UpdatedAt:          now,
	}
	if err := c.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
		return FormInstance{}, err
	}
	if err := c.audit(ctx, "workflow.form.submit", "form_instance", instance.ID, "medium", map[string]any{"template_key": template.Key}); err != nil {
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

// submitExistingDraft turns a saved draft into a submitted workflow instance.
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
		if !strings.EqualFold(next.Status, workflowFormStatusDraft) {
			return BadRequest("only draft form instances can be submitted by id")
		}
		now := tx.Now()
		next.Status = workflowFormStatusSubmitted
		if payload != nil {
			next.Payload = workflowPayload(payload)
		}
		next.SubmittedAt = now
		next.UpdatedAt = now
		if err := tx.store.UpsertFormInstance(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.audit(ctx, "workflow.form.submit", string(ResourceFormInstance), next.ID, string(SeverityMedium), map[string]any{
			"template_id": next.TemplateID,
			"from_draft":  true,
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

// ApproveForm marks a submitted form instance as approved.
func (c WorkflowService) ApproveForm(ctx RequestContext, id string, _ ApproveFormInput) (FormInstance, error) {
	instance, err := c.reviewForm(ctx, id, "approve", "approved", "")
	if err != nil {
		return FormInstance{}, err
	}
	c.logInfo(ctx, "form approved",
		"form_instance_id", instance.ID,
		"approved_by", instance.ApprovedBy,
	)
	return instance, nil
}

// RejectForm marks a submitted form instance as rejected and records reviewer metadata.
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

// ReturnForm sends a submitted form back to the applicant for revision.
func (c WorkflowService) ReturnForm(ctx RequestContext, id string, input ReturnFormInput) (FormInstance, error) {
	instance, err := c.reviewForm(ctx, id, "return", "rejected", input.Reason)
	if err != nil {
		return FormInstance{}, err
	}
	c.logInfo(ctx, "form returned",
		"form_instance_id", instance.ID,
		"returned_by", instance.ApprovedBy,
	)
	return instance, nil
}

// CancelForm lets an applicant cancel a submitted workflow form before approval.
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
			if err := tx.Service.Attendance().applyLeaveWorkflowReview(ctx, next, "cancel", workflowFormStatusCancelled); err != nil {
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
		if err := tx.Service.Attendance().applyLeaveWorkflowReview(ctx, next, "cancel", workflowFormStatusCancelled); err != nil {
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

// DuplicateForm copies an existing visible form into a new draft for the current account.
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

// ExportForm returns a JSON file payload for a visible form instance.
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

// BulkReviewForms applies one notification-page review action to multiple form instances.
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

// reviewForm updates one form review state and records the visible review timeline entry.
func (c WorkflowService) reviewForm(ctx RequestContext, id string, kind string, status string, comment string) (FormInstance, error) {
	action := ActionUpdate
	if kind == "approve" {
		action = ActionApprove
	}
	_, _, authzAudit, err := c.Authorize(ctx,
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
			if err := tx.Service.Attendance().applyLeaveWorkflowReview(ctx, next, kind, status); err != nil {
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
		if err := tx.Service.Attendance().applyLeaveWorkflowReview(ctx, next, kind, status); err != nil {
			return err
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
		instance = next
		return nil
	}); err != nil {
		return FormInstance{}, err
	}
	return instance, nil
}

func normalizeBulkReviewAction(action string) (string, string, error) {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "approve", "approved":
		return "approve", "approved", nil
	case "reject", "rejected", "deny", "denied", "disapprove", "not_approve":
		return "reject", "rejected", nil
	case "return", "returned":
		return "return", "rejected", nil
	default:
		return "", "", BadRequest("action must be approve, reject, or return")
	}
}

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

func normalizeWorkflowStatus(status string) string {
	return strings.TrimSpace(strings.ToLower(status))
}

func workflowReviewStatus(status string) string {
	switch status {
	case "approved":
		return "success"
	case "rejected":
		return "destructive"
	case "cancelled", "canceled":
		return "secondary"
	default:
		return "warning"
	}
}

func workflowReviewStatusText(status string) string {
	switch status {
	case "approved":
		return "已核准"
	case "rejected":
		return "已退回"
	case "cancelled", "canceled":
		return "已取消"
	default:
		return "审核中"
	}
}

func workflowApplicantLabel(account Account, employee Employee) string {
	name := utils.FirstNonEmpty(employee.Name, account.DisplayName, account.Email, account.ID)
	if employee.OrgUnitID != "" {
		return name + "（" + employee.OrgUnitID + "）"
	}
	return name
}

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

func normalizeWorkflowPendingStatus(status string) bool {
	switch normalizeWorkflowStatus(status) {
	case "approved", "rejected", "cancelled", "canceled":
		return false
	default:
		return true
	}
}

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

func workflowPayloadMentionsAccount(payload map[string]any, accountID string) bool {
	if accountID == "" {
		return false
	}
	for _, key := range []string{"notify_account_ids", "notified_account_ids", "cc_account_ids"} {
		if stringSliceContains(payload[key], accountID) {
			return true
		}
	}
	return false
}

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

// workflowPayload returns a non-nil copy so stored instances never share caller maps.
func workflowPayload(payload map[string]any) map[string]any {
	next := utils.CopyStringMap(payload)
	if next == nil {
		return map[string]any{}
	}
	return next
}

// requireFormInstanceVisible enforces self-scoped workflow decisions on direct instance operations.
func requireFormInstanceVisible(instance FormInstance, account Account, decision CheckResult) error {
	switch decision.Scope {
	case ScopeSelf, ScopeOwn:
		if instance.ApplicantAccountID != account.ID {
			return NotFound("form instance", instance.ID)
		}
	}
	return nil
}

// safeWorkflowFileName keeps exported form filenames portable across common filesystems.
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
