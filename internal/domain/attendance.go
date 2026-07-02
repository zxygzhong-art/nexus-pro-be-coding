package domain

import (
	"strings"
	"time"
)

// LeaveBalance tracks remaining leave hours for one employee and leave type.
type LeaveBalance struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	EmployeeID     string    `json:"employee_id"`
	LeaveType      string    `json:"leave_type"`
	RemainingHours float64   `json:"remaining_hours"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// LeaveRequest records a requested leave period and its workflow status.
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

// CreateLeaveRequestInput carries the payload for creating a leave request.
type CreateLeaveRequestInput struct {
	EmployeeID string  `json:"employee_id,omitempty"`
	LeaveType  string  `json:"leave_type"`
	StartAt    string  `json:"start_at"`
	EndAt      string  `json:"end_at"`
	Hours      float64 `json:"hours"`
	Reason     string  `json:"reason,omitempty"`
}

// LeaveRequestQuery filters leave requests for scoped list and reporting reads.
type LeaveRequestQuery struct {
	EmployeeIDs []string `json:"employee_ids,omitempty"`
	Status      string   `json:"status,omitempty"`
	FromDate    string   `json:"from_date,omitempty"`
	ToDate      string   `json:"to_date,omitempty"`
}

// AttendanceWorksite defines one allowed geographic clock-in area.
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

// AttendanceShift defines clock-in and clock-out windows, including overnight shifts.
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

// AttendanceShiftAssignment binds one employee to a shift and worksite.
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

// AttendanceClockRecord stores one accepted or rejected clock attempt.
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

// AttendanceCorrectionRequest records a manual clock correction workflow.
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

// AttendanceClockStatus summarizes today's clock state for the current account.
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

// AttendancePolicyResponse describes the current tenant attendance policy projection.
type AttendancePolicyResponse struct {
	WorkTime   AttendancePolicyWorkTime `json:"work_time"`
	LeaveTypes []AttendanceLeaveType    `json:"leave_types"`
}

// AttendancePolicy stores the tenant-level policy behind the attendance settings page.
type AttendancePolicy struct {
	ID                 string                   `json:"id"`
	TenantID           string                   `json:"tenant_id"`
	WorkTime           AttendancePolicyWorkTime `json:"work_time"`
	LeaveTypes         []AttendanceLeaveType    `json:"leave_types"`
	UpdatedByAccountID string                   `json:"updated_by_account_id,omitempty"`
	CreatedAt          time.Time                `json:"created_at"`
	UpdatedAt          time.Time                `json:"updated_at"`
}

// AttendancePolicyWorkTime contains working hours, weekends, and calculation-cycle options.
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

// AttendanceLeaveType describes one leave type shown in the attendance policy page.
type AttendanceLeaveType struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Quota string `json:"quota"`
	Rule  string `json:"rule"`
	Proof string `json:"proof"`
}

// UpdateAttendancePolicyInput carries the editable attendance settings from workspace.
type UpdateAttendancePolicyInput struct {
	WorkTime   AttendancePolicyWorkTime `json:"work_time"`
	LeaveTypes []AttendanceLeaveType    `json:"leave_types"`
}

// Validate rejects incomplete attendance policies before service-layer normalization.
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

// CreateAttendanceWorksiteInput carries the payload for creating a worksite.
type CreateAttendanceWorksiteInput struct {
	Name         string  `json:"name"`
	Address      string  `json:"address,omitempty"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	RadiusMeters int     `json:"radius_meters"`
	Status       string  `json:"status,omitempty"`
}

// UpdateAttendanceWorksiteInput carries partial updates for a worksite.
type UpdateAttendanceWorksiteInput struct {
	ID           string   `json:"id"`
	Name         *string  `json:"name,omitempty"`
	Address      *string  `json:"address,omitempty"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	RadiusMeters *int     `json:"radius_meters,omitempty"`
	Status       *string  `json:"status,omitempty"`
}

// CreateAttendanceShiftInput carries the payload for creating a shift.
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

// UpdateAttendanceShiftInput carries partial updates for a shift.
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

// CreateAttendanceShiftAssignmentInput binds an employee to a shift and worksite.
type CreateAttendanceShiftAssignmentInput struct {
	EmployeeID    string `json:"employee_id"`
	ShiftID       string `json:"shift_id"`
	WorksiteID    string `json:"worksite_id"`
	EffectiveFrom string `json:"effective_from"`
	EffectiveTo   string `json:"effective_to,omitempty"`
	Status        string `json:"status,omitempty"`
}

// CreateAttendanceClockRecordInput carries location evidence for a clock attempt.
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

// AttendanceClockRecordQuery filters clock records before pagination.
type AttendanceClockRecordQuery struct {
	EmployeeID   string `json:"employee_id,omitempty"`
	FromDate     string `json:"from_date,omitempty"`
	ToDate       string `json:"to_date,omitempty"`
	Direction    string `json:"direction,omitempty"`
	RecordStatus string `json:"record_status,omitempty"`
	Source       string `json:"source,omitempty"`
}

// CreateAttendanceCorrectionInput carries one manual correction request.
type CreateAttendanceCorrectionInput struct {
	EmployeeID         string `json:"employee_id,omitempty"`
	Direction          string `json:"direction"`
	RequestedClockedAt string `json:"requested_clocked_at"`
	Reason             string `json:"reason"`
}

// ReviewAttendanceCorrectionInput carries approval or rejection notes.
type ReviewAttendanceCorrectionInput struct {
	Reason string `json:"reason,omitempty"`
}

// AttendanceCorrectionQuery filters correction requests before pagination.
type AttendanceCorrectionQuery struct {
	EmployeeID string `json:"employee_id,omitempty"`
	FromDate   string `json:"from_date,omitempty"`
	ToDate     string `json:"to_date,omitempty"`
	Status     string `json:"status,omitempty"`
	Direction  string `json:"direction,omitempty"`
}
