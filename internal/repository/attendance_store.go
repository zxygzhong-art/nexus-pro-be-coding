package repository

import (
	"context"
	"time"

	"nexus-pro-be/internal/domain"
)

// AttendanceStore 定義考勤儲存層的行為契約。
type AttendanceStore interface {
	UpsertAttendancePolicy(context.Context, domain.AttendancePolicy) error
	GetAttendancePolicy(ctx context.Context, tenantID string) (domain.AttendancePolicy, bool, error)

	UpsertLeaveBalance(context.Context, domain.LeaveBalance) error
	GetLeaveBalance(ctx context.Context, tenantID, id string) (domain.LeaveBalance, bool, error)
	ListLeaveBalances(ctx context.Context, tenantID string) ([]domain.LeaveBalance, error)
	ReserveLeaveBalance(ctx context.Context, tenantID, employeeID, leaveType string, hours float64, updatedAt time.Time) (domain.LeaveBalance, bool, bool, error)
	ReleaseLeaveBalance(ctx context.Context, tenantID, employeeID, leaveType string, hours float64, updatedAt time.Time) (domain.LeaveBalance, bool, error)

	UpsertLeaveRequest(context.Context, domain.LeaveRequest) error
	GetLeaveRequest(ctx context.Context, tenantID, id string) (domain.LeaveRequest, bool, error)
	GetLeaveRequestByFormInstanceID(ctx context.Context, tenantID, formInstanceID string) (domain.LeaveRequest, bool, error)
	ListLeaveRequests(ctx context.Context, tenantID string) ([]domain.LeaveRequest, error)
	ListLeaveRequestsByQuery(ctx context.Context, tenantID string, query domain.LeaveRequestQuery) ([]domain.LeaveRequest, error)
	ListLeaveRequestPageByQuery(ctx context.Context, tenantID string, query domain.LeaveRequestQuery, page domain.PageRequest) ([]domain.LeaveRequest, int, error)

	UpsertAttendanceWorksite(context.Context, domain.AttendanceWorksite) error
	GetAttendanceWorksite(ctx context.Context, tenantID, id string) (domain.AttendanceWorksite, bool, error)
	ListAttendanceWorksites(ctx context.Context, tenantID string) ([]domain.AttendanceWorksite, error)

	UpsertAttendanceShift(context.Context, domain.AttendanceShift) error
	GetAttendanceShift(ctx context.Context, tenantID, id string) (domain.AttendanceShift, bool, error)
	ListAttendanceShifts(ctx context.Context, tenantID string) ([]domain.AttendanceShift, error)

	UpsertAttendanceShiftAssignment(context.Context, domain.AttendanceShiftAssignment) error
	GetAttendanceShiftAssignment(ctx context.Context, tenantID, id string) (domain.AttendanceShiftAssignment, bool, error)
	ListAttendanceShiftAssignments(ctx context.Context, tenantID string) ([]domain.AttendanceShiftAssignment, error)
	FindEffectiveAttendanceShiftAssignment(ctx context.Context, tenantID, employeeID string, at time.Time) (domain.AttendanceShiftAssignment, bool, error)

	UpsertAttendanceClockRecord(context.Context, domain.AttendanceClockRecord) error
	GetAttendanceClockRecord(ctx context.Context, tenantID, id string) (domain.AttendanceClockRecord, bool, error)
	GetAcceptedAttendanceClockRecord(ctx context.Context, tenantID, employeeID, workDate, direction string) (domain.AttendanceClockRecord, bool, error)
	ListAttendanceClockRecords(ctx context.Context, tenantID string, query domain.AttendanceClockRecordQuery) ([]domain.AttendanceClockRecord, error)

	UpsertAttendanceCorrectionRequest(context.Context, domain.AttendanceCorrectionRequest) error
	GetAttendanceCorrectionRequest(ctx context.Context, tenantID, id string) (domain.AttendanceCorrectionRequest, bool, error)
	ListAttendanceCorrectionRequests(ctx context.Context, tenantID string, query domain.AttendanceCorrectionQuery) ([]domain.AttendanceCorrectionRequest, error)
}
