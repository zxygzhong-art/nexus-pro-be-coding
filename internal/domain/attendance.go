package domain

import (
	"strings"
	"time"
)

// LeaveBalance 定義請假 balance 的資料結構。
type LeaveBalance struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	EmployeeID       string     `json:"employee_id"`
	LeaveType        string     `json:"leave_type"`
	LeaveTypeID      string     `json:"leave_type_id"`
	EntitlementYear  int        `json:"entitlement_year"`
	GrantedMinutes   int        `json:"granted_minutes"`
	UsedMinutes      int        `json:"used_minutes"`
	RemainingMinutes int        `json:"remaining_minutes"`
	Source           string     `json:"source"`
	LastSyncedAt     *time.Time `json:"last_synced_at,omitempty"`
	// SnapshotRemainingMinutes is the unmodified upstream bucket. Effective
	// availability is derived by applying the append-only local entries.
	SnapshotRemainingMinutes int       `json:"snapshot_remaining_minutes,omitempty"`
	PendingMinutes           int       `json:"pending_minutes,omitempty"`
	LocalUsedMinutes         int       `json:"local_used_minutes,omitempty"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// LeaveBalanceQuery scopes leave balances to selected employees before pagination.
type LeaveBalanceQuery struct {
	EmployeeIDs []string `json:"employee_ids,omitempty"`
}

// LeaveRequest 定義請假請求的資料結構。
type LeaveRequest struct {
	ID                   string         `json:"id"`
	TenantID             string         `json:"tenant_id"`
	EmployeeID           string         `json:"employee_id"`
	LeaveType            string         `json:"leave_type"`
	LeaveTypeID          string         `json:"leave_type_id,omitempty"`
	PolicyVersion        int            `json:"policy_version,omitempty"`
	RuleSnapshot         map[string]any `json:"rule_snapshot,omitempty"`
	EvaluationSnapshot   map[string]any `json:"evaluation_snapshot,omitempty"`
	StartAt              time.Time      `json:"start_at"`
	EndAt                time.Time      `json:"end_at"`
	RequestedMinutes     int            `json:"requested_minutes"`
	Reason               string         `json:"reason,omitempty"`
	Status               string         `json:"status"`
	FormInstanceID       string         `json:"form_instance_id,omitempty"`
	ReconciliationStatus string         `json:"reconciliation_status,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	EffectStatus         string         `json:"-"`
	EffectResult         map[string]any `json:"-"`
	EffectAppliedAt      *time.Time     `json:"-"`
}

// LeaveBalanceEntry is the append-only minute-level overlay on one annual balance.
// Negative amounts reduce effective availability; positive amounts release or reconcile it.
type LeaveBalanceEntry struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	BalanceID       string    `json:"balance_id"`
	LeaveRecordID   string    `json:"leave_record_id,omitempty"`
	EmployeeID      string    `json:"employee_id"`
	LeaveTypeID     string    `json:"leave_type_id"`
	EntitlementYear int       `json:"entitlement_year"`
	EntryType       string    `json:"entry_type"`
	AmountMinutes   int       `json:"amount_minutes"`
	IdempotencyKey  string    `json:"idempotency_key"`
	OccurredAt      time.Time `json:"occurred_at"`
	CreatedAt       time.Time `json:"created_at"`
}

// LeaveRecord is one Nexus or eHRMS leave fact tied to exactly one annual balance.
type LeaveRecord struct {
	ID                   string     `json:"id"`
	TenantID             string     `json:"tenant_id"`
	EmployeeID           string     `json:"employee_id"`
	LeaveTypeID          string     `json:"leave_type_id"`
	BalanceID            string     `json:"balance_id"`
	EntitlementYear      int        `json:"entitlement_year"`
	Source               string     `json:"source"`
	EventDate            time.Time  `json:"event_date"`
	StartAt              time.Time  `json:"start_at"`
	EndAt                time.Time  `json:"end_at"`
	NetMinutes           int        `json:"net_minutes"`
	Remark               string     `json:"remark,omitempty"`
	Status               string     `json:"status"`
	MatchedRecordID      string     `json:"matched_record_id,omitempty"`
	ReconciliationStatus string     `json:"reconciliation_status"`
	LastSeenAt           *time.Time `json:"last_seen_at,omitempty"`
	DeletedAt            *time.Time `json:"deleted_at,omitempty"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// LeaveType is one tenant leave catalog row synced from EHRMS /leave-types.
// Enabled maps to leave_types.status (active/inactive).
// Code is the upstream leave-type identity (no separate external_code mapping).
// ID defaults to Code. When a category and item share the same upstream code,
// the non-item node uses <kind>:<code> so both rows remain addressable.
type LeaveType struct {
	ID                string         `json:"id"`
	TenantID          string         `json:"tenant_id,omitempty"`
	Code              string         `json:"code"`
	Kind              string         `json:"kind"`
	ParentID          string         `json:"parent_id,omitempty"`
	ParentCode        string         `json:"parent_code,omitempty"`
	NameZH            string         `json:"name_zh"`
	NameEN            string         `json:"name_en"`
	Category          string         `json:"category"`
	RequiresBalance   bool           `json:"-"`
	MaxBalanceMinutes int            `json:"max_balance_minutes"`
	Unit              string         `json:"-"`
	Enabled           bool           `json:"enabled"`
	DisplayOrder      int            `json:"display_order"`
	RawPayload        map[string]any `json:"-"`
	LastSyncedAt      *time.Time     `json:"-"`
	UpdatedAt         time.Time      `json:"-"`
}

// LeaveTypeCatalog is the local leave catalog rendered by forms and HR settings.
type LeaveTypeCatalog struct {
	Items   []LeaveType `json:"items"`
	Total   int         `json:"total"`
	Enabled int         `json:"enabled"`
}

// SetLeaveTypeEnabledInput changes leave_types.status for one leave type.
type SetLeaveTypeEnabledInput struct {
	Enabled bool `json:"enabled"`
}

// DefaultLeaveTypes is a convenience catalog for in-memory test seeding only.
// Production Postgres leave_types rows are synced from EHRMS /leave-types.
func DefaultLeaveTypes() []LeaveType {
	return []LeaveType{
		{ID: "sick_full", Code: "sick_full", Kind: "item", NameZH: "全薪病假", NameEN: "Full Pay Sick Leave", Category: "statutory", RequiresBalance: true, Enabled: true, DisplayOrder: 1},
		{ID: "flexible", Code: "flexible", Kind: "item", NameZH: "彈性休假", NameEN: "Additional Leave", Category: "company", RequiresBalance: true, Enabled: true, DisplayOrder: 2},
		{ID: "personal", Code: "personal", Kind: "item", NameZH: "事假", NameEN: "Personal Leave", Category: "statutory", Enabled: true, DisplayOrder: 3},
		{ID: "family_care", Code: "family_care", Kind: "item", NameZH: "家庭照顧假", NameEN: "Family Care Leave", Category: "statutory", Enabled: true, DisplayOrder: 4},
		{ID: "sick_half", Code: "sick_half", Kind: "item", NameZH: "半薪病假", NameEN: "Half Pay Sick Leave", Category: "statutory", RequiresBalance: true, Enabled: true, DisplayOrder: 5},
		{ID: "menstrual", Code: "menstrual", Kind: "item", NameZH: "生理假", NameEN: "Menstruation Leave", Category: "statutory", Enabled: true, DisplayOrder: 6},
		{ID: "marriage", Code: "marriage", Kind: "item", NameZH: "婚假", NameEN: "Marriage Leave", Category: "statutory", Enabled: true, DisplayOrder: 7},
		{ID: "maternity", Code: "maternity", Kind: "item", NameZH: "八週產假", NameEN: "8-Week Maternity Leave", Category: "statutory", Enabled: true, DisplayOrder: 8},
		{ID: "paternity", Code: "paternity", Kind: "item", NameZH: "陪產假", NameEN: "Paternity Leave", Category: "statutory", Enabled: true, DisplayOrder: 9},
		{ID: "bereavement", Code: "bereavement", Kind: "item", NameZH: "喪假", NameEN: "Bereavement Leave", Category: "statutory", Enabled: true, DisplayOrder: 10},
		{ID: "official", Code: "official", Kind: "item", NameZH: "公假", NameEN: "Official Leave", Category: "statutory", Enabled: true, DisplayOrder: 11},
		{ID: "prenatal", Code: "prenatal", Kind: "item", NameZH: "產檢假", NameEN: "Prenatal Leave", Category: "statutory", Enabled: true, DisplayOrder: 12},
		{ID: "compensatory", Code: "compensatory", Kind: "item", NameZH: "補休假", NameEN: "Compensatory Leave", Category: "company", RequiresBalance: true, Enabled: true, DisplayOrder: 13},
		{ID: "annual", Code: "annual", Kind: "item", NameZH: "特休假", NameEN: "Annual Leave", Category: "statutory", RequiresBalance: true, Enabled: true, DisplayOrder: 14},
		{ID: "business_trip", Code: "business_trip", Kind: "item", NameZH: "外勤", NameEN: "Business Trip", Category: "company", Enabled: true, DisplayOrder: 15},
	}
}

// StableLeaveTypeID derives the immutable local identity from a canonical leave code.
func StableLeaveTypeID(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return ""
	}
	return code
}

// CreateLeaveRequestInput 定義請假請求輸入的資料結構。
type CreateLeaveRequestInput struct {
	EmployeeID string  `json:"employee_id,omitempty"`
	LeaveType  string  `json:"leave_type"`
	StartAt    string  `json:"start_at"`
	EndAt      string  `json:"end_at"`
	Hours      float64 `json:"hours"`
	Reason     string  `json:"reason,omitempty"`
}

// EvaluateLeaveRequestInput describes a dry-run leave eligibility check.
type EvaluateLeaveRequestInput struct {
	EmployeeID string  `json:"employee_id,omitempty"`
	LeaveType  string  `json:"leave_type"`
	StartAt    string  `json:"start_at"`
	EndAt      string  `json:"end_at"`
	Hours      float64 `json:"hours,omitempty"`
}

// LeaveRuleSnapshot freezes the effective leave policy used by one request.
type LeaveRuleSnapshot struct {
	LeaveTypeID     string   `json:"leave_type_id"`
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	GrantMode       string   `json:"grant_mode"`
	RequiresBalance bool     `json:"requires_balance"`
	ProofAfterHours *float64 `json:"proof_after_hours,omitempty"`
	PolicyVersion   int      `json:"policy_version"`
}

// LeaveRequestEvaluation is the shared decision returned to API, workflow, and agent callers.
type LeaveRequestEvaluation struct {
	Eligible              bool              `json:"eligible"`
	Status                string            `json:"status"`
	Message               string            `json:"message"`
	EmployeeID            string            `json:"employee_id"`
	LeaveTypeID           string            `json:"leave_type_id"`
	LeaveType             string            `json:"leave_type"`
	LeaveTypeName         string            `json:"leave_type_name"`
	RequestedMinutes      int               `json:"requested_minutes"`
	PolicyVersion         int               `json:"policy_version"`
	BalanceRequired       bool              `json:"balance_required"`
	BalanceInitialized    bool              `json:"balance_initialized"`
	BalanceFallbackReason string            `json:"balance_fallback_reason,omitempty"`
	AvailableMinutes      int               `json:"available_minutes,omitempty"`
	ProofRequired         bool              `json:"proof_required"`
	Rule                  LeaveRuleSnapshot `json:"rule"`
}

// LeaveRequestQuery 定義請假請求查詢的資料結構。
type LeaveRequestQuery struct {
	EmployeeIDs []string `json:"employee_ids,omitempty"`
	Status      string   `json:"status,omitempty"`
	FromDate    string   `json:"from_date,omitempty"`
	ToDate      string   `json:"to_date,omitempty"`
}

// OvertimeRequest 定義加班申請的資料結構。
type OvertimeRequest struct {
	ID               string         `json:"id"`
	TenantID         string         `json:"tenant_id"`
	EmployeeID       string         `json:"employee_id"`
	WorkDate         string         `json:"work_date"`
	StartAt          time.Time      `json:"start_at"`
	EndAt            time.Time      `json:"end_at"`
	Hours            float64        `json:"hours"`
	OvertimeType     string         `json:"overtime_type"`
	CompensationType string         `json:"compensation_type"`
	Reason           string         `json:"reason,omitempty"`
	Status           string         `json:"status"`
	FormInstanceID   string         `json:"form_instance_id,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	EffectStatus     string         `json:"-"`
	EffectResult     map[string]any `json:"-"`
	EffectAppliedAt  *time.Time     `json:"-"`
}

// CreateOvertimeRequestInput 定義加班申請輸入的資料結構。
type CreateOvertimeRequestInput struct {
	EmployeeID       string  `json:"employee_id,omitempty"`
	StartAt          string  `json:"start_at"`
	EndAt            string  `json:"end_at"`
	Hours            float64 `json:"hours"`
	OvertimeType     string  `json:"overtime_type,omitempty"`
	CompensationType string  `json:"compensation_type,omitempty"`
	Reason           string  `json:"reason,omitempty"`
}

// OvertimeRequestQuery 定義加班申請查詢的資料結構。
type OvertimeRequestQuery struct {
	EmployeeIDs []string `json:"employee_ids,omitempty"`
	Status      string   `json:"status,omitempty"`
	FromDate    string   `json:"from_date,omitempty"`
	ToDate      string   `json:"to_date,omitempty"`
}

// AttendanceWorksite 定義考勤工作地點的資料結構。
type AttendanceWorksite struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Name         string    `json:"name"`
	Address      string    `json:"address,omitempty"`
	Latitude     float64   `json:"latitude"`
	Longitude    float64   `json:"longitude"`
	RadiusMeters int       `json:"radius_meters"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AttendanceClockRecord 定義考勤打卡 record 的資料結構。
type AttendanceClockRecord struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	EmployeeID      string    `json:"employee_id"`
	WorksiteID      string    `json:"worksite_id,omitempty"`
	WorksiteName    string    `json:"worksite_name,omitempty"`
	WorksiteAddress string    `json:"worksite_address,omitempty"`
	WorkDate        string    `json:"work_date"`
	Direction       string    `json:"direction"`
	ClientEventID   string    `json:"client_event_id,omitempty"`
	ClockedAt       time.Time `json:"clocked_at"`
	Latitude        float64   `json:"latitude"`
	Longitude       float64   `json:"longitude"`
	AccuracyMeters  float64   `json:"accuracy_meters,omitempty"`
	DistanceMeters  float64   `json:"distance_meters,omitempty"`
	// LocationCaptured distinguishes a real (0,0) coordinate from records such
	// as approved manual corrections, which deliberately carry no GPS evidence.
	LocationCaptured    bool           `json:"location_captured"`
	RecordStatus        string         `json:"record_status"`
	RejectionReason     string         `json:"rejection_reason,omitempty"`
	Source              string         `json:"source"`
	DeviceID            string         `json:"device_id,omitempty"`
	DeviceInfo          map[string]any `json:"device_info,omitempty"`
	CorrectionRequestID string         `json:"correction_request_id,omitempty"`
	Voided              bool           `json:"voided,omitempty"`
	VoidedAt            *time.Time     `json:"voided_at,omitempty"`
	VoidedByAccountID   string         `json:"voided_by_account_id,omitempty"`
	VoidReason          string         `json:"void_reason,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
}

// AttendanceDailySummary 定義考勤日彙總的資料結構。
// Slim eHRMS clock/shift day sheet keyed by tenant + employee + work date.
type AttendanceDailySummary struct {
	TenantID    string         `json:"tenant_id"`
	EmployeeID  string         `json:"employee_id"`
	WorkDate    string         `json:"work_date"`
	ShiftStart  string         `json:"shift_start,omitempty"`
	ShiftEnd    string         `json:"shift_end,omitempty"`
	ShiftHours  float64        `json:"shift_hours,omitempty"`
	DailyHours  float64        `json:"daily_hours,omitempty"`
	ClockHours  float64        `json:"clock_hours,omitempty"`
	ClockStart  string         `json:"clock_start,omitempty"`
	ClockEnd    string         `json:"clock_end,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
	Source      string         `json:"source"`
	ExternalRef string         `json:"external_ref"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// AttendanceDayProjection is the rebuildable, policy-versioned read model for
// one employee and business date. Raw clocks, leave cases and policy versions
// remain the sources of truth; this row is safe to replace after any input
// changes.
type AttendanceDayProjection struct {
	TenantID             string         `json:"tenant_id"`
	EmployeeID           string         `json:"employee_id"`
	WorkDate             string         `json:"work_date"`
	PolicyVersion        int            `json:"policy_version"`
	ScheduledStartAt     *time.Time     `json:"scheduled_start_at,omitempty"`
	ScheduledEndAt       *time.Time     `json:"scheduled_end_at,omitempty"`
	ClockInRecordID      string         `json:"clock_in_record_id,omitempty"`
	ClockOutRecordID     string         `json:"clock_out_record_id,omitempty"`
	LastPunchRecordID    string         `json:"last_punch_record_id,omitempty"`
	PunchCount           int            `json:"punch_count"`
	WorkedMinutes        int            `json:"worked_minutes"`
	ApprovedLeaveMinutes int            `json:"approved_leave_minutes"`
	PendingLeaveMinutes  int            `json:"pending_leave_minutes"`
	RequiredMinutes      int            `json:"required_minutes"`
	OvertimeMinutes      int            `json:"overtime_minutes"`
	DayStatus            string         `json:"day_status"`
	AnomalyReasons       []string       `json:"anomaly_reasons,omitempty"`
	InputFingerprint     string         `json:"input_fingerprint"`
	Payload              map[string]any `json:"payload,omitempty"`
	ComputedAt           time.Time      `json:"computed_at"`
	UpdatedAt            time.Time      `json:"updated_at"`

	// Runtime-only materialization used by the clock-status response. Persistence
	// stores stable record IDs; a projector may attach the source rows it loaded.
	ClockIn     *AttendanceClockRecord `json:"-"`
	ClockOut    *AttendanceClockRecord `json:"-"`
	LastPunch   *AttendanceClockRecord `json:"-"`
	CanClockIn  bool                   `json:"-"`
	CanClockOut bool                   `json:"-"`
}

// AttendanceCorrectionRequest 定義考勤 correction 請求的資料結構。
type AttendanceCorrectionRequest struct {
	ID                       string         `json:"id"`
	TenantID                 string         `json:"tenant_id"`
	EmployeeID               string         `json:"employee_id"`
	Direction                string         `json:"direction"`
	RequestedClockedAt       time.Time      `json:"requested_clocked_at"`
	WorkDate                 string         `json:"work_date"`
	CorrectionType           string         `json:"correction_type"`
	TargetClockRecordID      string         `json:"target_clock_record_id,omitempty"`
	ReplacementClockRecordID string         `json:"replacement_clock_record_id,omitempty"`
	Reason                   string         `json:"reason"`
	Status                   string         `json:"status"`
	FormInstanceID           string         `json:"form_instance_id,omitempty"`
	ClockRecordID            string         `json:"clock_record_id,omitempty"`
	ReviewedByAccountID      string         `json:"reviewed_by_account_id,omitempty"`
	ReviewReason             string         `json:"review_reason,omitempty"`
	ReviewedAt               *time.Time     `json:"reviewed_at,omitempty"`
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
	EffectStatus             string         `json:"-"`
	EffectResult             map[string]any `json:"-"`
	EffectAppliedAt          *time.Time     `json:"-"`
}

// AttendanceClockStatus 定義考勤打卡狀態的資料結構。
type AttendanceClockStatus struct {
	EmployeeID           string                 `json:"employee_id"`
	WorkDate             string                 `json:"work_date"`
	Worksite             *AttendanceWorksite    `json:"worksite,omitempty"`
	Worksites            []AttendanceWorksite   `json:"worksites"`
	RequireWorksite      bool                   `json:"require_worksite"`
	ClockIn              *AttendanceClockRecord `json:"clock_in,omitempty"`
	ClockOut             *AttendanceClockRecord `json:"clock_out,omitempty"`
	LastPunch            *AttendanceClockRecord `json:"last_punch,omitempty"`
	PunchCount           int                    `json:"punch_count"`
	NextAction           string                 `json:"next_action"`
	CanClockIn           bool                   `json:"can_clock_in"`
	CanClockOut          bool                   `json:"can_clock_out"`
	WorkedMinutes        int                    `json:"worked_minutes"`
	ApprovedLeaveMinutes int                    `json:"approved_leave_minutes"`
	RequiredMinutes      int                    `json:"required_minutes"`
	DayStatus            string                 `json:"day_status"`
	AnomalyReasons       []string               `json:"anomaly_reasons,omitempty"`
}

// AttendanceMonthlySummary is the authoritative self-service projection for one calendar month.
type AttendanceMonthlySummary struct {
	EmployeeID     string                        `json:"employee_id"`
	Month          string                        `json:"month"`
	AttendanceDays int                           `json:"attendance_days"`
	WorkedMinutes  int                           `json:"worked_minutes"`
	RecordCount    int                           `json:"record_count"`
	AbnormalDays   int                           `json:"abnormal_days"`
	Days           []AttendanceMonthlyDaySummary `json:"days"`
}

// AttendanceMonthlyDaySummary exposes the projected state needed by the monthly attendance calendar.
type AttendanceMonthlyDaySummary struct {
	WorkDate       string   `json:"work_date"`
	WorkedMinutes  int      `json:"worked_minutes"`
	RecordCount    int      `json:"record_count"`
	DayStatus      string   `json:"day_status"`
	AnomalyReasons []string `json:"anomaly_reasons,omitempty"`
}

// AttendancePolicyResponse 定義考勤政策回應的資料結構。
type AttendancePolicyResponse struct {
	WorkTime AttendancePolicyWorkTime `json:"work_time"`
	Version  int                      `json:"version,omitempty"`
}

// AttendancePolicyValidationResult reports whether a draft can be published safely.
type AttendancePolicyValidationResult struct {
	Valid            bool                     `json:"valid"`
	Issues           []string                 `json:"issues"`
	ProjectedVersion int                      `json:"projected_version"`
	Policy           AttendancePolicyResponse `json:"policy"`
}

// AttendancePolicy 定義考勤政策的資料結構。
type AttendancePolicy struct {
	TenantID             string                   `json:"tenant_id"`
	WorkTime             AttendancePolicyWorkTime `json:"work_time"`
	Version              int                      `json:"version,omitempty"`
	EffectiveFrom        *time.Time               `json:"effective_from,omitempty"`
	PublishedByAccountID string                   `json:"published_by_account_id,omitempty"`
	PublishedAt          time.Time                `json:"published_at"`
}

// AttendancePolicyWorkTime 定義考勤政策 work 時間的資料結構。
type AttendancePolicyWorkTime struct {
	RequireWorksite         bool     `json:"require_worksite"`
	ClockMode               string   `json:"clock_mode"`
	FlexibleClockInEarliest string   `json:"flexible_clock_in_earliest"`
	FlexibleClockOutLatest  string   `json:"flexible_clock_out_latest"`
	StandardStart           string   `json:"standard_start"`
	StandardEnd             string   `json:"standard_end"`
	BreakStart              string   `json:"break_start"`
	BreakEnd                string   `json:"break_end"`
	Weekend                 string   `json:"weekend"`
	CycleStart              string   `json:"cycle_start"`
	CycleEnd                string   `json:"cycle_end"`
	TimeOptions             []string `json:"time_options,omitempty"`
	WeekendOptions          []string `json:"weekend_options,omitempty"`
	CycleStartOptions       []string `json:"cycle_start_options,omitempty"`
	CycleEndOptions         []string `json:"cycle_end_options,omitempty"`
}

// Leave grant modes.
const (
	LeaveGrantModeAnnualGrant    = "annual_grant"
	LeaveGrantModeEvent          = "event"
	LeaveGrantModeOvertimeCredit = "overtime_credit"
	LeaveGrantModeUnlimited      = "unlimited"
)

// LeaveEntitlementRule 定義假別額度檔位規則。
type LeaveEntitlementRule struct {
	JobLevel       string  `json:"job_level"`
	TenureMinYears int     `json:"tenure_min_years"`
	TenureMaxYears *int    `json:"tenure_max_years,omitempty"`
	QuotaHours     float64 `json:"quota_hours"`
	Prorate        bool    `json:"prorate"`
	Priority       int     `json:"priority"`
}

// AttendanceLeaveType 定義考勤請假 type 的資料結構。
type AttendanceLeaveType struct {
	ID              string                 `json:"id,omitempty"`
	Code            string                 `json:"code"`
	Name            string                 `json:"name"`
	Quota           string                 `json:"quota"`
	Rule            string                 `json:"rule"`
	Proof           string                 `json:"proof"`
	GrantMode       string                 `json:"grant_mode,omitempty"`
	RequiresBalance bool                   `json:"requires_balance"`
	ProofAfterHours *float64               `json:"proof_after_hours,omitempty"`
	Active          bool                   `json:"active"`
	Entitlements    []LeaveEntitlementRule `json:"entitlements,omitempty"`
}

// UpdateAttendancePolicyInput 定義考勤政策輸入的資料結構。
type UpdateAttendancePolicyInput struct {
	BaseVersion int                      `json:"base_version,omitempty"`
	WorkTime    AttendancePolicyWorkTime `json:"work_time"`
}

// GrantLeaveBalancesInput 定義發放請假餘額輸入。
type GrantLeaveBalancesInput struct {
	EmployeeID  string `json:"employee_id,omitempty"`
	PeriodStart string `json:"period_start,omitempty"`
	PeriodEnd   string `json:"period_end,omitempty"`
}

// GrantLeaveBalancesResult 定義發放請假餘額結果。
type GrantLeaveBalancesResult struct {
	Granted       int        `json:"granted"`
	Updated       int        `json:"updated"`
	Skipped       int        `json:"skipped"`
	Failed        int        `json:"failed"`
	PeriodStart   string     `json:"period_start"`
	PeriodEnd     string     `json:"period_end"`
	PolicyVersion int        `json:"policy_version"`
	RowErrors     []RowError `json:"row_errors,omitempty"`
}

// Validate 驗證目前流程。
func (in UpdateAttendancePolicyInput) Validate() error {
	if mode := strings.ToLower(strings.TrimSpace(in.WorkTime.ClockMode)); mode != "" && mode != "flexible" && mode != "fixed" {
		return BadRequest("clock_mode must be flexible or fixed")
	}
	if strings.TrimSpace(in.WorkTime.StandardStart) == "" || strings.TrimSpace(in.WorkTime.StandardEnd) == "" {
		return BadRequest("standard_start and standard_end are required")
	}
	if strings.TrimSpace(in.WorkTime.BreakStart) == "" || strings.TrimSpace(in.WorkTime.BreakEnd) == "" {
		return BadRequest("break_start and break_end are required")
	}
	if strings.TrimSpace(in.WorkTime.Weekend) == "" || strings.TrimSpace(in.WorkTime.CycleStart) == "" || strings.TrimSpace(in.WorkTime.CycleEnd) == "" {
		return BadRequest("weekend, cycle_start and cycle_end are required")
	}
	return nil
}

// CreateAttendanceWorksiteInput 定義考勤工作地點輸入的資料結構。
type CreateAttendanceWorksiteInput struct {
	Name         string  `json:"name"`
	Address      string  `json:"address,omitempty"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	RadiusMeters int     `json:"radius_meters"`
	Status       string  `json:"status,omitempty"`
}

// UpdateAttendanceWorksiteInput 定義考勤工作地點輸入的資料結構。
type UpdateAttendanceWorksiteInput struct {
	ID           string   `json:"id"`
	Name         *string  `json:"name,omitempty"`
	Address      *string  `json:"address,omitempty"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	RadiusMeters *int     `json:"radius_meters,omitempty"`
	Status       *string  `json:"status,omitempty"`
}

// CreateAttendanceClockRecordInput 定義考勤打卡 record 輸入的資料結構。
type CreateAttendanceClockRecordInput struct {
	EmployeeID     string         `json:"employee_id,omitempty"`
	Direction      string         `json:"direction"`
	ClientEventID  string         `json:"client_event_id,omitempty"`
	Latitude       float64        `json:"latitude"`
	Longitude      float64        `json:"longitude"`
	AccuracyMeters float64        `json:"accuracy_meters,omitempty"`
	LocationSource string         `json:"location_source,omitempty"`
	DeviceID       string         `json:"device_id,omitempty"`
	DeviceInfo     map[string]any `json:"device_info,omitempty"`
}

// AttendanceClockRecordQuery 定義考勤打卡 record 查詢的資料結構。
type AttendanceClockRecordQuery struct {
	EmployeeID   string   `json:"employee_id,omitempty"`
	EmployeeIDs  []string `json:"employee_ids,omitempty"`
	FromDate     string   `json:"from_date,omitempty"`
	ToDate       string   `json:"to_date,omitempty"`
	Direction    string   `json:"direction,omitempty"`
	RecordStatus string   `json:"record_status,omitempty"`
	Source       string   `json:"source,omitempty"`
}

// AttendanceDailySummaryQuery 定義考勤日彙總查詢的資料結構。
type AttendanceDailySummaryQuery struct {
	EmployeeID  string   `json:"employee_id,omitempty"`
	EmployeeIDs []string `json:"employee_ids,omitempty"`
	FromDate    string   `json:"from_date,omitempty"`
	ToDate      string   `json:"to_date,omitempty"`
	Source      string   `json:"source,omitempty"`
}

// EHRMSAttendanceRecord 表示 eHRMS 考勤 record。
type EHRMSAttendanceRecord map[string]string

// EHRMSLeaveBalanceRecord 表示 eHRMS 假別餘額 record。
type EHRMSLeaveBalanceRecord map[string]string

// EHRMSLeaveDetailRecord 表示 eHRMS 已休逐筆明細 record。
type EHRMSLeaveDetailRecord map[string]string

// EHRMSLeaveTypeRecord 表示 eHRMS 假別目錄 record。
type EHRMSLeaveTypeRecord map[string]string

// EHRMSLeaveTypeSyncResponse summarizes one catalog-only EHRMS sync.
type EHRMSLeaveTypeSyncResponse struct {
	Fetched     int `json:"fetched"`
	Upserted    int `json:"upserted"`
	Deactivated int `json:"deactivated"`
}

// EHRMSAttendanceQuery 定義按單一 eHRMS 員工查詢考勤的條件。
type EHRMSAttendanceQuery struct {
	EmployeeID string
	Start      string
	End        string
}

// EHRMSAttendanceSyncInput 定義 eHRMS 考勤 sync 輸入的資料結構。
type EHRMSAttendanceSyncInput struct {
	Mode string `json:"mode,omitempty"`
}

// EHRMSAttendanceSyncResponse 定義 eHRMS 考勤 sync 回應的資料結構。
type EHRMSAttendanceSyncResponse struct {
	Fetched               int                   `json:"fetched"`
	Created               int                   `json:"created"`
	Updated               int                   `json:"updated"`
	Skipped               int                   `json:"skipped"`
	Failed                int                   `json:"failed"`
	LeaveTypesFetched     int                   `json:"leave_types_fetched"`
	LeaveTypesUpserted    int                   `json:"leave_types_upserted"`
	LeaveTypesDeactivated int                   `json:"leave_types_deactivated"`
	LeaveBalancesFetched  int                   `json:"leave_balances_fetched"`
	LeaveBalancesUpserted int                   `json:"leave_balances_upserted"`
	LeaveBalancesSkipped  int                   `json:"leave_balances_skipped"`
	LeaveBalancesFailed   int                   `json:"leave_balances_failed"`
	LeaveDetailsFetched   int                   `json:"leave_details_fetched"`
	LeaveDetailsCreated   int                   `json:"leave_details_created"`
	LeaveDetailsUpdated   int                   `json:"leave_details_updated"`
	LeaveDetailsSkipped   int                   `json:"leave_details_skipped"`
	LeaveDetailsFailed    int                   `json:"leave_details_failed"`
	Mode                  string                `json:"mode"`
	Start                 string                `json:"start,omitempty"`
	Results               []BatchEmployeeResult `json:"results,omitempty"`
	RowErrors             []RowError            `json:"row_errors,omitempty"`
}

// CreateAttendanceCorrectionInput 定義考勤 correction 輸入的資料結構。
type CreateAttendanceCorrectionInput struct {
	EmployeeID          string `json:"employee_id,omitempty"`
	CorrectionType      string `json:"correction_type,omitempty"`
	TargetClockRecordID string `json:"target_clock_record_id,omitempty"`
	Direction           string `json:"direction,omitempty"`
	RequestedClockedAt  string `json:"requested_clocked_at,omitempty"`
	Reason              string `json:"reason"`
}

// ReviewAttendanceCorrectionInput 定義審核考勤 correction 輸入的資料結構。
type ReviewAttendanceCorrectionInput struct {
	Reason string `json:"reason,omitempty"`
}

// AttendanceCorrectionQuery 定義考勤 correction 查詢的資料結構。
type AttendanceCorrectionQuery struct {
	EmployeeID string `json:"employee_id,omitempty"`
	FromDate   string `json:"from_date,omitempty"`
	ToDate     string `json:"to_date,omitempty"`
	Status     string `json:"status,omitempty"`
	Direction  string `json:"direction,omitempty"`
}
