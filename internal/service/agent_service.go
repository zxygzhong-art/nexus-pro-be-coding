package service

import (
	"strings"

	"nexus-pro-be/internal/utils"
)

type AgentService struct {
	*Service
	store agentStore
}

func (c *Service) Agent() AgentService {
	return AgentService{Service: c, store: c.store}
}

func (c *Service) ListAgentRuns(ctx RequestContext) ([]AgentRun, error) {
	return c.Agent().ListRuns(ctx)
}

func (c *Service) ListAgentRunPage(ctx RequestContext, page PageRequest) (PageResponse[AgentRun], error) {
	return c.Agent().ListRunPage(ctx, page)
}

func (c *Service) CreateAgentRun(ctx RequestContext, input CreateAgentRunInput) (AgentRun, error) {
	return c.Agent().CreateRun(ctx, input)
}

func (c AgentService) ListRuns(ctx RequestContext) ([]AgentRun, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return nil, err
	}
	return c.store.ListAgentRuns(goContext(ctx), ctx.TenantID)
}

func (c AgentService) ListRunPage(ctx RequestContext, page PageRequest) (PageResponse[AgentRun], error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return PageResponse[AgentRun]{}, err
	}
	page = utils.NormalizePageRequest(page)
	items, total, err := c.store.ListAgentRunPage(goContext(ctx), ctx.TenantID, page)
	if err != nil {
		return PageResponse[AgentRun]{}, err
	}
	return utils.PageResponseFromStore(items, total, page), nil
}

func (c AgentService) CreateRun(ctx RequestContext, input CreateAgentRunInput) (AgentRun, error) {
	account, _, err := c.resolveAccount(ctx)
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
