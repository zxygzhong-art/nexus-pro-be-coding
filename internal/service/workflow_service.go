package service

import (
	"strings"

	"nexus-pro-be/internal/utils"
)

type WorkflowService struct {
	*Service
	store workflowStore
}

func (c *Service) Workflow() WorkflowService {
	return WorkflowService{Service: c, store: c.store}
}

func (c WorkflowService) ListFormTemplates(ctx RequestContext) ([]FormTemplate, error) {
	if _, _, err := c.requireWorkflowAuthz(ctx, ResourceType("form_template"), ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListFormTemplates(goContext(ctx), ctx.TenantID)
}

func (c WorkflowService) ListFormTemplatePage(ctx RequestContext, page PageRequest) (PageResponse[FormTemplate], error) {
	items, err := c.ListFormTemplates(ctx)
	if err != nil {
		return PageResponse[FormTemplate]{}, err
	}
	items = utils.SortFormTemplates(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

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

func (c WorkflowService) SubmitForm(ctx RequestContext, input SubmitFormInput) (FormInstance, error) {
	templateKey := strings.TrimSpace(input.TemplateKey)
	account, _, err := c.requireWorkflowAuthz(ctx, ResourceFormInstance, ActionSubmit, templateKey)
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
	instance := FormInstance{
		ID:                 utils.NewID("fi"),
		TenantID:           ctx.TenantID,
		TemplateID:         template.ID,
		ApplicantAccountID: account.ID,
		Status:             "submitted",
		Payload:            utils.CopyStringMap(input.Payload),
		SubmittedAt:        c.Now(),
		UpdatedAt:          c.Now(),
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

func (c WorkflowService) ApproveForm(ctx RequestContext, id string, _ ApproveFormInput) (FormInstance, error) {
	_, _, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppWorkflow, ResourceType: ResourceFormInstance, ResourceID: id, Action: ActionApprove},
		AuditTarget{Event: "workflow.form.approve", Resource: string(ResourceFormInstance), Target: id},
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
		if strings.EqualFold(next.Status, "approved") {
			instance = next
			return nil
		}
		next.Status = "approved"
		next.ApprovedBy = ctx.AccountID
		next.UpdatedAt = tx.Now()
		if err := tx.store.UpsertFormInstance(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.audit(ctx, "workflow.form.approve", string(ResourceFormInstance), next.ID, string(SeverityHigh), map[string]any{
			"template_id": next.TemplateID,
			"approved_by": next.ApprovedBy,
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
	c.logInfo(ctx, "form approved",
		"form_instance_id", instance.ID,
		"approved_by", instance.ApprovedBy,
	)
	return instance, nil
}
