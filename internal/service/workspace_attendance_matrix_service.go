package service

import (
	"encoding/json"
	"fmt"
	"math"
	"nexus-pro-be/internal/utils"
	"sort"
	"strings"
	"time"
)

func workspaceMonthDates(start time.Time, end time.Time) []WorkspaceDate {
	out := []WorkspaceDate{}
	for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
		out = append(out, WorkspaceDate{
			Key:     day.Format(time.DateOnly),
			Y:       day.Year(),
			M:       int(day.Month()),
			D:       day.Day(),
			DOW:     int(day.Weekday()),
			Holiday: workspaceHolidayName(day),
		})
	}
	return out
}

// workspaceHolidayName 處理工作區假日名稱。
func workspaceHolidayName(_ time.Time) *string {
	return nil
}

// workspaceLeaveLegend 處理工作區請假 legend。
func workspaceLeaveLegend() []WorkspaceLeaveLegendItem {
	return []WorkspaceLeaveLegendItem{
		{Code: "病", Label: "全薪病假"},
		{Code: "彈", Label: "彈性休假"},
		{Code: "事", Label: "事假"},
		{Code: "照", Label: "家庭照顧假"},
		{Code: "半", Label: "半薪病假"},
		{Code: "理", Label: "生理假"},
		{Code: "婚", Label: "婚假"},
		{Code: "產", Label: "八週產假"},
		{Code: "陪", Label: "陪產假"},
		{Code: "喪", Label: "喪假"},
		{Code: "公", Label: "公假"},
		{Code: "檢", Label: "產檢假"},
		{Code: "補", Label: "補休假"},
		{Code: "特", Label: "特休假"},
	}
}

// workspaceAuditLogMatches 處理工作區稽核 log matches。
func workspaceAuditLogMatches(log AuditLog, query WorkspaceAuditLogQuery, accounts map[string]Account, employees map[string]Employee) bool {
	if from, ok := workspaceParseAuditTime(query.From, false); ok && log.CreatedAt.Before(from) {
		return false
	}
	if to, ok := workspaceParseAuditTime(query.To, true); ok && !log.CreatedAt.Before(to) {
		return false
	}
	account := accounts[log.ActorAccountID]
	employee := employees[account.EmployeeID]
	projected := workspaceAuditLogProjection(log, accounts, employees)
	operatorID := strings.TrimSpace(query.OperatorID)
	if operatorID != "" && operatorID != log.ActorAccountID && operatorID != account.EmployeeID && operatorID != workspaceEmployeeDisplayID(employee) && operatorID != projected.Operator {
		return false
	}
	if filterType := strings.ToLower(strings.TrimSpace(query.Type)); filterType != "" {
		haystack := strings.ToLower(strings.Join([]string{projected.Type, log.Resource, log.Action}, " "))
		if !strings.Contains(haystack, filterType) {
			return false
		}
	}
	if keyword := strings.ToLower(strings.TrimSpace(query.Keyword)); keyword != "" {
		haystack := strings.ToLower(strings.Join([]string{projected.Operator, projected.Type, projected.Action, projected.Detail, log.Resource, log.Target, log.Action}, " "))
		if !strings.Contains(haystack, keyword) {
			return false
		}
	}
	return true
}

// workspaceAuditLogQueryEmpty 處理工作區稽核 log 查詢空值。
func workspaceAuditLogQueryEmpty(query WorkspaceAuditLogQuery) bool {
	return strings.TrimSpace(query.OperatorID) == "" &&
		strings.TrimSpace(query.Type) == "" &&
		strings.TrimSpace(query.From) == "" &&
		strings.TrimSpace(query.To) == "" &&
		strings.TrimSpace(query.Keyword) == ""
}

// workspaceAuditLogProjection 處理工作區稽核 log projection。
func workspaceAuditLogProjection(log AuditLog, accounts map[string]Account, employees map[string]Employee) WorkspaceAuditLog {
	account := accounts[log.ActorAccountID]
	employee := employees[account.EmployeeID]
	return WorkspaceAuditLog{
		ID:       log.ID,
		Time:     log.CreatedAt.UTC().Format("2006/01/02 15:04"),
		Operator: workspaceAuditOperator(log, account, employee),
		Type:     workspaceAuditType(log),
		Action:   workspaceAuditAction(log),
		Detail:   workspaceAuditDetail(log),
	}
}

// workspaceAuditOperator 處理工作區稽核 operator。
func workspaceAuditOperator(log AuditLog, account Account, employee Employee) string {
	return utils.FirstNonEmpty(employee.Name, account.DisplayName, account.Email, log.ActorAccountID, "系統")
}

// workspaceAuditType 處理工作區稽核 type。
func workspaceAuditType(log AuditLog) string {
	text := strings.ToLower(strings.Join([]string{log.Resource, log.Action}, " "))
	switch {
	case strings.Contains(text, "employee"):
		return "員工管理"
	case strings.Contains(text, "org"):
		return "組織架構"
	case strings.Contains(text, "attendance") || strings.Contains(text, "leave") || strings.Contains(text, "clock") || strings.Contains(text, "shift"):
		return "假勤制度"
	case strings.Contains(text, "form") || strings.Contains(text, "workflow"):
		return "表單設計"
	case strings.Contains(text, "iam") || strings.Contains(text, "permission") || strings.Contains(text, "admin"):
		return "管理員設定"
	default:
		return "系統"
	}
}

// workspaceAuditAction 處理工作區稽核 action。
func workspaceAuditAction(log AuditLog) string {
	action := utils.FirstNonEmpty(log.Action, log.Resource)
	if log.Target != "" {
		return action + " " + log.Target
	}
	return action
}

// workspaceAuditDetail 處理工作區稽核 detail。
func workspaceAuditDetail(log AuditLog) string {
	if len(log.Details) > 0 {
		if raw, err := json.Marshal(log.Details); err == nil {
			return string(raw)
		}
	}
	return utils.FirstNonEmpty(log.Result, log.TraceID)
}

// workspaceParseAuditTime 處理工作區 parse 稽核時間。
func workspaceParseAuditTime(value string, exclusiveEnd bool) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.DateOnly, value); err == nil {
		if exclusiveEnd {
			parsed = parsed.AddDate(0, 0, 1)
		}
		return parsed.UTC(), true
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC(), true
	}
	return time.Time{}, false
}

// workspaceLeaveCells 處理工作區請假儲存格。
func workspaceLeaveCells(leaves []LeaveRequest, start time.Time, end time.Time) map[string]map[string]workspaceLeaveCell {
	out := map[string]map[string]workspaceLeaveCell{}
	for _, leave := range leaves {
		if leave.EmployeeID == "" {
			continue
		}
		first := maxTime(workspaceDateOnly(leave.StartAt), start)
		last := minTime(workspaceDateOnly(leave.EndAt), end.AddDate(0, 0, -1))
		if last.Before(first) {
			continue
		}
		days := int(last.Sub(first).Hours()/24) + 1
		hours := leave.Hours
		if hours <= 0 {
			hours = float64(days) * workspaceDayHours
		}
		hoursPerDay := math.Min(workspaceDayHours, hours/float64(days))
		code, label := workspaceLeaveCodeLabel(leave.LeaveType)
		if out[leave.EmployeeID] == nil {
			out[leave.EmployeeID] = map[string]workspaceLeaveCell{}
		}
		for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
			key := day.Format(time.DateOnly)
			cell := out[leave.EmployeeID][key]
			if cell.Code == "" {
				cell.Code = code
				cell.Label = label
			}
			cell.Hours += hoursPerDay
			out[leave.EmployeeID][key] = cell
		}
	}
	return out
}

// workspaceOvertimeCells 處理工作區加班儲存格。僅累計核准加班的每日時數。
func workspaceOvertimeCells(overtimes []OvertimeRequest, start time.Time, end time.Time) map[string]map[string]float64 {
	out := map[string]map[string]float64{}
	for _, overtime := range overtimes {
		if overtime.EmployeeID == "" || overtime.Hours <= 0 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(overtime.Status), "approved") {
			continue
		}
		day := workspaceDateOnly(overtime.StartAt)
		if day.Before(start) || !day.Before(end) {
			continue
		}
		key := day.Format(time.DateOnly)
		if overtime.WorkDate != "" {
			if parsed, err := time.Parse(time.DateOnly, overtime.WorkDate); err == nil && !parsed.Before(start) && parsed.Before(end) {
				key = overtime.WorkDate
			}
		}
		if out[overtime.EmployeeID] == nil {
			out[overtime.EmployeeID] = map[string]float64{}
		}
		out[overtime.EmployeeID][key] += overtime.Hours
	}
	return out
}

// workspaceClockCells 處理工作區打卡儲存格。
func workspaceClockCells(clocks []AttendanceClockRecord, summaries []AttendanceDailySummary, worksites []AttendanceWorksite, leaveCells map[string]map[string]workspaceLeaveCell, overtimeCells map[string]map[string]float64) map[string]map[string]workspaceClockCell {
	worksiteNames := map[string]string{}
	for _, worksite := range worksites {
		worksiteNames[worksite.ID] = worksite.Name
	}
	type pair struct {
		in  *AttendanceClockRecord
		out *AttendanceClockRecord
	}
	pairs := map[string]map[string]*pair{}
	for _, record := range clocks {
		if record.EmployeeID == "" || record.WorkDate == "" {
			continue
		}
		if pairs[record.EmployeeID] == nil {
			pairs[record.EmployeeID] = map[string]*pair{}
		}
		p := pairs[record.EmployeeID][record.WorkDate]
		if p == nil {
			p = &pair{}
			pairs[record.EmployeeID][record.WorkDate] = p
		}
		rec := record
		switch record.Direction {
		case clockDirectionIn:
			if p.in == nil || record.ClockedAt.Before(p.in.ClockedAt) {
				p.in = &rec
			}
		case clockDirectionOut:
			if p.out == nil || record.ClockedAt.After(p.out.ClockedAt) {
				p.out = &rec
			}
		}
	}
	out := map[string]map[string]workspaceClockCell{}
	for employeeID, byDate := range pairs {
		out[employeeID] = map[string]workspaceClockCell{}
		for date, p := range byDate {
			cell := workspaceClockCell{}
			if p.in != nil {
				cell.In = p.in.ClockedAt.Format("15:04")
				cell.InLoc = utils.FirstNonEmpty(worksiteNames[p.in.WorksiteID], p.in.WorksiteID)
			}
			if p.out != nil {
				cell.Out = p.out.ClockedAt.Format("15:04")
				cell.OutLoc = utils.FirstNonEmpty(worksiteNames[p.out.WorksiteID], p.out.WorksiteID)
			}
			switch {
			case p.in != nil && p.in.RecordStatus == clockRecordStatusRejected:
				cell.Abnormal = true
				cell.Reason = utils.FirstNonEmpty(p.in.RejectionReason, "上班卡未通過")
			case p.out != nil && p.out.RecordStatus == clockRecordStatusRejected:
				cell.Abnormal = true
				cell.Reason = utils.FirstNonEmpty(p.out.RejectionReason, "下班卡未通過")
			case p.in == nil && p.out != nil:
				cell.Abnormal = true
				cell.Reason = "缺上班卡"
			case p.in != nil && p.out == nil:
				cell.Abnormal = true
				cell.Reason = "缺下班卡"
			case p.in != nil && p.out != nil && p.out.ClockedAt.Sub(p.in.ClockedAt).Hours() < workspaceDayHours:
				if !workspaceShortHoursExempted(employeeID, date, p.out.ClockedAt.Sub(p.in.ClockedAt).Hours(), leaveCells, overtimeCells) {
					cell.Abnormal = true
					cell.Reason = "工時未滿 8 小時"
				}
			}
			out[employeeID][date] = cell
		}
	}
	for _, summary := range summaries {
		if summary.EmployeeID == "" || summary.WorkDate == "" || summary.ClockHours <= 0 {
			continue
		}
		if out[summary.EmployeeID] == nil {
			out[summary.EmployeeID] = map[string]workspaceClockCell{}
		}
		if _, exists := out[summary.EmployeeID][summary.WorkDate]; exists {
			continue
		}
		out[summary.EmployeeID][summary.WorkDate] = workspaceClockCell{}
	}
	return out
}

// workspaceShortHoursExempted 判斷工時不足是否可由核准的請假或加班豁免。
func workspaceShortHoursExempted(employeeID string, date string, workedHours float64, leaveCells map[string]map[string]workspaceLeaveCell, overtimeCells map[string]map[string]float64) bool {
	leaveHours := 0.0
	if cell, ok := leaveCells[employeeID][date]; ok {
		leaveHours = cell.Hours
	}
	if workedHours+leaveHours >= workspaceDayHours {
		return true
	}
	// 週末或假日的打卡若對應核准加班，不視為工時異常。
	if overtimeCells[employeeID][date] > 0 {
		if day, err := time.Parse(time.DateOnly, date); err == nil {
			if dow := day.Weekday(); dow == time.Saturday || dow == time.Sunday {
				return true
			}
		}
	}
	return false
}

// workspaceAttendanceMatrix 處理工作區考勤矩陣。
func workspaceAttendanceMatrix(employees []Employee, cards map[string]WorkspaceEmployeeCard, dates []WorkspaceDate, leaveCells map[string]map[string]workspaceLeaveCell, overtimeCells map[string]map[string]float64, summaryCells map[string]map[string]AttendanceDailySummary) WorkspaceAttendanceMatrix {
	rows := []WorkspaceAttendanceRow{}
	totalLeaveHours := 0.0
	totalOvertimeHours := 0.0
	perfect := 0
	workdays := workspaceWorkdayCount(dates)
	holidays := workspaceHolidayCount(dates)
	for _, employee := range employees {
		row := WorkspaceAttendanceRow{Employee: cards[employee.ID], Cells: make([]WorkspaceDayCell, 0, len(dates)), Summary: WorkspaceEmployeeHours{LeaveByType: map[string]float64{}, WorkDays: workdays}}
		for _, date := range dates {
			cell := workspaceBaseDayCell(date)
			if leave, ok := leaveCells[employee.ID][date.Key]; ok {
				cell.Type = "leave"
				cell.Leave = leave.Code
				cell.Hours = leave.Hours
				cell.Label = leave.Label
				row.Summary.LeaveHours += leave.Hours
				row.Summary.LeaveByType[leave.Code] += leave.Hours
			}
			if overtime := overtimeCells[employee.ID][date.Key]; overtime > 0 {
				cell.Overtime = overtime
				row.Summary.OvertimeHours += overtime
			}
			if summary, ok := summaryCells[employee.ID][date.Key]; ok && summary.ClockHours > 0 && cell.Type != "leave" {
				cell.Type = "work"
				cell.Hours = summary.ClockHours
				cell.Label = "eHRMS"
			}
			row.Cells = append(row.Cells, cell)
		}
		row.Summary.DueHours = float64(workdays) * workspaceDayHours
		row.Summary.AttendedHours = math.Max(0, row.Summary.DueHours-row.Summary.LeaveHours-row.Summary.DeductHours) + row.Summary.OvertimeHours
		if row.Summary.LeaveHours == 0 {
			perfect++
		}
		totalLeaveHours += row.Summary.LeaveHours
		totalOvertimeHours += row.Summary.OvertimeHours
		rows = append(rows, row)
	}
	return WorkspaceAttendanceMatrix{Rows: rows, Summary: WorkspaceAttendanceMatrixSum{Holidays: holidays, LeaveHours: totalLeaveHours, OvertimeHours: totalOvertimeHours, Perfect: perfect, Workdays: workdays}}
}

// workspaceSummaryCells 處理 eHRMS 日彙總儲存格。
func workspaceSummaryCells(items []AttendanceDailySummary) map[string]map[string]AttendanceDailySummary {
	out := map[string]map[string]AttendanceDailySummary{}
	for _, item := range items {
		if item.EmployeeID == "" || item.WorkDate == "" {
			continue
		}
		if out[item.EmployeeID] == nil {
			out[item.EmployeeID] = map[string]AttendanceDailySummary{}
		}
		out[item.EmployeeID][item.WorkDate] = item
	}
	return out
}

// workspaceClockMatrix 處理工作區打卡矩陣。
func workspaceClockMatrix(employees []Employee, cards map[string]WorkspaceEmployeeCard, dates []WorkspaceDate, leaveCells map[string]map[string]workspaceLeaveCell, clockCells map[string]map[string]workspaceClockCell) WorkspaceClockMatrix {
	rows := []WorkspaceClockRow{}
	abnormals := []WorkspaceClockAbnormal{}
	abnormalPeople := map[string]struct{}{}
	normalDays := 0
	for _, employee := range employees {
		row := WorkspaceClockRow{Employee: cards[employee.ID], Cells: make([]WorkspaceDayCell, 0, len(dates))}
		for _, date := range dates {
			cell := workspaceBaseDayCell(date)
			if leave, ok := leaveCells[employee.ID][date.Key]; ok {
				cell.Type = "leave"
				cell.Leave = leave.Code
				cell.Hours = leave.Hours
				cell.Label = leave.Label
			}
			if clock, ok := clockCells[employee.ID][date.Key]; ok {
				if cell.Type != "leave" && cell.Type != "holiday" && cell.Type != "weekend" {
					cell.Type = "work"
				}
				cell.In = clock.In
				cell.Out = clock.Out
				cell.InLoc = clock.InLoc
				cell.OutLoc = clock.OutLoc
				cell.Abnormal = clock.Abnormal
				cell.Reason = clock.Reason
				if clock.Abnormal {
					abnormalPeople[employee.ID] = struct{}{}
					abnormals = append(abnormals, WorkspaceClockAbnormal{Date: date, Employee: cards[employee.ID], Record: cell})
				} else if cell.Type == "work" {
					normalDays++
				}
			}
			row.Cells = append(row.Cells, cell)
		}
		rows = append(rows, row)
	}
	sort.SliceStable(abnormals, func(i, j int) bool {
		if abnormals[i].Date.Key != abnormals[j].Date.Key {
			return abnormals[i].Date.Key < abnormals[j].Date.Key
		}
		return abnormals[i].Employee.ID < abnormals[j].Employee.ID
	})
	return WorkspaceClockMatrix{Rows: rows, Abnormals: abnormals, Summary: WorkspaceClockSummary{AbnormalDays: len(abnormals), AbnormalPeople: len(abnormalPeople), NormalDays: normalDays}}
}

// workspaceEmployeeCards 處理工作區員工 cards。
func workspaceEmployeeCards(employees []Employee, orgNames map[string]string) map[string]WorkspaceEmployeeCard {
	out := map[string]WorkspaceEmployeeCard{}
	for _, employee := range employees {
		out[employee.ID] = WorkspaceEmployeeCard{
			ID:       workspaceEmployeeDisplayID(employee),
			Avatar:   workspaceAvatar(employee.Name),
			NameZH:   employee.Name,
			NameEN:   workspaceEmployeeNameEN(employee),
			Email:    employee.CompanyEmail,
			Dept:     workspaceOrgName(orgNames, employee.OrgUnitID),
			Title:    employee.Position,
			Type:     workspaceCategoryLabel(employee.Category),
			Phone:    employee.Phone,
			Status:   workspaceStatusLabel(workspaceEmployeeStatus(employee)),
			HireDate: workspaceFormatDateSlash(employee.HireDate),
		}
	}
	return out
}

// workspaceBaseDayCell 處理工作區 base day 儲存格。
func workspaceBaseDayCell(date WorkspaceDate) WorkspaceDayCell {
	if date.Holiday != nil {
		return WorkspaceDayCell{Type: "holiday", Holiday: *date.Holiday}
	}
	if date.DOW == 0 || date.DOW == 6 {
		return WorkspaceDayCell{Type: "weekend"}
	}
	return WorkspaceDayCell{Type: "work"}
}

// workspaceWorkdayCount 處理工作區 workday count。
func workspaceWorkdayCount(dates []WorkspaceDate) int {
	count := 0
	for _, date := range dates {
		if workspaceBaseDayCell(date).Type == "work" {
			count++
		}
	}
	return count
}

// workspaceHolidayCount 處理工作區假日 count。
func workspaceHolidayCount(dates []WorkspaceDate) int {
	count := 0
	for _, date := range dates {
		if date.Holiday != nil {
			count++
		}
	}
	return count
}

// workspaceLeaveCodeLabel 處理工作區請假碼 label。
func workspaceLeaveCodeLabel(leaveType string) (string, string) {
	normalized := strings.ToLower(strings.TrimSpace(leaveType))
	labels := map[string]WorkspaceLeaveLegendItem{
		"paid_sick":      {Code: "病", Label: "全薪病假"},
		"sick":           {Code: "病", Label: "全薪病假"},
		"flex":           {Code: "彈", Label: "彈性休假"},
		"personal":       {Code: "事", Label: "事假"},
		"family_care":    {Code: "照", Label: "家庭照顧假"},
		"half_sick":      {Code: "半", Label: "半薪病假"},
		"menstrual":      {Code: "理", Label: "生理假"},
		"marriage":       {Code: "婚", Label: "婚假"},
		"maternity":      {Code: "產", Label: "八週產假"},
		"paternity":      {Code: "陪", Label: "陪產假"},
		"bereavement":    {Code: "喪", Label: "喪假"},
		"official":       {Code: "公", Label: "公假"},
		"prenatal_check": {Code: "檢", Label: "產檢假"},
		"compensatory":   {Code: "補", Label: "補休假"},
		"annual":         {Code: "特", Label: "特休假"},
	}
	if item, ok := labels[normalized]; ok {
		return item.Code, item.Label
	}
	trimmed := strings.TrimSpace(leaveType)
	if trimmed == "" {
		return "假", "請假"
	}
	runes := []rune(trimmed)
	return string(runes[0]), trimmed
}

// workspaceEmployeeNameEN 處理工作區員工名稱 en。
func workspaceEmployeeNameEN(employee Employee) string {
	return workspaceStringFromMaps([]map[string]any{employee.BasicInfo, employee.EmploymentInfo, employee.ContactInfo}, "name_en", "english_name", "en_name")
}

// workspaceEmployeeInfoDate 處理工作區員工 info 日期。
func workspaceEmployeeInfoDate(employee Employee, keys ...string) string {
	return workspaceStringFromMaps([]map[string]any{employee.BasicInfo, employee.EmploymentInfo}, keys...)
}

// workspaceStringFromMaps 處理工作區字串 來源 map。
func workspaceStringFromMaps(maps []map[string]any, keys ...string) string {
	for _, values := range maps {
		for _, key := range keys {
			if values == nil {
				continue
			}
			switch v := values[key].(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			case fmt.Stringer:
				if strings.TrimSpace(v.String()) != "" {
					return strings.TrimSpace(v.String())
				}
			}
		}
	}
	return ""
}

// workspaceParseFlexibleDate 處理工作區 parse flexible 日期。
func workspaceParseFlexibleDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.DateOnly, "2006/01/02", time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

// workspaceFormatDateSlash 處理工作區 format 日期 slash。
func workspaceFormatDateSlash(value *time.Time) string {
	if value == nil {
		return ""
	}
	return workspaceFormatTimeSlash(*value)
}

// workspaceFormatTimeSlash 處理工作區 format 時間 slash。
func workspaceFormatTimeSlash(value time.Time) string {
	return value.UTC().Format("2006/01/02")
}

// workspaceAvatar 處理工作區 avatar。
func workspaceAvatar(name string) string {
	runes := []rune(strings.TrimSpace(name))
	if len(runes) == 0 {
		return ""
	}
	return string(runes[0])
}

// workspaceCategoryLabel 處理工作區分類 label。
func workspaceCategoryLabel(category string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case string(EmployeeCategoryFullTime):
		return "全職"
	case string(EmployeeCategoryPartTime):
		return "兼職"
	case string(EmployeeCategoryIntern):
		return "實習"
	case string(EmployeeCategoryContractor):
		return "約聘"
	default:
		return category
	}
}

// workspaceStatusLabel 處理工作區狀態 label。
func workspaceStatusLabel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case string(EmployeeStatusActive), string(EmployeeStatusProbation), string(EmployeeStatusLeaveSuspended), "", "在職", "試用", "試用中", "留停", "留職停薪":
		return "在職"
	case string(EmployeeStatusOnboarding):
		return "待加入"
	case string(EmployeeStatusResigned), string(EmployeeStatusDeleted), "離職", "已離職", "已停用":
		return "已停用"
	default:
		return "已停用"
	}
}

// workspaceDateOnly 處理工作區日期 only。
func workspaceDateOnly(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

// workspaceWeekdayZH 處理工作區星期 zh。
func workspaceWeekdayZH(value time.Time) string {
	labels := []string{"週日", "週一", "週二", "週三", "週四", "週五", "週六"}
	return labels[int(value.Weekday())]
}

// workspaceMonthNameZH 處理工作區月份名稱 zh。
func workspaceMonthNameZH(month time.Month) string {
	names := []string{"", "一月", "二月", "三月", "四月", "五月", "六月", "七月", "八月", "九月", "十月", "十一月", "十二月"}
	return names[int(month)]
}

// workspaceRate 處理工作區速率。
func workspaceRate(numerator int, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator) * 100
}

// workspaceRateString 處理工作區速率字串。
func workspaceRateString(numerator float64, denominator float64) string {
	if denominator <= 0 {
		return "0.0"
	}
	return fmt.Sprintf("%.1f", numerator/denominator*100)
}

// workspaceRateLabel 處理工作區速率 label。
func workspaceRateLabel(numerator int, denominator int) string {
	return fmt.Sprintf("%.1f%%", workspaceRate(numerator, denominator))
}

// workspacePercent 處理工作區百分比。
func workspacePercent(value int, total int) int {
	if total <= 0 {
		return 0
	}
	return int(math.Round(float64(value) / float64(total) * 100))
}

// workspacePercentFloat 處理工作區百分比 float。
func workspacePercentFloat(value float64, total float64) int {
	if total <= 0 {
		return 0
	}
	return int(math.Round(value / total * 100))
}

// sortedKeys 處理 sorted keys。
func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// maxInt 取得較大值整數。
func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

// maxTime 取得較大值時間。
func maxTime(a time.Time, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// minTime 取得較小值時間。
func minTime(a time.Time, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// Workspace 處理工作區 aggregate 的服務流程。
