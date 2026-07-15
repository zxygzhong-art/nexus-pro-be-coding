package service

import (
	"strings"
	"time"

	"nexus-pro-be/internal/utils"
)

const (
	attendanceStatusActive   = "active"
	attendanceStatusInactive = "inactive"

	clockDirectionIn  = "clock_in"
	clockDirectionOut = "clock_out"

	clockRecordStatusAccepted = "accepted"
	clockRecordStatusAbnormal = "abnormal"
	clockRecordStatusRejected = "rejected"
	clockModeFlexible         = "flexible"
	clockModeFixed            = "fixed"

	clockRejectionDuplicate             = "duplicate"
	clockRejectionInvalidSequence       = "invalid_sequence"
	clockRejectionLowAccuracy           = "low_location_accuracy"
	clockRejectionOutsideGeofence       = "outside_geofence"
	clockRejectionOutsideWindow         = "outside_time_window"
	clockRejectionInsufficientWorkHours = "insufficient_work_hours"

	clockSourceGeofence         = "geofence"
	clockSourceManualCorrection = "manual_correction"

	correctionStatusPending  = "pending"
	correctionStatusApproved = "approved"
	correctionStatusRejected = "rejected"

	correctionTypeAddRecord     = "add_record"
	correctionTypeVoidRecord    = "void_record"
	correctionTypeReplaceRecord = "replace_record"

	overtimeTypeWeekday = "weekday"
	overtimeTypeHoliday = "holiday"

	overtimeCompensationLeave = "leave"
	overtimeCompensationPay   = "pay"

	compensatoryLeaveType = "compensatory"

	clockMaxAccuracyMeters = 200.0
)

var attendanceClockLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// AttendanceService 定義考勤服務的資料結構。
type AttendanceService struct {
	*Service
	store attendanceStore
}

// Attendance 處理考勤的服務流程。
func (c *Service) Attendance() AttendanceService {
	return AttendanceService{Service: c, store: c.store}
}

// ListLeaveBalances 列出請假 balances 的服務流程。
func (c AttendanceService) ListLeaveBalances(ctx RequestContext) ([]LeaveBalance, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppAttendance, ResourceType: ResourceLeave, Action: ActionRead})
	if err != nil {
		return nil, err
	}
	if !decision.Allowed {
		return nil, forbiddenAuthz(decision)
	}
	items, err := c.store.ListLeaveBalances(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	allowed, all, err := c.attendanceEmployeeScope(ctx, account, decision)
	if err != nil {
		return nil, err
	}
	if all {
		return items, nil
	}
	return filterLeaveBalancesByEmployees(items, allowed), nil
}

// ListLeaveBalancePage 列出請假 balance 分頁的服務流程。
func (c AttendanceService) ListLeaveBalancePage(ctx RequestContext, page PageRequest) (PageResponse[LeaveBalance], error) {
	return c.ListLeaveBalancePageByQuery(ctx, LeaveBalanceQuery{}, page)
}

// ListLeaveBalancePageByQuery filters authorized balances before sorting and pagination.
func (c AttendanceService) ListLeaveBalancePageByQuery(ctx RequestContext, query LeaveBalanceQuery, page PageRequest) (PageResponse[LeaveBalance], error) {
	items, err := c.ListLeaveBalances(ctx)
	if err != nil {
		return PageResponse[LeaveBalance]{}, err
	}
	if employeeIDs := employeeIDsFromSlice(query.EmployeeIDs); len(employeeIDs) > 0 {
		allowed := make(map[string]struct{}, len(employeeIDs))
		for _, employeeID := range employeeIDs {
			allowed[employeeID] = struct{}{}
		}
		items = filterLeaveBalancesByEmployees(items, allowed)
	}
	items = utils.SortLeaveBalances(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// ListLeaveRequests 列出請假請求的服務流程。
func (c AttendanceService) ListLeaveRequests(ctx RequestContext) ([]LeaveRequest, error) {
	return c.listLeaveRequestsByQuery(ctx, LeaveRequestQuery{})
}

// listLeaveRequestsByQuery 列出請假請求 by 查詢的服務流程。
func (c AttendanceService) listLeaveRequestsByQuery(ctx RequestContext, query LeaveRequestQuery) ([]LeaveRequest, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppAttendance, ResourceType: ResourceLeave, Action: ActionRead})
	if err != nil {
		return nil, err
	}
	if !decision.Allowed {
		return nil, forbiddenAuthz(decision)
	}
	allowed, all, err := c.attendanceEmployeeScope(ctx, account, decision)
	if err != nil {
		return nil, err
	}
	if !all {
		query.EmployeeIDs = employeeIDsFromSet(allowed)
		if len(query.EmployeeIDs) == 0 {
			return []LeaveRequest{}, nil
		}
	}
	return c.store.ListLeaveRequestsByQuery(goContext(ctx), ctx.TenantID, normalizeLeaveRequestQuery(query))
}

// ListLeaveRequestPage 列出請假請求分頁的服務流程。
func (c AttendanceService) ListLeaveRequestPage(ctx RequestContext, page PageRequest) (PageResponse[LeaveRequest], error) {
	return c.ListLeaveRequestPageByQuery(ctx, LeaveRequestQuery{}, page)
}

// ListLeaveRequestPageByQuery intersects requested employees with the caller's authorized scope.
func (c AttendanceService) ListLeaveRequestPageByQuery(ctx RequestContext, query LeaveRequestQuery, page PageRequest) (PageResponse[LeaveRequest], error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return PageResponse[LeaveRequest]{}, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppAttendance, ResourceType: ResourceLeave, Action: ActionRead})
	if err != nil {
		return PageResponse[LeaveRequest]{}, err
	}
	if !decision.Allowed {
		return PageResponse[LeaveRequest]{}, forbiddenAuthz(decision)
	}
	allowed, all, err := c.attendanceEmployeeScope(ctx, account, decision)
	if err != nil {
		return PageResponse[LeaveRequest]{}, err
	}
	if !all {
		query.EmployeeIDs = intersectEmployeeIDs(query.EmployeeIDs, allowed)
		if len(query.EmployeeIDs) == 0 {
			page = utils.NormalizePageRequest(page)
			return PageResponse[LeaveRequest]{Items: []LeaveRequest{}, Page: page.Page, PageSize: page.PageSize, Sort: page.Sort}, nil
		}
	}
	page = utils.NormalizePageRequest(page)
	items, total, err := c.store.ListLeaveRequestPageByQuery(goContext(ctx), ctx.TenantID, normalizeLeaveRequestQuery(query), page)
	if err != nil {
		return PageResponse[LeaveRequest]{}, err
	}
	return utils.PageResponseFromStore(items, total, page), nil
}

// attendanceEmployeeScope 處理考勤員工範圍的服務流程。
func (c AttendanceService) attendanceEmployeeScope(ctx RequestContext, account Account, decision CheckResult) (map[string]struct{}, bool, error) {
	if decision.Scope == "" || decision.Scope == "all" || decision.Scope == "tenant" {
		return nil, true, nil
	}
	ids := stringSliceFromAny(decision.Conditions["employee_ids"])
	if len(ids) == 0 && decision.Scope == "self" && account.EmployeeID != "" {
		ids = []string{account.EmployeeID}
	}
	allowed := map[string]struct{}{}
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			allowed[id] = struct{}{}
		}
	}
	orgIDs := stringSliceFromAny(decision.Conditions["org_unit_ids"])
	if len(orgIDs) > 0 {
		employees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
		if err != nil {
			return nil, false, err
		}
		orgScope := map[string]struct{}{}
		for _, id := range orgIDs {
			orgScope[id] = struct{}{}
		}
		units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
		if err != nil {
			return nil, false, err
		}
		for _, employee := range employees {
			if orgUnitInScope(units, employee.OrgUnitID, orgScope) {
				allowed[employee.ID] = struct{}{}
			}
		}
	}
	return allowed, false, nil
}

// requireAttendanceAuthz 處理 require 考勤授權的服務流程。
func (c AttendanceService) requireAttendanceAuthz(ctx RequestContext, resource ResourceType, action Action, resourceID string) (Account, CheckResult, error) {
	return c.Service.requireServiceAuthz(ctx, AppAttendance, resource, action, resourceID)
}

// ensureAttendanceEmployeeAllowed 確保考勤員工 allowed 的服務流程。
func (c AttendanceService) ensureAttendanceEmployeeAllowed(ctx RequestContext, account Account, decision CheckResult, employeeID string) error {
	allowed, all, err := c.attendanceEmployeeScope(ctx, account, decision)
	if err != nil {
		return err
	}
	if all {
		return nil
	}
	if _, ok := allowed[employeeID]; ok {
		return nil
	}
	return forbiddenDataScope("employee is outside data scope")
}
