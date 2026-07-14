package service

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"nexus-pro-be/internal/repository"
	"nexus-pro-be/internal/utils"
	"sort"
	"strconv"
	"strings"
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
	employees = workspaceOrganizationEmployees(employees)
	units, err := c.Service.HR().ListOrgUnits(ctx)
	if err != nil {
		return WorkspaceOrganizationResponse{}, err
	}
	positions, err := c.store.ListPositions(goContext(ctx), ctx.TenantID)
	if err != nil {
		return WorkspaceOrganizationResponse{}, err
	}
	orgNames := workspaceOrgNames(units)
	displayIDs := workspaceEmployeeDisplayIDs(employees)
	byID := map[string]Employee{}
	for _, employee := range employees {
		byID[employee.ID] = employee
	}
	derivedManagers, orgUnitsByID := workspaceDerivedManagersByOrgUnit(units, positions, employees)
	effectiveParents := map[string]string{}
	managerIDs := map[string]struct{}{}
	for _, employee := range employees {
		parentID, source, _ := workspaceEffectiveManager(employee, derivedManagers, orgUnitsByID)
		effectiveParents[employee.ID] = parentID
		if parentID != "" {
			managerIDs[parentID] = struct{}{}
		}
		_ = source
	}
	rows := make([]WorkspaceOrganizationRow, 0, len(employees))
	levelMemo := map[string]int{}
	for _, employee := range employees {
		displayID := displayIDs[employee.ID]
		managerID, source, managerIssue := workspaceEffectiveManager(employee, derivedManagers, orgUnitsByID)
		parentID := workspaceParentNone
		if managerID != "" {
			if managerDisplayID, ok := displayIDs[managerID]; ok {
				parentID = managerDisplayID
			}
		}
		_, isManager := managerIDs[employee.ID]
		rows = append(rows, WorkspaceOrganizationRow{
			ID:            displayID,
			NameZH:        employee.Name,
			NameEN:        workspaceEmployeeNameEN(employee),
			Dept:          workspaceOrgName(orgNames, employee.OrgUnitID),
			Title:         employee.Position,
			Level:         workspaceEffectiveEmployeeLevel(employee.ID, effectiveParents, byID, levelMemo),
			IsManager:     isManager,
			ParentID:      parentID,
			OrgUnitID:     employee.OrgUnitID,
			ManagerSource: source,
			IsOverride:    source == "override",
			ManagerIssue:  managerIssue,
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
	positions, err := c.store.ListPositions(goContext(ctx), ctx.TenantID)
	if err != nil {
		return WorkspaceTurnoverResponse{}, err
	}
	employees = workspaceTurnoverEligibleEmployees(employees, units, positions)
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
	clocks, err := c.visibleWorkspaceClockRecords(ctx, AttendanceClockRecordQuery{
		FromDate: start.Format(time.DateOnly),
		ToDate:   end.AddDate(0, 0, -1).Format(time.DateOnly),
	})
	if err != nil {
		return WorkspaceAttendanceResponse{}, err
	}
	summaries, err := c.visibleWorkspaceDailySummaries(ctx, AttendanceDailySummaryQuery{
		FromDate: start.Format(time.DateOnly),
		ToDate:   end.AddDate(0, 0, -1).Format(time.DateOnly),
		Source:   "ehrms",
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
	leaves, err := c.Service.Attendance().listLeaveRequestsByQuery(ctx, LeaveRequestQuery{
		Status:   "approved",
		FromDate: start.Format(time.DateOnly),
		ToDate:   end.AddDate(0, 0, -1).Format(time.DateOnly),
	})
	if err != nil {
		return WorkspaceAttendanceResponse{}, err
	}
	policy, err := c.Service.Attendance().loadAttendancePolicyResponse(ctx)
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
	leaveByEmployeeDate := workspaceSummaryLeaveCells(summaries)
	leaveByEmployeeDate = workspaceMergeApprovedLeaveCells(leaveByEmployeeDate, leaves, policy.WorkTime, start, end)
	overtimeByEmployeeDate := workspaceOvertimeCells(overtimes, start, end)
	summaryByEmployeeDate := workspaceSummaryCells(summaries)
	clockByEmployeeDate := workspaceClockCells(clocks, summaries, worksites, leaveByEmployeeDate, overtimeByEmployeeDate)
	attendanceMatrix := workspaceAttendanceMatrix(monthEmployees, cards, dates, leaveByEmployeeDate, overtimeByEmployeeDate, summaryByEmployeeDate)
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

// ExportWorkspaceAttendanceCSV 匯出工作區考勤 CSV。
func (c WorkspaceService) ExportWorkspaceAttendanceCSV(ctx RequestContext, query WorkspaceAttendanceQuery, kind string) ([]byte, string, error) {
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
	header := []string{"員工編號", "部門", "姓名", "英文名", "應出勤天數", "應出勤時數", "實出勤時數", "請假時數", "加班時數", "扣除時數"}
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
