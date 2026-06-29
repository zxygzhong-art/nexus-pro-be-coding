package service

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"nexus-pro-be/internal/repository"
	"nexus-pro-be/internal/utils"
)

const (
	workspaceParentNone = "__none__"
	workspaceDayHours   = 8.0
)

// WorkspaceService implements read-only workspace page aggregates.
type WorkspaceService struct {
	*Service
	store repository.Store
}

// Workspace returns the workspace dashboard service facade.
func (c *Service) Workspace() WorkspaceService {
	return WorkspaceService{Service: c, store: c.store}
}

// WorkspaceOverview returns HR, attendance, and lifecycle widgets for one month.
func (c WorkspaceService) WorkspaceOverview(ctx RequestContext, query WorkspaceOverviewQuery) (WorkspaceOverviewResponse, error) {
	now := c.Now()
	start, end := workspaceMonthRange(query.Year, query.Month, now)
	targetDate := workspaceTargetDate(query.Date, start, end, now)

	employees, err := c.visibleWorkspaceEmployees(ctx, "workspace.overview")
	if err != nil {
		return WorkspaceOverviewResponse{}, err
	}
	leaves, err := c.Service.Attendance().ListLeaveRequests(ctx)
	if err != nil {
		return WorkspaceOverviewResponse{}, err
	}
	clocks, err := c.visibleWorkspaceClockRecords(ctx, AttendanceClockRecordQuery{
		FromDate: start.Format(time.DateOnly),
		ToDate:   end.AddDate(0, 0, -1).Format(time.DateOnly),
	})
	if err != nil {
		return WorkspaceOverviewResponse{}, err
	}

	monthLeaves := workspaceFilterLeaves(leaves, start, end)
	targetLeaves := workspaceLeaveEmployeesForDate(monthLeaves, targetDate)
	checkedIn := workspaceCheckedInEmployees(clocks, targetDate)
	activeOnDate := workspaceCountActiveAt(employees, targetDate)
	absent := activeOnDate - len(checkedIn) - len(targetLeaves)
	if absent < 0 {
		absent = 0
	}
	hires := workspaceCountHires(employees, start, end)
	separations := workspaceCountSeparations(employees, start, end)
	activeAtEnd := workspaceCountActiveAt(employees, end.Add(-time.Nanosecond))
	segments := workspaceAttendanceSegments(len(checkedIn), len(targetLeaves), absent)

	return WorkspaceOverviewResponse{
		Month:       start.Format("2006-01"),
		Year:        start.Year(),
		MonthNumber: int(start.Month()),
		HRSummary: WorkspaceHRSummary{
			Title:          fmt.Sprintf("%d年%d月人力概況", start.Year(), int(start.Month())),
			Active:         activeAtEnd,
			Hires:          hires,
			Separations:    separations,
			SeparationRate: workspaceRateString(float64(separations), float64(maxInt(activeAtEnd, 1))),
		},
		Attendance: WorkspaceOverviewAttendance{
			CheckedIn:  len(checkedIn),
			Leave:      len(targetLeaves),
			Absent:     absent,
			Segments:   segments,
			DailyLeave: workspaceDailyLeave(start, end, monthLeaves, targetDate),
		},
		TodoCategories: workspaceTodoCategories(employees, now),
	}, nil
}

// WorkspaceOrganization returns a manager-to-employee hierarchy.
func (c WorkspaceService) WorkspaceOrganization(ctx RequestContext) (WorkspaceOrganizationResponse, error) {
	employees, err := c.visibleWorkspaceEmployees(ctx, "workspace.organization")
	if err != nil {
		return WorkspaceOrganizationResponse{}, err
	}
	units, err := c.Service.HR().ListOrgUnits(ctx)
	if err != nil {
		return WorkspaceOrganizationResponse{}, err
	}
	orgNames := workspaceOrgNames(units)
	displayIDs := workspaceEmployeeDisplayIDs(employees)
	byID := map[string]Employee{}
	managerIDs := map[string]struct{}{}
	for _, employee := range employees {
		byID[employee.ID] = employee
		if employee.ManagerEmployeeID != "" {
			managerIDs[employee.ManagerEmployeeID] = struct{}{}
		}
	}
	rows := make([]WorkspaceOrganizationRow, 0, len(employees))
	levelMemo := map[string]int{}
	for _, employee := range employees {
		displayID := displayIDs[employee.ID]
		parentID := workspaceParentNone
		if employee.ManagerEmployeeID != "" {
			if managerDisplayID, ok := displayIDs[employee.ManagerEmployeeID]; ok {
				parentID = managerDisplayID
			}
		}
		_, isManager := managerIDs[employee.ID]
		rows = append(rows, WorkspaceOrganizationRow{
			ID:        displayID,
			NameZH:    employee.Name,
			NameEN:    workspaceEmployeeNameEN(employee),
			Dept:      workspaceOrgName(orgNames, employee.OrgUnitID),
			Title:     employee.Position,
			Level:     workspaceEmployeeLevel(employee.ID, byID, levelMemo),
			IsManager: isManager,
			ParentID:  parentID,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Level != rows[j].Level {
			return rows[i].Level < rows[j].Level
		}
		if rows[i].ParentID != rows[j].ParentID {
			return rows[i].ParentID < rows[j].ParentID
		}
		return rows[i].ID < rows[j].ID
	})
	return WorkspaceOrganizationResponse{ParentNone: workspaceParentNone, Rows: rows}, nil
}

// WorkspaceTurnover returns monthly and annual employment movement analysis.
func (c WorkspaceService) WorkspaceTurnover(ctx RequestContext, query WorkspaceTurnoverQuery) (WorkspaceTurnoverResponse, error) {
	now := c.Now()
	start, end := workspaceMonthRange(query.Year, query.Month, now)
	annualYear := query.AnnualYear
	if annualYear <= 0 {
		annualYear = start.Year()
	}
	employees, err := c.visibleWorkspaceEmployees(ctx, "workspace.turnover")
	if err != nil {
		return WorkspaceTurnoverResponse{}, err
	}
	units, err := c.Service.HR().ListOrgUnits(ctx)
	if err != nil {
		return WorkspaceTurnoverResponse{}, err
	}
	orgs := workspaceOrgCatalog(units)
	monthly := workspaceMonthlyTurnover(employees, orgs, start, end, now)
	annual := workspaceAnnualTurnover(employees, orgs, annualYear, now)
	return WorkspaceTurnoverResponse{Monthly: monthly, Annual: annual}, nil
}

// WorkspaceAttendance returns monthly work-hour and clock matrices.
func (c WorkspaceService) WorkspaceAttendance(ctx RequestContext, query WorkspaceAttendanceQuery) (WorkspaceAttendanceResponse, error) {
	now := c.Now()
	start, end := workspaceMonthRange(query.Year, query.Month, now)
	employees, err := c.visibleWorkspaceEmployees(ctx, "workspace.attendance")
	if err != nil {
		return WorkspaceAttendanceResponse{}, err
	}
	units, err := c.Service.HR().ListOrgUnits(ctx)
	if err != nil {
		return WorkspaceAttendanceResponse{}, err
	}
	leaves, err := c.Service.Attendance().ListLeaveRequests(ctx)
	if err != nil {
		return WorkspaceAttendanceResponse{}, err
	}
	clocks, err := c.visibleWorkspaceClockRecords(ctx, AttendanceClockRecordQuery{
		FromDate: start.Format(time.DateOnly),
		ToDate:   end.AddDate(0, 0, -1).Format(time.DateOnly),
	})
	if err != nil {
		return WorkspaceAttendanceResponse{}, err
	}
	worksites, err := c.store.ListAttendanceWorksites(goContext(ctx), ctx.TenantID)
	if err != nil {
		return WorkspaceAttendanceResponse{}, err
	}

	dates := workspaceMonthDates(start, end)
	orgNames := workspaceOrgNames(units)
	cards := workspaceEmployeeCards(employees, orgNames)
	monthEmployees := workspaceEmployeesPresentInRange(employees, start, end)
	leaveByEmployeeDate := workspaceLeaveCells(workspaceFilterLeaves(leaves, start, end), start, end)
	clockByEmployeeDate := workspaceClockCells(clocks, worksites)
	attendanceMatrix := workspaceAttendanceMatrix(monthEmployees, cards, dates, leaveByEmployeeDate)
	clockMatrix := workspaceClockMatrix(monthEmployees, cards, dates, leaveByEmployeeDate, clockByEmployeeDate)

	return WorkspaceAttendanceResponse{
		Year:        start.Year(),
		Month:       int(start.Month()),
		IsFuture:    start.After(now),
		Label:       fmt.Sprintf("%d 年 %d 月", start.Year(), int(start.Month())),
		PeriodLabel: fmt.Sprintf("%d 年 %d/%d-%d/%d 期間", start.Year(), int(start.Month()), start.Day(), int(end.AddDate(0, 0, -1).Month()), end.AddDate(0, 0, -1).Day()),
		Dates:       dates,
		LeaveLegend: workspaceLeaveLegend(),
		Attendance:  attendanceMatrix,
		Clock:       clockMatrix,
	}, nil
}

// WorkspaceAdmins returns the HR workspace administrator projection.
func (c WorkspaceService) WorkspaceAdmins(ctx RequestContext) (WorkspaceAdminsResponse, error) {
	if _, _, err := c.Service.requireServiceAuthz(ctx, AppIAM, ResourcePermissionAssign, ActionRead, ""); err != nil {
		return WorkspaceAdminsResponse{}, err
	}
	accounts, err := c.store.ListAccounts(goContext(ctx), ctx.TenantID)
	if err != nil {
		return WorkspaceAdminsResponse{}, err
	}
	employees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
	if err != nil {
		return WorkspaceAdminsResponse{}, err
	}
	units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return WorkspaceAdminsResponse{}, err
	}
	permissionSets, err := c.store.ListPermissionSets(goContext(ctx), ctx.TenantID)
	if err != nil {
		return WorkspaceAdminsResponse{}, err
	}
	assignments, err := c.store.ListPermissionSetAssignments(goContext(ctx), ctx.TenantID)
	if err != nil {
		return WorkspaceAdminsResponse{}, err
	}
	orgNames := workspaceOrgNames(units)
	employeeByID := map[string]Employee{}
	for _, employee := range employees {
		employeeByID[employee.ID] = employee
	}
	accountByEmployeeID := map[string]Account{}
	for _, account := range accounts {
		if account.EmployeeID != "" {
			accountByEmployeeID[account.EmployeeID] = account
		}
	}
	permissionSetByID := map[string]PermissionSet{}
	for _, permissionSet := range permissionSets {
		permissionSetByID[permissionSet.ID] = permissionSet
	}
	assignmentsByAccount := map[string][]PermissionSetAssignment{}
	for _, assignment := range assignments {
		if assignment.PrincipalType != string(PrincipalTypeAccount) || !workspaceAssignmentActive(assignment, c.Now()) {
			continue
		}
		assignmentsByAccount[assignment.PrincipalID] = append(assignmentsByAccount[assignment.PrincipalID], assignment)
	}

	admins := []WorkspaceAdmin{}
	adminAccountIDs := map[string]struct{}{}
	for _, account := range accounts {
		permissions, assignedAt := workspaceAdminPermissions(account, assignmentsByAccount[account.ID], permissionSetByID)
		if !workspaceHasAdminPermissions(permissions) {
			continue
		}
		employee := employeeByID[account.EmployeeID]
		adminAccountIDs[account.ID] = struct{}{}
		admins = append(admins, WorkspaceAdmin{
			ID:          workspaceAdminDisplayID(account, employee),
			AccountID:   account.ID,
			Avatar:      workspaceAvatar(workspaceAdminName(account, employee)),
			NameZH:      workspaceAdminName(account, employee),
			NameEN:      workspaceEmployeeNameEN(employee),
			Dept:        workspaceOrgName(orgNames, employee.OrgUnitID),
			Title:       employee.Position,
			AssignedAt:  workspaceFormatAdminTime(assignedAt),
			AssignedBy:  "系統",
			Permissions: permissions,
		})
	}
	sort.SliceStable(admins, func(i, j int) bool {
		if admins[i].AssignedAt != admins[j].AssignedAt {
			return admins[i].AssignedAt < admins[j].AssignedAt
		}
		return admins[i].ID < admins[j].ID
	})
	candidates := workspaceAdminCandidates(employees, accountByEmployeeID, adminAccountIDs, orgNames)
	return WorkspaceAdminsResponse{Admins: admins, Candidates: candidates, Sections: workspaceAdminSections()}, nil
}

// WorkspaceAuditLogs returns the workspace page audit-log projection.
func (c WorkspaceService) WorkspaceAuditLogs(ctx RequestContext, query WorkspaceAuditLogQuery, page PageRequest) (PageResponse[WorkspaceAuditLog], error) {
	if _, _, _, err := c.Authorize(ctx, CheckRequest{Resource: "audit.log", Action: ActionRead}, AuditTarget{Event: "workspace.audit_log.query", Resource: "audit_log"}); err != nil {
		return PageResponse[WorkspaceAuditLog]{}, err
	}
	logs, err := c.store.ListAuditLogs(goContext(ctx), ctx.TenantID)
	if err != nil {
		return PageResponse[WorkspaceAuditLog]{}, err
	}
	accounts, err := c.store.ListAccounts(goContext(ctx), ctx.TenantID)
	if err != nil {
		return PageResponse[WorkspaceAuditLog]{}, err
	}
	employees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
	if err != nil {
		return PageResponse[WorkspaceAuditLog]{}, err
	}
	accountByID := map[string]Account{}
	for _, account := range accounts {
		accountByID[account.ID] = account
	}
	employeeByID := map[string]Employee{}
	for _, employee := range employees {
		employeeByID[employee.ID] = employee
	}
	projected := make([]WorkspaceAuditLog, 0, len(logs))
	for _, log := range logs {
		if !workspaceAuditLogMatches(log, query, accountByID, employeeByID) {
			continue
		}
		projected = append(projected, workspaceAuditLogProjection(log, accountByID, employeeByID))
	}
	sort.SliceStable(projected, func(i, j int) bool {
		return projected[i].Time > projected[j].Time
	})
	return utils.PageResponse(projected, page), nil
}

// visibleWorkspaceEmployees returns all employees visible under the current HR read scope.
func (c WorkspaceService) visibleWorkspaceEmployees(ctx RequestContext, event string) ([]Employee, error) {
	account, decision, audit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionRead},
		AuditTarget{Event: event, Resource: string(ResourceEmployeeCollection)},
	)
	if err != nil {
		return nil, err
	}
	hr := c.Service.HR()
	query, err := hr.employeeQueryWithDecisionScope(ctx, account, EmployeeQuery{}, decision)
	if err != nil {
		return nil, err
	}
	items, err := hr.listEmployeesForQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	items, err = hr.applyEmployeeDecision(ctx, account, items, decision)
	if err != nil {
		return nil, err
	}
	if err := audit.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

// visibleWorkspaceClockRecords returns clock records visible under the attendance data scope.
func (c WorkspaceService) visibleWorkspaceClockRecords(ctx RequestContext, query AttendanceClockRecordQuery) ([]AttendanceClockRecord, error) {
	attendance := c.Service.Attendance()
	account, decision, err := attendance.requireAttendanceAuthz(ctx, ResourceAttendanceClock, ActionRead, "")
	if err != nil {
		return nil, err
	}
	items, err := c.store.ListAttendanceClockRecords(goContext(ctx), ctx.TenantID, normalizeClockRecordQuery(query))
	if err != nil {
		return nil, err
	}
	return attendance.filterClockRecordsByDecision(ctx, account, decision, items)
}

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

// workspaceMonthRange resolves a year/month pair into a UTC month range.
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

// workspaceTargetDate resolves an overview date inside the selected month.
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

// workspaceFilterLeaves keeps leave requests that overlap the selected range.
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

// workspaceLeaveEffective excludes rejected or cancelled leave requests from reporting.
func workspaceLeaveEffective(item LeaveRequest) bool {
	status := strings.ToLower(strings.TrimSpace(item.Status))
	return status != "rejected" && status != "cancelled" && status != "canceled"
}

// workspaceLeaveEmployeesForDate returns unique employees on leave for a date.
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

// workspaceCheckedInEmployees returns unique employees with an accepted clock-in for a date.
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

// workspaceCountActiveAt counts employees considered active at one point in time.
func workspaceCountActiveAt(items []Employee, at time.Time) int {
	count := 0
	for _, item := range items {
		if workspaceEmployeeActiveAt(item, at) {
			count++
		}
	}
	return count
}

// workspaceEmployeeActiveAt checks lifecycle status and employment dates for headcount.
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

// workspaceEmployeesPresentInRange filters employees that belong in a monthly matrix.
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

// workspaceEmployeeStatus returns the preferred lifecycle status field.
func workspaceEmployeeStatus(item Employee) string {
	return strings.ToLower(utils.FirstNonEmpty(item.EmploymentStatus, item.Status))
}

// workspaceCountHires counts employees whose hire date is within the range.
func workspaceCountHires(items []Employee, start time.Time, end time.Time) int {
	count := 0
	for _, item := range items {
		if item.HireDate != nil && !item.HireDate.Before(start) && item.HireDate.Before(end) {
			count++
		}
	}
	return count
}

// workspaceCountSeparations counts employees whose separation date falls in the range.
func workspaceCountSeparations(items []Employee, start time.Time, end time.Time) int {
	count := 0
	for _, item := range items {
		if workspaceEmployeeSeparatedInRange(item, start, end) {
			count++
		}
	}
	return count
}

// workspaceEmployeeSeparatedInRange detects explicit or status-derived separations.
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

// workspaceAttendanceSegments converts counts into rounded percentage segments.
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

// workspaceDailyLeave builds the monthly leave bar chart.
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

// workspaceTodoCategories builds lifecycle reminder cards from employee state.
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

// workspaceTodoCategory filters and formats one lifecycle reminder category.
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

// workspaceOrgNames builds an organization id-to-name map.
func workspaceOrgNames(units []OrgUnit) map[string]string {
	out := map[string]string{}
	for _, unit := range units {
		out[unit.ID] = unit.Name
	}
	return out
}

// workspaceOrgCatalog builds organization metadata used by turnover grouping.
func workspaceOrgCatalog(units []OrgUnit) map[string]workspaceOrgInfo {
	out := map[string]workspaceOrgInfo{}
	for _, unit := range units {
		out[unit.ID] = workspaceOrgInfo{ID: unit.ID, Name: unit.Name, ParentID: unit.ParentID, Path: utils.CopyStrings(unit.Path)}
	}
	return out
}

// workspaceOrgName resolves a display name for an organization id.
func workspaceOrgName(names map[string]string, id string) string {
	if name := strings.TrimSpace(names[id]); name != "" {
		return name
	}
	if strings.TrimSpace(id) == "" {
		return "未分配"
	}
	return id
}

// workspaceOrgBUAndDept resolves BU and department names from organization hierarchy.
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

// workspaceEmployeeDisplayIDs maps internal employee ids to frontend-facing ids.
func workspaceEmployeeDisplayIDs(employees []Employee) map[string]string {
	out := map[string]string{}
	for _, employee := range employees {
		out[employee.ID] = workspaceEmployeeDisplayID(employee)
	}
	return out
}

// workspaceEmployeeDisplayID returns employee_no when available.
func workspaceEmployeeDisplayID(employee Employee) string {
	return utils.FirstNonEmpty(strings.TrimSpace(employee.EmployeeNo), employee.ID)
}

// workspaceEmployeeLevel computes an employee's tree level with cycle protection.
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

// workspaceMonthlyTurnover aggregates a monthly turnover response.
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

// workspaceAnnualTurnover aggregates an annual turnover response.
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

// workspaceMovementByDept groups movement stats by BU and department.
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

// workspaceMovementByBU groups annual movement stats by BU.
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

// workspaceApplyMovement updates movement counts for one employee.
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

// workspaceMovementTotal sums movement stats.
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

// workspaceMonthlyTurnoverRows formats monthly movement stats for the table.
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

// workspaceMonthlyRow formats one monthly turnover row.
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

// workspaceAnnualTurnoverRows formats annual movement stats for the table.
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

// workspaceAnnualRow formats one annual turnover row.
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

// workspaceSumMovement adds one movement stat into another.
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

// workspaceTurnoverKey returns a stable row key for turnover tables.
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

// workspaceMonthlyKPIs formats monthly KPI cards.
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

// workspaceAnnualKPIs formats annual KPI cards.
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

// workspaceComparisonFromStats builds sorted comparison bars from movement stats.
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

// workspaceHeadcountTrend builds monthly headcount trend points.
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

// workspaceRateTrend builds monthly turnover-rate trend points.
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

// workspacePieFromStats builds an annual BU distribution pie.
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

// workspaceMonthDates builds calendar date columns for a month.
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

// workspaceHolidayName returns known holiday labels when the backend has a deterministic source.
func workspaceHolidayName(_ time.Time) *string {
	return nil
}

// workspaceLeaveLegend returns the standard leave code legend used by the matrix.
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

// workspaceAdminPermissions converts account permission sets into page permission levels.
func workspaceAdminPermissions(account Account, assignments []PermissionSetAssignment, permissionSets map[string]PermissionSet) (map[string]string, time.Time) {
	permissions := workspaceEmptyAdminPermissions()
	permissionSetIDs := utils.CopyStrings(account.DirectPermissionSetIDs)
	assignedAt := account.CreatedAt
	for _, assignment := range assignments {
		if assignment.Effect != "" && assignment.Effect != string(EffectAllow) {
			continue
		}
		permissionSetIDs = append(permissionSetIDs, assignment.PermissionSetID)
		if assignedAt.IsZero() || (!assignment.CreatedAt.IsZero() && assignment.CreatedAt.Before(assignedAt)) {
			assignedAt = assignment.CreatedAt
		}
	}
	for _, id := range uniqueStrings(permissionSetIDs) {
		permissionSet, ok := permissionSets[id]
		if !ok {
			continue
		}
		for _, permission := range permissionSet.Permissions {
			key := workspaceAdminPermissionKey(permission)
			if key == "" {
				continue
			}
			workspaceMergeAdminPermission(permissions, key, workspaceAdminPermissionLevel(permission))
		}
	}
	return permissions, assignedAt
}

// workspaceEmptyAdminPermissions returns the default permission matrix.
func workspaceEmptyAdminPermissions() map[string]string {
	return map[string]string{
		"employees":    "none",
		"salary":       "none",
		"organization": "none",
		"forms":        "none",
		"leave-policy": "none",
		"admins":       "none",
	}
}

// workspaceAdminPermissionKey maps a low-level permission onto one admin page section key.
func workspaceAdminPermissionKey(permission Permission) string {
	resource := strings.ToLower(strings.Join([]string{
		permission.Resource,
		string(permission.ApplicationCode),
		string(permission.ResourceType),
	}, "."))
	switch {
	case strings.Contains(resource, "salary") || strings.Contains(resource, "payroll"):
		return "salary"
	case strings.Contains(resource, "employee"):
		return "employees"
	case strings.Contains(resource, "org_unit") || strings.Contains(resource, "organization"):
		return "organization"
	case strings.Contains(resource, "form") || strings.Contains(resource, "workflow"):
		return "forms"
	case strings.Contains(resource, "attendance") || strings.Contains(resource, "leave") || strings.Contains(resource, "clock") || strings.Contains(resource, "worksite") || strings.Contains(resource, "shift") || strings.Contains(resource, "correction"):
		return "leave-policy"
	case strings.Contains(resource, "iam") || strings.Contains(resource, "permission") || strings.Contains(resource, "user_group") || strings.Contains(resource, "data_scope") || strings.Contains(resource, "field_policy") || strings.Contains(resource, "assumable_role"):
		return "admins"
	default:
		return ""
	}
}

// workspaceAdminPermissionLevel converts action semantics into view/edit levels.
func workspaceAdminPermissionLevel(permission Permission) string {
	switch strings.ToLower(string(permission.Action)) {
	case "", "read", "list", "check", "explain", "simulate":
		return "view"
	default:
		return "edit"
	}
}

// workspaceMergeAdminPermission keeps the strongest level for a permission key.
func workspaceMergeAdminPermission(permissions map[string]string, key string, level string) {
	if level == "edit" || permissions[key] == "" || permissions[key] == "none" {
		permissions[key] = level
		return
	}
	if level == "view" && permissions[key] != "edit" {
		permissions[key] = "view"
	}
}

// workspaceHasAdminPermissions checks whether a permission matrix grants any page section.
func workspaceHasAdminPermissions(permissions map[string]string) bool {
	for _, level := range permissions {
		if level != "" && level != "none" {
			return true
		}
	}
	return false
}

// workspaceAssignmentActive checks assignment time windows.
func workspaceAssignmentActive(assignment PermissionSetAssignment, now time.Time) bool {
	if assignment.StartsAt != nil && assignment.StartsAt.After(now) {
		return false
	}
	if assignment.ExpiresAt != nil && !assignment.ExpiresAt.After(now) {
		return false
	}
	return true
}

// workspaceAdminCandidates returns active account-bound employees that are not already admins.
func workspaceAdminCandidates(employees []Employee, accounts map[string]Account, adminAccountIDs map[string]struct{}, orgNames map[string]string) []WorkspaceAdminCandidate {
	candidates := []WorkspaceAdminCandidate{}
	for _, employee := range employees {
		if workspaceEmployeeStatus(employee) == string(EmployeeStatusDeleted) || workspaceEmployeeStatus(employee) == string(EmployeeStatusResigned) {
			continue
		}
		account, ok := accounts[employee.ID]
		if !ok || account.Status != string(AccountStatusActive) {
			continue
		}
		if _, exists := adminAccountIDs[account.ID]; exists {
			continue
		}
		candidates = append(candidates, WorkspaceAdminCandidate{
			ID:        workspaceEmployeeDisplayID(employee),
			AccountID: account.ID,
			Avatar:    workspaceAvatar(employee.Name),
			NameZH:    employee.Name,
			NameEN:    workspaceEmployeeNameEN(employee),
			Dept:      workspaceOrgName(orgNames, employee.OrgUnitID),
			Title:     employee.Position,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	if len(candidates) > 50 {
		return candidates[:50]
	}
	return candidates
}

// workspaceAdminSections returns the fixed admin-settings section definitions.
func workspaceAdminSections() []WorkspaceAdminSection {
	return []WorkspaceAdminSection{
		{Group: "人員", Items: []WorkspaceAdminSectionItem{
			{Key: "employees", Label: "員工管理", Icon: "users"},
			{Key: "salary", Label: "員工薪資", Icon: "dollar-sign"},
			{Key: "organization", Label: "組織架構", Icon: "network"},
		}},
		{Group: "出勤假勤", Items: []WorkspaceAdminSectionItem{{Key: "leave-policy", Label: "假勤制度", Icon: "calendar-clock"}}},
		{Group: "表單", Items: []WorkspaceAdminSectionItem{{Key: "forms", Label: "表單設計", Icon: "file-text"}}},
		{Group: "系統設定", Items: []WorkspaceAdminSectionItem{{Key: "admins", Label: "管理員設定", Icon: "shield-check"}}},
	}
}

// workspaceAdminDisplayID returns the employee-facing id for an administrator account.
func workspaceAdminDisplayID(account Account, employee Employee) string {
	if employee.ID != "" {
		return workspaceEmployeeDisplayID(employee)
	}
	return account.ID
}

// workspaceAdminName returns the best display name for an administrator account.
func workspaceAdminName(account Account, employee Employee) string {
	return utils.FirstNonEmpty(employee.Name, account.DisplayName, account.Email, account.ID)
}

// workspaceFormatAdminTime formats an assignment timestamp for the admin page.
func workspaceFormatAdminTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006/01/02")
}

// workspaceAuditLogMatches applies workspace audit-log filters.
func workspaceAuditLogMatches(log AuditLog, query WorkspaceAuditLogQuery, accounts map[string]Account, employees map[string]Employee) bool {
	if from, ok := workspaceParseAuditTime(query.From, false); ok && log.CreatedAt.Before(from) {
		return false
	}
	if to, ok := workspaceParseAuditTime(query.To, true); ok && !log.CreatedAt.Before(to) {
		return false
	}
	account := accounts[log.ActorAccountID]
	employee := employees[account.EmployeeID]
	operatorID := strings.TrimSpace(query.OperatorID)
	if operatorID != "" && operatorID != log.ActorAccountID && operatorID != account.EmployeeID && operatorID != workspaceEmployeeDisplayID(employee) {
		return false
	}
	projected := workspaceAuditLogProjection(log, accounts, employees)
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

// workspaceAuditLogProjection converts one audit log into the page-level row.
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

// workspaceAuditOperator resolves an audit actor display name.
func workspaceAuditOperator(log AuditLog, account Account, employee Employee) string {
	return utils.FirstNonEmpty(employee.Name, account.DisplayName, account.Email, log.ActorAccountID, "系統")
}

// workspaceAuditType maps low-level audit resources to workspace page categories.
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

// workspaceAuditAction returns a concise action label.
func workspaceAuditAction(log AuditLog) string {
	action := utils.FirstNonEmpty(log.Action, log.Resource)
	if log.Target != "" {
		return action + " " + log.Target
	}
	return action
}

// workspaceAuditDetail formats audit details without losing structured context.
func workspaceAuditDetail(log AuditLog) string {
	if len(log.Details) > 0 {
		if raw, err := json.Marshal(log.Details); err == nil {
			return string(raw)
		}
	}
	return utils.FirstNonEmpty(log.Result, log.TraceID)
}

// workspaceParseAuditTime parses date and timestamp filters.
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

// workspaceLeaveCells indexes leave cells by employee and date.
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

// workspaceClockCells indexes accepted clock records by employee and date.
func workspaceClockCells(clocks []AttendanceClockRecord, worksites []AttendanceWorksite) map[string]map[string]workspaceClockCell {
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
				cell.Abnormal = true
				cell.Reason = "工時未滿 8 小時"
			}
			out[employeeID][date] = cell
		}
	}
	return out
}

// workspaceAttendanceMatrix builds the work-hour matrix rows and summary.
func workspaceAttendanceMatrix(employees []Employee, cards map[string]WorkspaceEmployeeCard, dates []WorkspaceDate, leaveCells map[string]map[string]workspaceLeaveCell) WorkspaceAttendanceMatrix {
	rows := []WorkspaceAttendanceRow{}
	totalLeaveHours := 0.0
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
			row.Cells = append(row.Cells, cell)
		}
		row.Summary.DueHours = float64(workdays) * workspaceDayHours
		row.Summary.AttendedHours = math.Max(0, row.Summary.DueHours-row.Summary.LeaveHours-row.Summary.DeductHours)
		if row.Summary.LeaveHours == 0 {
			perfect++
		}
		totalLeaveHours += row.Summary.LeaveHours
		rows = append(rows, row)
	}
	return WorkspaceAttendanceMatrix{Rows: rows, Summary: WorkspaceAttendanceMatrixSum{Holidays: holidays, LeaveHours: totalLeaveHours, Perfect: perfect, Workdays: workdays}}
}

// workspaceClockMatrix builds the clock matrix rows, abnormal list, and summary.
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

// workspaceEmployeeCards formats employee metadata for workspace tables.
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

// workspaceBaseDayCell creates a calendar-aware base matrix cell.
func workspaceBaseDayCell(date WorkspaceDate) WorkspaceDayCell {
	if date.Holiday != nil {
		return WorkspaceDayCell{Type: "holiday", Holiday: *date.Holiday}
	}
	if date.DOW == 0 || date.DOW == 6 {
		return WorkspaceDayCell{Type: "weekend"}
	}
	return WorkspaceDayCell{Type: "work"}
}

// workspaceWorkdayCount counts non-weekend, non-holiday dates.
func workspaceWorkdayCount(dates []WorkspaceDate) int {
	count := 0
	for _, date := range dates {
		if workspaceBaseDayCell(date).Type == "work" {
			count++
		}
	}
	return count
}

// workspaceHolidayCount counts holidays in a date slice.
func workspaceHolidayCount(dates []WorkspaceDate) int {
	count := 0
	for _, date := range dates {
		if date.Holiday != nil {
			count++
		}
	}
	return count
}

// workspaceLeaveCodeLabel maps leave types to matrix code labels.
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

// workspaceEmployeeNameEN returns the stored English name when present.
func workspaceEmployeeNameEN(employee Employee) string {
	return workspaceStringFromMaps([]map[string]any{employee.BasicInfo, employee.EmploymentInfo, employee.ContactInfo}, "name_en", "english_name", "en_name")
}

// workspaceEmployeeInfoDate returns the first matching profile date string.
func workspaceEmployeeInfoDate(employee Employee, keys ...string) string {
	return workspaceStringFromMaps([]map[string]any{employee.BasicInfo, employee.EmploymentInfo}, keys...)
}

// workspaceStringFromMaps returns the first string-like value found by key.
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

// workspaceParseFlexibleDate parses the date formats used in profile metadata.
func workspaceParseFlexibleDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.DateOnly, "2006/01/02", time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

// workspaceFormatDateSlash formats a nullable date pointer.
func workspaceFormatDateSlash(value *time.Time) string {
	if value == nil {
		return ""
	}
	return workspaceFormatTimeSlash(*value)
}

// workspaceFormatTimeSlash formats a date as yyyy/mm/dd.
func workspaceFormatTimeSlash(value time.Time) string {
	return value.UTC().Format("2006/01/02")
}

// workspaceAvatar returns the first rune of a display name.
func workspaceAvatar(name string) string {
	runes := []rune(strings.TrimSpace(name))
	if len(runes) == 0 {
		return ""
	}
	return string(runes[0])
}

// workspaceCategoryLabel localizes employee category values.
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

// workspaceStatusLabel localizes employee status values.
func workspaceStatusLabel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case string(EmployeeStatusActive), "":
		return "在職"
	case string(EmployeeStatusProbation):
		return "試用"
	case string(EmployeeStatusOnboarding):
		return "待加入"
	case string(EmployeeStatusLeaveSuspended):
		return "留職停薪"
	case string(EmployeeStatusResigned):
		return "已離職"
	case string(EmployeeStatusDeleted):
		return "已停用"
	default:
		return status
	}
}

// workspaceDateOnly truncates a timestamp to a UTC date.
func workspaceDateOnly(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

// workspaceWeekdayZH formats weekday labels in Traditional Chinese.
func workspaceWeekdayZH(value time.Time) string {
	labels := []string{"週日", "週一", "週二", "週三", "週四", "週五", "週六"}
	return labels[int(value.Weekday())]
}

// workspaceMonthNameZH formats Traditional Chinese month names.
func workspaceMonthNameZH(month time.Month) string {
	names := []string{"", "一月", "二月", "三月", "四月", "五月", "六月", "七月", "八月", "九月", "十月", "十一月", "十二月"}
	return names[int(month)]
}

// workspaceRate returns a percentage value.
func workspaceRate(numerator int, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator) * 100
}

// workspaceRateString returns a one-decimal percentage without the percent sign.
func workspaceRateString(numerator float64, denominator float64) string {
	if denominator <= 0 {
		return "0.0"
	}
	return fmt.Sprintf("%.1f", numerator/denominator*100)
}

// workspaceRateLabel returns a one-decimal percentage label.
func workspaceRateLabel(numerator int, denominator int) string {
	return fmt.Sprintf("%.1f%%", workspaceRate(numerator, denominator))
}

// workspacePercent returns a rounded integer percentage.
func workspacePercent(value int, total int) int {
	if total <= 0 {
		return 0
	}
	return int(math.Round(float64(value) / float64(total) * 100))
}

// workspacePercentFloat returns a rounded integer percentage from floats.
func workspacePercentFloat(value float64, total float64) int {
	if total <= 0 {
		return 0
	}
	return int(math.Round(value / total * 100))
}

// sortedKeys returns sorted map keys for deterministic output.
func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// maxInt returns the larger integer.
func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

// maxTime returns the later time.
func maxTime(a time.Time, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// minTime returns the earlier time.
func minTime(a time.Time, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
