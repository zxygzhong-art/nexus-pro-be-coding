package service

import (
	"strings"

	"nexus-pro-api/internal/domain"
)

// ListLeaveTypes returns the system catalog with tenant enablement overrides.
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

// SetLeaveTypeEnabled changes availability for new requests without modifying the definition.
func (c AttendanceService) SetLeaveTypeEnabled(ctx RequestContext, code string, input SetLeaveTypeEnabledInput) (LeaveType, error) {
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceLeave, ActionUpdate, code); err != nil {
		return LeaveType{}, err
	}
	code = normalizeLeaveTypeCode(code)
	items, err := c.loadLeaveTypes(ctx)
	if err != nil {
		return LeaveType{}, err
	}
	item, found := findLeaveType(items, code, false)
	if !found {
		return LeaveType{}, domain.NotFound("leave type", code)
	}
	now := c.Now()
	if err := c.withTransaction(ctx, func(tx AttendanceService) error {
		if err := tx.store.UpsertLeaveTypeEnabled(goContext(ctx), ctx.TenantID, code, input.Enabled, ctx.AccountID, now); err != nil {
			return err
		}
		return tx.audit(ctx, "attendance.leave_type.set_enabled", string(ResourceLeave), code, string(SeverityMedium), map[string]any{
			"code": code, "enabled": input.Enabled,
		})
	}); err != nil {
		return LeaveType{}, err
	}
	item.Enabled = input.Enabled
	return item, nil
}

// loadLeaveTypes is the internal source of truth for forms, validation, legends, and mappings.
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
		if normalizeLeaveTypeCode(item.Code) == wanted && (!enabledOnly || item.Enabled) {
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
		Unit:            item.Unit,
		GrantMode:       grantMode,
		RequiresBalance: item.RequiresBalance,
		PaidRatio:       item.PaidRatio,
	}
}
