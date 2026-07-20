package service

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	"nexus-pro-api/internal/utils"
)

func (c HRService) ExportEmployeesCSV(ctx RequestContext, query EmployeeQuery) ([]byte, string, error) {
	query = normalizeEmployeeQuery(query)
	items, decision, err := c.exportEmployees(ctx, query)
	if err != nil {
		return nil, "", err
	}
	if len(items) > maxEmployeeExportRows {
		return nil, "", employeeExportLimitError()
	}
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)
	columns := employeeExportColumnsForPolicy(decision.FieldPolicies)
	headers := make([]string, 0, len(columns))
	for _, column := range columns {
		headers = append(headers, column.header)
	}
	_ = w.Write(headers)
	for _, item := range items {
		record := make([]string, 0, len(columns))
		for _, column := range columns {
			record = append(record, neutralizeSpreadsheetCell(column.value(item)))
		}
		_ = w.Write(record)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "employees.csv", nil
}

// neutralizeSpreadsheetCell 處理 neutralize spreadsheet 儲存格。
func neutralizeSpreadsheetCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}

// rejectOversizedEmployeeExport 駁回 oversized 員工 export 的服務流程。
func (c HRService) rejectOversizedEmployeeExport(ctx RequestContext, query EmployeeQuery) error {
	total, err := c.store.CountEmployeesByQuery(goContext(ctx), ctx.TenantID, query)
	if err != nil {
		return err
	}
	if total > maxEmployeeExportRows {
		return employeeExportLimitError()
	}
	return nil
}

// employeeExportLimitError 處理員工 export 限制錯誤。
func employeeExportLimitError() error {
	return Conflict(fmt.Sprintf("employee export exceeds synchronous limit of %d rows; use async export job", maxEmployeeExportRows))
}

// BatchDeleteEmployees 處理批次 delete 員工的服務流程。
func (c HRService) BatchDeleteEmployees(ctx RequestContext, input BatchDeleteEmployeesInput) (BatchEmployeeResponse, error) {
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return BatchEmployeeResponse{}, BadRequest("reason is required")
	}
	ids := uniqueStrings(input.EmployeeIDs)
	if len(ids) == 0 {
		return BatchEmployeeResponse{}, BadRequest("employee_ids is required")
	}
	account, collectionDecision, _, err := c.Authorize(ctx,
		CheckRequest{
			ApplicationCode: AppHR,
			ResourceType:    ResourceEmployee,
			Action:          ActionDelete,
			Context: map[string]any{"filters": map[string]any{
				"employee_ids": ids,
			}},
		},
		AuditTarget{Event: "hr.employee.batch_delete", Resource: string(ResourceEmployeeCollection)},
	)
	if err != nil {
		return BatchEmployeeResponse{}, err
	}
	checks := make([]CheckRequest, 0, len(ids))
	for _, id := range ids {
		checks = append(checks, CheckRequest{
			ApplicationCode:  AppHR,
			ResourceType:     ResourceEmployee,
			ResourceID:       id,
			TargetEmployeeID: id,
			Action:           ActionDelete,
		})
	}
	batch, err := c.Authz().BatchCheck(ctx, BatchCheckRequest{Checks: checks})
	if err != nil {
		return BatchEmployeeResponse{}, err
	}
	if len(batch.Results) != len(ids) {
		return BatchEmployeeResponse{}, fmt.Errorf("authz batch-check returned %d results for %d employees", len(batch.Results), len(ids))
	}
	results := make([]BatchEmployeeResult, 0, len(ids))
	for i, id := range ids {
		decision := batch.Results[i]
		if !decision.Allowed {
			results = append(results, BatchEmployeeResult{EmployeeID: id, Success: false, Code: authzReasonCode(decision), Message: decision.Reason})
			continue
		}
		employee, accountDisabled, err := c.deleteEmployeeWithDecision(ctx, account, decision, id)
		if err != nil {
			results = append(results, BatchEmployeeResult{EmployeeID: id, Success: false, Code: errorCode(err), Message: err.Error()})
			continue
		}
		action := "soft_deleted"
		if accountDisabled {
			action = "soft_deleted_account_disabled"
		}
		results = append(results, BatchEmployeeResult{EmployeeID: employee.ID, Success: true, Action: action})
	}
	succeeded := 0
	for _, result := range results {
		if result.Success {
			succeeded++
		}
	}
	auditDetails := auditDecisionDetails(ctx, collectionDecision, map[string]any{
		"reason":                 reason,
		"result":                 batchEmployeeAuditResult(succeeded, len(results)),
		"requested_employee_ids": ids,
		"succeeded_employee_ids": batchEmployeeResultIDs(results, true),
		"failed_employee_ids":    batchEmployeeResultIDs(results, false),
		"row_count":              len(results),
		"results":                results,
	})
	if err := c.audit(ctx, "hr.employee.batch_delete", string(ResourceEmployeeCollection), "", string(SeverityHigh), auditDetails); err != nil {
		return BatchEmployeeResponse{}, err
	}
	c.logWarn(ctx, "employee batch delete completed",
		"requested_count", len(uniqueStrings(input.EmployeeIDs)),
		"succeeded_count", succeeded,
		"failed_count", len(results)-succeeded,
		"reason_present", strings.TrimSpace(input.Reason) != "",
	)
	return BatchEmployeeResponse{Results: results}, nil
}

// batchEmployeeAuditResult 處理批次員工稽覈結果。
func batchEmployeeAuditResult(succeeded, total int) string {
	switch {
	case total <= 0 || succeeded == 0:
		return "failed"
	case succeeded < total:
		return "partial_success"
	default:
		return "success"
	}
}

// deleteEmployeeWithDecision 刪除員工 with 決策的服務流程。
func (c HRService) deleteEmployeeWithDecision(ctx RequestContext, account Account, decision CheckResult, id string) (Employee, bool, error) {
	var employee Employee
	accountDisabled := false
	if err := c.withTransaction(ctx, func(tx HRService) error {
		next, ok, err := tx.store.GetEmployee(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("employee", id)
		}
		if employeeStatus(next) == string(EmployeeStatusDeleted) {
			return Conflict("employee is already deleted")
		}
		visible, err := tx.filterEmployeesByDecision(ctx, account, []Employee{next}, decision)
		if err != nil {
			return err
		}
		if len(visible) == 0 {
			return ForbiddenDataScope("employee is outside data scope")
		}
		before := next
		next.Status = string(EmployeeStatusDeleted)
		next.EmploymentStatus = string(EmployeeStatusDeleted)
		next.UpdatedAt = tx.Now()
		next = tx.appendHistoryForChangedEmployment(before, next, "刪除")
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		if next.AccountID != "" {
			linkedAccount, ok, err := tx.store.GetAccount(goContext(ctx), ctx.TenantID, next.AccountID)
			if err != nil {
				return err
			}
			if ok {
				beforeAccount := linkedAccount
				linkedAccount.Status = string(AccountStatusDisabled)
				if err := tx.store.UpsertAccount(goContext(ctx), linkedAccount); err != nil {
					return err
				}
				if err := tx.Service.syncAccountTenantMembershipTuple(ctx, beforeAccount, linkedAccount); err != nil {
					return err
				}
				accountDisabled = true
			}
		}
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeOffboarded), next.ID, map[string]any{"employee_id": next.ID, "status": string(EmployeeStatusDeleted)}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.delete", string(ResourceEmployee), next.ID, string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{
			"previous_status":  employeeStatus(before),
			"status":           string(EmployeeStatusDeleted),
			"account_disabled": accountDisabled,
		})); err != nil {
			return err
		}
		employee = next
		return nil
	}); err != nil {
		return Employee{}, false, err
	}
	return employee, accountDisabled, nil
}

// batchEmployeeResultIDs 處理批次員工結果 IDs。
func batchEmployeeResultIDs(results []BatchEmployeeResult, success bool) []string {
	ids := make([]string, 0)
	for _, result := range results {
		if result.Success == success {
			ids = append(ids, result.EmployeeID)
		}
	}
	return ids
}

// InviteEmployee 邀請員工的服務流程。
func (c HRService) InviteEmployee(ctx RequestContext, id string, input InviteEmployeeInput) (Employee, error) {
	account, decision, audit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionInvite},
		AuditTarget{Resource: string(ResourceEmployee), Target: id},
	)
	if err != nil {
		return Employee{}, err
	}
	var employee Employee
	inviteAccountID := ""
	if err := c.withTransaction(ctx, func(tx HRService) error {
		next, ok, err := tx.store.GetEmployee(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("employee", id)
		}
		visible, err := tx.filterEmployeesByDecision(ctx, account, []Employee{next}, decision)
		if err != nil {
			return err
		}
		if len(visible) == 0 {
			return ForbiddenDataScope("employee is outside data scope")
		}
		switch employeeStatus(next) {
		case string(EmployeeStatusDeleted), string(EmployeeStatusResigned):
			return Conflict("deleted or resigned employee cannot be invited")
		}
		before := next
		email := strings.TrimSpace(input.Email)
		if email == "" {
			email = next.CompanyEmail
		}
		if email == "" {
			return BadRequest("invite email is required")
		}
		accountID := next.AccountID
		if accountID == "" {
			accountID = utils.NewID("acct")
		}
		if err := tx.ensureAccountEmailAvailableForAccount(ctx, email, accountID); err != nil {
			return err
		}
		inviteAccount := Account{
			ID:          accountID,
			TenantID:    ctx.TenantID,
			DisplayName: next.Name,
			Email:       email,
			EmployeeID:  next.ID,
			Status:      string(AccountStatusPendingInvite),
			CreatedAt:   tx.Now(),
		}
		beforeAccount := Account{}
		existing, ok, err := tx.store.GetAccount(goContext(ctx), ctx.TenantID, accountID)
		if err != nil {
			return err
		}
		if ok {
			beforeAccount = existing
			inviteAccount = existing
			inviteAccount.Email = email
			inviteAccount.EmployeeID = next.ID
			inviteAccount.Status = string(AccountStatusPendingInvite)
		}
		if err := tx.store.UpsertAccount(goContext(ctx), inviteAccount); err != nil {
			return err
		}
		if err := tx.Service.syncAccountTenantMembershipTuple(ctx, beforeAccount, inviteAccount); err != nil {
			return err
		}
		next.AccountID = inviteAccount.ID
		next.EmploymentStatus = utils.FirstNonEmpty(next.EmploymentStatus, string(EmployeeStatusOnboarding))
		next.Status = utils.FirstNonEmpty(next.Status, next.EmploymentStatus)
		next.UpdatedAt = tx.Now()
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.touchEmployeeAuthzIfNeeded(ctx, before, next, string(EventEmployeeAuthzSubjectInvite)); err != nil {
			return err
		}
		if err := tx.provisionEmployeeAccountIdentity(ctx, next, inviteAccount, true); err != nil {
			return err
		}
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeInvited), next.ID, map[string]any{"employee_id": next.ID, "account_id": inviteAccount.ID}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.invite", string(ResourceEmployee), next.ID, string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{"email": email, "account_id": inviteAccount.ID})); err != nil {
			return err
		}
		if err := audit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		inviteAccountID = inviteAccount.ID
		employee = next
		return nil
	}); err != nil {
		return Employee{}, err
	}
	c.runIdentityProvisioningFastPath(ctx)
	c.logInfo(ctx, "employee invitation created",
		"employee_id", employee.ID,
		"employee_no", employee.EmployeeNo,
		"account_id", inviteAccountID,
		"status", employeeStatus(employee),
	)
	return employee, nil
}

// TransitionEmployeeStatus 轉換員工狀態的服務流程。
func (c HRService) TransitionEmployeeStatus(ctx RequestContext, id string, input StatusTransitionInput) (Employee, error) {
	status := normalizeEmployeeStatus(input.Status)
	if status == "" {
		return Employee{}, BadRequest("status is required")
	}
	if status == string(EmployeeStatusDeleted) {
		return Employee{}, BadRequest("deleted status requires delete")
	}
	if !validEmployeeStatus(status, false) {
		return Employee{}, BadRequest("invalid employee status")
	}
	account, decision, audit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionStatusTransition},
		AuditTarget{Resource: string(ResourceEmployee), Target: id},
	)
	if err != nil {
		return Employee{}, err
	}
	var employee Employee
	previousStatus := ""
	transitionType := ""
	if err := c.withTransaction(ctx, func(tx HRService) error {
		next, ok, err := tx.store.GetEmployee(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("employee", id)
		}
		currentStatus := employeeStatus(next)
		previousStatus = currentStatus
		reinstating := isEmployeeReinstatement(currentStatus, status)
		if err := ensureEmployeeStatusTransitionWithOptions(currentStatus, status, true); err != nil {
			return err
		}
		visible, err := tx.filterEmployeesByDecision(ctx, account, []Employee{next}, decision)
		if err != nil {
			return err
		}
		if len(visible) == 0 {
			return ForbiddenDataScope("employee is outside data scope")
		}
		var transitionStart *time.Time
		switch status {
		case string(EmployeeStatusLeaveSuspended):
			fields := make([]FieldError, 0, 3)
			if strings.TrimSpace(input.Reason) == "" {
				fields = append(fields, FieldError{Tab: "employment_info", Field: "reason", Code: "required", Message: "reason is required"})
			}
			if strings.TrimSpace(input.StartDate) == "" {
				fields = append(fields, FieldError{Tab: "employment_info", Field: "start_date", Code: "required", Message: "start_date is required"})
			}
			if strings.TrimSpace(input.EndDate) == "" {
				fields = append(fields, FieldError{Tab: "employment_info", Field: "end_date", Code: "required", Message: "end_date is required"})
			}
			if len(fields) > 0 {
				return domainValidation("leave suspension requires reason, start_date and end_date", fields...)
			}
			start, err := utils.ParseDate(input.StartDate)
			if err != nil {
				return BadRequest("start_date must be RFC3339 or YYYY-MM-DD")
			}
			end, err := utils.ParseDate(input.EndDate)
			if err != nil {
				return BadRequest("end_date must be RFC3339 or YYYY-MM-DD")
			}
			if end.Before(start) {
				return domainValidation("leave suspension date range is invalid", FieldError{Tab: "employment_info", Field: "end_date", Code: "invalid", Message: "end_date must be on or after start_date"})
			}
			transitionStart = &start
		case string(EmployeeStatusResigned):
			if strings.TrimSpace(input.EndDate) == "" || strings.TrimSpace(input.Reason) == "" {
				return domainValidation("resignation requires end_date and reason", FieldError{Tab: "employment_info", Field: "end_date", Code: "required", Message: "end_date is required"}, FieldError{Tab: "employment_info", Field: "reason", Code: "required", Message: "reason is required"})
			}
			resignDate, err := utils.ParseDate(input.EndDate)
			if err != nil {
				return BadRequest("end_date must be RFC3339 or YYYY-MM-DD")
			}
			if next.HireDate != nil && resignDate.Before(*next.HireDate) {
				return domainValidation("resignation date range is invalid", FieldError{Tab: "employment_info", Field: "end_date", Code: "invalid", Message: "end_date must be on or after hire_date"})
			}
			next.ResignDate = &resignDate
			if next.AccountID != "" {
				linkedAccount, ok, err := tx.store.GetAccount(goContext(ctx), ctx.TenantID, next.AccountID)
				if err != nil {
					return err
				}
				if ok {
					beforeAccount := linkedAccount
					linkedAccount.Status = string(AccountStatusDisabled)
					if err := tx.store.UpsertAccount(goContext(ctx), linkedAccount); err != nil {
						return err
					}
					if err := tx.Service.syncAccountTenantMembershipTuple(ctx, beforeAccount, linkedAccount); err != nil {
						return err
					}
				}
			}
		}
		if reinstating {
			if strings.TrimSpace(input.StartDate) == "" || strings.TrimSpace(input.Reason) == "" {
				return domainValidation("reinstatement requires start_date and reason", FieldError{Tab: "employment_info", Field: "start_date", Code: "required", Message: "start_date is required"}, FieldError{Tab: "employment_info", Field: "reason", Code: "required", Message: "reason is required"})
			}
			start, err := utils.ParseDate(input.StartDate)
			if err != nil {
				return BadRequest("start_date must be RFC3339 or YYYY-MM-DD")
			}
			if next.ResignDate != nil && start.Before(*next.ResignDate) {
				return domainValidation("reinstatement date range is invalid", FieldError{Tab: "employment_info", Field: "start_date", Code: "invalid", Message: "start_date must be on or after resign_date"})
			}
			transitionStart = &start
			next.ResignDate = nil
			if next.AccountID != "" {
				linkedAccount, ok, err := tx.store.GetAccount(goContext(ctx), ctx.TenantID, next.AccountID)
				if err != nil {
					return err
				}
				if ok {
					beforeAccount := linkedAccount
					linkedAccount.Status = string(AccountStatusActive)
					if err := tx.store.UpsertAccount(goContext(ctx), linkedAccount); err != nil {
						return err
					}
					if err := tx.Service.syncAccountTenantMembershipTuple(ctx, beforeAccount, linkedAccount); err != nil {
						return err
					}
				}
			}
		}
		before := next
		next.Status = status
		next.EmploymentStatus = status
		next.UpdatedAt = tx.Now()
		next.EmploymentInfo = mergeMap(next.EmploymentInfo, input.Details)
		if next.EmploymentInfo == nil {
			next.EmploymentInfo = map[string]any{}
		}
		if reinstating {
			delete(next.EmploymentInfo, "resign_date")
			delete(next.EmploymentInfo, "resign_reason")
		}
		next.EmploymentInfo["transition_reason"] = input.Reason
		next.EmploymentInfo["transition_start_date"] = input.StartDate
		next.EmploymentInfo["transition_end_date"] = input.EndDate
		next.EmploymentInfo["transition_type"] = employeeTransitionType(currentStatus, status)
		next = tx.appendHistoryForChangedEmployment(before, next, input.Reason)
		if reinstating && transitionStart != nil && len(next.InternalExperiences) > 0 {
			next.InternalExperiences[len(next.InternalExperiences)-1].StartDate = transitionStart
		}
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		eventType := string(EventEmployeeStatusChanged)
		if status == string(EmployeeStatusResigned) {
			eventType = string(EventEmployeeOffboarded)
		} else if reinstating {
			eventType = string(EventEmployeeReinstated)
		}
		if err := tx.appendEmployeeEvent(ctx, eventType, next.ID, map[string]any{"employee_id": next.ID, "status": status, "reason": input.Reason}); err != nil {
			return err
		}
		if err := tx.audit(ctx, employeeStatusTransitionAuditAction(currentStatus, status), string(ResourceEmployee), next.ID, string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{
			"previous_status": currentStatus,
			"status":          status,
			"transition_type": employeeTransitionType(currentStatus, status),
			"reason":          input.Reason,
			"start_date":      input.StartDate,
			"end_date":        input.EndDate,
		})); err != nil {
			return err
		}
		if err := audit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		transitionType = employeeTransitionType(currentStatus, status)
		employee = next
		return nil
	}); err != nil {
		return Employee{}, err
	}
	c.logWarn(ctx, "employee status transitioned",
		"employee_id", employee.ID,
		"employee_no", employee.EmployeeNo,
		"previous_status", previousStatus,
		"status", employeeStatus(employee),
		"transition_type", transitionType,
	)
	return employee, nil
}

// errorCode 處理錯誤碼。
func errorCode(err error) string {
	if appErr, ok := AsAppError(err); ok {
		return appErr.Code
	}
	return "error"
}

// ensureEmployeeStatusTransition 確保員工狀態轉換。
func ensureEmployeeStatusTransition(current, next string) error {
	return ensureEmployeeStatusTransitionWithOptions(current, next, false)
}

// ensureEmployeeStatusTransitionWithOptions 確保員工狀態轉換 with 選項。
func ensureEmployeeStatusTransitionWithOptions(current, next string, allowReinstatement bool) error {
	current = normalizeEmployeeStatus(current)
	next = normalizeEmployeeStatus(next)
	switch current {
	case string(EmployeeStatusDeleted):
		if current != "" && current != next {
			return Conflict("terminal employee status cannot be changed")
		}
	case string(EmployeeStatusResigned):
		if current != "" && current != next && !(allowReinstatement && isEmployeeReinstatement(current, next)) {
			return Conflict("terminal employee status cannot be changed")
		}
	}
	return nil
}

// isEmployeeReinstatement 判斷是否為員工 reinstatement。
func isEmployeeReinstatement(current, next string) bool {
	current = normalizeEmployeeStatus(current)
	next = normalizeEmployeeStatus(next)
	if current != string(EmployeeStatusResigned) {
		return false
	}
	switch next {
	case string(EmployeeStatusActive), string(EmployeeStatusProbation), string(EmployeeStatusOnboarding):
		return true
	default:
		return false
	}
}

// employeeTransitionType 處理員工轉換 type。
func employeeTransitionType(current, next string) string {
	current = normalizeEmployeeStatus(current)
	next = normalizeEmployeeStatus(next)
	switch {
	case isEmployeeReinstatement(current, next):
		return "reinstatement"
	case next == string(EmployeeStatusResigned):
		return "resignation"
	case next == string(EmployeeStatusLeaveSuspended):
		return "leave_suspension"
	default:
		return "status_change"
	}
}

// employeeStatusTransitionAuditAction 處理員工狀態轉換稽覈 action。
func employeeStatusTransitionAuditAction(current, next string) string {
	switch employeeTransitionType(current, next) {
	case "reinstatement":
		return "hr.employee.reinstate"
	case "resignation":
		return "hr.employee.offboard"
	case "leave_suspension":
		return "hr.employee.leave_suspend"
	default:
		return "hr.employee.status_transition"
	}
}
