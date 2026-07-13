package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

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
		ID:             utils.NewID("amodel"),
		TenantID:       ctx.TenantID,
		Name:           input.Name,
		Provider:       input.Provider,
		ModelName:      input.ModelName,
		LiteLLMModel:   input.LiteLLMModel,
		APIBaseURL:     input.APIBaseURL,
		APIKey:         input.APIKey,
		RateLimitRPM:   input.RateLimitRPM,
		Status:         domain.AgentModelStatus(input.Status),
		TimeoutSeconds: input.TimeoutSeconds,
		MonthlyQuota:   input.MonthlyQuota,
		LastTestStatus: "untested",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		return domain.AgentModel{}, err
	}
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		if err := tx.store.UpsertAgentModel(goContext(ctx), model); err != nil {
			return err
		}
		return tx.recordAgentAdminAudit(ctx, account, "model", model.ID, model.Name, "create", "model created")
	}); err != nil {
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
	if input.APIBaseURL != nil {
		model.APIBaseURL = *input.APIBaseURL
	}
	if input.APIKey != nil {
		model.APIKey = *input.APIKey
	}
	if input.RateLimitRPM != nil {
		model.RateLimitRPM = *input.RateLimitRPM
	}
	if input.Status != nil {
		model.Status = domain.AgentModelStatus(*input.Status)
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
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		if err := tx.store.UpsertAgentModel(goContext(ctx), model); err != nil {
			return err
		}
		return tx.recordAgentAdminAudit(ctx, account, "model", model.ID, model.Name, "update", "model updated")
	}); err != nil {
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
	var deleted domain.AgentModel
	err = c.withTransaction(ctx, func(tx AgentService) error {
		count, err := tx.store.CountAgentDefinitionsByModel(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if count > 0 {
			return Conflict("agent model is used by agent definitions")
		}
		model, ok, err := tx.store.DeleteAgentModel(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("agent model", id)
		}
		if err := tx.recordAgentAdminAudit(ctx, account, "model", model.ID, model.Name, "delete", "model deleted"); err != nil {
			return err
		}
		deleted = model
		return nil
	})
	return deleted, err
}

// SyncModel 將本地模型別名同步到 LiteLLM，並寫回最近一次同步狀態。
func (c AgentService) SyncModel(ctx RequestContext, id string) (domain.AgentModel, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceModel, ActionUpdate, id)
	if err != nil {
		return domain.AgentModel{}, err
	}
	model, err := c.currentAgentModel(ctx, id)
	if err != nil {
		return domain.AgentModel{}, err
	}
	if model.Status == domain.AgentModelStatusDisabled {
		return c.updateAgentModelTestResult(ctx, account, model, "failed", "model is disabled; sync skipped", "sync")
	}
	if c.liteLLMAdmin == nil {
		message := "LiteLLM admin client is not configured"
		updated, updateErr := c.updateAgentModelTestResult(ctx, account, model, "failed", message, "sync")
		if updateErr != nil {
			return domain.AgentModel{}, updateErr
		}
		return updated, domain.E(503, "service_unavailable", message)
	}
	message, err := c.liteLLMAdmin.SyncModel(goContext(ctx), model)
	if err != nil {
		updated, updateErr := c.updateAgentModelTestResult(ctx, account, model, "failed", err.Error(), "sync")
		if updateErr != nil {
			return domain.AgentModel{}, updateErr
		}
		return updated, domain.E(502, "bad_gateway", err.Error())
	}
	return c.updateAgentModelTestResult(ctx, account, model, "ok", message, "sync")
}

// TestModel 執行 LiteLLM 路由探測並寫回 last_test_*。
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
	message := "local configuration check ok; LiteLLM client is not configured"
	if model.Status == domain.AgentModelStatusDisabled {
		status = "failed"
		message = "model is disabled"
	} else if c.liteLLMAdmin != nil {
		if result, testErr := c.liteLLMAdmin.TestModel(goContext(ctx), model); testErr != nil {
			status = "failed"
			message = testErr.Error()
		} else {
			message = result
		}
	}
	return c.updateAgentModelTestResult(ctx, account, model, status, message, "test")
}

func (c AgentService) updateAgentModelTestResult(ctx RequestContext, account Account, model domain.AgentModel, status, message, action string) (domain.AgentModel, error) {
	var updated domain.AgentModel
	err := c.withTransaction(ctx, func(tx AgentService) error {
		next, ok, err := tx.store.UpdateAgentModelTestResult(goContext(ctx), ctx.TenantID, model.ID, status, message, c.Now())
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("agent model", model.ID)
		}
		if err := tx.recordAgentAdminAudit(ctx, account, "model", next.ID, next.Name, action, message); err != nil {
			return err
		}
		updated = next
		return nil
	})
	return updated, err
}

// ListDefinitions 列出工作區 Agent。
func (c AgentService) ListDefinitions(ctx RequestContext) ([]domain.AgentDefinition, error) {
	if _, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionRead, ""); err != nil {
		return nil, err
	}
	items, err := c.store.ListAgentDefinitions(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	for index := range items {
		items[index], err = c.definitionWithVersions(ctx, items[index])
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

// GetDefinition 取得工作區 Agent。
func (c AgentService) GetDefinition(ctx RequestContext, id string) (domain.AgentDefinition, error) {
	if _, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionRead, id); err != nil {
		return domain.AgentDefinition{}, err
	}
	agent, err := c.currentAgentDefinition(ctx, id)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	return c.definitionWithVersions(ctx, agent)
}

// definitionWithVersions 補齊獨立儲存的版本快照，讓真實 Postgres 回應與管理 UI 契約一致。
func (c AgentService) definitionWithVersions(ctx RequestContext, agent domain.AgentDefinition) (domain.AgentDefinition, error) {
	versions, err := c.store.ListAgentDefinitionVersions(goContext(ctx), ctx.TenantID, agent.ID)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	agent.Versions = versions
	return agent, nil
}

// CreateDefinition 建立工作區 Agent。
func (c AgentService) CreateDefinition(ctx RequestContext, input domain.CreateAgentDefinitionInput) (domain.AgentDefinition, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionCreate, "")
	if err != nil {
		return domain.AgentDefinition{}, err
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
		SystemPrompt:       input.SystemPrompt,
		Tools:              input.Tools,
		Status:             domain.AgentDefinitionStatusDraft,
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
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		if err := tx.store.UpsertAgentDefinition(goContext(ctx), agent); err != nil {
			return err
		}
		if err := tx.snapshotAgentDefinition(ctx, agent, account.ID, "initial version"); err != nil {
			return err
		}
		return tx.recordAgentAdminAudit(ctx, account, "agent", agent.ID, agent.Name, "create", "agent created")
	}); err != nil {
		return domain.AgentDefinition{}, err
	}
	return agent, nil
}

// UpdateDefinition 更新工作區 Agent；prompt/tools/model 變動會建立新版本，發布狀態僅能透過專用接口流轉。
func (c AgentService) UpdateDefinition(ctx RequestContext, id string, input domain.UpdateAgentDefinitionInput) (domain.AgentDefinition, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionUpdate, id)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	agent, err := c.currentAgentDefinition(ctx, id)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	beforePrompt, beforeModel := agent.SystemPrompt, agent.ModelID
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
	if input.SystemPrompt != nil {
		agent.SystemPrompt = *input.SystemPrompt
	}
	if input.Tools != nil {
		agent.Tools = input.Tools
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
	versionChanged := beforePrompt != agent.SystemPrompt || beforeModel != agent.ModelID || beforeTools != strings.Join(agent.Tools, "\x00")
	if versionChanged {
		agent.Version++
	}
	agent, err = c.normalizeAgentDefinition(ctx, agent)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	note := strings.TrimSpace(input.VersionNote)
	if note == "" {
		note = "updated"
	}
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		if err := tx.store.UpsertAgentDefinition(goContext(ctx), agent); err != nil {
			return err
		}
		if versionChanged {
			if err := tx.snapshotAgentDefinition(ctx, agent, account.ID, note); err != nil {
				return err
			}
		}
		return tx.recordAgentAdminAudit(ctx, account, "agent", agent.ID, agent.Name, "update", "agent updated")
	}); err != nil {
		return domain.AgentDefinition{}, err
	}
	return agent, nil
}

// PublishDefinition 將工作區 Agent 發布到可試用與助理列表。
func (c AgentService) PublishDefinition(ctx RequestContext, id string) (domain.AgentDefinition, error) {
	return c.transitionDefinitionPublishStatus(ctx, id, domain.AgentDefinitionStatusPublished, "publish", "agent published")
}

// UnpublishDefinition 停止發布工作區 Agent，並保留 draft 狀態供後續編輯。
func (c AgentService) UnpublishDefinition(ctx RequestContext, id string) (domain.AgentDefinition, error) {
	return c.transitionDefinitionPublishStatus(ctx, id, domain.AgentDefinitionStatusDraft, "unpublish", "agent unpublished")
}

func (c AgentService) transitionDefinitionPublishStatus(ctx RequestContext, id string, status domain.AgentDefinitionStatus, action, detail string) (domain.AgentDefinition, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionUpdate, id)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	agent, err := c.currentAgentDefinition(ctx, id)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	agent.Status = status
	agent.UpdatedByAccountID = account.ID
	agent.UpdatedAt = c.Now()
	agent, err = c.normalizeAgentDefinition(ctx, agent)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		if err := tx.store.UpsertAgentDefinition(goContext(ctx), agent); err != nil {
			return err
		}
		return tx.recordAgentAdminAudit(ctx, account, "agent", agent.ID, agent.Name, action, detail)
	}); err != nil {
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
	id = strings.TrimSpace(id)
	var deleted domain.AgentDefinition
	err = c.withTransaction(ctx, func(tx AgentService) error {
		agent, err := tx.currentAgentDefinition(ctx, id)
		if err != nil {
			return err
		}
		if agent.Status == domain.AgentDefinitionStatusPublished {
			return Conflict("published agent definition must be unpublished before deletion")
		}
		agent, ok, err := tx.store.DeleteAgentDefinition(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("agent definition", id)
		}
		if err := tx.recordAgentAdminAudit(ctx, account, "agent", agent.ID, agent.Name, "delete", "agent deleted"); err != nil {
			return err
		}
		deleted = agent
		return nil
	})
	return deleted, err
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
	reply, toolsUsed, trialErr := c.trialReply(ctx, agent, model, message)
	latencyMs := int(c.Now().Sub(start).Milliseconds())
	if latencyMs <= 0 {
		latencyMs = 1
	}
	auditDetail := "agent trial executed"
	if trialErr != nil {
		auditDetail = "agent trial failed"
	}
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		if _, ok, err := tx.store.UpdateAgentDefinitionUsage(goContext(ctx), ctx.TenantID, agent.ID, trialErr == nil, latencyMs, message, c.Now()); err != nil {
			return err
		} else if !ok {
			return NotFound("agent definition", id)
		}
		return tx.recordAgentAdminAudit(ctx, account, "agent", agent.ID, agent.Name, "trial", auditDetail)
	}); err != nil {
		return domain.AgentTrialResult{}, err
	}
	if trialErr != nil {
		return domain.AgentTrialResult{}, trialErr
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
	baseCtx, cancel := context.WithTimeout(goContext(ctx), effectiveAgentRuntimeTimeout(agent.TimeoutSeconds, model.TimeoutSeconds))
	defer cancel()
	baseCtx = WithAgentRequestContext(baseCtx, ctx)
	modelName := strings.TrimSpace(model.LiteLLMModel)
	if modelName == "" {
		modelName = strings.TrimSpace(model.ModelName)
	}
	req := AgentChatRuntimeRequest{
		RequestContext: ctx,
		RunID:          "trial-" + agent.ID,
		SessionID:      "trial-" + agent.ID,
		ModelName:      modelName,
		Message:        buildAgentRuntimeMessage(agent.SystemPrompt, nil, nil, message),
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
	detail := fmt.Sprintf("rollback to v%d", input.Version)
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		if err := tx.store.UpsertAgentDefinition(goContext(ctx), agent); err != nil {
			return err
		}
		if err := tx.snapshotAgentDefinition(ctx, agent, account.ID, detail); err != nil {
			return err
		}
		return tx.recordAgentAdminAudit(ctx, account, "agent", agent.ID, agent.Name, "rollback", detail)
	}); err != nil {
		return domain.AgentDefinition{}, err
	}
	return agent, nil
}

// Tools 回傳靜態可用工具目錄。
func (c AgentService) Tools(ctx RequestContext) ([]domain.AgentToolMeta, error) {
	if _, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionRead, ""); err != nil {
		return nil, err
	}
	return agentToolCatalog(), nil
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
	model.APIBaseURL = strings.TrimRight(strings.TrimSpace(model.APIBaseURL), "/")
	model.APIKey = strings.TrimSpace(model.APIKey)
	if model.APIKey == "" {
		return domain.AgentModel{}, BadRequest("api_key is required")
	}
	if agentModelProviderRequiresBaseURL(model.Provider) {
		if model.APIBaseURL == "" {
			return domain.AgentModel{}, BadRequest("api_base_url is required for custom provider")
		}
		if parsed, err := url.ParseRequestURI(model.APIBaseURL); err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return domain.AgentModel{}, BadRequest("api_base_url must be a valid absolute URL")
		}
	}
	if model.RateLimitRPM < 0 {
		return domain.AgentModel{}, BadRequest("rate_limit_rpm must be greater than or equal to 0")
	}
	model.APIKeySet = model.APIKey != ""
	model.APIKeyPreview = maskAgentModelAPIKey(model.APIKey)
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
	return model, nil
}

func agentModelProviderRequiresBaseURL(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "custom", "openai-compatible", "openai_compatible", "compatible":
		return true
	default:
		return false
	}
}

func maskAgentModelAPIKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	return "****" + value[len(value)-4:]
}

func (c AgentService) normalizeAgentDefinition(ctx RequestContext, agent domain.AgentDefinition) (domain.AgentDefinition, error) {
	agent.Name = strings.TrimSpace(agent.Name)
	if agent.Name == "" {
		return domain.AgentDefinition{}, BadRequest("name is required")
	}
	if err := c.requireAgentModelReference(ctx, agent.ModelID); err != nil {
		return domain.AgentDefinition{}, err
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
	case domain.AgentDefinitionStatusDraft, domain.AgentDefinitionStatusPublished:
	default:
		return domain.AgentDefinition{}, BadRequest("status must be draft or published")
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
	if err := c.validateAgentVisibilityTargets(ctx, &agent); err != nil {
		return domain.AgentDefinition{}, err
	}
	return agent, nil
}

// validateAgentVisibilityTargets 驗證可見範圍目標存在於目前租戶，並封閉空白 scoped 設定。
func (c AgentService) validateAgentVisibilityTargets(ctx RequestContext, agent *domain.AgentDefinition) error {
	if agent.Visibility == domain.AgentVisibilityAll {
		agent.VisibilityTargets = []string{}
		return nil
	}
	if len(agent.VisibilityTargets) == 0 {
		return BadRequest("visibility_targets is required for scoped visibility")
	}
	for _, targetID := range agent.VisibilityTargets {
		switch agent.Visibility {
		case domain.AgentVisibilityDepartment:
			_, ok, err := c.Service.store.GetOrgUnit(goContext(ctx), ctx.TenantID, targetID)
			if err != nil {
				return err
			}
			if !ok {
				return BadRequest("visibility target org unit does not exist: " + targetID)
			}
		case domain.AgentVisibilityRole:
			_, ok, err := c.Service.store.GetAssumableRole(goContext(ctx), ctx.TenantID, targetID)
			if err != nil {
				return err
			}
			if !ok {
				return BadRequest("visibility target role does not exist: " + targetID)
			}
		}
	}
	return nil
}

func (c AgentService) requireAgentModelReference(ctx RequestContext, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return BadRequest("model_id is required")
	}
	_, err := c.currentAgentModel(ctx, id)
	return err
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
