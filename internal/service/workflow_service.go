package service

import "strings"

type WorkflowService struct {
	*Service
}

func (c *Service) Workflow() WorkflowService {
	return WorkflowService{Service: c}
}

func (c *Service) ListFormTemplates(ctx RequestContext) ([]FormTemplate, error) {
	return c.Workflow().ListFormTemplates(ctx)
}

func (c *Service) ListFormTemplatePage(ctx RequestContext, page PageRequest) (PageResponse[FormTemplate], error) {
	return c.Workflow().ListFormTemplatePage(ctx, page)
}

func (c *Service) CreateFormTemplate(ctx RequestContext, input CreateFormTemplateInput) (FormTemplate, error) {
	return c.Workflow().CreateFormTemplate(ctx, input)
}

func (c *Service) SubmitForm(ctx RequestContext, input SubmitFormInput) (FormInstance, error) {
	return c.Workflow().SubmitForm(ctx, input)
}

func (c WorkflowService) ListFormTemplates(ctx RequestContext) ([]FormTemplate, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return nil, err
	}
	return c.store.ListFormTemplates(goContext(ctx), ctx.TenantID)
}

func (c WorkflowService) ListFormTemplatePage(ctx RequestContext, page PageRequest) (PageResponse[FormTemplate], error) {
	items, err := c.ListFormTemplates(ctx)
	if err != nil {
		return PageResponse[FormTemplate]{}, err
	}
	items = sortFormTemplates(items, page.Sort)
	return pageResponse(items, page), nil
}

func (c WorkflowService) CreateFormTemplate(ctx RequestContext, input CreateFormTemplateInput) (FormTemplate, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return FormTemplate{}, err
	}
	if strings.TrimSpace(input.Key) == "" || strings.TrimSpace(input.Name) == "" {
		return FormTemplate{}, BadRequest("template key and name are required")
	}
	tpl := FormTemplate{
		ID:          newID("ft"),
		TenantID:    ctx.TenantID,
		Key:         strings.TrimSpace(input.Key),
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		Schema:      copyStringMap(input.Schema),
		CreatedAt:   c.Now(),
	}
	if err := c.store.UpsertFormTemplate(goContext(ctx), tpl); err != nil {
		return FormTemplate{}, err
	}
	if err := c.audit(ctx, "workflow.form_template.create", "form_template", tpl.ID, "medium", map[string]any{"key": tpl.Key}); err != nil {
		return FormTemplate{}, err
	}
	return tpl, nil
}

func (c WorkflowService) SubmitForm(ctx RequestContext, input SubmitFormInput) (FormInstance, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return FormInstance{}, err
	}
	if strings.TrimSpace(input.TemplateKey) == "" {
		return FormInstance{}, BadRequest("template_key is required")
	}
	template, ok, err := c.store.GetFormTemplateByKey(goContext(ctx), ctx.TenantID, input.TemplateKey)
	if err != nil {
		return FormInstance{}, err
	}
	if !ok {
		return FormInstance{}, NotFound("form template", input.TemplateKey)
	}
	instance := FormInstance{
		ID:                 newID("fi"),
		TenantID:           ctx.TenantID,
		TemplateID:         template.ID,
		ApplicantAccountID: account.ID,
		Status:             "submitted",
		Payload:            copyStringMap(input.Payload),
		SubmittedAt:        c.Now(),
		UpdatedAt:          c.Now(),
	}
	if err := c.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
		return FormInstance{}, err
	}
	if err := c.audit(ctx, "workflow.form.submit", "form_instance", instance.ID, "medium", map[string]any{"template_key": template.Key}); err != nil {
		return FormInstance{}, err
	}
	return instance, nil
}
