package service

import (
	"strings"

	"nexus-pro-api/internal/domain"
)

// agent_tool_gateway.go — Agent 工具的授權閘道（依賴 authz 內部機制，屬基底 service）。

type AgentToolCaller interface {
	Call(ctx RequestContext, call AgentToolCall) (AgentToolResult, error)
}

// AgentToolCall 定義 agent 工具呼叫的資料結構。
type AgentToolCall struct {
	Name    string
	Authz   CheckRequest
	Execute func() (AgentToolResult, error)
}

// AgentToolResult 定義 agent 工具結果的資料結構。
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

// agentToolGateway 處理 agent 工具 gateway 的服務流程。
func (c *Service) AgentToolGateway() AgentToolCaller {
	return authzToolGateway{service: c}
}

// Call 呼叫目前流程。
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
	decision, err = g.applyOpenFGAAgentToolDecision(ctx, account, req, decision)
	if err != nil {
		return AgentToolResult{Name: call.Name, Decision: decision}, err
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

func (g authzToolGateway) applyOpenFGAAgentToolDecision(ctx RequestContext, account Account, req CheckRequest, decision CheckResult) (CheckResult, error) {
	if g.service == nil || !g.service.openFGAScopeChecksAvailable() {
		return decision, nil
	}
	toolID := strings.TrimSpace(req.ResourceID)
	if toolID == "" {
		return decision, nil
	}
	object := openFGATypeAgentTool + ":" + toolID
	subject := openFGASubjectTypeAccount + ":" + account.ID
	allowed, err := g.service.relationships.CheckRelationship(goContext(ctx), domain.RelationshipCheck{
		TenantID: ctx.TenantID,
		Subject:  subject,
		Relation: openFGARelationCanRun,
		Object:   object,
	})
	if err != nil {
		g.service.logWarn(ctx, "openfga agent tool can_run check failed; denying tool run",
			"tool_id", toolID,
			"error", err,
		)
		return decision, Forbidden("agent tool can_run relationship check unavailable")
	}
	if !allowed {
		decision.Allowed = false
		decision.Reason = "relationship denied"
		decision.MissingPermissions = uniqueStrings(append(decision.MissingPermissions, openFGATypeAgentTool+"."+openFGARelationCanRun))
		return decision, Forbidden("agent tool can_run relationship denied")
	}
	decision.MatchedBy = uniqueStrings(append(decision.MatchedBy, "openfga:"+object+"#"+openFGARelationCanRun))
	return decision, nil
}
