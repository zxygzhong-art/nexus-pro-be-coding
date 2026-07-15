package service

import (
	"math"
	"strconv"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
)

const (
	leaveUnitDay  = "day"
	leaveUnitHour = "hour"

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
		var err error
		next, err = tx.attendancePolicyFromInput(ctx, account.ID, input)
		if err != nil {
			return err
		}
		if err := tx.store.UpsertAttendancePolicy(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.audit(ctx, "attendance.policy.update", string(ResourceLeave), next.ID, string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{
			"leave_type_count":           len(next.LeaveTypes),
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
	now := c.Now()
	createdAt := now
	// The built-in policy is the implicit version 1, so the first explicit publish advances to version 2.
	currentVersion := defaultAttendancePolicyResponse().Version
	version := currentVersion + 1
	if ok && !current.CreatedAt.IsZero() {
		createdAt = current.CreatedAt
	}
	if ok && current.Version > 0 {
		currentVersion = current.Version
		version = currentVersion + 1
	}
	if input.BaseVersion > 0 && input.BaseVersion != currentVersion {
		return AttendancePolicy{}, domain.Conflict("attendance policy changed after this draft was loaded")
	}
	nextLeaveTypes := normalizeAttendanceLeaveTypes(input.LeaveTypes)
	currentLeaveTypes := defaultAttendancePolicyResponse().LeaveTypes
	if ok {
		currentLeaveTypes = normalizeAttendanceLeaveTypes(current.LeaveTypes)
	}
	retained := make(map[string]struct{}, len(nextLeaveTypes)*2)
	for _, leaveType := range nextLeaveTypes {
		retained[leaveType.ID] = struct{}{}
		retained[strings.ToLower(leaveType.Code)] = struct{}{}
	}
	for _, leaveType := range currentLeaveTypes {
		if _, keptByID := retained[leaveType.ID]; keptByID {
			continue
		}
		if _, keptByCode := retained[strings.ToLower(leaveType.Code)]; keptByCode {
			continue
		}
		linked, linkErr := c.leaveTypeHasLinkedData(ctx, leaveType)
		if linkErr != nil {
			return AttendancePolicy{}, linkErr
		}
		if linked {
			return AttendancePolicy{}, BadRequest("leave type " + leaveType.Name + " has mappings, balances, or requests; deactivate it instead of deleting it")
		}
	}
	policy := AttendancePolicy{
		ID:                 "current",
		TenantID:           ctx.TenantID,
		WorkTime:           normalizeAttendancePolicyWorkTime(input.WorkTime),
		LeaveTypes:         nextLeaveTypes,
		Version:            version,
		EffectiveFrom:      &now,
		UpdatedByAccountID: strings.TrimSpace(accountID),
		CreatedAt:          createdAt,
		UpdatedAt:          now,
	}
	if err := validateAttendancePolicy(policy); err != nil {
		return policy, err
	}
	return policy, nil
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
			TimeOptions:             attendancePolicyTimeOptions(),
			WeekendOptions:          attendancePolicyWeekendOptions(),
			CycleStartOptions:       attendancePolicyCycleStartOptions(),
			CycleEndOptions:         attendancePolicyCycleEndOptions(),
		},
		LeaveTypes: attendancePolicyLeaveTypes(),
		Version:    1,
	}
}

// attendancePolicyResponse 處理考勤政策回應。
func attendancePolicyResponse(policy AttendancePolicy) AttendancePolicyResponse {
	workTime := normalizeAttendancePolicyWorkTime(policy.WorkTime)
	leaveTypes := normalizeAttendanceLeaveTypes(policy.LeaveTypes)
	if len(leaveTypes) == 0 {
		leaveTypes = attendancePolicyLeaveTypes()
	}
	version := policy.Version
	if version <= 0 {
		version = 1
	}
	return AttendancePolicyResponse{WorkTime: workTime, LeaveTypes: leaveTypes, Version: version}
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
		TimeOptions:             defaults.TimeOptions,
		WeekendOptions:          defaults.WeekendOptions,
		CycleStartOptions:       defaults.CycleStartOptions,
		CycleEndOptions:         defaults.CycleEndOptions,
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

// normalizeAttendanceLeaveTypes 正規化考勤請假 types。
func normalizeAttendanceLeaveTypes(items []AttendanceLeaveType) []AttendanceLeaveType {
	out := make([]AttendanceLeaveType, 0, len(items))
	for _, item := range items {
		next := normalizeAttendanceLeaveType(item)
		if next.Code == "" && next.Name == "" {
			continue
		}
		out = append(out, next)
	}
	return out
}

func normalizeAttendanceLeaveType(item AttendanceLeaveType) AttendanceLeaveType {
	code := normalizeLeaveTypeCode(item.Code)
	seed, hasSeed := attendanceLeaveTypeByCode(code)
	next := AttendanceLeaveType{
		ID:              strings.TrimSpace(item.ID),
		Code:            code,
		Name:            strings.TrimSpace(item.Name),
		Quota:           strings.TrimSpace(item.Quota),
		Rule:            strings.TrimSpace(item.Rule),
		Proof:           strings.TrimSpace(item.Proof),
		Unit:            strings.TrimSpace(item.Unit),
		GrantMode:       strings.TrimSpace(item.GrantMode),
		RequiresBalance: item.RequiresBalance,
		PaidRatio:       item.PaidRatio,
		ProofAfterHours: cloneFloatPtr(item.ProofAfterHours),
		Active:          true,
		Entitlements:    normalizeLeaveEntitlements(item.Entitlements),
	}
	if next.ID == "" {
		next.ID = domain.StableLeaveTypeID(code)
	}
	// Detect "legacy display-only" payloads (only code/name/quota/rule/proof).
	legacyOnly := item.GrantMode == "" && item.Unit == "" && len(item.Entitlements) == 0 && item.PaidRatio == 0 && item.ProofAfterHours == nil && !item.RequiresBalance && !item.Active
	if legacyOnly && hasSeed {
		next.Name = firstNonBlank(next.Name, seed.Name)
		next.Unit = seed.Unit
		next.GrantMode = seed.GrantMode
		next.RequiresBalance = seed.RequiresBalance
		next.PaidRatio = seed.PaidRatio
		next.Active = seed.Active
		next.ProofAfterHours = cloneFloatPtr(seed.ProofAfterHours)
		next.Entitlements = normalizeLeaveEntitlements(seed.Entitlements)
		next.Quota = firstNonBlank(next.Quota, seed.Quota)
		next.Rule = firstNonBlank(next.Rule, seed.Rule)
		next.Proof = firstNonBlank(next.Proof, seed.Proof)
		return next
	}
	if next.Unit == "" {
		next.Unit = leaveUnitDay
	}
	if next.GrantMode == "" {
		next.GrantMode = defaultGrantModeForLeaveType(next.Code)
	}
	if item.GrantMode == "" {
		next.RequiresBalance = leaveTypeRequiresBalance(next.GrantMode)
	} else if next.GrantMode == domain.LeaveGrantModeAnnualGrant || next.GrantMode == domain.LeaveGrantModeOvertimeCredit {
		next.RequiresBalance = true
	}
	if item.PaidRatio == 0 && item.GrantMode == "" && next.Code != leaveTypeCodePersonal && next.Code != leaveTypeCodeSickHalf {
		next.PaidRatio = defaultPaidRatioForLeaveType(next.Code)
	}
	if item.Active {
		next.Active = true
	} else if item.GrantMode != "" {
		// Explicit structured payload may disable a leave type.
		next.Active = item.Active
	}
	if next.GrantMode == domain.LeaveGrantModeAnnualGrant && len(next.Entitlements) == 0 && hasSeed {
		next.Entitlements = normalizeLeaveEntitlements(seed.Entitlements)
	}
	// Personal leave is application-based and must never depend on a pre-granted balance row.
	if next.Code == leaveTypeCodePersonal {
		next.GrantMode = domain.LeaveGrantModeEvent
		next.RequiresBalance = false
		next.Entitlements = nil
	}
	if next.Name == "" && hasSeed {
		next.Name = seed.Name
	}
	derivedQuota, derivedRule, derivedProof := deriveLeaveTypeDisplay(next)
	next.Quota = firstNonBlank(next.Quota, derivedQuota)
	next.Rule = firstNonBlank(next.Rule, derivedRule)
	next.Proof = firstNonBlank(next.Proof, derivedProof)
	return next
}

func cloneFloatPtr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeLeaveEntitlements(items []domain.LeaveEntitlementRule) []domain.LeaveEntitlementRule {
	if len(items) == 0 {
		return nil
	}
	out := make([]domain.LeaveEntitlementRule, 0, len(items))
	for _, item := range items {
		jobLevel := strings.TrimSpace(item.JobLevel)
		if jobLevel == "" {
			jobLevel = "*"
		}
		next := domain.LeaveEntitlementRule{
			JobLevel:       jobLevel,
			TenureMinYears: item.TenureMinYears,
			QuotaHours:     item.QuotaHours,
			Prorate:        item.Prorate,
			Priority:       item.Priority,
		}
		if item.TenureMaxYears != nil {
			max := *item.TenureMaxYears
			next.TenureMaxYears = &max
		}
		out = append(out, next)
	}
	return out
}

// validateAttendancePolicy 驗證考勤政策。
func validateAttendancePolicy(policy AttendancePolicy) error {
	if policy.WorkTime.ClockMode != clockModeFlexible && policy.WorkTime.ClockMode != clockModeFixed {
		return BadRequest("clock_mode must be flexible or fixed")
	}
	if !stringInSlice(policy.WorkTime.StandardStart, policy.WorkTime.TimeOptions) || !stringInSlice(policy.WorkTime.StandardEnd, policy.WorkTime.TimeOptions) {
		return BadRequest("standard time must use a configured time option")
	}
	if !stringInSlice(policy.WorkTime.FlexibleClockInEarliest, policy.WorkTime.TimeOptions) || !stringInSlice(policy.WorkTime.FlexibleClockOutLatest, policy.WorkTime.TimeOptions) {
		return BadRequest("flexible clock range must use configured time options")
	}
	if parseHHMMMinutes(policy.WorkTime.FlexibleClockInEarliest) > parseHHMMMinutes(policy.WorkTime.FlexibleClockOutLatest) {
		return BadRequest("flexible clock earliest time must not be later than latest time")
	}
	if !stringInSlice(policy.WorkTime.BreakStart, policy.WorkTime.TimeOptions) || !stringInSlice(policy.WorkTime.BreakEnd, policy.WorkTime.TimeOptions) {
		return BadRequest("break time must use a configured time option")
	}
	if !stringInSlice(policy.WorkTime.Weekend, policy.WorkTime.WeekendOptions) {
		return BadRequest("weekend must use a configured weekend option")
	}
	if !stringInSlice(policy.WorkTime.CycleStart, policy.WorkTime.CycleStartOptions) || !stringInSlice(policy.WorkTime.CycleEnd, policy.WorkTime.CycleEndOptions) {
		return BadRequest("cycle must use configured cycle options")
	}
	if len(policy.LeaveTypes) == 0 {
		return BadRequest("leave_types is required")
	}
	seen := map[string]struct{}{}
	seenIDs := map[string]struct{}{}
	for _, item := range policy.LeaveTypes {
		if item.ID == "" || item.Code == "" || item.Name == "" {
			return BadRequest("leave type id, code, and name are required")
		}
		if _, ok := seenIDs[item.ID]; ok {
			return BadRequest("leave type id must be unique")
		}
		seenIDs[item.ID] = struct{}{}
		if _, ok := seen[item.Code]; ok {
			return BadRequest("leave type code must be unique")
		}
		seen[item.Code] = struct{}{}
		if !isValidLeaveGrantMode(item.GrantMode) {
			return BadRequest("leave type grant_mode is invalid")
		}
		if item.Unit != leaveUnitDay && item.Unit != leaveUnitHour {
			return BadRequest("leave type unit must be day or hour")
		}
		if item.GrantMode == domain.LeaveGrantModeAnnualGrant {
			hasWildcard := false
			for _, ent := range item.Entitlements {
				if ent.QuotaHours < 0 {
					return BadRequest("entitlement quota_hours must be >= 0")
				}
				if ent.TenureMinYears < 0 {
					return BadRequest("entitlement tenure_min_years must be >= 0")
				}
				if ent.TenureMaxYears != nil && *ent.TenureMaxYears <= ent.TenureMinYears {
					return BadRequest("entitlement tenure_max_years must be greater than tenure_min_years")
				}
				if ent.JobLevel == "*" {
					hasWildcard = true
				}
			}
			if !hasWildcard {
				return BadRequest("annual_grant leave type requires a job_level=* entitlement")
			}
		}
	}
	return nil
}

func isValidLeaveGrantMode(mode string) bool {
	switch mode {
	case domain.LeaveGrantModeAnnualGrant, domain.LeaveGrantModeEvent, domain.LeaveGrantModeOvertimeCredit, domain.LeaveGrantModeUnlimited:
		return true
	default:
		return false
	}
}

func leaveTypeRequiresBalance(mode string) bool {
	switch mode {
	case domain.LeaveGrantModeAnnualGrant, domain.LeaveGrantModeOvertimeCredit:
		return true
	default:
		return false
	}
}

func defaultGrantModeForLeaveType(code string) string {
	if seed, ok := attendanceLeaveTypeByCode(code); ok {
		return seed.GrantMode
	}
	return domain.LeaveGrantModeAnnualGrant
}

func defaultPaidRatioForLeaveType(code string) float64 {
	if seed, ok := attendanceLeaveTypeByCode(code); ok {
		return seed.PaidRatio
	}
	return 1
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

func findLeaveTypeInPolicy(policy AttendancePolicyResponse, code string) (AttendanceLeaveType, bool) {
	code = normalizeLeaveTypeCode(code)
	for _, item := range policy.LeaveTypes {
		if item.Code == code {
			return item, true
		}
	}
	return AttendanceLeaveType{}, false
}

func compensatoryLeaveTypeCode(policy AttendancePolicyResponse) string {
	for _, item := range policy.LeaveTypes {
		if item.GrantMode == domain.LeaveGrantModeOvertimeCredit && item.Active {
			return item.Code
		}
	}
	return leaveTypeCodeCompensatory
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

func roundHalfHour(hours float64) float64 {
	if hours <= 0 {
		return 0
	}
	return math.Round(hours*2) / 2
}

func deriveLeaveTypeDisplay(item AttendanceLeaveType) (quota, rule, proof string) {
	dayHours := 8.0
	switch item.GrantMode {
	case domain.LeaveGrantModeUnlimited:
		quota = "不限天數"
		rule = "需主管核可"
	case domain.LeaveGrantModeOvertimeCredit:
		quota = "依加班時數"
		rule = "期限內請畢"
	case domain.LeaveGrantModeEvent:
		quota = "依事件申請"
		rule = "事件發生時申請"
	default:
		if len(item.Entitlements) == 1 && item.Entitlements[0].JobLevel == "*" {
			days := item.Entitlements[0].QuotaHours / dayHours
			if days == float64(int(days)) {
				quota = strconv.Itoa(int(days)) + " 天 / 年"
			} else {
				quota = strings.TrimRight(strings.TrimRight(strconv.FormatFloat(days, 'f', 1, 64), "0"), ".") + " 天 / 年"
			}
		} else if len(item.Entitlements) > 1 {
			quota = "依年資 / 職級"
		} else {
			quota = "依公司政策"
		}
		if hasProrate(item.Entitlements) {
			rule = "依年資折算"
		} else {
			rule = "無累計"
		}
	}
	if item.ProofAfterHours != nil && *item.ProofAfterHours > 0 {
		days := *item.ProofAfterHours / dayHours
		proof = strconv.FormatFloat(days, 'f', -1, 64) + " 天以上需證明"
	} else {
		proof = "—"
	}
	return quota, rule, proof
}

func hasProrate(items []domain.LeaveEntitlementRule) bool {
	for _, item := range items {
		if item.Prorate {
			return true
		}
	}
	return false
}

func intPtr(v int) *int { return &v }

func floatPtr(v float64) *float64 { return &v }

func entitlement(jobLevel string, minYears int, maxYears *int, quotaHours float64, prorate bool, priority int) domain.LeaveEntitlementRule {
	return domain.LeaveEntitlementRule{
		JobLevel:       jobLevel,
		TenureMinYears: minYears,
		TenureMaxYears: maxYears,
		QuotaHours:     quotaHours,
		Prorate:        prorate,
		Priority:       priority,
	}
}

func attendanceLeaveTypeByCode(code string) (AttendanceLeaveType, bool) {
	for _, item := range attendancePolicyLeaveTypes() {
		if item.Code == code {
			return item, true
		}
	}
	return AttendanceLeaveType{}, false
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

// attendancePolicyLeaveTypes 處理考勤政策請假 types。
func attendancePolicyLeaveTypes() []AttendanceLeaveType {
	proof24 := floatPtr(24)
	items := []AttendanceLeaveType{
		{
			Code: leaveTypeCodeSickFull, Name: "全薪病假", Unit: leaveUnitDay,
			GrantMode: domain.LeaveGrantModeAnnualGrant, RequiresBalance: true, PaidRatio: 1, Active: true,
			ProofAfterHours: proof24,
			Quota:           "30 天 / 年", Rule: "無累計", Proof: "3 天以上需診斷證明",
			Entitlements: []domain.LeaveEntitlementRule{entitlement("*", 0, nil, 240, false, 0)},
		},
		{
			Code: leaveTypeCodeFlexible, Name: "彈性休假", Unit: leaveUnitDay,
			GrantMode: domain.LeaveGrantModeAnnualGrant, RequiresBalance: true, PaidRatio: 1, Active: true,
			Quota: "依公司政策", Rule: "依年資折算", Proof: "—",
			Entitlements: []domain.LeaveEntitlementRule{entitlement("*", 0, nil, 0, true, 0)},
		},
		{
			Code: leaveTypeCodePersonal, Name: "事假", Unit: leaveUnitDay,
			GrantMode: domain.LeaveGrantModeEvent, RequiresBalance: false, PaidRatio: 0, Active: true,
			Quota: "14 天 / 年", Rule: "無累計（不支薪）", Proof: "—",
		},
		{
			Code: leaveTypeCodeFamilyCare, Name: "家庭照顧假", Unit: leaveUnitDay,
			GrantMode: domain.LeaveGrantModeAnnualGrant, RequiresBalance: true, PaidRatio: 0, Active: true,
			Quota: "7 天 / 年", Rule: "併入事假計算", Proof: "得要求相關證明",
			Entitlements: []domain.LeaveEntitlementRule{entitlement("*", 0, nil, 56, false, 0)},
		},
		{
			Code: leaveTypeCodeSickHalf, Name: "半薪病假", Unit: leaveUnitDay,
			GrantMode: domain.LeaveGrantModeAnnualGrant, RequiresBalance: true, PaidRatio: 0.5, Active: true,
			Quota: "30 天 / 年", Rule: "無累計", Proof: "診斷證明",
			Entitlements: []domain.LeaveEntitlementRule{entitlement("*", 0, nil, 240, false, 0)},
		},
		{
			Code: leaveTypeCodeMenstrual, Name: "生理假", Unit: leaveUnitDay,
			GrantMode: domain.LeaveGrantModeEvent, RequiresBalance: false, PaidRatio: 1, Active: true,
			Quota: "每月 1 日", Rule: "全年逾 3 日併入病假", Proof: "—",
		},
		{
			Code: leaveTypeCodeMarriage, Name: "婚假", Unit: leaveUnitDay,
			GrantMode: domain.LeaveGrantModeEvent, RequiresBalance: false, PaidRatio: 1, Active: true,
			Quota: "8 天", Rule: "登記後 3 個月內請畢", Proof: "結婚證明",
		},
		{
			Code: leaveTypeCodeMaternity, Name: "八週產假", Unit: leaveUnitDay,
			GrantMode: domain.LeaveGrantModeEvent, RequiresBalance: false, PaidRatio: 1, Active: true,
			Quota: "56 天", Rule: "一次請足", Proof: "醫療證明",
		},
		{
			Code: leaveTypeCodePaternity, Name: "陪產假", Unit: leaveUnitDay,
			GrantMode: domain.LeaveGrantModeEvent, RequiresBalance: false, PaidRatio: 1, Active: true,
			Quota: "7 天", Rule: "分娩前後 15 日內", Proof: "出生證明",
		},
		{
			Code: leaveTypeCodeBereavement, Name: "喪假", Unit: leaveUnitDay,
			GrantMode: domain.LeaveGrantModeEvent, RequiresBalance: false, PaidRatio: 1, Active: true,
			Quota: "3 - 8 天", Rule: "依親等決定天數", Proof: "訃聞或證明",
		},
		{
			Code: leaveTypeCodeOfficial, Name: "公假", Unit: leaveUnitDay,
			GrantMode: domain.LeaveGrantModeUnlimited, RequiresBalance: false, PaidRatio: 1, Active: true,
			Quota: "不限天數", Rule: "需主管核可", Proof: "政府傳票或公文",
		},
		{
			Code: leaveTypeCodePrenatal, Name: "產檢假", Unit: leaveUnitDay,
			GrantMode: domain.LeaveGrantModeEvent, RequiresBalance: false, PaidRatio: 1, Active: true,
			Quota: "7 天", Rule: "妊娠期間", Proof: "產檢證明",
		},
		{
			Code: leaveTypeCodeCompensatory, Name: "補休假", Unit: leaveUnitHour,
			GrantMode: domain.LeaveGrantModeOvertimeCredit, RequiresBalance: true, PaidRatio: 1, Active: true,
			Quota: "依加班時數", Rule: "期限內請畢", Proof: "加班紀錄",
		},
		{
			Code: leaveTypeCodeAnnual, Name: "特休假", Unit: leaveUnitDay,
			GrantMode: domain.LeaveGrantModeAnnualGrant, RequiresBalance: true, PaidRatio: 1, Active: true,
			Quota: "依年資 3 - 30 天", Rule: "依年資折算，可遞延一年", Proof: "—",
			Entitlements: []domain.LeaveEntitlementRule{
				entitlement("*", 0, intPtr(1), 24, true, 0),
				entitlement("*", 1, intPtr(2), 56, true, 0),
				entitlement("*", 2, intPtr(3), 80, true, 0),
				entitlement("*", 3, intPtr(5), 112, true, 0),
				entitlement("*", 5, intPtr(10), 120, true, 0),
				entitlement("*", 10, nil, 160, true, 0),
				entitlement("senior", 3, nil, 128, true, 10),
			},
		},
	}
	for index := range items {
		items[index].ID = domain.StableLeaveTypeID(items[index].Code)
	}
	return items
}

// twoDigit 處理 two digit。
func twoDigit(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}
