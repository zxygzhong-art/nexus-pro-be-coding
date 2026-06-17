package service

import (
	"fmt"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

func (c HRService) PreviewEmployeeImport(ctx RequestContext, input EmployeeImportPreviewInput) (EmployeeImportSession, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return EmployeeImportSession{}, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionImport})
	if err != nil {
		return EmployeeImportSession{}, err
	}
	if !decision.Allowed {
		c.logWarn(ctx, "employee import preview denied",
			"reason", decision.Reason,
			"missing_permissions", decision.MissingPermissions,
		)
		return EmployeeImportSession{}, forbiddenAuthz(decision)
	}
	if decision.RequiresApproval && !ctx.ApprovalConfirmed {
		c.auditAuthzDecision(ctx, "hr.employee.import.preview", "employee_import_session", "", decision)
		c.logWarn(ctx, "employee import preview requires approval",
			"risk_level", decision.RiskLevel,
			"approval_type", decision.ApprovalType,
			"approval_reason", decision.ApprovalReason,
		)
		return EmployeeImportSession{}, domain.ForbiddenReason("approval_required", "high-risk action requires approval")
	}
	authzAudit := AuthzAudit{service: c.Service, target: AuditTarget{Event: "hr.employee.import.preview", Resource: string(ResourceEmployeeImport)}, decision: decision}
	filename := strings.TrimSpace(input.Filename)
	if filename == "" {
		filename = "employees.csv"
	}
	raw := []byte(input.Content)
	if len(raw) > maxEmployeeImportBytes {
		return EmployeeImportSession{}, BadRequest("employee import file exceeds 10MB limit")
	}
	rows, err := parseEmployeeImport(filename, raw)
	if err != nil {
		return EmployeeImportSession{}, BadRequest(err.Error())
	}
	if len(rows) > maxEmployeeImportRows {
		return EmployeeImportSession{}, BadRequest(fmt.Sprintf("employee import supports at most %d rows", maxEmployeeImportRows))
	}
	objectKey := "employee-imports/" + ctx.TenantID + "/" + utils.NewID("file") + "/" + filename
	if err := c.objectStore.PutObject(goContext(ctx), objectKey, importContentType(filename), raw); err != nil {
		return EmployeeImportSession{}, BadRequest("store import file: " + err.Error())
	}
	valid := 0
	rowErrors := make([]RowError, 0)
	batch := newEmployeeImportBatchIndex()
	for i := range rows {
		errors, err := c.validateEmployeeImportRow(ctx, rows[i], batch)
		if err != nil {
			return EmployeeImportSession{}, err
		}
		rows[i].Errors = append(rows[i].Errors, errors...)
		rows[i].Valid = len(rows[i].Errors) == 0
		if rows[i].Valid {
			valid++
		}
		rowErrors = append(rowErrors, rows[i].Errors...)
	}
	session := EmployeeImportSession{
		ID:        utils.NewID("eimp"),
		TenantID:  ctx.TenantID,
		Filename:  filename,
		ObjectKey: objectKey,
		Status:    "previewed",
		Rows:      rows,
		Summary: map[string]any{
			"total":       len(rows),
			"valid":       valid,
			"invalid":     len(rows) - valid,
			"error_count": len(rowErrors),
		},
		CreatedAt: c.Now(),
		ExpiresAt: c.Now().Add(24 * time.Hour),
	}
	if err := c.store.UpsertEmployeeImportSession(goContext(ctx), session); err != nil {
		return EmployeeImportSession{}, err
	}
	if err := c.audit(ctx, "hr.employee.import.preview", string(ResourceEmployeeImport), session.ID, string(SeverityMedium), session.Summary); err != nil {
		return EmployeeImportSession{}, err
	}
	if err := authzAudit.Commit(ctx); err != nil {
		return EmployeeImportSession{}, err
	}
	c.logInfo(ctx, "employee import preview created",
		"session_id", session.ID,
		"filename", filename,
		"total_rows", len(rows),
		"valid_rows", valid,
		"invalid_rows", len(rows)-valid,
		"error_count", len(rowErrors),
	)
	return session, nil
}

func (c HRService) ConfirmEmployeeImport(ctx RequestContext, sessionID string, input EmployeeImportConfirmInput) (EmployeeImportSession, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return EmployeeImportSession{}, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: sessionID, Action: ActionImport})
	if err != nil {
		return EmployeeImportSession{}, err
	}
	if !decision.Allowed {
		c.logWarn(ctx, "employee import confirmation denied",
			"session_id", sessionID,
			"reason", decision.Reason,
			"missing_permissions", decision.MissingPermissions,
		)
		return EmployeeImportSession{}, forbiddenAuthz(decision)
	}
	if decision.RequiresApproval && !ctx.ApprovalConfirmed {
		_ = c.auditAuthzDecision(ctx, "hr.employee.import.confirm", "employee_import_session", sessionID, decision)
		c.logWarn(ctx, "employee import confirmation requires approval",
			"session_id", sessionID,
			"risk_level", decision.RiskLevel,
			"approval_type", decision.ApprovalType,
			"approval_reason", decision.ApprovalReason,
		)
		return EmployeeImportSession{}, domain.ForbiddenReason("approval_required", "high-risk action requires approval")
	}
	authzAudit := AuthzAudit{service: c.Service, target: AuditTarget{Event: "hr.employee.import.confirm", Resource: string(ResourceEmployeeImport), Target: sessionID}, decision: decision}
	var session EmployeeImportSession
	confirmedCount := 0
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
		next, ok, err := tx.store.GetEmployeeImportSession(goContext(ctx), ctx.TenantID, sessionID)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("employee import session", sessionID)
		}
		if terminalEmployeeImportStatus(next.Status) {
			return Conflict("employee import session already confirmed")
		}
		if tx.Now().After(next.ExpiresAt) {
			return BadRequest("employee import session expired")
		}
		results := make([]BatchEmployeeResult, 0, len(next.Rows))
		rowErrors := make([]RowError, 0)
		type importEmployeeWrite struct {
			row      EmployeeImportRow
			employee Employee
		}
		employees := make([]importEmployeeWrite, 0, len(next.Rows))
		reservedEmployeeNos := map[string]struct{}{}
		batch := newEmployeeImportBatchIndex()
		for i, row := range next.Rows {
			row.Errors = nil
			errors, err := tx.validateEmployeeImportRow(ctx, row, batch)
			if err != nil {
				return err
			}
			if len(errors) > 0 {
				row.Errors = errors
				row.Valid = false
				rowErrors = append(rowErrors, errors...)
				results = append(results, BatchEmployeeResult{RowNumber: row.RowNumber, Success: false, Code: "import_validation_failed", Message: firstRowErrorMessage(errors)})
				next.Rows[i] = row
				continue
			}
			employee, err := tx.employeeFromCreateInput(ctx, row.Employee, reservedEmployeeNos)
			if err != nil {
				if errors, ok := employeeImportErrorsFromError(row.RowNumber, err); ok {
					row.Errors = errors
					row.Valid = false
					rowErrors = append(rowErrors, errors...)
					results = append(results, BatchEmployeeResult{RowNumber: row.RowNumber, Success: false, Code: "import_validation_failed", Message: firstRowErrorMessage(errors)})
					next.Rows[i] = row
					continue
				}
				return err
			}
			row.Valid = true
			next.Rows[i] = row
			employees = append(employees, importEmployeeWrite{row: row, employee: employee})
		}
		if len(rowErrors) > 0 {
			tx.logWarn(ctx, "employee import confirmation failed validation",
				"session_id", next.ID,
				"total_rows", len(next.Rows),
				"error_count", len(rowErrors),
			)
			return ImportValidationFailed("employee import contains invalid rows", rowErrors)
		}
		for _, item := range employees {
			employee := item.employee
			if err := tx.store.UpsertEmployee(goContext(ctx), employee); err != nil {
				return err
			}
			if err := tx.touchEmployeeAuthzIfNeeded(ctx, Employee{}, employee, string(EventEmployeeAuthzSubjectImport)); err != nil {
				return err
			}
			if err := tx.linkEmployeeAccount(ctx, employee); err != nil {
				return err
			}
			if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeCreated), employee.ID, map[string]any{"employee_id": employee.ID, "import_session_id": next.ID}); err != nil {
				return err
			}
			results = append(results, BatchEmployeeResult{RowNumber: item.row.RowNumber, EmployeeID: employee.ID, Success: true})
		}
		now := tx.Now()
		next.Status = "confirmed"
		next.ConfirmedAt = &now
		if next.Summary == nil {
			next.Summary = map[string]any{}
		}
		next.Summary["total"] = len(next.Rows)
		next.Summary["confirmed"] = len(employees)
		next.Summary["failed"] = 0
		next.Summary["results"] = results
		next.Summary["row_errors"] = rowErrors
		next.Summary["error_count"] = len(rowErrors)
		next.Summary["mode"] = strings.TrimSpace(input.Mode)
		if err := tx.store.UpsertEmployeeImportSession(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeImported), next.ID, map[string]any{"session_id": next.ID, "success": len(employees), "failed": len(next.Rows) - len(employees)}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.import.confirm", string(ResourceEmployeeImport), next.ID, string(SeverityHigh), next.Summary); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx); err != nil {
			return err
		}
		confirmedCount = len(employees)
		session = next
		return nil
	}); err != nil {
		return EmployeeImportSession{}, err
	}
	c.logInfo(ctx, "employee import confirmed",
		"session_id", session.ID,
		"total_rows", len(session.Rows),
		"confirmed_count", confirmedCount,
		"failed_count", len(session.Rows)-confirmedCount,
		"mode", strings.TrimSpace(input.Mode),
	)
	return session, nil
}

type employeeImportBatchIndex struct {
	employeeNos   map[string]int
	companyEmails map[string]int
	accountIDs    map[string]int
}

func newEmployeeImportBatchIndex() *employeeImportBatchIndex {
	return &employeeImportBatchIndex{
		employeeNos:   map[string]int{},
		companyEmails: map[string]int{},
		accountIDs:    map[string]int{},
	}
}

func terminalEmployeeImportStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "confirmed", "partially_confirmed", "failed":
		return true
	default:
		return false
	}
}

func (c *Service) validateEmployeeImportRow(ctx RequestContext, row EmployeeImportRow, batch *employeeImportBatchIndex) ([]RowError, error) {
	employee, err := c.employeeCreateCandidate(ctx, row.Employee)
	if err != nil {
		errors, ok := employeeImportErrorsFromError(row.RowNumber, err)
		if ok {
			return append(errors, employeeImportBatchErrors(row, batch)...), nil
		}
		return nil, err
	}
	err = c.validateEmployee(ctx, employee, "create")
	errors, ok := employeeImportErrorsFromError(row.RowNumber, err)
	if err != nil && !ok {
		return nil, err
	}
	errors = append(errors, employeeImportBatchErrors(row, batch)...)
	return errors, nil
}

func employeeImportBatchErrors(row EmployeeImportRow, batch *employeeImportBatchIndex) []RowError {
	if batch == nil {
		return nil
	}
	errors := make([]RowError, 0, 3)
	if employeeNo := strings.TrimSpace(row.Employee.EmployeeNo); employeeNo != "" {
		if firstRow, ok := batch.employeeNos[employeeNo]; ok {
			errors = append(errors, RowError{Row: row.RowNumber, Field: "employee_no", Code: "duplicate_in_file", Message: fmt.Sprintf("employee_no is duplicated with row %d", firstRow)})
		} else {
			batch.employeeNos[employeeNo] = row.RowNumber
		}
	}
	if email := strings.ToLower(strings.TrimSpace(row.Employee.CompanyEmail)); email != "" {
		if firstRow, ok := batch.companyEmails[email]; ok {
			errors = append(errors, RowError{Row: row.RowNumber, Field: "company_email", Code: "duplicate_in_file", Message: fmt.Sprintf("company_email is duplicated with row %d", firstRow)})
		} else {
			batch.companyEmails[email] = row.RowNumber
		}
	}
	if accountID := strings.TrimSpace(row.Employee.AccountID); accountID != "" {
		if firstRow, ok := batch.accountIDs[accountID]; ok {
			errors = append(errors, RowError{Row: row.RowNumber, Field: "account_id", Code: "duplicate_in_file", Message: fmt.Sprintf("account_id is duplicated with row %d", firstRow)})
		} else {
			batch.accountIDs[accountID] = row.RowNumber
		}
	}
	return errors
}

func employeeImportErrorsFromError(row int, err error) ([]RowError, bool) {
	if err == nil {
		return nil, true
	}
	appErr, ok := AsAppError(err)
	if !ok || appErr.Status >= 500 {
		return nil, false
	}
	if len(appErr.RowErrors) > 0 {
		return appErr.RowErrors, true
	}
	if len(appErr.FieldErrors) > 0 {
		out := make([]RowError, 0, len(appErr.FieldErrors))
		for _, field := range appErr.FieldErrors {
			out = append(out, RowError{Row: row, Field: field.Field, Code: field.Code, Message: field.Message})
		}
		return out, true
	}
	return []RowError{{Row: row, Code: appErr.Code, Message: appErr.Message}}, true
}

func firstRowErrorMessage(errors []RowError) string {
	if len(errors) == 0 {
		return "employee import row failed"
	}
	return errors[0].Message
}
