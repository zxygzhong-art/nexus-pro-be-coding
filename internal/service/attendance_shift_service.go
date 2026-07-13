package service

import (
	"strings"
	"time"

	"nexus-pro-be/internal/utils"
)

// ListAttendanceWorksitePage 列出考勤工作地點分頁的服務流程。
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

// CreateAttendanceWorksite 建立考勤工作地點的服務流程。
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

// UpdateAttendanceWorksite 更新考勤工作地點的服務流程。
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

// ListAttendanceShiftPage 列出考勤班別分頁的服務流程。
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

// CreateAttendanceShift 建立考勤班別的服務流程。
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

// UpdateAttendanceShift 更新考勤班別的服務流程。
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

// ListAttendanceShiftAssignmentPage 列出可選的員工班別指派。
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

// CreateAttendanceShiftAssignment 建立可選的員工班別指派。
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
	if err := c.ensureNoOverlappingActiveShiftAssignment(ctx, employeeID, effectiveFrom, effectiveTo, status); err != nil {
		return AttendanceShiftAssignment{}, err
	}
	now := c.Now()
	item := AttendanceShiftAssignment{ID: utils.NewID("asa"), TenantID: ctx.TenantID, EmployeeID: employeeID, ShiftID: shift.ID, WorksiteID: worksite.ID, EffectiveFrom: effectiveFrom, EffectiveTo: effectiveTo, Status: status, CreatedAt: now, UpdatedAt: now}
	if err := c.store.UpsertAttendanceShiftAssignment(goContext(ctx), item); err != nil {
		return AttendanceShiftAssignment{}, err
	}
	if err := c.audit(ctx, "attendance.shift_assignment.create", string(ResourceAttendanceShiftAssignment), item.ID, string(SeverityMedium), map[string]any{"employee_id": item.EmployeeID}); err != nil {
		return AttendanceShiftAssignment{}, err
	}
	return item, nil
}

// ensureNoOverlappingActiveShiftAssignment 防止啟用中的排班區間重疊。
func (c AttendanceService) ensureNoOverlappingActiveShiftAssignment(ctx RequestContext, employeeID string, effectiveFrom time.Time, effectiveTo *time.Time, status string) error {
	if !strings.EqualFold(status, attendanceStatusActive) {
		return nil
	}
	items, err := c.store.ListAttendanceShiftAssignments(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.EmployeeID == employeeID && strings.EqualFold(item.Status, attendanceStatusActive) && attendanceAssignmentRangesOverlap(effectiveFrom, effectiveTo, item.EffectiveFrom, item.EffectiveTo) {
			return BadRequest("active shift assignment overlaps existing assignment")
		}
	}
	return nil
}

// attendanceAssignmentRangesOverlap 判斷兩個排班生效區間是否重疊。
func attendanceAssignmentRangesOverlap(leftFrom time.Time, leftTo *time.Time, rightFrom time.Time, rightTo *time.Time) bool {
	if rightTo != nil && rightTo.Before(leftFrom) {
		return false
	}
	if leftTo != nil && leftTo.Before(rightFrom) {
		return false
	}
	return true
}

// normalizeAttendanceStatus 正規化考勤狀態。
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

// validateWorksiteInput 驗證工作地點輸入。
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

// validateShiftInput 驗證班別輸入。
func validateShiftInput(name, clockInStart, clockInEnd, clockOutStart, clockOutEnd string, lateGraceMinutes, earlyLeaveGraceMinutes int, status string) error {
	if strings.TrimSpace(name) == "" {
		return BadRequest("name is required")
	}
	if _, err := parseClockWindowTime(clockInStart, "clock_in_start"); err != nil {
		return err
	}
	if _, err := parseClockWindowTime(clockInEnd, "clock_in_end"); err != nil {
		return err
	}
	if _, err := parseClockWindowTime(clockOutStart, "clock_out_start"); err != nil {
		return err
	}
	if _, err := parseClockWindowTime(clockOutEnd, "clock_out_end"); err != nil {
		return err
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

// parseClockWindowTime 解析打卡 window 時間。
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

// optionalAttendanceDateTime 處理可選考勤日期時間。
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
