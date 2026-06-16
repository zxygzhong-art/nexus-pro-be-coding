package service

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
		return EmployeeImportSession{}, Forbidden(decision.Reason)
	}
	if decision.RequiresApproval && !ctx.ApprovalConfirmed {
		c.auditAuthzDecision(ctx, "hr.employee.import.preview", "employee_import_session", "", decision)
		return EmployeeImportSession{}, Forbidden("high-risk action requires approval")
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
	objectKey := "employee-imports/" + ctx.TenantID + "/" + newID("file") + "/" + filename
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
		ID:        newID("eimp"),
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
		return EmployeeImportSession{}, Forbidden(decision.Reason)
	}
	if decision.RequiresApproval && !ctx.ApprovalConfirmed {
		_ = c.auditAuthzDecision(ctx, "hr.employee.import.confirm", "employee_import_session", sessionID, decision)
		return EmployeeImportSession{}, Forbidden("high-risk action requires approval")
	}
	authzAudit := AuthzAudit{service: c.Service, target: AuditTarget{Event: "hr.employee.import.confirm", Resource: string(ResourceEmployeeImport), Target: sessionID}, decision: decision}
	var session EmployeeImportSession
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
		switch {
		case len(employees) == len(next.Rows):
			next.Status = "confirmed"
		case len(employees) > 0:
			next.Status = "partially_confirmed"
		default:
			next.Status = "failed"
		}
		next.ConfirmedAt = &now
		if next.Summary == nil {
			next.Summary = map[string]any{}
		}
		next.Summary["total"] = len(next.Rows)
		next.Summary["confirmed"] = len(employees)
		next.Summary["failed"] = len(next.Rows) - len(employees)
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
		session = next
		return nil
	}); err != nil {
		return EmployeeImportSession{}, err
	}
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

func parseEmployeeImport(filename string, raw []byte) ([]EmployeeImportRow, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".xlsx" {
		return parseEmployeeXLSX(raw)
	}
	return parseEmployeeCSV(raw)
}

func importContentType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	default:
		return "text/csv; charset=utf-8"
	}
}

func parseEmployeeCSV(raw []byte) ([]EmployeeImportRow, error) {
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})))
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}
	return employeeRowsFromRecords(records)
}

func parseEmployeeXLSX(raw []byte) ([]EmployeeImportRow, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("parse xlsx: %w", err)
	}
	files := map[string]*zip.File{}
	for _, file := range zr.File {
		files[file.Name] = file
	}
	shared, err := readXLSXSharedStrings(files["xl/sharedStrings.xml"])
	if err != nil {
		return nil, err
	}
	sheet := files["xl/worksheets/sheet1.xml"]
	if sheet == nil {
		return nil, fmt.Errorf("xlsx sheet1.xml not found")
	}
	records, err := readXLSXSheet(sheet, shared)
	if err != nil {
		return nil, err
	}
	return employeeRowsFromRecords(records)
}

func employeeRowsFromRecords(records [][]string) ([]EmployeeImportRow, error) {
	if len(records) < 2 {
		return nil, fmt.Errorf("import file must include header and at least one data row")
	}
	rows := make([]EmployeeImportRow, 0, len(records)-1)
	for i, record := range records[1:] {
		record = padRecord(record, employeeImportColumnCount())
		rows = append(rows, EmployeeImportRow{
			RowNumber: i + 2,
			Input:     employeeImportInputFromRecord(record),
			Employee:  employeeCreateInputFromImportRecord(record),
		})
	}
	return rows, nil
}

type xlsxSST struct {
	Items []struct {
		Text string `xml:"t"`
	} `xml:"si"`
}

func readXLSXSharedStrings(file *zip.File) ([]string, error) {
	if file == nil {
		return nil, nil
	}
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	var sst xlsxSST
	if err := xml.NewDecoder(rc).Decode(&sst); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(sst.Items))
	for _, item := range sst.Items {
		out = append(out, item.Text)
	}
	return out, nil
}

type xlsxWorksheet struct {
	Rows []struct {
		Cells []struct {
			Ref   string `xml:"r,attr"`
			Type  string `xml:"t,attr"`
			Value string `xml:"v"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

func readXLSXSheet(file *zip.File, shared []string) ([][]string, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	var sheet xlsxWorksheet
	if err := xml.Unmarshal(raw, &sheet); err != nil {
		return nil, err
	}
	records := make([][]string, 0, len(sheet.Rows))
	for _, row := range sheet.Rows {
		record := make([]string, employeeImportColumnCount())
		for idx, cell := range row.Cells {
			col := idx
			if cell.Ref != "" {
				col = xlsxColumnIndex(cell.Ref)
			}
			if col < 0 || col >= len(record) {
				continue
			}
			value := cell.Value
			if cell.Type == "s" {
				i, _ := strconv.Atoi(value)
				if i >= 0 && i < len(shared) {
					value = shared[i]
				}
			}
			record[col] = value
		}
		records = append(records, record)
	}
	return records, nil
}

func xlsxColumnIndex(ref string) int {
	col := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		col = col*26 + int(r-'A'+1)
	}
	return col - 1
}

func normalizeImportDate(value string) string {
	value = strings.TrimSpace(value)
	if strings.Count(value, "/") == 2 {
		parts := strings.Split(value, "/")
		if len(parts[1]) == 1 {
			parts[1] = "0" + parts[1]
		}
		if len(parts[2]) == 1 {
			parts[2] = "0" + parts[2]
		}
		return strings.Join(parts, "-")
	}
	return value
}

func padRecord(record []string, size int) []string {
	if len(record) >= size {
		return record
	}
	out := make([]string, size)
	copy(out, record)
	return out
}
