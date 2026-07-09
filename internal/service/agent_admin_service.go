package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

// ListModels 列出工作區模型設定。
func (c AgentService) ListModels(ctx RequestContext) ([]domain.AgentModel, error) {
	if _, _, err := c.requireAgentAuthz(ctx, ResourceModel, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListAgentModels(goContext(ctx), ctx.TenantID)
}

// GetModel 取得工作區模型設定。
func (c AgentService) GetModel(ctx RequestContext, id string) (domain.AgentModel, error) {
	if _, _, err := c.requireAgentAuthz(ctx, ResourceModel, ActionRead, id); err != nil {
		return domain.AgentModel{}, err
	}
	return c.currentAgentModel(ctx, id)
}

// CreateModel 建立工作區模型設定。
func (c AgentService) CreateModel(ctx RequestContext, input domain.CreateAgentModelInput) (domain.AgentModel, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceModel, ActionCreate, "")
	if err != nil {
		return domain.AgentModel{}, err
	}
	now := c.Now()
	model, err := c.normalizeAgentModel(ctx, domain.AgentModel{
		ID:              utils.NewID("amodel"),
		TenantID:        ctx.TenantID,
		Name:            input.Name,
		Provider:        input.Provider,
		ModelName:       input.ModelName,
		LiteLLMModel:    input.LiteLLMModel,
		IsDefault:       input.IsDefault,
		Status:          domain.AgentModelStatus(input.Status),
		FallbackModelID: input.FallbackModelID,
		TimeoutSeconds:  input.TimeoutSeconds,
		MonthlyQuota:    input.MonthlyQuota,
		LastTestStatus:  "untested",
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		return domain.AgentModel{}, err
	}
	if model.IsDefault {
		if err := c.store.ClearDefaultAgentModel(goContext(ctx), ctx.TenantID, model.ID); err != nil {
			return domain.AgentModel{}, err
		}
	}
	if err := c.store.UpsertAgentModel(goContext(ctx), model); err != nil {
		return domain.AgentModel{}, err
	}
	if err := c.recordAgentAdminAudit(ctx, account, "model", model.ID, model.Name, "create", "model created"); err != nil {
		return domain.AgentModel{}, err
	}
	return model, nil
}

// UpdateModel 更新工作區模型設定。
func (c AgentService) UpdateModel(ctx RequestContext, id string, input domain.UpdateAgentModelInput) (domain.AgentModel, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceModel, ActionUpdate, id)
	if err != nil {
		return domain.AgentModel{}, err
	}
	model, err := c.currentAgentModel(ctx, id)
	if err != nil {
		return domain.AgentModel{}, err
	}
	if input.Name != nil {
		model.Name = *input.Name
	}
	if input.Provider != nil {
		model.Provider = *input.Provider
	}
	if input.ModelName != nil {
		model.ModelName = *input.ModelName
	}
	if input.LiteLLMModel != nil {
		model.LiteLLMModel = *input.LiteLLMModel
	}
	if input.IsDefault != nil {
		model.IsDefault = *input.IsDefault
	}
	if input.Status != nil {
		model.Status = domain.AgentModelStatus(*input.Status)
	}
	if input.FallbackModelID != nil {
		model.FallbackModelID = *input.FallbackModelID
	}
	if input.TimeoutSeconds != nil {
		model.TimeoutSeconds = *input.TimeoutSeconds
	}
	if input.MonthlyQuota != nil {
		model.MonthlyQuota = *input.MonthlyQuota
	}
	model.UpdatedAt = c.Now()
	model, err = c.normalizeAgentModel(ctx, model)
	if err != nil {
		return domain.AgentModel{}, err
	}
	if model.IsDefault {
		if err := c.store.ClearDefaultAgentModel(goContext(ctx), ctx.TenantID, model.ID); err != nil {
			return domain.AgentModel{}, err
		}
	}
	if err := c.store.UpsertAgentModel(goContext(ctx), model); err != nil {
		return domain.AgentModel{}, err
	}
	if err := c.recordAgentAdminAudit(ctx, account, "model", model.ID, model.Name, "update", "model updated"); err != nil {
		return domain.AgentModel{}, err
	}
	return model, nil
}

// DeleteModel 刪除工作區模型設定；已被 Agent 使用時阻擋。
func (c AgentService) DeleteModel(ctx RequestContext, id string) (domain.AgentModel, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceModel, ActionDelete, id)
	if err != nil {
		return domain.AgentModel{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.AgentModel{}, BadRequest("id is required")
	}
	count, err := c.store.CountAgentDefinitionsByModel(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.AgentModel{}, err
	}
	if count > 0 {
		return domain.AgentModel{}, Conflict("agent model is used by agent definitions")
	}
	model, ok, err := c.store.DeleteAgentModel(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.AgentModel{}, err
	}
	if !ok {
		return domain.AgentModel{}, NotFound("agent model", id)
	}
	if err := c.recordAgentAdminAudit(ctx, account, "model", model.ID, model.Name, "delete", "model deleted"); err != nil {
		return domain.AgentModel{}, err
	}
	return model, nil
}

// TestModel 執行本地模型設定檢查並寫回 last_test_*；不宣稱外部 provider 已連通。
func (c AgentService) TestModel(ctx RequestContext, id string) (domain.AgentModel, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceModel, ActionUpdate, id)
	if err != nil {
		return domain.AgentModel{}, err
	}
	model, err := c.currentAgentModel(ctx, id)
	if err != nil {
		return domain.AgentModel{}, err
	}
	status := "ok"
	message := "local configuration check ok; provider connectivity not verified"
	if model.Status == domain.AgentModelStatusDisabled {
		status = "failed"
		message = "model is disabled"
	}
	model, ok, err := c.store.UpdateAgentModelTestResult(goContext(ctx), ctx.TenantID, model.ID, status, message, c.Now())
	if err != nil {
		return domain.AgentModel{}, err
	}
	if !ok {
		return domain.AgentModel{}, NotFound("agent model", id)
	}
	if err := c.recordAgentAdminAudit(ctx, account, "model", model.ID, model.Name, "test", message); err != nil {
		return domain.AgentModel{}, err
	}
	return model, nil
}

// ListDefinitions 列出工作區 Agent。
func (c AgentService) ListDefinitions(ctx RequestContext) ([]domain.AgentDefinition, error) {
	if _, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListAgentDefinitions(goContext(ctx), ctx.TenantID)
}

// GetDefinition 取得工作區 Agent。
func (c AgentService) GetDefinition(ctx RequestContext, id string) (domain.AgentDefinition, error) {
	if _, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionRead, id); err != nil {
		return domain.AgentDefinition{}, err
	}
	return c.currentAgentDefinition(ctx, id)
}

// CreateDefinition 建立工作區 Agent。
func (c AgentService) CreateDefinition(ctx RequestContext, input domain.CreateAgentDefinitionInput) (domain.AgentDefinition, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionCreate, "")
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	if strings.TrimSpace(input.TemplateID) != "" {
		template, ok := findAgentTemplate(input.TemplateID)
		if !ok {
			return domain.AgentDefinition{}, BadRequest("template_id is invalid")
		}
		if strings.TrimSpace(input.Name) == "" {
			input.Name = template.Name
		}
		if strings.TrimSpace(input.Description) == "" {
			input.Description = template.Description
		}
		if strings.TrimSpace(input.Emoji) == "" {
			input.Emoji = template.Emoji
		}
		if strings.TrimSpace(input.Category) == "" {
			input.Category = string(template.Category)
		}
		if strings.TrimSpace(input.SystemPrompt) == "" {
			input.SystemPrompt = template.SystemPrompt
		}
		if len(input.Tools) == 0 {
			input.Tools = template.Tools
		}
	}
	now := c.Now()
	agent, err := c.normalizeAgentDefinition(ctx, domain.AgentDefinition{
		ID:                 utils.NewID("adef"),
		TenantID:           ctx.TenantID,
		Name:               input.Name,
		Description:        input.Description,
		Emoji:              input.Emoji,
		Category:           domain.AgentCategory(input.Category),
		ModelID:            input.ModelID,
		FallbackModelID:    input.FallbackModelID,
		SystemPrompt:       input.SystemPrompt,
		Tools:              input.Tools,
		Status:             domain.AgentDefinitionStatus(input.Status),
		Visibility:         domain.AgentVisibility(input.Visibility),
		VisibilityTargets:  input.VisibilityTargets,
		TimeoutSeconds:     input.TimeoutSeconds,
		Version:            1,
		CreatedByAccountID: account.ID,
		UpdatedByAccountID: account.ID,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	if err := c.store.UpsertAgentDefinition(goContext(ctx), agent); err != nil {
		return domain.AgentDefinition{}, err
	}
	if err := c.snapshotAgentDefinition(ctx, agent, account.ID, "initial version"); err != nil {
		return domain.AgentDefinition{}, err
	}
	if err := c.recordAgentAdminAudit(ctx, account, "agent", agent.ID, agent.Name, "create", "agent created"); err != nil {
		return domain.AgentDefinition{}, err
	}
	return agent, nil
}

// UpdateDefinition 更新工作區 Agent；prompt/tools/model/status 變動會建立新版本。
func (c AgentService) UpdateDefinition(ctx RequestContext, id string, input domain.UpdateAgentDefinitionInput) (domain.AgentDefinition, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionUpdate, id)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	agent, err := c.currentAgentDefinition(ctx, id)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	beforePrompt, beforeModel, beforeStatus := agent.SystemPrompt, agent.ModelID, agent.Status
	beforeTools := strings.Join(agent.Tools, "\x00")
	if input.Name != nil {
		agent.Name = *input.Name
	}
	if input.Description != nil {
		agent.Description = *input.Description
	}
	if input.Emoji != nil {
		agent.Emoji = *input.Emoji
	}
	if input.Category != nil {
		agent.Category = domain.AgentCategory(*input.Category)
	}
	if input.ModelID != nil {
		agent.ModelID = *input.ModelID
	}
	if input.FallbackModelID != nil {
		agent.FallbackModelID = *input.FallbackModelID
	}
	if input.SystemPrompt != nil {
		agent.SystemPrompt = *input.SystemPrompt
	}
	if input.Tools != nil {
		agent.Tools = input.Tools
	}
	if input.Status != nil {
		agent.Status = domain.AgentDefinitionStatus(*input.Status)
	}
	if input.Visibility != nil {
		agent.Visibility = domain.AgentVisibility(*input.Visibility)
	}
	if input.VisibilityTargets != nil {
		agent.VisibilityTargets = input.VisibilityTargets
	}
	if input.TimeoutSeconds != nil {
		agent.TimeoutSeconds = *input.TimeoutSeconds
	}
	agent.UpdatedByAccountID = account.ID
	agent.UpdatedAt = c.Now()
	versionChanged := beforePrompt != agent.SystemPrompt || beforeModel != agent.ModelID || beforeTools != strings.Join(agent.Tools, "\x00") || beforeStatus != agent.Status
	if versionChanged {
		agent.Version++
	}
	agent, err = c.normalizeAgentDefinition(ctx, agent)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	if err := c.store.UpsertAgentDefinition(goContext(ctx), agent); err != nil {
		return domain.AgentDefinition{}, err
	}
	if versionChanged {
		note := strings.TrimSpace(input.VersionNote)
		if note == "" {
			note = "updated"
		}
		if err := c.snapshotAgentDefinition(ctx, agent, account.ID, note); err != nil {
			return domain.AgentDefinition{}, err
		}
	}
	if err := c.recordAgentAdminAudit(ctx, account, "agent", agent.ID, agent.Name, "update", "agent updated"); err != nil {
		return domain.AgentDefinition{}, err
	}
	return agent, nil
}

// DeleteDefinition 刪除工作區 Agent。
func (c AgentService) DeleteDefinition(ctx RequestContext, id string) (domain.AgentDefinition, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionDelete, id)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	agent, ok, err := c.store.DeleteAgentDefinition(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	if !ok {
		return domain.AgentDefinition{}, NotFound("agent definition", id)
	}
	if err := c.recordAgentAdminAudit(ctx, account, "agent", agent.ID, agent.Name, "delete", "agent deleted"); err != nil {
		return domain.AgentDefinition{}, err
	}
	return agent, nil
}

// Trial 試用 Agent；若已注入 AgentChatRuntime 則走真實 runtime，否則回退 mock。
func (c AgentService) Trial(ctx RequestContext, id string, input domain.AgentTrialInput) (domain.AgentTrialResult, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionUpdate, id)
	if err != nil {
		return domain.AgentTrialResult{}, err
	}
	agent, err := c.currentAgentDefinition(ctx, id)
	if err != nil {
		return domain.AgentTrialResult{}, err
	}
	if agent.Status != domain.AgentDefinitionStatusPublished {
		return domain.AgentTrialResult{}, BadRequest("agent definition must be published")
	}
	model, err := c.currentAgentModel(ctx, agent.ModelID)
	if err != nil {
		return domain.AgentTrialResult{}, err
	}
	start := c.Now()
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return domain.AgentTrialResult{}, BadRequest("message is required")
	}
	reply, toolsUsed, err := c.trialReply(ctx, agent, model, message)
	if err != nil {
		return domain.AgentTrialResult{}, err
	}
	latencyMs := int(c.Now().Sub(start).Milliseconds())
	if latencyMs <= 0 {
		latencyMs = 1
	}
	if _, ok, err := c.store.UpdateAgentDefinitionUsage(goContext(ctx), ctx.TenantID, agent.ID, true, latencyMs, message, c.Now()); err != nil {
		return domain.AgentTrialResult{}, err
	} else if !ok {
		return domain.AgentTrialResult{}, NotFound("agent definition", id)
	}
	if err := c.recordAgentAdminAudit(ctx, account, "agent", agent.ID, agent.Name, "trial", "agent trial executed"); err != nil {
		return domain.AgentTrialResult{}, err
	}
	return domain.AgentTrialResult{Reply: reply, LatencyMs: latencyMs, ToolsUsed: toolsUsed, ModelName: model.ModelName}, nil
}

// trialReply 優先使用已注入的 AgentChatRuntime，並只回報 runtime 實際成功呼叫的工具。
func (c AgentService) trialReply(ctx RequestContext, agent domain.AgentDefinition, model domain.AgentModel, message string) (string, []string, error) {
	if c.agentChatRuntime == nil {
		return fmt.Sprintf("[%s] mock reply using %s: %s", agent.Name, model.ModelName, message), []string{}, nil
	}
	var answer strings.Builder
	toolsUsed := make([]string, 0)
	seenTools := map[string]struct{}{}
	emit := func(_ context.Context, event domain.AgentChatEvent) error {
		if event.Event == domain.AgentChatEventMessageDelta {
			answer.WriteString(event.Delta)
			return nil
		}
		if event.Event == domain.AgentChatEventToolResult && event.Status == "ok" && strings.TrimSpace(event.Name) != "" {
			if _, exists := seenTools[event.Name]; !exists {
				seenTools[event.Name] = struct{}{}
				toolsUsed = append(toolsUsed, event.Name)
			}
		}
		return nil
	}
	baseCtx := WithAgentRequestContext(goContext(ctx), ctx)
	req := AgentChatRuntimeRequest{
		RequestContext: ctx,
		RunID:          "trial-" + agent.ID,
		SessionID:      "trial-" + agent.ID,
		Message:        message,
		Mode:           "trial",
		Tools:          c.filteredAgentReadOnlyTools(ctx, agent.Tools, true, emit),
	}
	if err := c.agentChatRuntime.RunAgentChat(baseCtx, req, emit); err != nil {
		return "", []string{}, err
	}
	reply := strings.TrimSpace(answer.String())
	if reply == "" {
		return fmt.Sprintf("[%s] mock reply using %s: %s", agent.Name, model.ModelName, message), toolsUsed, nil
	}
	return reply, toolsUsed, nil
}

// RollbackDefinition 以歷史版本建立新的目前版本。
func (c AgentService) RollbackDefinition(ctx RequestContext, id string, input domain.RollbackAgentDefinitionInput) (domain.AgentDefinition, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionUpdate, id)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	agent, err := c.currentAgentDefinition(ctx, id)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	version, ok, err := c.store.GetAgentDefinitionVersion(goContext(ctx), ctx.TenantID, agent.ID, input.Version)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	if !ok {
		return domain.AgentDefinition{}, NotFound("agent definition version", fmt.Sprintf("%s:%d", id, input.Version))
	}
	agent.SystemPrompt = version.SystemPrompt
	agent.Tools = version.Tools
	agent.ModelID = version.ModelID
	agent.Version++
	agent.UpdatedByAccountID = account.ID
	agent.UpdatedAt = c.Now()
	agent, err = c.normalizeAgentDefinition(ctx, agent)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	if err := c.store.UpsertAgentDefinition(goContext(ctx), agent); err != nil {
		return domain.AgentDefinition{}, err
	}
	if err := c.snapshotAgentDefinition(ctx, agent, account.ID, fmt.Sprintf("rollback to v%d", input.Version)); err != nil {
		return domain.AgentDefinition{}, err
	}
	if err := c.recordAgentAdminAudit(ctx, account, "agent", agent.ID, agent.Name, "rollback", fmt.Sprintf("rollback to v%d", input.Version)); err != nil {
		return domain.AgentDefinition{}, err
	}
	return agent, nil
}

// ListDefinitionVersions 列出 Agent 版本。
func (c AgentService) ListDefinitionVersions(ctx RequestContext, id string) ([]domain.AgentDefinitionVersion, error) {
	if _, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionRead, id); err != nil {
		return nil, err
	}
	return c.store.ListAgentDefinitionVersions(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
}

// ListAgentAudits 列出 Agent 管理審計。
func (c AgentService) ListAgentAudits(ctx RequestContext) ([]domain.AgentAudit, error) {
	if _, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListAgentAudits(goContext(ctx), ctx.TenantID)
}

// ListAudits 列出 Agent 管理審計。
func (c AgentService) ListAudits(ctx RequestContext) ([]domain.AgentAudit, error) {
	return c.ListAgentAudits(ctx)
}

// Templates 回傳靜態 Agent 模板。
func (c AgentService) Templates(ctx RequestContext) ([]domain.AgentTemplate, error) {
	if _, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionRead, ""); err != nil {
		return nil, err
	}
	return agentTemplates(), nil
}

// ListTemplates 回傳靜態 Agent 模板。
func (c AgentService) ListTemplates(ctx RequestContext) ([]domain.AgentTemplate, error) {
	return c.Templates(ctx)
}

// Tools 回傳靜態可用工具目錄。
func (c AgentService) Tools(ctx RequestContext) ([]domain.AgentToolMeta, error) {
	if _, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionRead, ""); err != nil {
		return nil, err
	}
	return agentToolCatalog(), nil
}

// ListTools 回傳靜態可用工具目錄。
func (c AgentService) ListTools(ctx RequestContext) ([]domain.AgentToolMeta, error) {
	return c.Tools(ctx)
}

func agentTemplates() []domain.AgentTemplate {
	return []domain.AgentTemplate{
		{ID: "hr-helper", Name: "HR Helper", Description: "Answer HR policy and workflow questions.", Emoji: "HR", Category: domain.AgentCategoryWorkflow, SystemPrompt: "You are a helpful HR assistant.", Tools: []string{"get_my_profile", "my_leave_balances"}},
		{ID: "analytics", Name: "Analytics Partner", Description: "Summarize workspace insights.", Emoji: "AI", Category: domain.AgentCategoryAnalytics, SystemPrompt: "You summarize operational data clearly.", Tools: []string{"workspace_insights"}},
	}
}

func findAgentTemplate(id string) (domain.AgentTemplate, bool) {
	id = strings.TrimSpace(id)
	for _, template := range agentTemplates() {
		if template.ID == id {
			return template, true
		}
	}
	return domain.AgentTemplate{}, false
}

func agentToolCatalog() []domain.AgentToolMeta {
	return []domain.AgentToolMeta{
		{Value: "knowledge.search", Label: "Knowledge Search", Description: "Search tenant knowledge content.", Readonly: true, RequiredPermission: "agent.tool.call:knowledge.search"},
		{Value: "get_my_profile", Label: "My Profile", Description: "Read current account profile.", Readonly: true, RequiredPermission: "agent.tool.call:get_my_profile"},
		{Value: "list_employees", Label: "List Employees", Description: "Read employee summaries.", Readonly: true, RequiredPermission: "agent.tool.call:list_employees"},
		{Value: "get_employee", Label: "Get Employee", Description: "Read one employee summary.", Readonly: true, RequiredPermission: "agent.tool.call:get_employee"},
		{Value: "my_leave_balances", Label: "My Leave Balances", Description: "Read current account leave balances.", Readonly: true, RequiredPermission: "agent.tool.call:my_leave_balances"},
		{Value: "my_clock_records", Label: "My Clock Records", Description: "Read current account clock records.", Readonly: true, RequiredPermission: "agent.tool.call:my_clock_records"},
		{Value: "my_pending_reviews", Label: "My Pending Reviews", Description: "Read pending workflow reviews.", Readonly: true, RequiredPermission: "agent.tool.call:my_pending_reviews"},
		{Value: "workspace_insights", Label: "Workspace Insights", Description: "Read workspace insight reports.", Readonly: true, RequiredPermission: "agent.tool.call:workspace_insights"},
	}
}

// ExportBundle 匯出 Agent 管理設定。
func (c AgentService) ExportBundle(ctx RequestContext) (domain.AgentBundle, error) {
	if _, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionRead, ""); err != nil {
		return domain.AgentBundle{}, err
	}
	agents, err := c.store.ListAgentDefinitions(goContext(ctx), ctx.TenantID)
	if err != nil {
		return domain.AgentBundle{}, err
	}
	models, err := c.store.ListAgentModels(goContext(ctx), ctx.TenantID)
	if err != nil {
		return domain.AgentBundle{}, err
	}
	return domain.AgentBundle{ExportedAt: c.Now(), Agents: agents, Models: models}, nil
}

// ImportBundle 匯入 Agent 管理設定。
func (c AgentService) ImportBundle(ctx RequestContext, bundle domain.AgentBundle) (domain.ImportAgentBundleResult, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionCreate, "")
	if err != nil {
		return domain.ImportAgentBundleResult{}, err
	}
	result := domain.ImportAgentBundleResult{}
	importedModelIDs, err := agentModelIDsFromBundle(bundle.Models)
	if err != nil {
		return result, err
	}
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
		agentSvc := tx.Agent()
		for _, model := range bundle.Models {
			model.ID = strings.TrimSpace(model.ID)
			model.TenantID = ctx.TenantID
			model.CreatedAt = nonZeroTime(model.CreatedAt, tx.Now())
			model.UpdatedAt = tx.Now()
			normalized, err := agentSvc.normalizeAgentModelForImport(ctx, model, importedModelIDs)
			if err != nil {
				return err
			}
			if err := agentSvc.store.UpsertAgentModel(goContext(ctx), normalized); err != nil {
				return err
			}
			result.Models++
		}
		for _, agent := range bundle.Agents {
			agent.ID = strings.TrimSpace(agent.ID)
			if agent.ID == "" {
				return BadRequest("agent id is required")
			}
			agent.TenantID = ctx.TenantID
			agent.CreatedAt = nonZeroTime(agent.CreatedAt, tx.Now())
			agent.UpdatedAt = tx.Now()
			if strings.TrimSpace(agent.CreatedByAccountID) == "" {
				agent.CreatedByAccountID = account.ID
			}
			agent.UpdatedByAccountID = account.ID
			normalized, err := agentSvc.normalizeAgentDefinitionForImport(ctx, agent, importedModelIDs)
			if err != nil {
				return err
			}
			if err := agentSvc.store.UpsertAgentDefinition(goContext(ctx), normalized); err != nil {
				return err
			}
			if err := agentSvc.ensureImportedAgentVersion(ctx, normalized, account.ID); err != nil {
				return err
			}
			result.Agents++
		}
		return agentSvc.recordAgentAdminAudit(ctx, account, "agent", "bundle", "bundle", "import", "bundle imported")
	}); err != nil {
		return result, err
	}
	return result, nil
}

func (c AgentService) currentAgentModel(ctx RequestContext, id string) (domain.AgentModel, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.AgentModel{}, BadRequest("id is required")
	}
	model, ok, err := c.store.GetAgentModel(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.AgentModel{}, err
	}
	if !ok {
		return domain.AgentModel{}, NotFound("agent model", id)
	}
	return model, nil
}

func (c AgentService) currentAgentDefinition(ctx RequestContext, id string) (domain.AgentDefinition, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.AgentDefinition{}, BadRequest("id is required")
	}
	agent, ok, err := c.store.GetAgentDefinition(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	if !ok {
		return domain.AgentDefinition{}, NotFound("agent definition", id)
	}
	return agent, nil
}

func (c AgentService) normalizeAgentModel(ctx RequestContext, model domain.AgentModel) (domain.AgentModel, error) {
	return c.normalizeAgentModelForImport(ctx, model, nil)
}

func (c AgentService) normalizeAgentModelForImport(ctx RequestContext, model domain.AgentModel, importedModelIDs map[string]struct{}) (domain.AgentModel, error) {
	model.Name = strings.TrimSpace(model.Name)
	if model.Name == "" {
		return domain.AgentModel{}, BadRequest("name is required")
	}
	model.Provider = strings.TrimSpace(model.Provider)
	if model.Provider == "" {
		model.Provider = "openai"
	}
	model.ModelName = strings.TrimSpace(model.ModelName)
	if model.ModelName == "" {
		return domain.AgentModel{}, BadRequest("model_name is required")
	}
	model.LiteLLMModel = strings.TrimSpace(model.LiteLLMModel)
	if model.LiteLLMModel == "" {
		model.LiteLLMModel = model.ModelName
	}
	if model.Status == "" {
		model.Status = domain.AgentModelStatusActive
	}
	if model.Status != domain.AgentModelStatusActive && model.Status != domain.AgentModelStatusDisabled {
		return domain.AgentModel{}, BadRequest("status must be active or disabled")
	}
	if model.TimeoutSeconds <= 0 {
		model.TimeoutSeconds = 60
	}
	if model.MonthlyQuota <= 0 {
		model.MonthlyQuota = 100000
	}
	if model.LastTestStatus == "" {
		model.LastTestStatus = "untested"
	}
	if model.FallbackModelID != "" {
		if err := c.requireAgentModelReference(ctx, model.FallbackModelID, importedModelIDs); err != nil {
			return domain.AgentModel{}, err
		}
	}
	return model, nil
}

func (c AgentService) normalizeAgentDefinition(ctx RequestContext, agent domain.AgentDefinition) (domain.AgentDefinition, error) {
	return c.normalizeAgentDefinitionForImport(ctx, agent, nil)
}

func (c AgentService) normalizeAgentDefinitionForImport(ctx RequestContext, agent domain.AgentDefinition, importedModelIDs map[string]struct{}) (domain.AgentDefinition, error) {
	agent.Name = strings.TrimSpace(agent.Name)
	if agent.Name == "" {
		return domain.AgentDefinition{}, BadRequest("name is required")
	}
	if err := c.requireAgentModelReference(ctx, agent.ModelID, importedModelIDs); err != nil {
		return domain.AgentDefinition{}, err
	}
	if agent.FallbackModelID != "" {
		if err := c.requireAgentModelReference(ctx, agent.FallbackModelID, importedModelIDs); err != nil {
			return domain.AgentDefinition{}, err
		}
	}
	if strings.TrimSpace(string(agent.Category)) == "" {
		agent.Category = domain.AgentCategoryWorkflow
	}
	switch agent.Category {
	case domain.AgentCategoryWorkflow, domain.AgentCategoryDoc, domain.AgentCategoryAnalytics, domain.AgentCategoryIT:
	default:
		return domain.AgentDefinition{}, BadRequest("category must be workflow, doc, analytics, or it")
	}
	if strings.TrimSpace(agent.Emoji) == "" {
		agent.Emoji = "AI"
	}
	if agent.Status == "" {
		agent.Status = domain.AgentDefinitionStatusDraft
	}
	switch agent.Status {
	case domain.AgentDefinitionStatusDraft, domain.AgentDefinitionStatusPublished, domain.AgentDefinitionStatusArchived:
	default:
		return domain.AgentDefinition{}, BadRequest("status must be draft, published, or archived")
	}
	if agent.Visibility == "" {
		agent.Visibility = domain.AgentVisibilityAll
	}
	switch agent.Visibility {
	case domain.AgentVisibilityAll, domain.AgentVisibilityDepartment, domain.AgentVisibilityRole:
	default:
		return domain.AgentDefinition{}, BadRequest("visibility must be all, department, or role")
	}
	if agent.TimeoutSeconds <= 0 {
		agent.TimeoutSeconds = 60
	}
	if agent.Version <= 0 {
		agent.Version = 1
	}
	agent.Tools = uniqueStrings(agent.Tools)
	if err := validateAgentTools(agent.Tools); err != nil {
		return domain.AgentDefinition{}, err
	}
	agent.VisibilityTargets = uniqueStrings(agent.VisibilityTargets)
	return agent, nil
}

func (c AgentService) requireAgentModelReference(ctx RequestContext, id string, importedModelIDs map[string]struct{}) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return BadRequest("model_id is required")
	}
	if _, ok := importedModelIDs[id]; ok {
		return nil
	}
	_, err := c.currentAgentModel(ctx, id)
	return err
}

func agentModelIDsFromBundle(models []domain.AgentModel) (map[string]struct{}, error) {
	ids := make(map[string]struct{}, len(models))
	for _, model := range models {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			return nil, BadRequest("model id is required")
		}
		ids[id] = struct{}{}
	}
	return ids, nil
}

func validateAgentTools(tools []string) error {
	if len(tools) == 0 {
		return nil
	}
	catalog := map[string]struct{}{}
	for _, tool := range agentToolCatalog() {
		catalog[tool.Value] = struct{}{}
	}
	for _, tool := range tools {
		if _, ok := catalog[tool]; !ok {
			return BadRequest("agent tool is invalid: " + tool)
		}
	}
	return nil
}

func (c AgentService) ensureImportedAgentVersion(ctx RequestContext, agent domain.AgentDefinition, actorID string) error {
	if _, ok, err := c.store.GetAgentDefinitionVersion(goContext(ctx), ctx.TenantID, agent.ID, agent.Version); err != nil {
		return err
	} else if ok {
		return nil
	}
	return c.snapshotAgentDefinition(ctx, agent, actorID, "imported bundle")
}

func (c AgentService) snapshotAgentDefinition(ctx RequestContext, agent domain.AgentDefinition, actorID, note string) error {
	return c.store.InsertAgentDefinitionVersion(goContext(ctx), domain.AgentDefinitionVersion{
		ID:                 utils.NewID("adefv"),
		TenantID:           ctx.TenantID,
		AgentID:            agent.ID,
		Version:            agent.Version,
		SystemPrompt:       agent.SystemPrompt,
		Tools:              agent.Tools,
		ModelID:            agent.ModelID,
		Note:               note,
		CreatedByAccountID: actorID,
		CreatedAt:          c.Now(),
	})
}

func (c AgentService) recordAgentAdminAudit(ctx RequestContext, account Account, entityType, entityID, entityName, action, detail string) error {
	if err := c.store.InsertAgentAudit(goContext(ctx), domain.AgentAudit{
		ID:               utils.NewID("aaud"),
		TenantID:         ctx.TenantID,
		EntityType:       entityType,
		EntityID:         entityID,
		EntityName:       entityName,
		Action:           action,
		ActorAccountID:   account.ID,
		ActorDisplayName: account.DisplayName,
		Detail:           detail,
		CreatedAt:        c.Now(),
	}); err != nil {
		return err
	}
	payload := map[string]any{"entity_type": entityType, "entity_id": entityID, "entity_name": entityName, "detail": detail}
	if raw, err := json.Marshal(payload); err == nil {
		payload["raw"] = string(raw)
	}
	return c.audit(ctx, "ai.agent."+entityType+"."+action, "agent_"+entityType, entityID, "high", payload)
}

func nonZeroTime(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value
}
