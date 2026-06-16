package service

import (
	"strings"
	"time"
)

type AttendanceService struct {
	*Service
}

func (c *Service) Attendance() AttendanceService {
	return AttendanceService{Service: c}
}

func (c *Service) ListLeaveBalances(ctx RequestContext) ([]LeaveBalance, error) {
	return c.Attendance().ListLeaveBalances(ctx)
}

func (c *Service) ListLeaveBalancePage(ctx RequestContext, page PageRequest) (PageResponse[LeaveBalance], error) {
	return c.Attendance().ListLeaveBalancePage(ctx, page)
}

func (c *Service) ListLeaveRequests(ctx RequestContext) ([]LeaveRequest, error) {
	return c.Attendance().ListLeaveRequests(ctx)
}

func (c *Service) ListLeaveRequestPage(ctx RequestContext, page PageRequest) (PageResponse[LeaveRequest], error) {
	return c.Attendance().ListLeaveRequestPage(ctx, page)
}

func (c *Service) CreateLeaveRequest(ctx RequestContext, input CreateLeaveRequestInput) (LeaveRequest, error) {
	return c.Attendance().CreateLeaveRequest(ctx, input)
}

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
		return []LeaveBalance{}, nil
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

func (c AttendanceService) ListLeaveBalancePage(ctx RequestContext, page PageRequest) (PageResponse[LeaveBalance], error) {
	items, err := c.ListLeaveBalances(ctx)
	if err != nil {
		return PageResponse[LeaveBalance]{}, err
	}
	items = sortLeaveBalances(items, page.Sort)
	return pageResponse(items, page), nil
}

func (c AttendanceService) ListLeaveRequests(ctx RequestContext) ([]LeaveRequest, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppAttendance, ResourceType: ResourceLeave, Action: ActionRead})
	if err != nil {
		return nil, err
	}
	if !decision.Allowed {
		return []LeaveRequest{}, nil
	}
	items, err := c.store.ListLeaveRequests(goContext(ctx), ctx.TenantID)
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
	return filterLeaveRequestsByEmployees(items, allowed), nil
}

func (c AttendanceService) ListLeaveRequestPage(ctx RequestContext, page PageRequest) (PageResponse[LeaveRequest], error) {
	items, err := c.ListLeaveRequests(ctx)
	if err != nil {
		return PageResponse[LeaveRequest]{}, err
	}
	items = sortLeaveRequests(items, page.Sort)
	return pageResponse(items, page), nil
}

func (c AttendanceService) CreateLeaveRequest(ctx RequestContext, input CreateLeaveRequestInput) (LeaveRequest, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return LeaveRequest{}, err
	}
	employeeID := strings.TrimSpace(input.EmployeeID)
	if employeeID == "" {
		employeeID = account.EmployeeID
	}
	if employeeID == "" {
		return LeaveRequest{}, BadRequest("employee_id is required")
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{
		ApplicationCode:  AppAttendance,
		ResourceType:     ResourceLeave,
		ResourceID:       employeeID,
		Target:           employeeID,
		TargetEmployeeID: employeeID,
		Action:           ActionCreate,
	})
	if err != nil {
		return LeaveRequest{}, err
	}
	if !decision.Allowed {
		return LeaveRequest{}, Forbidden(decision.Reason)
	}
	allowed, all, err := c.attendanceEmployeeScope(ctx, account, decision)
	if err != nil {
		return LeaveRequest{}, err
	}
	if !all {
		if _, ok := allowed[employeeID]; !ok {
			return LeaveRequest{}, Forbidden("employee is outside data scope")
		}
	}
	if _, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, employeeID); err != nil {
		return LeaveRequest{}, err
	} else if !ok {
		return LeaveRequest{}, NotFound("employee", employeeID)
	}
	if strings.TrimSpace(input.LeaveType) == "" {
		return LeaveRequest{}, BadRequest("leave_type is required")
	}
	if input.Hours <= 0 {
		return LeaveRequest{}, BadRequest("hours must be greater than zero")
	}
	startAt, err := parseDateTime(input.StartAt)
	if err != nil {
		return LeaveRequest{}, BadRequest("start_at must be RFC3339 or YYYY-MM-DD")
	}
	endAt, err := parseDateTime(input.EndAt)
	if err != nil {
		return LeaveRequest{}, BadRequest("end_at must be RFC3339 or YYYY-MM-DD")
	}
	if !endAt.After(startAt) {
		return LeaveRequest{}, BadRequest("end_at must be after start_at")
	}
	var req LeaveRequest
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
		balance, err := tx.Attendance().reserveLeaveBalance(ctx, employeeID, input.LeaveType, input.Hours)
		if err != nil {
			return err
		}
		template, ok, err := tx.store.GetFormTemplateByKey(goContext(ctx), ctx.TenantID, "leave-request")
		if err != nil {
			return err
		}
		if !ok {
			template = FormTemplate{
				ID:        newID("ft"),
				TenantID:  ctx.TenantID,
				Key:       "leave-request",
				Name:      "请假申请",
				Schema:    map[string]any{"type": "object"},
				CreatedAt: tx.Now(),
			}
			if err := tx.store.UpsertFormTemplate(goContext(ctx), template); err != nil {
				return err
			}
		}
		instance := FormInstance{
			ID:                 newID("fi"),
			TenantID:           ctx.TenantID,
			TemplateID:         template.ID,
			ApplicantAccountID: account.ID,
			Status:             "submitted",
			Payload: map[string]any{
				"employee_id": employeeID,
				"leave_type":  input.LeaveType,
				"start_at":    startAt.Format(time.RFC3339),
				"end_at":      endAt.Format(time.RFC3339),
				"hours":       input.Hours,
				"reason":      input.Reason,
			},
			SubmittedAt: tx.Now(),
			UpdatedAt:   tx.Now(),
		}
		if err := tx.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
			return err
		}
		req = LeaveRequest{
			ID:             newID("lr"),
			TenantID:       ctx.TenantID,
			EmployeeID:     employeeID,
			LeaveType:      strings.TrimSpace(input.LeaveType),
			StartAt:        startAt,
			EndAt:          endAt,
			Hours:          input.Hours,
			Reason:         strings.TrimSpace(input.Reason),
			Status:         "pending_approval",
			FormInstanceID: instance.ID,
			CreatedAt:      tx.Now(),
		}
		if err := tx.store.UpsertLeaveRequest(goContext(ctx), req); err != nil {
			return err
		}
		if err := tx.audit(ctx, "attendance.leave_balance.reserve", "leave_balance", balance.ID, "medium", map[string]any{
			"employee_id":     employeeID,
			"leave_type":      balance.LeaveType,
			"reserved_hours":  input.Hours,
			"remaining_hours": balance.RemainingHours,
		}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "attendance.leave_request.create", "leave_request", req.ID, "medium", map[string]any{"leave_type": req.LeaveType, "hours": req.Hours}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return LeaveRequest{}, err
	}
	return req, nil
}

func (c AttendanceService) reserveLeaveBalance(ctx RequestContext, employeeID, leaveType string, hours float64) (LeaveBalance, error) {
	balance, reserved, found, err := c.store.ReserveLeaveBalance(goContext(ctx), ctx.TenantID, employeeID, leaveType, hours, c.Now())
	if err != nil {
		return LeaveBalance{}, err
	}
	if !found {
		return LeaveBalance{}, BadRequest("leave balance is required for this leave type")
	}
	if !reserved {
		return LeaveBalance{}, BadRequest("leave balance is insufficient")
	}
	return balance, nil
}

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

func filterLeaveBalancesByEmployees(items []LeaveBalance, allowed map[string]struct{}) []LeaveBalance {
	out := make([]LeaveBalance, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.EmployeeID]; ok {
			out = append(out, item)
		}
	}
	return out
}

func filterLeaveRequestsByEmployees(items []LeaveRequest, allowed map[string]struct{}) []LeaveRequest {
	out := make([]LeaveRequest, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.EmployeeID]; ok {
			out = append(out, item)
		}
	}
	return out
}
