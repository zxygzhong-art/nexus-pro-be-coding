package service

import (
	"math"
	"strconv"
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
	clockRecordStatusRejected = "rejected"

	clockRejectionDuplicate       = "duplicate"
	clockRejectionOutsideGeofence = "outside_geofence"

	clockSourceGeofence         = "geofence"
	clockSourceManualCorrection = "manual_correction"

	correctionStatusPending  = "pending"
	correctionStatusApproved = "approved"
	correctionStatusRejected = "rejected"
)

// AttendanceService implements leave balance and leave request workflows.
type AttendanceService struct {
	*Service
	store attendanceStore
}

// Attendance returns the attendance service facade.
func (c *Service) Attendance() AttendanceService {
	return AttendanceService{Service: c, store: c.store}
}

// ListLeaveBalances returns leave balances visible under the current authorization scope.
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

// ListLeaveBalancePage returns paginated visible leave balances.
func (c AttendanceService) ListLeaveBalancePage(ctx RequestContext, page PageRequest) (PageResponse[LeaveBalance], error) {
	items, err := c.ListLeaveBalances(ctx)
	if err != nil {
		return PageResponse[LeaveBalance]{}, err
	}
	items = utils.SortLeaveBalances(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// ListLeaveRequests returns leave requests visible under the current authorization scope.
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

// ListLeaveRequestPage returns paginated visible leave requests.
func (c AttendanceService) ListLeaveRequestPage(ctx RequestContext, page PageRequest) (PageResponse[LeaveRequest], error) {
	items, err := c.ListLeaveRequests(ctx)
	if err != nil {
		return PageResponse[LeaveRequest]{}, err
	}
	items = utils.SortLeaveRequests(items, page.Sort)
	return utils.PageResponse(items, page), nil
}

// CurrentAttendancePolicy returns the current tenant attendance policy projection.
func (c AttendanceService) CurrentAttendancePolicy(ctx RequestContext) (AttendancePolicyResponse, error) {
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceLeave, ActionRead, ""); err != nil {
		return AttendancePolicyResponse{}, err
	}
	return AttendancePolicyResponse{
		WorkTime: AttendancePolicyWorkTime{
			StandardStart:     "09:00",
			StandardEnd:       "18:00",
			BreakStart:        "12:00",
			BreakEnd:          "13:00",
			Weekend:           "週六、週日",
			CycleStart:        "1 日",
			CycleEnd:          "本月 月底（最後一日）",
			TimeOptions:       attendancePolicyTimeOptions(),
			WeekendOptions:    []string{"週六、週日", "週日", "無"},
			CycleStartOptions: attendancePolicyCycleStartOptions(),
			CycleEndOptions:   attendancePolicyCycleEndOptions(),
		},
		LeaveTypes: attendancePolicyLeaveTypes(),
	}, nil
}

// CreateLeaveRequest creates a leave request and reserves leave balance atomically.
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
	startAt, err := utils.ParseDateTime(input.StartAt)
	if err != nil {
		return LeaveRequest{}, BadRequest("start_at must be RFC3339 or YYYY-MM-DD")
	}
	endAt, err := utils.ParseDateTime(input.EndAt)
	if err != nil {
		return LeaveRequest{}, BadRequest("end_at must be RFC3339 or YYYY-MM-DD")
	}
	if !endAt.After(startAt) {
		return LeaveRequest{}, BadRequest("end_at must be after start_at")
	}
	var req LeaveRequest
	if err := c.withTransaction(ctx, func(tx AttendanceService) error {
		balance, err := tx.reserveLeaveBalance(ctx, employeeID, input.LeaveType, input.Hours)
		if err != nil {
			return err
		}
		template, ok, err := tx.store.GetFormTemplateByKey(goContext(ctx), ctx.TenantID, "leave-request")
		if err != nil {
			return err
		}
		if !ok {
			template = FormTemplate{
				ID:        utils.NewID("ft"),
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
			ID:                 utils.NewID("fi"),
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
			ID:             utils.NewID("lr"),
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
	c.logInfo(ctx, "leave request created",
		"leave_request_id", req.ID,
		"employee_id", req.EmployeeID,
		"leave_type", req.LeaveType,
		"hours", req.Hours,
		"status", req.Status,
		"form_instance_id", req.FormInstanceID,
	)
	return req, nil
}

// ListAttendanceWorksitePage returns configured worksites visible to the caller.
func (c AttendanceService) ListAttendanceWorksitePage(ctx RequestContext, page PageRequest) (PageResponse[AttendanceWorksite], error) {
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceAttendanceWorksite, ActionRead, ""); err != nil {
		return PageResponse[AttendanceWorksite]{}, err
	}
	items, err := c.store.ListAttendanceWorksites(goContext(ctx), ctx.TenantID)
	if err != nil {
		return PageResponse[AttendanceWorksite]{}, err
	}
	return utils.PageResponse(items, page), nil
}

// CreateAttendanceWorksite creates a tenant-scoped clock-in geofence.
func (c AttendanceService) CreateAttendanceWorksite(ctx RequestContext, input CreateAttendanceWorksiteInput) (AttendanceWorksite, error) {
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceAttendanceWorksite, ActionCreate, ""); err != nil {
		return AttendanceWorksite{}, err
	}
	status := normalizeAttendanceStatus(input.Status)
	if err := validateWorksiteInput(input.Name, input.Latitude, input.Longitude, input.RadiusMeters, status); err != nil {
		return AttendanceWorksite{}, err
	}
	now := c.Now()
	item := AttendanceWorksite{
		ID:           utils.NewID("aws"),
		TenantID:     ctx.TenantID,
		Name:         strings.TrimSpace(input.Name),
		Address:      strings.TrimSpace(input.Address),
		Latitude:     input.Latitude,
		Longitude:    input.Longitude,
		RadiusMeters: input.RadiusMeters,
		Status:       status,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := c.store.UpsertAttendanceWorksite(goContext(ctx), item); err != nil {
		return AttendanceWorksite{}, err
	}
	if err := c.audit(ctx, "attendance.worksite.create", string(ResourceAttendanceWorksite), item.ID, string(SeverityMedium), map[string]any{"name": item.Name}); err != nil {
		return AttendanceWorksite{}, err
	}
	return item, nil
}

// UpdateAttendanceWorksite applies partial updates to one worksite.
func (c AttendanceService) UpdateAttendanceWorksite(ctx RequestContext, input UpdateAttendanceWorksiteInput) (AttendanceWorksite, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return AttendanceWorksite{}, BadRequest("id is required")
	}
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceAttendanceWorksite, ActionUpdate, id); err != nil {
		return AttendanceWorksite{}, err
	}
	item, ok, err := c.store.GetAttendanceWorksite(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return AttendanceWorksite{}, err
	}
	if !ok {
		return AttendanceWorksite{}, NotFound("attendance worksite", id)
	}
	if input.Name != nil {
		item.Name = strings.TrimSpace(*input.Name)
	}
	if input.Address != nil {
		item.Address = strings.TrimSpace(*input.Address)
	}
	if input.Latitude != nil {
		item.Latitude = *input.Latitude
	}
	if input.Longitude != nil {
		item.Longitude = *input.Longitude
	}
	if input.RadiusMeters != nil {
		item.RadiusMeters = *input.RadiusMeters
	}
	if input.Status != nil {
		item.Status = normalizeAttendanceStatus(*input.Status)
	}
	if err := validateWorksiteInput(item.Name, item.Latitude, item.Longitude, item.RadiusMeters, item.Status); err != nil {
		return AttendanceWorksite{}, err
	}
	item.UpdatedAt = c.Now()
	if err := c.store.UpsertAttendanceWorksite(goContext(ctx), item); err != nil {
		return AttendanceWorksite{}, err
	}
	if err := c.audit(ctx, "attendance.worksite.update", string(ResourceAttendanceWorksite), item.ID, string(SeverityMedium), map[string]any{"name": item.Name}); err != nil {
		return AttendanceWorksite{}, err
	}
	return item, nil
}

// ListAttendanceShiftPage returns configured shifts visible to the caller.
func (c AttendanceService) ListAttendanceShiftPage(ctx RequestContext, page PageRequest) (PageResponse[AttendanceShift], error) {
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceAttendanceShift, ActionRead, ""); err != nil {
		return PageResponse[AttendanceShift]{}, err
	}
	items, err := c.store.ListAttendanceShifts(goContext(ctx), ctx.TenantID)
	if err != nil {
		return PageResponse[AttendanceShift]{}, err
	}
	return utils.PageResponse(items, page), nil
}

// CreateAttendanceShift creates a same-day shift definition.
func (c AttendanceService) CreateAttendanceShift(ctx RequestContext, input CreateAttendanceShiftInput) (AttendanceShift, error) {
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceAttendanceShift, ActionCreate, ""); err != nil {
		return AttendanceShift{}, err
	}
	status := normalizeAttendanceStatus(input.Status)
	if err := validateShiftInput(input.Name, input.ClockInStart, input.ClockInEnd, input.ClockOutStart, input.ClockOutEnd, input.LateGraceMinutes, input.EarlyLeaveGraceMinutes, status); err != nil {
		return AttendanceShift{}, err
	}
	now := c.Now()
	item := AttendanceShift{
		ID:                     utils.NewID("ash"),
		TenantID:               ctx.TenantID,
		Name:                   strings.TrimSpace(input.Name),
		ClockInStart:           strings.TrimSpace(input.ClockInStart),
		ClockInEnd:             strings.TrimSpace(input.ClockInEnd),
		ClockOutStart:          strings.TrimSpace(input.ClockOutStart),
		ClockOutEnd:            strings.TrimSpace(input.ClockOutEnd),
		LateGraceMinutes:       input.LateGraceMinutes,
		EarlyLeaveGraceMinutes: input.EarlyLeaveGraceMinutes,
		Status:                 status,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	if err := c.store.UpsertAttendanceShift(goContext(ctx), item); err != nil {
		return AttendanceShift{}, err
	}
	if err := c.audit(ctx, "attendance.shift.create", string(ResourceAttendanceShift), item.ID, string(SeverityMedium), map[string]any{"name": item.Name}); err != nil {
		return AttendanceShift{}, err
	}
	return item, nil
}

// UpdateAttendanceShift applies partial updates to one shift.
func (c AttendanceService) UpdateAttendanceShift(ctx RequestContext, input UpdateAttendanceShiftInput) (AttendanceShift, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return AttendanceShift{}, BadRequest("id is required")
	}
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceAttendanceShift, ActionUpdate, id); err != nil {
		return AttendanceShift{}, err
	}
	item, ok, err := c.store.GetAttendanceShift(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return AttendanceShift{}, err
	}
	if !ok {
		return AttendanceShift{}, NotFound("attendance shift", id)
	}
	if input.Name != nil {
		item.Name = strings.TrimSpace(*input.Name)
	}
	if input.ClockInStart != nil {
		item.ClockInStart = strings.TrimSpace(*input.ClockInStart)
	}
	if input.ClockInEnd != nil {
		item.ClockInEnd = strings.TrimSpace(*input.ClockInEnd)
	}
	if input.ClockOutStart != nil {
		item.ClockOutStart = strings.TrimSpace(*input.ClockOutStart)
	}
	if input.ClockOutEnd != nil {
		item.ClockOutEnd = strings.TrimSpace(*input.ClockOutEnd)
	}
	if input.LateGraceMinutes != nil {
		item.LateGraceMinutes = *input.LateGraceMinutes
	}
	if input.EarlyLeaveGraceMinutes != nil {
		item.EarlyLeaveGraceMinutes = *input.EarlyLeaveGraceMinutes
	}
	if input.Status != nil {
		item.Status = normalizeAttendanceStatus(*input.Status)
	}
	if err := validateShiftInput(item.Name, item.ClockInStart, item.ClockInEnd, item.ClockOutStart, item.ClockOutEnd, item.LateGraceMinutes, item.EarlyLeaveGraceMinutes, item.Status); err != nil {
		return AttendanceShift{}, err
	}
	item.UpdatedAt = c.Now()
	if err := c.store.UpsertAttendanceShift(goContext(ctx), item); err != nil {
		return AttendanceShift{}, err
	}
	if err := c.audit(ctx, "attendance.shift.update", string(ResourceAttendanceShift), item.ID, string(SeverityMedium), map[string]any{"name": item.Name}); err != nil {
		return AttendanceShift{}, err
	}
	return item, nil
}

// ListAttendanceShiftAssignmentPage returns shift assignments under the caller's employee scope.
func (c AttendanceService) ListAttendanceShiftAssignmentPage(ctx RequestContext, page PageRequest) (PageResponse[AttendanceShiftAssignment], error) {
	account, decision, err := c.requireAttendanceAuthz(ctx, ResourceAttendanceShiftAssignment, ActionRead, "")
	if err != nil {
		return PageResponse[AttendanceShiftAssignment]{}, err
	}
	items, err := c.store.ListAttendanceShiftAssignments(goContext(ctx), ctx.TenantID)
	if err != nil {
		return PageResponse[AttendanceShiftAssignment]{}, err
	}
	items, err = c.filterShiftAssignmentsByDecision(ctx, account, decision, items)
	if err != nil {
		return PageResponse[AttendanceShiftAssignment]{}, err
	}
	return utils.PageResponse(items, page), nil
}

// CreateAttendanceShiftAssignment binds an employee to a shift and worksite.
func (c AttendanceService) CreateAttendanceShiftAssignment(ctx RequestContext, input CreateAttendanceShiftAssignmentInput) (AttendanceShiftAssignment, error) {
	account, decision, err := c.requireAttendanceAuthz(ctx, ResourceAttendanceShiftAssignment, ActionCreate, "")
	if err != nil {
		return AttendanceShiftAssignment{}, err
	}
	employeeID := strings.TrimSpace(input.EmployeeID)
	if employeeID == "" || strings.TrimSpace(input.ShiftID) == "" || strings.TrimSpace(input.WorksiteID) == "" {
		return AttendanceShiftAssignment{}, BadRequest("employee_id, shift_id and worksite_id are required")
	}
	if err := c.ensureAttendanceEmployeeAllowed(ctx, account, decision, employeeID); err != nil {
		return AttendanceShiftAssignment{}, err
	}
	if _, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, employeeID); err != nil {
		return AttendanceShiftAssignment{}, err
	} else if !ok {
		return AttendanceShiftAssignment{}, NotFound("employee", employeeID)
	}
	shift, ok, err := c.store.GetAttendanceShift(goContext(ctx), ctx.TenantID, strings.TrimSpace(input.ShiftID))
	if err != nil {
		return AttendanceShiftAssignment{}, err
	}
	if !ok {
		return AttendanceShiftAssignment{}, NotFound("attendance shift", input.ShiftID)
	}
	worksite, ok, err := c.store.GetAttendanceWorksite(goContext(ctx), ctx.TenantID, strings.TrimSpace(input.WorksiteID))
	if err != nil {
		return AttendanceShiftAssignment{}, err
	}
	if !ok {
		return AttendanceShiftAssignment{}, NotFound("attendance worksite", input.WorksiteID)
	}
	if !strings.EqualFold(shift.Status, attendanceStatusActive) || !strings.EqualFold(worksite.Status, attendanceStatusActive) {
		return AttendanceShiftAssignment{}, BadRequest("shift and worksite must be active")
	}
	effectiveFrom, err := utils.ParseDateTime(input.EffectiveFrom)
	if err != nil {
		return AttendanceShiftAssignment{}, BadRequest("effective_from must be RFC3339 or YYYY-MM-DD")
	}
	effectiveTo, err := optionalAttendanceDateTime(input.EffectiveTo)
	if err != nil {
		return AttendanceShiftAssignment{}, BadRequest("effective_to must be RFC3339 or YYYY-MM-DD")
	}
	if effectiveTo != nil && effectiveTo.Before(effectiveFrom) {
		return AttendanceShiftAssignment{}, BadRequest("effective_to must be after effective_from")
	}
	status := normalizeAttendanceStatus(input.Status)
	now := c.Now()
	item := AttendanceShiftAssignment{
		ID:            utils.NewID("asa"),
		TenantID:      ctx.TenantID,
		EmployeeID:    employeeID,
		ShiftID:       shift.ID,
		WorksiteID:    worksite.ID,
		EffectiveFrom: effectiveFrom,
		EffectiveTo:   effectiveTo,
		Status:        status,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := c.store.UpsertAttendanceShiftAssignment(goContext(ctx), item); err != nil {
		return AttendanceShiftAssignment{}, err
	}
	if err := c.audit(ctx, "attendance.shift_assignment.create", string(ResourceAttendanceShiftAssignment), item.ID, string(SeverityMedium), map[string]any{"employee_id": item.EmployeeID}); err != nil {
		return AttendanceShiftAssignment{}, err
	}
	return item, nil
}

// AttendanceClockStatus returns today's assignment and accepted clock records.
func (c AttendanceService) AttendanceClockStatus(ctx RequestContext) (AttendanceClockStatus, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return AttendanceClockStatus{}, err
	}
	if account.EmployeeID == "" {
		return AttendanceClockStatus{}, BadRequest("employee_id is required")
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{
		ApplicationCode:  AppAttendance,
		ResourceType:     ResourceAttendanceClock,
		Action:           ActionRead,
		TargetEmployeeID: account.EmployeeID,
	})
	if err != nil {
		return AttendanceClockStatus{}, err
	}
	if !decision.Allowed {
		return AttendanceClockStatus{}, Forbidden(decision.Reason)
	}
	now := c.Now()
	workDate := attendanceWorkDate(now)
	status := AttendanceClockStatus{EmployeeID: account.EmployeeID, WorkDate: workDate, NextAction: "no_assignment"}
	assignment, ok, err := c.store.FindEffectiveAttendanceShiftAssignment(goContext(ctx), ctx.TenantID, account.EmployeeID, now)
	if err != nil {
		return AttendanceClockStatus{}, err
	}
	if !ok {
		return status, nil
	}
	status.Assignment = &assignment
	if shift, ok, err := c.store.GetAttendanceShift(goContext(ctx), ctx.TenantID, assignment.ShiftID); err != nil {
		return AttendanceClockStatus{}, err
	} else if ok {
		status.Shift = &shift
	}
	if worksite, ok, err := c.store.GetAttendanceWorksite(goContext(ctx), ctx.TenantID, assignment.WorksiteID); err != nil {
		return AttendanceClockStatus{}, err
	} else if ok {
		status.Worksite = &worksite
	}
	if record, ok, err := c.store.GetAcceptedAttendanceClockRecord(goContext(ctx), ctx.TenantID, account.EmployeeID, workDate, clockDirectionIn); err != nil {
		return AttendanceClockStatus{}, err
	} else if ok {
		status.ClockIn = &record
	}
	if record, ok, err := c.store.GetAcceptedAttendanceClockRecord(goContext(ctx), ctx.TenantID, account.EmployeeID, workDate, clockDirectionOut); err != nil {
		return AttendanceClockStatus{}, err
	} else if ok {
		status.ClockOut = &record
	}
	status.NextAction = nextClockAction(status.ClockIn, status.ClockOut)
	return status, nil
}

// CreateAttendanceClockRecord records one geofenced clock attempt.
func (c AttendanceService) CreateAttendanceClockRecord(ctx RequestContext, input CreateAttendanceClockRecordInput) (AttendanceClockRecord, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return AttendanceClockRecord{}, err
	}
	employeeID := strings.TrimSpace(input.EmployeeID)
	if employeeID == "" {
		employeeID = account.EmployeeID
	}
	if employeeID == "" {
		return AttendanceClockRecord{}, BadRequest("employee_id is required")
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{
		ApplicationCode:  AppAttendance,
		ResourceType:     ResourceAttendanceClock,
		Action:           ActionCreate,
		TargetEmployeeID: employeeID,
		Target:           employeeID,
	})
	if err != nil {
		return AttendanceClockRecord{}, err
	}
	if !decision.Allowed {
		return AttendanceClockRecord{}, Forbidden(decision.Reason)
	}
	if err := c.ensureAttendanceEmployeeAllowed(ctx, account, decision, employeeID); err != nil {
		return AttendanceClockRecord{}, err
	}
	if _, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, employeeID); err != nil {
		return AttendanceClockRecord{}, err
	} else if !ok {
		return AttendanceClockRecord{}, NotFound("employee", employeeID)
	}
	direction, err := normalizeClockDirection(input.Direction)
	if err != nil {
		return AttendanceClockRecord{}, err
	}
	if err := validateCoordinates(input.Latitude, input.Longitude); err != nil {
		return AttendanceClockRecord{}, err
	}
	if input.AccuracyMeters < 0 {
		return AttendanceClockRecord{}, BadRequest("accuracy_meters must be greater than or equal to zero")
	}
	now := c.Now()
	assignment, shift, worksite, err := c.attendanceAssignmentBundle(ctx, employeeID, now)
	if err != nil {
		return AttendanceClockRecord{}, err
	}
	workDate := attendanceWorkDate(now)
	distance := haversineMeters(input.Latitude, input.Longitude, worksite.Latitude, worksite.Longitude)
	recordStatus := clockRecordStatusAccepted
	rejectionReason := ""
	if _, exists, err := c.store.GetAcceptedAttendanceClockRecord(goContext(ctx), ctx.TenantID, employeeID, workDate, direction); err != nil {
		return AttendanceClockRecord{}, err
	} else if exists {
		recordStatus = clockRecordStatusRejected
		rejectionReason = clockRejectionDuplicate
	} else if distance > float64(worksite.RadiusMeters) {
		recordStatus = clockRecordStatusRejected
		rejectionReason = clockRejectionOutsideGeofence
	}
	deviceInfo := utils.CopyStringMap(input.DeviceInfo)
	if deviceInfo == nil {
		deviceInfo = map[string]any{}
	}
	if strings.TrimSpace(input.LocationSource) != "" {
		deviceInfo["location_source"] = strings.TrimSpace(input.LocationSource)
	}
	record := AttendanceClockRecord{
		ID:                utils.NewID("acr"),
		TenantID:          ctx.TenantID,
		EmployeeID:        employeeID,
		ShiftAssignmentID: assignment.ID,
		ShiftID:           shift.ID,
		WorksiteID:        worksite.ID,
		WorkDate:          workDate,
		Direction:         direction,
		ClockedAt:         now,
		Latitude:          input.Latitude,
		Longitude:         input.Longitude,
		AccuracyMeters:    input.AccuracyMeters,
		DistanceMeters:    distance,
		RecordStatus:      recordStatus,
		RejectionReason:   rejectionReason,
		Source:            clockSourceGeofence,
		DeviceID:          strings.TrimSpace(input.DeviceID),
		DeviceInfo:        deviceInfo,
		CreatedAt:         now,
	}
	if err := c.store.UpsertAttendanceClockRecord(goContext(ctx), record); err != nil {
		return AttendanceClockRecord{}, err
	}
	if err := c.audit(ctx, "attendance.clock_record.create", string(ResourceAttendanceClock), record.ID, string(SeverityMedium), map[string]any{
		"employee_id":      record.EmployeeID,
		"direction":        record.Direction,
		"record_status":    record.RecordStatus,
		"rejection_reason": record.RejectionReason,
	}); err != nil {
		return AttendanceClockRecord{}, err
	}
	return record, nil
}

// ListAttendanceClockRecordPage returns clock records under the caller's employee scope.
func (c AttendanceService) ListAttendanceClockRecordPage(ctx RequestContext, query AttendanceClockRecordQuery, page PageRequest) (PageResponse[AttendanceClockRecord], error) {
	account, decision, err := c.requireAttendanceAuthz(ctx, ResourceAttendanceClock, ActionRead, "")
	if err != nil {
		return PageResponse[AttendanceClockRecord]{}, err
	}
	query = normalizeClockRecordQuery(query)
	items, err := c.store.ListAttendanceClockRecords(goContext(ctx), ctx.TenantID, query)
	if err != nil {
		return PageResponse[AttendanceClockRecord]{}, err
	}
	items, err = c.filterClockRecordsByDecision(ctx, account, decision, items)
	if err != nil {
		return PageResponse[AttendanceClockRecord]{}, err
	}
	return utils.PageResponse(items, page), nil
}

// CreateAttendanceCorrection submits a manual clock correction request.
func (c AttendanceService) CreateAttendanceCorrection(ctx RequestContext, input CreateAttendanceCorrectionInput) (AttendanceCorrectionRequest, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	employeeID := strings.TrimSpace(input.EmployeeID)
	if employeeID == "" {
		employeeID = account.EmployeeID
	}
	if employeeID == "" {
		return AttendanceCorrectionRequest{}, BadRequest("employee_id is required")
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{
		ApplicationCode:  AppAttendance,
		ResourceType:     ResourceAttendanceCorrection,
		Action:           ActionCreate,
		TargetEmployeeID: employeeID,
		Target:           employeeID,
	})
	if err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	if !decision.Allowed {
		return AttendanceCorrectionRequest{}, Forbidden(decision.Reason)
	}
	if err := c.ensureAttendanceEmployeeAllowed(ctx, account, decision, employeeID); err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	if _, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, employeeID); err != nil {
		return AttendanceCorrectionRequest{}, err
	} else if !ok {
		return AttendanceCorrectionRequest{}, NotFound("employee", employeeID)
	}
	direction, err := normalizeClockDirection(input.Direction)
	if err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	requestedAt, err := utils.ParseDateTime(input.RequestedClockedAt)
	if err != nil {
		return AttendanceCorrectionRequest{}, BadRequest("requested_clocked_at must be RFC3339 or YYYY-MM-DD")
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return AttendanceCorrectionRequest{}, BadRequest("reason is required")
	}
	if _, _, _, err := c.attendanceAssignmentBundle(ctx, employeeID, requestedAt); err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	var correction AttendanceCorrectionRequest
	if err := c.withTransaction(ctx, func(tx AttendanceService) error {
		template, ok, err := tx.store.GetFormTemplateByKey(goContext(ctx), ctx.TenantID, "attendance-correction")
		if err != nil {
			return err
		}
		if !ok {
			template = FormTemplate{
				ID:        utils.NewID("ft"),
				TenantID:  ctx.TenantID,
				Key:       "attendance-correction",
				Name:      "补卡申请",
				Schema:    map[string]any{"type": "object"},
				CreatedAt: tx.Now(),
			}
			if err := tx.store.UpsertFormTemplate(goContext(ctx), template); err != nil {
				return err
			}
		}
		instance := FormInstance{
			ID:                 utils.NewID("fi"),
			TenantID:           ctx.TenantID,
			TemplateID:         template.ID,
			ApplicantAccountID: account.ID,
			Status:             "submitted",
			Payload: map[string]any{
				"application_code":     string(AppAttendance),
				"resource_type":        string(ResourceAttendanceCorrection),
				"action":               string(ActionCreate),
				"employee_id":          employeeID,
				"direction":            direction,
				"requested_clocked_at": requestedAt.Format(time.RFC3339),
				"reason":               reason,
			},
			SubmittedAt: tx.Now(),
			UpdatedAt:   tx.Now(),
		}
		if err := tx.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
			return err
		}
		correction = AttendanceCorrectionRequest{
			ID:                 utils.NewID("acorr"),
			TenantID:           ctx.TenantID,
			EmployeeID:         employeeID,
			Direction:          direction,
			RequestedClockedAt: requestedAt,
			WorkDate:           attendanceWorkDate(requestedAt),
			Reason:             reason,
			Status:             correctionStatusPending,
			FormInstanceID:     instance.ID,
			CreatedAt:          tx.Now(),
			UpdatedAt:          tx.Now(),
		}
		if err := tx.store.UpsertAttendanceCorrectionRequest(goContext(ctx), correction); err != nil {
			return err
		}
		return tx.audit(ctx, "attendance.correction.create", string(ResourceAttendanceCorrection), correction.ID, string(SeverityMedium), map[string]any{"employee_id": employeeID, "direction": direction})
	}); err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	return correction, nil
}

// ListAttendanceCorrectionPage returns correction requests under the caller's employee scope.
func (c AttendanceService) ListAttendanceCorrectionPage(ctx RequestContext, query AttendanceCorrectionQuery, page PageRequest) (PageResponse[AttendanceCorrectionRequest], error) {
	account, decision, err := c.requireAttendanceAuthz(ctx, ResourceAttendanceCorrection, ActionRead, "")
	if err != nil {
		return PageResponse[AttendanceCorrectionRequest]{}, err
	}
	query = normalizeCorrectionQuery(query)
	items, err := c.store.ListAttendanceCorrectionRequests(goContext(ctx), ctx.TenantID, query)
	if err != nil {
		return PageResponse[AttendanceCorrectionRequest]{}, err
	}
	items, err = c.filterCorrectionsByDecision(ctx, account, decision, items)
	if err != nil {
		return PageResponse[AttendanceCorrectionRequest]{}, err
	}
	return utils.PageResponse(items, page), nil
}

// ApproveAttendanceCorrection approves a correction and writes the accepted manual clock record.
func (c AttendanceService) ApproveAttendanceCorrection(ctx RequestContext, id string, input ReviewAttendanceCorrectionInput) (AttendanceCorrectionRequest, error) {
	return c.reviewAttendanceCorrection(ctx, strings.TrimSpace(id), correctionStatusApproved, input)
}

// RejectAttendanceCorrection rejects a correction without writing a clock record.
func (c AttendanceService) RejectAttendanceCorrection(ctx RequestContext, id string, input ReviewAttendanceCorrectionInput) (AttendanceCorrectionRequest, error) {
	return c.reviewAttendanceCorrection(ctx, strings.TrimSpace(id), correctionStatusRejected, input)
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

func (c AttendanceService) requireAttendanceAuthz(ctx RequestContext, resource ResourceType, action Action, resourceID string) (Account, CheckResult, error) {
	return c.Service.requireServiceAuthz(ctx, AppAttendance, resource, action, resourceID)
}

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

// attendancePolicyTimeOptions returns half-hour options used by attendance policy forms.
func attendancePolicyTimeOptions() []string {
	options := make([]string, 0, 48)
	for hour := 0; hour < 24; hour++ {
		options = append(options, twoDigit(hour)+":00", twoDigit(hour)+":30")
	}
	return options
}

// attendancePolicyCycleStartOptions returns valid monthly cycle start days.
func attendancePolicyCycleStartOptions() []string {
	options := make([]string, 0, 28)
	for day := 1; day <= 28; day++ {
		options = append(options, strconv.Itoa(day)+" 日")
	}
	return options
}

// attendancePolicyCycleEndOptions returns same-month and next-month cycle end options.
func attendancePolicyCycleEndOptions() []string {
	options := make([]string, 0, 58)
	for day := 1; day <= 30; day++ {
		options = append(options, "本月 "+strconv.Itoa(day)+" 日")
	}
	options = append(options, "本月 月底（最後一日）")
	for day := 1; day <= 28; day++ {
		options = append(options, "次月 "+strconv.Itoa(day)+" 日")
	}
	return options
}

// attendancePolicyLeaveTypes returns the default leave-type catalog.
func attendancePolicyLeaveTypes() []AttendanceLeaveType {
	return []AttendanceLeaveType{
		{Code: "病", Name: "全薪病假", Quota: "30 天 / 年", Rule: "無累計", Proof: "3 天以上需診斷證明"},
		{Code: "彈", Name: "彈性休假", Quota: "依公司政策", Rule: "無累計", Proof: "—"},
		{Code: "事", Name: "事假", Quota: "14 天 / 年", Rule: "無累計（不支薪）", Proof: "—"},
		{Code: "照", Name: "家庭照顧假", Quota: "7 天 / 年", Rule: "併入事假計算", Proof: "得要求相關證明"},
		{Code: "半", Name: "半薪病假", Quota: "30 天 / 年", Rule: "無累計", Proof: "診斷證明"},
		{Code: "理", Name: "生理假", Quota: "每月 1 日", Rule: "全年逾 3 日併入病假", Proof: "—"},
		{Code: "婚", Name: "婚假", Quota: "8 天", Rule: "登記後 3 個月內請畢", Proof: "結婚證明"},
		{Code: "產", Name: "八週產假", Quota: "56 天", Rule: "一次請足", Proof: "醫療證明"},
		{Code: "陪", Name: "陪產假", Quota: "7 天", Rule: "分娩前後 15 日內", Proof: "出生證明"},
		{Code: "喪", Name: "喪假", Quota: "3 - 8 天", Rule: "依親等決定天數", Proof: "訃聞或證明"},
		{Code: "公", Name: "公假", Quota: "不限天數", Rule: "需主管核可", Proof: "政府傳票或公文"},
		{Code: "檢", Name: "產檢假", Quota: "7 天", Rule: "妊娠期間", Proof: "產檢證明"},
		{Code: "補", Name: "補休假", Quota: "依加班時數", Rule: "期限內請畢", Proof: "加班紀錄"},
		{Code: "特", Name: "特休假", Quota: "依年資 3 - 30 天", Rule: "滿一年起算，可遞延一年", Proof: "—"},
	}
}

// twoDigit formats a small integer with a leading zero.
func twoDigit(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func (c AttendanceService) filterShiftAssignmentsByDecision(ctx RequestContext, account Account, decision CheckResult, items []AttendanceShiftAssignment) ([]AttendanceShiftAssignment, error) {
	allowed, all, err := c.attendanceEmployeeScope(ctx, account, decision)
	if err != nil {
		return nil, err
	}
	if all {
		return items, nil
	}
	out := make([]AttendanceShiftAssignment, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.EmployeeID]; ok {
			out = append(out, item)
		}
	}
	return out, nil
}

func (c AttendanceService) filterClockRecordsByDecision(ctx RequestContext, account Account, decision CheckResult, items []AttendanceClockRecord) ([]AttendanceClockRecord, error) {
	allowed, all, err := c.attendanceEmployeeScope(ctx, account, decision)
	if err != nil {
		return nil, err
	}
	if all {
		return items, nil
	}
	out := make([]AttendanceClockRecord, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.EmployeeID]; ok {
			out = append(out, item)
		}
	}
	return out, nil
}

func (c AttendanceService) filterCorrectionsByDecision(ctx RequestContext, account Account, decision CheckResult, items []AttendanceCorrectionRequest) ([]AttendanceCorrectionRequest, error) {
	allowed, all, err := c.attendanceEmployeeScope(ctx, account, decision)
	if err != nil {
		return nil, err
	}
	if all {
		return items, nil
	}
	out := make([]AttendanceCorrectionRequest, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.EmployeeID]; ok {
			out = append(out, item)
		}
	}
	return out, nil
}

func normalizeAttendanceStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", attendanceStatusActive:
		return attendanceStatusActive
	case attendanceStatusInactive:
		return attendanceStatusInactive
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func validateWorksiteInput(name string, latitude, longitude float64, radiusMeters int, status string) error {
	if strings.TrimSpace(name) == "" {
		return BadRequest("name is required")
	}
	if err := validateCoordinates(latitude, longitude); err != nil {
		return err
	}
	if radiusMeters <= 0 {
		return BadRequest("radius_meters must be greater than zero")
	}
	switch status {
	case attendanceStatusActive, attendanceStatusInactive:
		return nil
	default:
		return BadRequest("status must be active or inactive")
	}
}

func validateShiftInput(name, clockInStart, clockInEnd, clockOutStart, clockOutEnd string, lateGraceMinutes, earlyLeaveGraceMinutes int, status string) error {
	if strings.TrimSpace(name) == "" {
		return BadRequest("name is required")
	}
	inStart, err := parseClockWindowTime(clockInStart, "clock_in_start")
	if err != nil {
		return err
	}
	inEnd, err := parseClockWindowTime(clockInEnd, "clock_in_end")
	if err != nil {
		return err
	}
	outStart, err := parseClockWindowTime(clockOutStart, "clock_out_start")
	if err != nil {
		return err
	}
	outEnd, err := parseClockWindowTime(clockOutEnd, "clock_out_end")
	if err != nil {
		return err
	}
	if inEnd.Before(inStart) {
		return BadRequest("clock_in_end must not be before clock_in_start")
	}
	if outEnd.Before(outStart) {
		return BadRequest("clock_out_end must not be before clock_out_start")
	}
	if outStart.Before(inStart) {
		return BadRequest("clock_out_start must not be before clock_in_start")
	}
	if lateGraceMinutes < 0 || earlyLeaveGraceMinutes < 0 {
		return BadRequest("grace minutes must be greater than or equal to zero")
	}
	switch status {
	case attendanceStatusActive, attendanceStatusInactive:
		return nil
	default:
		return BadRequest("status must be active or inactive")
	}
}

func parseClockWindowTime(value, field string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, BadRequest(field + " is required")
	}
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, BadRequest(field + " must be HH:MM")
	}
	return parsed, nil
}

func optionalAttendanceDateTime(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := utils.ParseDateTime(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func normalizeClockDirection(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case clockDirectionIn:
		return clockDirectionIn, nil
	case clockDirectionOut:
		return clockDirectionOut, nil
	default:
		return "", BadRequest("direction must be clock_in or clock_out")
	}
}

func validateCoordinates(latitude, longitude float64) error {
	if latitude < -90 || latitude > 90 {
		return BadRequest("latitude must be between -90 and 90")
	}
	if longitude < -180 || longitude > 180 {
		return BadRequest("longitude must be between -180 and 180")
	}
	return nil
}

func attendanceWorkDate(at time.Time) string {
	return at.UTC().Format("2006-01-02")
}

func (c AttendanceService) attendanceAssignmentBundle(ctx RequestContext, employeeID string, at time.Time) (AttendanceShiftAssignment, AttendanceShift, AttendanceWorksite, error) {
	assignment, ok, err := c.store.FindEffectiveAttendanceShiftAssignment(goContext(ctx), ctx.TenantID, employeeID, at)
	if err != nil {
		return AttendanceShiftAssignment{}, AttendanceShift{}, AttendanceWorksite{}, err
	}
	if !ok {
		return AttendanceShiftAssignment{}, AttendanceShift{}, AttendanceWorksite{}, BadRequest("attendance shift assignment is required")
	}
	shift, ok, err := c.store.GetAttendanceShift(goContext(ctx), ctx.TenantID, assignment.ShiftID)
	if err != nil {
		return AttendanceShiftAssignment{}, AttendanceShift{}, AttendanceWorksite{}, err
	}
	if !ok {
		return AttendanceShiftAssignment{}, AttendanceShift{}, AttendanceWorksite{}, BadRequest("attendance shift is required")
	}
	worksite, ok, err := c.store.GetAttendanceWorksite(goContext(ctx), ctx.TenantID, assignment.WorksiteID)
	if err != nil {
		return AttendanceShiftAssignment{}, AttendanceShift{}, AttendanceWorksite{}, err
	}
	if !ok {
		return AttendanceShiftAssignment{}, AttendanceShift{}, AttendanceWorksite{}, BadRequest("attendance worksite is required")
	}
	if !strings.EqualFold(assignment.Status, attendanceStatusActive) || !strings.EqualFold(shift.Status, attendanceStatusActive) || !strings.EqualFold(worksite.Status, attendanceStatusActive) {
		return AttendanceShiftAssignment{}, AttendanceShift{}, AttendanceWorksite{}, BadRequest("attendance assignment, shift and worksite must be active")
	}
	return assignment, shift, worksite, nil
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusMeters = 6371000.0
	toRad := func(deg float64) float64 { return deg * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	rLat1 := toRad(lat1)
	rLat2 := toRad(lat2)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rLat1)*math.Cos(rLat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadiusMeters * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func nextClockAction(clockIn, clockOut *AttendanceClockRecord) string {
	if clockIn == nil {
		return clockDirectionIn
	}
	if clockOut == nil {
		return clockDirectionOut
	}
	return "complete"
}

func normalizeClockRecordQuery(query AttendanceClockRecordQuery) AttendanceClockRecordQuery {
	query.EmployeeID = strings.TrimSpace(query.EmployeeID)
	query.FromDate = normalizeAttendanceDateQuery(query.FromDate)
	query.ToDate = normalizeAttendanceDateQuery(query.ToDate)
	if direction, err := normalizeClockDirection(query.Direction); err == nil {
		query.Direction = direction
	} else {
		query.Direction = ""
	}
	query.RecordStatus = strings.ToLower(strings.TrimSpace(query.RecordStatus))
	query.Source = strings.ToLower(strings.TrimSpace(query.Source))
	return query
}

func normalizeCorrectionQuery(query AttendanceCorrectionQuery) AttendanceCorrectionQuery {
	query.EmployeeID = strings.TrimSpace(query.EmployeeID)
	query.FromDate = normalizeAttendanceDateQuery(query.FromDate)
	query.ToDate = normalizeAttendanceDateQuery(query.ToDate)
	query.Status = strings.ToLower(strings.TrimSpace(query.Status))
	if direction, err := normalizeClockDirection(query.Direction); err == nil {
		query.Direction = direction
	} else {
		query.Direction = ""
	}
	return query
}

func normalizeAttendanceDateQuery(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := utils.ParseDate(value); err == nil {
		return attendanceWorkDate(parsed)
	}
	return value
}

func (c AttendanceService) reviewAttendanceCorrection(ctx RequestContext, id, nextStatus string, input ReviewAttendanceCorrectionInput) (AttendanceCorrectionRequest, error) {
	if id == "" {
		return AttendanceCorrectionRequest{}, BadRequest("id is required")
	}
	existing, ok, err := c.store.GetAttendanceCorrectionRequest(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	if !ok {
		return AttendanceCorrectionRequest{}, NotFound("attendance correction", id)
	}
	action := ActionUpdate
	event := "attendance.correction.reject"
	if nextStatus == correctionStatusApproved {
		action = ActionApprove
		event = "attendance.correction.approve"
	}
	account, decision, authzAudit, err := c.Authorize(ctx, CheckRequest{
		ApplicationCode:  AppAttendance,
		ResourceType:     ResourceAttendanceCorrection,
		ResourceID:       id,
		Target:           existing.EmployeeID,
		TargetEmployeeID: existing.EmployeeID,
		Action:           action,
	}, AuditTarget{Event: event, Resource: string(ResourceAttendanceCorrection), Target: id})
	if err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	if err := c.ensureAttendanceEmployeeAllowed(ctx, account, decision, existing.EmployeeID); err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	var correction AttendanceCorrectionRequest
	if err := c.withTransaction(ctx, func(tx AttendanceService) error {
		current, ok, err := tx.store.GetAttendanceCorrectionRequest(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("attendance correction", id)
		}
		if !strings.EqualFold(current.Status, correctionStatusPending) {
			return BadRequest("correction request is not pending")
		}
		now := tx.Now()
		reviewReason := strings.TrimSpace(input.Reason)
		current.Status = nextStatus
		current.ReviewedByAccountID = account.ID
		current.ReviewReason = reviewReason
		current.ReviewedAt = &now
		current.UpdatedAt = now
		if nextStatus == correctionStatusApproved {
			assignment, shift, worksite, err := tx.attendanceAssignmentBundle(ctx, current.EmployeeID, current.RequestedClockedAt)
			if err != nil {
				return err
			}
			if _, exists, err := tx.store.GetAcceptedAttendanceClockRecord(goContext(ctx), ctx.TenantID, current.EmployeeID, current.WorkDate, current.Direction); err != nil {
				return err
			} else if exists {
				return Conflict("accepted clock record already exists")
			}
			record := AttendanceClockRecord{
				ID:                  utils.NewID("acr"),
				TenantID:            ctx.TenantID,
				EmployeeID:          current.EmployeeID,
				ShiftAssignmentID:   assignment.ID,
				ShiftID:             shift.ID,
				WorksiteID:          worksite.ID,
				WorkDate:            current.WorkDate,
				Direction:           current.Direction,
				ClockedAt:           current.RequestedClockedAt,
				Latitude:            worksite.Latitude,
				Longitude:           worksite.Longitude,
				AccuracyMeters:      0,
				DistanceMeters:      0,
				RecordStatus:        clockRecordStatusAccepted,
				Source:              clockSourceManualCorrection,
				CorrectionRequestID: current.ID,
				CreatedAt:           now,
			}
			if err := tx.store.UpsertAttendanceClockRecord(goContext(ctx), record); err != nil {
				return err
			}
			current.ClockRecordID = record.ID
		}
		if current.FormInstanceID != "" {
			instance, ok, err := tx.store.GetFormInstance(goContext(ctx), ctx.TenantID, current.FormInstanceID)
			if err != nil {
				return err
			}
			if ok {
				instance.Status = nextStatus
				instance.ApprovedBy = account.ID
				instance.UpdatedAt = now
				if err := tx.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
					return err
				}
			}
		}
		if err := tx.store.UpsertAttendanceCorrectionRequest(goContext(ctx), current); err != nil {
			return err
		}
		if err := tx.audit(ctx, event, string(ResourceAttendanceCorrection), current.ID, string(SeverityHigh), map[string]any{
			"employee_id": current.EmployeeID,
			"direction":   current.Direction,
			"status":      current.Status,
		}); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		correction = current
		return nil
	}); err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	return correction, nil
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
