package domain

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

// WorkspaceOrganizationResponse 定義工作區 organization 回應的資料結構。
type WorkspaceOrganizationResponse struct {
	ParentNone string                     `json:"parent_none"`
	Rows       []WorkspaceOrganizationRow `json:"rows"`
}

// WorkspaceOrganizationRow 定義工作區 organization 列的資料結構。
type WorkspaceOrganizationRow struct {
	ID             string `json:"id"`
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
	IsOverride     bool   `json:"is_override,omitempty"`
	ManagerIssue   string `json:"manager_issue,omitempty"`
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
// 口徑說明：統計範圍排除隸屬已關閉組織單元或已停用崗位的員工，
// 因此在職數會小於概覽頁（全員月末快照）。離職率 = 當月離職 ÷ 當月平均在職
// （月初與月末快照平均）；每列均滿足 上月在職 + 新進 − 離職 − 資遣 = 本月在職。
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
// 口徑說明：年離職率 = 年度離職 ÷ 年度平均在職（年初與年末快照平均）；
// 年淨增減 = 年新進 − 年離職 − 年資遣，且恆等於 年末在職 − 年初在職
// （每列均滿足 年初在職 + 新進 − 離職 − 資遣 = 年末在職 的閉合恆等式）。
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
	Year  int `json:"year,omitempty"`
	Month int `json:"month,omitempty"`
}

// WorkspaceAttendanceResponse 定義工作區考勤回應的資料結構。
type WorkspaceAttendanceResponse struct {
	Year        int                        `json:"year"`
	Month       int                        `json:"month"`
	IsFuture    bool                       `json:"is_future"`
	Label       string                     `json:"label"`
	PeriodLabel string                     `json:"period_label"`
	Dates       []WorkspaceDate            `json:"dates"`
	LeaveLegend []WorkspaceLeaveLegendItem `json:"leave_legend"`
	Attendance  WorkspaceAttendanceMatrix  `json:"attendance"`
	Clock       WorkspaceClockMatrix       `json:"clock"`
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
	ID         string `json:"id"`
	EmployeeID string `json:"employee_id"`
	Avatar     string `json:"avatar"`
	NameZH     string `json:"name_zh"`
	NameEN     string `json:"name_en"`
	Email      string `json:"email"`
	Dept       string `json:"dept"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	Phone      string `json:"phone"`
	Status     string `json:"status"`
	HireDate   string `json:"hire_date"`
}

// WorkspaceDayCell 定義工作區 day 儲存格的資料結構。
type WorkspaceDayCell struct {
	Type     string  `json:"type"`
	Holiday  string  `json:"holiday,omitempty"`
	Leave    string  `json:"leave,omitempty"`
	Hours    float64 `json:"hours,omitempty"`
	Overtime float64 `json:"overtime,omitempty"`
	Label    string  `json:"label,omitempty"`
	In       string  `json:"in,omitempty"`
	Out      string  `json:"out,omitempty"`
	InLoc    string  `json:"in_loc,omitempty"`
	OutLoc   string  `json:"out_loc,omitempty"`
	Recorded bool    `json:"recorded,omitempty"`
	Abnormal bool    `json:"abnormal,omitempty"`
	Reason   string  `json:"reason,omitempty"`
}

// WorkspaceEmployeeHours 定義工作區員工小時的資料結構。
type WorkspaceEmployeeHours struct {
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
