package service

import (
	"fmt"
	"nexus-pro-be/internal/utils"
	"sort"
	"strings"
)

// AgentService implements agent run orchestration and tool authorization.
type AgentService struct {
	*Service
	store agentStore
}

// Agent returns the agent service facade.
func (c *Service) Agent() AgentService {
	return AgentService{Service: c, store: c.store}
}

// ListRuns returns all visible agent runs for the current tenant.
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

// ListRunPage returns a paginated list of visible agent runs.
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

// CreateRun creates and completes an agent run after tool authorization.
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

	run := AgentRun{
		ID:        utils.NewID("arun"),
		TenantID:  ctx.TenantID,
		AccountID: account.ID,
		Mode:      mode,
		Prompt:    strings.TrimSpace(input.Prompt),
		Status:    string(AgentRunStatusQueued),
		CreatedAt: c.Now(),
		UpdatedAt: c.Now(),
	}
	if err := c.store.UpsertAgentRun(goContext(ctx), run); err != nil {
		return AgentRun{}, err
	}
	c.logInfo(ctx, "agent run created",
		"run_id", run.ID,
		"mode", run.Mode,
		"status", run.Status,
	)
	run, err = c.transitionRun(ctx, run, AgentRunStatusRunning)
	if err != nil {
		return AgentRun{}, err
	}
	toolResult, err := c.agentToolGateway().Call(ctx, AgentToolCall{
		Name: "knowledge.search",
		Authz: CheckRequest{
			ApplicationCode: AppAgent,
			ResourceType:    ResourceTool,
			ResourceID:      "knowledge.search",
			Action:          ActionCall,
		},
		Execute: func() (AgentToolResult, error) {
			answer, refs, err := c.answerAgentPrompt(ctx, run.Prompt)
			if err != nil {
				return AgentToolResult{}, err
			}
			return AgentToolResult{Answer: answer, References: refs}, nil
		},
	})
	if err != nil {
		_ = c.failRun(ctx, run, err)
		return AgentRun{}, err
	}
	run.Answer = toolResult.Answer
	run.References = toolResult.References
	run.ToolDecisions = []CheckResult{toolResult.Decision}
	run, err = c.transitionRun(ctx, run, AgentRunStatusCompleted)
	if err != nil {
		return AgentRun{}, err
	}
	if err := c.audit(ctx, "ai.agent.run.create", "agent_run", run.ID, "high", map[string]any{
		"mode":          run.Mode,
		"tool":          toolResult.Name,
		"tool_allowed":  toolResult.Decision.Allowed,
		"tool_boundary": toolResult.Decision.PermissionBoundary,
	}); err != nil {
		return AgentRun{}, err
	}
	return run, nil
}

func (c AgentService) transitionRun(ctx RequestContext, run AgentRun, status AgentRunStatus) (AgentRun, error) {
	previousStatus := run.Status
	run.Status = string(status)
	run.UpdatedAt = c.Now()
	if err := c.store.UpsertAgentRun(goContext(ctx), run); err != nil {
		return AgentRun{}, err
	}
	c.logInfo(ctx, "agent run status changed",
		"run_id", run.ID,
		"mode", run.Mode,
		"previous_status", previousStatus,
		"status", run.Status,
	)
	return run, nil
}

func (c AgentService) failRun(ctx RequestContext, run AgentRun, cause error) error {
	run.Answer = cause.Error()
	c.logWarn(ctx, "agent run failed",
		"run_id", run.ID,
		"mode", run.Mode,
		"error", cause.Error(),
	)
	_, err := c.transitionRun(ctx, run, AgentRunStatusFailed)
	return err
}

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

type agentToolCaller interface {
	Call(ctx RequestContext, call AgentToolCall) (AgentToolResult, error)
}

// AgentToolCall describes one tool invocation guarded by authorization.
type AgentToolCall struct {
	Name    string
	Authz   CheckRequest
	Execute func() (AgentToolResult, error)
}

// AgentToolResult returns the outcome of an authorized agent tool call.
type AgentToolResult struct {
	Name       string
	Decision   CheckResult
	Answer     string
	References []Reference
	Data       map[string]any
}

type authzToolGateway struct {
	service *Service
}

func (c *Service) agentToolGateway() agentToolCaller {
	return authzToolGateway{service: c}
}

// Call authorizes and executes one agent tool invocation.
func (g authzToolGateway) Call(ctx RequestContext, call AgentToolCall) (AgentToolResult, error) {
	account, _, err := g.service.resolveAccount(ctx)
	if err != nil {
		return AgentToolResult{}, err
	}
	req := call.Authz
	if req.ApplicationCode == "" {
		req.ApplicationCode = "agent"
	}
	if req.ResourceType == "" {
		req.ResourceType = "tool"
	}
	if req.Action == "" {
		req.Action = "call"
	}
	if req.ResourceID == "" {
		req.ResourceID = call.Name
	}
	decision, err := g.service.evaluateAuthz(ctx, account, req)
	if err != nil {
		return AgentToolResult{}, err
	}
	if !decision.Allowed {
		return AgentToolResult{Name: call.Name, Decision: decision}, Forbidden("agent tool call denied: " + decision.Reason)
	}
	if decision.RequiresApproval {
		if err := g.service.confirmApproval(ctx, req); err != nil {
			return AgentToolResult{Name: call.Name, Decision: decision}, err
		}
	}
	if call.Execute == nil {
		return AgentToolResult{Name: call.Name, Decision: decision}, nil
	}
	result, err := call.Execute()
	if err != nil {
		return AgentToolResult{}, err
	}
	result.Name = call.Name
	result.Decision = decision
	return result, nil
}

func (c *Service) answerAgentPrompt(ctx RequestContext, prompt string) (string, []Reference, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return "", nil, err
	}
	articles, err := c.store.ListKnowledgeArticles(goContext(ctx), ctx.TenantID)
	if err != nil {
		return "", nil, err
	}
	if len(articles) == 0 {
		return "当前租户没有可检索的知识库内容，已为你创建占位 Agent Run。", nil, nil
	}

	tokens := tokenize(prompt)
	matches := make([]KnowledgeArticle, 0)
	for _, article := range articles {
		decision, err := c.evaluateAuthz(ctx, account, CheckRequest{
			ApplicationCode: AppAgent,
			ResourceType:    ResourceKnowledgeArticle,
			ResourceID:      article.ID,
			Action:          ActionRead,
		})
		if err != nil {
			return "", nil, err
		}
		if !decision.Allowed {
			continue
		}
		if articleMatches(article, tokens) {
			matches = append(matches, article)
		}
	}
	if len(matches) == 0 {
		return "未检索到与当前问题匹配的知识库内容。", []Reference{}, nil
	}
	sortKnowledgeMatches(matches, tokens)

	refs := make([]Reference, 0, len(matches))
	lines := make([]string, 0, len(matches)+1)
	lines = append(lines, "基于租户知识库，给出以下建议：")
	for _, article := range matches {
		snippet := truncateRunes(article.Content, 120)
		refs = append(refs, Reference{
			Title:   article.Title,
			Snippet: snippet,
			Source:  "knowledge_article",
		})
		lines = append(lines, fmt.Sprintf("- %s: %s", article.Title, snippet))
	}
	if strings.Contains(strings.ToLower(prompt), "请假") || strings.Contains(strings.ToLower(prompt), "leave") {
		lines = append(lines, "建议优先引用请假制度、余额规则和审批流模板。")
	}
	return strings.Join(lines, "\n"), refs, nil
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

func tokenize(value string) []string {
	value = strings.ToLower(value)
	fields := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ' ', '\n', '\t', '，', '。', ',', '.', ';', ':', '/', '\\', '|', '(', ')', '[', ']', '{', '}', '!', '?', '"', '\'', '、':
			return true
		default:
			return false
		}
	})
	return uniqueStrings(fields)
}

func articleMatches(article KnowledgeArticle, tokens []string) bool {
	return articleMatchScore(article, tokens) > 0
}

func sortKnowledgeMatches(items []KnowledgeArticle, tokens []string) {
	sort.SliceStable(items, func(i, j int) bool {
		left := articleMatchScore(items[i], tokens)
		right := articleMatchScore(items[j], tokens)
		if left != right {
			return left > right
		}
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].Title < items[j].Title
	})
}

func articleMatchScore(article KnowledgeArticle, tokens []string) int {
	title := strings.ToLower(article.Title)
	body := strings.ToLower(article.Content + " " + strings.Join(article.Tags, " "))
	score := 0
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if strings.Contains(title, token) {
			score += 3
		}
		if strings.Contains(body, token) {
			score++
		}
	}
	return score
}
