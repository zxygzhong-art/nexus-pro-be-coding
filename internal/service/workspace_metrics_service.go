package service

import (
	"fmt"
	"math"
	"nexus-pro-be/internal/utils"
	"sort"
	"strconv"
	"strings"
	"time"
)

type workspaceOrgInfo struct {
	ID       string
	Name     string
	ParentID string
	Path     []string
}

type workspaceMovementStats struct {
	BU         string
	Dept       string
	Base       int
	Prev       int
	Hires      int
	Resigned   int
	Layoff     int
	OnLeave    int
	End        int
	YTDSep     int
	YTDHires   int
	YTDEnd     int
	YTDOnLeave int
}

type workspaceLeaveCell struct {
	Code  string
	Label string
	Hours float64
}

type workspaceClockCell struct {
	In       string
	Out      string
	InLoc    string
	OutLoc   string
	Abnormal bool
	Reason   string
}

// workspaceMonthRange 處理工作區月份 range。
func workspaceMonthRange(year int, month int, now time.Time) (time.Time, time.Time) {
	if year <= 0 {
		year = now.Year()
	}
	if month < 1 || month > 12 {
		month = int(now.Month())
	}
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0)
}

// workspaceTargetDate 處理工作區 target 日期。
func workspaceTargetDate(raw string, start time.Time, end time.Time, now time.Time) time.Time {
	if parsed, err := time.Parse(time.DateOnly, strings.TrimSpace(raw)); err == nil {
		parsed = parsed.UTC()
		if !parsed.Before(start) && parsed.Before(end) {
			return parsed
		}
	}
	if !now.Before(start) && now.Before(end) {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}
	return start
}

// workspaceFilterLeaves 處理工作區篩選 leaves。
func workspaceFilterLeaves(items []LeaveRequest, start time.Time, end time.Time) []LeaveRequest {
	out := make([]LeaveRequest, 0, len(items))
	for _, item := range items {
		if !workspaceLeaveEffective(item) {
			continue
		}
		if item.EndAt.Before(start) || !item.StartAt.Before(end) {
			continue
		}
		out = append(out, item)
	}
	return out
}

// workspaceLeaveEffective 處理工作區請假 effective。只有審核通過的請假才計入工時統計。
func workspaceLeaveEffective(item LeaveRequest) bool {
	return strings.ToLower(strings.TrimSpace(item.Status)) == "approved"
}

// workspaceLeaveEmployeesForDate 處理工作區請假員工 for 日期。
func workspaceLeaveEmployeesForDate(items []LeaveRequest, date time.Time) map[string]struct{} {
	out := map[string]struct{}{}
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.AddDate(0, 0, 1)
	for _, item := range items {
		if item.EmployeeID == "" {
			continue
		}
		if item.EndAt.Before(dayStart) || !item.StartAt.Before(dayEnd) {
			continue
		}
		out[item.EmployeeID] = struct{}{}
	}
	return out
}

// workspaceCheckedInEmployees 處理工作區 checked in 員工。
func workspaceCheckedInEmployees(items []AttendanceClockRecord, date time.Time) map[string]struct{} {
	out := map[string]struct{}{}
	key := date.Format(time.DateOnly)
	for _, item := range items {
		if item.WorkDate == key && item.Direction == clockDirectionIn && item.RecordStatus == clockRecordStatusAccepted {
			out[item.EmployeeID] = struct{}{}
		}
	}
	return out
}

// workspaceCheckedInEmployeesFromSummaries 處理工作區 eHRMS 日彙總 checked in 員工。
func workspaceCheckedInEmployeesFromSummaries(items []AttendanceDailySummary, date time.Time) map[string]struct{} {
	out := map[string]struct{}{}
	key := date.Format(time.DateOnly)
	for _, item := range items {
		if item.WorkDate == key && item.ClockHours > 0 {
			out[item.EmployeeID] = struct{}{}
		}
	}
	return out
}

// workspaceCountActiveAt 處理工作區 count 啟用中 at。
func workspaceCountActiveAt(items []Employee, at time.Time) int {
	count := 0
	for _, item := range items {
		if workspaceEmployeeActiveAt(item, at) {
			count++
		}
	}
	return count
}

// workspaceEmployeeActiveAt 處理工作區員工啟用中 at。
func workspaceEmployeeActiveAt(item Employee, at time.Time) bool {
	status := strings.ToLower(utils.FirstNonEmpty(item.EmploymentStatus, item.Status))
	if status == string(EmployeeStatusDeleted) {
		return false
	}
	if status == string(EmployeeStatusResigned) && (item.ResignDate == nil || !item.ResignDate.After(at)) {
		return false
	}
	if item.HireDate != nil && item.HireDate.After(at) {
		return false
	}
	if item.ResignDate != nil && !item.ResignDate.After(at) {
		return false
	}
	return true
}

// workspaceEmployeesPresentInRange 處理工作區員工 present in range。
func workspaceEmployeesPresentInRange(items []Employee, start time.Time, end time.Time) []Employee {
	out := make([]Employee, 0, len(items))
	for _, item := range items {
		if workspaceEmployeeStatus(item) == string(EmployeeStatusDeleted) {
			continue
		}
		if item.HireDate != nil && !item.HireDate.Before(end) {
			continue
		}
		if item.ResignDate != nil && item.ResignDate.Before(start) {
			continue
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return workspaceEmployeeDisplayID(out[i]) < workspaceEmployeeDisplayID(out[j])
	})
	return out
}

// workspaceEmployeeStatus 處理工作區員工狀態。
func workspaceEmployeeStatus(item Employee) string {
	return strings.ToLower(utils.FirstNonEmpty(item.EmploymentStatus, item.Status))
}

// workspaceCountHires 處理工作區 count hires。
func workspaceCountHires(items []Employee, start time.Time, end time.Time) int {
	count := 0
	for _, item := range items {
		if item.HireDate != nil && !item.HireDate.Before(start) && item.HireDate.Before(end) {
			count++
		}
	}
	return count
}

// workspaceCountSeparations 處理工作區 count separations。
func workspaceCountSeparations(items []Employee, start time.Time, end time.Time) int {
	count := 0
	for _, item := range items {
		if workspaceEmployeeSeparatedInRange(item, start, end) {
			count++
		}
	}
	return count
}

// workspaceEmployeeSeparatedInRange 處理工作區員工 separated in range。
func workspaceEmployeeSeparatedInRange(item Employee, start time.Time, end time.Time) bool {
	if item.ResignDate != nil && !item.ResignDate.Before(start) && item.ResignDate.Before(end) {
		return true
	}
	status := workspaceEmployeeStatus(item)
	if status != string(EmployeeStatusResigned) && status != string(EmployeeStatusDeleted) {
		return false
	}
	return !item.UpdatedAt.Before(start) && item.UpdatedAt.Before(end)
}

// workspaceAttendanceSegments 處理工作區考勤 segments。
func workspaceAttendanceSegments(checkedIn int, leave int, absent int) []WorkspaceAttendanceSlice {
	total := checkedIn + leave + absent
	if total <= 0 {
		return []WorkspaceAttendanceSlice{}
	}
	segments := []WorkspaceAttendanceSlice{
		{Key: "checked-in", Label: "已上班", Percent: workspacePercent(checkedIn, total), Tone: "success"},
		{Key: "leave", Label: "請假", Percent: workspacePercent(leave, total), Tone: "warning"},
	}
	if absent > 0 {
		segments = append(segments, WorkspaceAttendanceSlice{Key: "absent", Label: "未打卡", Percent: workspacePercent(absent, total), Tone: "danger"})
	}
	return segments
}

// workspaceDailyLeave 處理工作區每日請假。
func workspaceDailyLeave(start time.Time, end time.Time, leaves []LeaveRequest, activeDate time.Time) []WorkspaceDailyLeave {
	counts := map[int]int{}
	maxValue := 0
	for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
		employees := workspaceLeaveEmployeesForDate(leaves, day)
		counts[day.Day()] = len(employees)
		if len(employees) > maxValue {
			maxValue = len(employees)
		}
	}
	out := make([]WorkspaceDailyLeave, 0, len(counts))
	for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
		value := counts[day.Day()]
		height := 0
		if maxValue > 0 {
			height = workspacePercent(value, maxValue)
		}
		out = append(out, WorkspaceDailyLeave{
			Day:           day.Day(),
			Value:         value,
			HeightPercent: height,
			ShowLabel:     day.Day() == 1 || day.Day()%5 == 0,
			Active:        day.Equal(activeDate),
			Tooltip:       fmt.Sprintf("%d/%d(%s) · 請假 %d 人", int(day.Month()), day.Day(), workspaceWeekdayZH(day), value),
		})
	}
	return out
}

// workspaceTodoCategories 處理工作區待辦分類。
func workspaceTodoCategories(employees []Employee, now time.Time) []WorkspaceTodoCategory {
	categories := []WorkspaceTodoCategory{
		workspaceTodoCategory("onboarding", "待入職", "user-plus", "已發 offer 待報到", "預計到職日", employees, func(item Employee) (string, bool) {
			if workspaceEmployeeStatus(item) != string(EmployeeStatusOnboarding) {
				return "", false
			}
			return workspaceFormatDateSlash(item.HireDate), true
		}),
		workspaceTodoCategory("regularize", "待轉正", "user-check", "試用期屆滿待簽核", "試用期屆滿日", employees, func(item Employee) (string, bool) {
			if workspaceEmployeeStatus(item) != string(EmployeeStatusProbation) {
				return "", false
			}
			if date := workspaceEmployeeInfoDate(item, "probation_end_date", "regularize_date"); date != "" {
				return date, true
			}
			if item.HireDate != nil {
				t := item.HireDate.AddDate(0, 3, 0)
				return workspaceFormatTimeSlash(t), true
			}
			return "", true
		}),
		workspaceTodoCategory("resign", "待離職", "user-x", "已收離職單待交接", "預計離職日", employees, func(item Employee) (string, bool) {
			if item.ResignDate == nil || item.ResignDate.Before(now) {
				return "", false
			}
			return workspaceFormatTimeSlash(*item.ResignDate), true
		}),
		workspaceTodoCategory("unpaid", "留職停薪", "pause-circle", "留停中或待核准", "留停期間", employees, func(item Employee) (string, bool) {
			if workspaceEmployeeStatus(item) != string(EmployeeStatusLeaveSuspended) {
				return "", false
			}
			start := workspaceEmployeeInfoDate(item, "leave_start_date", "unpaid_leave_start_date")
			end := workspaceEmployeeInfoDate(item, "leave_end_date", "unpaid_leave_end_date")
			switch {
			case start != "" && end != "":
				return start + " - " + end, true
			case start != "":
				return start, true
			default:
				return "", true
			}
		}),
		workspaceTodoCategory("contract", "合約到期", "file-clock", "未來 60 天內合約屆期", "合約到期日", employees, func(item Employee) (string, bool) {
			date := workspaceEmployeeInfoDate(item, "contract_expiry_date")
			if date == "" {
				return "", false
			}
			parsed, ok := workspaceParseFlexibleDate(date)
			if !ok || parsed.Before(now) || parsed.After(now.AddDate(0, 0, 60)) {
				return "", false
			}
			return date, true
		}),
	}
	for i := range categories {
		categories[i].Count = len(categories[i].People)
	}
	return categories
}

// workspaceTodoCategory 處理工作區待辦分類。
func workspaceTodoCategory(key string, label string, icon string, desc string, dateLabel string, employees []Employee, include func(Employee) (string, bool)) WorkspaceTodoCategory {
	people := []WorkspaceTodoPerson{}
	for _, employee := range employees {
		date, ok := include(employee)
		if !ok {
			continue
		}
		people = append(people, WorkspaceTodoPerson{
			ID:     workspaceEmployeeDisplayID(employee),
			NameZH: employee.Name,
			NameEN: workspaceEmployeeNameEN(employee),
			Date:   date,
		})
	}
	sort.SliceStable(people, func(i, j int) bool {
		if people[i].Date != people[j].Date {
			return people[i].Date < people[j].Date
		}
		return people[i].ID < people[j].ID
	})
	if len(people) > 5 {
		people = people[:5]
	}
	return WorkspaceTodoCategory{Key: key, Label: label, Icon: icon, Desc: desc, DateLabel: dateLabel, People: people}
}

// workspaceOrgNames 處理工作區組織 names。
func workspaceOrgNames(units []OrgUnit) map[string]string {
	out := map[string]string{}
	for _, unit := range units {
		out[unit.ID] = unit.Name
	}
	return out
}

// workspaceOrgCatalog 處理工作區組織目錄。
func workspaceOrgCatalog(units []OrgUnit) map[string]workspaceOrgInfo {
	out := map[string]workspaceOrgInfo{}
	for _, unit := range units {
		out[unit.ID] = workspaceOrgInfo{ID: unit.ID, Name: unit.Name, ParentID: unit.ParentID, Path: utils.CopyStrings(unit.Path)}
	}
	return out
}

// workspaceOrgName 處理工作區組織名稱。
func workspaceOrgName(names map[string]string, id string) string {
	if name := strings.TrimSpace(names[id]); name != "" {
		return name
	}
	if strings.TrimSpace(id) == "" {
		return "未分配"
	}
	return id
}

// workspaceOrgBUAndDept 處理工作區組織 bu and 部門。
func workspaceOrgBUAndDept(orgs map[string]workspaceOrgInfo, orgID string) (string, string) {
	info, ok := orgs[orgID]
	if !ok {
		if orgID == "" {
			return "未分配", "未分配"
		}
		return orgID, orgID
	}
	dept := utils.FirstNonEmpty(info.Name, info.ID)
	bu := dept
	if len(info.Path) > 0 {
		if root, ok := orgs[info.Path[0]]; ok && root.Name != "" {
			bu = root.Name
		}
	} else if info.ParentID != "" {
		current := info
		seen := map[string]struct{}{current.ID: {}}
		for current.ParentID != "" {
			parent, ok := orgs[current.ParentID]
			if !ok {
				break
			}
			if _, exists := seen[parent.ID]; exists {
				break
			}
			seen[parent.ID] = struct{}{}
			bu = utils.FirstNonEmpty(parent.Name, parent.ID)
			current = parent
		}
	}
	return bu, dept
}

// workspaceEmployeeDisplayIDs 處理工作區員工顯示 IDs。
func workspaceEmployeeDisplayIDs(employees []Employee) map[string]string {
	out := map[string]string{}
	for _, employee := range employees {
		out[employee.ID] = workspaceEmployeeDisplayID(employee)
	}
	return out
}

// workspaceEmployeeByDisplayID 處理工作區員工 by 顯示 ID。
func workspaceEmployeeByDisplayID(employees []Employee, displayID string) (Employee, bool) {
	normalized := strings.TrimSpace(displayID)
	if normalized == "" {
		return Employee{}, false
	}
	for _, employee := range employees {
		if workspaceEmployeeDisplayID(employee) == normalized || employee.ID == normalized {
			return employee, true
		}
	}
	return Employee{}, false
}

// workspaceEmployeeDisplayID 處理工作區員工顯示 ID。
func workspaceEmployeeDisplayID(employee Employee) string {
	return utils.FirstNonEmpty(strings.TrimSpace(employee.EmployeeNo), employee.ID)
}

// workspaceManagerCycle 處理工作區主管 cycle。
func workspaceManagerCycle(employees []Employee, employeeID, managerID string) bool {
	if managerID == "" {
		return false
	}
	byID := map[string]Employee{}
	for _, employee := range employees {
		byID[employee.ID] = employee
	}
	seen := map[string]struct{}{}
	for current := managerID; current != ""; {
		if current == employeeID {
			return true
		}
		if _, exists := seen[current]; exists {
			return true
		}
		seen[current] = struct{}{}
		manager, ok := byID[current]
		if !ok {
			return false
		}
		current = manager.ManagerEmployeeID
	}
	return false
}

// workspaceEmployeeLevel 處理工作區員工 level。
func workspaceEmployeeLevel(id string, employees map[string]Employee, memo map[string]int) int {
	if level, ok := memo[id]; ok {
		return level
	}
	employee, ok := employees[id]
	if !ok || employee.ManagerEmployeeID == "" {
		memo[id] = 1
		return 1
	}
	seen := map[string]struct{}{id: {}}
	level := 1
	managerID := employee.ManagerEmployeeID
	for managerID != "" {
		if _, exists := seen[managerID]; exists {
			break
		}
		seen[managerID] = struct{}{}
		manager, ok := employees[managerID]
		if !ok {
			break
		}
		level++
		managerID = manager.ManagerEmployeeID
	}
	memo[id] = level
	return level
}

// workspaceDerivedManagersByOrgUnit 依組織單元主管崗推導預設主管。
// 多 incumbent 時取員工編號/ID 字典序最小者作為 primary。
func workspaceDerivedManagersByOrgUnit(units []OrgUnit, positions []Position, employees []Employee) map[string]string {
	activePositions := map[string]Position{}
	for _, position := range positions {
		if position.Status == string(PositionStatusDisabled) {
			continue
		}
		activePositions[position.ID] = position
	}
	incumbentsByPosition := map[string][]Employee{}
	for _, employee := range employees {
		if strings.TrimSpace(employee.PositionID) == "" {
			continue
		}
		if !workspaceEmployeeIsActive(employee) {
			continue
		}
		incumbentsByPosition[employee.PositionID] = append(incumbentsByPosition[employee.PositionID], employee)
	}
	for positionID, incumbents := range incumbentsByPosition {
		sort.SliceStable(incumbents, func(i, j int) bool {
			left := workspaceEmployeeDisplayID(incumbents[i])
			right := workspaceEmployeeDisplayID(incumbents[j])
			if left != right {
				return left < right
			}
			return incumbents[i].ID < incumbents[j].ID
		})
		incumbentsByPosition[positionID] = incumbents
	}
	unitsByID := map[string]OrgUnit{}
	for _, unit := range units {
		unitsByID[unit.ID] = unit
	}
	out := map[string]string{}
	memo := map[string]string{}
	var resolve func(orgUnitID string, seen map[string]struct{}) string
	resolve = func(orgUnitID string, seen map[string]struct{}) string {
		orgUnitID = strings.TrimSpace(orgUnitID)
		if orgUnitID == "" {
			return ""
		}
		if managerID, ok := memo[orgUnitID]; ok {
			return managerID
		}
		if _, exists := seen[orgUnitID]; exists {
			return ""
		}
		seen[orgUnitID] = struct{}{}
		unit, ok := unitsByID[orgUnitID]
		if !ok {
			memo[orgUnitID] = ""
			return ""
		}
		if positionID := strings.TrimSpace(unit.ManagerPositionID); positionID != "" {
			if _, ok := activePositions[positionID]; ok {
				if incumbents := incumbentsByPosition[positionID]; len(incumbents) > 0 {
					memo[orgUnitID] = incumbents[0].ID
					return incumbents[0].ID
				}
			}
		}
		managerID := resolve(unit.ParentID, seen)
		memo[orgUnitID] = managerID
		return managerID
	}
	for _, unit := range units {
		out[unit.ID] = resolve(unit.ID, map[string]struct{}{})
	}
	return out
}

func workspaceEffectiveManager(employee Employee, derivedManagers map[string]string) (string, string) {
	if override := strings.TrimSpace(employee.ManagerEmployeeID); override != "" {
		return override, "override"
	}
	if derived := strings.TrimSpace(derivedManagers[employee.OrgUnitID]); derived != "" && derived != employee.ID {
		return derived, "org_unit"
	}
	return "", "none"
}

func workspaceEffectiveEmployeeLevel(id string, parents map[string]string, employees map[string]Employee, memo map[string]int) int {
	if level, ok := memo[id]; ok {
		return level
	}
	if _, ok := employees[id]; !ok {
		memo[id] = 1
		return 1
	}
	seen := map[string]struct{}{id: {}}
	level := 1
	managerID := parents[id]
	for managerID != "" {
		if _, exists := seen[managerID]; exists {
			break
		}
		seen[managerID] = struct{}{}
		if _, ok := employees[managerID]; !ok {
			break
		}
		level++
		managerID = parents[managerID]
	}
	memo[id] = level
	return level
}

func workspaceEmployeeIsActive(employee Employee) bool {
	status := strings.TrimSpace(employee.EmploymentStatus)
	if status == "" {
		status = strings.TrimSpace(string(employee.Status))
	}
	switch status {
	case string(EmployeeStatusResigned), string(EmployeeStatusDeleted), "inactive", "disabled":
		return false
	default:
		return true
	}
}

// workspaceMonthlyTurnover 處理工作區每月人員異動。
func workspaceMonthlyTurnover(employees []Employee, orgs map[string]workspaceOrgInfo, start time.Time, end time.Time, now time.Time) WorkspaceTurnoverMonthly {
	stats := workspaceMovementByDept(employees, orgs, start, end, time.Date(start.Year(), 1, 1, 0, 0, 0, 0, time.UTC))
	rows := workspaceMonthlyTurnoverRows(stats)
	total := workspaceMovementTotal(stats)
	prevMonthStart := start.AddDate(0, -1, 0)
	prevMonthEnd := start
	prevStats := workspaceMovementTotal(workspaceMovementByDept(employees, orgs, prevMonthStart, prevMonthEnd, time.Date(start.Year(), 1, 1, 0, 0, 0, 0, time.UTC)))
	rate := workspaceRate(total.Resigned+total.Layoff, maxInt(total.Prev, 1))
	prevRate := workspaceRate(prevStats.Resigned+prevStats.Layoff, maxInt(prevStats.Prev, 1))
	return WorkspaceTurnoverMonthly{
		Year:           start.Year(),
		Month:          int(start.Month()),
		IsFuture:       start.After(now),
		Title:          fmt.Sprintf("%s在職統計", workspaceMonthNameZH(start.Month())),
		Stats:          workspaceMonthlyKPIs(total, rate, prevRate),
		HireComparison: workspaceComparisonFromStats(stats, func(s workspaceMovementStats) float64 { return float64(s.Hires) }, "人", false),
		RateComparison: workspaceComparisonFromStats(stats, func(s workspaceMovementStats) float64 { return workspaceRate(s.Resigned+s.Layoff, maxInt(s.Prev, 1)) }, "%", true),
		Rows:           rows,
		CSVHeaders:     []string{"BU", "部門", "上月在職人數", "新進人數", "離職人數", "資遣", "當月留停", "本月在職人數", "預估離職率", "YTD離職率"},
	}
}

// workspaceAnnualTurnover 處理工作區年度人員異動。
func workspaceAnnualTurnover(employees []Employee, orgs map[string]workspaceOrgInfo, year int, now time.Time) WorkspaceTurnoverAnnual {
	annualStart := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	annualEnd := annualStart.AddDate(1, 0, 0)
	stats := workspaceMovementByBU(employees, orgs, annualStart, annualEnd)
	total := workspaceMovementTotal(stats)
	base := maxInt(total.Base, 1)
	rate := workspaceRate(total.Resigned+total.Layoff, base)
	return WorkspaceTurnoverAnnual{
		Year:               year,
		IsFuture:           annualStart.After(now),
		Title:              fmt.Sprintf("%d 年度在職統計", year),
		KPIs:               workspaceAnnualKPIs(total, rate),
		HeadcountTrend:     workspaceHeadcountTrend(employees, year, now),
		RateTrend:          workspaceRateTrend(employees, year, now),
		Pie:                workspacePieFromStats(stats),
		DeptRateComparison: workspaceComparisonFromStats(workspaceMovementByDept(employees, orgs, annualStart, annualEnd, annualStart), func(s workspaceMovementStats) float64 { return workspaceRate(s.Resigned+s.Layoff, maxInt(s.Base, 1)) }, "%", true),
		Rows:               workspaceAnnualTurnoverRows(stats),
		CSVHeaders:         []string{"BU", "年初在職", "年新進", "年離職", "年資遣", "年留停", "年末在職", "年離職率"},
	}
}

// workspaceMovementByDept 處理工作區 movement by 部門。
func workspaceMovementByDept(employees []Employee, orgs map[string]workspaceOrgInfo, start time.Time, end time.Time, ytdStart time.Time) []workspaceMovementStats {
	byKey := map[string]*workspaceMovementStats{}
	for _, employee := range employees {
		bu, dept := workspaceOrgBUAndDept(orgs, employee.OrgUnitID)
		key := bu + "\x00" + dept
		stat := byKey[key]
		if stat == nil {
			stat = &workspaceMovementStats{BU: bu, Dept: dept}
			byKey[key] = stat
		}
		workspaceApplyMovement(stat, employee, start, end, ytdStart)
	}
	out := make([]workspaceMovementStats, 0, len(byKey))
	for _, stat := range byKey {
		out = append(out, *stat)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].BU != out[j].BU {
			return out[i].BU < out[j].BU
		}
		return out[i].Dept < out[j].Dept
	})
	return out
}

// workspaceMovementByBU 處理工作區 movement by bu。
func workspaceMovementByBU(employees []Employee, orgs map[string]workspaceOrgInfo, start time.Time, end time.Time) []workspaceMovementStats {
	byKey := map[string]*workspaceMovementStats{}
	for _, employee := range employees {
		bu, _ := workspaceOrgBUAndDept(orgs, employee.OrgUnitID)
		stat := byKey[bu]
		if stat == nil {
			stat = &workspaceMovementStats{BU: bu, Dept: bu}
			byKey[bu] = stat
		}
		workspaceApplyMovement(stat, employee, start, end, start)
	}
	out := make([]workspaceMovementStats, 0, len(byKey))
	for _, stat := range byKey {
		out = append(out, *stat)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].BU < out[j].BU })
	return out
}

// workspaceApplyMovement 處理工作區 apply movement。
func workspaceApplyMovement(stat *workspaceMovementStats, employee Employee, start time.Time, end time.Time, ytdStart time.Time) {
	if workspaceEmployeeActiveAt(employee, start.Add(-time.Nanosecond)) {
		stat.Prev++
	}
	if workspaceEmployeeActiveAt(employee, ytdStart.Add(-time.Nanosecond)) {
		stat.Base++
	}
	if workspaceEmployeeActiveAt(employee, end.Add(-time.Nanosecond)) {
		stat.End++
		stat.YTDEnd++
	}
	if employee.HireDate != nil && !employee.HireDate.Before(start) && employee.HireDate.Before(end) {
		stat.Hires++
	}
	if employee.HireDate != nil && !employee.HireDate.Before(ytdStart) && employee.HireDate.Before(end) {
		stat.YTDHires++
	}
	if workspaceEmployeeSeparatedInRange(employee, start, end) {
		stat.Resigned++
	}
	if workspaceEmployeeSeparatedInRange(employee, ytdStart, end) {
		stat.YTDSep++
	}
	if workspaceEmployeeStatus(employee) == string(EmployeeStatusLeaveSuspended) && workspaceEmployeeActiveAt(employee, end.Add(-time.Nanosecond)) {
		stat.OnLeave++
		stat.YTDOnLeave++
	}
}

// workspaceMovementTotal 處理工作區 movement total。
func workspaceMovementTotal(items []workspaceMovementStats) workspaceMovementStats {
	total := workspaceMovementStats{BU: "總計", Dept: "總計"}
	for _, item := range items {
		total.Base += item.Base
		total.Prev += item.Prev
		total.Hires += item.Hires
		total.Resigned += item.Resigned
		total.Layoff += item.Layoff
		total.OnLeave += item.OnLeave
		total.End += item.End
		total.YTDSep += item.YTDSep
		total.YTDHires += item.YTDHires
		total.YTDEnd += item.YTDEnd
		total.YTDOnLeave += item.YTDOnLeave
	}
	return total
}

// workspaceMonthlyTurnoverRows 處理工作區每月人員異動列。
func workspaceMonthlyTurnoverRows(stats []workspaceMovementStats) []WorkspaceTurnoverRow {
	rows := []WorkspaceTurnoverRow{}
	byBU := map[string][]workspaceMovementStats{}
	for _, stat := range stats {
		byBU[stat.BU] = append(byBU[stat.BU], stat)
	}
	bus := sortedKeys(byBU)
	total := workspaceMovementStats{BU: "總計", Dept: "總計"}
	for _, bu := range bus {
		group := byBU[bu]
		subtotal := workspaceMovementStats{BU: bu + " 合計", Dept: ""}
		for i, stat := range group {
			span := 0
			if i == 0 {
				span = len(group)
			}
			rows = append(rows, workspaceMonthlyRow(stat, "dept", span))
			subtotal = workspaceSumMovement(subtotal, stat)
			total = workspaceSumMovement(total, stat)
		}
		if len(group) > 1 {
			rows = append(rows, workspaceMonthlyRow(subtotal, "subtotal", 1))
		}
	}
	rows = append(rows, workspaceMonthlyRow(total, "total", 1))
	return rows
}

// workspaceMonthlyRow 處理工作區每月列。
func workspaceMonthlyRow(stat workspaceMovementStats, rowType string, rowSpan int) WorkspaceTurnoverRow {
	sep := stat.Resigned + stat.Layoff
	return WorkspaceTurnoverRow{
		Key:       workspaceTurnoverKey(stat, rowType),
		RowType:   rowType,
		BU:        stat.BU,
		Dept:      stat.Dept,
		BURowSpan: rowSpan,
		Prev:      stat.Prev,
		Hires:     stat.Hires,
		Resigned:  stat.Resigned,
		Layoff:    stat.Layoff,
		OnLeave:   stat.OnLeave,
		End:       stat.End,
		MonthRate: workspaceRateLabel(sep, maxInt(stat.Prev, 1)),
		YTDRate:   workspaceRateLabel(stat.YTDSep, maxInt(stat.Base, 1)),
	}
}

// workspaceAnnualTurnoverRows 處理工作區年度人員異動列。
func workspaceAnnualTurnoverRows(stats []workspaceMovementStats) []WorkspaceAnnualRow {
	rows := make([]WorkspaceAnnualRow, 0, len(stats)+1)
	total := workspaceMovementStats{BU: "總計"}
	for _, stat := range stats {
		rows = append(rows, workspaceAnnualRow(stat))
		total = workspaceSumMovement(total, stat)
	}
	rows = append(rows, workspaceAnnualRow(total))
	return rows
}

// workspaceAnnualRow 處理工作區年度列。
func workspaceAnnualRow(stat workspaceMovementStats) WorkspaceAnnualRow {
	sep := stat.Resigned + stat.Layoff
	return WorkspaceAnnualRow{
		BU:       stat.BU,
		Base:     stat.Base,
		Hires:    stat.Hires,
		Resigned: stat.Resigned,
		Layoff:   stat.Layoff,
		OnLeave:  stat.OnLeave,
		End:      stat.End,
		Sep:      sep,
		Rate:     workspaceRateLabel(sep, maxInt(stat.Base, 1)),
	}
}

// workspaceSumMovement 處理工作區總和 movement。
func workspaceSumMovement(total workspaceMovementStats, item workspaceMovementStats) workspaceMovementStats {
	total.Base += item.Base
	total.Prev += item.Prev
	total.Hires += item.Hires
	total.Resigned += item.Resigned
	total.Layoff += item.Layoff
	total.OnLeave += item.OnLeave
	total.End += item.End
	total.YTDSep += item.YTDSep
	total.YTDHires += item.YTDHires
	total.YTDEnd += item.YTDEnd
	total.YTDOnLeave += item.YTDOnLeave
	return total
}

// workspaceTurnoverKey 處理工作區人員異動 key。
func workspaceTurnoverKey(stat workspaceMovementStats, rowType string) string {
	switch rowType {
	case "total":
		return "grand-total"
	case "subtotal":
		return stat.BU + "-subtotal"
	default:
		return stat.BU + "-" + stat.Dept
	}
}

// workspaceMonthlyKPIs 處理工作區每月 kp is。
func workspaceMonthlyKPIs(total workspaceMovementStats, rate float64, prevRate float64) []WorkspaceKPI {
	diff := rate - prevRate
	trendTone := "flat"
	trendText := ""
	if math.Abs(diff) >= 0.05 {
		if diff > 0 {
			trendTone = "down"
			trendText = fmt.Sprintf("較上月上升 %.1f%%", math.Abs(diff))
		} else {
			trendTone = "up"
			trendText = fmt.Sprintf("較上月下降 %.1f%%", math.Abs(diff))
		}
	}
	return []WorkspaceKPI{
		{Key: "active", Label: "在職人數", Value: strconv.Itoa(total.End), Unit: "人", TrendTone: "flat"},
		{Key: "hires", Label: "本月新進", Value: strconv.Itoa(total.Hires), Unit: "人", TrendTone: "flat"},
		{Key: "sep", Label: "本月離職", Value: strconv.Itoa(total.Resigned + total.Layoff), Unit: "人", TrendTone: "flat"},
		{Key: "rate", Label: "本月離職率", Value: fmt.Sprintf("%.1f", rate), Unit: "%", TrendText: trendText, TrendTone: trendTone},
	}
}

// workspaceAnnualKPIs 處理工作區年度 kp is。
func workspaceAnnualKPIs(total workspaceMovementStats, rate float64) []WorkspaceKPI {
	net := total.Hires - total.Resigned - total.Layoff
	netText := strconv.Itoa(net)
	if net > 0 {
		netText = "+" + netText
	}
	return []WorkspaceKPI{
		{Key: "total", Label: "年度員工總數", Value: strconv.Itoa(total.End), Unit: "人", TrendTone: "flat"},
		{Key: "sep", Label: "年度離職總數", Value: strconv.Itoa(total.Resigned + total.Layoff), Unit: "人", TrendTone: "flat"},
		{Key: "net", Label: "年淨增減", Value: netText, Unit: "人", TrendTone: "flat"},
		{Key: "rate", Label: "年度離職率", Value: fmt.Sprintf("%.1f", rate), Unit: "%", TrendTone: "flat"},
	}
}

// workspaceComparisonFromStats 處理工作區 comparison 來源 stats。
func workspaceComparisonFromStats(stats []workspaceMovementStats, valueFn func(workspaceMovementStats) float64, unit string, decimal bool) []WorkspaceComparisonItem {
	items := make([]WorkspaceComparisonItem, 0, len(stats))
	maxValue := 0.0
	for _, stat := range stats {
		value := valueFn(stat)
		if value <= 0 {
			continue
		}
		if value > maxValue {
			maxValue = value
		}
		label := fmt.Sprintf("%.0f %s", value, unit)
		if decimal {
			label = fmt.Sprintf("%.1f%s", value, unit)
		}
		items = append(items, WorkspaceComparisonItem{Name: stat.Dept, Value: value, Label: label})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Value != items[j].Value {
			return items[i].Value > items[j].Value
		}
		return items[i].Name < items[j].Name
	})
	for i := range items {
		items[i].Percent = workspacePercentFloat(items[i].Value, maxValue)
	}
	if len(items) > 30 {
		return items[:30]
	}
	return items
}

// workspaceHeadcountTrend 處理工作區 headcount 趨勢。
func workspaceHeadcountTrend(employees []Employee, year int, now time.Time) []WorkspaceTrendPoint {
	points := make([]WorkspaceTrendPoint, 0, 12)
	maxValue := 0
	values := make([]int, 12)
	futures := make([]bool, 12)
	for month := 1; month <= 12; month++ {
		end := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
		future := end.After(now) && year >= now.Year()
		futures[month-1] = future
		if future {
			continue
		}
		values[month-1] = workspaceCountActiveAt(employees, end.Add(-time.Nanosecond))
		if values[month-1] > maxValue {
			maxValue = values[month-1]
		}
	}
	for month := 1; month <= 12; month++ {
		value := values[month-1]
		future := futures[month-1]
		label := strconv.Itoa(value)
		tone := "flat"
		if future {
			label = "—"
			tone = "future"
		} else if month > 1 && value > values[month-2] {
			tone = "up"
		} else if month > 1 && value < values[month-2] {
			tone = "down"
		}
		points = append(points, WorkspaceTrendPoint{Month: month, Value: float64(value), Label: label, Percent: workspacePercent(value, maxInt(maxValue, 1)), Future: future, Tone: tone})
	}
	return points
}

// workspaceRateTrend 處理工作區速率趨勢。
func workspaceRateTrend(employees []Employee, year int, now time.Time) []WorkspaceTrendPoint {
	points := make([]WorkspaceTrendPoint, 0, 12)
	values := make([]float64, 12)
	futures := make([]bool, 12)
	maxValue := 0.0
	for month := 1; month <= 12; month++ {
		start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0)
		future := end.After(now) && year >= now.Year()
		futures[month-1] = future
		if future {
			continue
		}
		sep := workspaceCountSeparations(employees, start, end)
		prev := workspaceCountActiveAt(employees, start.Add(-time.Nanosecond))
		values[month-1] = workspaceRate(sep, maxInt(prev, 1))
		if values[month-1] > maxValue {
			maxValue = values[month-1]
		}
	}
	for month := 1; month <= 12; month++ {
		value := values[month-1]
		future := futures[month-1]
		label := fmt.Sprintf("%.1f%%", value)
		tone := "flat"
		if future {
			label = "—"
			tone = "future"
		} else if month > 1 && value > values[month-2] {
			tone = "up"
		} else if month > 1 && value < values[month-2] {
			tone = "down"
		}
		points = append(points, WorkspaceTrendPoint{Month: month, Value: value, Label: label, Percent: workspacePercentFloat(value, maxValue), Future: future, Tone: tone})
	}
	return points
}

// workspacePieFromStats 處理工作區 pie 來源 stats。
func workspacePieFromStats(stats []workspaceMovementStats) []WorkspacePieItem {
	type pair struct {
		name  string
		value int
	}
	pairs := make([]pair, 0, len(stats))
	total := 0
	for _, stat := range stats {
		if stat.End <= 0 {
			continue
		}
		pairs = append(pairs, pair{name: stat.BU, value: stat.End})
		total += stat.End
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].value != pairs[j].value {
			return pairs[i].value > pairs[j].value
		}
		return pairs[i].name < pairs[j].name
	})
	if len(pairs) > 8 {
		other := pair{name: "其他"}
		for _, item := range pairs[8:] {
			other.value += item.value
		}
		pairs = append(pairs[:8], other)
	}
	out := make([]WorkspacePieItem, 0, len(pairs))
	cursor := 0.0
	for i, item := range pairs {
		percent := workspacePercent(item.value, maxInt(total, 1))
		end := cursor + float64(item.value)/float64(maxInt(total, 1))*100
		out = append(out, WorkspacePieItem{Name: item.name, Value: item.value, Percent: percent, Start: cursor, End: end, ColorIndex: i})
		cursor = end
	}
	return out
}

// workspaceMonthDates 處理工作區月份 dates。
