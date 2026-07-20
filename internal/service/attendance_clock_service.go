package service

import (
	"math"
	"strings"
	"time"

	"nexus-pro-api/internal/utils"
)

// AttendanceClockStatus returns punch state together with the authoritative worksite policy.
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
	employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, account.EmployeeID)
	if err != nil {
		return AttendanceClockStatus{}, err
	}
	// A dangling legacy employee link keeps the read endpoint available, but it must
	// never advertise a clock mutation that CreateAttendanceClockRecord will reject.
	activeEmployment := ok && attendanceEmployeeAllowsActiveOperations(employee)
	now := c.Now()
	workDate := attendanceWorkDate(now)
	projection, err := c.loadAttendanceDayProjection(ctx, account.EmployeeID, workDate, now)
	if err != nil {
		return AttendanceClockStatus{}, err
	}
	// 只有實際跨午夜班次仍在下班窗口內時，狀態才沿用前一工作日。
	if _, shift, hasShift, shiftErr := c.optionalAttendanceShift(ctx, account.EmployeeID, now); shiftErr != nil {
		return AttendanceClockStatus{}, shiftErr
	} else if hasShift && clockOutBelongsToPreviousWorkDate(shift, now) {
		previousWorkDate := attendanceWorkDate(now.Add(-24 * time.Hour))
		previous, lookupErr := c.loadAttendanceDayProjection(ctx, account.EmployeeID, previousWorkDate, now)
		if lookupErr != nil {
			return AttendanceClockStatus{}, lookupErr
		}
		if previous.ClockIn != nil {
			projection = previous
			workDate = previousWorkDate
		}
	}
	status := attendanceClockStatusFromProjection(account.EmployeeID, workDate, projection)
	policy, err := c.loadAttendancePolicyResponse(ctx)
	if err != nil {
		return AttendanceClockStatus{}, err
	}
	worksites, err := c.activeAttendanceWorksites(ctx)
	if err != nil {
		return AttendanceClockStatus{}, err
	}
	status.RequireWorksite = policy.WorkTime.RequireWorksite
	status.Worksites = worksites
	if len(worksites) > 0 {
		primary := worksites[0]
		status.Worksite = &primary
	}
	if !activeEmployment {
		status.NextAction = "complete"
		status.CanClockIn = false
		status.CanClockOut = false
	}
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
	if _, err := c.requireAttendanceEmployeeActive(ctx, employeeID); err != nil {
		return AttendanceClockRecord{}, err
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
	clientEventID := strings.TrimSpace(input.ClientEventID)
	if clientEventID != "" {
		existing, ok, lookupErr := c.store.GetAttendanceClockRecordByClientEventID(goContext(ctx), ctx.TenantID, clientEventID)
		if lookupErr != nil {
			return AttendanceClockRecord{}, lookupErr
		}
		if ok {
			if existing.EmployeeID != employeeID {
				return AttendanceClockRecord{}, Conflict("client_event_id already exists").WithReasonCode("attendance_client_event_conflict")
			}
			return applyAttendanceClockFieldPolicy(existing, decision.FieldPolicies), nil
		}
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
	_, hasClockIn, err := c.store.GetEarliestAcceptedAttendanceClockIn(goContext(ctx), ctx.TenantID, employeeID, workDate)
	if err != nil {
		return AttendanceClockRecord{}, err
	}
	rejectionReason = clockRejectionReason(direction, worksite, input.AccuracyMeters, distance, hasClockIn, policy.WorkTime.RequireWorksite)
	if rejectionReason == "" {
		outsideWindow, windowErr := clockOutsideConfiguredWindow(direction, shift, hasShift, policy.WorkTime, now)
		if windowErr != nil {
			return AttendanceClockRecord{}, windowErr
		}
		if outsideWindow {
			rejectionReason = clockRejectionOutsideWindow
		}
	}
	if rejectionReason != "" {
		switch rejectionReason {
		case clockRejectionDuplicate, clockRejectionInvalidSequence, clockRejectionOutsideGeofence, clockRejectionLowAccuracy:
			recordStatus = clockRecordStatusRejected
		default:
			// 時間與工時異常屬於日投影的軟異常，原始卡仍保留為有效卡。
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
		WorksiteName:      worksite.Name,
		WorksiteAddress:   worksite.Address,
		WorkDate:          workDate,
		Direction:         direction,
		ClientEventID:     clientEventID,
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
		if clientEventID != "" {
			existing, ok, lookupErr := c.store.GetAttendanceClockRecordByClientEventID(goContext(ctx), ctx.TenantID, clientEventID)
			if lookupErr == nil && ok && existing.EmployeeID == employeeID {
				return applyAttendanceClockFieldPolicy(existing, decision.FieldPolicies), nil
			}
		}
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
	items, err = c.attachAttendanceClockWorksiteDetails(ctx, items)
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
	if _, err := c.requireAttendanceEmployeeActive(ctx, employeeID); err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	correctionType, err := normalizeAttendanceCorrectionType(input.CorrectionType)
	if err != nil {
		return AttendanceCorrectionRequest{}, err
	}
	targetRecordID := strings.TrimSpace(input.TargetClockRecordID)
	var targetRecord AttendanceClockRecord
	if correctionType != correctionTypeAddRecord {
		if targetRecordID == "" {
			return AttendanceCorrectionRequest{}, BadRequest("target_clock_record_id is required")
		}
		targetRecord, err = c.findAttendanceClockRecord(ctx, employeeID, targetRecordID)
		if err != nil {
			return AttendanceCorrectionRequest{}, err
		}
		if targetRecord.Voided {
			return AttendanceCorrectionRequest{}, BadRequest("target clock record is already voided").WithReasonCode("attendance_correction_invalid_state")
		}
	}
	direction := targetRecord.Direction
	requestedAt := targetRecord.ClockedAt
	workDate := targetRecord.WorkDate
	if correctionType != correctionTypeVoidRecord {
		direction, err = normalizeClockDirection(input.Direction)
		if err != nil {
			return AttendanceCorrectionRequest{}, err
		}
		requestedAt, err = utils.ParseDateTime(input.RequestedClockedAt)
		if err != nil {
			return AttendanceCorrectionRequest{}, BadRequest("requested_clocked_at must be RFC3339 or YYYY-MM-DD")
		}
		if correctionType == correctionTypeAddRecord {
			workDate = attendanceWorkDate(requestedAt)
		}
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
				"application_code":       string(AppAttendance),
				"resource_type":          string(ResourceAttendanceCorrection),
				"action":                 string(ActionCreate),
				"employee_id":            employeeID,
				"correction_type":        correctionType,
				"target_clock_record_id": targetRecordID,
				"direction":              direction,
				"requested_clocked_at":   requestedAt.Format(time.RFC3339),
				"reason":                 reason,
			},
			SubmittedAt: tx.Now(),
			UpdatedAt:   tx.Now(),
		}
		if err := tx.store.UpsertFormInstance(goContext(ctx), instance); err != nil {
			return err
		}
		correction = AttendanceCorrectionRequest{
			ID:                  utils.NewID("acorr"),
			TenantID:            ctx.TenantID,
			EmployeeID:          employeeID,
			CorrectionType:      correctionType,
			TargetClockRecordID: targetRecordID,
			Direction:           direction,
			RequestedClockedAt:  requestedAt,
			WorkDate:            workDate,
			Reason:              reason,
			Status:              correctionStatusPending,
			FormInstanceID:      instance.ID,
			CreatedAt:           tx.Now(),
			UpdatedAt:           tx.Now(),
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

// ApproveAttendanceCorrection 覈準考勤 correction 的服務流程。
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

// attachAttendanceClockWorksiteDetails adds display metadata only for worksites already linked to authorized records.
func (c AttendanceService) attachAttendanceClockWorksiteDetails(ctx RequestContext, items []AttendanceClockRecord) ([]AttendanceClockRecord, error) {
	if len(items) == 0 {
		return items, nil
	}
	worksites, err := c.store.ListAttendanceWorksites(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	worksitesByID := make(map[string]AttendanceWorksite, len(worksites))
	for _, worksite := range worksites {
		worksitesByID[worksite.ID] = worksite
	}
	for index := range items {
		worksite, ok := worksitesByID[items[index].WorksiteID]
		if !ok {
			continue
		}
		items[index].WorksiteName = worksite.Name
		items[index].WorksiteAddress = worksite.Address
	}
	return items, nil
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

// normalizeAttendanceCorrectionType keeps the legacy add-record request as the default.
func normalizeAttendanceCorrectionType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", correctionTypeAddRecord:
		return correctionTypeAddRecord, nil
	case correctionTypeVoidRecord:
		return correctionTypeVoidRecord, nil
	case correctionTypeReplaceRecord:
		return correctionTypeReplaceRecord, nil
	default:
		return "", BadRequest("correction_type must be add_record, void_record, or replace_record")
	}
}

// findAttendanceClockRecord locates a raw employee punch without hiding voided audit history.
func (c AttendanceService) findAttendanceClockRecord(ctx RequestContext, employeeID, recordID string) (AttendanceClockRecord, error) {
	records, err := c.store.ListAttendanceClockRecords(goContext(ctx), ctx.TenantID, AttendanceClockRecordQuery{EmployeeID: employeeID})
	if err != nil {
		return AttendanceClockRecord{}, err
	}
	for _, record := range records {
		if record.ID == recordID {
			return record, nil
		}
	}
	return AttendanceClockRecord{}, NotFound("attendance clock record", recordID)
}

// applyApprovedAttendanceCorrection preserves the target raw punch and applies add, void, or replace atomically.
func (c AttendanceService) applyApprovedAttendanceCorrection(ctx RequestContext, current *AttendanceCorrectionRequest, reviewerID, voidReason string, now time.Time) error {
	correctionType, err := normalizeAttendanceCorrectionType(current.CorrectionType)
	if err != nil {
		return err
	}
	current.CorrectionType = correctionType
	if correctionType == correctionTypeVoidRecord || correctionType == correctionTypeReplaceRecord {
		target, err := c.findAttendanceClockRecord(ctx, current.EmployeeID, current.TargetClockRecordID)
		if err != nil {
			return err
		}
		if target.Voided {
			return BadRequest("target clock record is already voided").WithReasonCode("attendance_correction_invalid_state")
		}
		if !strings.EqualFold(target.RecordStatus, clockRecordStatusAccepted) {
			return BadRequest("only accepted clock records can be voided").WithReasonCode("attendance_correction_invalid_state")
		}
		target.Voided = true
		target.VoidedAt = &now
		target.VoidedByAccountID = reviewerID
		target.VoidReason = strings.TrimSpace(voidReason)
		if target.VoidReason == "" {
			target.VoidReason = current.Reason
		}
		if err := c.store.UpsertAttendanceClockRecord(goContext(ctx), target); err != nil {
			return err
		}
	}
	if correctionType == correctionTypeVoidRecord {
		return nil
	}
	if current.Direction == clockDirectionIn {
		if _, exists, err := c.store.GetEarliestAcceptedAttendanceClockIn(goContext(ctx), ctx.TenantID, current.EmployeeID, current.WorkDate); err != nil {
			return err
		} else if exists {
			return Conflict("accepted clock-in record already exists").WithReasonCode("attendance_clock_sequence_conflict")
		}
	}
	worksite, err := c.firstActiveAttendanceWorksite(ctx)
	if err != nil {
		return err
	}
	record := AttendanceClockRecord{
		ID:                  utils.NewID("acr"),
		TenantID:            ctx.TenantID,
		EmployeeID:          current.EmployeeID,
		WorksiteID:          worksite.ID,
		WorksiteName:        worksite.Name,
		WorksiteAddress:     worksite.Address,
		WorkDate:            current.WorkDate,
		Direction:           current.Direction,
		ClockedAt:           current.RequestedClockedAt,
		Latitude:            worksite.Latitude,
		Longitude:           worksite.Longitude,
		RecordStatus:        clockRecordStatusAccepted,
		Source:              clockSourceManualCorrection,
		CorrectionRequestID: current.ID,
		CreatedAt:           now,
	}
	if err := c.store.UpsertAttendanceClockRecord(goContext(ctx), record); err != nil {
		return err
	}
	current.ClockRecordID = record.ID
	if correctionType == correctionTypeReplaceRecord {
		current.ReplacementClockRecordID = record.ID
	}
	return nil
}

// clockRejectionReason 處理打卡 rejection reason。
func clockRejectionReason(direction string, worksite AttendanceWorksite, accuracyMeters, distanceMeters float64, hasClockIn, requireWorksite bool) string {
	if direction == clockDirectionOut && !hasClockIn {
		return clockRejectionInvalidSequence
	}
	if direction == clockDirectionIn && hasClockIn {
		return clockRejectionDuplicate
	}
	if requireWorksite && distanceMeters > float64(worksite.RadiusMeters) {
		return clockRejectionOutsideGeofence
	}
	if requireWorksite && accuracyMeters > clockMaxAccuracyMeters {
		return clockRejectionLowAccuracy
	}
	return ""
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

// clockWindowMinutes 取得指定打卡方向的班次起訖分鐘。
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

// parseClockWindowMinute 將 HH:MM 班次時間轉換為當天分鐘數。
func parseClockWindowMinute(value, field string) (int, error) {
	parsed, err := parseClockWindowTime(value, field)
	if err != nil {
		return 0, err
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

// clockMinuteOfDay 取得業務時區的當天分鐘數。
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

// attendanceWorkDateForClock 讓跨午夜班次的下班卡歸屬前一工作日。
func attendanceWorkDateForClock(direction string, shift AttendanceShift, at time.Time) string {
	if direction == clockDirectionOut && clockOutBelongsToPreviousWorkDate(shift, at) {
		return at.In(attendanceClockLocation).AddDate(0, 0, -1).Format(time.DateOnly)
	}
	return attendanceWorkDate(at)
}

// clockOutBelongsToPreviousWorkDate 判斷跨午夜班次的下班卡日期歸屬。
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

// optionalAttendanceShift 讀取有效排班；未排班時回退到全局考勤政策。
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
		return AttendanceShiftAssignment{}, AttendanceShift{}, false, BadRequest("attendance shift is required").WithReasonCode("attendance_shift_required")
	}
	return assignment, shift, true, nil
}

// activeAttendanceWorksites returns every active tenant worksite in repository priority order.
func (c AttendanceService) activeAttendanceWorksites(ctx RequestContext) ([]AttendanceWorksite, error) {
	items, err := c.store.ListAttendanceWorksites(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	active := make([]AttendanceWorksite, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(item.Status, attendanceStatusActive) {
			active = append(active, item)
		}
	}
	return active, nil
}

// nearestActiveAttendanceWorksite selects the closest configured worksite using the same set exposed by clock status.
func (c AttendanceService) nearestActiveAttendanceWorksite(ctx RequestContext, latitude, longitude float64) (AttendanceWorksite, float64, error) {
	items, err := c.activeAttendanceWorksites(ctx)
	if err != nil {
		return AttendanceWorksite{}, 0, err
	}
	var selected AttendanceWorksite
	selectedDistance := math.MaxFloat64
	for _, item := range items {
		distance := haversineMeters(latitude, longitude, item.Latitude, item.Longitude)
		if distance < selectedDistance {
			selected, selectedDistance = item, distance
		}
	}
	if selected.ID == "" {
		return AttendanceWorksite{}, 0, BadRequest("attendance worksite is required").WithReasonCode("attendance_worksite_required")
	}
	return selected, selectedDistance, nil
}

// firstActiveAttendanceWorksite returns the primary active worksite for manual correction records.
func (c AttendanceService) firstActiveAttendanceWorksite(ctx RequestContext) (AttendanceWorksite, error) {
	items, err := c.activeAttendanceWorksites(ctx)
	if err != nil {
		return AttendanceWorksite{}, err
	}
	if len(items) > 0 {
		return items[0], nil
	}
	return AttendanceWorksite{}, BadRequest("attendance worksite is required").WithReasonCode("attendance_worksite_required")
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
			return BadRequest("correction request is not pending").WithReasonCode("attendance_correction_invalid_state")
		}
		now := tx.Now()
		reviewReason := strings.TrimSpace(input.Reason)
		current.Status = nextStatus
		current.ReviewedByAccountID = account.ID
		current.ReviewReason = reviewReason
		current.ReviewedAt = &now
		current.UpdatedAt = now
		if nextStatus == correctionStatusApproved {
			if err := tx.applyApprovedAttendanceCorrection(ctx, &current, account.ID, reviewReason, now); err != nil {
				return err
			}
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
