package service

import (
	"fmt"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

// reconcileExternalLeaveRecord converges one synchronized fact onto either an
// exact Nexus case or its own eHRMS case. Corrections, cancellations and
// tombstones first reverse any previously applied external reconciliation.
func (c AttendanceService) reconcileExternalLeaveRecord(ctx RequestContext, external ExternalLeaveRecord) error {
	previousCase, previousCaseFound, err := c.store.GetLeaveCaseByExternalRecord(goContext(ctx), ctx.TenantID, external.ID)
	if err != nil {
		return err
	}
	previousRequest, previousRequestFound, err := c.leaveRequestBoundToCase(ctx, previousCase, previousCaseFound)
	if err != nil {
		return err
	}

	matched, candidates, err := c.matchExternalLeaveRequest(ctx, external)
	if err != nil {
		return err
	}
	active := strings.EqualFold(external.Status, "active") && external.DeletedAt == nil
	if !active {
		matched = LeaveRequest{}
	}

	if previousRequestFound && (matched.ID == "" || matched.ID != previousRequest.ID) {
		if _, err := c.setExternalReconciliationTarget(ctx, previousRequest, previousCase.ID, external, false); err != nil {
			return err
		}
		previousRequest.ReconciliationStatus = "nexus_only"
		previousRequest.UpdatedAt = c.Now()
		if err := c.store.UpsertLeaveRequest(goContext(ctx), previousRequest); err != nil {
			return err
		}
		previousCase.Origin = "nexus"
		previousCase.Status = "active"
		previousCase.UpdatedAt = c.Now()
		if err := c.store.UpsertLeaveCase(goContext(ctx), previousCase); err != nil {
			return err
		}
	}

	if matched.ID != "" {
		leaveCase, err := c.ensureNexusLeaveCase(ctx, matched)
		if err != nil {
			return err
		}
		leaveCase.Origin = "both"
		leaveCase.Status = "active"
		leaveCase.UpdatedAt = c.Now()
		if err := c.store.UpsertLeaveCase(goContext(ctx), leaveCase); err != nil {
			return err
		}
		if err := c.store.UpsertLeaveCaseSource(goContext(ctx), LeaveCaseSource{
			TenantID: ctx.TenantID, LeaveCaseID: leaveCase.ID,
			ExternalLeaveRecordID: external.ID,
			MatchMethod:           "exact", MatchStatus: "confirmed", CreatedAt: c.Now(),
		}); err != nil {
			return err
		}
		if previousCaseFound && previousCase.ID != leaveCase.ID {
			if err := c.store.DeleteLeaveCaseIfUnreferenced(goContext(ctx), ctx.TenantID, previousCase.ID); err != nil {
				return err
			}
		}
		reconciled, err := c.setExternalReconciliationTarget(ctx, matched, leaveCase.ID, external, true)
		if err != nil {
			return err
		}
		matched.ReconciliationStatus = "matched"
		if !reconciled {
			matched.ReconciliationStatus = "pending_balance_confirmation"
		}
		matched.UpdatedAt = c.Now()
		return c.store.UpsertLeaveRequest(goContext(ctx), matched)
	}

	if active {
		if err := c.markExternalLeaveCandidates(ctx, candidates); err != nil {
			return err
		}
	}
	standalone, err := c.upsertStandaloneEHRMSLeaveCase(ctx, external)
	if err != nil {
		return err
	}
	if previousCaseFound && previousCase.ID != standalone.ID {
		return c.store.DeleteLeaveCaseIfUnreferenced(goContext(ctx), ctx.TenantID, previousCase.ID)
	}
	return nil
}

func (c AttendanceService) matchExternalLeaveRequest(ctx RequestContext, external ExternalLeaveRecord) (LeaveRequest, []LeaveRequest, error) {
	if !strings.EqualFold(external.Status, "active") || external.DeletedAt != nil {
		return LeaveRequest{}, nil, nil
	}
	requests, err := c.store.ListLeaveRequestsByQuery(goContext(ctx), ctx.TenantID, LeaveRequestQuery{
		EmployeeIDs: []string{external.EmployeeID},
		FromDate:    external.StartAt.Format(time.DateOnly), ToDate: external.EndAt.Format(time.DateOnly),
	})
	if err != nil {
		return LeaveRequest{}, nil, err
	}
	matches := make([]LeaveRequest, 0, 1)
	candidates := make([]LeaveRequest, 0, len(requests))
	for _, request := range requests {
		if normalizeLeaveRequestStatus(request.Status) != "approved" || request.LeaveTypeID != external.LeaveTypeID {
			continue
		}
		candidates = append(candidates, request)
		if request.StartAt.Equal(external.StartAt) && request.EndAt.Equal(external.EndAt) && request.RequestedMinutes == external.NetMinutes {
			matches = append(matches, request)
		}
	}
	if len(matches) == 1 {
		return matches[0], candidates, nil
	}
	return LeaveRequest{}, candidates, nil
}

func (c AttendanceService) markExternalLeaveCandidates(ctx RequestContext, candidates []LeaveRequest) error {
	status := ""
	switch len(candidates) {
	case 0:
		return nil
	case 1:
		status = "mismatch"
	default:
		status = "ambiguous"
	}
	for _, request := range candidates {
		request.ReconciliationStatus = status
		request.UpdatedAt = c.Now()
		if err := c.store.UpsertLeaveRequest(goContext(ctx), request); err != nil {
			return err
		}
	}
	return nil
}

func (c AttendanceService) leaveRequestBoundToCase(ctx RequestContext, leaveCase LeaveCase, found bool) (LeaveRequest, bool, error) {
	if !found || leaveCase.ID == "" || leaveCase.Origin == "ehrms" {
		return LeaveRequest{}, false, nil
	}
	requests, err := c.store.ListLeaveRequestsByQuery(goContext(ctx), ctx.TenantID, LeaveRequestQuery{
		EmployeeIDs: []string{leaveCase.EmployeeID},
		FromDate:    leaveCase.StartAt.Format(time.DateOnly), ToDate: leaveCase.EndAt.Format(time.DateOnly),
	})
	if err != nil {
		return LeaveRequest{}, false, err
	}
	for _, request := range requests {
		item, ok, lookupErr := c.store.GetLeaveCaseByLeaveRequest(goContext(ctx), ctx.TenantID, request.ID)
		if lookupErr != nil {
			return LeaveRequest{}, false, lookupErr
		}
		if ok && item.ID == leaveCase.ID {
			return request, true, nil
		}
	}
	return LeaveRequest{}, false, nil
}

// setExternalReconciliationTarget adjusts the applied amount rather than
// appending an absolute duplicate. A changed or deleted upstream record targets
// zero; a confirmed exact match targets each allocation's reserved minutes.
func (c AttendanceService) setExternalReconciliationTarget(ctx RequestContext, request LeaveRequest, caseID string, external ExternalLeaveRecord, matched bool) (bool, error) {
	allocations, err := c.store.ListLeaveRequestAllocationsByRequestCycle(
		goContext(ctx), ctx.TenantID, request.ID, leaveRequestBalanceCycle(request),
	)
	if err != nil {
		return false, err
	}
	entries, err := c.store.ListLeaveBalanceEntries(goContext(ctx), ctx.TenantID)
	if err != nil {
		return false, err
	}
	currentByAllocation := map[int64]int{}
	entryCountByAllocation := map[int64]int{}
	for _, entry := range entries {
		if strings.TrimSpace(stringFromAny(entry.Metadata["external_leave_record_id"])) != external.ID {
			continue
		}
		currentByAllocation[entry.AllocationID] += entry.AmountMinutes
		entryCountByAllocation[entry.AllocationID]++
	}
	version := externalLeaveLedgerVersion(external)
	fullyReconciled := true
	for _, allocation := range allocations {
		target := 0
		if matched {
			confirmedTarget, confirmed, confirmErr := c.externalAllocationReconciliationTarget(ctx, request, allocation, external, entries)
			if confirmErr != nil {
				return false, confirmErr
			}
			if confirmedTarget {
				target = allocation.ReservedMinutes
			}
			fullyReconciled = fullyReconciled && confirmed
		}
		delta := target - currentByAllocation[allocation.ID]
		if delta == 0 {
			continue
		}
		entryType := leaveBalanceEntryExternalReconcile
		if delta < 0 {
			entryType = leaveBalanceEntryExternalReversal
		}
		now := c.Now()
		_, err := c.store.AppendLeaveBalanceEntry(goContext(ctx), domain.LeaveBalanceEntry{
			ID: utils.NewID("lbe"), TenantID: ctx.TenantID,
			EmployeeID: request.EmployeeID, LeaveTypeID: request.LeaveTypeID,
			BalanceID: allocation.LeaveBalanceID, LeaveRequestID: request.ID,
			LeaveCaseID: caseID, AllocationID: allocation.ID,
			EntryType: entryType, AmountMinutes: delta,
			IdempotencyKey: fmt.Sprintf(
				"external-leave:%s:version:%s:allocation:%d:generation:%d:target:%d",
				external.ID, version, allocation.ID, entryCountByAllocation[allocation.ID], target,
			),
			Metadata: map[string]any{
				"source": "ehrms_reconciliation", "external_leave_record_id": external.ID,
				"external_payload_hash": external.PayloadHash, "target_minutes": target,
			},
			OccurredAt: now, CreatedAt: now,
		})
		if err != nil {
			return false, err
		}
	}
	return fullyReconciled, nil
}

// externalAllocationReconciliationTarget confirms that the authoritative
// snapshot has absorbed the external fact before neutralizing a local consume.
// Local-anchor minutes remain Nexus-owned even when the logical case is also
// observed in eHRMS.
func (c AttendanceService) externalAllocationReconciliationTarget(ctx RequestContext, request LeaveRequest, allocation LeaveRequestAllocation, external ExternalLeaveRecord, entries []domain.LeaveBalanceEntry) (bool, bool, error) {
	balance, found, err := c.store.GetLeaveBalance(goContext(ctx), ctx.TenantID, allocation.LeaveBalanceID)
	if err != nil {
		return false, false, err
	}
	if !found {
		return false, false, Conflict("reconciled leave allocation balance was not found")
	}
	if strings.EqualFold(balance.Source, "local_anchor") {
		return false, true, nil
	}
	if external.FirstSeenAt.Before(request.CreatedAt) {
		return true, true, nil
	}
	for _, entry := range entries {
		if entry.AllocationID != allocation.ID || entry.EntryType != leaveBalanceEntryReserve {
			continue
		}
		baselineRemaining := workflowIntFromAny(entry.Metadata["snapshot_remaining_minutes"])
		baselineUsed := workflowIntFromAny(entry.Metadata["snapshot_used_minutes"])
		absorbedByRemaining := balance.RemainingMinutes <= baselineRemaining-allocation.ReservedMinutes
		absorbedByUsed := balance.UsedMinutes >= baselineUsed+allocation.ReservedMinutes
		if absorbedByRemaining || absorbedByUsed {
			return true, true, nil
		}
		return false, false, nil
	}
	return false, false, nil
}

func externalLeaveLedgerVersion(external ExternalLeaveRecord) string {
	deletedAt := "active"
	if external.DeletedAt != nil {
		deletedAt = external.DeletedAt.UTC().Format(time.RFC3339Nano)
	}
	return ehrmsStableID("elv", external.PayloadHash, strings.ToLower(external.Status), external.LastSeenAt.UTC().Format(time.RFC3339Nano), deletedAt)
}

func (c AttendanceService) upsertStandaloneEHRMSLeaveCase(ctx RequestContext, external ExternalLeaveRecord) (LeaveCase, error) {
	now := c.Now()
	id := ehrmsStableID("lc", ctx.TenantID, "ehrms", external.ID)
	item := LeaveCase{
		ID: id, TenantID: ctx.TenantID, EmployeeID: external.EmployeeID, LeaveTypeID: external.LeaveTypeID,
		StartAt: external.StartAt, EndAt: external.EndAt, NetMinutes: external.NetMinutes,
		Status: external.Status, Origin: "ehrms", CreatedAt: now, UpdatedAt: now,
	}
	if external.DeletedAt != nil {
		item.Status = "cancelled"
	}
	if existing, ok, err := c.store.GetLeaveCaseByExternalRecord(goContext(ctx), ctx.TenantID, external.ID); err != nil {
		return LeaveCase{}, err
	} else if ok && existing.ID == id {
		item.CreatedAt = existing.CreatedAt
	}
	if err := c.store.UpsertLeaveCase(goContext(ctx), item); err != nil {
		return LeaveCase{}, err
	}
	if err := c.store.UpsertLeaveCaseSource(goContext(ctx), LeaveCaseSource{
		TenantID: ctx.TenantID, LeaveCaseID: item.ID, ExternalLeaveRecordID: external.ID,
		MatchMethod: "direct", MatchStatus: "confirmed", CreatedAt: now,
	}); err != nil {
		return LeaveCase{}, err
	}
	return item, nil
}
