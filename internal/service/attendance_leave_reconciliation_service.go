package service

import (
	"fmt"
	"strings"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

// reconcileEHRMSLeaveRecord matches one active eHRMS record to exactly one
// Nexus record. Only the eHRMS row stores matched_record_id.
func (c AttendanceService) reconcileEHRMSLeaveRecord(ctx RequestContext, external LeaveRecord) error {
	previousMatchedID := strings.TrimSpace(external.MatchedRecordID)
	matched, candidates, err := c.matchNexusLeaveRecord(ctx, external)
	if err != nil {
		return err
	}
	active := external.Status == "active" && external.DeletedAt == nil
	if !active {
		matched = LeaveRecord{}
	}

	if previousMatchedID != "" && previousMatchedID != matched.ID {
		if err := c.appendExternalReconciliation(ctx, external, previousMatchedID, -external.NetMinutes, "unmatch"); err != nil {
			return err
		}
		if request, ok, getErr := c.store.GetLeaveRequest(goContext(ctx), ctx.TenantID, previousMatchedID); getErr != nil {
			return getErr
		} else if ok {
			request.ReconciliationStatus = "nexus_only"
			request.UpdatedAt = c.Now()
			if err := c.store.UpsertLeaveRequest(goContext(ctx), request); err != nil {
				return err
			}
		}
	}

	external.MatchedRecordID = ""
	external.ReconciliationStatus = "unmatched"
	if matched.ID != "" {
		external.MatchedRecordID = matched.ID
		external.ReconciliationStatus = "matched"
		if err := c.appendExternalReconciliation(ctx, external, matched.ID, external.NetMinutes, "match"); err != nil {
			return err
		}
		if request, ok, getErr := c.store.GetLeaveRequest(goContext(ctx), ctx.TenantID, matched.ID); getErr != nil {
			return getErr
		} else if ok {
			request.ReconciliationStatus = "matched"
			request.UpdatedAt = c.Now()
			if err := c.store.UpsertLeaveRequest(goContext(ctx), request); err != nil {
				return err
			}
		}
	} else if active {
		switch len(candidates) {
		case 0:
			// No eligible Nexus record exists.
		case 1:
			external.ReconciliationStatus = "mismatch"
		default:
			external.ReconciliationStatus = "ambiguous"
		}
		for _, candidate := range candidates {
			if request, ok, getErr := c.store.GetLeaveRequest(goContext(ctx), ctx.TenantID, candidate.ID); getErr != nil {
				return getErr
			} else if ok {
				request.ReconciliationStatus = external.ReconciliationStatus
				request.UpdatedAt = c.Now()
				if err := c.store.UpsertLeaveRequest(goContext(ctx), request); err != nil {
					return err
				}
			}
		}
	}
	external.UpdatedAt = c.Now()
	return c.store.UpsertLeaveRecord(goContext(ctx), external)
}

func (c AttendanceService) matchNexusLeaveRecord(ctx RequestContext, external LeaveRecord) (LeaveRecord, []LeaveRecord, error) {
	items, err := c.store.ListLeaveRecords(goContext(ctx), ctx.TenantID)
	if err != nil {
		return LeaveRecord{}, nil, err
	}
	claimedNexusRecords := make(map[string]struct{})
	for _, item := range items {
		if item.Source == "ehrms" && item.ID != external.ID && strings.TrimSpace(item.MatchedRecordID) != "" {
			claimedNexusRecords[item.MatchedRecordID] = struct{}{}
		}
	}
	candidates := make([]LeaveRecord, 0)
	matches := make([]LeaveRecord, 0, 1)
	for _, item := range items {
		if item.Source != "nexus" || item.Status != "active" || item.DeletedAt != nil ||
			item.EmployeeID != external.EmployeeID || item.LeaveTypeID != external.LeaveTypeID ||
			item.EntitlementYear != external.EntitlementYear {
			continue
		}
		if _, claimed := claimedNexusRecords[item.ID]; claimed {
			continue
		}
		candidates = append(candidates, item)
		if item.StartAt.Equal(external.StartAt) && item.EndAt.Equal(external.EndAt) && item.NetMinutes == external.NetMinutes {
			matches = append(matches, item)
		}
	}
	if len(matches) == 1 {
		return matches[0], candidates, nil
	}
	return LeaveRecord{}, candidates, nil
}

func (c AttendanceService) appendExternalReconciliation(ctx RequestContext, external LeaveRecord, nexusRecordID string, delta int, action string) error {
	if delta == 0 || external.BalanceID == "" {
		return nil
	}
	entryType := leaveBalanceEntryExternalReconcile
	if delta < 0 {
		entryType = leaveBalanceEntryExternalReversal
	}
	now := c.Now()
	_, err := c.store.AppendLeaveBalanceEntry(goContext(ctx), domain.LeaveBalanceEntry{
		ID: utils.NewID("lbe"), TenantID: ctx.TenantID, BalanceID: external.BalanceID,
		LeaveRecordID: external.ID, EmployeeID: external.EmployeeID, LeaveTypeID: external.LeaveTypeID,
		EntitlementYear: external.EntitlementYear, EntryType: entryType, AmountMinutes: delta,
		IdempotencyKey: fmt.Sprintf("ehrms-record:%s:%s:%s:%d", external.ID, action, nexusRecordID, external.NetMinutes),
		OccurredAt:     now, CreatedAt: now,
	})
	return err
}
