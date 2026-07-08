package service

import (
	"strconv"
	"strings"
)

// CurrentAttendancePolicy 處理目前考勤政策的服務流程。
func (c AttendanceService) CurrentAttendancePolicy(ctx RequestContext) (AttendancePolicyResponse, error) {
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceLeave, ActionRead, ""); err != nil {
		return AttendancePolicyResponse{}, err
	}
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
	next, err := c.attendancePolicyFromInput(ctx, account.ID, input)
	if err != nil {
		return AttendancePolicyResponse{}, err
	}
	if err := c.Service.withTenantTransaction(ctx, func(txService *Service) error {
		tx := txService.Attendance()
		if err := tx.store.UpsertAttendancePolicy(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.audit(ctx, "attendance.policy.update", string(ResourceLeave), next.ID, string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{
			"leave_type_count": len(next.LeaveTypes),
			"standard_start":   next.WorkTime.StandardStart,
			"standard_end":     next.WorkTime.StandardEnd,
			"weekend":          next.WorkTime.Weekend,
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
	if ok && !current.CreatedAt.IsZero() {
		createdAt = current.CreatedAt
	}
	policy := AttendancePolicy{
		ID:                 "current",
		TenantID:           ctx.TenantID,
		WorkTime:           normalizeAttendancePolicyWorkTime(input.WorkTime),
		LeaveTypes:         normalizeAttendanceLeaveTypes(input.LeaveTypes),
		UpdatedByAccountID: strings.TrimSpace(accountID),
		CreatedAt:          createdAt,
		UpdatedAt:          now,
	}
	if err := validateAttendancePolicy(policy); err != nil {
		return AttendancePolicy{}, err
	}
	return policy, nil
}

// defaultAttendancePolicyResponse 處理預設考勤政策回應。
func defaultAttendancePolicyResponse() AttendancePolicyResponse {
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
			WeekendOptions:    attendancePolicyWeekendOptions(),
			CycleStartOptions: attendancePolicyCycleStartOptions(),
			CycleEndOptions:   attendancePolicyCycleEndOptions(),
		},
		LeaveTypes: attendancePolicyLeaveTypes(),
	}
}

// attendancePolicyResponse 處理考勤政策回應。
func attendancePolicyResponse(policy AttendancePolicy) AttendancePolicyResponse {
	workTime := normalizeAttendancePolicyWorkTime(policy.WorkTime)
	leaveTypes := normalizeAttendanceLeaveTypes(policy.LeaveTypes)
	if len(leaveTypes) == 0 {
		leaveTypes = attendancePolicyLeaveTypes()
	}
	return AttendancePolicyResponse{WorkTime: workTime, LeaveTypes: leaveTypes}
}

// normalizeAttendancePolicyWorkTime 正規化考勤政策 work 時間。
func normalizeAttendancePolicyWorkTime(workTime AttendancePolicyWorkTime) AttendancePolicyWorkTime {
	defaults := defaultAttendancePolicyResponse().WorkTime
	out := AttendancePolicyWorkTime{
		StandardStart:     strings.TrimSpace(workTime.StandardStart),
		StandardEnd:       strings.TrimSpace(workTime.StandardEnd),
		BreakStart:        strings.TrimSpace(workTime.BreakStart),
		BreakEnd:          strings.TrimSpace(workTime.BreakEnd),
		Weekend:           strings.TrimSpace(workTime.Weekend),
		CycleStart:        strings.TrimSpace(workTime.CycleStart),
		CycleEnd:          strings.TrimSpace(workTime.CycleEnd),
		TimeOptions:       defaults.TimeOptions,
		WeekendOptions:    defaults.WeekendOptions,
		CycleStartOptions: defaults.CycleStartOptions,
		CycleEndOptions:   defaults.CycleEndOptions,
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
		next := AttendanceLeaveType{
			Code:  strings.TrimSpace(item.Code),
			Name:  strings.TrimSpace(item.Name),
			Quota: strings.TrimSpace(item.Quota),
			Rule:  strings.TrimSpace(item.Rule),
			Proof: strings.TrimSpace(item.Proof),
		}
		if next.Code == "" && next.Name == "" {
			continue
		}
		out = append(out, next)
	}
	return out
}

// validateAttendancePolicy 驗證考勤政策。
func validateAttendancePolicy(policy AttendancePolicy) error {
	if !stringInSlice(policy.WorkTime.StandardStart, policy.WorkTime.TimeOptions) || !stringInSlice(policy.WorkTime.StandardEnd, policy.WorkTime.TimeOptions) {
		return BadRequest("standard time must use a configured time option")
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
	for _, item := range policy.LeaveTypes {
		if item.Code == "" || item.Name == "" {
			return BadRequest("leave type code and name are required")
		}
		if _, ok := seen[item.Code]; ok {
			return BadRequest("leave type code must be unique")
		}
		seen[item.Code] = struct{}{}
	}
	return nil
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

// twoDigit 處理 two digit。
func twoDigit(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}
