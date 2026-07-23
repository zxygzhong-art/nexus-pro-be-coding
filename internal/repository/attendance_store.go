package repository

import (
	"context"
	"time"

	"nexus-pro-api/internal/domain"
)

// AttendanceStore 定義考勤儲存層的行為契約。
type AttendanceStore interface {
	InsertAttendancePolicyVersion(context.Context, domain.AttendancePolicy) error
	GetAttendancePolicy(ctx context.Context, tenantID string) (domain.AttendancePolicy, bool, error)
	GetAttendancePolicyAsOf(ctx context.Context, tenantID string, asOf time.Time) (domain.AttendancePolicy, bool, error)
	// ListLeaveTypes returns tenant leave_types rows ordered by display_order.
	ListLeaveTypes(ctx context.Context, tenantID string) ([]domain.LeaveType, error)
	// UpsertLeaveType writes one EHRMS-synced leave catalog row (preserves status on update).
	UpsertLeaveType(context.Context, domain.LeaveType) error
	// UpsertLeaveTypeEnabled writes leave_types.status (active/inactive) for one existing ID.
	UpsertLeaveTypeEnabled(ctx context.Context, tenantID, id string, enabled bool, updatedByAccountID string, updatedAt time.Time) error
	// DeactivateMissingLeaveTypes marks EHRMS-sourced leave types not in activeCodes as inactive.
	DeactivateMissingLeaveTypes(ctx context.Context, tenantID string, activeCodes []string, updatedAt time.Time) (int64, error)

	UpsertLeaveBalance(context.Context, domain.LeaveBalance) error
	EnsureLocalLeaveBalanceAnchor(context.Context, domain.LeaveBalance) (domain.LeaveBalance, error)
	GetLeaveBalance(ctx context.Context, tenantID, id string) (domain.LeaveBalance, bool, error)
	GetLeaveBalanceForOverlay(ctx context.Context, tenantID, employeeID, leaveTypeID string, asOf time.Time) (domain.LeaveBalance, bool, error)
	ListLeaveBalancesForOverlay(ctx context.Context, tenantID, employeeID, leaveTypeID string, asOf time.Time) ([]domain.LeaveBalance, error)
	ListLeaveBalances(ctx context.Context, tenantID string) ([]domain.LeaveBalance, error)
	AppendLeaveBalanceEntry(context.Context, domain.LeaveBalanceEntry) (bool, error)
	AppendStandaloneLeaveBalanceEntry(context.Context, domain.LeaveBalanceEntry) (bool, error)
	ListLeaveBalanceEntries(ctx context.Context, tenantID string) ([]domain.LeaveBalanceEntry, error)
	ListLeaveBalanceEntriesByBalance(ctx context.Context, tenantID, balanceID string) ([]domain.LeaveBalanceEntry, error)
	UpsertLeaveRecord(context.Context, domain.LeaveRecord) error
	GetLeaveRecord(ctx context.Context, tenantID, id string) (domain.LeaveRecord, bool, error)
	ListLeaveRecords(ctx context.Context, tenantID string) ([]domain.LeaveRecord, error)
	ListActiveLeaveRecordsByQuery(ctx context.Context, tenantID string, employeeIDs []string, fromAt, toAt time.Time) ([]domain.LeaveRecord, error)

	UpsertLeaveRequest(context.Context, domain.LeaveRequest) error
	GetLeaveRequest(ctx context.Context, tenantID, id string) (domain.LeaveRequest, bool, error)
	GetLeaveRequestByFormInstanceID(ctx context.Context, tenantID, formInstanceID string) (domain.LeaveRequest, bool, error)
	ListLeaveRequests(ctx context.Context, tenantID string) ([]domain.LeaveRequest, error)
	ListLeaveRequestsByQuery(ctx context.Context, tenantID string, query domain.LeaveRequestQuery) ([]domain.LeaveRequest, error)
	ListLeaveRequestPageByQuery(ctx context.Context, tenantID string, query domain.LeaveRequestQuery, page domain.PageRequest) ([]domain.LeaveRequest, int, error)
	UpsertAttendanceWorksite(context.Context, domain.AttendanceWorksite) error
	GetAttendanceWorksite(ctx context.Context, tenantID, id string) (domain.AttendanceWorksite, bool, error)
	ListAttendanceWorksites(ctx context.Context, tenantID string) ([]domain.AttendanceWorksite, error)

	UpsertAttendanceClockRecord(context.Context, domain.AttendanceClockRecord) error
	GetAttendanceClockRecordByClientEventID(ctx context.Context, tenantID, clientEventID string) (domain.AttendanceClockRecord, bool, error)
	GetEarliestAcceptedAttendanceClockIn(ctx context.Context, tenantID, employeeID, workDate string) (domain.AttendanceClockRecord, bool, error)
	GetLatestAcceptedAttendanceClockOut(ctx context.Context, tenantID, employeeID, workDate string) (domain.AttendanceClockRecord, bool, error)
	GetLatestAcceptedAttendanceClockRecord(ctx context.Context, tenantID, employeeID, workDate string) (domain.AttendanceClockRecord, bool, error)
	ListAttendanceClockRecords(ctx context.Context, tenantID string, query domain.AttendanceClockRecordQuery) ([]domain.AttendanceClockRecord, error)

	UpsertAttendanceDailySummary(context.Context, domain.AttendanceDailySummary) error
	GetAttendanceDailySummaryByExternalRef(ctx context.Context, tenantID, externalRef string) (domain.AttendanceDailySummary, bool, error)
	GetAttendanceDailySummaryByEmployeeDate(ctx context.Context, tenantID, employeeID, workDate string) (domain.AttendanceDailySummary, bool, error)
	ListAttendanceDailySummaries(ctx context.Context, tenantID string, query domain.AttendanceDailySummaryQuery) ([]domain.AttendanceDailySummary, error)
	UpsertAttendanceDayProjection(context.Context, domain.AttendanceDayProjection) error
	GetAttendanceDayProjection(ctx context.Context, tenantID, employeeID, workDate string) (domain.AttendanceDayProjection, bool, error)
	ListAttendanceDayProjections(ctx context.Context, tenantID string, employeeIDs []string, fromDate, toDate string) ([]domain.AttendanceDayProjection, error)

	UpsertAttendanceCorrectionRequest(context.Context, domain.AttendanceCorrectionRequest) error
	GetAttendanceCorrectionRequest(ctx context.Context, tenantID, id string) (domain.AttendanceCorrectionRequest, bool, error)
	GetAttendanceCorrectionRequestForUpdate(ctx context.Context, tenantID, id string) (domain.AttendanceCorrectionRequest, bool, error)
	ClaimAttendanceCorrectionReview(ctx context.Context, tenantID, formInstanceID, reviewerID string, claimedAt time.Time) (domain.AttendanceCorrectionRequest, bool, error)
	GetAttendanceCorrectionRequestByFormInstanceID(ctx context.Context, tenantID, formInstanceID string) (domain.AttendanceCorrectionRequest, bool, error)
	ListAttendanceCorrectionRequests(ctx context.Context, tenantID string, query domain.AttendanceCorrectionQuery) ([]domain.AttendanceCorrectionRequest, error)

	UpsertOvertimeRequest(context.Context, domain.OvertimeRequest) error
	GetOvertimeRequest(ctx context.Context, tenantID, id string) (domain.OvertimeRequest, bool, error)
	GetOvertimeRequestByFormInstanceID(ctx context.Context, tenantID, formInstanceID string) (domain.OvertimeRequest, bool, error)
	ListOvertimeRequestsByQuery(ctx context.Context, tenantID string, query domain.OvertimeRequestQuery) ([]domain.OvertimeRequest, error)
}
