package domain

import "encoding/json"

// WorkspaceOverviewQuery 定義工作區總覽查詢的資料結構。
type WorkspaceOverviewQuery struct {
	Year  int    `json:"year,omitempty"`
	Month int    `json:"month,omitempty"`
	Date  string `json:"date,omitempty"`
}

// WorkspaceOverviewResponse 定義工作區總覽回應的資料結構。
type WorkspaceOverviewResponse struct {
	Month          string                      `json:"month"`
	Year           int                         `json:"year"`
	MonthNumber    int                         `json:"month_number"`
	HRSummary      WorkspaceHRSummary          `json:"hr_summary"`
	Attendance     WorkspaceOverviewAttendance `json:"attendance"`
	TodoCategories []WorkspaceTodoCategory     `json:"todo_categories"`
}

// WorkspaceHRSummary 定義工作區 HR 摘要的資料結構。
// 口徑說明（與人員異動頁一致的「時點快照」口徑）：
//   - Active：當月末時點在職快照，包含 active/probation/onboarding(已到職)/leave_suspended，
//     排除已離職與已刪除；因此會略大於員工頁僅統計 status=active 的在職數。
//   - Hires：當月 hire_date 落入區間的新進人數。
//   - Separations：當月有效離職時間（resign_date 優先）落入區間的離職人數。
//   - SeparationRate：Separations ÷ 當月平均在職（月初與月末快照平均）× 100%。
type WorkspaceHRSummary struct {
	Title          string `json:"title"`
	Active         int    `json:"active"`
	Hires          int    `json:"hires"`
	Separations    int    `json:"separations"`
	SeparationRate string `json:"separation_rate"`
}

// WorkspaceOverviewAttendance 定義工作區總覽考勤的資料結構。
type WorkspaceOverviewAttendance struct {
	CheckedIn  int                        `json:"checked_in"`
	Leave      int                        `json:"leave"`
	Absent     int                        `json:"absent"`
	Segments   []WorkspaceAttendanceSlice `json:"segments"`
	DailyLeave []WorkspaceDailyLeave      `json:"daily_leave"`
}

// WorkspaceAttendanceSlice 定義工作區考勤 slice 的資料結構。
type WorkspaceAttendanceSlice struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Percent int    `json:"percent"`
	Tone    string `json:"tone"`
}

// WorkspaceDailyLeave 定義工作區每日請假的資料結構。
type WorkspaceDailyLeave struct {
	Day           int    `json:"day"`
	Value         int    `json:"value"`
	HeightPercent int    `json:"height_percent"`
	ShowLabel     bool   `json:"show_label"`
	Active        bool   `json:"active"`
	Tooltip       string `json:"tooltip"`
}

// WorkspaceTodoCategory 定義工作區待辦分類的資料結構。
type WorkspaceTodoCategory struct {
	Key       string                `json:"key"`
	Label     string                `json:"label"`
	Icon      string                `json:"icon"`
	Desc      string                `json:"desc"`
	DateLabel string                `json:"date_label"`
	People    []WorkspaceTodoPerson `json:"people"`
	Count     int                   `json:"count"`
}

// WorkspaceTodoPerson 定義工作區待辦 person 的資料結構。
type WorkspaceTodoPerson struct {
	ID     string `json:"id"`
	NameZH string `json:"name_zh"`
	NameEN string `json:"name_en"`
	Date   string `json:"date"`
}

// WorkspaceOrgUnitDirectoryResponse 定義組織單位管理頁的讀取投影。
// 員工資料依權限選擇性包含，避免組織單位唯讀角色被迫取得員工名冊權限。
type WorkspaceOrgUnitDirectoryResponse struct {
	Rows                []WorkspaceOrgUnitDirectoryRow      `json:"rows"`
	UnassignedEmployees []WorkspaceOrgUnitDirectoryEmployee `json:"unassigned_employees"`
	EmployeesIncluded   bool                                `json:"employees_included"`
}

// WorkspaceOrgUnitDirectoryRow 將一個組織單位與其直屬員工投影在同一列。
type WorkspaceOrgUnitDirectoryRow struct {
	OrgUnit         OrgUnit                             `json:"org_unit"`
	DirectEmployees []WorkspaceOrgUnitDirectoryEmployee `json:"direct_employees"`
}

// WorkspaceOrgUnitDirectoryEmployee 是組織單位管理頁所需的最小員工摘要。
type WorkspaceOrgUnitDirectoryEmployee struct {
	ID           string `json:"id"`
	EmployeeNo   string `json:"employee_no,omitempty"`
	Name         string `json:"name"`
	CompanyEmail string `json:"company_email,omitempty"`
	OrgUnitID    string `json:"org_unit_id,omitempty"`
	Position     string `json:"position,omitempty"`
}

// WorkspaceOrganizationResponse 定義工作區 organization 回應的資料結構。
type WorkspaceOrganizationResponse struct {
	ParentNone string                     `json:"parent_none"`
	OrgUnits   []OrgUnit                  `json:"org_units"`
	Rows       []WorkspaceOrganizationRow `json:"rows"`
}

// WorkspaceOrganizationRow 定義工作區 organization 列的資料結構。
type WorkspaceOrganizationRow struct {
	ID             string `json:"id"`
	EmployeeID     string `json:"employee_id,omitempty"`
	NameZH         string `json:"name_zh"`
	NameEN         string `json:"name_en"`
	Dept           string `json:"dept"`
	Title          string `json:"title"`
	Level          int    `json:"level"`
	IsManager      bool   `json:"is_manager"`
	ShowInOrgChart bool   `json:"show_in_org_chart"`
	ParentID       string `json:"parent_id"`
	OrgUnitID      string `json:"org_unit_id,omitempty"`
	ManagerSource  string `json:"manager_source,omitempty"`
	ManagerIssue   string `json:"manager_issue,omitempty"`
	IsOverride     bool   `json:"is_override,omitempty"`
}

// WorkspaceTurnoverQuery 定義工作區人員異動查詢的資料結構。
type WorkspaceTurnoverQuery struct {
	Year       int `json:"year,omitempty"`
	Month      int `json:"month,omitempty"`
	AnnualYear int `json:"annual_year,omitempty"`
}

// WorkspaceTurnoverResponse 定義工作區人員異動回應的資料結構。
type WorkspaceTurnoverResponse struct {
	Monthly WorkspaceTurnoverMonthly `json:"monthly"`
	Annual  WorkspaceTurnoverAnnual  `json:"annual"`
}

// WorkspaceTurnoverMonthly 定義工作區人員異動每月的資料結構。
// 口徑說明：組織或崗位停用不等於員工離職，統計範圍仍包含其中的員工。
// 待加入與留停不計入期末在職；期末日離職者保留在本期期末，於次期扣除。
// 當月離職率 = 非資遣離職 ÷ ((上月期末 + 當月期末) / 2)；YTD 為各月離職率累加。
type WorkspaceTurnoverMonthly struct {
	Year           int                       `json:"year"`
	Month          int                       `json:"month"`
	IsFuture       bool                      `json:"is_future"`
	Title          string                    `json:"title"`
	Stats          []WorkspaceKPI            `json:"stats"`
	HireComparison []WorkspaceComparisonItem `json:"hire_comparison"`
	RateComparison []WorkspaceComparisonItem `json:"rate_comparison"`
	Rows           []WorkspaceTurnoverRow    `json:"rows"`
	CSVHeaders     []string                  `json:"csv_headers"`
}

// WorkspaceTurnoverAnnual 定義工作區人員異動年度的資料結構。
// 口徑說明：年度離職率為各月非資遣離職率累加；資遣仍單列並影響期末人數，
// 但不納入離職總數與離職率。
type WorkspaceTurnoverAnnual struct {
	Year               int                       `json:"year"`
	IsFuture           bool                      `json:"is_future"`
	Title              string                    `json:"title"`
	KPIs               []WorkspaceKPI            `json:"kpis"`
	HeadcountTrend     []WorkspaceTrendPoint     `json:"headcount_trend"`
	RateTrend          []WorkspaceTrendPoint     `json:"rate_trend"`
	Pie                []WorkspacePieItem        `json:"pie"`
	DeptRateComparison []WorkspaceComparisonItem `json:"dept_rate_comparison"`
	Rows               []WorkspaceAnnualRow      `json:"rows"`
	CSVHeaders         []string                  `json:"csv_headers"`
}

// WorkspaceKPI 定義工作區 KPI 的資料結構。
type WorkspaceKPI struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Value     string `json:"value"`
	Unit      string `json:"unit"`
	TrendText string `json:"trend_text"`
	TrendTone string `json:"trend_tone"`
}

// WorkspaceComparisonItem 定義工作區 comparison 項目的資料結構。
type WorkspaceComparisonItem struct {
	Name    string  `json:"name"`
	Value   float64 `json:"value"`
	Label   string  `json:"label"`
	Percent int     `json:"percent"`
}

// WorkspaceTurnoverRow 定義工作區人員異動列的資料結構。
type WorkspaceTurnoverRow struct {
	Key       string `json:"key"`
	RowType   string `json:"row_type"`
	BU        string `json:"bu"`
	Dept      string `json:"dept"`
	BURowSpan int    `json:"bu_row_span"`
	Prev      int    `json:"prev"`
	Hires     int    `json:"hires"`
	Resigned  int    `json:"resigned"`
	Layoff    int    `json:"layoff"`
	OnLeave   int    `json:"onleave"`
	End       int    `json:"end"`
	MonthRate string `json:"month_rate"`
	YTDRate   string `json:"ytd_rate"`
}

// WorkspaceTrendPoint 定義工作區趨勢 point 的資料結構。
type WorkspaceTrendPoint struct {
	Month   int     `json:"month"`
	Value   float64 `json:"value"`
	Label   string  `json:"label"`
	Percent int     `json:"percent"`
	Future  bool    `json:"future"`
	Tone    string  `json:"tone"`
}

// WorkspacePieItem 定義工作區 pie 項目的資料結構。
type WorkspacePieItem struct {
	Name       string  `json:"name"`
	Value      int     `json:"value"`
	Percent    int     `json:"percent"`
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	ColorIndex int     `json:"color_index"`
}

// WorkspaceAnnualRow 定義工作區年度列的資料結構。
type WorkspaceAnnualRow struct {
	BU       string `json:"bu"`
	Base     int    `json:"base"`
	Hires    int    `json:"hires"`
	Resigned int    `json:"resigned"`
	Layoff   int    `json:"layoff"`
	OnLeave  int    `json:"onleave"`
	End      int    `json:"end"`
	Sep      int    `json:"sep"`
	Rate     string `json:"rate"`
}

// WorkspaceAttendanceQuery 定義工作區考勤查詢的資料結構。
type WorkspaceAttendanceQuery struct {
	Year             int    `json:"year,omitempty"`
	Month            int    `json:"month,omitempty"`
	Projection       string `json:"projection,omitempty"`
	DepartmentID     string `json:"department_id,omitempty"`
	Keyword          string `json:"keyword,omitempty"`
	Page             int    `json:"page,omitempty"`
	PageSize         int    `json:"page_size,omitempty"`
	Paginated        bool   `json:"-"`
	ForceAll         bool   `json:"-"`
	IncludeAbnormals bool   `json:"-"`
}

// WorkspaceClockAbnormalQuery paginates abnormal records within one bounded
// employee page so the endpoint never scans an entire large tenant.
type WorkspaceClockAbnormalQuery struct {
	Year             int    `json:"year,omitempty"`
	Month            int    `json:"month,omitempty"`
	BaseDepartmentID string `json:"base_department_id,omitempty"`
	DepartmentID     string `json:"department_id,omitempty"`
	Keyword          string `json:"keyword,omitempty"`
	Severity         string `json:"severity,omitempty"`
	Page             int    `json:"page,omitempty"`
	PageSize         int    `json:"page_size,omitempty"`
	EmployeePage     int    `json:"employee_page,omitempty"`
	EmployeePageSize int    `json:"employee_page_size,omitempty"`
}

// WorkspaceAttendanceResponse 定義工作區考勤回應的資料結構。
type WorkspaceAttendanceResponse struct {
	Year         int                            `json:"year"`
	Month        int                            `json:"month"`
	IsFuture     bool                           `json:"is_future"`
	Label        string                         `json:"label"`
	PeriodLabel  string                         `json:"period_label"`
	Dates        []WorkspaceDate                `json:"dates"`
	LeaveLegend  []WorkspaceLeaveLegendItem     `json:"leave_legend"`
	Pagination   *WorkspaceAttendancePagination `json:"pagination,omitempty"`
	SummaryScope string                         `json:"summary_scope,omitempty"`
	Attendance   WorkspaceAttendanceMatrix      `json:"-"`
	Clock        WorkspaceClockMatrix           `json:"-"`
	Projection   string                         `json:"-"`
}

// MarshalJSON keeps the legacy response complete while allowing projected
// requests to omit the unused matrix entirely.
func (r WorkspaceAttendanceResponse) MarshalJSON() ([]byte, error) {
	type response struct {
		Year         int                            `json:"year"`
		Month        int                            `json:"month"`
		IsFuture     bool                           `json:"is_future"`
		Label        string                         `json:"label"`
		PeriodLabel  string                         `json:"period_label"`
		Dates        []WorkspaceDate                `json:"dates"`
		LeaveLegend  []WorkspaceLeaveLegendItem     `json:"leave_legend"`
		Pagination   *WorkspaceAttendancePagination `json:"pagination,omitempty"`
		SummaryScope string                         `json:"summary_scope,omitempty"`
		Attendance   *WorkspaceAttendanceMatrix     `json:"attendance,omitempty"`
		Clock        any                            `json:"clock,omitempty"`
	}
	payload := response{
		Year: r.Year, Month: r.Month, IsFuture: r.IsFuture, Label: r.Label,
		PeriodLabel: r.PeriodLabel, Dates: r.Dates, LeaveLegend: r.LeaveLegend,
		Pagination: r.Pagination, SummaryScope: r.SummaryScope,
	}
	switch r.Projection {
	case "attendance":
		payload.Attendance = &r.Attendance
	case "clock":
		payload.Clock = struct {
			Rows    []WorkspaceClockRow   `json:"rows"`
			Summary WorkspaceClockSummary `json:"summary"`
		}{Rows: r.Clock.Rows, Summary: r.Clock.Summary}
	default:
		payload.Attendance = &r.Attendance
		payload.Clock = &r.Clock
	}
	return json.Marshal(payload)
}

// WorkspaceAttendancePagination describes the employee-row page represented
// by a projected matrix.
type WorkspaceAttendancePagination struct {
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// WorkspaceDate 定義工作區日期的資料結構。
type WorkspaceDate struct {
	Key     string  `json:"key"`
	Y       int     `json:"y"`
	M       int     `json:"m"`
	D       int     `json:"d"`
	DOW     int     `json:"dow"`
	Holiday *string `json:"holiday"`
}

// WorkspaceLeaveLegendItem 定義工作區請假 legend 項目的資料結構。
type WorkspaceLeaveLegendItem struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// WorkspaceAttendanceMatrix 定義工作區考勤矩陣的資料結構。
type WorkspaceAttendanceMatrix struct {
	Rows    []WorkspaceAttendanceRow     `json:"rows"`
	Summary WorkspaceAttendanceMatrixSum `json:"summary"`
}

// WorkspaceAttendanceRow 定義工作區考勤列的資料結構。
type WorkspaceAttendanceRow struct {
	Employee WorkspaceEmployeeCard  `json:"employee"`
	Cells    []WorkspaceDayCell     `json:"cells"`
	Summary  WorkspaceEmployeeHours `json:"summary"`
}

// WorkspaceEmployeeCard 定義工作區員工 card 的資料結構。
type WorkspaceEmployeeCard struct {
	ID           string `json:"id"`
	EmployeeID   string `json:"employee_id"`
	Avatar       string `json:"avatar"`
	NameZH       string `json:"name_zh"`
	NameEN       string `json:"name_en"`
	Email        string `json:"email"`
	DepartmentID string `json:"department_id"`
	Dept         string `json:"dept"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	Phone        string `json:"phone"`
	Status       string `json:"status"`
	HireDate     string `json:"hire_date"`
}

// WorkspaceDayCell 定義工作區 day 儲存格的資料結構。
type WorkspaceDayCell struct {
	Type         string  `json:"type"`
	Holiday      string  `json:"holiday,omitempty"`
	Leave        string  `json:"leave,omitempty"`
	Hours        float64 `json:"hours,omitempty"`
	ActualHours  float64 `json:"actual_hours,omitempty"`
	MaxHours     float64 `json:"max_hours,omitempty"`
	CountedHours float64 `json:"counted_hours,omitempty"`
	Overtime     float64 `json:"overtime,omitempty"`
	Label        string  `json:"label,omitempty"`
	In           string  `json:"in,omitempty"`
	Out          string  `json:"out,omitempty"`
	InLoc        string  `json:"in_loc,omitempty"`
	OutLoc       string  `json:"out_loc,omitempty"`
	Recorded     bool    `json:"recorded,omitempty"`
	Abnormal     bool    `json:"abnormal,omitempty"`
	Reason       string  `json:"reason,omitempty"`
}

// WorkspaceEmployeeHours 定義工作區員工小時的資料結構。
type WorkspaceEmployeeHours struct {
	ActualHours   float64            `json:"actual_hours"`
	AttendedHours float64            `json:"attended_hours"`
	Birthday      bool               `json:"birthday"`
	DeductHours   float64            `json:"deduct_hours"`
	DueHours      float64            `json:"due_hours"`
	LeaveByType   map[string]float64 `json:"leave_by_type"`
	LeaveHours    float64            `json:"leave_hours"`
	OvertimeHours float64            `json:"overtime_hours"`
	WorkDays      int                `json:"work_days"`
}

// WorkspaceAttendanceMatrixSum 定義工作區考勤矩陣總和的資料結構。
type WorkspaceAttendanceMatrixSum struct {
	Holidays      int     `json:"holidays"`
	LeaveHours    float64 `json:"leave_hours"`
	OvertimeHours float64 `json:"overtime_hours"`
	Perfect       int     `json:"perfect"`
	Workdays      int     `json:"workdays"`
}

// WorkspaceClockMatrix 定義工作區打卡矩陣的資料結構。
type WorkspaceClockMatrix struct {
	Abnormals []WorkspaceClockAbnormal `json:"abnormals"`
	Rows      []WorkspaceClockRow      `json:"rows"`
	Summary   WorkspaceClockSummary    `json:"summary"`
}

// WorkspaceClockAbnormal 定義工作區打卡 abnormal 的資料結構。
type WorkspaceClockAbnormal struct {
	Date     WorkspaceDate         `json:"date"`
	Employee WorkspaceEmployeeCard `json:"employee"`
	Record   WorkspaceDayCell      `json:"record"`
}

// WorkspaceClockRow 定義工作區打卡列的資料結構。
type WorkspaceClockRow struct {
	Employee WorkspaceEmployeeCard `json:"employee"`
	Cells    []WorkspaceDayCell    `json:"cells"`
}

// WorkspaceClockSummary 定義工作區打卡摘要的資料結構。
type WorkspaceClockSummary struct {
	AbnormalDays   int `json:"abnormal_days"`
	AbnormalPeople int `json:"abnormal_people"`
	NormalDays     int `json:"normal_days"`
}

// WorkspaceClockAbnormalResponse is the on-demand abnormal-record list. Its
// pagination applies to abnormal records, not employee rows.
type WorkspaceClockAbnormalResponse struct {
	Items              []WorkspaceClockAbnormal      `json:"items"`
	Pagination         WorkspaceAttendancePagination `json:"pagination"`
	SummaryScope       string                        `json:"summary_scope"`
	EmployeePagination WorkspaceAttendancePagination `json:"employee_pagination"`
}

// WorkspaceAuditLogQuery 定義工作區稽覈 log 查詢的資料結構。
type WorkspaceAuditLogQuery struct {
	OperatorID string `json:"operator_id,omitempty"`
	Type       string `json:"type,omitempty"`
	From       string `json:"from,omitempty"`
	To         string `json:"to,omitempty"`
	Keyword    string `json:"keyword,omitempty"`
}

// WorkspaceAuditLog 定義工作區稽覈 log 的資料結構。
type WorkspaceAuditLog struct {
	ID       string `json:"id"`
	Time     string `json:"time"`
	Operator string `json:"operator"`
	Type     string `json:"type"`
	Action   string `json:"action"`
	Detail   string `json:"detail"`
}

const WorkspaceAuditSystemOperatorID = "__system__"

// WorkspaceAuditLogFacetSource contains only non-sensitive fields needed to build tenant-wide facets.
type WorkspaceAuditLogFacetSource struct {
	ActorAccountID string `json:"actor_account_id"`
	Action         string `json:"action"`
	Resource       string `json:"resource"`
}

// WorkspaceAuditLogOperatorFacet exposes a stable filter ID with its current display label.
type WorkspaceAuditLogOperatorFacet struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// WorkspaceAuditLogFacets contains tenant-wide operator and audit-type filter options.
type WorkspaceAuditLogFacets struct {
	Operators []WorkspaceAuditLogOperatorFacet `json:"operators"`
	Types     []string                         `json:"types"`
}
