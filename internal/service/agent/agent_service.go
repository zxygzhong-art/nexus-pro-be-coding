package agent

import (
	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
	"sort"
	"strings"
)

// AgentService 定義 agent 服務的資料結構。
type AgentService struct {
	*Service
	store agentStore
}

// ListRuns 列出執行紀錄的服務流程。
func (c AgentService) ListRuns(ctx RequestContext) ([]AgentRun, error) {
	account, decision, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionRead, "")
	if err != nil {
		return nil, err
	}
	if accountIDs, ok := agentAccountIDsForDecision(account, decision); ok && len(accountIDs) == 1 {
		return c.store.ListAgentRunsByAccount(goContext(ctx), ctx.TenantID, accountIDs[0])
	}
	items, err := c.store.ListAgentRuns(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	return filterAgentRunsByDecision(account, decision, items), nil
}

// ListRunPage 列出執行分頁的服務流程。
func (c AgentService) ListRunPage(ctx RequestContext, page PageRequest) (PageResponse[AgentRun], error) {
	account, decision, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionRead, "")
	if err != nil {
		return PageResponse[AgentRun]{}, err
	}
	page = utils.NormalizePageRequest(page)
	if accountIDs, ok := agentAccountIDsForDecision(account, decision); ok && len(accountIDs) == 1 {
		items, total, err := c.store.ListAgentRunPageByAccount(goContext(ctx), ctx.TenantID, accountIDs[0], page)
		if err != nil {
			return PageResponse[AgentRun]{}, err
		}
		return utils.PageResponseFromStore(items, total, page), nil
	}
	if decision.Scope == "" || decision.Scope == ScopeAll || decision.Scope == ScopeTenant || decision.Scope == ScopeSystem {
		items, total, err := c.store.ListAgentRunPage(goContext(ctx), ctx.TenantID, page)
		if err != nil {
			return PageResponse[AgentRun]{}, err
		}
		return utils.PageResponseFromStore(items, total, page), nil
	}
	items, err := c.store.ListAgentRuns(goContext(ctx), ctx.TenantID)
	if err != nil {
		return PageResponse[AgentRun]{}, err
	}
	items = filterAgentRunsByDecision(account, decision, items)
	sortAgentRuns(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CreateRun 建立執行的服務流程。
func (c AgentService) CreateRun(ctx RequestContext, input CreateAgentRunInput) (AgentRun, error) {
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionCreate, "")
	if err != nil {
		return AgentRun{}, err
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return AgentRun{}, BadRequest("prompt is required")
	}
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "policy_qa"
	}
	agentID := strings.TrimSpace(input.AgentID)
	userPrompt := strings.TrimSpace(input.Prompt)
	knowledgeBaseIDs := []string{}
	if agentID != "" {
		definition, err := c.publishedAgentDefinition(ctx, agentID)
		if err != nil {
			return AgentRun{}, err
		}
		knowledgeBaseIDs = definition.KnowledgeBaseIDs
	}

	run := AgentRun{
		ID:        utils.NewID("arun"),
		TenantID:  ctx.TenantID,
		AccountID: account.ID,
		AgentID:   agentID,
		Mode:      mode,
		Prompt:    userPrompt,
		Status:    string(AgentRunStatusQueued),
		CreatedAt: c.Now(),
		UpdatedAt: c.Now(),
	}
	if err := c.store.UpsertAgentRun(goContext(ctx), run); err != nil {
		return AgentRun{}, err
	}
	c.LogInfo(ctx, "agent run created",
		"run_id", run.ID,
		"mode", run.Mode,
		"status", run.Status,
	)
	run, err = c.transitionRun(ctx, run, AgentRunStatusRunning)
	if err != nil {
		return AgentRun{}, err
	}
	toolResult, err := c.AgentToolGateway().Call(ctx, AgentToolCall{
		Name: "knowledge.search",
		Authz: CheckRequest{
			ApplicationCode: AppAgent,
			ResourceType:    ResourceTool,
			ResourceID:      "knowledge.search",
			Action:          ActionCall,
		},
		Execute: func() (AgentToolResult, error) {
			answer, refs, err := c.answerAgentPrompt(ctx, userPrompt, knowledgeBaseIDs)
			if err != nil {
				return AgentToolResult{}, err
			}
			return AgentToolResult{Answer: answer, References: refs}, nil
		},
	})
	if err != nil {
		_ = c.FailRun(ctx, run, err)
		return AgentRun{}, err
	}
	run.Answer = toolResult.Answer
	run.References = toolResult.References
	run.ToolDecisions = []CheckResult{toolResult.Decision}
	run, err = c.transitionRun(ctx, run, AgentRunStatusCompleted)
	if err != nil {
		return AgentRun{}, err
	}
	if err := c.RecordAudit(ctx, "ai.agent.run.create", "agent_run", run.ID, "high", map[string]any{
		"mode":          run.Mode,
		"tool":          toolResult.Name,
		"tool_allowed":  toolResult.Decision.Allowed,
		"tool_boundary": toolResult.Decision.PermissionBoundary,
	}); err != nil {
		return AgentRun{}, err
	}
	return run, nil
}

// transitionRun 轉換執行的服務流程。
func (c AgentService) transitionRun(ctx RequestContext, run AgentRun, status AgentRunStatus) (AgentRun, error) {
	previousStatus := run.Status
	now := c.Now()
	run.Status = string(status)
	run.UpdatedAt = now
	switch status {
	case AgentRunStatusRunning:
		if run.StartedAt == nil {
			run.StartedAt = &now
		}
	case AgentRunStatusCompleted, AgentRunStatusFailed, AgentRunStatusCancelled:
		run.CompletedAt = &now
	}
	if err := c.store.UpsertAgentRun(goContext(ctx), run); err != nil {
		return AgentRun{}, err
	}
	c.LogInfo(ctx, "agent run status changed",
		"run_id", run.ID,
		"mode", run.Mode,
		"previous_status", previousStatus,
		"status", run.Status,
	)
	return run, nil
}

// FailRun persists a sanitized legacy runtime failure without exposing the raw provider cause.
func (c AgentService) FailRun(ctx RequestContext, run AgentRun, cause error) error {
	run.Answer = agentRuntimeFailureAnswer(ctx)
	c.LogWarn(ctx, "agent run failed",
		"run_id", run.ID,
		"mode", run.Mode,
		"error", cause,
	)
	_, err := c.transitionRun(ctx, run, AgentRunStatusFailed)
	return err
}

// filterAgentRunsByDecision 處理篩選 agent 執行紀錄 by 決策。
func filterAgentRunsByDecision(account Account, decision CheckResult, items []AgentRun) []AgentRun {
	if decision.Scope == "" || decision.Scope == ScopeAll || decision.Scope == ScopeTenant || decision.Scope == ScopeSystem {
		return append([]AgentRun(nil), items...)
	}
	accountIDs, _ := agentAccountIDsForDecision(account, decision)
	allowed := stringSet(accountIDs)
	out := make([]AgentRun, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.AccountID]; ok {
			out = append(out, item)
		}
	}
	return out
}

// agentAccountIDsForDecision 處理 agent 帳號 IDs for 決策。
func agentAccountIDsForDecision(account Account, decision CheckResult) ([]string, bool) {
	if decision.Scope == "" || decision.Scope == ScopeAll || decision.Scope == ScopeTenant || decision.Scope == ScopeSystem {
		return nil, false
	}
	accountIDs := stringSliceFromAny(decision.Conditions["account_ids"])
	if len(accountIDs) == 0 {
		accountIDs = []string{account.ID}
	}
	return uniqueStrings(accountIDs), true
}

// sortAgentRuns 排序agent 執行紀錄。
func sortAgentRuns(items []AgentRun, order string) {
	sort.SliceStable(items, func(i, j int) bool {
		switch order {
		case "created_at_asc":
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		default:
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
	})
}

// answerAgentPrompt 僅在 Agent 綁定的知識庫中搜尋並回傳可追溯引用。
func (c AgentService) answerAgentPrompt(ctx RequestContext, prompt string, knowledgeBaseIDs []string) (string, []Reference, error) {
	result, err := New(c.Service).SearchKnowledge(ctx, domain.KnowledgeSearchInput{Query: prompt, KnowledgeBaseIDs: knowledgeBaseIDs})
	if err != nil {
		return "", nil, err
	}
	if len(result.Hits) == 0 {
		return "當前 Agent 綁定的知識庫中沒有匹配內容。", nil, nil
	}
	references := make([]Reference, 0, len(result.Hits))
	for _, hit := range result.Hits {
		references = append(references, Reference{Title: hit.Title, Snippet: hit.Snippet, Source: hit.Source})
	}
	return result.Hits[0].Snippet, references, nil
}

func (c AgentService) publishedAgentDefinition(ctx RequestContext, id string) (domain.AgentDefinition, error) {
	agent, ok, err := c.store.GetAgentDefinition(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	if !ok || agent.Status != domain.AgentDefinitionStatusPublished {
		return domain.AgentDefinition{}, NotFound("published agent definition", id)
	}
	publishedVersion := agent.PublishedVersion
	publishedModelChecksum := ""
	if publishedVersion <= 0 {
		publishedVersion = agent.Version
	}
	if snapshot, found, snapshotErr := c.store.GetAgentDefinitionVersion(goContext(ctx), ctx.TenantID, agent.ID, publishedVersion); snapshotErr != nil {
		return domain.AgentDefinition{}, snapshotErr
	} else if found {
		publishedModelChecksum = snapshot.ModelConfigChecksum
		agent.PublishedRevisionID = snapshot.ID
		agent.Name = snapshot.Name
		agent.Description = snapshot.Description
		agent.Emoji = snapshot.Emoji
		agent.Category = snapshot.Category
		agent.Visibility = snapshot.Visibility
		agent.VisibilityTargets = snapshot.VisibilityTargets
		agent.MainAgentRole = snapshot.MainAgentRole
		agent.SubAgents = snapshot.SubAgents
		agent.SystemPrompt = snapshot.SystemPrompt
		agent.WelcomeMessage = snapshot.WelcomeMessage
		agent.SuggestedQuestions = snapshot.SuggestedQuestions
		agent.SuggestedQuestionTranslations = snapshot.SuggestedQuestionTranslations
		agent.Tools = snapshot.Tools
		agent.ExternalToolIDs = snapshot.ExternalToolIDs
		agent.KnowledgeBaseIDs = snapshot.KnowledgeBaseIDs
		agent.ModelID = snapshot.ModelID
		agent.TimeoutSeconds = snapshot.TimeoutSeconds
	}
	account, _, err := c.ResolveAccount(ctx)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	visible, err := c.AgentDefinitionVisibleToAccount(ctx, account, agent)
	if err != nil {
		return domain.AgentDefinition{}, err
	}
	if !visible {
		return domain.AgentDefinition{}, NotFound("published agent definition", id)
	}
	if err := c.validatePublishedAgentModelBindings(ctx, agent, publishedModelChecksum); err != nil {
		return domain.AgentDefinition{}, err
	}
	return agent, nil
}

// validatePublishedAgentModelBindings makes model routing immutable at the
// revision boundary. Any desired model change requires a new publish before
// chat or trial execution can resume.
func (c AgentService) validatePublishedAgentModelBindings(ctx RequestContext, agent domain.AgentDefinition, rootChecksum string) error {
	validate := func(modelID, expectedChecksum string) error {
		model, err := c.currentAgentModel(ctx, modelID)
		if err != nil {
			return err
		}
		if model.Status != domain.AgentModelStatusActive || strings.TrimSpace(expectedChecksum) == "" || domain.AgentModelSyncConfigHash(model) != expectedChecksum {
			return Conflict("published agent model configuration changed; republish the agent definition").WithReasonCode("agent_model_config_drift")
		}
		return nil
	}
	if err := validate(agent.ModelID, rootChecksum); err != nil {
		return err
	}
	for _, member := range agent.SubAgents {
		if err := validate(member.ModelID, member.ModelConfigChecksum); err != nil {
			return err
		}
	}
	return nil
}
