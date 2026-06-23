package repository

import (
	"context"
	"time"

	"nexus-pro-be/internal/domain"
)

// AttendanceStore persists leave balances and leave requests.
type AttendanceStore interface {
	UpsertLeaveBalance(context.Context, domain.LeaveBalance) error
	GetLeaveBalance(ctx context.Context, tenantID, id string) (domain.LeaveBalance, bool, error)
	ListLeaveBalances(ctx context.Context, tenantID string) ([]domain.LeaveBalance, error)
	ReserveLeaveBalance(ctx context.Context, tenantID, employeeID, leaveType string, hours float64, updatedAt time.Time) (domain.LeaveBalance, bool, bool, error)

	UpsertLeaveRequest(context.Context, domain.LeaveRequest) error
	GetLeaveRequest(ctx context.Context, tenantID, id string) (domain.LeaveRequest, bool, error)
	ListLeaveRequests(ctx context.Context, tenantID string) ([]domain.LeaveRequest, error)
}
