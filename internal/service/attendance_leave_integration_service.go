package service

import (
	"sort"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

const (
	leaveMappingStatusMapped    = "mapped"
	leaveMappingStatusLocalOnly = "local_only"
	leaveSyncIssueUnmapped      = "unmapped_leave_type"
)

// ListLeaveTypeIntegrations returns the policy, usage, mapping, and unresolved sync state used by HR.
func (c AttendanceService) ListLeaveTypeIntegrations(ctx RequestContext) (LeaveTypeIntegrationResponse, error) {
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceLeave, ActionRead, ""); err != nil {
		return LeaveTypeIntegrationResponse{}, err
	}
	return c.loadLeaveTypeIntegrations(ctx)
}

// SaveLeaveTypeExternalMapping validates and persists one EHRMS-to-local leave alias.
func (c AttendanceService) SaveLeaveTypeExternalMapping(ctx RequestContext, input SaveLeaveTypeExternalMappingInput) (LeaveTypeExternalMapping, error) {
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceLeave, ActionUpdate, ""); err != nil {
		return LeaveTypeExternalMapping{}, err
	}
	source := strings.ToLower(strings.TrimSpace(input.Source))
	if source == "" {
		source = ehrmsAttendanceSource
	}
	externalCode := strings.TrimSpace(input.ExternalCode)
	leaveTypeID := strings.TrimSpace(input.LeaveTypeID)
	if externalCode == "" || leaveTypeID == "" {
		return LeaveTypeExternalMapping{}, BadRequest("external_code and leave_type_id are required")
	}
	if err := validateOptionalDateRange(input.EffectiveFrom, input.EffectiveTo); err != nil {
		return LeaveTypeExternalMapping{}, err
	}

	policy, err := c.loadAttendancePolicyResponse(ctx)
	if err != nil {
		return LeaveTypeExternalMapping{}, err
	}
	var leaveType AttendanceLeaveType
	found := false
	for _, item := range policy.LeaveTypes {
		if item.ID == leaveTypeID || strings.EqualFold(item.Code, strings.TrimPrefix(leaveTypeID, "lt_")) {
			leaveType = item
			leaveTypeID = item.ID
			found = true
			break
		}
	}
	if !found {
		return LeaveTypeExternalMapping{}, BadRequest("leave_type_id must reference a current policy leave type")
	}

	now := c.Now()
	mapping := LeaveTypeExternalMapping{
		ID: strings.TrimSpace(input.ID), TenantID: ctx.TenantID, Source: source, ExternalCode: externalCode,
		LeaveTypeID: leaveTypeID, LeaveTypeCode: leaveType.Code,
		EffectiveFrom: strings.TrimSpace(input.EffectiveFrom), EffectiveTo: strings.TrimSpace(input.EffectiveTo),
		CreatedAt: now, UpdatedAt: now,
	}
	if mapping.ID == "" {
		mapping.ID = utils.NewID("ltm")
	}
	if err := c.Service.withTenantTransaction(ctx, func(txService *Service) error {
		if err := txService.store.LockLeaveTypeExternalMappingKey(goContext(ctx), ctx.TenantID, source, externalCode); err != nil {
			return err
		}
		existing, err := txService.store.ListLeaveTypeExternalMappings(goContext(ctx), ctx.TenantID)
		if err != nil {
			return err
		}
		for _, item := range existing {
			if item.ID != mapping.ID && strings.EqualFold(item.Source, source) && strings.EqualFold(item.ExternalCode, externalCode) && dateRangesOverlap(item.EffectiveFrom, item.EffectiveTo, mapping.EffectiveFrom, mapping.EffectiveTo) {
				return domain.Conflict("external leave code already has an overlapping mapping")
			}
			if item.ID == mapping.ID {
				mapping.CreatedAt = item.CreatedAt
			}
		}
		if err := txService.store.UpsertLeaveTypeExternalMapping(goContext(ctx), mapping); err != nil {
			return err
		}
		if err := txService.store.ResolveLeaveTypeSyncIssues(goContext(ctx), ctx.TenantID, source, externalCode, now); err != nil {
			return err
		}
		return txService.audit(ctx, "attendance.leave_type_mapping.upsert", string(ResourceLeave), mapping.ID, string(SeverityMedium), map[string]any{
			"source": source, "external_code": externalCode, "leave_type_id": leaveTypeID,
		})
	}); err != nil {
		return LeaveTypeExternalMapping{}, err
	}
	return mapping, nil
}

// ExpireLeaveTypeExternalMapping ends one external alias while preserving its history.
func (c AttendanceService) ExpireLeaveTypeExternalMapping(ctx RequestContext, id string) (LeaveTypeExternalMapping, error) {
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceLeave, ActionUpdate, id); err != nil {
		return LeaveTypeExternalMapping{}, err
	}
	mappings, err := c.store.ListLeaveTypeExternalMappings(goContext(ctx), ctx.TenantID)
	if err != nil {
		return LeaveTypeExternalMapping{}, err
	}
	var mapping LeaveTypeExternalMapping
	for _, item := range mappings {
		if item.ID == id {
			mapping = item
			break
		}
	}
	if mapping.ID == "" {
		return LeaveTypeExternalMapping{}, domain.NotFound("leave type mapping", id)
	}
	now := c.Now()
	effectiveTo := now.Format(time.DateOnly)
	updated, err := c.store.ExpireLeaveTypeExternalMapping(goContext(ctx), ctx.TenantID, id, effectiveTo, now)
	if err != nil {
		return LeaveTypeExternalMapping{}, err
	}
	if !updated {
		return LeaveTypeExternalMapping{}, domain.NotFound("leave type mapping", id)
	}
	mapping.EffectiveTo = effectiveTo
	mapping.UpdatedAt = now
	if err := c.audit(ctx, "attendance.leave_type_mapping.expire", string(ResourceLeave), id, string(SeverityMedium), map[string]any{
		"source": mapping.Source, "external_code": mapping.ExternalCode, "effective_to": effectiveTo,
	}); err != nil {
		return LeaveTypeExternalMapping{}, err
	}
	return mapping, nil
}

// loadLeaveTypeIntegrations aggregates links and historical usage without repeating authorization checks.
func (c AttendanceService) loadLeaveTypeIntegrations(ctx RequestContext) (LeaveTypeIntegrationResponse, error) {
	policy, err := c.loadAttendancePolicyResponse(ctx)
	if err != nil {
		return LeaveTypeIntegrationResponse{}, err
	}
	mappings, err := c.store.ListLeaveTypeExternalMappings(goContext(ctx), ctx.TenantID)
	if err != nil {
		return LeaveTypeIntegrationResponse{}, err
	}
	issues, err := c.store.ListOpenLeaveTypeSyncIssues(goContext(ctx), ctx.TenantID)
	if err != nil {
		return LeaveTypeIntegrationResponse{}, err
	}
	balances, err := c.store.ListLeaveBalances(goContext(ctx), ctx.TenantID)
	if err != nil {
		return LeaveTypeIntegrationResponse{}, err
	}
	requests, err := c.store.ListLeaveRequests(goContext(ctx), ctx.TenantID)
	if err != nil {
		return LeaveTypeIntegrationResponse{}, err
	}

	today := c.Now().Format(time.DateOnly)
	response := LeaveTypeIntegrationResponse{UnmappedIssues: issues, Items: make([]LeaveTypeIntegration, 0, len(policy.LeaveTypes))}
	byID := make(map[string]int, len(policy.LeaveTypes))
	byCode := make(map[string]int, len(policy.LeaveTypes))
	for _, leaveType := range policy.LeaveTypes {
		item := LeaveTypeIntegration{
			LeaveTypeID: leaveType.ID, LeaveTypeCode: leaveType.Code, LeaveTypeName: leaveType.Name,
			Active: leaveType.Active, MappingStatus: leaveMappingStatusLocalOnly, Mappings: []LeaveTypeExternalMapping{},
		}
		response.Items = append(response.Items, item)
		byID[item.LeaveTypeID] = len(response.Items) - 1
		byCode[strings.ToLower(item.LeaveTypeCode)] = len(response.Items) - 1
	}
	for _, mapping := range mappings {
		index, ok := byID[mapping.LeaveTypeID]
		if !ok {
			index, ok = byCode[strings.ToLower(mapping.LeaveTypeCode)]
		}
		if !ok {
			continue
		}
		item := &response.Items[index]
		item.Mappings = append(item.Mappings, mapping)
		if mapping.EffectiveTo == "" || mapping.EffectiveTo > today {
			item.MappingStatus = leaveMappingStatusMapped
		}
	}
	for _, balance := range balances {
		index, ok := byID[balance.LeaveTypeID]
		if !ok {
			index, ok = byCode[strings.ToLower(balance.LeaveType)]
		}
		if !ok {
			continue
		}
		item := &response.Items[index]
		item.BalanceCount++
		if strings.EqualFold(balance.Source, ehrmsAttendanceSource) {
			item.EHRMSBalanceCount++
			response.EHRMSBalances++
			if item.LastEHRMSSyncAt == nil || balance.UpdatedAt.After(*item.LastEHRMSSyncAt) {
				updatedAt := balance.UpdatedAt
				item.LastEHRMSSyncAt = &updatedAt
			}
		}
	}
	for _, request := range requests {
		index, ok := byID[request.LeaveTypeID]
		if !ok {
			index, ok = byCode[strings.ToLower(request.LeaveType)]
		}
		if ok {
			response.Items[index].RequestCount++
		}
	}
	for index := range response.Items {
		item := &response.Items[index]
		if item.MappingStatus == leaveMappingStatusMapped || item.EHRMSBalanceCount > 0 {
			item.MappingStatus = leaveMappingStatusMapped
			response.Mapped++
		}
	}
	response.NeedsMapping += len(response.UnmappedIssues)
	sort.Slice(response.Items, func(i, j int) bool { return response.Items[i].LeaveTypeCode < response.Items[j].LeaveTypeCode })
	return response, nil
}

// leaveTypeHasLinkedData protects stable leave identities from destructive policy removal.
func (c AttendanceService) leaveTypeHasLinkedData(ctx RequestContext, leaveType AttendanceLeaveType) (bool, error) {
	mappings, err := c.store.ListLeaveTypeExternalMappings(goContext(ctx), ctx.TenantID)
	if err != nil {
		return false, err
	}
	for _, mapping := range mappings {
		if mapping.LeaveTypeID == leaveType.ID || strings.EqualFold(mapping.LeaveTypeCode, leaveType.Code) {
			return true, nil
		}
	}
	balances, err := c.store.ListLeaveBalances(goContext(ctx), ctx.TenantID)
	if err != nil {
		return false, err
	}
	for _, balance := range balances {
		if balance.LeaveTypeID == leaveType.ID || strings.EqualFold(balance.LeaveType, leaveType.Code) {
			return true, nil
		}
	}
	requests, err := c.store.ListLeaveRequests(goContext(ctx), ctx.TenantID)
	if err != nil {
		return false, err
	}
	for _, request := range requests {
		if request.LeaveTypeID == leaveType.ID || strings.EqualFold(request.LeaveType, leaveType.Code) {
			return true, nil
		}
	}
	return false, nil
}

func validateOptionalDateRange(from, to string) error {
	for _, value := range []string{strings.TrimSpace(from), strings.TrimSpace(to)} {
		if value != "" {
			if _, err := time.Parse(time.DateOnly, value); err != nil {
				return BadRequest("effective dates must use YYYY-MM-DD")
			}
		}
	}
	if from != "" && to != "" && to <= from {
		return BadRequest("effective_to must be after effective_from")
	}
	return nil
}

func dateRangesOverlap(leftFrom, leftTo, rightFrom, rightTo string) bool {
	return (leftTo == "" || rightFrom == "" || leftTo > rightFrom) && (rightTo == "" || leftFrom == "" || rightTo > leftFrom)
}
