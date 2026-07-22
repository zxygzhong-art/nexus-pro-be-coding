package service

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"nexus-pro-api/internal/repository"
	"nexus-pro-api/internal/utils"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	workspaceParentNone                = "__none__"
	workspaceDayHours                  = 8.0
	workspaceAttendanceProjection      = "attendance"
	workspaceClockProjection           = "clock"
	workspaceAttendanceDefaultPageSize = 50
	workspaceAttendanceMaximumPageSize = 100
)

// WorkspaceService 定義工作區服務的資料結構。
type WorkspaceService struct {
	*Service
	store repository.Store
}

// Workspace 處理工作區的服務流程。
func (c *Service) Workspace() WorkspaceService {
	return WorkspaceService{Service: c, store: c.store}
}

// WorkspaceOverview 處理工作區總覽的服務流程。
func (c WorkspaceService) WorkspaceOverview(ctx RequestContext, query WorkspaceOverviewQuery) (WorkspaceOverviewResponse, error) {
	now := c.Now()
	start, end := workspaceMonthRange(query.Year, query.Month, now)
	targetDate := workspaceTargetDate(query.Date, start, end, now)

	employees, err := c.visibleWorkspaceEmployees(ctx, "workspace.overview")
	if err != nil {
		return WorkspaceOverviewResponse{}, err
	}
	leaveEmployeeIDs, err := c.workspaceAuthorizedAttendanceEmployeeIDs(ctx, ResourceLeave, employees)
	if err != nil {
		return WorkspaceOverviewResponse{}, err
	}
	effectiveLeaves := []attendanceEffectiveLeave{}
	if len(leaveEmployeeIDs) > 0 {
		effectiveLeaves, err = c.Service.Attendance().loadEffectiveAttendanceLeaves(ctx, leaveEmployeeIDs, start.Format(time.DateOnly), end.AddDate(0, 0, -1).Format(time.DateOnly))
		if err != nil {
			return WorkspaceOverviewResponse{}, err
		}
	}
	leaves := workspaceApprovedLeaveRequestsFromEffective(effectiveLeaves)
	clocks, err := c.visibleWorkspaceClockRecords(ctx, AttendanceClockRecordQuery{
		FromDate: start.Format(time.DateOnly),
		ToDate:   end.AddDate(0, 0, -1).Format(time.DateOnly),
	})
	if err != nil {
		return WorkspaceOverviewResponse{}, err
	}
	summaries, err := c.visibleWorkspaceDailySummaries(ctx, AttendanceDailySummaryQuery{
		FromDate: start.Format(time.DateOnly),
		ToDate:   end.AddDate(0, 0, -1).Format(time.DateOnly),
		Source:   "ehrms",
	})
	if err != nil {
		return WorkspaceOverviewResponse{}, err
	}

	monthLeaves := workspaceFilterLeaves(leaves, start, end)
	targetLeaves := workspaceLeaveEmployeesForDate(monthLeaves, targetDate)
	checkedIn := workspaceCheckedInEmployees(clocks, targetDate)
	for employeeID := range workspaceCheckedInEmployeesFromSummaries(summaries, targetDate) {
		checkedIn[employeeID] = struct{}{}
	}
	activeOnDate := workspaceCountActiveAt(employees, targetDate)
	absent := activeOnDate - len(checkedIn) - len(targetLeaves)
	if absent < 0 {
		absent = 0
	}
	hires := workspaceCountHires(employees, start, end)
	separations := workspaceCountSeparations(employees, start, end)
	activeAtStart := workspaceCountActiveAt(employees, start.Add(-time.Nanosecond))
	activeAtEnd := workspaceCountActiveAt(employees, end.Add(-time.Nanosecond))
	segments := workspaceAttendanceSegments(len(checkedIn), len(targetLeaves), absent)

	return WorkspaceOverviewResponse{
		Month:       start.Format("2006-01"),
		Year:        start.Year(),
		MonthNumber: int(start.Month()),
		HRSummary: WorkspaceHRSummary{
			Title:       fmt.Sprintf("%d年%d月人力概況", start.Year(), int(start.Month())),
			Active:      activeAtEnd,
			Hires:       hires,
			Separations: separations,
			// 離職率口徑與人員異動頁一致：當月離職 ÷ 當月平均在職（月初與月末平均）。
			SeparationRate: workspaceRateString(float64(separations), workspaceAverageHeadcount(activeAtStart, activeAtEnd)),
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

// workspaceApprovedLeaveRequestsFromEffective adapts canonical approved facts
// to the existing workspace metric helpers without reintroducing request rows
// as an approval source.
func workspaceApprovedLeaveRequestsFromEffective(items []attendanceEffectiveLeave) []LeaveRequest {
	out := make([]LeaveRequest, 0, len(items))
	for _, item := range items {
		if item.FactStatus != attendanceLeaveFactApproved {
			continue
		}
		out = append(out, LeaveRequest{
			ID:               item.SourceFactID,
			EmployeeID:       item.EmployeeID,
			LeaveType:        item.LeaveType,
			LeaveTypeID:      item.LeaveTypeID,
			StartAt:          item.StartAt,
			EndAt:            item.EndAt,
			RequestedMinutes: item.NetMinutes,
			Status:           "approved",
		})
	}
	return out
}

// WorkspaceOrgUnitsDirectory 返回組織單位管理頁所需的完整後端投影。
func (c WorkspaceService) WorkspaceOrgUnitsDirectory(ctx RequestContext, includeEmployees bool) (WorkspaceOrgUnitDirectoryResponse, error) {
	units, err := c.Service.HR().ListOrgUnits(ctx)
	if err != nil {
		return WorkspaceOrgUnitDirectoryResponse{}, err
	}
	units = utils.SortOrgUnits(units, "code_asc")
	rows := make([]WorkspaceOrgUnitDirectoryRow, 0, len(units))
	rowIndexes := make(map[string]int, len(units))
	for _, unit := range units {
		rowIndexes[unit.ID] = len(rows)
		rows = append(rows, WorkspaceOrgUnitDirectoryRow{
			OrgUnit:         unit,
			DirectEmployees: []WorkspaceOrgUnitDirectoryEmployee{},
		})
	}
	response := WorkspaceOrgUnitDirectoryResponse{
		Rows:                rows,
		UnassignedEmployees: []WorkspaceOrgUnitDirectoryEmployee{},
		EmployeesIncluded:   includeEmployees,
	}
	if !includeEmployees {
		return response, nil
	}

	employees, err := c.visibleWorkspaceEmployees(ctx, "workspace.org_units_directory")
	if err != nil {
		return WorkspaceOrgUnitDirectoryResponse{}, err
	}
	employees = workspaceDirectoryEmployees(employees, c.Now())
	for _, employee := range employees {
		summary := WorkspaceOrgUnitDirectoryEmployee{
			ID:           employee.ID,
			EmployeeNo:   employee.EmployeeNo,
			Name:         employee.Name,
			CompanyEmail: employee.CompanyEmail,
			OrgUnitID:    employee.OrgUnitID,
			Position:     employee.Position,
		}
		if strings.TrimSpace(employee.OrgUnitID) == "" {
			response.UnassignedEmployees = append(response.UnassignedEmployees, summary)
			continue
		}
		index, visibleUnit := rowIndexes[employee.OrgUnitID]
		if !visibleUnit {
			continue
		}
		response.Rows[index].DirectEmployees = append(response.Rows[index].DirectEmployees, summary)
	}
	for index := range response.Rows {
		sortWorkspaceOrgUnitDirectoryEmployees(response.Rows[index].DirectEmployees)
	}
	sortWorkspaceOrgUnitDirectoryEmployees(response.UnassignedEmployees)
	return response, nil
}

func sortWorkspaceOrgUnitDirectoryEmployees(employees []WorkspaceOrgUnitDirectoryEmployee) {
	sort.SliceStable(employees, func(i, j int) bool {
		if employees[i].Name != employees[j].Name {
			return employees[i].Name < employees[j].Name
		}
		return employees[i].ID < employees[j].ID
	})
}

// WorkspaceOrganization 處理工作區 organization 的服務流程。
func (c WorkspaceService) WorkspaceOrganization(ctx RequestContext) (WorkspaceOrganizationResponse, error) {
	employees, err := c.visibleWorkspaceEmployees(ctx, "workspace.organization")
	if err != nil {
		return WorkspaceOrganizationResponse{}, err
	}
	employees = workspaceOrganizationEmployees(employees, c.Now())
	units, err := c.Service.HR().ListOrgUnits(ctx)
	if err != nil {
		return WorkspaceOrganizationResponse{}, err
	}
	orgNames := workspaceOrgNames(units)
	activeUnits := make([]OrgUnit, 0, len(units))
	for _, unit := range units {
		if !unit.Closed {
			activeUnits = append(activeUnits, unit)
		}
	}
	activeUnits = utils.SortOrgUnits(activeUnits, "code_asc")
	displayIDs := workspaceEmployeeDisplayIDs(employees)
	byID := map[string]Employee{}
	for _, employee := range employees {
		byID[employee.ID] = employee
	}
	now := c.Now()
	effectiveParents := map[string]string{}
	managerIssues := map[string]string{}
	managerSources := map[string]string{}
	managerIDs := map[string]struct{}{}
	for _, employee := range employees {
		resolved := ResolveEffectiveManager(employee, employees, units, now)
		effectiveParents[employee.ID] = strings.TrimSpace(resolved.ManagerEmployeeID)
		managerSources[employee.ID] = resolved.Source
		managerIssues[employee.ID] = resolved.Issue
		if resolved.ManagerEmployeeID != "" {
			managerIDs[resolved.ManagerEmployeeID] = struct{}{}
		}
	}
	rows := make([]WorkspaceOrganizationRow, 0, len(employees))
	levelMemo := map[string]int{}
	for _, employee := range employees {
		displayID := displayIDs[employee.ID]
		managerID := effectiveParents[employee.ID]
		source := managerSources[employee.ID]
		if source == "" {
			source = managerSourceNone
		}
		parentID := workspaceParentNone
		if managerID != "" {
			if managerDisplayID, ok := displayIDs[managerID]; ok {
				parentID = managerDisplayID
			}
		}
		_, isManager := managerIDs[employee.ID]
		rows = append(rows, WorkspaceOrganizationRow{
			ID:             displayID,
			EmployeeID:     employee.ID,
			NameZH:         employee.Name,
			NameEN:         workspaceEmployeeNameEN(employee),
			Dept:           workspaceOrgName(orgNames, employee.OrgUnitID),
			Title:          employee.Position,
			Level:          workspaceEffectiveEmployeeLevel(employee.ID, effectiveParents, byID, levelMemo),
			IsManager:      isManager,
			ShowInOrgChart: employee.ShowInOrgChart,
			ParentID:       parentID,
			OrgUnitID:      employee.OrgUnitID,
			ManagerSource:  source,
			ManagerIssue:   managerIssues[employee.ID],
			IsOverride:     source == managerSourceOverride,
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
	return WorkspaceOrganizationResponse{ParentNone: workspaceParentNone, OrgUnits: activeUnits, Rows: rows}, nil
}

// WorkspaceTurnover 處理工作區人員異動的服務流程。
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

// WorkspaceAttendance 處理工作區考勤的服務流程。
func (c WorkspaceService) WorkspaceAttendance(ctx RequestContext, query WorkspaceAttendanceQuery) (WorkspaceAttendanceResponse, error) {
	query, err := normalizeWorkspaceAttendanceQuery(query)
	if err != nil {
		return WorkspaceAttendanceResponse{}, err
	}
	now := c.Now()
	start, end := workspaceMonthRange(query.Year, query.Month, now)
	employees, total, err := c.visibleWorkspaceAttendanceEmployeePage(ctx, query, start, end, "workspace.attendance")
	if err != nil {
		return WorkspaceAttendanceResponse{}, err
	}
	units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return WorkspaceAttendanceResponse{}, err
	}
	leaveTypes, err := c.Attendance().loadLeaveTypes(ctx)
	if err != nil {
		return WorkspaceAttendanceResponse{}, err
	}

	employeeIDs := workspaceEmployeeIDs(employees)
	fromDate := start.Format(time.DateOnly)
	toDate := end.AddDate(0, 0, -1).Format(time.DateOnly)
	dates := workspaceMonthDates(start, end)
	clocks := []AttendanceClockRecord{}
	summaries := []AttendanceDailySummary{}
	overtimes := []OvertimeRequest{}
	leaves := []attendanceEffectiveLeave{}
	policiesByDate := map[string]AttendancePolicyResponse{}
	workTimesByDate := map[string]AttendancePolicyWorkTime{}
	if len(employeeIDs) > 0 {
		for _, date := range dates {
			policy, policyErr := c.Service.Attendance().loadAttendancePolicyResponseForWorkDate(ctx, date.Key)
			if policyErr != nil {
				return WorkspaceAttendanceResponse{}, policyErr
			}
			policiesByDate[date.Key] = policy
			workTimesByDate[date.Key] = policy.WorkTime
		}
		clocks, err = c.store.ListAttendanceClockRecords(goContext(ctx), ctx.TenantID, normalizeClockRecordQuery(AttendanceClockRecordQuery{
			EmployeeIDs: employeeIDs,
			FromDate:    fromDate,
			ToDate:      toDate,
		}))
		if err != nil {
			return WorkspaceAttendanceResponse{}, err
		}
		summaries, err = c.store.ListAttendanceDailySummaries(goContext(ctx), ctx.TenantID, normalizeAttendanceDailySummaryQuery(AttendanceDailySummaryQuery{
			EmployeeIDs: employeeIDs,
			FromDate:    fromDate,
			ToDate:      toDate,
			Source:      "ehrms",
		}))
		if err != nil {
			return WorkspaceAttendanceResponse{}, err
		}
		leaveEmployeeIDs, scopeErr := c.workspaceAuthorizedAttendanceEmployeeIDs(ctx, ResourceLeave, employees)
		if scopeErr != nil {
			return WorkspaceAttendanceResponse{}, scopeErr
		}
		if len(leaveEmployeeIDs) > 0 {
			overtimes, err = c.store.ListOvertimeRequestsByQuery(goContext(ctx), ctx.TenantID, normalizeOvertimeRequestQuery(OvertimeRequestQuery{
				EmployeeIDs: leaveEmployeeIDs,
				Status:      "approved",
				FromDate:    fromDate,
				ToDate:      toDate,
			}))
			if err != nil {
				return WorkspaceAttendanceResponse{}, err
			}
			leaves, err = c.Service.Attendance().loadEffectiveAttendanceLeaves(ctx, leaveEmployeeIDs, fromDate, toDate)
			if err != nil {
				return WorkspaceAttendanceResponse{}, err
			}
		}
	}
	includeAttendance := query.Projection == "" || query.Projection == workspaceAttendanceProjection
	includeClock := query.Projection == "" || query.Projection == workspaceClockProjection
	worksites := []AttendanceWorksite{}
	if includeClock {
		worksites, err = c.store.ListAttendanceWorksites(goContext(ctx), ctx.TenantID)
		if err != nil {
			return WorkspaceAttendanceResponse{}, err
		}
	}

	orgNames := workspaceOrgNames(units)
	cards := workspaceEmployeeCards(employees, orgNames)
	if query.Projection != "" {
		workspaceCompactEmployeeCards(cards)
	}
	leaveLegend := workspaceLeaveLegend(leaveTypes)
	leaveByEmployeeDate := workspaceEffectiveLeaveCells(leaves, workTimesByDate, leaveTypes, leaveLegend, start, end)
	overtimeByEmployeeDate := workspaceOvertimeCells(overtimes, start, end)
	clockByEmployeeDate := workspaceClockCells(clocks, summaries, worksites, leaveByEmployeeDate, overtimeByEmployeeDate)
	attendanceMatrix := WorkspaceAttendanceMatrix{}
	if includeAttendance {
		projections, projectionErr := c.Service.Attendance().projectAttendanceDays(ctx, employeeIDs, clocks, leaves, policiesByDate, fromDate, toDate, now)
		if projectionErr != nil {
			return WorkspaceAttendanceResponse{}, projectionErr
		}
		attendanceEvidenceByEmployeeDate := workspaceAttendanceEvidenceCells(summaries, projections)
		attendanceMatrix = workspaceAttendanceMatrix(employees, cards, dates, leaveByEmployeeDate, overtimeByEmployeeDate, attendanceEvidenceByEmployeeDate, clockByEmployeeDate, now)
	}
	clockMatrix := WorkspaceClockMatrix{}
	if includeClock {
		clockMatrix = workspaceClockMatrix(employees, cards, dates, leaveByEmployeeDate, clockByEmployeeDate, query.Projection == "" || query.IncludeAbnormals)
	}
	var pagination *WorkspaceAttendancePagination
	summaryScope := ""
	if query.Paginated && !query.ForceAll {
		pagination = &WorkspaceAttendancePagination{Total: total, Page: query.Page, PageSize: query.PageSize}
		summaryScope = "page"
	}

	return WorkspaceAttendanceResponse{
		Year:         start.Year(),
		Month:        int(start.Month()),
		IsFuture:     start.After(now),
		Label:        fmt.Sprintf("%d 年 %d 月", start.Year(), int(start.Month())),
		PeriodLabel:  fmt.Sprintf("%d 年 %d/%d-%d/%d 期間", start.Year(), int(start.Month()), start.Day(), int(end.AddDate(0, 0, -1).Month()), end.AddDate(0, 0, -1).Day()),
		Dates:        dates,
		LeaveLegend:  leaveLegend,
		Pagination:   pagination,
		SummaryScope: summaryScope,
		Attendance:   attendanceMatrix,
		Clock:        clockMatrix,
		Projection:   query.Projection,
	}, nil
}

func normalizeWorkspaceAttendanceQuery(query WorkspaceAttendanceQuery) (WorkspaceAttendanceQuery, error) {
	query.Projection = strings.ToLower(strings.TrimSpace(query.Projection))
	query.DepartmentID = strings.TrimSpace(query.DepartmentID)
	query.Keyword = strings.TrimSpace(query.Keyword)
	if query.Projection != "" && query.Projection != workspaceAttendanceProjection && query.Projection != workspaceClockProjection {
		return WorkspaceAttendanceQuery{}, BadRequest("projection must be attendance or clock")
	}
	if query.ForceAll {
		query.Paginated = false
		query.Page = 0
		query.PageSize = 0
		return query, nil
	}
	query.Paginated = query.Paginated || query.Projection != ""
	if !query.Paginated {
		query.Page = 0
		query.PageSize = 0
		return query, nil
	}
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = workspaceAttendanceDefaultPageSize
	}
	if query.PageSize > workspaceAttendanceMaximumPageSize {
		query.PageSize = workspaceAttendanceMaximumPageSize
	}
	return query, nil
}

func workspaceEmployeeIDs(employees []Employee) []string {
	ids := make([]string, 0, len(employees))
	for _, employee := range employees {
		if employee.ID != "" {
			ids = append(ids, employee.ID)
		}
	}
	return ids
}

func workspaceCompactEmployeeCards(cards map[string]WorkspaceEmployeeCard) {
	for employeeID, card := range cards {
		card.Avatar = ""
		card.Email = ""
		card.Title = ""
		card.Type = ""
		card.Phone = ""
		card.Status = ""
		card.HireDate = ""
		cards[employeeID] = card
	}
}

// WorkspaceClockAbnormals returns abnormalities for a bounded employee page.
// The abnormal pagination total is therefore scoped to employee_pagination,
// which is explicit in the response and avoids a hidden tenant-wide scan.
func (c WorkspaceService) WorkspaceClockAbnormals(ctx RequestContext, query WorkspaceClockAbnormalQuery) (WorkspaceClockAbnormalResponse, error) {
	query.Severity = strings.ToLower(strings.TrimSpace(query.Severity))
	query.BaseDepartmentID = strings.TrimSpace(query.BaseDepartmentID)
	query.DepartmentID = strings.TrimSpace(query.DepartmentID)
	query.Keyword = strings.TrimSpace(query.Keyword)
	switch query.Severity {
	case "", "full_day", "missing_punch", "time_anomaly":
	default:
		return WorkspaceClockAbnormalResponse{}, BadRequest("severity must be full_day, missing_punch, or time_anomaly")
	}
	query.Page, query.PageSize = normalizeWorkspaceAttendancePage(query.Page, query.PageSize)
	query.EmployeePage, query.EmployeePageSize = normalizeWorkspaceAttendancePage(query.EmployeePage, query.EmployeePageSize)
	period, err := c.WorkspaceAttendance(ctx, WorkspaceAttendanceQuery{
		Year:             query.Year,
		Month:            query.Month,
		Projection:       workspaceClockProjection,
		DepartmentID:     query.BaseDepartmentID,
		Keyword:          query.Keyword,
		Page:             query.EmployeePage,
		PageSize:         query.EmployeePageSize,
		Paginated:        true,
		IncludeAbnormals: true,
	})
	if err != nil {
		return WorkspaceClockAbnormalResponse{}, err
	}
	filtered := make([]WorkspaceClockAbnormal, 0, len(period.Clock.Abnormals))
	for _, item := range period.Clock.Abnormals {
		if query.DepartmentID != "" && item.Employee.DepartmentID != query.DepartmentID {
			continue
		}
		if query.Severity != "" && workspaceClockAbnormalSeverity(item) != query.Severity {
			continue
		}
		filtered = append(filtered, item)
	}
	total := len(filtered)
	start := (query.Page - 1) * query.PageSize
	items := []WorkspaceClockAbnormal{}
	if start < total {
		end := start + query.PageSize
		if end > total {
			end = total
		}
		items = append(items, filtered[start:end]...)
	}
	employeePagination := WorkspaceAttendancePagination{Page: query.EmployeePage, PageSize: query.EmployeePageSize}
	if period.Pagination != nil {
		employeePagination = *period.Pagination
	}
	return WorkspaceClockAbnormalResponse{
		Items:              items,
		Pagination:         WorkspaceAttendancePagination{Total: total, Page: query.Page, PageSize: query.PageSize},
		SummaryScope:       "employee_page",
		EmployeePagination: employeePagination,
	}, nil
}

func normalizeWorkspaceAttendancePage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = workspaceAttendanceDefaultPageSize
	}
	if pageSize > workspaceAttendanceMaximumPageSize {
		pageSize = workspaceAttendanceMaximumPageSize
	}
	return page, pageSize
}

func workspaceClockAbnormalSeverity(item WorkspaceClockAbnormal) string {
	missingIn := strings.TrimSpace(item.Record.In) == ""
	missingOut := strings.TrimSpace(item.Record.Out) == ""
	switch {
	case missingIn && missingOut:
		return "full_day"
	case missingIn || missingOut:
		return "missing_punch"
	default:
		return "time_anomaly"
	}
}

// ExportWorkspaceAttendanceCSV 匯出工作區考勤 CSV。
func (c WorkspaceService) ExportWorkspaceAttendanceCSV(ctx RequestContext, query WorkspaceAttendanceQuery, kind string) ([]byte, string, error) {
	query.Projection = ""
	query.ForceAll = true
	query.Paginated = false
	query.Page = 0
	query.PageSize = 0
	item, err := c.WorkspaceAttendance(ctx, query)
	if err != nil {
		return nil, "", err
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "attendance"
	}
	var rows [][]string
	switch kind {
	case "attendance":
		rows = workspaceAttendanceCSVRows(item)
	case "clock":
		rows = workspaceClockCSVRows(item)
	case "abnormal":
		rows = workspaceClockAbnormalCSVRows(item)
	default:
		return nil, "", BadRequest("invalid workspace attendance export kind")
	}
	if err := c.audit(ctx, "workspace.attendance.export", string(ResourceAttendanceClock), "", string(SeverityHigh), map[string]any{
		"year": item.Year, "month": item.Month, "kind": kind, "row_count": workspaceCSVDataRows(rows),
	}); err != nil {
		return nil, "", err
	}
	raw, err := workspaceCSV(rows)
	if err != nil {
		return nil, "", err
	}
	return raw, fmt.Sprintf("workspace-attendance-%s-%04d-%02d.csv", kind, item.Year, item.Month), nil
}

// ExportWorkspaceTurnoverCSV 匯出工作區人員異動 CSV。
func (c WorkspaceService) ExportWorkspaceTurnoverCSV(ctx RequestContext, query WorkspaceTurnoverQuery, kind string) ([]byte, string, error) {
	item, err := c.WorkspaceTurnover(ctx, query)
	if err != nil {
		return nil, "", err
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "monthly"
	}
	var rows [][]string
	var year, month int
	switch kind {
	case "monthly":
		year, month = item.Monthly.Year, item.Monthly.Month
		rows = workspaceTurnoverMonthlyCSVRows(item.Monthly)
	case "annual":
		year = item.Annual.Year
		rows = workspaceTurnoverAnnualCSVRows(item.Annual)
	default:
		return nil, "", BadRequest("invalid workspace turnover export kind")
	}
	if err := c.audit(ctx, "workspace.turnover.export", string(ResourceEmployeeCollection), "", string(SeverityHigh), map[string]any{
		"year": year, "month": month, "kind": kind, "row_count": workspaceCSVDataRows(rows),
	}); err != nil {
		return nil, "", err
	}
	raw, err := workspaceCSV(rows)
	if err != nil {
		return nil, "", err
	}
	if kind == "annual" {
		return raw, fmt.Sprintf("workspace-turnover-annual-%04d.csv", year), nil
	}
	return raw, fmt.Sprintf("workspace-turnover-monthly-%04d-%02d.csv", year, month), nil
}

func workspaceAttendanceCSVRows(item WorkspaceAttendanceResponse) [][]string {
	header := []string{"員工編號", "部門", "姓名", "英文名", "應出勤天數", "應出勤時數", "實際工時", "計入出勤時數", "請假時數", "加班時數", "扣除時數"}
	for _, date := range item.Dates {
		header = append(header, fmt.Sprintf("%d/%d", date.M, date.D))
	}
	rows := [][]string{header}
	for _, row := range item.Attendance.Rows {
		record := []string{
			row.Employee.ID,
			row.Employee.Dept,
			row.Employee.NameZH,
			row.Employee.NameEN,
			strconv.Itoa(row.Summary.WorkDays),
			workspaceFloat(row.Summary.DueHours),
			workspaceFloat(row.Summary.ActualHours),
			workspaceFloat(row.Summary.AttendedHours),
			workspaceFloat(row.Summary.LeaveHours),
			workspaceFloat(row.Summary.OvertimeHours),
			workspaceFloat(row.Summary.DeductHours),
		}
		for _, cell := range row.Cells {
			record = append(record, workspaceAttendanceCellLabel(cell))
		}
		rows = append(rows, record)
	}
	return rows
}

func workspaceClockCSVRows(item WorkspaceAttendanceResponse) [][]string {
	header := []string{"員工編號", "部門", "姓名", "英文名", "日期", "星期", "上班打卡", "上班地點", "下班打卡", "下班地點", "備註"}
	rows := [][]string{header}
	for _, row := range item.Clock.Rows {
		for i, cell := range row.Cells {
			if i >= len(item.Dates) {
				continue
			}
			date := item.Dates[i]
			if cell.Type != "work" && !(cell.Type == "leave" && cell.Hours == 4) {
				continue
			}
			rows = append(rows, []string{
				row.Employee.ID,
				row.Employee.Dept,
				row.Employee.NameZH,
				row.Employee.NameEN,
				fmt.Sprintf("%d/%d", date.M, date.D),
				strconv.Itoa(date.DOW),
				cell.In,
				cell.InLoc,
				cell.Out,
				cell.OutLoc,
				workspaceClockNote(cell),
			})
		}
	}
	return rows
}

func workspaceClockAbnormalCSVRows(item WorkspaceAttendanceResponse) [][]string {
	header := []string{"員工編號", "部門", "姓名", "英文名", "日期", "星期", "上班打卡", "上班地點", "下班打卡", "下班地點", "異常原因"}
	rows := [][]string{header}
	for _, row := range item.Clock.Abnormals {
		rows = append(rows, []string{
			row.Employee.ID,
			row.Employee.Dept,
			row.Employee.NameZH,
			row.Employee.NameEN,
			fmt.Sprintf("%d/%d", row.Date.M, row.Date.D),
			strconv.Itoa(row.Date.DOW),
			row.Record.In,
			row.Record.InLoc,
			row.Record.Out,
			row.Record.OutLoc,
			firstNonEmpty(row.Record.Reason, "需補卡"),
		})
	}
	return rows
}

func workspaceTurnoverMonthlyCSVRows(item WorkspaceTurnoverMonthly) [][]string {
	rows := [][]string{append([]string(nil), item.CSVHeaders...)}
	for _, row := range item.Rows {
		rows = append(rows, []string{
			row.BU,
			row.Dept,
			strconv.Itoa(row.Prev),
			strconv.Itoa(row.Hires),
			strconv.Itoa(row.Resigned),
			strconv.Itoa(row.Layoff),
			strconv.Itoa(row.OnLeave),
			strconv.Itoa(row.End),
			row.MonthRate,
			row.YTDRate,
		})
	}
	return rows
}

func workspaceTurnoverAnnualCSVRows(item WorkspaceTurnoverAnnual) [][]string {
	rows := [][]string{append([]string(nil), item.CSVHeaders...)}
	for _, row := range item.Rows {
		rows = append(rows, []string{
			row.BU,
			strconv.Itoa(row.Base),
			strconv.Itoa(row.Hires),
			strconv.Itoa(row.Resigned),
			strconv.Itoa(row.Layoff),
			strconv.Itoa(row.OnLeave),
			strconv.Itoa(row.End),
			row.Rate,
		})
	}
	return rows
}

func workspaceCSV(rows [][]string) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)
	for _, row := range rows {
		record := make([]string, 0, len(row))
		for _, cell := range row {
			record = append(record, neutralizeSpreadsheetCell(cell))
		}
		if err := w.Write(record); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func workspaceCSVDataRows(rows [][]string) int {
	if len(rows) == 0 {
		return 0
	}
	return len(rows) - 1
}

func workspaceAttendanceCellLabel(cell WorkspaceDayCell) string {
	if cell.Type == "absence" {
		return "缺勤"
	}
	if cell.Holiday != "" {
		return cell.Holiday
	}
	if cell.Leave != "" {
		return firstNonEmpty(cell.Label, cell.Leave)
	}
	if cell.Label != "" {
		return cell.Label
	}
	if cell.Hours > 0 {
		return workspaceFloat(cell.Hours)
	}
	return ""
}

func workspaceClockNote(cell WorkspaceDayCell) string {
	if cell.Abnormal {
		return fmt.Sprintf("打卡異常:%s（需補卡）", firstNonEmpty(cell.Reason, "需補卡"))
	}
	if cell.Type == "leave" {
		return fmt.Sprintf("%s（半天）", firstNonEmpty(cell.Label, "請假"))
	}
	return ""
}

func workspaceFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// WorkspaceAuditLogs 處理工作區稽覈 logs 的服務流程。
func (c WorkspaceService) WorkspaceAuditLogs(ctx RequestContext, query WorkspaceAuditLogQuery, page PageRequest) (PageResponse[WorkspaceAuditLog], error) {
	if _, _, _, err := c.Authorize(ctx, CheckRequest{Resource: "audit.log", Action: ActionRead}, AuditTarget{Event: "workspace.audit_log.query", Resource: "audit_log"}); err != nil {
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
	if workspaceAuditLogQueryEmpty(query) {
		page = utils.NormalizePageRequest(page)
		logs, total, err := c.store.ListAuditLogPage(goContext(ctx), ctx.TenantID, page)
		if err != nil {
			return PageResponse[WorkspaceAuditLog]{}, err
		}
		projected := make([]WorkspaceAuditLog, 0, len(logs))
		for _, log := range logs {
			projected = append(projected, workspaceAuditLogProjection(log, accountByID, employeeByID))
		}
		return utils.PageResponseFromStore(projected, total, page), nil
	}
	page = utils.NormalizePageRequest(page)
	logs, total, err := c.store.ListAuditLogPageFiltered(goContext(ctx), ctx.TenantID, query, page)
	if err != nil {
		return PageResponse[WorkspaceAuditLog]{}, err
	}
	projected := make([]WorkspaceAuditLog, 0, len(logs))
	for _, log := range logs {
		projected = append(projected, workspaceAuditLogProjection(log, accountByID, employeeByID))
	}
	return utils.PageResponseFromStore(projected, total, page), nil
}

// WorkspaceAuditLogFacets builds tenant-wide filter options without reading sensitive audit details.
func (c WorkspaceService) WorkspaceAuditLogFacets(ctx RequestContext) (WorkspaceAuditLogFacets, error) {
	if _, _, _, err := c.Authorize(ctx, CheckRequest{Resource: "audit.log", Action: ActionRead}, AuditTarget{Event: "workspace.audit_log.facets", Resource: "audit_log"}); err != nil {
		return WorkspaceAuditLogFacets{}, err
	}
	sources, err := c.store.ListAuditLogFacetSources(goContext(ctx), ctx.TenantID)
	if err != nil {
		return WorkspaceAuditLogFacets{}, err
	}
	accounts, err := c.store.ListAccounts(goContext(ctx), ctx.TenantID)
	if err != nil {
		return WorkspaceAuditLogFacets{}, err
	}
	employees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
	if err != nil {
		return WorkspaceAuditLogFacets{}, err
	}
	accountByID := make(map[string]Account, len(accounts))
	for _, account := range accounts {
		accountByID[account.ID] = account
	}
	employeeByID := make(map[string]Employee, len(employees))
	for _, employee := range employees {
		employeeByID[employee.ID] = employee
	}

	operatorByID := map[string]WorkspaceAuditLogOperatorFacet{}
	typeSet := map[string]struct{}{}
	for _, source := range sources {
		operatorID := strings.TrimSpace(source.ActorAccountID)
		if operatorID == "" {
			operatorID = WorkspaceAuditSystemOperatorID
		}
		account := accountByID[source.ActorAccountID]
		employee := employeeByID[account.EmployeeID]
		operatorByID[operatorID] = WorkspaceAuditLogOperatorFacet{
			ID: operatorID,
			Label: workspaceAuditOperator(AuditLog{
				ActorAccountID: source.ActorAccountID,
			}, account, employee),
		}
		typeSet[workspaceAuditType(AuditLog{Action: source.Action, Resource: source.Resource})] = struct{}{}
	}
	operators := make([]WorkspaceAuditLogOperatorFacet, 0, len(operatorByID))
	for _, operator := range operatorByID {
		operators = append(operators, operator)
	}
	sort.Slice(operators, func(i, j int) bool {
		if operators[i].Label != operators[j].Label {
			return operators[i].Label < operators[j].Label
		}
		return operators[i].ID < operators[j].ID
	})
	typeCatalog := []string{"員工管理", "組織架構", "假勤制度", "表單設計", "管理員設定", "系統"}
	types := make([]string, 0, len(typeSet))
	for _, auditType := range typeCatalog {
		if _, ok := typeSet[auditType]; ok {
			types = append(types, auditType)
		}
	}
	return WorkspaceAuditLogFacets{Operators: operators, Types: types}, nil
}

// visibleWorkspaceEmployees 處理可見工作區員工的服務流程。
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

// visibleWorkspaceAttendanceEmployees projects the roster needed by the
// attendance matrix under the caller's attendance scope. Requiring a separate
// HR grant here would contradict the workspace route and endpoint contract.
func (c WorkspaceService) visibleWorkspaceAttendanceEmployees(ctx RequestContext, event string) ([]Employee, error) {
	account, decision, audit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppAttendance, ResourceType: ResourceAttendanceClock, Action: ActionRead},
		AuditTarget{Event: event, Resource: string(ResourceAttendanceClock)},
	)
	if err != nil {
		return nil, err
	}
	items, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	allowed, all, err := c.Service.Attendance().attendanceEmployeeScope(ctx, account, decision)
	if err != nil {
		return nil, err
	}
	if !all {
		filtered := make([]Employee, 0, len(items))
		for _, item := range items {
			if _, ok := allowed[item.ID]; ok {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if err := audit.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

// visibleWorkspaceAttendanceEmployeePage pushes authorization scope, month
// presence, search, department, ordering, and pagination into the employee
// store before any monthly attendance facts are read.
func (c WorkspaceService) visibleWorkspaceAttendanceEmployeePage(ctx RequestContext, query WorkspaceAttendanceQuery, start, end time.Time, event string) ([]Employee, int, error) {
	account, decision, audit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppAttendance, ResourceType: ResourceAttendanceClock, Action: ActionRead},
		AuditTarget{Event: event, Resource: string(ResourceAttendanceClock)},
	)
	if err != nil {
		return nil, 0, err
	}
	scope, err := c.workspaceAttendanceEmployeeScopeConstraint(ctx, account, decision)
	if err != nil {
		return nil, 0, err
	}
	employeeQuery := EmployeeQuery{
		Keyword:      query.Keyword,
		DepartmentID: query.DepartmentID,
		PresentFrom:  start.Format(time.RFC3339),
		PresentTo:    end.Format(time.RFC3339),
		Sort:         "attendance_asc",
		Scope:        scope,
	}
	var employees []Employee
	var total int
	if query.Paginated && !query.ForceAll {
		employeeQuery.Page = query.Page
		employeeQuery.PageSize = query.PageSize
		employees, total, err = c.store.ListEmployeePageByQuery(goContext(ctx), ctx.TenantID, employeeQuery)
	} else {
		employees, err = c.store.ListEmployeesByQuery(goContext(ctx), ctx.TenantID, employeeQuery)
		total = len(employees)
	}
	if err != nil {
		return nil, 0, err
	}
	if err := audit.Commit(ctx); err != nil {
		return nil, 0, err
	}
	return employees, total, nil
}

func (c WorkspaceService) workspaceAttendanceEmployeeScopeConstraint(ctx RequestContext, account Account, decision CheckResult) (EmployeeScopeConstraint, error) {
	if decision.Scope == "" || decision.Scope == ScopeAll || decision.Scope == ScopeTenant || decision.Scope == ScopeSystem {
		return EmployeeScopeConstraint{}, nil
	}
	employeeIDs := uniqueStrings(stringSliceFromAny(decision.Conditions["employee_ids"]))
	if len(employeeIDs) == 0 && (decision.Scope == ScopeSelf || decision.Scope == ScopeOwn) && account.EmployeeID != "" {
		employeeIDs = []string{account.EmployeeID}
	}
	orgUnitIDs := uniqueStrings(stringSliceFromAny(decision.Conditions["org_unit_ids"]))
	if len(orgUnitIDs) > 0 {
		units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
		if err != nil {
			return EmployeeScopeConstraint{}, err
		}
		orgUnitIDs = orgUnitIDsInSubtree(units, orgUnitIDs)
	}
	statuses := uniqueStrings(stringSliceFromAny(decision.Conditions["employee_statuses"]))
	if len(statuses) == 0 {
		statuses = uniqueStrings(stringSliceFromAny(decision.Conditions["statuses"]))
	}
	if len(employeeIDs) == 0 && len(orgUnitIDs) == 0 && len(statuses) == 0 {
		return EmployeeScopeConstraint{DenyAll: true}, nil
	}
	return EmployeeScopeConstraint{
		EmployeeIDs:    employeeIDs,
		OrgUnitIDs:     orgUnitIDs,
		Statuses:       statuses,
		MatchAnyEntity: len(employeeIDs) > 0 && len(orgUnitIDs) > 0,
	}, nil
}

func (c WorkspaceService) workspaceAuthorizedAttendanceEmployeeIDs(ctx RequestContext, resource ResourceType, employees []Employee) ([]string, error) {
	account, decision, err := c.Service.Attendance().requireAttendanceAuthz(ctx, resource, ActionRead, "")
	if err != nil {
		return nil, err
	}
	scope, err := c.workspaceAttendanceEmployeeScopeConstraint(ctx, account, decision)
	if err != nil {
		return nil, err
	}
	if scope.DenyAll {
		return []string{}, nil
	}
	employeeSet := stringSet(scope.EmployeeIDs)
	orgSet := stringSet(scope.OrgUnitIDs)
	statusSet := stringSet(scope.Statuses)
	ids := make([]string, 0, len(employees))
	for _, employee := range employees {
		employeeMatch := len(employeeSet) > 0
		if employeeMatch {
			_, employeeMatch = employeeSet[employee.ID]
		}
		orgMatch := len(orgSet) > 0
		if orgMatch {
			_, orgMatch = orgSet[employee.OrgUnitID]
		}
		if scope.MatchAnyEntity {
			if !employeeMatch && !orgMatch {
				continue
			}
		} else if (len(employeeSet) > 0 && !employeeMatch) || (len(orgSet) > 0 && !orgMatch) {
			continue
		}
		if len(statusSet) > 0 {
			if _, ok := statusSet[workspaceEmployeeStatus(employee)]; !ok {
				continue
			}
		}
		ids = append(ids, employee.ID)
	}
	return ids, nil
}

// visibleWorkspaceClockRecords 處理可見工作區打卡 records 的服務流程。
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

// visibleWorkspaceDailySummaries 處理可見工作區日彙總。
func (c WorkspaceService) visibleWorkspaceDailySummaries(ctx RequestContext, query AttendanceDailySummaryQuery) ([]AttendanceDailySummary, error) {
	attendance := c.Service.Attendance()
	account, decision, err := attendance.requireAttendanceAuthz(ctx, ResourceAttendanceClock, ActionRead, "")
	if err != nil {
		return nil, err
	}
	query = normalizeAttendanceDailySummaryQuery(query)
	items, err := c.store.ListAttendanceDailySummaries(goContext(ctx), ctx.TenantID, query)
	if err != nil {
		return nil, err
	}
	allowed, all, err := attendance.attendanceEmployeeScope(ctx, account, decision)
	if err != nil {
		return nil, err
	}
	if all {
		return items, nil
	}
	out := make([]AttendanceDailySummary, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.EmployeeID]; ok {
			out = append(out, item)
		}
	}
	return out, nil
}
