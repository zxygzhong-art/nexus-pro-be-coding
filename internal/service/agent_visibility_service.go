package service

import (
	"strings"

	"nexus-pro-be/internal/domain"
)

// agentDefinitionVisibleToAccount 判斷已發布 Agent 是否對目前帳號與臨時角色可見。
func (c *Service) agentDefinitionVisibleToAccount(ctx RequestContext, account Account, agent domain.AgentDefinition) (bool, error) {
	targets := make(map[string]struct{}, len(agent.VisibilityTargets))
	for _, target := range agent.VisibilityTargets {
		if target = strings.TrimSpace(target); target != "" {
			targets[target] = struct{}{}
		}
	}
	switch agent.Visibility {
	case "", domain.AgentVisibilityAll:
		return true, nil
	case domain.AgentVisibilityDepartment:
		if len(targets) == 0 || strings.TrimSpace(account.EmployeeID) == "" {
			return false, nil
		}
		employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, account.EmployeeID)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
		_, visible := targets[strings.TrimSpace(employee.OrgUnitID)]
		return visible, nil
	case domain.AgentVisibilityRole:
		if len(targets) == 0 {
			return false, nil
		}
		role, _, err := c.activeAssumableRole(ctx, account)
		if err != nil {
			return false, err
		}
		if role == nil {
			return false, nil
		}
		_, visible := targets[role.ID]
		return visible, nil
	default:
		return false, nil
	}
}
