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
	ListAttendanceShiftAssignments(ctx context.Context, tenantID string) ([]domain.AttendanceShiftAssignment, error)
	FindEffectiveAttendanceShiftAssignment(ctx context.Context, tenantID, employeeID string, at time.Time) (domain.AttendanceShiftAssignment, bool, error)

	UpsertAttendanceClockRecord(context.Context, domain.AttendanceClockRecord) error
	GetAcceptedAttendanceClockRecord(ctx context.Context, tenantID, employeeID, workDate, direction string) (domain.AttendanceClockRecord, bool, error)
	ListAttendanceClockRecords(ctx context.Context, tenantID string, query domain.AttendanceClockRecordQuery) ([]domain.AttendanceClockRecord, error)

	UpsertAttendanceDailySummary(context.Context, domain.AttendanceDailySummary) error
	GetAttendanceDailySummaryByExternalRef(ctx context.Context, tenantID, externalRef string) (domain.AttendanceDailySummary, bool, error)
	GetAttendanceDailySummaryByEmployeeDate(ctx context.Context, tenantID, employeeID, workDate string) (domain.AttendanceDailySummary, bool, error)
	ListAttendanceDailySummaries(ctx context.Context, tenantID string, query domain.AttendanceDailySummaryQuery) ([]domain.AttendanceDailySummary, error)

	UpsertAttendanceCorrectionRequest(context.Context, domain.AttendanceCorrectionRequest) error
	GetAttendanceCorrectionRequest(ctx context.Context, tenantID, id string) (domain.AttendanceCorrectionRequest, bool, error)
	GetAttendanceCorrectionRequestByFormInstanceID(ctx context.Context, tenantID, formInstanceID string) (domain.AttendanceCorrectionRequest, bool, error)
	ListAttendanceCorrectionRequests(ctx context.Context, tenantID string, query domain.AttendanceCorrectionQuery) ([]domain.AttendanceCorrectionRequest, error)

	UpsertOvertimeRequest(context.Context, domain.OvertimeRequest) error
	GetOvertimeRequest(ctx context.Context, tenantID, id string) (domain.OvertimeRequest, bool, error)
	GetOvertimeRequestByFormInstanceID(ctx context.Context, tenantID, formInstanceID string) (domain.OvertimeRequest, bool, error)
	ListOvertimeRequestsByQuery(ctx context.Context, tenantID string, query domain.OvertimeRequestQuery) ([]domain.OvertimeRequest, error)
}
