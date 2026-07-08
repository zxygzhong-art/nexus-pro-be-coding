package domain

import (
	"strings"
	"time"
)

// LeaveBalance 定義請假 balance 的資料結構。
type LeaveBalance struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	EmployeeID     string    `json:"employee_id"`
	LeaveType      string    `json:"leave_type"`
	RemainingHours float64   `json:"remaining_hours"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// LeaveRequest 定義請假請求的資料結構。
type LeaveRequest struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	EmployeeID     string    `json:"employee_id"`
	LeaveType      string    `json:"leave_type"`
	StartAt        time.Time `json:"start_at"`
	EndAt          time.Time `json:"end_at"`
	Hours          float64   `json:"hours"`
	Reason         string    `json:"reason,omitempty"`
	Status         string    `json:"status"`
	FormInstanceID string    `json:"form_instance_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
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

// LeaveRequestQuery 定義請假請求查詢的資料結構。
type LeaveRequestQuery struct {
	EmployeeIDs []string `json:"employee_ids,omitempty"`
	Status      string   `json:"status,omitempty"`
	FromDate    string   `json:"from_date,omitempty"`
	ToDate      string   `json:"to_date,omitempty"`
}

// OvertimeRequest 定義加班申請的資料結構。
type OvertimeRequest struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	EmployeeID       string    `json:"employee_id"`
	WorkDate         string    `json:"work_date"`
	StartAt          time.Time `json:"start_at"`
	EndAt            time.Time `json:"end_at"`
	Hours            float64   `json:"hours"`
	OvertimeType     string    `json:"overtime_type"`
	CompensationType string    `json:"compensation_type"`
	Reason           string    `json:"reason,omitempty"`
	Status           string    `json:"status"`
	FormInstanceID   string    `json:"form_instance_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
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

// AttendanceShift 定義考勤班別的資料結構。
type AttendanceShift struct {
	ID                     string    `json:"id"`
	TenantID               string    `json:"tenant_id"`
	Name                   string    `json:"name"`
	ClockInStart           string    `json:"clock_in_start"`
	ClockInEnd             string    `json:"clock_in_end"`
	ClockOutStart          string    `json:"clock_out_start"`
	ClockOutEnd            string    `json:"clock_out_end"`
	LateGraceMinutes       int       `json:"late_grace_minutes,omitempty"`
	EarlyLeaveGraceMinutes int       `json:"early_leave_grace_minutes,omitempty"`
	Status                 string    `json:"status"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// AttendanceShiftAssignment 定義考勤班別指派的資料結構。
type AttendanceShiftAssignment struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	EmployeeID    string     `json:"employee_id"`
	ShiftID       string     `json:"shift_id"`
	WorksiteID    string     `json:"worksite_id"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to,omitempty"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// AttendanceClockRecord 定義考勤打卡 record 的資料結構。
type AttendanceClockRecord struct {
	ID                  string         `json:"id"`
	TenantID            string         `json:"tenant_id"`
	EmployeeID          string         `json:"employee_id"`
	ShiftAssignmentID   string         `json:"shift_assignment_id,omitempty"`
	ShiftID             string         `json:"shift_id,omitempty"`
	WorksiteID          string         `json:"worksite_id,omitempty"`
	WorkDate            string         `json:"work_date"`
	Direction           string         `json:"direction"`
	ClockedAt           time.Time      `json:"clocked_at"`
	Latitude            float64        `json:"latitude"`
	Longitude           float64        `json:"longitude"`
	AccuracyMeters      float64        `json:"accuracy_meters,omitempty"`
	DistanceMeters      float64        `json:"distance_meters,omitempty"`
	RecordStatus        string         `json:"record_status"`
	RejectionReason     string         `json:"rejection_reason,omitempty"`
	Source              string         `json:"source"`
	DeviceID            string         `json:"device_id,omitempty"`
	DeviceInfo          map[string]any `json:"device_info,omitempty"`
	CorrectionRequestID string         `json:"correction_request_id,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
}

// AttendanceDailySummary 定義考勤日彙總的資料結構。
type AttendanceDailySummary struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	EmployeeID  string    `json:"employee_id"`
	WorkDate    string    `json:"work_date"`
	ShiftStart  string    `json:"shift_start,omitempty"`
	ShiftEnd    string    `json:"shift_end,omitempty"`
	ShiftHours  float64   `json:"shift_hours,omitempty"`
	DailyHours  float64   `json:"daily_hours,omitempty"`
	ClockHours  float64   `json:"clock_hours,omitempty"`
	Source      string    `json:"source"`
	ExternalRef string    `json:"external_ref"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AttendanceCorrectionRequest 定義考勤 correction 請求的資料結構。
type AttendanceCorrectionRequest struct {
	ID                  string     `json:"id"`
	TenantID            string     `json:"tenant_id"`
	EmployeeID          string     `json:"employee_id"`
	Direction           string     `json:"direction"`
	RequestedClockedAt  time.Time  `json:"requested_clocked_at"`
	WorkDate            string     `json:"work_date"`
	Reason              string     `json:"reason"`
	Status              string     `json:"status"`
	FormInstanceID      string     `json:"form_instance_id,omitempty"`
	ClockRecordID       string     `json:"clock_record_id,omitempty"`
	ReviewedByAccountID string     `json:"reviewed_by_account_id,omitempty"`
	ReviewReason        string     `json:"review_reason,omitempty"`
	ReviewedAt          *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// AttendanceClockStatus 定義考勤打卡狀態的資料結構。
type AttendanceClockStatus struct {
	EmployeeID string                     `json:"employee_id"`
	WorkDate   string                     `json:"work_date"`
	Assignment *AttendanceShiftAssignment `json:"assignment,omitempty"`
	Shift      *AttendanceShift           `json:"shift,omitempty"`
	Worksite   *AttendanceWorksite        `json:"worksite,omitempty"`
	ClockIn    *AttendanceClockRecord     `json:"clock_in,omitempty"`
	ClockOut   *AttendanceClockRecord     `json:"clock_out,omitempty"`
	NextAction string                     `json:"next_action"`
}

// AttendancePolicyResponse 定義考勤政策回應的資料結構。
type AttendancePolicyResponse struct {
	WorkTime   AttendancePolicyWorkTime `json:"work_time"`
	LeaveTypes []AttendanceLeaveType    `json:"leave_types"`
}

// AttendancePolicy 定義考勤政策的資料結構。
type AttendancePolicy struct {
	ID                 string                   `json:"id"`
	TenantID           string                   `json:"tenant_id"`
	WorkTime           AttendancePolicyWorkTime `json:"work_time"`
	LeaveTypes         []AttendanceLeaveType    `json:"leave_types"`
	UpdatedByAccountID string                   `json:"updated_by_account_id,omitempty"`
	CreatedAt          time.Time                `json:"created_at"`
	UpdatedAt          time.Time                `json:"updated_at"`
}

// AttendancePolicyWorkTime 定義考勤政策 work 時間的資料結構。
type AttendancePolicyWorkTime struct {
	StandardStart     string   `json:"standard_start"`
	StandardEnd       string   `json:"standard_end"`
	BreakStart        string   `json:"break_start"`
	BreakEnd          string   `json:"break_end"`
	Weekend           string   `json:"weekend"`
	CycleStart        string   `json:"cycle_start"`
	CycleEnd          string   `json:"cycle_end"`
	TimeOptions       []string `json:"time_options"`
	WeekendOptions    []string `json:"weekend_options"`
	CycleStartOptions []string `json:"cycle_start_options"`
	CycleEndOptions   []string `json:"cycle_end_options"`
}

// AttendanceLeaveType 定義考勤請假 type 的資料結構。
type AttendanceLeaveType struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Quota string `json:"quota"`
	Rule  string `json:"rule"`
	Proof string `json:"proof"`
}

// UpdateAttendancePolicyInput 定義考勤政策輸入的資料結構。
type UpdateAttendancePolicyInput struct {
	WorkTime   AttendancePolicyWorkTime `json:"work_time"`
	LeaveTypes []AttendanceLeaveType    `json:"leave_types"`
}

// Validate 驗證目前流程。
func (in UpdateAttendancePolicyInput) Validate() error {
	if strings.TrimSpace(in.WorkTime.StandardStart) == "" || strings.TrimSpace(in.WorkTime.StandardEnd) == "" {
		return BadRequest("standard_start and standard_end are required")
	}
	if strings.TrimSpace(in.WorkTime.BreakStart) == "" || strings.TrimSpace(in.WorkTime.BreakEnd) == "" {
		return BadRequest("break_start and break_end are required")
	}
	if strings.TrimSpace(in.WorkTime.Weekend) == "" || strings.TrimSpace(in.WorkTime.CycleStart) == "" || strings.TrimSpace(in.WorkTime.CycleEnd) == "" {
		return BadRequest("weekend, cycle_start and cycle_end are required")
	}
	if len(in.LeaveTypes) == 0 {
		return BadRequest("leave_types is required")
	}
	for _, item := range in.LeaveTypes {
		if strings.TrimSpace(item.Code) == "" || strings.TrimSpace(item.Name) == "" {
			return BadRequest("leave type code and name are required")
		}
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

// CreateAttendanceShiftInput 定義考勤班別輸入的資料結構。
type CreateAttendanceShiftInput struct {
	Name                   string `json:"name"`
	ClockInStart           string `json:"clock_in_start"`
	ClockInEnd             string `json:"clock_in_end"`
	ClockOutStart          string `json:"clock_out_start"`
	ClockOutEnd            string `json:"clock_out_end"`
	LateGraceMinutes       int    `json:"late_grace_minutes,omitempty"`
	EarlyLeaveGraceMinutes int    `json:"early_leave_grace_minutes,omitempty"`
	Status                 string `json:"status,omitempty"`
}

// UpdateAttendanceShiftInput 定義考勤班別輸入的資料結構。
type UpdateAttendanceShiftInput struct {
	ID                     string  `json:"id"`
	Name                   *string `json:"name,omitempty"`
	ClockInStart           *string `json:"clock_in_start,omitempty"`
	ClockInEnd             *string `json:"clock_in_end,omitempty"`
	ClockOutStart          *string `json:"clock_out_start,omitempty"`
	ClockOutEnd            *string `json:"clock_out_end,omitempty"`
	LateGraceMinutes       *int    `json:"late_grace_minutes,omitempty"`
	EarlyLeaveGraceMinutes *int    `json:"early_leave_grace_minutes,omitempty"`
	Status                 *string `json:"status,omitempty"`
}

// CreateAttendanceShiftAssignmentInput 定義考勤班別指派輸入的資料結構。
type CreateAttendanceShiftAssignmentInput struct {
	EmployeeID    string `json:"employee_id"`
	ShiftID       string `json:"shift_id"`
	WorksiteID    string `json:"worksite_id"`
	EffectiveFrom string `json:"effective_from"`
	EffectiveTo   string `json:"effective_to,omitempty"`
	Status        string `json:"status,omitempty"`
}

// CreateAttendanceClockRecordInput 定義考勤打卡 record 輸入的資料結構。
type CreateAttendanceClockRecordInput struct {
	EmployeeID     string         `json:"employee_id,omitempty"`
	Direction      string         `json:"direction"`
	Latitude       float64        `json:"latitude"`
	Longitude      float64        `json:"longitude"`
	AccuracyMeters float64        `json:"accuracy_meters,omitempty"`
	LocationSource string         `json:"location_source,omitempty"`
	DeviceID       string         `json:"device_id,omitempty"`
	DeviceInfo     map[string]any `json:"device_info,omitempty"`
}

// AttendanceClockRecordQuery 定義考勤打卡 record 查詢的資料結構。
type AttendanceClockRecordQuery struct {
	EmployeeID   string `json:"employee_id,omitempty"`
	FromDate     string `json:"from_date,omitempty"`
	ToDate       string `json:"to_date,omitempty"`
	Direction    string `json:"direction,omitempty"`
	RecordStatus string `json:"record_status,omitempty"`
	Source       string `json:"source,omitempty"`
}

// AttendanceDailySummaryQuery 定義考勤日彙總查詢的資料結構。
type AttendanceDailySummaryQuery struct {
	EmployeeID string `json:"employee_id,omitempty"`
	FromDate   string `json:"from_date,omitempty"`
	ToDate     string `json:"to_date,omitempty"`
	Source     string `json:"source,omitempty"`
}

// EHRMSAttendanceRecord 表示 eHRMS 考勤 record。
type EHRMSAttendanceRecord map[string]string

// EHRMSAttendanceSyncInput 定義 eHRMS 考勤 sync 輸入的資料結構。
type EHRMSAttendanceSyncInput struct {
	Mode  string `json:"mode,omitempty"`
	Since string `json:"since,omitempty"`
}

// EHRMSAttendanceSyncResponse 定義 eHRMS 考勤 sync 回應的資料結構。
type EHRMSAttendanceSyncResponse struct {
	Fetched   int                   `json:"fetched"`
	Created   int                   `json:"created"`
	Updated   int                   `json:"updated"`
	Skipped   int                   `json:"skipped"`
	Failed    int                   `json:"failed"`
	Mode      string                `json:"mode"`
	Since     string                `json:"since,omitempty"`
	Results   []BatchEmployeeResult `json:"results,omitempty"`
	RowErrors []RowError            `json:"row_errors,omitempty"`
}

// CreateAttendanceCorrectionInput 定義考勤 correction 輸入的資料結構。
type CreateAttendanceCorrectionInput struct {
	EmployeeID         string `json:"employee_id,omitempty"`
	Direction          string `json:"direction"`
	RequestedClockedAt string `json:"requested_clocked_at"`
	Reason             string `json:"reason"`
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
