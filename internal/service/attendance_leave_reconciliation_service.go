package service

import (
	"strings"
	"time"
)

func (c AttendanceService) reconcileExternalLeaveRecord(ctx RequestContext, external ExternalLeaveRecord) error {
	existingExternalCase, externalCaseFound, err := c.store.GetLeaveCaseBySource(goContext(ctx), ctx.TenantID, "ehrms_record", external.ID)
	if err != nil {
		return err
	}
	if externalCaseFound && existingExternalCase.Origin == "both" {
		return nil
	}
	requests, err := c.store.ListLeaveRequestsByQuery(goContext(ctx), ctx.TenantID, LeaveRequestQuery{
		EmployeeIDs: []string{external.EmployeeID}, FromDate: external.StartAt.Format(time.DateOnly), ToDate: external.EndAt.Format(time.DateOnly),
	})
	if err != nil {
		return err
	}
	matches := make([]LeaveRequest, 0, 1)
	candidates := make([]LeaveRequest, 0, len(requests))
	for _, request := range requests {
		if normalizeLeaveRequestStatus(request.Status) != "approved" || request.LeaveTypeID != external.LeaveTypeID {
			continue
		}
		candidates = append(candidates, request)
		if !request.StartAt.Equal(external.StartAt) || !request.EndAt.Equal(external.EndAt) || leaveMinutes(request.Hours) != external.NetMinutes {
			continue
		}
		matches = append(matches, request)
	}
	if len(matches) != 1 {
		if len(matches) > 1 {
			for _, request := range matches {
				request.ReconciliationStatus = "ambiguous"
				request.UpdatedAt = c.Now()
				if err := c.store.UpsertLeaveRequest(goContext(ctx), request); err != nil {
					return err
				}
			}
		} else if len(candidates) == 1 {
			request := candidates[0]
			request.ReconciliationStatus = "mismatch"
			request.UpdatedAt = c.Now()
			if err := c.store.UpsertLeaveRequest(goContext(ctx), request); err != nil {
				return err
			}
		} else if len(candidates) > 1 {
			for _, request := range candidates {
				request.ReconciliationStatus = "ambiguous"
				request.UpdatedAt = c.Now()
				if err := c.store.UpsertLeaveRequest(goContext(ctx), request); err != nil {
					return err
				}
			}
		}
		_, err := c.ensureEHRMSLeaveCase(ctx, external)
		return err
	}

	request := matches[0]
	leaveCase, err := c.ensureNexusLeaveCase(ctx, request)
	if err != nil {
		return err
	}
	leaveCase.Origin = "both"
	leaveCase.UpdatedAt = c.Now()
	if err := c.store.UpsertLeaveCase(goContext(ctx), leaveCase); err != nil {
		return err
	}
	if err := c.store.UpsertLeaveCaseSource(goContext(ctx), LeaveCaseSource{
		TenantID: ctx.TenantID, LeaveCaseID: leaveCase.ID, SourceType: "ehrms_record", SourceID: external.ID,
		MatchMethod: "exact", MatchStatus: "confirmed", CreatedAt: c.Now(),
	}); err != nil {
		return err
	}
	if externalCaseFound && existingExternalCase.ID != leaveCase.ID {
		if err := c.store.DeleteLeaveCaseIfUnreferenced(goContext(ctx), ctx.TenantID, existingExternalCase.ID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(request.LeaveBalanceID) != "" {
		if err := c.appendLeaveBalanceEntry(ctx, request, request.LeaveBalanceID, leaveCase.ID, leaveBalanceEntryExternalReconcile, external.NetMinutes, leaveRequestBalanceCycle(request)); err != nil {
			return err
		}
	}
	request.ReconciliationStatus = "matched"
	request.UpdatedAt = c.Now()
	return c.store.UpsertLeaveRequest(goContext(ctx), request)
}

func (c AttendanceService) ensureEHRMSLeaveCase(ctx RequestContext, external ExternalLeaveRecord) (LeaveCase, error) {
	if item, ok, err := c.store.GetLeaveCaseBySource(goContext(ctx), ctx.TenantID, "ehrms_record", external.ID); err != nil || ok {
		return item, err
	}
	now := c.Now()
	item := LeaveCase{
		ID: ehrmsStableID("lc", ctx.TenantID, "ehrms", external.ID), TenantID: ctx.TenantID,
		EmployeeID: external.EmployeeID, LeaveTypeID: external.LeaveTypeID,
		StartAt: external.StartAt, EndAt: external.EndAt, NetMinutes: external.NetMinutes,
		Status: external.Status, Origin: "ehrms", CreatedAt: now, UpdatedAt: now,
	}
	if err := c.store.UpsertLeaveCase(goContext(ctx), item); err != nil {
		return LeaveCase{}, err
	}
	if err := c.store.UpsertLeaveCaseSource(goContext(ctx), LeaveCaseSource{
		TenantID: ctx.TenantID, LeaveCaseID: item.ID, SourceType: "ehrms_record", SourceID: external.ID,
		MatchMethod: "direct", MatchStatus: "confirmed", CreatedAt: now,
	}); err != nil {
		return LeaveCase{}, err
	}
	return item, nil
}
