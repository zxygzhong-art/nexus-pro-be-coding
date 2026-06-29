package domain

// WorkspaceOverviewQuery selects the month and date used by workspace overview widgets.
type WorkspaceOverviewQuery struct {
	Year  int    `json:"year,omitempty"`
	Month int    `json:"month,omitempty"`
	Date  string `json:"date,omitempty"`
}

// WorkspaceOverviewResponse aggregates HR, attendance, and lifecycle task widgets.
type WorkspaceOverviewResponse struct {
	Month          string                      `json:"month"`
	Year           int                         `json:"year"`
	MonthNumber    int                         `json:"month_number"`
	HRSummary      WorkspaceHRSummary          `json:"hr_summary"`
	Attendance     WorkspaceOverviewAttendance `json:"attendance"`
	TodoCategories []WorkspaceTodoCategory     `json:"todo_categories"`
}

// WorkspaceHRSummary contains the headline monthly people metrics.
type WorkspaceHRSummary struct {
	Title          string `json:"title"`
	Active         int    `json:"active"`
	Hires          int    `json:"hires"`
	Separations    int    `json:"separations"`
	SeparationRate string `json:"separation_rate"`
}

// WorkspaceOverviewAttendance summarizes today's attendance and monthly leave bars.
type WorkspaceOverviewAttendance struct {
	CheckedIn  int                        `json:"checked_in"`
	Leave      int                        `json:"leave"`
	Absent     int                        `json:"absent"`
	Segments   []WorkspaceAttendanceSlice `json:"segments"`
	DailyLeave []WorkspaceDailyLeave      `json:"daily_leave"`
}

// WorkspaceAttendanceSlice describes one proportional attendance segment.
type WorkspaceAttendanceSlice struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Percent int    `json:"percent"`
	Tone    string `json:"tone"`
}

// WorkspaceDailyLeave describes one day in the monthly leave bar chart.
type WorkspaceDailyLeave struct {
	Day           int    `json:"day"`
	Value         int    `json:"value"`
	HeightPercent int    `json:"height_percent"`
	ShowLabel     bool   `json:"show_label"`
	Active        bool   `json:"active"`
	Tooltip       string `json:"tooltip"`
}

// WorkspaceTodoCategory groups lifecycle reminders shown on the overview page.
type WorkspaceTodoCategory struct {
	Key       string                `json:"key"`
	Label     string                `json:"label"`
	Icon      string                `json:"icon"`
	Desc      string                `json:"desc"`
	DateLabel string                `json:"date_label"`
	People    []WorkspaceTodoPerson `json:"people"`
	Count     int                   `json:"count"`
}

// WorkspaceTodoPerson is the compact employee card used in lifecycle reminders.
type WorkspaceTodoPerson struct {
	ID     string `json:"id"`
	NameZH string `json:"name_zh"`
	NameEN string `json:"name_en"`
	Date   string `json:"date"`
}

// WorkspaceOrganizationResponse returns the employee hierarchy rows.
type WorkspaceOrganizationResponse struct {
	ParentNone string                     `json:"parent_none"`
	Rows       []WorkspaceOrganizationRow `json:"rows"`
}

// WorkspaceOrganizationRow is one employee node in the organization chart.
type WorkspaceOrganizationRow struct {
	ID        string `json:"id"`
	NameZH    string `json:"name_zh"`
	NameEN    string `json:"name_en"`
	Dept      string `json:"dept"`
	Title     string `json:"title"`
	Level     int    `json:"level"`
	IsManager bool   `json:"is_manager"`
	ParentID  string `json:"parent_id"`
}

// WorkspaceTurnoverQuery selects the monthly and annual turnover windows.
type WorkspaceTurnoverQuery struct {
	Year       int `json:"year,omitempty"`
	Month      int `json:"month,omitempty"`
	AnnualYear int `json:"annual_year,omitempty"`
}

// WorkspaceTurnoverResponse aggregates monthly and annual employment analysis.
type WorkspaceTurnoverResponse struct {
	Monthly WorkspaceTurnoverMonthly `json:"monthly"`
	Annual  WorkspaceTurnoverAnnual  `json:"annual"`
}

// WorkspaceTurnoverMonthly contains monthly headcount, turnover, chart, and table data.
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

// WorkspaceTurnoverAnnual contains annual headcount, turnover, chart, and table data.
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

// WorkspaceKPI is a compact metric card for turnover analysis.
type WorkspaceKPI struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Value     string `json:"value"`
	Unit      string `json:"unit"`
	TrendText string `json:"trend_text"`
	TrendTone string `json:"trend_tone"`
}

// WorkspaceComparisonItem is one bar in a comparison chart.
type WorkspaceComparisonItem struct {
	Name    string  `json:"name"`
	Value   float64 `json:"value"`
	Label   string  `json:"label"`
	Percent int     `json:"percent"`
}

// WorkspaceTurnoverRow is one monthly turnover table row.
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

// WorkspaceTrendPoint is one monthly point in annual trend charts.
type WorkspaceTrendPoint struct {
	Month   int     `json:"month"`
	Value   float64 `json:"value"`
	Label   string  `json:"label"`
	Percent int     `json:"percent"`
	Future  bool    `json:"future"`
	Tone    string  `json:"tone"`
}

// WorkspacePieItem is one segment in the annual distribution chart.
type WorkspacePieItem struct {
	Name       string  `json:"name"`
	Value      int     `json:"value"`
	Percent    int     `json:"percent"`
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	ColorIndex int     `json:"color_index"`
}

// WorkspaceAnnualRow is one annual turnover table row.
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

// WorkspaceAttendanceQuery selects one month for the attendance and clock matrices.
type WorkspaceAttendanceQuery struct {
	Year  int `json:"year,omitempty"`
	Month int `json:"month,omitempty"`
}

// WorkspaceAttendanceResponse aggregates monthly attendance and clock matrices.
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

// WorkspaceDate is one calendar column in monthly matrices.
type WorkspaceDate struct {
	Key     string  `json:"key"`
	Y       int     `json:"y"`
	M       int     `json:"m"`
	D       int     `json:"d"`
	DOW     int     `json:"dow"`
	Holiday *string `json:"holiday"`
}

// WorkspaceLeaveLegendItem labels one leave code used in matrices.
type WorkspaceLeaveLegendItem struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// WorkspaceAttendanceMatrix contains work-hour rows and summary metrics.
type WorkspaceAttendanceMatrix struct {
	Rows    []WorkspaceAttendanceRow     `json:"rows"`
	Summary WorkspaceAttendanceMatrixSum `json:"summary"`
}

// WorkspaceAttendanceRow is one employee row in the work-hour matrix.
type WorkspaceAttendanceRow struct {
	Employee WorkspaceEmployeeCard  `json:"employee"`
	Cells    []WorkspaceDayCell     `json:"cells"`
	Summary  WorkspaceEmployeeHours `json:"summary"`
}

// WorkspaceEmployeeCard is the compact employee identity shared by workspace tables.
type WorkspaceEmployeeCard struct {
	ID       string `json:"id"`
	Avatar   string `json:"avatar"`
	NameZH   string `json:"name_zh"`
	NameEN   string `json:"name_en"`
	Email    string `json:"email"`
	Dept     string `json:"dept"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Phone    string `json:"phone"`
	Status   string `json:"status"`
	HireDate string `json:"hire_date"`
}

// WorkspaceDayCell describes a day cell in attendance and clock matrices.
type WorkspaceDayCell struct {
	Type     string  `json:"type"`
	Holiday  string  `json:"holiday,omitempty"`
	Leave    string  `json:"leave,omitempty"`
	Hours    float64 `json:"hours,omitempty"`
	Label    string  `json:"label,omitempty"`
	In       string  `json:"in,omitempty"`
	Out      string  `json:"out,omitempty"`
	InLoc    string  `json:"in_loc,omitempty"`
	OutLoc   string  `json:"out_loc,omitempty"`
	Abnormal bool    `json:"abnormal,omitempty"`
	Reason   string  `json:"reason,omitempty"`
}

// WorkspaceEmployeeHours summarizes one employee's monthly work-hour row.
type WorkspaceEmployeeHours struct {
	AttendedHours float64            `json:"attended_hours"`
	Birthday      bool               `json:"birthday"`
	DeductHours   float64            `json:"deduct_hours"`
	DueHours      float64            `json:"due_hours"`
	LeaveByType   map[string]float64 `json:"leave_by_type"`
	LeaveHours    float64            `json:"leave_hours"`
	WorkDays      int                `json:"work_days"`
}

// WorkspaceAttendanceMatrixSum summarizes the work-hour matrix.
type WorkspaceAttendanceMatrixSum struct {
	Holidays   int     `json:"holidays"`
	LeaveHours float64 `json:"leave_hours"`
	Perfect    int     `json:"perfect"`
	Workdays   int     `json:"workdays"`
}

// WorkspaceClockMatrix contains raw clock rows and abnormal records.
type WorkspaceClockMatrix struct {
	Abnormals []WorkspaceClockAbnormal `json:"abnormals"`
	Rows      []WorkspaceClockRow      `json:"rows"`
	Summary   WorkspaceClockSummary    `json:"summary"`
}

// WorkspaceClockAbnormal identifies one abnormal clock day.
type WorkspaceClockAbnormal struct {
	Date     WorkspaceDate         `json:"date"`
	Employee WorkspaceEmployeeCard `json:"employee"`
	Record   WorkspaceDayCell      `json:"record"`
}

// WorkspaceClockRow is one employee row in the clock matrix.
type WorkspaceClockRow struct {
	Employee WorkspaceEmployeeCard `json:"employee"`
	Cells    []WorkspaceDayCell    `json:"cells"`
}

// WorkspaceClockSummary summarizes the clock matrix.
type WorkspaceClockSummary struct {
	AbnormalDays   int `json:"abnormal_days"`
	AbnormalPeople int `json:"abnormal_people"`
	NormalDays     int `json:"normal_days"`
}

// WorkspaceAdminsResponse projects IAM grants into the HR administrator settings page.
type WorkspaceAdminsResponse struct {
	Admins     []WorkspaceAdmin          `json:"admins"`
	Candidates []WorkspaceAdminCandidate `json:"candidates"`
	Sections   []WorkspaceAdminSection   `json:"sections"`
}

// WorkspaceAdmin is one account with HR workspace administration permissions.
type WorkspaceAdmin struct {
	ID          string            `json:"id"`
	AccountID   string            `json:"account_id"`
	Avatar      string            `json:"avatar"`
	NameZH      string            `json:"name_zh"`
	NameEN      string            `json:"name_en"`
	Dept        string            `json:"dept"`
	Title       string            `json:"title"`
	AssignedAt  string            `json:"assigned_at"`
	AssignedBy  string            `json:"assigned_by"`
	Permissions map[string]string `json:"permissions"`
}

// WorkspaceAdminCandidate is one employee/account that can be granted HR admin access.
type WorkspaceAdminCandidate struct {
	ID        string `json:"id"`
	AccountID string `json:"account_id"`
	Avatar    string `json:"avatar"`
	NameZH    string `json:"name_zh"`
	NameEN    string `json:"name_en"`
	Dept      string `json:"dept"`
	Title     string `json:"title"`
}

// WorkspaceAdminSection describes one permission group displayed by the admin settings page.
type WorkspaceAdminSection struct {
	Group string                      `json:"group"`
	Items []WorkspaceAdminSectionItem `json:"items"`
}

// WorkspaceAdminSectionItem describes one editable permission column.
type WorkspaceAdminSectionItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Icon  string `json:"icon"`
}

// WorkspaceAuditLogQuery filters the workspace audit log projection.
type WorkspaceAuditLogQuery struct {
	OperatorID string `json:"operator_id,omitempty"`
	Type       string `json:"type,omitempty"`
	From       string `json:"from,omitempty"`
	To         string `json:"to,omitempty"`
	Keyword    string `json:"keyword,omitempty"`
}

// WorkspaceAuditLog is the page-level audit log projection.
type WorkspaceAuditLog struct {
	ID       string `json:"id"`
	Time     string `json:"time"`
	Operator string `json:"operator"`
	Type     string `json:"type"`
	Action   string `json:"action"`
	Detail   string `json:"detail"`
}
