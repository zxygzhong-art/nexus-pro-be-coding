package service

import "nexus-pro-be/internal/utils"

type AuditService struct {
	*Service
	store auditStore
}

func (c *Service) Audit() AuditService {
	return AuditService{Service: c, store: c.store}
}

func (c AuditService) ListLogs(ctx RequestContext) ([]AuditLog, error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return nil, err
	}
	return c.store.ListAuditLogs(goContext(ctx), ctx.TenantID)
}

func (c AuditService) ListLogPage(ctx RequestContext, page PageRequest) (PageResponse[AuditLog], error) {
	if _, _, err := c.resolveAccount(ctx); err != nil {
		return PageResponse[AuditLog]{}, err
	}
	page = utils.NormalizePageRequest(page)
	items, total, err := c.store.ListAuditLogPage(goContext(ctx), ctx.TenantID, page)
	if err != nil {
		return PageResponse[AuditLog]{}, err
	}
	return utils.PageResponseFromStore(items, total, page), nil
}
