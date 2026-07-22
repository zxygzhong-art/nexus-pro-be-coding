package service

import (
	"fmt"
	"math"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

const (
	leaveBalanceEntryReserve           = "reserve"
	leaveBalanceEntryRelease           = "release"
	leaveBalanceEntryLocalConsume      = "local_consume"
	leaveBalanceEntryLocalRefund       = "local_refund"
	leaveBalanceEntryExternalReconcile = "external_reconcile"
	leaveBalanceEntryExternalReversal  = "external_reversal"
	leaveBalanceEntryOvertimeCredit    = "overtime_credit"
)

func leaveMinutes(hours float64) int {
	return int(math.Round(hours * 60))
}

func leaveHours(minutes int) float64 {
	return math.Round(float64(minutes)/60*100) / 100
}

// listEffectiveLeaveBalances projects immutable upstream snapshots and local
// anchor rows through the append-only integer-minute overlay.
func (c AttendanceService) listEffectiveLeaveBalances(ctx RequestContext) ([]LeaveBalance, error) {
	items, err := c.store.ListLeaveBalances(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	entries, err := c.store.ListLeaveBalanceEntries(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	return applyLeaveBalanceOverlay(items, entries), nil
}

func applyLeaveBalanceOverlay(items []LeaveBalance, entries []domain.LeaveBalanceEntry) []LeaveBalance {
	totalByBalance := map[string]int{}
	pendingByBalance := map[string]int{}
	localByBalance := map[string]int{}
	for _, entry := range entries {
		totalByBalance[entry.BalanceID] += entry.AmountMinutes
		switch entry.EntryType {
		case leaveBalanceEntryReserve, leaveBalanceEntryRelease:
			pendingByBalance[entry.BalanceID] += entry.AmountMinutes
		case leaveBalanceEntryLocalConsume, leaveBalanceEntryLocalRefund,
			leaveBalanceEntryExternalReconcile, leaveBalanceEntryExternalReversal:
			localByBalance[entry.BalanceID] += entry.AmountMinutes
		}
	}
	out := make([]LeaveBalance, len(items))
	for index, item := range items {
		item.SnapshotRemainingMinutes = item.RemainingMinutes
		item.PendingMinutes = max(0, -pendingByBalance[item.ID])
		item.LocalUsedMinutes = max(0, -localByBalance[item.ID])
		item.RemainingMinutes += totalByBalance[item.ID]
		out[index] = item
	}
	return out
}

// leaveBalanceAllocationsForOverlay locks every eligible bucket in a stable
// expiry order, then splits one request across as many buckets as necessary.
// No snapshot row is updated; callers persist only allocations and entries.
func (c AttendanceService) leaveBalanceAllocationsForOverlay(ctx RequestContext, employeeID, leaveTypeID string, requestedMinutes int, asOf time.Time) ([]LeaveRequestAllocation, int, string, error) {
	items, err := c.store.ListLeaveBalancesForOverlay(goContext(ctx), ctx.TenantID, employeeID, leaveTypeID, asOf)
	if err != nil {
		return nil, 0, "", err
	}
	if len(items) == 0 {
		return nil, 0, leaveEvaluationBalanceMissing, nil
	}
	entries, err := c.store.ListLeaveBalanceEntries(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, 0, "", err
	}
	effective := applyLeaveBalanceOverlay(items, entries)
	availableMinutes := 0
	for _, item := range effective {
		availableMinutes += max(0, item.RemainingMinutes)
	}
	if availableMinutes < requestedMinutes {
		return nil, availableMinutes, leaveEvaluationBalanceInsufficient, nil
	}

	remaining := requestedMinutes
	allocations := make([]LeaveRequestAllocation, 0, len(effective))
	for _, item := range effective {
		if remaining == 0 {
			break
		}
		reserved := min(max(0, item.RemainingMinutes), remaining)
		if reserved == 0 {
			continue
		}
		allocations = append(allocations, LeaveRequestAllocation{
			TenantID: ctx.TenantID, LeaveBalanceID: item.ID,
			EmployeeID: employeeID, LeaveTypeID: leaveTypeID, ReservedMinutes: reserved,
		})
		remaining -= reserved
	}
	return allocations, availableMinutes, "", nil
}

func (c AttendanceService) appendLeaveBalanceEntry(ctx RequestContext, request LeaveRequest, allocation LeaveRequestAllocation, caseID, entryType string, amountMinutes, cycle int) error {
	if strings.TrimSpace(allocation.LeaveBalanceID) == "" || amountMinutes == 0 {
		return nil
	}
	now := c.Now()
	idempotencyKey := fmt.Sprintf("leave-request:%s:cycle:%d:balance:%s:%s", request.ID, cycle, allocation.LeaveBalanceID, entryType)
	metadata := map[string]any{"source": "nexus_workflow", "cycle": cycle}
	if entryType == leaveBalanceEntryReserve {
		balance, found, err := c.store.GetLeaveBalance(goContext(ctx), ctx.TenantID, allocation.LeaveBalanceID)
		if err != nil {
			return err
		}
		if !found {
			return Conflict("allocated leave balance was not found")
		}
		metadata["snapshot_remaining_minutes"] = balance.RemainingMinutes
		metadata["snapshot_used_minutes"] = balance.UsedMinutes
		metadata["balance_source"] = balance.Source
	}
	_, err := c.store.AppendLeaveBalanceEntry(goContext(ctx), domain.LeaveBalanceEntry{
		ID: utils.NewID("lbe"), TenantID: ctx.TenantID,
		EmployeeID: request.EmployeeID, LeaveTypeID: request.LeaveTypeID,
		BalanceID: allocation.LeaveBalanceID, LeaveRequestID: request.ID,
		LeaveCaseID: caseID, AllocationID: allocation.ID,
		EntryType: entryType, AmountMinutes: amountMinutes, IdempotencyKey: idempotencyKey,
		Metadata:   metadata,
		OccurredAt: now, CreatedAt: now,
	})
	return err
}

func leaveRequestBalanceCycle(request LeaveRequest) int {
	if request.EvaluationSnapshot == nil {
		return 1
	}
	switch value := request.EvaluationSnapshot["balance_cycle"].(type) {
	case int:
		if value > 0 {
			return value
		}
	case int64:
		if value > 0 {
			return int(value)
		}
	case float64:
		if value > 0 {
			return int(value)
		}
	}
	return 1
}

func nextLeaveRequestBalanceCycle(existing LeaveRequest, resubmitting bool) int {
	if !resubmitting {
		return 1
	}
	return leaveRequestBalanceCycle(existing) + 1
}

func (c AttendanceService) ensureNexusLeaveCase(ctx RequestContext, request LeaveRequest) (LeaveCase, error) {
	if item, ok, err := c.store.GetLeaveCaseByLeaveRequest(goContext(ctx), ctx.TenantID, request.ID); err != nil || ok {
		return item, err
	}
	now := c.Now()
	item := LeaveCase{
		ID: ehrmsStableID("lc", ctx.TenantID, "nexus", request.ID), TenantID: ctx.TenantID,
		EmployeeID: request.EmployeeID, LeaveTypeID: request.LeaveTypeID,
		StartAt: request.StartAt, EndAt: request.EndAt, NetMinutes: request.RequestedMinutes,
		Status: "active", Origin: "nexus", CreatedAt: now, UpdatedAt: now,
	}
	if err := c.store.UpsertLeaveCase(goContext(ctx), item); err != nil {
		return LeaveCase{}, err
	}
	if err := c.store.UpsertLeaveCaseSource(goContext(ctx), LeaveCaseSource{
		TenantID: ctx.TenantID, LeaveCaseID: item.ID, LeaveRequestID: request.ID,
		MatchMethod: "direct", MatchStatus: "confirmed", CreatedAt: now,
	}); err != nil {
		return LeaveCase{}, err
	}
	return item, nil
}
