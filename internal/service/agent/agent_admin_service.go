package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
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
		ID:       utils.NewID("amodel"),
		TenantID: ctx.TenantID,
		Name:     input.Name,
		Provider:           input.Provider,
		ModelName:          input.ModelName,
		APIBaseURL:         input.APIBaseURL,
		APIKey:             input.APIKey,
		RateLimitRPM:       input.RateLimitRPM,
		Status:             domain.AgentModelStatus(input.Status),
		TimeoutSeconds:     input.TimeoutSeconds,
		MonthlyQuota:       input.MonthlyQuota,
		LastTestStatus:     "untested",
		SyncStatus:         domain.AgentModelSyncStatusPending,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		return domain.AgentModel{}, err
	}
	storedModel, err := c.protectAgentModelCredential(model)
	if err != nil {
		return domain.AgentModel{}, err
	}
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		if err := tx.store.UpsertAgentModel(goContext(ctx), storedModel); err != nil {
			return err
		}
		if err := tx.appendAgentModelSyncEvent(ctx, model.ID, domain.EventAgentModelUpsert); err != nil {
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
	model.SyncStatus = domain.AgentModelSyncStatusPending
	model.LastSyncError = ""
	model, err = c.normalizeAgentModel(ctx, model)
	if err != nil {
		return domain.AgentModel{}, err
	}
	storedModel, err := c.protectAgentModelCredential(model)
	if err != nil {
		return domain.AgentModel{}, err
	}
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		if err := tx.store.UpsertAgentModel(goContext(ctx), storedModel); err != nil {
			return err
		}
		if err := tx.appendAgentModelSyncEvent(ctx, model.ID, domain.EventAgentModelUpsert); err != nil {
			return err
		}
		return tx.recordAgentAdminAudit(ctx, account, "model", model.ID, model.Name, "update", "model updated")
	}); err != nil {
		return domain.AgentModel{}, err
	}
	return model, nil
}

// agentDefinitionRefSummary 產生阻擋刪除的引用方摘要（最多 5 個名稱）。
func agentDefinitionRefSummary(refs []domain.AgentDefinitionRef) string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, ref.Name)
	}
	sort.Strings(names)
	if len(names) > 5 {
		return strings.Join(names[:5], ", ") + fmt.Sprintf(" (+%d more)", len(names)-5)
	}
	return strings.Join(names, ", ")
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
		// 只阻擋「目前」定義的引用；歷史版本是不可變審計快照，不應鎖死模型生命週期。
		refs, err := tx.store.ListAgentDefinitionRefsByModel(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if len(refs) > 0 {
			return Conflict("agent model is used by agent definitions: " + agentDefinitionRefSummary(refs)).WithReasonCode("agent_model_in_use")
		}
		model, ok, err := tx.store.DeleteAgentModel(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("agent model", id)
		}
		if err := tx.appendAgentModelSyncEvent(ctx, model.ID, domain.EventAgentModelDelete); err != nil {
			return err
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
	if c.LiteLLMAdmin() == nil {
		message := "LiteLLM admin client is not configured"
		updated, updateErr := c.updateAgentModelSyncResult(ctx, account, model, domain.AgentModelSyncStatusFailed, message, false)
		if updateErr != nil {
			return domain.AgentModel{}, updateErr
		}
		return updated, domain.E(503, "service_unavailable", "LiteLLM synchronization is temporarily unavailable").WithReasonCode("agent_runtime_unavailable")
	}
	var message string
	if model.Status == domain.AgentModelStatusDisabled {
		message, err = c.LiteLLMAdmin().DeleteModel(goContext(ctx), model.ID)
	} else {
		message, err = c.LiteLLMAdmin().SyncModel(goContext(ctx), model)
	}
	if err != nil {
		updated, updateErr := c.updateAgentModelSyncResult(ctx, account, model, domain.AgentModelSyncStatusFailed, err.Error(), false)
		if updateErr != nil {
			return domain.AgentModel{}, updateErr
		}
		return updated, domain.E(502, "bad_gateway", "LiteLLM synchronization failed").WithReasonCode("agent_runtime_unavailable")
	}
	return c.updateAgentModelSyncResult(ctx, account, model, domain.AgentModelSyncStatusSynced, message, true)
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
	} else if c.LiteLLMAdmin() != nil {
		if result, testErr := c.LiteLLMAdmin().TestModel(goContext(ctx), model); testErr != nil {
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

// updateAgentModelSyncResult 獨立寫回 LiteLLM 同步狀態，不覆蓋連通性測試結果。
func (c AgentService) updateAgentModelSyncResult(ctx RequestContext, account Account, model domain.AgentModel, status domain.AgentModelSyncStatus, message string, succeeded bool) (domain.AgentModel, error) {
	var updated domain.AgentModel
	err := c.withTransaction(ctx, func(tx AgentService) error {
		now := c.Now()
		lastSyncedAt := model.LastSyncedAt
		configHash := model.SyncedConfigHash
		lastError := message
		if succeeded {
			lastSyncedAt = &now
			configHash = domain.AgentModelSyncConfigHash(model)
			lastError = ""
		}
		next, ok, err := tx.store.UpdateAgentModelSyncResult(goContext(ctx), ctx.TenantID, model.ID, status, lastError, configHash, lastSyncedAt, now)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("agent model", model.ID)
		}
		if err := tx.recordAgentAdminAudit(ctx, account, "model", next.ID, next.Name, "sync", message); err != nil {
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

// definitionWithVersions attaches stored versions and normalizes every response collection.
func (c AgentService) definitionWithVersions(ctx RequestContext, agent domain.AgentDefinition) (domain.AgentDefinition, error) {
	versions, err := c.store.ListAgentDefinitionVersions(goContext(ctx), ctx.TenantID, agent.ID)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	agent.Versions = versions
	return normalizeAgentDefinitionResponse(agent), nil
}

// CreateDefinition 建立工作區 Agent。
func (c AgentService) CreateDefinition(ctx RequestContext, input domain.CreateAgentDefinitionInput) (domain.AgentDefinition, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionCreate, "")
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	now := c.Now()
	agent, err := c.normalizeAgentDefinition(ctx, domain.AgentDefinition{
		ID:                            utils.NewID("adef"),
		TenantID:                      ctx.TenantID,
		Name:                          input.Name,
		Description:                   input.Description,
		Emoji:                         input.Emoji,
		Category:                      domain.AgentCategory(input.Category),
		ModelID:                       input.ModelID,
		MainAgentRole:                 input.MainAgentRole,
		SubAgents:                     input.SubAgents,
		SystemPrompt:                  input.SystemPrompt,
		WelcomeMessage:                input.WelcomeMessage,
		SuggestedQuestions:            input.SuggestedQuestions,
		SuggestedQuestionTranslations: input.SuggestedQuestionTranslations,
		Tools:                         input.Tools,
		ExternalToolIDs:               input.ExternalToolIDs,
		KnowledgeBaseIDs:              input.KnowledgeBaseIDs,
		Status:                        domain.AgentDefinitionStatusDraft,
		Visibility:                    domain.AgentVisibility(input.Visibility),
		VisibilityTargets:             input.VisibilityTargets,
		TimeoutSeconds:                input.TimeoutSeconds,
		Version:                       1,
		CreatedByAccountID:            account.ID,
		UpdatedByAccountID:            account.ID,
		CreatedAt:                     now,
		UpdatedAt:                     now,
	})
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	agent.DraftRevisionID = agentDefinitionRevisionID(agent.ID, agent.Version)
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
	return normalizeAgentDefinitionResponse(agent), nil
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
	beforeRuntimeConfig := agentDefinitionRuntimeSignature(agent)
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
	if input.MainAgentRole != nil {
		agent.MainAgentRole = *input.MainAgentRole
	}
	if input.SubAgents != nil {
		agent.SubAgents = input.SubAgents
	}
	if input.SystemPrompt != nil {
		agent.SystemPrompt = *input.SystemPrompt
	}
	if input.WelcomeMessage != nil {
		agent.WelcomeMessage = *input.WelcomeMessage
	}
	if input.SuggestedQuestions != nil {
		agent.SuggestedQuestions = input.SuggestedQuestions
		if input.SuggestedQuestionTranslations == nil {
			agent.SuggestedQuestionTranslations = nil
		}
	}
	if input.SuggestedQuestionTranslations != nil {
		agent.SuggestedQuestionTranslations = input.SuggestedQuestionTranslations
		agent.SuggestedQuestions = nil
	}
	if input.Tools != nil {
		agent.Tools = input.Tools
	}
	if input.ExternalToolIDs != nil {
		agent.ExternalToolIDs = input.ExternalToolIDs
	}
	if input.KnowledgeBaseIDs != nil {
		agent.KnowledgeBaseIDs = input.KnowledgeBaseIDs
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
	agent, err = c.normalizeAgentDefinition(ctx, agent)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	versionChanged := beforeRuntimeConfig != agentDefinitionRuntimeSignature(agent)
	if versionChanged {
		agent.Version++
		agent.DraftRevisionID = agentDefinitionRevisionID(agent.ID, agent.Version)
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
	return normalizeAgentDefinitionResponse(agent), nil
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
	if status == domain.AgentDefinitionStatusPublished {
		agent.PublishedVersion = agent.Version
		agent.PublishedRevisionID = agent.DraftRevisionID
	} else {
		agent.PublishedVersion = 0
		agent.PublishedRevisionID = ""
	}
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
	return normalizeAgentDefinitionResponse(agent), nil
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
		if agent.PublishedRevisionID != "" || agent.Status == domain.AgentDefinitionStatusPublished {
			return Conflict("published agent definition must be unpublished before deletion").WithReasonCode("agent_definition_published")
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
	agent, err := c.publishedAgentDefinition(ctx, id)
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
	reply, toolsUsed, agentsUsed, trialErr := c.trialReply(ctx, agent, model, message)
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
		return domain.AgentTrialResult{}, domain.E(503, "service_unavailable", "agent runtime is temporarily unavailable").WithReasonCode("agent_runtime_unavailable")
	}
	return domain.AgentTrialResult{Reply: reply, LatencyMs: latencyMs, ToolsUsed: toolsUsed, AgentsUsed: agentsUsed, ModelName: model.ModelName}, nil
}

// trialReply 優先使用已注入的 AgentChatRuntime，並只回報 runtime 實際成功呼叫的工具。
func (c AgentService) trialReply(ctx RequestContext, agent domain.AgentDefinition, model domain.AgentModel, message string) (string, []string, []string, error) {
	if c.AgentChatRuntime() == nil {
		return fmt.Sprintf("[%s] mock reply using %s: %s", agent.Name, model.ModelName, message), []string{}, []string{agent.Name}, nil
	}
	var answer strings.Builder
	toolsUsed := make([]string, 0)
	agentsUsed := make([]string, 0)
	seenTools := map[string]struct{}{}
	seenAgents := map[string]struct{}{}
	emit := func(_ context.Context, event domain.AgentChatEvent) error {
		if agentName := strings.TrimSpace(event.AgentName); agentName != "" {
			if _, exists := seenAgents[agentName]; !exists {
				seenAgents[agentName] = struct{}{}
				agentsUsed = append(agentsUsed, agentName)
			}
		}
		if event.Event == domain.AgentChatEventMessageDelta && (len(agent.SubAgents) == 0 || event.AgentName == "" || event.AgentName == agent.Name) {
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
	runtimeTimeout := effectiveAgentRuntimeTimeout(agent.TimeoutSeconds, model.TimeoutSeconds)
	resolvedSubAgents, runtimeTimeout, err := c.resolveAgentTeamMembers(ctx, agent.SubAgents, runtimeTimeout)
	if err != nil {
		return "", []string{}, []string{}, err
	}
	baseCtx, cancel := context.WithTimeout(goContext(ctx), runtimeTimeout)
	defer cancel()
	baseCtx = WithAgentRequestContext(baseCtx, ctx)
	modelName := strings.TrimSpace(model.LiteLLMModel)
	if modelName == "" {
		modelName = strings.TrimSpace(model.ModelName)
	}
	runtimeSubAgents := make([]AgentChatSubAgentRuntimeRequest, 0, len(resolvedSubAgents))
	for _, member := range resolvedSubAgents {
		runtimeSubAgents = append(runtimeSubAgents, AgentChatSubAgentRuntimeRequest{
			ID:        member.ID,
			Name:      member.Name,
			Role:      member.Role,
			ModelName: member.ModelName,
			Tools:     c.filteredReadonlyAgentTools(ctx, member.ToolNames, true, emit, member.KnowledgeBaseIDs),
		})
	}
	req := AgentChatRuntimeRequest{
		RequestContext: ctx,
		RunID:          "trial-" + agent.ID,
		SessionID:      "trial-" + agent.ID,
		AgentName:      agent.Name,
		AgentRole:      agent.MainAgentRole,
		ModelName:      modelName,
		Message:        buildAgentRuntimeMessage(agent.SystemPrompt, nil, nil, message),
		Mode:           "trial",
		Tools:          c.filteredReadonlyAgentTools(ctx, agent.Tools, true, emit, agent.KnowledgeBaseIDs),
		SubAgents:      runtimeSubAgents,
	}
	if err := c.AgentChatRuntime().RunAgentChat(baseCtx, req, emit); err != nil {
		return "", []string{}, agentsUsed, err
	}
	reply := strings.TrimSpace(answer.String())
	if reply == "" {
		return fmt.Sprintf("[%s] mock reply using %s: %s", agent.Name, model.ModelName, message), toolsUsed, agentsUsed, nil
	}
	return reply, toolsUsed, agentsUsed, nil
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
	agent.Name = version.Name
	agent.Description = version.Description
	agent.Emoji = version.Emoji
	agent.Category = version.Category
	agent.Visibility = version.Visibility
	agent.VisibilityTargets = version.VisibilityTargets
	agent.SystemPrompt = version.SystemPrompt
	agent.WelcomeMessage = version.WelcomeMessage
	agent.SuggestedQuestions = version.SuggestedQuestions
	agent.SuggestedQuestionTranslations = version.SuggestedQuestionTranslations
	agent.Tools = version.Tools
	agent.ExternalToolIDs = version.ExternalToolIDs
	agent.KnowledgeBaseIDs = version.KnowledgeBaseIDs
	agent.ModelID = version.ModelID
	agent.TimeoutSeconds = version.TimeoutSeconds
	agent.MainAgentRole = version.MainAgentRole
	agent.SubAgents = version.SubAgents
	agent.Version++
	agent.DraftRevisionID = agentDefinitionRevisionID(agent.ID, agent.Version)
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
	return normalizeAgentDefinitionResponse(agent), nil
}

// Tools 回傳靜態可用工具目錄。
func (c AgentService) Tools(ctx RequestContext) ([]domain.AgentToolMeta, error) {
	if _, _, err := c.requireAgentAuthz(ctx, ResourceTool, ActionRead, ""); err != nil {
		return nil, err
	}
	return domain.AgentToolCatalog(), nil
}

// ListExternalTools returns external tool registrations owned by the current tenant.
func (c AgentService) ListExternalTools(ctx RequestContext) ([]domain.AgentExternalTool, error) {
	if _, _, err := c.requireAgentAuthz(ctx, ResourceTool, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListAgentExternalTools(goContext(ctx), ctx.TenantID)
}

// CreateExternalTool registers an MCP connection or one manually described HTTP capability.
func (c AgentService) CreateExternalTool(ctx RequestContext, input domain.CreateAgentExternalToolInput) (domain.AgentExternalTool, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceTool, ActionCreate, "")
	if err != nil {
		return domain.AgentExternalTool{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return domain.AgentExternalTool{}, BadRequest("name is required")
	}
	if len([]rune(name)) > 120 {
		return domain.AgentExternalTool{}, BadRequest("name must not exceed 120 characters")
	}
	description := strings.TrimSpace(input.Description)
	if len([]rune(description)) > 500 {
		return domain.AgentExternalTool{}, BadRequest("description must not exceed 500 characters")
	}
	kind := strings.ToLower(strings.TrimSpace(input.Kind))
	if kind == "" {
		kind = "mcp"
	}
	if kind != "mcp" && kind != "http" {
		return domain.AgentExternalTool{}, BadRequest("kind must be mcp or http")
	}
	transport, err := normalizeExternalToolTransport(kind, input.Transport)
	if err != nil {
		return domain.AgentExternalTool{}, err
	}
	endpointURL, err := normalizeExternalToolEndpoint(input.EndpointURL)
	if err != nil {
		return domain.AgentExternalTool{}, err
	}
	auth, err := normalizeExternalToolAuth(input)
	if err != nil {
		return domain.AgentExternalTool{}, err
	}
	timeoutSeconds := input.TimeoutSeconds
	if timeoutSeconds == 0 {
		timeoutSeconds = int(externalToolDefaultTimeout / time.Second)
	}
	if timeoutSeconds < 1 || timeoutSeconds > 120 {
		return domain.AgentExternalTool{}, BadRequest("timeout_seconds must be between 1 and 120")
	}
	now := c.Now()
	id := utils.NewID("atool")
	credentialCiphertext := ""
	if auth.secret != "" {
		if c.CredentialCipher() == nil {
			return domain.AgentExternalTool{}, domain.E(503, "service_unavailable", "external tool credential storage is not configured")
		}
		credentialCiphertext, err = c.CredentialCipher().Encrypt([]byte(auth.secret), domain.ExternalToolCredentialAAD(ctx.TenantID, id))
		if err != nil {
			return domain.AgentExternalTool{}, domain.E(500, "internal_error", "failed to protect external tool credential")
		}
	}
	item := domain.AgentExternalTool{
		ID:                   id,
		TenantID:             ctx.TenantID,
		Name:                 name,
		Description:          description,
		Kind:                 kind,
		Transport:            transport,
		EndpointURL:          endpointURL,
		AuthType:             auth.authType,
		AuthHeaderName:       auth.headerName,
		AuthUsername:         auth.username,
		TimeoutSeconds:       timeoutSeconds,
		AuthSecretCiphertext: credentialCiphertext,
		CredentialSet:        credentialCiphertext != "",
		Status:               string(domain.ExternalToolConnectionStatusActive),
		LastTestStatus:       string(domain.ConnectionTestStatusUntested),
		CreatedByAccountID:   account.ID,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if kind == "http" {
		capability, capabilityErr := manualHTTPExternalToolCapability(item, input, now)
		if capabilityErr != nil {
			return domain.AgentExternalTool{}, capabilityErr
		}
		item.Capabilities = []domain.ExternalToolCapability{capability}
	}
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		if err := tx.store.InsertAgentExternalTool(goContext(ctx), item); err != nil {
			return err
		}
		if len(item.Capabilities) > 0 {
			store, err := tx.externalToolLifecycleStore()
			if err != nil {
				return err
			}
			if err := store.ReplaceAgentExternalToolCapabilities(goContext(ctx), ctx.TenantID, item.ID, item.Capabilities); err != nil {
				return err
			}
		}
		return tx.recordAgentAdminAudit(ctx, account, "external_tool", item.ID, item.Name, "create", "external tool registered")
	}); err != nil {
		return domain.AgentExternalTool{}, err
	}
	return item, nil
}

// DeleteExternalTool archives external tool metadata while preserving revision and execution audit references.
func (c AgentService) DeleteExternalTool(ctx RequestContext, id string) (domain.AgentExternalTool, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceTool, ActionDelete, id)
	if err != nil {
		return domain.AgentExternalTool{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.AgentExternalTool{}, BadRequest("id is required")
	}
	var deleted domain.AgentExternalTool
	err = c.withTransaction(ctx, func(tx AgentService) error {
		item, ok, err := tx.store.DeleteAgentExternalTool(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("agent external tool", id)
		}
		if err := tx.recordAgentAdminAudit(ctx, account, "external_tool", item.ID, item.Name, "archive", "external tool archived"); err != nil {
			return err
		}
		deleted = item
		return nil
	})
	return deleted, err
}

type normalizedExternalToolAuth struct {
	authType   string
	headerName string
	username   string
	secret     string
}

// normalizeExternalToolTransport keeps MCP transport explicit while HTTP APIs use their native transport.
func normalizeExternalToolTransport(kind, value string) (string, error) {
	transport := strings.ToLower(strings.TrimSpace(value))
	if kind == "http" {
		if transport == "" || transport == "http" {
			return "http", nil
		}
		return "", BadRequest("transport must be http when kind is http")
	}
	if transport == "" || transport == "http" {
		transport = "streamable_http"
	}
	if transport != "sse" && transport != "streamable_http" {
		return "", BadRequest("transport must be sse or streamable_http when kind is mcp")
	}
	return transport, nil
}

// normalizeExternalToolAuth validates supported credential shapes without logging or returning the secret.
func normalizeExternalToolAuth(input domain.CreateAgentExternalToolInput) (normalizedExternalToolAuth, error) {
	authType := strings.ToLower(strings.TrimSpace(input.AuthType))
	if authType == "" {
		authType = "none"
	}
	headerName := strings.TrimSpace(input.AuthHeaderName)
	username := strings.TrimSpace(input.AuthUsername)
	secret := input.AuthSecret
	if len(secret) > 8192 {
		return normalizedExternalToolAuth{}, BadRequest("auth_secret must not exceed 8192 bytes")
	}
	switch authType {
	case "none":
		if headerName != "" || username != "" || secret != "" {
			return normalizedExternalToolAuth{}, BadRequest("authentication fields require an auth_type")
		}
	case "bearer":
		if secret == "" {
			return normalizedExternalToolAuth{}, BadRequest("auth_secret is required for bearer authentication")
		}
		if headerName != "" || username != "" {
			return normalizedExternalToolAuth{}, BadRequest("bearer authentication does not accept auth_header_name or auth_username")
		}
	case "api_key":
		if headerName == "" {
			headerName = "X-API-Key"
		}
		if len(headerName) > 100 || !validHTTPHeaderName(headerName) {
			return normalizedExternalToolAuth{}, BadRequest("auth_header_name must be a valid HTTP header name")
		}
		if secret == "" {
			return normalizedExternalToolAuth{}, BadRequest("auth_secret is required for api_key authentication")
		}
		if username != "" {
			return normalizedExternalToolAuth{}, BadRequest("api_key authentication does not accept auth_username")
		}
	case "basic":
		if username == "" {
			return normalizedExternalToolAuth{}, BadRequest("auth_username is required for basic authentication")
		}
		if len([]rune(username)) > 200 {
			return normalizedExternalToolAuth{}, BadRequest("auth_username must not exceed 200 characters")
		}
		if secret == "" {
			return normalizedExternalToolAuth{}, BadRequest("auth_secret is required for basic authentication")
		}
		if headerName != "" {
			return normalizedExternalToolAuth{}, BadRequest("basic authentication does not accept auth_header_name")
		}
	default:
		return normalizedExternalToolAuth{}, BadRequest("auth_type must be none, bearer, api_key, or basic")
	}
	return normalizedExternalToolAuth{authType: authType, headerName: headerName, username: username, secret: secret}, nil
}

// validHTTPHeaderName applies the RFC token character set used by HTTP header field names.
func validHTTPHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			continue
		}
		switch char {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}

// normalizeExternalToolEndpoint accepts explicit HTTP(S) endpoints and rejects embedded credentials.
func normalizeExternalToolEndpoint(value string) (string, error) {
	raw := strings.TrimSpace(value)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", BadRequest("endpoint_url must be an absolute http or https URL")
	}
	if parsed.User != nil {
		return "", BadRequest("endpoint_url must not contain embedded credentials")
	}
	parsed.Fragment = ""
	return parsed.String(), nil
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
	if strings.TrimSpace(model.APIKeyCiphertext) != "" {
		if c.CredentialCipher() == nil {
			return domain.AgentModel{}, domain.E(503, "service_unavailable", "agent model credential storage is not configured")
		}
		plaintext, err := c.CredentialCipher().Decrypt(model.APIKeyCiphertext, domain.AgentModelCredentialAAD(model.TenantID, model.ID))
		if err != nil {
			return domain.AgentModel{}, domain.E(500, "internal_error", "failed to open agent model credential")
		}
		model.APIKey = string(plaintext)
	}
	return model, nil
}

// protectAgentModelCredential encrypts an API key before the model reaches any repository implementation.
func (c AgentService) protectAgentModelCredential(model domain.AgentModel) (domain.AgentModel, error) {
	if strings.TrimSpace(model.APIKey) == "" {
		return domain.AgentModel{}, BadRequest("api_key is required")
	}
	if c.CredentialCipher() == nil {
		return domain.AgentModel{}, domain.E(503, "service_unavailable", "agent model credential storage is not configured")
	}
	ciphertext, err := c.CredentialCipher().Encrypt([]byte(model.APIKey), domain.AgentModelCredentialAAD(model.TenantID, model.ID))
	if err != nil {
		return domain.AgentModel{}, domain.E(500, "internal_error", "failed to protect agent model credential")
	}
	model.APIKeyCiphertext = ciphertext
	model.APIKeyPreview = maskAgentModelAPIKey(model.APIKey)
	model.APIKeySet = true
	model.APIKey = ""
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
	model.LiteLLMModel = domain.AgentModelLiteLLMAlias(model.ID)
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
	if model.SyncStatus == "" {
		model.SyncStatus = domain.AgentModelSyncStatusPending
	}
	return model, nil
}

// appendAgentModelSyncEvent 在模型資料交易內追加不含密鑰的 LiteLLM 同步事件。
func (c AgentService) appendAgentModelSyncEvent(ctx RequestContext, modelID string, eventType domain.EventType) error {
	payload, err := domain.AgentModelSyncPayload{ModelID: modelID}.Map()
	if err != nil {
		return err
	}
	return c.store.AppendOutboxEvent(goContext(ctx), domain.OutboxEvent{
		ID:            utils.NewID("outbox"),
		TenantID:      ctx.TenantID,
		EventType:     string(eventType),
		AggregateType: domain.OutboxAggregateAgentModel,
		AggregateID:   modelID,
		Payload:       payload,
		Status:        "pending",
		CreatedAt:     c.Now(),
	})
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
	agent.WelcomeMessage = strings.TrimSpace(agent.WelcomeMessage)
	if utf8.RuneCountInString(agent.WelcomeMessage) > 1000 {
		return domain.AgentDefinition{}, BadRequest("welcome_message supports at most 1000 characters")
	}
	if len(agent.SuggestedQuestionTranslations) == 0 && len(agent.SuggestedQuestions) > 0 {
		agent.SuggestedQuestionTranslations = localizedQuestionsFromLegacy(agent.SuggestedQuestions)
	}
	translations, err := normalizeSuggestedQuestionTranslations(agent.SuggestedQuestionTranslations)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	agent.SuggestedQuestionTranslations = translations
	agent.SuggestedQuestions = domain.LocalizedSuggestedQuestions(
		agent.SuggestedQuestionTranslations,
		domain.DefaultPreferredLocale,
		nil,
	)
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
	if agent.PublishedVersion < 0 || agent.PublishedVersion > agent.Version {
		return domain.AgentDefinition{}, BadRequest("published_version must reference an existing version")
	}
	agent.MainAgentRole = strings.TrimSpace(agent.MainAgentRole)
	if agent.MainAgentRole == "" {
		agent.MainAgentRole = "理解使用者目標，按子 Agent 的職責進行委派，並驗證與彙總最終結果。"
	}
	agent.Tools = uniqueStrings(agent.Tools)
	if err := validateAgentTools(agent.Tools); err != nil {
		return domain.AgentDefinition{}, err
	}
	agent.ExternalToolIDs = uniqueStrings(agent.ExternalToolIDs)
	if err := c.validateExternalToolReferences(ctx, agent.ExternalToolIDs); err != nil {
		return domain.AgentDefinition{}, err
	}
	agent.KnowledgeBaseIDs = uniqueStrings(agent.KnowledgeBaseIDs)
	if err := c.validateKnowledgeBaseReferences(ctx, agent.KnowledgeBaseIDs); err != nil {
		return domain.AgentDefinition{}, err
	}
	if len(agent.SubAgents) > 6 {
		return domain.AgentDefinition{}, BadRequest("sub_agents supports at most 6 members")
	}
	seenMemberIDs := map[string]struct{}{}
	seenMemberNames := map[string]struct{}{}
	for index := range agent.SubAgents {
		member := &agent.SubAgents[index]
		member.ID = strings.TrimSpace(member.ID)
		if member.ID == "" {
			member.ID = utils.NewID("asub")
		}
		member.Name = strings.TrimSpace(member.Name)
		if member.Name == "" {
			return domain.AgentDefinition{}, BadRequest("sub agent name is required")
		}
		member.Role = strings.TrimSpace(member.Role)
		if member.Role == "" {
			return domain.AgentDefinition{}, BadRequest("sub agent role is required")
		}
		member.ModelID = strings.TrimSpace(member.ModelID)
		if member.ModelID == "" {
			member.ModelID = agent.ModelID
		}
		if err := c.requireAgentModelReference(ctx, member.ModelID); err != nil {
			return domain.AgentDefinition{}, err
		}
		member.Tools = uniqueStrings(member.Tools)
		if err := validateAgentTools(member.Tools); err != nil {
			return domain.AgentDefinition{}, err
		}
		member.ExternalToolIDs = uniqueStrings(member.ExternalToolIDs)
		if err := c.validateExternalToolReferences(ctx, member.ExternalToolIDs); err != nil {
			return domain.AgentDefinition{}, err
		}
		member.KnowledgeBaseIDs = uniqueStrings(member.KnowledgeBaseIDs)
		if err := c.validateKnowledgeBaseReferences(ctx, member.KnowledgeBaseIDs); err != nil {
			return domain.AgentDefinition{}, err
		}
		if _, exists := seenMemberIDs[member.ID]; exists {
			return domain.AgentDefinition{}, BadRequest("sub agent id must be unique")
		}
		nameKey := strings.ToLower(member.Name)
		if _, exists := seenMemberNames[nameKey]; exists {
			return domain.AgentDefinition{}, BadRequest("sub agent name must be unique")
		}
		seenMemberIDs[member.ID] = struct{}{}
		seenMemberNames[nameKey] = struct{}{}
	}
	agent.VisibilityTargets = uniqueStrings(agent.VisibilityTargets)
	if err := c.validateAgentVisibilityTargets(ctx, &agent); err != nil {
		return domain.AgentDefinition{}, err
	}
	return agent, nil
}

// normalizeAgentDefinitionResponse keeps Agent JSON collections non-null without changing shared slice semantics.
func normalizeAgentDefinitionResponse(agent domain.AgentDefinition) domain.AgentDefinition {
	agent.SubAgents = agentResponseSlice(agent.SubAgents)
	for index := range agent.SubAgents {
		agent.SubAgents[index].Tools = agentResponseSlice(agent.SubAgents[index].Tools)
		agent.SubAgents[index].KnowledgeBaseIDs = agentResponseSlice(agent.SubAgents[index].KnowledgeBaseIDs)
	}
	agent.SuggestedQuestions = agentResponseSlice(agent.SuggestedQuestions)
	agent.SuggestedQuestionTranslations = agentResponseSlice(agent.SuggestedQuestionTranslations)
	agent.Tools = agentResponseSlice(agent.Tools)
	agent.KnowledgeBaseIDs = agentResponseSlice(agent.KnowledgeBaseIDs)
	agent.VisibilityTargets = agentResponseSlice(agent.VisibilityTargets)
	agent.Versions = agentResponseSlice(agent.Versions)
	for index := range agent.Versions {
		version := &agent.Versions[index]
		version.SubAgents = agentResponseSlice(version.SubAgents)
		for memberIndex := range version.SubAgents {
			version.SubAgents[memberIndex].Tools = agentResponseSlice(version.SubAgents[memberIndex].Tools)
			version.SubAgents[memberIndex].KnowledgeBaseIDs = agentResponseSlice(version.SubAgents[memberIndex].KnowledgeBaseIDs)
		}
		version.SuggestedQuestions = agentResponseSlice(version.SuggestedQuestions)
		version.SuggestedQuestionTranslations = agentResponseSlice(version.SuggestedQuestionTranslations)
		version.Tools = agentResponseSlice(version.Tools)
		version.KnowledgeBaseIDs = agentResponseSlice(version.KnowledgeBaseIDs)
	}
	agent.Usage.TopPrompts = agentResponseSlice(agent.Usage.TopPrompts)
	return agent
}

// agentResponseSlice clones an Agent collection into a stable non-nil response slice.
func agentResponseSlice[T any](items []T) []T {
	result := make([]T, len(items))
	copy(result, items)
	return result
}

// normalizeSuggestedQuestionTranslations validates supported locales while preserving question order.
func normalizeSuggestedQuestionTranslations(
	items []domain.LocalizedAgentSuggestedQuestion,
) ([]domain.LocalizedAgentSuggestedQuestion, error) {
	if len(items) > domain.MaxAgentSuggestedQuestions {
		return nil, BadRequest(fmt.Sprintf(
			"suggested_question_translations supports at most %d items",
			domain.MaxAgentSuggestedQuestions,
		))
	}
	result := make([]domain.LocalizedAgentSuggestedQuestion, 0, len(items))
	for _, item := range items {
		translations := make(map[string]string, len(item.Translations))
		for locale, value := range item.Translations {
			locale = strings.TrimSpace(locale)
			if locale != domain.PreferredLocaleZHTW && locale != domain.PreferredLocaleENUS {
				return nil, BadRequest("suggested question locale must be zh-TW or en-US")
			}
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if utf8.RuneCountInString(value) > domain.MaxAgentSuggestedQuestionCharacters {
				return nil, BadRequest(fmt.Sprintf(
					"each suggested question translation supports at most %d characters",
					domain.MaxAgentSuggestedQuestionCharacters,
				))
			}
			translations[locale] = value
		}
		if len(translations) > 0 {
			result = append(result, domain.LocalizedAgentSuggestedQuestion{Translations: translations})
		}
	}
	return result, nil
}

// localizedQuestionsFromLegacy upgrades the previous default-language array without changing its order.
func localizedQuestionsFromLegacy(values []string) []domain.LocalizedAgentSuggestedQuestion {
	values = uniqueStrings(values)
	result := make([]domain.LocalizedAgentSuggestedQuestion, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result = append(result, domain.LocalizedAgentSuggestedQuestion{Translations: map[string]string{
			domain.DefaultPreferredLocale: value,
		}})
	}
	return result
}

// localizedSuggestedQuestions resolves the account locale with deterministic per-question fallback.

// validateKnowledgeBaseReferences 保證 Agent 只能綁定目前租戶存在的知識庫。
func (c AgentService) validateKnowledgeBaseReferences(ctx RequestContext, ids []string) error {
	for _, id := range ids {
		if _, _, err := c.requireAgentAuthz(ctx, knowledgeBaseResource, ActionRead, id); err != nil {
			return err
		}
		if _, ok, err := c.store.GetKnowledgeBase(goContext(ctx), ctx.TenantID, id); err != nil {
			return err
		} else if !ok {
			return BadRequest("knowledge base does not exist: " + id)
		}
	}
	return nil
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
			_, ok, err := c.Service.Store().GetOrgUnit(goContext(ctx), ctx.TenantID, targetID)
			if err != nil {
				return err
			}
			if !ok {
				return BadRequest("visibility target org unit does not exist: " + targetID)
			}
		case domain.AgentVisibilityRole:
			_, ok, err := c.Service.Store().GetAssumableRole(goContext(ctx), ctx.TenantID, targetID)
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
	for _, tool := range domain.AgentToolCatalog() {
		catalog[tool.Value] = struct{}{}
	}
	for _, tool := range tools {
		if _, ok := catalog[tool]; !ok {
			return BadRequest("agent tool is invalid: " + tool)
		}
	}
	return nil
}

func (c AgentService) validateExternalToolReferences(ctx RequestContext, capabilityIDs []string) error {
	if len(capabilityIDs) == 0 {
		return nil
	}
	if len(capabilityIDs) > 32 {
		return BadRequest("external_tool_ids supports at most 32 capabilities")
	}
	connections, err := c.store.ListAgentExternalTools(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	available := make(map[string]struct{})
	for _, connection := range connections {
		if connection.Status != string(domain.ExternalToolConnectionStatusActive) || connection.ArchivedAt != nil {
			continue
		}
		for _, capability := range connection.Capabilities {
			if capability.Enabled && capability.ArchivedAt == nil {
				available[capability.ID] = struct{}{}
			}
		}
	}
	for _, capabilityID := range capabilityIDs {
		if _, ok := available[capabilityID]; !ok {
			return BadRequest("external tool capability is unavailable: " + capabilityID)
		}
	}
	return nil
}

func (c AgentService) snapshotAgentDefinition(ctx RequestContext, agent domain.AgentDefinition, actorID, note string) error {
	revisionID := strings.TrimSpace(agent.DraftRevisionID)
	if revisionID == "" {
		revisionID = agentDefinitionRevisionID(agent.ID, agent.Version)
	}
	model, err := c.currentAgentModel(ctx, agent.ModelID)
	if err != nil {
		return err
	}
	modelChecksum := domain.AgentModelSyncConfigHash(model)
	revisionMembers := append([]domain.AgentTeamMember(nil), agent.SubAgents...)
	for index := range revisionMembers {
		memberModel, memberErr := c.currentAgentModel(ctx, revisionMembers[index].ModelID)
		if memberErr != nil {
			return memberErr
		}
		revisionMembers[index].ModelConfigChecksum = domain.AgentModelSyncConfigHash(memberModel)
	}
	signatureAgent := agent
	signatureAgent.SubAgents = revisionMembers
	contentChecksum := fmt.Sprintf("%x", sha256.Sum256([]byte(agentDefinitionRuntimeSignature(signatureAgent)+"\x00"+modelChecksum)))
	return c.store.InsertAgentDefinitionVersion(goContext(ctx), domain.AgentDefinitionVersion{
		ID:                            revisionID,
		TenantID:                      ctx.TenantID,
		AgentID:                       agent.ID,
		Version:                       agent.Version,
		Name:                          agent.Name,
		Description:                   agent.Description,
		Emoji:                         agent.Emoji,
		Category:                      agent.Category,
		Visibility:                    agent.Visibility,
		VisibilityTargets:             agent.VisibilityTargets,
		MainAgentRole:                 agent.MainAgentRole,
		SubAgents:                     revisionMembers,
		SystemPrompt:                  agent.SystemPrompt,
		WelcomeMessage:                agent.WelcomeMessage,
		SuggestedQuestions:            agent.SuggestedQuestions,
		SuggestedQuestionTranslations: agent.SuggestedQuestionTranslations,
		Tools:                         agent.Tools,
		ExternalToolIDs:               agent.ExternalToolIDs,
		KnowledgeBaseIDs:              agent.KnowledgeBaseIDs,
		ModelID:                       agent.ModelID,
		ModelConfigChecksum:           modelChecksum,
		TimeoutSeconds:                agent.TimeoutSeconds,
		ConfigSchemaVersion:           1,
		Checksum:                      contentChecksum,
		Note:                          note,
		CreatedByAccountID:            actorID,
		CreatedAt:                     c.Now(),
	})
}

// agentDefinitionRevisionID matches the immutable identity written by the
// compatibility aggregate query, preventing a second insert for the same
// (agent, revision number) under a different primary key.
func agentDefinitionRevisionID(agentID string, version int) string {
	if version < 1 {
		version = 1
	}
	return strings.TrimSpace(agentID) + ":revision:" + strconv.Itoa(version)
}

// agentDefinitionChildRevisionID addresses a child's immutable capability
// bindings inside one root revision snapshot.
func agentDefinitionChildRevisionID(rootRevisionID, memberID string) string {
	return strings.TrimSpace(rootRevisionID) + ":member:" + strings.TrimSpace(memberID)
}

// agentDefinitionRuntimeSignature 生成會影響真實執行的穩定配置簽名。
func agentDefinitionRuntimeSignature(agent domain.AgentDefinition) string {
	payload := struct {
		Name                          string                                   `json:"name"`
		Description                   string                                   `json:"description"`
		Emoji                         string                                   `json:"emoji"`
		Category                      domain.AgentCategory                     `json:"category"`
		Visibility                    domain.AgentVisibility                   `json:"visibility"`
		VisibilityTargets             []string                                 `json:"visibility_targets"`
		MainAgentRole                 string                                   `json:"main_agent_role"`
		SubAgents                     []domain.AgentTeamMember                 `json:"sub_agents"`
		SystemPrompt                  string                                   `json:"system_prompt"`
		WelcomeMessage                string                                   `json:"welcome_message"`
		SuggestedQuestions            []string                                 `json:"suggested_questions"`
		SuggestedQuestionTranslations []domain.LocalizedAgentSuggestedQuestion `json:"suggested_question_translations"`
		Tools                         []string                                 `json:"tools"`
		ExternalToolIDs               []string                                 `json:"external_tool_ids"`
		KnowledgeBaseIDs              []string                                 `json:"knowledge_base_ids"`
		ModelID                       string                                   `json:"model_id"`
		TimeoutSeconds                int                                      `json:"timeout_seconds"`
	}{
		Name:                          agent.Name,
		Description:                   agent.Description,
		Emoji:                         agent.Emoji,
		Category:                      agent.Category,
		Visibility:                    agent.Visibility,
		VisibilityTargets:             agent.VisibilityTargets,
		MainAgentRole:                 agent.MainAgentRole,
		SubAgents:                     agent.SubAgents,
		SystemPrompt:                  agent.SystemPrompt,
		WelcomeMessage:                agent.WelcomeMessage,
		SuggestedQuestions:            agent.SuggestedQuestions,
		SuggestedQuestionTranslations: agent.SuggestedQuestionTranslations,
		Tools:                         agent.Tools,
		ExternalToolIDs:               agent.ExternalToolIDs,
		KnowledgeBaseIDs:              agent.KnowledgeBaseIDs,
		ModelID:                       agent.ModelID,
		TimeoutSeconds:                agent.TimeoutSeconds,
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

// recordAgentAdminAudit writes Agent administration events to the canonical audit log.
func (c AgentService) recordAgentAdminAudit(ctx RequestContext, account Account, entityType, entityID, entityName, action, detail string) error {
	payload := map[string]any{
		"entity_type":        entityType,
		"entity_id":          entityID,
		"entity_name":        entityName,
		"actor_display_name": account.DisplayName,
		"detail":             detail,
	}
	if raw, err := json.Marshal(payload); err == nil {
		payload["raw"] = string(raw)
	}
	return c.RecordAudit(ctx, "ai.agent."+entityType+"."+action, "agent_"+entityType, entityID, "high", payload)
}
