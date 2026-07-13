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
	status := AttendanceClockStatus{EmployeeID: account.EmployeeID, WorkDate: workDate}
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
	// 當日尚無紀錄時，保留跨日下班後的前一工作日狀態。
	if status.ClockIn == nil && status.ClockOut == nil {
		previousWorkDate := attendanceWorkDate(now.Add(-24 * time.Hour))
		if record, ok, lookupErr := c.store.GetAcceptedAttendanceClockRecord(goContext(ctx), ctx.TenantID, account.EmployeeID, previousWorkDate, clockDirectionIn); lookupErr != nil {
			return AttendanceClockStatus{}, lookupErr
		} else if ok {
			status.WorkDate = previousWorkDate
			status.ClockIn = &record
		}
		if record, ok, lookupErr := c.store.GetAcceptedAttendanceClockRecord(goContext(ctx), ctx.TenantID, account.EmployeeID, previousWorkDate, clockDirectionOut); lookupErr != nil {
			return AttendanceClockStatus{}, lookupErr
		} else if ok {
			status.ClockOut = &record
		}
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
	policy, err := c.loadAttendancePolicyResponse(ctx)
	if err != nil {
		return AttendanceClockRecord{}, err
	}
	assignment, shift, hasShift, err := c.optionalAttendanceShift(ctx, employeeID, now)
	if err != nil {
		return AttendanceClockRecord{}, err
	}
	var worksite AttendanceWorksite
	distance := 0.0
	if policy.WorkTime.RequireWorksite {
		worksite, distance, err = c.nearestActiveAttendanceWorksite(ctx, input.Latitude, input.Longitude)
		if err != nil {
			return AttendanceClockRecord{}, err
		}
	}
	workDate := attendanceWorkDate(now)
	if hasShift {
		workDate = attendanceWorkDateForClock(direction, shift, now)
	}
	recordStatus := clockRecordStatusAccepted
	rejectionReason := ""
	clockInRecord, hasClockIn, err := c.store.GetAcceptedAttendanceClockRecord(goContext(ctx), ctx.TenantID, employeeID, workDate, clockDirectionIn)
	if err != nil {
		return AttendanceClockRecord{}, err
	}
	// 跨日下班無當日上班卡時，歸屬到前一個有效上班日。
	if direction == clockDirectionOut && !hasClockIn {
		previousWorkDate := attendanceWorkDate(now.Add(-24 * time.Hour))
		if previousClockIn, previousHasClockIn, lookupErr := c.store.GetAcceptedAttendanceClockRecord(goContext(ctx), ctx.TenantID, employeeID, previousWorkDate, clockDirectionIn); lookupErr != nil {
			return AttendanceClockRecord{}, lookupErr
		} else if previousHasClockIn {
			workDate = previousWorkDate
			clockInRecord = previousClockIn
			hasClockIn = true
		}
	}
	_, hasClockOut, err := c.store.GetAcceptedAttendanceClockRecord(goContext(ctx), ctx.TenantID, employeeID, workDate, clockDirectionOut)
	if err != nil {
		return AttendanceClockRecord{}, err
	}
	if (direction == clockDirectionIn && hasClockIn) || (direction == clockDirectionOut && hasClockOut) {
		return AttendanceClockRecord{}, Conflict("accepted clock record already exists")
	}
	rejectionReason = clockRejectionReason(direction, worksite, input.AccuracyMeters, distance, hasClockIn, hasClockOut, policy.WorkTime.RequireWorksite)
	if rejectionReason == "" {
		outsideWindow, windowErr := clockOutsideConfiguredWindow(direction, shift, hasShift, policy.WorkTime, now)
		if windowErr != nil {
			return AttendanceClockRecord{}, windowErr
		}
		if outsideWindow {
			rejectionReason = clockRejectionOutsideWindow
		} else if policy.WorkTime.ClockMode == clockModeFlexible && clockHasInsufficientWorkHours(direction, clockInRecord, hasClockIn, now, policy.WorkTime) {
			rejectionReason = clockRejectionInsufficientWorkHours
		}
	}
	if rejectionReason != "" {
		switch {
		case clockTimeAnomalyCountsAsPunch(direction, policy.WorkTime.ClockMode, rejectionReason):
			// 固定异常卡及弹性异常上班卡仍推进当天考勤状态。
		case clockRetryableFlexibleClockOut(direction, policy.WorkTime.ClockMode, rejectionReason):
			recordStatus = clockRecordStatusAbnormal
		default:
			recordStatus = clockRecordStatusRejected
		}
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
			return AttendanceClockRecord{}, Conflict("accepted clock record already exists")
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

// filterShiftAssignmentsByDecision 依員工資料範圍篩選排班。
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
func clockRejectionReason(direction string, worksite AttendanceWorksite, accuracyMeters, distanceMeters float64, hasClockIn, hasClockOut, requireWorksite bool) string {
	if direction == clockDirectionOut && !hasClockIn {
		return clockRejectionInvalidSequence
	}
	if direction == clockDirectionIn && hasClockOut {
		return clockRejectionInvalidSequence
	}
	if requireWorksite && distanceMeters > float64(worksite.RadiusMeters) {
		return clockRejectionOutsideGeofence
	}
	if requireWorksite && accuracyMeters > clockMaxAccuracyMeters {
		return clockRejectionLowAccuracy
	}
	return ""
}

// clockHasInsufficientWorkHours 以两次打卡的实际间隔校验弹性工时是否达标。
func clockHasInsufficientWorkHours(direction string, clockIn AttendanceClockRecord, hasClockIn bool, at time.Time, workTime AttendancePolicyWorkTime) bool {
	if direction != clockDirectionOut || !hasClockIn {
		return false
	}
	requiredHours := standardDayHours(workTime)
	actualHours := at.Sub(clockIn.ClockedAt).Hours()
	return actualHours < requiredHours
}

// clockOutsideConfiguredWindow 判斷固定制遲到/早退或彈性制超出設定範圍的時間異常。
func clockOutsideConfiguredWindow(direction string, shift AttendanceShift, hasShift bool, workTime AttendancePolicyWorkTime, at time.Time) (bool, error) {
	if workTime.ClockMode == clockModeFlexible {
		earliest := parseHHMMMinutes(workTime.FlexibleClockInEarliest)
		latest := parseHHMMMinutes(workTime.FlexibleClockOutLatest)
		if earliest < 0 || latest < 0 {
			return false, BadRequest("flexible clock range must use HH:MM")
		}
		minute := clockMinuteOfDay(at)
		if direction == clockDirectionIn {
			return minute < earliest, nil
		}
		return minute > latest, nil
	}
	if hasShift {
		return clockOutsideFixedShiftBoundary(direction, shift, at)
	}
	boundaryValue := workTime.StandardStart
	if direction == clockDirectionOut {
		boundaryValue = workTime.StandardEnd
	}
	boundary := parseHHMMMinutes(boundaryValue)
	if boundary < 0 {
		return false, BadRequest("standard time must use HH:MM")
	}
	if direction == clockDirectionIn {
		return clockMinuteOfDay(at) > boundary, nil
	}
	return clockMinuteOfDay(at) < boundary, nil
}

// clockOutsideFixedShiftBoundary 只把晚於上班截止或早於下班起始的固定班次打卡標成異常。
func clockOutsideFixedShiftBoundary(direction string, shift AttendanceShift, at time.Time) (bool, error) {
	start, end, err := clockWindowMinutes(direction, shift)
	if err != nil {
		return false, err
	}
	minute := clockMinuteOfDay(at)
	if end < start {
		end += 24 * 60
		if minute < start {
			minute += 24 * 60
		}
	}
	if direction == clockDirectionIn {
		return minute > end+shift.LateGraceMinutes, nil
	}
	return minute < start-shift.EarlyLeaveGraceMinutes, nil
}

// clockTimeAnomalyCountsAsPunch 保留可推进考勤状态的异常卡，并让弹性异常下班继续可重打。
func clockTimeAnomalyCountsAsPunch(direction, clockMode, rejectionReason string) bool {
	if rejectionReason != clockRejectionOutsideWindow {
		return false
	}
	return direction == clockDirectionIn || clockMode == clockModeFixed
}

// clockRetryableFlexibleClockOut 把弹性制时间异常下班记录为异常尝试，不占用有效下班卡。
func clockRetryableFlexibleClockOut(direction, clockMode, rejectionReason string) bool {
	if direction != clockDirectionOut || clockMode != clockModeFlexible {
		return false
	}
	return rejectionReason == clockRejectionOutsideWindow || rejectionReason == clockRejectionInsufficientWorkHours
}

// clockWindowMinutes 取得指定打卡方向的班次起讫分钟。
func clockWindowMinutes(direction string, shift AttendanceShift) (int, int, error) {
	startField, endField := "clock_in_start", "clock_in_end"
	startValue, endValue := shift.ClockInStart, shift.ClockInEnd
	if direction == clockDirectionOut {
		startField, endField = "clock_out_start", "clock_out_end"
		startValue, endValue = shift.ClockOutStart, shift.ClockOutEnd
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

// parseClockWindowMinute 将 HH:MM 班次时间转换为当天分钟数。
func parseClockWindowMinute(value, field string) (int, error) {
	parsed, err := parseClockWindowTime(value, field)
	if err != nil {
		return 0, err
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

// clockMinuteOfDay 取得业务时区的当天分钟数。
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

// attendanceWorkDateForClock 让跨午夜班次的下班卡归属前一工作日。
func attendanceWorkDateForClock(direction string, shift AttendanceShift, at time.Time) string {
	if direction == clockDirectionOut && clockOutBelongsToPreviousWorkDate(shift, at) {
		return at.In(attendanceClockLocation).AddDate(0, 0, -1).Format(time.DateOnly)
	}
	return attendanceWorkDate(at)
}

// clockOutBelongsToPreviousWorkDate 判断跨午夜班次的下班卡日期归属。
func clockOutBelongsToPreviousWorkDate(shift AttendanceShift, at time.Time) bool {
	inStart, _, err := clockWindowMinutes(clockDirectionIn, shift)
	if err != nil {
		return false
	}
	outStart, outEnd, err := clockWindowMinutes(clockDirectionOut, shift)
	if err != nil || outStart >= inStart {
		return false
	}
	minute := clockMinuteOfDay(at)
	if outStart <= outEnd {
		return minute <= outEnd
	}
	return minute >= outStart || minute <= outEnd
}

// optionalAttendanceShift 读取有效排班；未排班时回退到全局考勤政策。
func (c AttendanceService) optionalAttendanceShift(ctx RequestContext, employeeID string, at time.Time) (AttendanceShiftAssignment, AttendanceShift, bool, error) {
	assignment, ok, err := c.store.FindEffectiveAttendanceShiftAssignment(goContext(ctx), ctx.TenantID, employeeID, at)
	if err != nil || !ok {
		return AttendanceShiftAssignment{}, AttendanceShift{}, false, err
	}
	shift, ok, err := c.store.GetAttendanceShift(goContext(ctx), ctx.TenantID, assignment.ShiftID)
	if err != nil {
		return AttendanceShiftAssignment{}, AttendanceShift{}, false, err
	}
	if !ok || !strings.EqualFold(shift.Status, attendanceStatusActive) {
		return AttendanceShiftAssignment{}, AttendanceShift{}, false, BadRequest("attendance shift is required")
	}
	return assignment, shift, true, nil
}

// nearestActiveAttendanceWorksite 選擇最接近打卡定位的啟用辦公地點。
func (c AttendanceService) nearestActiveAttendanceWorksite(ctx RequestContext, latitude, longitude float64) (AttendanceWorksite, float64, error) {
	items, err := c.store.ListAttendanceWorksites(goContext(ctx), ctx.TenantID)
	if err != nil {
		return AttendanceWorksite{}, 0, err
	}
	var selected AttendanceWorksite
	selectedDistance := math.MaxFloat64
	for _, item := range items {
		if !strings.EqualFold(item.Status, attendanceStatusActive) {
			continue
		}
		distance := haversineMeters(latitude, longitude, item.Latitude, item.Longitude)
		if distance < selectedDistance {
			selected, selectedDistance = item, distance
		}
	}
	if selected.ID == "" {
		return AttendanceWorksite{}, 0, BadRequest("attendance worksite is required")
	}
	return selected, selectedDistance, nil
}

// firstActiveAttendanceWorksite 取得補卡所需的一個啟用辦公地點。
func (c AttendanceService) firstActiveAttendanceWorksite(ctx RequestContext) (AttendanceWorksite, error) {
	items, err := c.store.ListAttendanceWorksites(goContext(ctx), ctx.TenantID)
	if err != nil {
		return AttendanceWorksite{}, err
	}
	for _, item := range items {
		if strings.EqualFold(item.Status, attendanceStatusActive) {
			return item, nil
		}
	}
	return AttendanceWorksite{}, BadRequest("attendance worksite is required")
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
			worksite, err := tx.firstActiveAttendanceWorksite(ctx)
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
