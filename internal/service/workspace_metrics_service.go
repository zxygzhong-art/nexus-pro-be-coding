package service

import (
	"fmt"
	"math"
	"nexus-pro-api/internal/utils"
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
	BU                     string
	Dept                   string
	Base                   int
	Prev                   int
	Hires                  int
	Resigned               int
	Layoff                 int
	OnLeave                int
	End                    int
	YTDSep                 int
	YTDHires               int
	YTDEnd                 int
	YTDOnLeave             int
	PrevLastDaySeparations int
	NonLastDaySeparations  int
	RatePrev               [12]int
	RateEnd                [12]int
	RateResigned           [12]int
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

// workspaceEmployeeSeparationTime 回傳員工的有效離職時間。
// 優先使用 resign_date；只有當狀態為 resigned/deleted 但缺少 resign_date
// （歷史匯入遺留資料）時，才退回以 updated_at 作為近似離職時間。
// 注意：workspaceEmployeeActiveAt 與 workspaceEmployeeSeparatedInRange 必須
// 共用此定義。兩者口徑一旦不一致（例如一邊用 resign_date、另一邊用 updated_at），
// 就會產生幻影離職，破壞「期初在職 + 期間新進 − 期間離職 = 期末在職」閉合恆等式。
func workspaceEmployeeSeparationTime(item Employee) (time.Time, bool) {
	if item.ResignDate != nil {
		return *item.ResignDate, true
	}
	status := workspaceEmployeeStatus(item)
	if status == string(EmployeeStatusResigned) || status == string(EmployeeStatusDeleted) {
		return item.UpdatedAt, true
	}
	return time.Time{}, false
}

// workspaceEmployeeActiveAt 判斷員工在指定時點是否在職：
// 已到職（hire_date 不晚於該時點）且尚未離職（有效離職時間晚於該時點）。
func workspaceEmployeeActiveAt(item Employee, at time.Time) bool {
	if item.HireDate != nil && item.HireDate.After(at) {
		return false
	}
	if separatedAt, ok := workspaceEmployeeSeparationTime(item); ok && !separatedAt.After(at) {
		return false
	}
	return true
}

// workspaceEmployeesPresentInRange 處理工作區員工 present in range。
func workspaceEmployeesPresentInRange(items []Employee, start time.Time, end time.Time) []Employee {
	out := make([]Employee, 0, len(items))
	for _, item := range items {
		status := workspaceEmployeeStatus(item)
		if status == string(EmployeeStatusDeleted) || (status == string(EmployeeStatusResigned) && item.ResignDate == nil) {
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

// workspaceEmployeeSeparatedInRange 判斷員工是否屬於 [start, end) 期間離職：
// 以有效離職時間（見 workspaceEmployeeSeparationTime）落在期間內為準。
// resign_date 已存在時絕不可再用 updated_at 兜底——批次同步（如 eHRMS 同步、
// 匯入）會刷新全部資料的 updated_at，若據此重算，會把多年前已離職的員工
// 重複計入本期離職（幻影離職），並破壞期初/期末閉合恆等式。
func workspaceEmployeeSeparatedInRange(item Employee, start time.Time, end time.Time) bool {
	separatedAt, ok := workspaceEmployeeSeparationTime(item)
	if !ok {
		return false
	}
	return !separatedAt.Before(start) && separatedAt.Before(end)
}

// workspaceEmployeeIsLayoff 依離職原因區分資遣。資遣仍會減少期末人數，
// 但依在職分析口徑不納入離職人數與離職率。
func workspaceEmployeeIsLayoff(item Employee) bool {
	reason := strings.ToLower(strings.TrimSpace(utils.FirstNonEmpty(
		stringFromMap(item.EmploymentInfo, "resign_reason"),
		stringFromMap(item.EmploymentInfo, "transition_reason"),
	)))
	if reason == "" {
		for i := len(item.InternalExperiences) - 1; i >= 0; i-- {
			if NormalizeEmployeeStatus(item.InternalExperiences[i].Status) == string(EmployeeStatusResigned) {
				reason = strings.ToLower(strings.TrimSpace(item.InternalExperiences[i].Reason))
				break
			}
		}
	}
	return strings.Contains(reason, "資遣") || strings.Contains(reason, "资遣") ||
		strings.Contains(reason, "layoff") || strings.Contains(reason, "laid off") ||
		strings.Contains(reason, "redundan")
}

func workspaceDateOnlyUTC(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

// workspaceSeparationOnClosingDay 判斷離職日是否為 periodEnd 前一日（即期末日）。
// 期末日離職者留在當期期末人數，於次期扣除。
func workspaceSeparationOnClosingDay(separatedAt time.Time, periodEnd time.Time) bool {
	closingDay := workspaceDateOnlyUTC(periodEnd).AddDate(0, 0, -1)
	return workspaceDateOnlyUTC(separatedAt).Equal(closingDay)
}

func workspaceEmployeeLeaveSuspensionRange(item Employee) (time.Time, time.Time, bool) {
	if workspaceEmployeeStatus(item) != string(EmployeeStatusLeaveSuspended) {
		return time.Time{}, time.Time{}, false
	}
	start := workspaceDateOnlyUTC(item.UpdatedAt)
	if raw := strings.TrimSpace(stringFromMap(item.EmploymentInfo, "transition_start_date")); raw != "" {
		if parsed, err := utils.ParseDate(raw); err == nil {
			start = workspaceDateOnlyUTC(parsed)
		}
	}
	var end time.Time
	if raw := strings.TrimSpace(stringFromMap(item.EmploymentInfo, "transition_end_date")); raw != "" {
		if parsed, err := utils.ParseDate(raw); err == nil {
			end = workspaceDateOnlyUTC(parsed)
		}
	}
	return start, end, true
}

func workspaceEmployeeLeaveSuspendedAt(item Employee, at time.Time) bool {
	start, end, ok := workspaceEmployeeLeaveSuspensionRange(item)
	if ok && workspaceDateInInclusiveRange(at, start, end) {
		return true
	}
	for _, experience := range item.InternalExperiences {
		if experience.Current || NormalizeEmployeeStatus(experience.Status) != string(EmployeeStatusLeaveSuspended) || experience.StartDate == nil {
			continue
		}
		experienceEnd := time.Time{}
		if experience.EndDate != nil {
			experienceEnd = *experience.EndDate
		}
		if workspaceDateInInclusiveRange(at, *experience.StartDate, experienceEnd) {
			return true
		}
	}
	return false
}

func workspaceEmployeeLeaveSuspendedInRange(item Employee, start time.Time, end time.Time) bool {
	leaveStart, _, ok := workspaceEmployeeLeaveSuspensionRange(item)
	if ok && !leaveStart.Before(start) && leaveStart.Before(end) {
		return true
	}
	for _, experience := range item.InternalExperiences {
		if experience.Current || NormalizeEmployeeStatus(experience.Status) != string(EmployeeStatusLeaveSuspended) || experience.StartDate == nil {
			continue
		}
		leaveStart = workspaceDateOnlyUTC(*experience.StartDate)
		if !leaveStart.Before(start) && leaveStart.Before(end) {
			return true
		}
	}
	return false
}

func workspaceDateInInclusiveRange(value time.Time, start time.Time, end time.Time) bool {
	day := workspaceDateOnlyUTC(value)
	start = workspaceDateOnlyUTC(start)
	if day.Before(start) {
		return false
	}
	return end.IsZero() || !day.After(workspaceDateOnlyUTC(end))
}

// workspaceEmployeeInTurnoverClosingHeadcountAt 使用在職分析的期末口徑：
// 待加入與留停不計入；期末日離職者仍計入本期，於次期扣除。
func workspaceEmployeeInTurnoverClosingHeadcountAt(item Employee, periodEnd time.Time) bool {
	closingAt := periodEnd.Add(-time.Nanosecond)
	if item.HireDate != nil && item.HireDate.After(closingAt) {
		return false
	}
	if workspaceEmployeeStatus(item) == string(EmployeeStatusOnboarding) {
		return false
	}
	if separatedAt, ok := workspaceEmployeeSeparationTime(item); ok && separatedAt.Before(periodEnd) && !workspaceSeparationOnClosingDay(separatedAt, periodEnd) {
		return false
	}
	return !workspaceEmployeeLeaveSuspendedAt(item, closingAt)
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
		workspaceTodoCategory("unpaid", "留職停薪", "pause-circle", "留停中或待覈準", "留停期間", employees, func(item Employee) (string, bool) {
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

// workspaceDirectoryEmployeeVisible 是所有組織目錄投影共用的人員口徑：
// 在職、待轉正（試用期）、待離職（有未來離職日）。
func workspaceDirectoryEmployeeVisible(employee Employee, now time.Time) bool {
	status := NormalizeEmployeeStatus(utils.FirstNonEmpty(employee.EmploymentStatus, employee.Status))
	if status == string(EmployeeStatusDeleted) {
		return false
	}
	if status == string(EmployeeStatusActive) || status == string(EmployeeStatusProbation) {
		return true
	}
	// 待離職：已有預計離職日且尚未到期（口徑同 overview resign todo）。
	return employee.ResignDate != nil && !employee.ResignDate.Before(now)
}

func workspaceDirectoryEmployees(employees []Employee, now time.Time) []Employee {
	out := make([]Employee, 0, len(employees))
	for _, employee := range employees {
		if workspaceDirectoryEmployeeVisible(employee, now) {
			out = append(out, employee)
		}
	}
	return out
}

// workspaceOrganizationEmployees 復用目錄人員口徑，並移除指向不可見主管的人工覆蓋。
func workspaceOrganizationEmployees(employees []Employee, now time.Time) []Employee {
	hiddenIDs := map[string]struct{}{}
	for _, employee := range employees {
		if !workspaceDirectoryEmployeeVisible(employee, now) {
			hiddenIDs[employee.ID] = struct{}{}
		}
	}

	out := workspaceDirectoryEmployees(employees, now)
	for index, employee := range out {
		if _, managerHidden := hiddenIDs[strings.TrimSpace(employee.ManagerEmployeeID)]; managerHidden {
			out[index].ManagerEmployeeID = ""
		}
	}
	return out
}

// workspaceAverageHeadcount 回傳期間平均在職人數 = (前期期末 + 本期期末) / 2，保底 1。
// 這是所有離職率的統一分母。採用期間平均在職（而非期初或期末單一時點快照），
// 是人資統計的常規口徑：當期內人數劇烈變動時，單一時點分母會讓離職率失真
// （例如期初基數極小時離職率可超過 100%）。
func workspaceAverageHeadcount(prev int, end int) float64 {
	avg := float64(prev+end) / 2
	if avg < 1 {
		return 1
	}
	return avg
}

// workspaceTurnoverRate 統一單月離職率口徑：非資遣離職人數 ÷ 月平均在職人數 × 100%。
func workspaceTurnoverRate(separations int, prev int, end int) float64 {
	return float64(separations) / workspaceAverageHeadcount(prev, end) * 100
}

// workspaceTurnoverRateLabel 處理離職率 label，口徑同 workspaceTurnoverRate。
func workspaceTurnoverRateLabel(separations int, prev int, end int) string {
	return fmt.Sprintf("%.1f%%", workspaceTurnoverRate(separations, prev, end))
}

// workspaceMonthlyTurnover 處理工作區每月人員異動。
func workspaceMonthlyTurnover(employees []Employee, orgs map[string]workspaceOrgInfo, start time.Time, end time.Time, now time.Time) WorkspaceTurnoverMonthly {
	stats := workspaceMovementByDept(employees, orgs, start, end, time.Date(start.Year(), 1, 1, 0, 0, 0, 0, time.UTC))
	rows := workspaceMonthlyTurnoverRows(stats)
	total := workspaceMovementTotal(stats)
	prevMonthStart := start.AddDate(0, -1, 0)
	prevMonthEnd := start
	prevStats := workspaceMovementTotal(workspaceMovementByDept(employees, orgs, prevMonthStart, prevMonthEnd, time.Date(start.Year(), 1, 1, 0, 0, 0, 0, time.UTC)))
	rate := workspaceTurnoverRate(total.Resigned, total.Prev, total.End)
	prevRate := workspaceTurnoverRate(prevStats.Resigned, prevStats.Prev, prevStats.End)
	return WorkspaceTurnoverMonthly{
		Year:           start.Year(),
		Month:          int(start.Month()),
		IsFuture:       start.After(now),
		Title:          fmt.Sprintf("%s在職統計", workspaceMonthNameZH(start.Month())),
		Stats:          workspaceMonthlyKPIs(total, rate, prevRate),
		HireComparison: workspaceComparisonFromStats(stats, func(s workspaceMovementStats) float64 { return float64(s.Hires) }, "人", false),
		RateComparison: workspaceComparisonFromStats(stats, func(s workspaceMovementStats) float64 {
			return workspaceTurnoverRate(s.Resigned, s.Prev, s.End)
		}, "%", true),
		Rows:       rows,
		CSVHeaders: []string{"BU", "部門", "上月在職人數", "新進人數", "離職人數", "資遣", "當月留停", "本月在職人數", "預估離職率", "YTD離職率"},
	}
}

// workspaceAnnualTurnover 處理工作區年度人員異動。
func workspaceAnnualTurnover(employees []Employee, orgs map[string]workspaceOrgInfo, year int, now time.Time) WorkspaceTurnoverAnnual {
	annualStart := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	annualEnd := annualStart.AddDate(1, 0, 0)
	stats := workspaceMovementByBU(employees, orgs, annualStart, annualEnd)
	total := workspaceMovementTotal(stats)
	rate := workspaceCumulativeTurnoverRate(total)
	return WorkspaceTurnoverAnnual{
		Year:           year,
		IsFuture:       annualStart.After(now),
		Title:          fmt.Sprintf("%d 年度在職統計", year),
		KPIs:           workspaceAnnualKPIs(total, rate),
		HeadcountTrend: workspaceHeadcountTrend(employees, year, now),
		RateTrend:      workspaceRateTrend(employees, year, now),
		Pie:            workspacePieFromStats(stats),
		DeptRateComparison: workspaceComparisonFromStats(workspaceMovementByDept(employees, orgs, annualStart, annualEnd, annualStart), func(s workspaceMovementStats) float64 {
			return workspaceCumulativeTurnoverRate(s)
		}, "%", true),
		Rows:       workspaceAnnualTurnoverRows(stats),
		CSVHeaders: []string{"BU", "年初在職", "年新進", "年離職", "年資遣", "年留停", "年末在職", "年離職率"},
	}
}

// workspaceMovementByDept 處理工作區 movement by 部門。
func workspaceMovementByDept(employees []Employee, orgs map[string]workspaceOrgInfo, start time.Time, end time.Time, ytdStart time.Time) []workspaceMovementStats {
	out := workspaceMovementByDeptPeriod(employees, orgs, start, end, ytdStart)
	byKey := map[string]*workspaceMovementStats{}
	for i := range out {
		byKey[out[i].BU+"\x00"+out[i].Dept] = &out[i]
	}
	for monthStart := ytdStart; monthStart.Before(end); monthStart = monthStart.AddDate(0, 1, 0) {
		monthEnd := monthStart.AddDate(0, 1, 0)
		monthly := workspaceMovementByDeptPeriod(employees, orgs, monthStart, monthEnd, monthStart)
		monthIndex := int(monthStart.Month()) - 1
		for _, stat := range monthly {
			if target := byKey[stat.BU+"\x00"+stat.Dept]; target != nil {
				target.RatePrev[monthIndex] = stat.Prev
				target.RateEnd[monthIndex] = stat.End
				target.RateResigned[monthIndex] = stat.Resigned
			}
		}
	}
	return out
}

func workspaceMovementByDeptPeriod(employees []Employee, orgs map[string]workspaceOrgInfo, start time.Time, end time.Time, ytdStart time.Time) []workspaceMovementStats {
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
		workspaceFinalizeMovement(stat)
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
	out := workspaceMovementByBUPeriod(employees, orgs, start, end)
	byKey := map[string]*workspaceMovementStats{}
	for i := range out {
		byKey[out[i].BU] = &out[i]
	}
	for monthStart := start; monthStart.Before(end); monthStart = monthStart.AddDate(0, 1, 0) {
		monthEnd := monthStart.AddDate(0, 1, 0)
		monthly := workspaceMovementByBUPeriod(employees, orgs, monthStart, monthEnd)
		monthIndex := int(monthStart.Month()) - 1
		for _, stat := range monthly {
			if target := byKey[stat.BU]; target != nil {
				target.RatePrev[monthIndex] = stat.Prev
				target.RateEnd[monthIndex] = stat.End
				target.RateResigned[monthIndex] = stat.Resigned
			}
		}
	}
	return out
}

func workspaceMovementByBUPeriod(employees []Employee, orgs map[string]workspaceOrgInfo, start time.Time, end time.Time) []workspaceMovementStats {
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
		workspaceFinalizeMovement(stat)
		out = append(out, *stat)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].BU < out[j].BU })
	return out
}

// workspaceApplyMovement 處理工作區 apply movement。
func workspaceApplyMovement(stat *workspaceMovementStats, employee Employee, start time.Time, end time.Time, ytdStart time.Time) {
	if workspaceEmployeeInTurnoverClosingHeadcountAt(employee, start) {
		stat.Prev++
	}
	if workspaceEmployeeInTurnoverClosingHeadcountAt(employee, ytdStart) {
		stat.Base++
	}
	if employee.HireDate != nil && !employee.HireDate.Before(start) && employee.HireDate.Before(end) {
		stat.Hires++
	}
	if employee.HireDate != nil && !employee.HireDate.Before(ytdStart) && employee.HireDate.Before(end) {
		stat.YTDHires++
	}
	if separatedAt, ok := workspaceEmployeeSeparationTime(employee); ok {
		if workspaceSeparationOnClosingDay(separatedAt, start) {
			stat.PrevLastDaySeparations++
		}
		if !separatedAt.Before(start) && separatedAt.Before(end) {
			if workspaceEmployeeIsLayoff(employee) {
				stat.Layoff++
			} else {
				stat.Resigned++
			}
			if !workspaceSeparationOnClosingDay(separatedAt, end) {
				stat.NonLastDaySeparations++
			}
		}
	}
	if workspaceEmployeeSeparatedInRange(employee, ytdStart, end) && !workspaceEmployeeIsLayoff(employee) {
		stat.YTDSep++
	}
	if workspaceEmployeeLeaveSuspendedInRange(employee, start, end) {
		stat.OnLeave++
	}
	if workspaceEmployeeLeaveSuspendedInRange(employee, ytdStart, end) {
		stat.YTDOnLeave++
	}
}

func workspaceFinalizeMovement(stat *workspaceMovementStats) {
	stat.End = maxInt(0, stat.Prev-stat.PrevLastDaySeparations-stat.NonLastDaySeparations-stat.OnLeave+stat.Hires)
	stat.YTDEnd = stat.End
}

func workspaceCumulativeTurnoverRate(stat workspaceMovementStats) float64 {
	total := 0.0
	for month := 0; month < len(stat.RatePrev); month++ {
		total += workspaceTurnoverRate(stat.RateResigned[month], stat.RatePrev[month], stat.RateEnd[month])
	}
	return total
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
		for month := 0; month < len(total.RatePrev); month++ {
			total.RatePrev[month] += item.RatePrev[month]
			total.RateEnd[month] += item.RateEnd[month]
			total.RateResigned[month] += item.RateResigned[month]
		}
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
		MonthRate: workspaceTurnoverRateLabel(stat.Resigned, stat.Prev, stat.End),
		YTDRate:   fmt.Sprintf("%.1f%%", workspaceCumulativeTurnoverRate(stat)),
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
	return WorkspaceAnnualRow{
		BU:       stat.BU,
		Base:     stat.Base,
		Hires:    stat.Hires,
		Resigned: stat.Resigned,
		Layoff:   stat.Layoff,
		OnLeave:  stat.OnLeave,
		End:      stat.End,
		Sep:      stat.Resigned,
		Rate:     fmt.Sprintf("%.1f%%", workspaceCumulativeTurnoverRate(stat)),
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
	for month := 0; month < len(total.RatePrev); month++ {
		total.RatePrev[month] += item.RatePrev[month]
		total.RateEnd[month] += item.RateEnd[month]
		total.RateResigned[month] += item.RateResigned[month]
	}
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
		{Key: "sep", Label: "本月離職", Value: strconv.Itoa(total.Resigned), Unit: "人", TrendTone: "flat"},
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
		{Key: "sep", Label: "年度離職總數", Value: strconv.Itoa(total.Resigned), Unit: "人", TrendTone: "flat"},
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
		start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0)
		future := end.After(now) && year >= now.Year()
		futures[month-1] = future
		if future {
			continue
		}
		values[month-1] = workspaceMovementForEmployees(employees, start, end).End
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
		stat := workspaceMovementForEmployees(employees, start, end)
		values[month-1] = workspaceTurnoverRate(stat.Resigned, stat.Prev, stat.End)
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

func workspaceMovementForEmployees(employees []Employee, start time.Time, end time.Time) workspaceMovementStats {
	stat := workspaceMovementStats{}
	for _, employee := range employees {
		workspaceApplyMovement(&stat, employee, start, end, start)
	}
	workspaceFinalizeMovement(&stat)
	return stat
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
