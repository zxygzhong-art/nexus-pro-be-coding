package service

import (
	"encoding/json"
	"fmt"
	"math"
	"nexus-pro-api/internal/utils"
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

// workspaceLeaveLegend projects enabled system leave types for workspace reports.
func workspaceLeaveLegend(leaveTypes []LeaveType) []WorkspaceLeaveLegendItem {
	legend := make([]WorkspaceLeaveLegendItem, 0, len(leaveTypes))
	for _, leaveType := range leaveTypes {
		if !leaveType.Enabled {
			continue
		}
		code := normalizeLeaveTypeCode(leaveType.Code)
		name := firstNonEmptyString(leaveType.NameZH, leaveType.NameEN, code)
		if code == "" || name == "" {
			continue
		}
		legend = append(legend, WorkspaceLeaveLegendItem{
			Code:  code,
			Label: name,
		})
	}
	return legend
}

// workspaceAuditLogQueryEmpty 處理工作區稽覈 log 查詢空值。
func workspaceAuditLogQueryEmpty(query WorkspaceAuditLogQuery) bool {
	return strings.TrimSpace(query.OperatorID) == "" &&
		strings.TrimSpace(query.Type) == "" &&
		strings.TrimSpace(query.From) == "" &&
		strings.TrimSpace(query.To) == "" &&
		strings.TrimSpace(query.Keyword) == ""
}

// workspaceAuditLogProjection 處理工作區稽覈 log projection。
func workspaceAuditLogProjection(log AuditLog, accounts map[string]Account, employees map[string]Employee) WorkspaceAuditLog {
	log = sanitizeAuditLog(log)
	account := accounts[log.ActorAccountID]
	employee := employees[account.EmployeeID]
	return WorkspaceAuditLog{
		ID:       log.ID,
		Time:     apiTimestamp(log.CreatedAt),
		Operator: workspaceAuditOperator(log, account, employee),
		Type:     workspaceAuditType(log),
		Action:   workspaceAuditAction(log),
		Detail:   workspaceAuditDetail(log),
	}
}

// workspaceAuditOperator 處理工作區稽覈 operator。
func workspaceAuditOperator(log AuditLog, account Account, employee Employee) string {
	return utils.FirstNonEmpty(employee.Name, account.DisplayName, account.Email, log.ActorAccountID, "系統")
}

// workspaceAuditType maps raw audit resources and actions to the stable workspace category catalog.
func workspaceAuditType(log AuditLog) string {
	text := strings.ToLower(strings.Join([]string{log.Resource, log.Action}, " "))
	switch {
	case strings.Contains(text, "employee"):
		return "員工管理"
	case strings.Contains(text, "org") || strings.Contains(text, "position"):
		return "組織架構"
	case strings.Contains(text, "attendance") || strings.Contains(text, "leave") || strings.Contains(text, "clock") || strings.Contains(text, "shift"):
		return "假勤制度"
	case strings.Contains(text, "form") || strings.Contains(text, "workflow"):
		return "表單設計"
	case strings.Contains(text, "iam") || strings.Contains(text, "authz") || strings.Contains(text, "permission") || strings.Contains(text, "admin"):
		return "管理員設定"
	default:
		return "系統"
	}
}

// workspaceAuditAction 處理工作區稽覈 action。
func workspaceAuditAction(log AuditLog) string {
	action := utils.FirstNonEmpty(log.Action, log.Resource)
	if log.Target != "" {
		return action + " " + log.Target
	}
	return action
}

// workspaceAuditDetail 處理工作區稽覈 detail。
func workspaceAuditDetail(log AuditLog) string {
	details := sanitizeAuditDetails(log.Details)
	if len(details) > 0 {
		if raw, err := json.Marshal(details); err == nil {
			return string(raw)
		}
	}
	return utils.FirstNonEmpty(sanitizeAuditText(log.Result), sanitizeAuditText(log.TraceID))
}

// workspaceSummaryLeaveCells 直接投影 eHRMS 每日假勤事實，避免從請假區間推算日期或分攤時數。
func workspaceSummaryLeaveCells(summaries []AttendanceDailySummary, legend []WorkspaceLeaveLegendItem) map[string]map[string]workspaceLeaveCell {
	out := map[string]map[string]workspaceLeaveCell{}
	for _, summary := range summaries {
		if summary.EmployeeID == "" || summary.WorkDate == "" {
			continue
		}
		cell := workspaceLeaveCell{}
		if summary.LeaveCounted && summary.LeaveHours > 0 {
			if leaveType, ok := workspacePolicyLeaveType(summary.LeaveType, legend); ok {
				cell.Code, cell.Label = leaveType.Code, leaveType.Label
				cell.Hours = summary.LeaveHours
			}
		}
		if summary.Leave2Counted && summary.Leave2Hours > 0 {
			if leaveType, ok := workspacePolicyLeaveType(summary.Leave2Type, legend); ok {
				if cell.Code == "" {
					cell.Code, cell.Label = leaveType.Code, leaveType.Label
				}
				cell.Hours += summary.Leave2Hours
			}
		}
		if cell.Hours <= 0 {
			continue
		}
		if out[summary.EmployeeID] == nil {
			out[summary.EmployeeID] = map[string]workspaceLeaveCell{}
		}
		out[summary.EmployeeID][summary.WorkDate] = cell
	}
	return out
}

// workspaceMergeApprovedLeaveCells merges local approved leave without double-counting an eHRMS daily fact.
func workspaceMergeApprovedLeaveCells(existing map[string]map[string]workspaceLeaveCell, leaves []LeaveRequest, workTime AttendancePolicyWorkTime, legend []WorkspaceLeaveLegendItem, start, end time.Time) map[string]map[string]workspaceLeaveCell {
	for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		date := day.Format(time.DateOnly)
		schedule, breaks := attendanceScheduleIntervals(date, workTime)
		for _, leave := range leaves {
			if leave.EmployeeID == "" || !strings.EqualFold(strings.TrimSpace(leave.Status), "approved") {
				continue
			}
			leaveType, ok := workspacePolicyLeaveType(leave.LeaveType, legend)
			if !ok {
				continue
			}
			approved, _ := attendanceLeaveIntervals([]LeaveRequest{leave}, schedule, breaks)
			hours := float64(intervalMinutes(approved)) / 60
			if hours <= 0 {
				continue
			}
			if existing[leave.EmployeeID] == nil {
				existing[leave.EmployeeID] = map[string]workspaceLeaveCell{}
			}
			cell := existing[leave.EmployeeID][date]
			if cell.Code == "" {
				cell.Code, cell.Label = leaveType.Code, leaveType.Label
			}
			// eHRMS and local workflow can describe the same leave, so use the larger daily fact.
			if hours > cell.Hours {
				cell.Hours = hours
			}
			existing[leave.EmployeeID][date] = cell
		}
	}
	return existing
}

// workspaceOvertimeCells 處理工作區加班儲存格。僅累計覈準加班的每日時數。
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

// workspaceAttendanceEvidence preserves raw work, the daily normal-hours ceiling,
// and the upstream candidate that may be credited before leave/day-type caps apply.
type workspaceAttendanceEvidence struct {
	ActualHours    float64
	MaxHours       float64
	CandidateHours float64
	Source         string
}

func workspaceDailyMaxHours(summary AttendanceDailySummary) float64 {
	switch {
	case summary.DailyHours > 0:
		return summary.DailyHours
	case summary.ShiftHours > 0:
		return summary.ShiftHours
	default:
		return workspaceDayHours
	}
}

func workspaceRoundHours(value float64) float64 {
	return math.Round(math.Max(0, value)*100) / 100
}

// workspaceAttendanceEvidenceCells prefers effective local punches for actual
// work while retaining eHRMS daily/shift ceilings for the same employee-day.
func workspaceAttendanceEvidenceCells(clocks []AttendanceClockRecord, summaries []AttendanceDailySummary, leaves []LeaveRequest, workTime AttendancePolicyWorkTime) map[string]map[string]workspaceAttendanceEvidence {
	recordsByEmployeeDate := map[string]map[string][]AttendanceClockRecord{}
	for _, record := range clocks {
		if record.EmployeeID == "" || record.WorkDate == "" || record.Voided || !strings.EqualFold(record.RecordStatus, clockRecordStatusAccepted) {
			continue
		}
		if recordsByEmployeeDate[record.EmployeeID] == nil {
			recordsByEmployeeDate[record.EmployeeID] = map[string][]AttendanceClockRecord{}
		}
		recordsByEmployeeDate[record.EmployeeID][record.WorkDate] = append(recordsByEmployeeDate[record.EmployeeID][record.WorkDate], record)
	}
	leavesByEmployee := map[string][]LeaveRequest{}
	for _, leave := range leaves {
		if leave.EmployeeID != "" {
			leavesByEmployee[leave.EmployeeID] = append(leavesByEmployee[leave.EmployeeID], leave)
		}
	}

	out := map[string]map[string]workspaceAttendanceEvidence{}
	for _, summary := range summaries {
		if summary.EmployeeID == "" || summary.WorkDate == "" {
			continue
		}
		actualHours := math.Max(0, summary.ClockHours)
		candidateHours := actualHours
		if summary.AttendCounted {
			candidateHours = math.Max(0, summary.AttendHours)
			if actualHours == 0 {
				actualHours = candidateHours
			}
		}
		if out[summary.EmployeeID] == nil {
			out[summary.EmployeeID] = map[string]workspaceAttendanceEvidence{}
		}
		out[summary.EmployeeID][summary.WorkDate] = workspaceAttendanceEvidence{
			ActualHours:    actualHours,
			MaxHours:       workspaceDailyMaxHours(summary),
			CandidateHours: candidateHours,
			Source:         "ehrms",
		}
	}
	for employeeID, recordsByDate := range recordsByEmployeeDate {
		if out[employeeID] == nil {
			out[employeeID] = map[string]workspaceAttendanceEvidence{}
		}
		for workDate, records := range recordsByDate {
			asOf := records[0].ClockedAt
			for _, record := range records[1:] {
				if record.ClockedAt.After(asOf) {
					asOf = record.ClockedAt
				}
			}
			projection := ProjectAttendanceDay(records, leavesByEmployee[employeeID], workDate, workTime, asOf)
			actualHours := math.Max(0, float64(projection.WorkedMinutes)/60)
			evidence := out[employeeID][workDate]
			if evidence.MaxHours <= 0 {
				evidence.MaxHours = workspaceDayHours
			}
			evidence.ActualHours = actualHours
			evidence.CandidateHours = actualHours
			evidence.Source = "clock"
			out[employeeID][workDate] = evidence
		}
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
		if record.EmployeeID == "" || record.WorkDate == "" || record.Voided || !strings.EqualFold(record.RecordStatus, clockRecordStatusAccepted) {
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
				cell.In = p.in.ClockedAt.In(attendanceClockLocation).Format("15:04")
				cell.InLoc = utils.FirstNonEmpty(worksiteNames[p.in.WorksiteID], p.in.WorksiteID)
			}
			if p.out != nil {
				cell.Out = p.out.ClockedAt.In(attendanceClockLocation).Format("15:04")
				cell.OutLoc = utils.FirstNonEmpty(worksiteNames[p.out.WorksiteID], p.out.WorksiteID)
			}
			switch {
			case p.in != nil && p.in.RejectionReason != "":
				cell.Abnormal = true
				cell.Reason = utils.FirstNonEmpty(p.in.RejectionReason, "上班卡未通過")
			case p.out != nil && p.out.RejectionReason != "":
				cell.Abnormal = true
				cell.Reason = utils.FirstNonEmpty(p.out.RejectionReason, "下班卡未通過")
			case p.in == nil && p.out != nil:
				cell.Abnormal = true
				cell.Reason = "缺上班卡"
			case p.in != nil && p.out == nil:
				cell.Abnormal = true
				cell.Reason = "缺下班卡"
			case p.in != nil && p.out != nil && p.out.ClockedAt.Sub(p.in.ClockedAt).Hours() < workspaceDayHours:
				if !workspaceShortHoursExempted(employeeID, date, p.out.ClockedAt.Sub(p.in.ClockedAt).Hours(), workspaceDayHours, leaveCells, overtimeCells) {
					cell.Abnormal = true
					cell.Reason = "工時未滿 8 小時"
				}
			}
			out[employeeID][date] = cell
		}
	}
	for _, summary := range summaries {
		if summary.EmployeeID == "" || summary.WorkDate == "" {
			continue
		}
		if out[summary.EmployeeID] == nil {
			out[summary.EmployeeID] = map[string]workspaceClockCell{}
		}
		if _, exists := out[summary.EmployeeID][summary.WorkDate]; exists {
			continue
		}
		cell, ok := workspaceClockCellFromSummary(summary, leaveCells, overtimeCells)
		if !ok {
			continue
		}
		out[summary.EmployeeID][summary.WorkDate] = cell
	}
	return out
}

// workspaceClockCellFromSummary 將 eHRMS 日彙總的打卡時間與缺卡、工時不足狀態投影到工作區矩陣。
func workspaceClockCellFromSummary(summary AttendanceDailySummary, leaveCells map[string]map[string]workspaceLeaveCell, overtimeCells map[string]map[string]float64) (workspaceClockCell, bool) {
	clockIn := utils.FirstNonEmpty(summary.ClockStart, summary.AttendStart)
	clockOut := utils.FirstNonEmpty(summary.ClockEnd, summary.AttendEnd)
	if clockIn == "" && clockOut == "" && summary.ClockHours <= 0 {
		return workspaceClockCell{}, false
	}
	cell := workspaceClockCell{In: clockIn, Out: clockOut}
	switch {
	case clockIn == "" && clockOut != "":
		cell.Abnormal = true
		cell.Reason = "缺上班卡"
	case clockIn != "" && clockOut == "":
		cell.Abnormal = true
		cell.Reason = "缺下班卡"
	default:
		expectedHours := summary.DailyHours
		if expectedHours <= 0 {
			expectedHours = summary.ShiftHours
		}
		if expectedHours > 0 && summary.ClockHours+0.01 < expectedHours && !workspaceShortHoursExempted(summary.EmployeeID, summary.WorkDate, summary.ClockHours, expectedHours, leaveCells, overtimeCells) {
			cell.Abnormal = true
			cell.Reason = "工時未達應出勤時數"
		}
	}
	return cell, true
}

// workspaceShortHoursExempted 判斷工時不足是否可由覈準的請假或加班補足當日應出勤時數。
func workspaceShortHoursExempted(employeeID string, date string, workedHours float64, expectedHours float64, leaveCells map[string]map[string]workspaceLeaveCell, overtimeCells map[string]map[string]float64) bool {
	leaveHours := 0.0
	if cell, ok := leaveCells[employeeID][date]; ok {
		leaveHours = cell.Hours
	}
	if workedHours+leaveHours >= expectedHours {
		return true
	}
	// 週末或假日的打卡若對應覈準加班，不視為工時異常。
	if overtimeCells[employeeID][date] > 0 {
		if day, err := time.Parse(time.DateOnly, date); err == nil {
			if dow := day.Weekday(); dow == time.Saturday || dow == time.Sunday {
				return true
			}
		}
	}
	return false
}

// workspaceAttendanceMatrix projects explicit attendance facts and marks only elapsed eligible workdays absent.
func workspaceAttendanceMatrix(employees []Employee, cards map[string]WorkspaceEmployeeCard, dates []WorkspaceDate, leaveCells map[string]map[string]workspaceLeaveCell, overtimeCells map[string]map[string]float64, attendanceEvidence map[string]map[string]workspaceAttendanceEvidence, clockCells map[string]map[string]workspaceClockCell, now time.Time) WorkspaceAttendanceMatrix {
	rows := []WorkspaceAttendanceRow{}
	totalLeaveHours := 0.0
	totalOvertimeHours := 0.0
	perfect := 0
	workdays := workspaceWorkdayCount(dates)
	holidays := workspaceHolidayCount(dates)
	todayKey := now.In(attendanceClockLocation).Format(time.DateOnly)
	for _, employee := range employees {
		row := WorkspaceAttendanceRow{Employee: cards[employee.ID], Cells: make([]WorkspaceDayCell, 0, len(dates)), Summary: WorkspaceEmployeeHours{LeaveByType: map[string]float64{}}}
		hasAbsence := false
		hasRecordedAttendance := false
		for _, date := range dates {
			cell := workspaceBaseDayCell(date)
			eligible := workspaceEmployeeEligibleOnDate(employee, date)
			isWorkday := cell.Type == "work"
			if cell.Type == "work" && !eligible {
				cell.Type = "empty"
			}
			evidence, hasEvidence := attendanceEvidence[employee.ID][date.Key]
			dailyMaxHours := workspaceDayHours
			if hasEvidence && evidence.MaxHours > 0 {
				dailyMaxHours = evidence.MaxHours
			}
			if isWorkday && eligible {
				row.Summary.WorkDays++
				row.Summary.DueHours += dailyMaxHours
			}
			leaveHours := 0.0
			if leave, ok := leaveCells[employee.ID][date.Key]; ok && eligible {
				cell.Type = "leave"
				cell.Leave = leave.Code
				cell.Hours = leave.Hours
				cell.Label = leave.Label
				leaveHours = leave.Hours
				row.Summary.LeaveHours += leave.Hours
				row.Summary.LeaveByType[leave.Code] += leave.Hours
			}
			if overtime := overtimeCells[employee.ID][date.Key]; overtime > 0 && eligible {
				cell.Overtime = overtime
				row.Summary.OvertimeHours += overtime
			}
			if hasEvidence && eligible {
				effectiveMaxHours := 0.0
				if isWorkday {
					effectiveMaxHours = dailyMaxHours
				}
				availableNormalHours := math.Max(0, effectiveMaxHours-leaveHours)
				countedHours := math.Min(math.Max(0, evidence.CandidateHours), availableNormalHours)
				cell.ActualHours = workspaceRoundHours(evidence.ActualHours)
				cell.MaxHours = workspaceRoundHours(effectiveMaxHours)
				cell.CountedHours = workspaceRoundHours(countedHours)
				row.Summary.ActualHours += math.Max(0, evidence.ActualHours)
				row.Summary.AttendedHours += countedHours
				if cell.Type == "work" && evidence.ActualHours > 0 {
					cell.Hours = cell.CountedHours
					if evidence.Source == "ehrms" {
						cell.Label = "eHRMS"
						cell.Recorded = true
					}
				}
			}
			if clock, ok := clockCells[employee.ID][date.Key]; ok && cell.Type == "work" && (clock.In != "" || clock.Out != "") {
				cell.Recorded = true
				if cell.Label == "" {
					cell.Label = "打卡"
				}
			}
			if cell.Type == "work" {
				switch {
				case cell.Recorded:
					hasRecordedAttendance = true
				case date.Key < todayKey:
					cell.Type = "absence"
					cell.Label = "缺勤"
					hasAbsence = true
				}
			}
			row.Cells = append(row.Cells, cell)
		}
		row.Summary.ActualHours = workspaceRoundHours(row.Summary.ActualHours)
		row.Summary.AttendedHours = workspaceRoundHours(row.Summary.AttendedHours)
		row.Summary.DueHours = workspaceRoundHours(row.Summary.DueHours)
		if row.Summary.LeaveHours == 0 && !hasAbsence && hasRecordedAttendance {
			perfect++
		}
		totalLeaveHours += row.Summary.LeaveHours
		totalOvertimeHours += row.Summary.OvertimeHours
		rows = append(rows, row)
	}
	return WorkspaceAttendanceMatrix{Rows: rows, Summary: WorkspaceAttendanceMatrixSum{Holidays: holidays, LeaveHours: totalLeaveHours, OvertimeHours: totalOvertimeHours, Perfect: perfect, Workdays: workdays}}
}

// workspaceEmployeeEligibleOnDate avoids absence marks before hire, after resignation, or outside active employment.
func workspaceEmployeeEligibleOnDate(employee Employee, date WorkspaceDate) bool {
	status := workspaceEmployeeStatus(employee)
	switch status {
	case string(EmployeeStatusDeleted), string(EmployeeStatusOnboarding), string(EmployeeStatusLeaveSuspended):
		return false
	case string(EmployeeStatusResigned):
		if employee.ResignDate == nil {
			return false
		}
	}
	if employee.HireDate != nil && date.Key < employee.HireDate.In(attendanceClockLocation).Format(time.DateOnly) {
		return false
	}
	if employee.ResignDate != nil && date.Key > employee.ResignDate.In(attendanceClockLocation).Format(time.DateOnly) {
		return false
	}
	return true
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
func workspaceClockMatrix(employees []Employee, cards map[string]WorkspaceEmployeeCard, dates []WorkspaceDate, leaveCells map[string]map[string]workspaceLeaveCell, clockCells map[string]map[string]workspaceClockCell, includeAbnormals bool) WorkspaceClockMatrix {
	rows := []WorkspaceClockRow{}
	var abnormals []WorkspaceClockAbnormal
	if includeAbnormals {
		abnormals = []WorkspaceClockAbnormal{}
	}
	abnormalPeople := map[string]struct{}{}
	abnormalDays := 0
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
					abnormalDays++
					abnormalPeople[employee.ID] = struct{}{}
					if includeAbnormals {
						abnormals = append(abnormals, WorkspaceClockAbnormal{Date: date, Employee: cards[employee.ID], Record: cell})
					}
				} else if cell.Type == "work" {
					normalDays++
				}
			}
			row.Cells = append(row.Cells, cell)
		}
		rows = append(rows, row)
	}
	if includeAbnormals {
		sort.SliceStable(abnormals, func(i, j int) bool {
			if abnormals[i].Date.Key != abnormals[j].Date.Key {
				return abnormals[i].Date.Key < abnormals[j].Date.Key
			}
			return abnormals[i].Employee.ID < abnormals[j].Employee.ID
		})
	}
	return WorkspaceClockMatrix{Rows: rows, Abnormals: abnormals, Summary: WorkspaceClockSummary{AbnormalDays: abnormalDays, AbnormalPeople: len(abnormalPeople), NormalDays: normalDays}}
}

// workspaceEmployeeCards 處理工作區員工 cards。
func workspaceEmployeeCards(employees []Employee, orgNames map[string]string) map[string]WorkspaceEmployeeCard {
	out := map[string]WorkspaceEmployeeCard{}
	for _, employee := range employees {
		out[employee.ID] = WorkspaceEmployeeCard{
			ID:           workspaceEmployeeDisplayID(employee),
			EmployeeID:   employee.ID,
			Avatar:       workspaceAvatar(employee.Name),
			NameZH:       employee.Name,
			NameEN:       workspaceEmployeeNameEN(employee),
			Email:        employee.CompanyEmail,
			DepartmentID: employee.OrgUnitID,
			Dept:         workspaceOrgName(orgNames, employee.OrgUnitID),
			Title:        employee.Position,
			Type:         workspaceCategoryLabel(employee.Category),
			Phone:        employee.Phone,
			Status:       workspaceStatusLabel(workspaceEmployeeStatus(employee)),
			HireDate:     workspaceFormatDateSlash(employee.HireDate),
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

func workspacePolicyLeaveType(raw string, legend []WorkspaceLeaveLegendItem) (WorkspaceLeaveLegendItem, bool) {
	normalized := normalizeLeaveTypeCode(raw)
	trimmed := strings.TrimSpace(raw)
	for _, item := range legend {
		if strings.EqualFold(item.Code, normalized) || strings.EqualFold(item.Code, trimmed) || strings.EqualFold(item.Label, trimmed) {
			return item, true
		}
	}
	return WorkspaceLeaveLegendItem{}, false
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

// workspaceRateString 處理工作區速率字串。
func workspaceRateString(numerator float64, denominator float64) string {
	if denominator <= 0 {
		return "0.0"
	}
	return fmt.Sprintf("%.1f", numerator/denominator*100)
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

// Workspace 處理工作區 aggregate 的服務流程。
