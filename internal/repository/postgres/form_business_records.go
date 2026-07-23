package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"nexus-pro-api/internal/domain"
	sqlc "nexus-pro-api/internal/platform/postgres/db"
)

const (
	businessTypeLeave           = "attendance.leave"
	businessTypeClockCorrection = "attendance.clock_correction"
	businessTypeOvertime        = "attendance.overtime"
)

type formBusinessRecordView struct {
	record         sqlc.FormBusinessRecord
	formStatus     string
	formApprovedBy string
	formUpdatedAt  time.Time
}

func businessView(v sqlc.GetFormBusinessRecordRow) formBusinessRecordView {
	return formBusinessRecordView{record: v.FormBusinessRecord, formStatus: v.FormStatus, formApprovedBy: v.FormApprovedBy, formUpdatedAt: timeFrom(v.FormUpdatedAt)}
}

func businessViewForUpdate(v sqlc.GetFormBusinessRecordForUpdateRow) formBusinessRecordView {
	return formBusinessRecordView{record: v.FormBusinessRecord, formStatus: v.FormStatus, formApprovedBy: v.FormApprovedBy, formUpdatedAt: timeFrom(v.FormUpdatedAt)}
}

func businessViewByForm(v sqlc.GetFormBusinessRecordByFormTypeRow) formBusinessRecordView {
	return formBusinessRecordView{record: v.FormBusinessRecord, formStatus: v.FormStatus, formApprovedBy: v.FormApprovedBy, formUpdatedAt: timeFrom(v.FormUpdatedAt)}
}

func businessViews(items []sqlc.ListFormBusinessRecordsByTypeRow) []formBusinessRecordView {
	out := make([]formBusinessRecordView, 0, len(items))
	for _, item := range items {
		out = append(out, formBusinessRecordView{
			record: item.FormBusinessRecord, formStatus: item.FormStatus,
			formApprovedBy: item.FormApprovedBy, formUpdatedAt: timeFrom(item.FormUpdatedAt),
		})
	}
	return out
}

func businessPageViews(items []sqlc.ListFormBusinessRecordPageByTypeRow) []formBusinessRecordView {
	out := make([]formBusinessRecordView, 0, len(items))
	for _, item := range items {
		out = append(out, formBusinessRecordView{
			record: item.FormBusinessRecord, formStatus: item.FormStatus,
			formApprovedBy: item.FormApprovedBy, formUpdatedAt: timeFrom(item.FormUpdatedAt),
		})
	}
	return out
}

func (s *Store) upsertFormBusinessRecord(execCtx context.Context, params sqlc.UpsertFormBusinessRecordParams) error {
	if params.SchemaVersion <= 0 {
		params.SchemaVersion = 1
	}
	if params.HandlerVersion <= 0 {
		params.HandlerVersion = 1
	}
	if params.EffectStatus == "" {
		params.EffectStatus = "not_applied"
	}
	if len(params.Data) == 0 {
		params.Data = []byte("{}")
	}
	if len(params.Result) == 0 {
		params.Result = []byte("{}")
	}
	if len(params.LastError) == 0 {
		params.LastError = []byte("{}")
	}
	_, err := s.q.UpsertFormBusinessRecord(tenantContext(execCtx, params.TenantID), params)
	return err
}

func (s *Store) getFormBusinessRecord(execCtx context.Context, tenantID, id string, forUpdate bool) (formBusinessRecordView, bool, error) {
	if forUpdate {
		v, err := s.q.GetFormBusinessRecordForUpdate(tenantContext(execCtx, tenantID), sqlc.GetFormBusinessRecordForUpdateParams{TenantID: tenantID, ID: id})
		if isNotFound(err) {
			return formBusinessRecordView{}, false, nil
		}
		if err != nil {
			return formBusinessRecordView{}, false, err
		}
		return businessViewForUpdate(v), true, nil
	}
	v, err := s.q.GetFormBusinessRecord(tenantContext(execCtx, tenantID), sqlc.GetFormBusinessRecordParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return formBusinessRecordView{}, false, nil
	}
	if err != nil {
		return formBusinessRecordView{}, false, err
	}
	return businessView(v), true, nil
}

func (s *Store) getFormBusinessRecordByForm(execCtx context.Context, tenantID, formInstanceID, businessType string) (formBusinessRecordView, bool, error) {
	v, err := s.q.GetFormBusinessRecordByFormType(tenantContext(execCtx, tenantID), sqlc.GetFormBusinessRecordByFormTypeParams{
		TenantID: tenantID, FormInstanceID: formInstanceID, BusinessType: businessType,
	})
	if isNotFound(err) {
		return formBusinessRecordView{}, false, nil
	}
	if err != nil {
		return formBusinessRecordView{}, false, err
	}
	return businessViewByForm(v), true, nil
}

func (s *Store) listFormBusinessRecords(execCtx context.Context, tenantID, businessType, status string, employeeIDs []string, fromDate, toDate string) ([]formBusinessRecordView, error) {
	items, err := s.q.ListFormBusinessRecordsByType(tenantContext(execCtx, tenantID), sqlc.ListFormBusinessRecordsByTypeParams{
		TenantID: tenantID, BusinessType: businessType, Status: strings.TrimSpace(status), SubjectEmployeeIds: employeeIDs,
		FromDate: strings.TrimSpace(fromDate), ToDate: strings.TrimSpace(toDate),
	})
	if err != nil {
		return nil, err
	}
	return businessViews(items), nil
}

func (s *Store) listFormBusinessRecordPage(execCtx context.Context, tenantID, businessType, status string, employeeIDs []string, fromDate, toDate string, page domain.PageRequest) ([]formBusinessRecordView, int, error) {
	tenantCtx := tenantContext(execCtx, tenantID)
	filter := sqlc.CountFormBusinessRecordsByTypeParams{
		TenantID: tenantID, BusinessType: businessType, Status: strings.TrimSpace(status), SubjectEmployeeIds: employeeIDs,
		FromDate: strings.TrimSpace(fromDate), ToDate: strings.TrimSpace(toDate),
	}
	total, err := s.q.CountFormBusinessRecordsByType(tenantCtx, filter)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.q.ListFormBusinessRecordPageByType(tenantCtx, sqlc.ListFormBusinessRecordPageByTypeParams{
		TenantID: filter.TenantID, BusinessType: filter.BusinessType, Status: filter.Status,
		SubjectEmployeeIds: filter.SubjectEmployeeIds, FromDate: filter.FromDate, ToDate: filter.ToDate,
		Sort: page.Sort, OffsetCount: int32((page.Page - 1) * page.PageSize), LimitCount: int32(page.PageSize),
	})
	if err != nil {
		return nil, 0, err
	}
	return businessPageViews(items), int(total), nil
}

func businessEffectState(current string, updatedAt time.Time) (string, pgtype.Timestamptz) {
	current = strings.ToLower(strings.TrimSpace(current))
	if current == "" {
		current = "not_applied"
	}
	if current == "applied" {
		return current, timestamptz(updatedAt)
	}
	return current, pgtype.Timestamptz{}
}

func businessWorkflowStatus(businessType, status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "approved", "rejected", "cancelled":
		return status
	case "returned":
		return "rejected"
	}
	if businessType == businessTypeClockCorrection {
		return "pending"
	}
	return "pending_approval"
}

func businessString(data map[string]any, key string) string {
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func businessInt(data map[string]any, key string) int {
	value, ok := data[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		parsed, _ := strconv.Atoi(typed)
		return parsed
	default:
		return 0
	}
}

func businessFloat(data map[string]any, key string) float64 {
	value, ok := data[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case string:
		parsed, _ := strconv.ParseFloat(typed, 64)
		return parsed
	default:
		return 0
	}
}

func businessTime(data map[string]any, key string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, businessString(data, key))
	return parsed
}

func businessTimePtr(data map[string]any, key string) *time.Time {
	parsed := businessTime(data, key)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

func businessMap(data map[string]any, key string) map[string]any {
	value, _ := data[key].(map[string]any)
	if value == nil {
		return map[string]any{}
	}
	return value
}

func effectAppliedAt(v sqlc.FormBusinessRecord) *time.Time {
	return timePtrFrom(v.AppliedAt)
}

func formBusinessLeave(v formBusinessRecordView) domain.LeaveRequest {
	data := jsonMap(v.record.Data)
	return domain.LeaveRequest{
		ID: v.record.ID, TenantID: v.record.TenantID, EmployeeID: textFrom(v.record.SubjectEmployeeID),
		LeaveType: businessString(data, "leave_type"), LeaveTypeID: businessString(data, "leave_type_id"),
		PolicyVersion: businessInt(data, "policy_version"), RuleSnapshot: businessMap(data, "rule_snapshot"),
		EvaluationSnapshot: businessMap(data, "evaluation_snapshot"), StartAt: businessTime(data, "start_at"),
		EndAt: businessTime(data, "end_at"), RequestedMinutes: businessInt(data, "requested_minutes"),
		Reason: businessString(data, "reason"), Status: businessWorkflowStatus(businessTypeLeave, v.formStatus),
		FormInstanceID: v.record.FormInstanceID, ReconciliationStatus: businessString(data, "reconciliation_status"),
		CreatedAt: timeFrom(v.record.CreatedAt), UpdatedAt: timeFrom(v.record.UpdatedAt), EffectStatus: v.record.EffectStatus,
		EffectResult: jsonMap(v.record.Result), EffectAppliedAt: effectAppliedAt(v.record),
	}
}

func formBusinessCorrection(v formBusinessRecordView) domain.AttendanceCorrectionRequest {
	data := jsonMap(v.record.Data)
	result := jsonMap(v.record.Result)
	return domain.AttendanceCorrectionRequest{
		ID: v.record.ID, TenantID: v.record.TenantID, EmployeeID: textFrom(v.record.SubjectEmployeeID),
		Direction: businessString(data, "direction"), RequestedClockedAt: businessTime(data, "requested_clocked_at"),
		WorkDate: dateTextFrom(v.record.BusinessDate), CorrectionType: businessString(data, "correction_type"),
		TargetClockRecordID: businessString(data, "target_clock_record_id"), Reason: businessString(data, "reason"),
		Status: businessWorkflowStatus(businessTypeClockCorrection, v.formStatus), FormInstanceID: v.record.FormInstanceID,
		ReplacementClockRecordID: businessString(result, "replacement_clock_record_id"), ClockRecordID: businessString(result, "clock_record_id"),
		ReviewedByAccountID: businessString(result, "reviewed_by_account_id"), ReviewReason: businessString(result, "review_reason"),
		ReviewedAt: businessTimePtr(result, "reviewed_at"), CreatedAt: timeFrom(v.record.CreatedAt), UpdatedAt: timeFrom(v.record.UpdatedAt),
		EffectStatus: v.record.EffectStatus, EffectResult: result, EffectAppliedAt: effectAppliedAt(v.record),
	}
}

func formBusinessOvertime(v formBusinessRecordView) domain.OvertimeRequest {
	data := jsonMap(v.record.Data)
	return domain.OvertimeRequest{
		ID: v.record.ID, TenantID: v.record.TenantID, EmployeeID: textFrom(v.record.SubjectEmployeeID),
		WorkDate: dateTextFrom(v.record.BusinessDate), StartAt: businessTime(data, "start_at"), EndAt: businessTime(data, "end_at"),
		Hours: businessFloat(data, "hours"), OvertimeType: businessString(data, "overtime_type"),
		CompensationType: businessString(data, "compensation_type"), Reason: businessString(data, "reason"),
		Status: businessWorkflowStatus(businessTypeOvertime, v.formStatus), FormInstanceID: v.record.FormInstanceID,
		CreatedAt: timeFrom(v.record.CreatedAt), UpdatedAt: timeFrom(v.record.UpdatedAt), EffectStatus: v.record.EffectStatus,
		EffectResult: jsonMap(v.record.Result), EffectAppliedAt: effectAppliedAt(v.record),
	}
}
