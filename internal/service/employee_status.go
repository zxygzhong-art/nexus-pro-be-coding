package service

import (
	"strings"
	"time"

	"nexus-pro-be/internal/utils"
)

func (c HRService) BatchDeleteEmployees(ctx RequestContext, input BatchDeleteEmployeesInput) (BatchEmployeeResponse, error) {
	if strings.TrimSpace(input.Reason) == "" {
		return BatchEmployeeResponse{}, BadRequest("reason is required")
	}
	results := make([]BatchEmployeeResult, 0, len(input.EmployeeIDs))
	for _, id := range uniqueStrings(input.EmployeeIDs) {
		employee, err := c.DeleteEmployee(ctx, id)
		if err != nil {
			results = append(results, BatchEmployeeResult{EmployeeID: id, Success: false, Code: errorCode(err), Message: err.Error()})
			continue
		}
		results = append(results, BatchEmployeeResult{EmployeeID: employee.ID, Success: true})
	}
	if err := c.audit(ctx, "hr.employee.batch_delete", string(ResourceEmployeeCollection), "", string(SeverityHigh), map[string]any{
		"reason":    input.Reason,
		"row_count": len(results),
		"results":   results,
	}); err != nil {
		return BatchEmployeeResponse{}, err
	}
	succeeded := 0
	for _, result := range results {
		if result.Success {
			succeeded++
		}
	}
	c.logWarn(ctx, "employee batch delete completed",
		"requested_count", len(uniqueStrings(input.EmployeeIDs)),
		"succeeded_count", succeeded,
		"failed_count", len(results)-succeeded,
		"reason_present", strings.TrimSpace(input.Reason) != "",
	)
	return BatchEmployeeResponse{Results: results}, nil
}

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
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
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
			return forbiddenDataScope("employee is outside data scope")
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
		inviteAccount := Account{
			ID:          accountID,
			TenantID:    ctx.TenantID,
			DisplayName: next.Name,
			Email:       email,
			EmployeeID:  next.ID,
			Status:      string(AccountStatusPendingInvite),
			CreatedAt:   tx.Now(),
		}
		existing, ok, err := tx.store.GetAccount(goContext(ctx), ctx.TenantID, accountID)
		if err != nil {
			return err
		}
		if ok {
			inviteAccount = existing
			inviteAccount.Email = email
			inviteAccount.EmployeeID = next.ID
			inviteAccount.Status = string(AccountStatusPendingInvite)
		}
		if err := tx.store.UpsertAccount(goContext(ctx), inviteAccount); err != nil {
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
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeInvited), next.ID, map[string]any{"employee_id": next.ID, "account_id": inviteAccount.ID}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.invite", string(ResourceEmployee), next.ID, string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{"email": email, "account_id": inviteAccount.ID})); err != nil {
			return err
		}
		if err := audit.CommitWith(ctx, tx); err != nil {
			return err
		}
		inviteAccountID = inviteAccount.ID
		employee = next
		return nil
	}); err != nil {
		return Employee{}, err
	}
	c.logInfo(ctx, "employee invitation created",
		"employee_id", employee.ID,
		"employee_no", employee.EmployeeNo,
		"account_id", inviteAccountID,
		"status", employeeStatus(employee),
	)
	return employee, nil
}

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
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
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
			return forbiddenDataScope("employee is outside data scope")
		}
		var transitionStart *time.Time
		switch status {
		case string(EmployeeStatusLeaveSuspended):
			if strings.TrimSpace(input.StartDate) == "" || strings.TrimSpace(input.EndDate) == "" {
				return domainValidation("leave suspension requires start_date and end_date", FieldError{Tab: "employment_info", Field: "start_date", Code: "required", Message: "start_date is required"}, FieldError{Tab: "employment_info", Field: "end_date", Code: "required", Message: "end_date is required"})
			}
			start, err := utils.ParseDate(input.StartDate)
			if err != nil {
				return BadRequest("start_date must be RFC3339 or YYYY-MM-DD")
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
			next.ResignDate = &resignDate
			if next.AccountID != "" {
				linkedAccount, ok, err := tx.store.GetAccount(goContext(ctx), ctx.TenantID, next.AccountID)
				if err != nil {
					return err
				}
				if ok {
					linkedAccount.Status = string(AccountStatusDisabled)
					if err := tx.store.UpsertAccount(goContext(ctx), linkedAccount); err != nil {
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
			transitionStart = &start
			next.ResignDate = nil
			if next.AccountID != "" {
				linkedAccount, ok, err := tx.store.GetAccount(goContext(ctx), ctx.TenantID, next.AccountID)
				if err != nil {
					return err
				}
				if ok {
					linkedAccount.Status = string(AccountStatusActive)
					if err := tx.store.UpsertAccount(goContext(ctx), linkedAccount); err != nil {
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
		if err := audit.CommitWith(ctx, tx); err != nil {
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

func errorCode(err error) string {
	if appErr, ok := AsAppError(err); ok {
		return appErr.Code
	}
	return "error"
}

func ensureEmployeeStatusTransition(current, next string) error {
	return ensureEmployeeStatusTransitionWithOptions(current, next, false)
}

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
