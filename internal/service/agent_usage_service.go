package service

import (
	"strings"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

const defaultAgentAccountUsageSort = "usage_desc"

var supportedAgentAccountUsageSorts = map[string]struct{}{
	defaultAgentAccountUsageSort: {},
	"session_count_asc":          {},
	"session_count_desc":         {},
	"message_count_asc":          {},
	"message_count_desc":         {},
	"total_tokens_asc":           {},
	"total_tokens_desc":          {},
	"cached_tokens_asc":          {},
	"cached_tokens_desc":         {},
	"actual_tokens_asc":          {},
	"actual_tokens_desc":         {},
	"last_active_at_asc":         {},
	"last_active_at_desc":        {},
}

// requireAccountUsageRead enforces tenant-wide access for usage management.
func (c AgentService) requireAccountUsageRead(ctx RequestContext) error {
	_, decision, err := c.requireAgentAuthz(ctx, ResourceDefinition, ActionRead, "")
	if err != nil {
		return err
	}
	if decision.Scope != "" && decision.Scope != ScopeAll && decision.Scope != ScopeTenant && decision.Scope != ScopeSystem {
		return domain.Forbidden("tenant-wide Agent usage requires all-tenant access")
	}
	return nil
}

// ListAccountUsage returns a filtered server-side page plus tenant-wide totals.
func (c AgentService) ListAccountUsage(ctx RequestContext, query domain.AgentAccountUsageQuery, page PageRequest) (domain.AgentUsageResponse, error) {
	if err := c.requireAccountUsageRead(ctx); err != nil {
		return domain.AgentUsageResponse{}, err
	}
	query.Query = strings.TrimSpace(query.Query)
	if len(query.Query) > 200 {
		return domain.AgentUsageResponse{}, BadRequest("query must not exceed 200 characters")
	}
	query.Status = strings.TrimSpace(query.Status)
	if query.Status != "" && query.Status != "active" && query.Status != "disabled" && query.Status != "pending_invite" {
		return domain.AgentUsageResponse{}, BadRequest("invalid account status")
	}
	sortOrder := strings.TrimSpace(page.Sort)
	page = utils.NormalizePageRequest(page)
	if sortOrder == "" {
		page.Sort = defaultAgentAccountUsageSort
	} else {
		page.Sort = sortOrder
	}
	if _, ok := supportedAgentAccountUsageSorts[page.Sort]; !ok {
		return domain.AgentUsageResponse{}, BadRequest("invalid usage sort")
	}

	items, total, err := c.store.ListAgentUsageByAccount(goContext(ctx), ctx.TenantID, query, page)
	if err != nil {
		return domain.AgentUsageResponse{}, err
	}
	summary, err := c.store.GetAgentUsageSummary(goContext(ctx), ctx.TenantID)
	if err != nil {
		return domain.AgentUsageResponse{}, err
	}
	if items == nil {
		items = []domain.AgentAccountUsage{}
	}
	response := domain.AgentUsageResponse{
		Items:    items,
		Total:    total,
		Page:     page.Page,
		PageSize: page.PageSize,
		Summary:  summary,
	}
	return response, nil
}

// ListAccountSessionUsage returns one account's session usage with server-side pagination.
func (c AgentService) ListAccountSessionUsage(ctx RequestContext, accountID string, page PageRequest) (domain.AgentSessionUsagePage, error) {
	if err := c.requireAccountUsageRead(ctx); err != nil {
		return domain.AgentSessionUsagePage{}, err
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return domain.AgentSessionUsagePage{}, BadRequest("account id is required")
	}

	account, found, err := c.store.GetAgentUsageByAccount(goContext(ctx), ctx.TenantID, accountID)
	if err != nil {
		return domain.AgentSessionUsagePage{}, err
	}
	if !found {
		return domain.AgentSessionUsagePage{}, NotFound("account", accountID)
	}

	page = utils.NormalizePageRequest(page)
	items, total, err := c.store.ListAgentUsageBySession(goContext(ctx), ctx.TenantID, accountID, page)
	if err != nil {
		return domain.AgentSessionUsagePage{}, err
	}
	if items == nil {
		items = []domain.AgentSessionUsage{}
	}
	return domain.AgentSessionUsagePage{
		Account:  account,
		Items:    items,
		Total:    total,
		Page:     page.Page,
		PageSize: page.PageSize,
	}, nil
}
