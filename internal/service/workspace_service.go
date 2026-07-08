package service

import (
	"fmt"
	"nexus-pro-be/internal/repository"
	"nexus-pro-be/internal/utils"
	"sort"
	"time"
)

const (
	workspaceParentNone = "__none__"
	workspaceDayHours   = 8.0
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
	leaves, err := c.Service.Attendance().listLeaveRequestsByQuery(ctx, LeaveRequestQuery{
		FromDate: start.Format(time.DateOnly),
		ToDate:   end.AddDate(0, 0, -1).Format(time.DateOnly),
	})
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

// WorkspaceOrganization 處理工作區 organization 的服務流程。
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
	leaves, err := c.Service.Attendance().listLeaveRequestsByQuery(ctx, LeaveRequestQuery{
		FromDate: start.Format(time.DateOnly),
		ToDate:   end.AddDate(0, 0, -1).Format(time.DateOnly),
	})
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
	overtimes, err := c.Service.Attendance().listOvertimeRequestsByQuery(ctx, OvertimeRequestQuery{
		Status:   "approved",
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
	overtimeByEmployeeDate := workspaceOvertimeCells(overtimes, start, end)
	clockByEmployeeDate := workspaceClockCells(clocks, worksites, leaveByEmployeeDate, overtimeByEmployeeDate)
	attendanceMatrix := workspaceAttendanceMatrix(monthEmployees, cards, dates, leaveByEmployeeDate, overtimeByEmployeeDate)
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

// WorkspaceAuditLogs 處理工作區稽核 logs 的服務流程。
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
	logs, err := c.store.ListAuditLogs(goContext(ctx), ctx.TenantID)
	if err != nil {
		return PageResponse[WorkspaceAuditLog]{}, err
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
