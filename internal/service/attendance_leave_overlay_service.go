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

// leaveBalanceForOverlay resolves the single annual balance a request may use.
func (c AttendanceService) leaveBalanceForOverlay(ctx RequestContext, employeeID, leaveTypeID string, requestedMinutes int, asOf time.Time) (LeaveBalance, int, string, error) {
	item, found, err := c.store.GetLeaveBalanceForOverlay(goContext(ctx), ctx.TenantID, employeeID, leaveTypeID, asOf)
	if err != nil {
		return LeaveBalance{}, 0, "", err
	}
	if !found {
		return LeaveBalance{}, 0, leaveEvaluationBalanceMissing, nil
	}
	entries, err := c.store.ListLeaveBalanceEntries(goContext(ctx), ctx.TenantID)
	if err != nil {
		return LeaveBalance{}, 0, "", err
	}
	effective := applyLeaveBalanceOverlay([]LeaveBalance{item}, entries)[0]
	availableMinutes := max(0, effective.RemainingMinutes)
	if availableMinutes < requestedMinutes {
		return item, availableMinutes, leaveEvaluationBalanceInsufficient, nil
	}
	return item, availableMinutes, "", nil
}

func (c AttendanceService) appendLeaveBalanceEntry(ctx RequestContext, request LeaveRequest, record LeaveRecord, entryType string, amountMinutes, cycle int) error {
	if strings.TrimSpace(record.BalanceID) == "" || amountMinutes == 0 {
		return nil
	}
	now := c.Now()
	idempotencyKey := fmt.Sprintf("leave-request:%s:cycle:%d:balance:%s:%s", request.ID, cycle, record.BalanceID, entryType)
	_, err := c.store.AppendLeaveBalanceEntry(goContext(ctx), domain.LeaveBalanceEntry{
		ID: utils.NewID("lbe"), TenantID: ctx.TenantID,
		EmployeeID: request.EmployeeID, LeaveTypeID: request.LeaveTypeID,
		BalanceID: record.BalanceID, LeaveRecordID: record.ID, EntitlementYear: record.EntitlementYear,
		EntryType: entryType, AmountMinutes: amountMinutes, IdempotencyKey: idempotencyKey,
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

func (c AttendanceService) ensureNexusLeaveRecord(ctx RequestContext, request LeaveRequest, balance LeaveBalance, status string) (LeaveRecord, error) {
	if item, ok, err := c.store.GetLeaveRecord(goContext(ctx), ctx.TenantID, request.ID); err != nil {
		return LeaveRecord{}, err
	} else if ok {
		item.EmployeeID = request.EmployeeID
		item.LeaveTypeID = request.LeaveTypeID
		item.BalanceID = balance.ID
		item.EntitlementYear = balance.EntitlementYear
		item.Status = status
		item.StartAt = request.StartAt
		item.EndAt = request.EndAt
		item.NetMinutes = request.RequestedMinutes
		item.Remark = request.Reason
		item.UpdatedAt = c.Now()
		return item, c.store.UpsertLeaveRecord(goContext(ctx), item)
	}
	now := c.Now()
	item := LeaveRecord{
		ID: request.ID, TenantID: ctx.TenantID, EmployeeID: request.EmployeeID,
		LeaveTypeID: request.LeaveTypeID, BalanceID: balance.ID,
		EntitlementYear: balance.EntitlementYear, Source: "nexus", EventDate: request.CreatedAt,
		StartAt: request.StartAt, EndAt: request.EndAt, NetMinutes: request.RequestedMinutes,
		Remark: request.Reason, Status: status, ReconciliationStatus: "not_required", UpdatedAt: now,
	}
	return item, c.store.UpsertLeaveRecord(goContext(ctx), item)
}
