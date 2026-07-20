package service

import (
	"sort"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

// FormBuilderCapabilities 回傳 Agent 建構表單所需的 schema、widget、資料源與流程角色能力。
func (c WorkflowService) FormBuilderCapabilities(ctx RequestContext) (domain.FormBuilderCapabilitiesResponse, error) {
	if _, _, err := c.RequireWorkflowAuthz(ctx, domain.ResourceFormDefinitionDraft, domain.ActionRead, ""); err != nil {
		return domain.FormBuilderCapabilitiesResponse{}, err
	}
	dataSources := make([]domain.FormBuilderDataSourceMetadata, 0, len(formDataSourceAllowedFields))
	labels := map[string]string{formDataSourceCurrentUser: "當前用戶", formDataSourceDepartments: "部門", formDataSourceEmployees: "員工", formDataSourcePositions: "職位", formDataSourceLeaveTypes: "假期類型"}
	for id, fields := range formDataSourceAllowedFields {
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		fieldItems := make([]domain.FormDataSourceField, 0, len(keys))
		for _, key := range keys {
			fieldItems = append(fieldItems, domain.FormDataSourceField{Key: key, Label: key, Type: "string"})
		}
		kind := "collection"
		if id == formDataSourceCurrentUser {
			kind = "object"
		}
		dataSources = append(dataSources, domain.FormBuilderDataSourceMetadata{ID: id, Label: labels[id], Kind: kind, Fields: fieldItems})
	}
	sort.Slice(dataSources, func(i, j int) bool { return dataSources[i].ID < dataSources[j].ID })
	targets := []domain.FormBuilderWorkflowTarget{
		{Role: "manager", Label: "直屬主管", Description: "申請人的直接主管"},
		{Role: "relative", Label: "相對層級主管", Description: "按 relative_level 解析的主管"},
		{Role: "dept-head", Label: "部門主管", Description: "申請人所屬部門負責人"},
		{Role: "hr", Label: "HR", Description: "HR 審批角色"},
		{Role: "finance", Label: "財務", Description: "財務審批角色"},
		{Role: "ceo", Label: "總經理", Description: "總經理審批角色"},
		{Role: "applicant", Label: "申請人", Description: "申請人本人"},
	}
	return domain.FormBuilderCapabilitiesResponse{
		SchemaVersion: domain.FormDefinitionSchemaVersion2,
		FieldTypes:    []string{"string", "number", "boolean", "date", "datetime", "string_array", "object"},
		Widgets:       []string{"input", "textarea", "number", "checkbox", "date", "datetime", "select", "radio", "multilist", "autofill", "readonly"},
		DataSources:   dataSources, WorkflowTargets: targets,
	}, nil
}

// ListFormDefinitionDrafts 列出草稿；調用方通過 ownerAccountID 控制“我的草稿”或管理員全量視圖。
func (c WorkflowService) ListFormDefinitionDrafts(ctx RequestContext, ownerAccountID, status string) ([]domain.FormDefinitionDraft, error) {
	if _, _, err := c.RequireWorkflowAuthz(ctx, domain.ResourceFormDefinitionDraft, domain.ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListFormDefinitionDrafts(goContext(ctx), ctx.TenantID, ownerAccountID, status)
}

// GetFormDefinitionDraft 取得單個草稿並執行資源級授權。
func (c WorkflowService) GetFormDefinitionDraft(ctx RequestContext, id string) (domain.FormDefinitionDraft, error) {
	_, decision, err := c.RequireWorkflowAuthz(ctx, domain.ResourceFormDefinitionDraft, domain.ActionRead, id)
	if err != nil {
		return domain.FormDefinitionDraft{}, err
	}
	draft, ok, err := c.store.GetFormDefinitionDraft(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.FormDefinitionDraft{}, err
	}
	if !ok {
		return domain.FormDefinitionDraft{}, NotFound("form definition draft", id)
	}
	if draft.OwnerAccountID != ctx.AccountID && decision.EffectiveScope != domain.ScopeAll && decision.EffectiveScope != domain.ScopeTenant {
		return domain.FormDefinitionDraft{}, Forbidden("draft is owned by another account")
	}
	return draft, nil
}

// CreateFormDefinitionDraft 建立 Agent 可控、不可直接發佈的表單定義草稿。
func (c WorkflowService) CreateFormDefinitionDraft(ctx RequestContext, input domain.CreateFormDefinitionDraftInput) (domain.FormDefinitionDraft, error) {
	if _, _, err := c.RequireWorkflowAuthz(ctx, domain.ResourceFormDefinitionDraft, domain.ActionCreate, ""); err != nil {
		return domain.FormDefinitionDraft{}, err
	}
	if input.AgentRunID != "" && input.ToolCallID != "" {
		if existing, ok, err := c.store.GetFormDefinitionDraftByAgentCall(goContext(ctx), ctx.TenantID, input.AgentRunID, input.ToolCallID); err != nil {
			return domain.FormDefinitionDraft{}, err
		} else if ok {
			return existing, nil
		}
	}
	validation := domain.ValidateFormDefinitionSchemaV2(input.Schema)
	compiled, _ := domain.CompileFormDefinitionSchemaV2(input.Schema)
	now := c.Now()
	draft := domain.FormDefinitionDraft{ID: utils.NewID("fdd"), TenantID: ctx.TenantID, OwnerAccountID: ctx.AccountID, BaseTemplateID: input.BaseTemplateID, SchemaVersion: input.Schema.SchemaVersion, AuthoringSchema: input.Schema, CompiledSchema: compiled, Status: domain.FormDefinitionDraftStatusDraft, Revision: 1, Source: firstNonEmpty(strings.TrimSpace(input.Source), "manual"), AgentID: input.AgentID, AgentRunID: input.AgentRunID, AgentSessionID: input.AgentSessionID, ToolCallID: input.ToolCallID, ValidationResult: validation, CreatedAt: now, UpdatedAt: now}
	if err := c.store.UpsertFormDefinitionDraft(goContext(ctx), draft); err != nil {
		return domain.FormDefinitionDraft{}, err
	}
	_ = c.audit(ctx, "workflow.form_definition_draft.create", string(domain.ResourceFormDefinitionDraft), draft.ID, string(domain.SeverityLow), map[string]any{"source": draft.Source, "valid": validation.Valid})
	return c.GetFormDefinitionDraft(ctx, draft.ID)
}

// UpdateFormDefinitionDraft 更新草稿，必須攜帶當前 revision 以阻止 Agent 覆蓋人工編輯。
func (c WorkflowService) UpdateFormDefinitionDraft(ctx RequestContext, id string, input domain.UpdateFormDefinitionDraftInput) (domain.FormDefinitionDraft, error) {
	_, decision, err := c.RequireWorkflowAuthz(ctx, domain.ResourceFormDefinitionDraft, domain.ActionUpdate, id)
	if err != nil {
		return domain.FormDefinitionDraft{}, err
	}
	draft, ok, err := c.store.GetFormDefinitionDraft(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.FormDefinitionDraft{}, err
	}
	if !ok {
		return domain.FormDefinitionDraft{}, NotFound("form definition draft", id)
	}
	if draft.OwnerAccountID != ctx.AccountID && decision.EffectiveScope != domain.ScopeAll && decision.EffectiveScope != domain.ScopeTenant {
		return domain.FormDefinitionDraft{}, Forbidden("draft is owned by another account")
	}
	if draft.Status == domain.FormDefinitionDraftStatusPublished {
		return domain.FormDefinitionDraft{}, BadRequest("published draft cannot be updated")
	}
	validation := domain.ValidateFormDefinitionSchemaV2(input.Schema)
	compiled, _ := domain.CompileFormDefinitionSchemaV2(input.Schema)
	draft.AuthoringSchema, draft.CompiledSchema, draft.SchemaVersion, draft.ValidationResult = input.Schema, compiled, input.Schema.SchemaVersion, validation
	draft.Source = firstNonEmpty(strings.TrimSpace(input.Source), draft.Source)
	draft.AgentRunID, draft.AgentSessionID, draft.ToolCallID, draft.UpdatedAt = firstNonEmpty(input.AgentRunID, draft.AgentRunID), firstNonEmpty(input.AgentSessionID, draft.AgentSessionID), firstNonEmpty(input.ToolCallID, draft.ToolCallID), c.Now()
	draft.Revision = input.Revision
	if err := c.store.UpsertFormDefinitionDraft(goContext(ctx), draft); err != nil {
		return domain.FormDefinitionDraft{}, err
	}
	_ = c.audit(ctx, "workflow.form_definition_draft.update", string(domain.ResourceFormDefinitionDraft), id, string(domain.SeverityLow), map[string]any{"valid": validation.Valid})
	return c.GetFormDefinitionDraft(ctx, id)
}

// ValidateFormDefinitionDraft 只執行確定性校驗與編譯，不產生髮布副作用。
func (c WorkflowService) ValidateFormDefinitionDraft(ctx RequestContext, id string) (domain.FormDefinitionPreview, error) {
	draft, err := c.GetFormDefinitionDraft(ctx, id)
	if err != nil {
		return domain.FormDefinitionPreview{}, err
	}
	validation := domain.ValidateFormDefinitionSchemaV2(draft.AuthoringSchema)
	compiled, _ := domain.CompileFormDefinitionSchemaV2(draft.AuthoringSchema)
	draft.ValidationResult, draft.CompiledSchema = validation, compiled
	return domain.FormDefinitionPreview{Draft: draft, Validation: validation, CompiledSchema: compiled}, nil
}

// PreviewFormDefinitionDraft 提供 Agent 與前端複用的預覽契約。
func (c WorkflowService) PreviewFormDefinitionDraft(ctx RequestContext, id string) (domain.FormDefinitionPreview, error) {
	return c.ValidateFormDefinitionDraft(ctx, id)
}

// SimulateFormDefinitionWorkflow 只把流程節點解析成可讀的審批序列，不啟動真實工作流。
func (c WorkflowService) SimulateFormDefinitionWorkflow(ctx RequestContext, id string) (domain.FormWorkflowSimulation, error) {
	draft, err := c.GetFormDefinitionDraft(ctx, id)
	if err != nil {
		return domain.FormWorkflowSimulation{}, err
	}
	stages := make([]domain.FormWorkflowSimulationStage, 0, len(draft.AuthoringSchema.Workflow.Stages))
	for _, stage := range draft.AuthoringSchema.Workflow.Stages {
		roles := stringSliceFromAny(stage.Config["roles"])
		if role := stringFromAny(stage.Config["role"]); role != "" {
			roles = append(roles, role)
		}
		stages = append(stages, domain.FormWorkflowSimulationStage{ID: stage.ID, Label: stage.Label, Type: stage.Type, TargetRoles: formDefinitionUniqueStrings(roles)})
	}
	return domain.FormWorkflowSimulation{Stages: stages}, nil
}

// SubmitFormDefinitionDraftForReview 把草稿送入管理員確認發佈隊列。
func (c WorkflowService) SubmitFormDefinitionDraftForReview(ctx RequestContext, id string, revision int64) (domain.FormDefinitionDraft, error) {
	_, _, err := c.RequireWorkflowAuthz(ctx, domain.ResourceFormDefinitionDraft, domain.ActionSubmit, id)
	if err != nil {
		return domain.FormDefinitionDraft{}, err
	}
	draft, ok, err := c.store.GetFormDefinitionDraft(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.FormDefinitionDraft{}, err
	}
	if !ok {
		return domain.FormDefinitionDraft{}, NotFound("form definition draft", id)
	}
	if draft.OwnerAccountID != ctx.AccountID {
		return domain.FormDefinitionDraft{}, Forbidden("only the draft owner can submit it for review")
	}
	if revision != draft.Revision {
		return domain.FormDefinitionDraft{}, domain.Conflict("form definition draft revision is stale")
	}
	validation := domain.ValidateFormDefinitionSchemaV2(draft.AuthoringSchema)
	if !validation.Valid {
		return domain.FormDefinitionDraft{}, domain.ValidationFailed("form definition draft is invalid", validation.Errors)
	}
	draft.ValidationResult, draft.Status, draft.SubmittedAt, draft.UpdatedAt, draft.Revision = validation, domain.FormDefinitionDraftStatusReviewPending, formDefinitionTimePtr(c.Now()), c.Now(), revision
	if err := c.store.UpsertFormDefinitionDraft(goContext(ctx), draft); err != nil {
		return domain.FormDefinitionDraft{}, err
	}
	_ = c.audit(ctx, "workflow.form_definition_draft.submit_review", string(domain.ResourceFormDefinitionDraft), id, string(domain.SeverityMedium), nil)
	return c.GetFormDefinitionDraft(ctx, id)
}

// PublishFormDefinitionDraft 由管理員確認後把 compiled schema 寫入既有 form_templates runtime。
func (c WorkflowService) PublishFormDefinitionDraft(ctx RequestContext, id string, revision int64) (domain.FormDefinitionDraft, error) {
	if _, _, err := c.RequireWorkflowAuthz(ctx, domain.ResourceFormDefinitionDraft, domain.ActionApprove, id); err != nil {
		return domain.FormDefinitionDraft{}, err
	}
	draft, ok, err := c.store.GetFormDefinitionDraft(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.FormDefinitionDraft{}, err
	}
	if !ok {
		return domain.FormDefinitionDraft{}, NotFound("form definition draft", id)
	}
	if draft.Status == domain.FormDefinitionDraftStatusPublished {
		return draft, nil
	}
	if draft.Status != domain.FormDefinitionDraftStatusReviewPending || revision != draft.Revision {
		return domain.FormDefinitionDraft{}, domain.Conflict("draft must be review_pending at the current revision")
	}
	validation := domain.ValidateFormDefinitionSchemaV2(draft.AuthoringSchema)
	compiled, _ := domain.CompileFormDefinitionSchemaV2(draft.AuthoringSchema)
	if !validation.Valid {
		return domain.FormDefinitionDraft{}, domain.ValidationFailed("form definition draft is invalid", validation.Errors)
	}
	templateID := utils.NewID("ft")
	key := firstNonEmpty(workspaceFormDesignSlug(draft.AuthoringSchema.Name), "form") + "-" + strings.TrimPrefix(draft.ID, "fdd_")
	now := c.Now()
	template := domain.FormTemplate{ID: templateID, TenantID: ctx.TenantID, Key: key, Name: draft.AuthoringSchema.Name, Description: draft.AuthoringSchema.Description, Schema: compiled, Status: "published", CurrentVersion: 1, CreatedAt: now, UpdatedAt: now}
	if err := c.withTransaction(ctx, func(tx WorkflowService) error {
		if err := tx.store.UpsertFormTemplate(goContext(ctx), template); err != nil {
			return err
		}
		draft.CompiledSchema, draft.ValidationResult, draft.Status, draft.PublishedTemplateID, draft.UpdatedAt, draft.Revision = compiled, validation, domain.FormDefinitionDraftStatusPublished, templateID, now, revision
		if err := tx.store.UpsertFormDefinitionDraft(goContext(ctx), draft); err != nil {
			return err
		}
		return tx.audit(ctx, "workflow.form_definition_draft.publish", string(domain.ResourceFormDefinitionDraft), id, string(domain.SeverityHigh), map[string]any{"template_id": templateID})
	}); err != nil {
		return domain.FormDefinitionDraft{}, err
	}
	return c.GetFormDefinitionDraft(ctx, id)
}

// formDefinitionTimePtr 建立 UTC 時間指針，避免生命週期更新時共享可變對象。
func formDefinitionTimePtr(value time.Time) *time.Time { value = value.UTC(); return &value }

// formDefinitionUniqueStrings 保持流程角色順序並去重。
func formDefinitionUniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
