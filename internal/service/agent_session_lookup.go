package service

import (
	"strings"

	"nexus-pro-api/internal/domain"
)

// CurrentAgentSession 取回目前帳號可見的 Agent 會話（agent 子包與確認執行器共用）。
func (c *Service) CurrentAgentSession(ctx RequestContext, accountID, id string) (domain.AgentSession, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.AgentSession{}, BadRequest("id is required")
	}
	session, ok, err := c.store.GetAgentSession(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return domain.AgentSession{}, err
	}
	if !ok || session.AccountID != accountID {
		return domain.AgentSession{}, NotFound("agent session", id)
	}
	return session, nil
}
