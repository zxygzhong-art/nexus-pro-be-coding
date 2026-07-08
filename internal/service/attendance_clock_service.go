package service

import (
	"math"
	"strings"
	"time"

	"nexus-pro-be/internal/utils"
)

// AttendanceClockStatus 處理考勤打卡狀態的服務流程。
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
		workDate = attendanceActiveWorkDate(shift, now)
		status.WorkDate = workDate
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

// CreateAttendanceClockRecord 建立考勤打卡 record 的服務流程。
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
	if account.EmployeeID == "" || employeeID != account.EmployeeID {
		return AttendanceClockRecord{}, Forbidden("geofence clock records can only be created for the current employee")
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
	workDate := attendanceWorkDateForClock(direction, shift, now)
	distance := haversineMeters(input.Latitude, input.Longitude, worksite.Latitude, worksite.Longitude)
	recordStatus := clockRecordStatusAccepted
	rejectionReason := ""
	_, hasClockIn, err := c.store.GetAcceptedAttendanceClockRecord(goContext(ctx), ctx.TenantID, employeeID, workDate, clockDirectionIn)
	if err != nil {
		return AttendanceClockRecord{}, err
	}
	_, hasClockOut, err := c.store.GetAcceptedAttendanceClockRecord(goContext(ctx), ctx.TenantID, employeeID, workDate, clockDirectionOut)
	if err != nil {
		return AttendanceClockRecord{}, err
	}
	if reason, err := clockRejectionReason(direction, shift, worksite, now, input.AccuracyMeters, distance, hasClockIn, hasClockOut); err != nil {
		return AttendanceClockRecord{}, err
	} else if reason != "" {
		recordStatus = clockRecordStatusRejected
		rejectionReason = reason
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
		if acceptedClockConflict(err) && record.RecordStatus == clockRecordStatusAccepted {
			record.ID = utils.NewID("acr")
			record.RecordStatus = clockRecordStatusRejected
			record.RejectionReason = clockRejectionDuplicate
			if retryErr := c.store.UpsertAttendanceClockRecord(goContext(ctx), record); retryErr == nil {
				err = nil
			} else {
				err = retryErr
			}
		}
		if err != nil {
			return AttendanceClockRecord{}, err
		}
	}
	if err := c.audit(ctx, "attendance.clock_record.create", string(ResourceAttendanceClock), record.ID, string(SeverityMedium), map[string]any{
		"employee_id":      record.EmployeeID,
		"direction":        record.Direction,
		"record_status":    record.RecordStatus,
		"rejection_reason": record.RejectionReason,
	}); err != nil {
		return AttendanceClockRecord{}, err
	}
	return applyAttendanceClockFieldPolicy(record, decision.FieldPolicies), nil
}

// ListAttendanceClockRecordPage 列出考勤打卡 record 分頁的服務流程。
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
	items = applyAttendanceClockFieldPolicies(items, decision.FieldPolicies)
	return utils.PageResponse(items, page), nil
}

// CreateAttendanceCorrection 建立考勤 correction 的服務流程。
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
		template, ok, err := tx.store.GetFormTemplateByKey(goContext(ctx), ctx.TenantID, "punch-fix")
		if err != nil {
			return err
		}
		if !ok {
			template = FormTemplate{
				ID:        utils.NewID("ft"),
				TenantID:  ctx.TenantID,
				Key:       "punch-fix",
				Name:      "HR-005 補卡單",
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

// ListAttendanceCorrectionPage 列出考勤 correction 分頁的服務流程。
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

// ApproveAttendanceCorrection 核准考勤 correction 的服務流程。
func (c AttendanceService) ApproveAttendanceCorrection(ctx RequestContext, id string, input ReviewAttendanceCorrectionInput) (AttendanceCorrectionRequest, error) {
	return c.reviewAttendanceCorrection(ctx, strings.TrimSpace(id), correctionStatusApproved, input)
}

// RejectAttendanceCorrection 駁回考勤 correction 的服務流程。
func (c AttendanceService) RejectAttendanceCorrection(ctx RequestContext, id string, input ReviewAttendanceCorrectionInput) (AttendanceCorrectionRequest, error) {
	return c.reviewAttendanceCorrection(ctx, strings.TrimSpace(id), correctionStatusRejected, input)
}

// filterShiftAssignmentsByDecision 處理篩選班別指派 by 決策的服務流程。
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

// filterClockRecordsByDecision 處理篩選打卡 records by 決策的服務流程。
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

// applyAttendanceClockFieldPolicies 處理 apply 考勤打卡欄位政策。
func applyAttendanceClockFieldPolicies(items []AttendanceClockRecord, policies map[string]string) []AttendanceClockRecord {
	if len(policies) == 0 {
		return items
	}
	out := make([]AttendanceClockRecord, 0, len(items))
	for _, item := range items {
		out = append(out, applyAttendanceClockFieldPolicy(item, policies))
	}
	return out
}

// applyAttendanceClockFieldPolicy 處理 apply 考勤打卡欄位政策。
func applyAttendanceClockFieldPolicy(item AttendanceClockRecord, policies map[string]string) AttendanceClockRecord {
	for field, effect := range policies {
		if effect != "mask" && effect != "hide" && effect != "deny" {
			continue
		}
		switch field {
		case "latitude":
			item.Latitude = 0
		case "longitude":
			item.Longitude = 0
		case "accuracy_meters":
			item.AccuracyMeters = 0
		case "distance_meters":
			item.DistanceMeters = 0
		case "device_id":
			item.DeviceID = ""
		case "device_info":
			item.DeviceInfo = nil
		case "location_source":
			item.DeviceInfo = removeClockDeviceInfoField(item.DeviceInfo, "location_source")
		}
	}
	return item
}

// removeClockDeviceInfoField 移除打卡 device info 欄位。
func removeClockDeviceInfoField(values map[string]any, field string) map[string]any {
	if len(values) == 0 {
		return values
	}
	if _, ok := values[field]; !ok {
		return values
	}
	out := utils.CopyStringMap(values)
	delete(out, field)
	return out
}

// filterCorrectionsByDecision 處理篩選 corrections by 決策的服務流程。
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

// normalizeClockDirection 正規化打卡 direction。
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

// clockRejectionReason 處理打卡 rejection reason。
func clockRejectionReason(direction string, shift AttendanceShift, worksite AttendanceWorksite, at time.Time, accuracyMeters, distanceMeters float64, hasClockIn, hasClockOut bool) (string, error) {
	if (direction == clockDirectionIn && hasClockIn) || (direction == clockDirectionOut && hasClockOut) {
		return clockRejectionDuplicate, nil
	}
	if direction == clockDirectionOut && !hasClockIn {
		return clockRejectionInvalidSequence, nil
	}
	if direction == clockDirectionIn && hasClockOut {
		return clockRejectionInvalidSequence, nil
	}
	ok, err := clockWithinShiftWindow(direction, shift, at)
	if err != nil {
		return "", err
	}
	if !ok {
		return clockRejectionOutsideWindow, nil
	}
	if distanceMeters > float64(worksite.RadiusMeters) {
		return clockRejectionOutsideGeofence, nil
	}
	if accuracyMeters > clockMaxAccuracyMeters {
		return clockRejectionLowAccuracy, nil
	}
	return "", nil
}

// clockWithinShiftWindow 處理打卡 within 班別 window。
func clockWithinShiftWindow(direction string, shift AttendanceShift, at time.Time) (bool, error) {
	start, end, err := clockWindowMinutes(direction, shift)
	if err != nil {
		return false, err
	}
	minute := clockMinuteOfDay(at)
	if start <= end {
		return minute >= start && minute <= end, nil
	}
	return minute >= start || minute <= end, nil
}

// clockWindowMinutes 處理打卡 window minutes。
func clockWindowMinutes(direction string, shift AttendanceShift) (int, int, error) {
	startField := "clock_in_start"
	endField := "clock_in_end"
	startValue := shift.ClockInStart
	endValue := shift.ClockInEnd
	if direction == clockDirectionOut {
		startField = "clock_out_start"
		endField = "clock_out_end"
		startValue = shift.ClockOutStart
		endValue = shift.ClockOutEnd
	}
	start, err := parseClockWindowMinute(startValue, startField)
	if err != nil {
		return 0, 0, err
	}
	end, err := parseClockWindowMinute(endValue, endField)
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

// parseClockWindowMinute 解析打卡 window minute。
func parseClockWindowMinute(value, field string) (int, error) {
	parsed, err := parseClockWindowTime(value, field)
	if err != nil {
		return 0, err
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

// clockMinuteOfDay 處理打卡 minute of day。
func clockMinuteOfDay(at time.Time) int {
	local := at.In(attendanceClockLocation)
	return local.Hour()*60 + local.Minute()
}

// validateCoordinates 驗證 coordinates。
func validateCoordinates(latitude, longitude float64) error {
	if latitude < -90 || latitude > 90 {
		return BadRequest("latitude must be between -90 and 90")
	}
	if longitude < -180 || longitude > 180 {
		return BadRequest("longitude must be between -180 and 180")
	}
	return nil
}

// attendanceWorkDate 處理考勤 work 日期。
func attendanceWorkDate(at time.Time) string {
	return at.In(attendanceClockLocation).Format("2006-01-02")
}

// attendanceWorkDateForClock 處理考勤 work 日期 for 打卡。
func attendanceWorkDateForClock(direction string, shift AttendanceShift, at time.Time) string {
	if direction == clockDirectionOut && clockOutBelongsToPreviousWorkDate(shift, at) {
		return at.In(attendanceClockLocation).AddDate(0, 0, -1).Format(time.DateOnly)
	}
	return attendanceWorkDate(at)
}

// attendanceActiveWorkDate 處理考勤啟用中 work 日期。
func attendanceActiveWorkDate(shift AttendanceShift, at time.Time) string {
	if clockOutBelongsToPreviousWorkDate(shift, at) {
		return at.In(attendanceClockLocation).AddDate(0, 0, -1).Format(time.DateOnly)
	}
	return attendanceWorkDate(at)
}

// clockOutBelongsToPreviousWorkDate 處理打卡 out belongs to previous work 日期。
func clockOutBelongsToPreviousWorkDate(shift AttendanceShift, at time.Time) bool {
	inStart, _, err := clockWindowMinutes(clockDirectionIn, shift)
	if err != nil {
		return false
	}
	outStart, outEnd, err := clockWindowMinutes(clockDirectionOut, shift)
	if err != nil {
		return false
	}
	if outStart >= inStart {
		return false
	}
	minute := clockMinuteOfDay(at)
	if outStart <= outEnd {
		return minute <= outEnd
	}
	return minute >= outStart || minute <= outEnd
}

// attendanceAssignmentBundle 處理考勤指派 bundle 的服務流程。
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

// haversineMeters 處理 haversine meters。
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

// nextClockAction 處理 next 打卡 action。
func nextClockAction(clockIn, clockOut *AttendanceClockRecord) string {
	if clockIn == nil {
		return clockDirectionIn
	}
	if clockOut == nil {
		return clockDirectionOut
	}
	return "complete"
}

// acceptedClockConflict 處理 accepted 打卡衝突。
func acceptedClockConflict(err error) bool {
	appErr, ok := AsAppError(err)
	return ok && appErr.Code == "conflict" && strings.Contains(appErr.Message, "accepted clock record")
}

// normalizeClockRecordQuery 正規化打卡 record 查詢。
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

// normalizeAttendanceDailySummaryQuery 正規化日彙總查詢。
func normalizeAttendanceDailySummaryQuery(query AttendanceDailySummaryQuery) AttendanceDailySummaryQuery {
	query.EmployeeID = strings.TrimSpace(query.EmployeeID)
	query.FromDate = normalizeAttendanceDateQuery(query.FromDate)
	query.ToDate = normalizeAttendanceDateQuery(query.ToDate)
	query.Source = strings.ToLower(strings.TrimSpace(query.Source))
	return query
}

// normalizeCorrectionQuery 正規化correction 查詢。
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

// normalizeAttendanceDateQuery 正規化考勤日期查詢。
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

// reviewAttendanceCorrection 處理審核考勤 correction 的服務流程。
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
