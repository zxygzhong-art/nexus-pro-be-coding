package service

import "nexus-pro-be/internal/utils"

// AuditService 定義稽覈服務的資料結構。
type AuditService struct {
	*Service
	store auditStore
}

// Audit 處理稽覈的服務流程。
func (c *Service) Audit() AuditService {
	return AuditService{Service: c, store: c.store}
}

// ListLogs 列出 logs 的服務流程。
func (c AuditService) ListLogs(ctx RequestContext) ([]AuditLog, error) {
	if _, _, err := c.requireAuditAuthz(ctx); err != nil {
		return nil, err
	}
	items, err := c.store.ListAuditLogs(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	return sanitizeAuditLogs(items), nil
}

// ListLogPage 列出 log 分頁的服務流程。
func (c AuditService) ListLogPage(ctx RequestContext, page PageRequest) (PageResponse[AuditLog], error) {
	if _, _, err := c.requireAuditAuthz(ctx); err != nil {
		return PageResponse[AuditLog]{}, err
	}
	page = utils.NormalizePageRequest(page)
	items, total, err := c.store.ListAuditLogPage(goContext(ctx), ctx.TenantID, page)
	if err != nil {
		return PageResponse[AuditLog]{}, err
	}
	items = sanitizeAuditLogs(items)
	return utils.PageResponseFromStore(items, total, page), nil
}

// RecordSecurityEvent 寫入不經授權檢查的安全邊界稽覈事件。
func (c AuditService) RecordSecurityEvent(ctx RequestContext, action, resource, target string, details map[string]any) error {
	return c.audit(ctx, action, resource, target, string(SeverityCritical), details)
}

// requireAuditAuthz 處理 require 稽覈授權的服務流程。
func (c AuditService) requireAuditAuthz(ctx RequestContext) (Account, CheckResult, error) {
	account, decision, _, err := c.Authorize(ctx, CheckRequest{Resource: "audit.log", Action: ActionRead}, AuditTarget{})
	return account, decision, err
}
