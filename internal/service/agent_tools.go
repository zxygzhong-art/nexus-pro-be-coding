package service

type agentToolCaller interface {
	Call(ctx RequestContext, call AgentToolCall) (AgentToolResult, error)
}

type AgentToolCall struct {
	Name    string
	Authz   CheckRequest
	Execute func() (AgentToolResult, error)
}

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
