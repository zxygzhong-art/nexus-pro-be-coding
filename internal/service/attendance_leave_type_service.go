package service

import (
	"strings"

	"nexus-pro-api/internal/domain"
)

// ListLeaveTypes returns the tenant leave_types catalog.
func (c AttendanceService) ListLeaveTypes(ctx RequestContext) (LeaveTypeCatalog, error) {
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceLeave, ActionRead, ""); err != nil {
		return LeaveTypeCatalog{}, err
	}
	items, err := c.loadLeaveTypes(ctx)
	if err != nil {
		return LeaveTypeCatalog{}, err
	}
	return leaveTypeCatalog(items), nil
}

// SetLeaveTypeEnabled updates leave_types.status for an existing catalog row.
func (c AttendanceService) SetLeaveTypeEnabled(ctx RequestContext, id string, input SetLeaveTypeEnabledInput) (LeaveType, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceLeave, ActionUpdate, id); err != nil {
		return LeaveType{}, err
	}
	items, err := c.loadLeaveTypes(ctx)
	if err != nil {
		return LeaveType{}, err
	}
	item, found := findLeaveTypeByID(items, id)
	if !found {
		return LeaveType{}, domain.NotFound("leave type", id)
	}
	now := c.Now()
	if err := c.withTransaction(ctx, func(tx AttendanceService) error {
		if err := tx.store.UpsertLeaveTypeEnabled(goContext(ctx), ctx.TenantID, id, input.Enabled, ctx.AccountID, now); err != nil {
			return err
		}
		return tx.audit(ctx, "attendance.leave_type.set_enabled", string(ResourceLeave), id, string(SeverityMedium), map[string]any{
			"id": id, "code": item.Code, "kind": item.Kind, "enabled": input.Enabled,
		})
	}); err != nil {
		return LeaveType{}, err
	}
	item.Enabled = input.Enabled
	return item, nil
}

func findLeaveTypeByID(items []LeaveType, id string) (LeaveType, bool) {
	wanted := strings.ToLower(strings.TrimSpace(id))
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item.ID)) == wanted {
			return item, true
		}
	}
	return LeaveType{}, false
}

// loadLeaveTypes is the internal source of truth for forms, validation, and legends.
func (c AttendanceService) loadLeaveTypes(ctx RequestContext) ([]LeaveType, error) {
	return c.store.ListLeaveTypes(goContext(ctx), ctx.TenantID)
}

func leaveTypeCatalog(items []LeaveType) LeaveTypeCatalog {
	catalog := LeaveTypeCatalog{Items: items, Total: len(items)}
	for _, item := range items {
		if item.Enabled {
			catalog.Enabled++
		}
	}
	return catalog
}

func findLeaveType(items []LeaveType, code string, enabledOnly bool) (LeaveType, bool) {
	wanted := normalizeLeaveTypeCode(code)
	for _, item := range items {
		if (item.Kind == "" || item.Kind == "item") && normalizeLeaveTypeCode(item.Code) == wanted && (!enabledOnly || item.Enabled) {
			return item, true
		}
	}
	return LeaveType{}, false
}

func leaveTypeRule(item LeaveType) domain.LeaveRuleSnapshot {
	grantMode := domain.LeaveGrantModeUnlimited
	if item.RequiresBalance {
		grantMode = domain.LeaveGrantModeAnnualGrant
	}
	return domain.LeaveRuleSnapshot{
		LeaveTypeID:     item.ID,
		Code:            item.Code,
		Name:            firstNonEmptyString(strings.TrimSpace(item.NameZH), strings.TrimSpace(item.NameEN), item.Code),
		GrantMode:       grantMode,
		RequiresBalance: item.RequiresBalance,
	}
}
