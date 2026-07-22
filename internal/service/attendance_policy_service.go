package service

import (
	"math"
	"strconv"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
)

const (
	leaveTypeCodeSickFull     = "sick_full"
	leaveTypeCodeFlexible     = "flexible"
	leaveTypeCodePersonal     = "personal"
	leaveTypeCodeFamilyCare   = "family_care"
	leaveTypeCodeSickHalf     = "sick_half"
	leaveTypeCodeMenstrual    = "menstrual"
	leaveTypeCodeMarriage     = "marriage"
	leaveTypeCodeMaternity    = "maternity"
	leaveTypeCodePaternity    = "paternity"
	leaveTypeCodeBereavement  = "bereavement"
	leaveTypeCodeOfficial     = "official"
	leaveTypeCodePrenatal     = "prenatal"
	leaveTypeCodeCompensatory = "compensatory"
	leaveTypeCodeAnnual       = "annual"
)

var legacyLeaveTypeCodeMap = map[string]string{
	"病":     leaveTypeCodeSickFull,
	"彈":     leaveTypeCodeFlexible,
	"事":     leaveTypeCodePersonal,
	"照":     leaveTypeCodeFamilyCare,
	"半":     leaveTypeCodeSickHalf,
	"理":     leaveTypeCodeMenstrual,
	"婚":     leaveTypeCodeMarriage,
	"產":     leaveTypeCodeMaternity,
	"陪":     leaveTypeCodePaternity,
	"喪":     leaveTypeCodeBereavement,
	"公":     leaveTypeCodeOfficial,
	"檢":     leaveTypeCodePrenatal,
	"補":     leaveTypeCodeCompensatory,
	"特":     leaveTypeCodeAnnual,
	"特休":    leaveTypeCodeAnnual,
	"特休假":   leaveTypeCodeAnnual,
	"事假":    leaveTypeCodePersonal,
	"病假":    leaveTypeCodeSickFull,
	"全薪病假":  leaveTypeCodeSickFull,
	"半薪病假":  leaveTypeCodeSickHalf,
	"家庭照顧假": leaveTypeCodeFamilyCare,
	"生理假":   leaveTypeCodeMenstrual,
	"婚假":    leaveTypeCodeMarriage,
	"喪假":    leaveTypeCodeBereavement,
	"八週產假":  leaveTypeCodeMaternity,
	"陪產假":   leaveTypeCodePaternity,
	"產檢假":   leaveTypeCodePrenatal,
	"公假":    leaveTypeCodeOfficial,
	"補休假":   leaveTypeCodeCompensatory,
	"彈性休假":  leaveTypeCodeFlexible,
	"外勤":    leaveTypeCodeOfficial,
}

var ehrmsLeaveTypeCodeMap = map[string]string{
	"additional leave":    leaveTypeCodeFlexible,
	"annual leave":        leaveTypeCodeAnnual,
	"compensatory leave":  leaveTypeCodeCompensatory,
	"full pay sick leave": leaveTypeCodeSickFull,
	"half pay sick leave": leaveTypeCodeSickHalf,
	"menstruation leave":  leaveTypeCodeMenstrual,
	"personal leave":      leaveTypeCodePersonal,
	"paid_sick":           leaveTypeCodeSickFull,
	"sick":                leaveTypeCodeSickFull,
	"flex":                leaveTypeCodeFlexible,
	"half_sick":           leaveTypeCodeSickHalf,
	"prenatal_check":      leaveTypeCodePrenatal,
}

// CurrentAttendancePolicy 處理目前考勤政策的服務流程。
func (c AttendanceService) CurrentAttendancePolicy(ctx RequestContext) (AttendancePolicyResponse, error) {
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceLeave, ActionRead, ""); err != nil {
		return AttendancePolicyResponse{}, err
	}
	return c.loadAttendancePolicyResponse(ctx)
}

// ValidateAttendancePolicy checks a draft without changing the published policy.
func (c AttendanceService) ValidateAttendancePolicy(ctx RequestContext, input UpdateAttendancePolicyInput) (domain.AttendancePolicyValidationResult, error) {
	account, _, err := c.requireAttendanceAuthz(ctx, ResourceLeave, ActionUpdate, "")
	if err != nil {
		return domain.AttendancePolicyValidationResult{}, err
	}
	next, err := c.attendancePolicyFromInput(ctx, account.ID, input)
	if err != nil {
		if appErr, ok := domain.AsAppError(err); ok && appErr.Status == 400 {
			return domain.AttendancePolicyValidationResult{
				Valid: false, Issues: []string{appErr.Message}, ProjectedVersion: next.Version, Policy: attendancePolicyResponse(next),
			}, nil
		}
		return domain.AttendancePolicyValidationResult{}, err
	}
	return domain.AttendancePolicyValidationResult{
		Valid: true, Issues: []string{}, ProjectedVersion: next.Version, Policy: attendancePolicyResponse(next),
	}, nil
}

// PublishAttendancePolicy validates and atomically advances the published policy version.
func (c AttendanceService) PublishAttendancePolicy(ctx RequestContext, input UpdateAttendancePolicyInput) (AttendancePolicyResponse, error) {
	validation, err := c.ValidateAttendancePolicy(ctx, input)
	if err != nil {
		return AttendancePolicyResponse{}, err
	}
	if !validation.Valid {
		return AttendancePolicyResponse{}, BadRequest(strings.Join(validation.Issues, "; "))
	}
	return c.UpdateAttendancePolicy(ctx, input)
}

// loadAttendancePolicyResponse 讀取考勤政策（不做額外授權檢查，供內部業務使用）。
func (c AttendanceService) loadAttendancePolicyResponse(ctx RequestContext) (AttendancePolicyResponse, error) {
	policy, ok, err := c.store.GetAttendancePolicy(goContext(ctx), ctx.TenantID)
	if err != nil {
		return AttendancePolicyResponse{}, err
	}
	if !ok {
		return defaultAttendancePolicyResponse(), nil
	}
	return attendancePolicyResponse(policy), nil
}

// UpdateAttendancePolicy 更新考勤政策的服務流程。
func (c AttendanceService) UpdateAttendancePolicy(ctx RequestContext, input UpdateAttendancePolicyInput) (AttendancePolicyResponse, error) {
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppAttendance, ResourceType: ResourceLeave, Action: ActionUpdate},
		AuditTarget{Event: "attendance.policy.update", Resource: string(ResourceLeave)},
	)
	if err != nil {
		return AttendancePolicyResponse{}, err
	}
	var next AttendancePolicy
	if err := c.Service.withTenantTransaction(ctx, func(txService *Service) error {
		tx := txService.Attendance()
		current, exists, err := tx.store.GetAttendancePolicy(goContext(ctx), ctx.TenantID)
		if err != nil {
			return err
		}
		next, err = tx.attendancePolicyFromCurrent(ctx, account.ID, input, current, exists)
		if err != nil {
			return err
		}
		if exists && current.Version == next.Version {
			next = current
			return authzAudit.CommitWith(ctx, tx.Service)
		}
		if !exists {
			baseline := defaultAttendancePolicyVersion(ctx.TenantID, tx.Now())
			if err := tx.store.InsertAttendancePolicyVersion(goContext(ctx), baseline); err != nil {
				return err
			}
			if next.Version == baseline.Version {
				next = baseline
			}
		}
		if next.Version > defaultAttendancePolicyResponse().Version || exists {
			if err := tx.store.InsertAttendancePolicyVersion(goContext(ctx), next); err != nil {
				return err
			}
		}
		if err := tx.audit(ctx, "attendance.policy.update", string(ResourceLeave), "current", string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{
			"clock_mode":                 next.WorkTime.ClockMode,
			"flexible_clock_in_earliest": next.WorkTime.FlexibleClockInEarliest,
			"flexible_clock_out_latest":  next.WorkTime.FlexibleClockOutLatest,
			"standard_start":             next.WorkTime.StandardStart,
			"standard_end":               next.WorkTime.StandardEnd,
			"weekend":                    next.WorkTime.Weekend,
			"version":                    next.Version,
		})); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		return AttendancePolicyResponse{}, err
	}
	return attendancePolicyResponse(next), nil
}

// attendancePolicyFromInput 處理考勤政策 來源 輸入的服務流程。
func (c AttendanceService) attendancePolicyFromInput(ctx RequestContext, accountID string, input UpdateAttendancePolicyInput) (AttendancePolicy, error) {
	current, ok, err := c.store.GetAttendancePolicy(goContext(ctx), ctx.TenantID)
	if err != nil {
		return AttendancePolicy{}, err
	}
	return c.attendancePolicyFromCurrent(ctx, accountID, input, current, ok)
}

func (c AttendanceService) attendancePolicyFromCurrent(ctx RequestContext, accountID string, input UpdateAttendancePolicyInput, current AttendancePolicy, exists bool) (AttendancePolicy, error) {
	now := c.Now()
	defaults := defaultAttendancePolicyResponse()
	currentVersion := defaults.Version
	currentWorkTime := normalizeAttendancePolicyWorkTime(defaults.WorkTime)
	if exists && current.Version > 0 {
		currentVersion = current.Version
	}
	if exists {
		currentWorkTime = normalizeAttendancePolicyWorkTime(current.WorkTime)
	}
	if input.BaseVersion > 0 && input.BaseVersion != currentVersion {
		return AttendancePolicy{}, domain.Conflict("attendance policy changed after this draft was loaded")
	}
	nextWorkTime := normalizeAttendancePolicyWorkTime(input.WorkTime)
	version := currentVersion
	if !attendancePolicyWorkTimeEqual(currentWorkTime, nextWorkTime) {
		version++
	}
	policy := AttendancePolicy{
		TenantID:             ctx.TenantID,
		WorkTime:             nextWorkTime,
		Version:              version,
		EffectiveFrom:        &now,
		PublishedByAccountID: strings.TrimSpace(accountID),
		PublishedAt:          now,
	}
	if err := validateAttendancePolicy(policy); err != nil {
		return policy, err
	}
	return policy, nil
}

func defaultAttendancePolicyVersion(tenantID string, now time.Time) AttendancePolicy {
	workTime := defaultAttendancePolicyResponse().WorkTime
	return AttendancePolicy{
		TenantID:      tenantID,
		WorkTime:      workTime,
		Version:       1,
		EffectiveFrom: &now,
		PublishedAt:   now,
	}
}

// defaultAttendancePolicyResponse 處理預設考勤政策回應。
func defaultAttendancePolicyResponse() AttendancePolicyResponse {
	return AttendancePolicyResponse{
		WorkTime: AttendancePolicyWorkTime{
			RequireWorksite:         true,
			ClockMode:               clockModeFlexible,
			FlexibleClockInEarliest: "00:00",
			FlexibleClockOutLatest:  "23:30",
			StandardStart:           "09:00",
			StandardEnd:             "17:00",
			BreakStart:              "12:00",
			BreakEnd:                "13:00",
			Weekend:                 "週六、週日",
			CycleStart:              "1 日",
			CycleEnd:                "本月 月底（最後一日）",
		},
		Version: 1,
	}
}

// attendancePolicyResponse 處理考勤政策回應。
func attendancePolicyResponse(policy AttendancePolicy) AttendancePolicyResponse {
	workTime := normalizeAttendancePolicyWorkTime(policy.WorkTime)
	version := policy.Version
	if version <= 0 {
		version = 1
	}
	return AttendancePolicyResponse{WorkTime: workTime, Version: version}
}

// normalizeAttendancePolicyWorkTime 正規化考勤政策 work 時間。
func normalizeAttendancePolicyWorkTime(workTime AttendancePolicyWorkTime) AttendancePolicyWorkTime {
	defaults := defaultAttendancePolicyResponse().WorkTime
	out := AttendancePolicyWorkTime{
		RequireWorksite:         workTime.RequireWorksite,
		ClockMode:               strings.ToLower(strings.TrimSpace(workTime.ClockMode)),
		FlexibleClockInEarliest: strings.TrimSpace(workTime.FlexibleClockInEarliest),
		FlexibleClockOutLatest:  strings.TrimSpace(workTime.FlexibleClockOutLatest),
		StandardStart:           strings.TrimSpace(workTime.StandardStart),
		StandardEnd:             strings.TrimSpace(workTime.StandardEnd),
		BreakStart:              strings.TrimSpace(workTime.BreakStart),
		BreakEnd:                strings.TrimSpace(workTime.BreakEnd),
		Weekend:                 strings.TrimSpace(workTime.Weekend),
		CycleStart:              strings.TrimSpace(workTime.CycleStart),
		CycleEnd:                strings.TrimSpace(workTime.CycleEnd),
	}
	if out.ClockMode == "" {
		out.ClockMode = defaults.ClockMode
	}
	if out.FlexibleClockInEarliest == "" {
		out.FlexibleClockInEarliest = defaults.FlexibleClockInEarliest
	}
	if out.FlexibleClockOutLatest == "" {
		out.FlexibleClockOutLatest = defaults.FlexibleClockOutLatest
	}
	if out.StandardStart == "" {
		out.StandardStart = defaults.StandardStart
	}
	if out.StandardEnd == "" {
		out.StandardEnd = defaults.StandardEnd
	}
	if out.BreakStart == "" {
		out.BreakStart = defaults.BreakStart
	}
	if out.BreakEnd == "" {
		out.BreakEnd = defaults.BreakEnd
	}
	if out.Weekend == "" {
		out.Weekend = defaults.Weekend
	}
	if out.CycleStart == "" {
		out.CycleStart = defaults.CycleStart
	}
	if out.CycleEnd == "" {
		out.CycleEnd = defaults.CycleEnd
	}
	return out
}

func attendancePolicyWorkTimeEqual(left, right AttendancePolicyWorkTime) bool {
	return left.RequireWorksite == right.RequireWorksite &&
		left.ClockMode == right.ClockMode &&
		left.FlexibleClockInEarliest == right.FlexibleClockInEarliest &&
		left.FlexibleClockOutLatest == right.FlexibleClockOutLatest &&
		left.StandardStart == right.StandardStart &&
		left.StandardEnd == right.StandardEnd &&
		left.BreakStart == right.BreakStart &&
		left.BreakEnd == right.BreakEnd &&
		left.Weekend == right.Weekend &&
		left.CycleStart == right.CycleStart &&
		left.CycleEnd == right.CycleEnd
}

// validateAttendancePolicy 驗證考勤政策。
func validateAttendancePolicy(policy AttendancePolicy) error {
	if policy.WorkTime.ClockMode != clockModeFlexible && policy.WorkTime.ClockMode != clockModeFixed {
		return BadRequest("clock_mode must be flexible or fixed")
	}
	if !stringInSlice(policy.WorkTime.StandardStart, attendancePolicyTimeOptions()) || !stringInSlice(policy.WorkTime.StandardEnd, attendancePolicyTimeOptions()) {
		return BadRequest("standard time must use a configured time option")
	}
	if !stringInSlice(policy.WorkTime.FlexibleClockInEarliest, attendancePolicyTimeOptions()) || !stringInSlice(policy.WorkTime.FlexibleClockOutLatest, attendancePolicyTimeOptions()) {
		return BadRequest("flexible clock range must use configured time options")
	}
	if parseHHMMMinutes(policy.WorkTime.FlexibleClockInEarliest) > parseHHMMMinutes(policy.WorkTime.FlexibleClockOutLatest) {
		return BadRequest("flexible clock earliest time must not be later than latest time")
	}
	if !stringInSlice(policy.WorkTime.BreakStart, attendancePolicyTimeOptions()) || !stringInSlice(policy.WorkTime.BreakEnd, attendancePolicyTimeOptions()) {
		return BadRequest("break time must use a configured time option")
	}
	if !stringInSlice(policy.WorkTime.Weekend, attendancePolicyWeekendOptions()) {
		return BadRequest("weekend must use a configured weekend option")
	}
	if !stringInSlice(policy.WorkTime.CycleStart, attendancePolicyCycleStartOptions()) || !stringInSlice(policy.WorkTime.CycleEnd, attendancePolicyCycleEndOptions()) {
		return BadRequest("cycle must use configured cycle options")
	}
	return nil
}

// normalizeLeaveTypeCode 將內部、舊版與 eHRMS 假別名稱統一為政策使用的 canonical code。
func normalizeLeaveTypeCode(code string) string {
	code = strings.TrimSpace(code)
	if mapped, ok := legacyLeaveTypeCodeMap[code]; ok {
		return mapped
	}
	normalized := strings.ToLower(code)
	if mapped, ok := ehrmsLeaveTypeCodeMap[normalized]; ok {
		return mapped
	}
	return normalized
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func standardDayHours(work AttendancePolicyWorkTime) float64 {
	start := parseHHMMMinutes(work.StandardStart)
	end := parseHHMMMinutes(work.StandardEnd)
	breakStart := parseHHMMMinutes(work.BreakStart)
	breakEnd := parseHHMMMinutes(work.BreakEnd)
	if start < 0 || end < 0 || end <= start {
		return 8
	}
	hours := float64(end-start) / 60
	if breakStart >= 0 && breakEnd > breakStart && breakStart >= start && breakEnd <= end {
		hours -= float64(breakEnd-breakStart) / 60
	}
	if hours <= 0 {
		return 8
	}
	return hours
}

// CalculateLeaveHoursWithinPolicy calculates payable leave hours across policy workdays and breaks.
func CalculateLeaveHoursWithinPolicy(startAt, endAt time.Time, work AttendancePolicyWorkTime) float64 {
	if !endAt.After(startAt) {
		return 0
	}
	standardStart := parseHHMMMinutes(work.StandardStart)
	standardEnd := parseHHMMMinutes(work.StandardEnd)
	breakStart := parseHHMMMinutes(work.BreakStart)
	breakEnd := parseHHMMMinutes(work.BreakEnd)
	if standardStart < 0 || standardEnd <= standardStart {
		return 0
	}

	localStart := startAt.In(attendanceClockLocation)
	localEnd := endAt.In(attendanceClockLocation)
	lastDay := time.Date(localEnd.Year(), localEnd.Month(), localEnd.Day(), 0, 0, 0, 0, attendanceClockLocation)
	totalHours := 0.0
	for day := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, attendanceClockLocation); !day.After(lastDay); day = day.AddDate(0, 0, 1) {
		if attendancePolicyDayIsWeekend(day, work.Weekend) {
			continue
		}
		workStartAt := day.Add(time.Duration(standardStart) * time.Minute)
		workEndAt := day.Add(time.Duration(standardEnd) * time.Minute)
		clippedStartAt := workStartAt
		if localStart.After(clippedStartAt) {
			clippedStartAt = localStart
		}
		clippedEndAt := workEndAt
		if localEnd.Before(clippedEndAt) {
			clippedEndAt = localEnd
		}
		if !clippedEndAt.After(clippedStartAt) {
			continue
		}

		dayHours := clippedEndAt.Sub(clippedStartAt).Hours()
		if breakStart >= 0 && breakEnd > breakStart {
			breakStartAt := day.Add(time.Duration(breakStart) * time.Minute)
			breakEndAt := day.Add(time.Duration(breakEnd) * time.Minute)
			overlapStartAt := breakStartAt
			if clippedStartAt.After(overlapStartAt) {
				overlapStartAt = clippedStartAt
			}
			overlapEndAt := breakEndAt
			if clippedEndAt.Before(overlapEndAt) {
				overlapEndAt = clippedEndAt
			}
			if overlapEndAt.After(overlapStartAt) {
				dayHours -= overlapEndAt.Sub(overlapStartAt).Hours()
			}
		}
		totalHours += math.Max(0, dayHours)
	}
	return math.Round(totalHours*100) / 100
}

// attendancePolicyDayIsWeekend applies the configured non-working weekdays to leave duration.
func attendancePolicyDayIsWeekend(day time.Time, weekend string) bool {
	switch strings.TrimSpace(weekend) {
	case "", "週六、週日":
		return day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
	case "週日":
		return day.Weekday() == time.Sunday
	case "無":
		return false
	default:
		return false
	}
}

func parseHHMMMinutes(value string) int {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return -1
	}
	hour, err1 := strconv.Atoi(parts[0])
	minute, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return -1
	}
	return hour*60 + minute
}

// stringInSlice 處理字串 in slice。
func stringInSlice(value string, options []string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

// attendancePolicyTimeOptions 處理考勤政策時間選項。
func attendancePolicyTimeOptions() []string {
	options := make([]string, 0, 48)
	for hour := 0; hour < 24; hour++ {
		options = append(options, twoDigit(hour)+":00", twoDigit(hour)+":30")
	}
	return options
}

// attendancePolicyWeekendOptions 處理考勤政策 weekend 選項。
func attendancePolicyWeekendOptions() []string {
	return []string{"週六、週日", "週日", "無"}
}

// attendancePolicyCycleStartOptions 處理考勤政策 cycle start 選項。
func attendancePolicyCycleStartOptions() []string {
	options := make([]string, 0, 28)
	for day := 1; day <= 28; day++ {
		options = append(options, strconv.Itoa(day)+" 日")
	}
	return options
}

// attendancePolicyCycleEndOptions 處理考勤政策 cycle end 選項。
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

// twoDigit 處理 two digit。
func twoDigit(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}
