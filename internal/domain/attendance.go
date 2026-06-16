package domain

import "time"

type LeaveBalance struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	EmployeeID     string    `json:"employee_id"`
	LeaveType      string    `json:"leave_type"`
	RemainingHours float64   `json:"remaining_hours"`
	UpdatedAt      time.Time `json:"updated_at"`
}

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
