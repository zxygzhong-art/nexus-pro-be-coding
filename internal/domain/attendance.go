package domain

import "time"

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
