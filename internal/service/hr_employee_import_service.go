package service

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

func employeeImportTemplateCSV() ([]byte, error) {
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(&buf)
	if err := writer.Write(employeeImportTemplateHeaders()); err != nil {
		return nil, err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// employeeImportTemplateHeaders 處理員工 import 範本 headers。
func employeeImportTemplateHeaders() []string {
	headers := make([]string, 0, len(employeeImportColumns))
	for _, column := range employeeImportColumns {
		headers = append(headers, column.header)
	}
	return headers
}

// employeeImportTemplateXLSX 處理員工 import 範本 XLSX。
func employeeImportTemplateXLSX() ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml":        xlsxContentTypesXML,
		"_rels/.rels":                xlsxRelsXML,
		"xl/workbook.xml":            xlsxWorkbookXML,
		"xl/_rels/workbook.xml.rels": xlsxWorkbookRelsXML,
		"xl/worksheets/sheet1.xml":   employeeImportTemplateSheetXML(),
		"xl/sharedStrings.xml":       employeeImportTemplateSharedStringsXML(),
	}
	for name, body := range files {
		file, err := zipWriter.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := file.Write([]byte(body)); err != nil {
			return nil, err
		}
	}
	if err := zipWriter.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// employeeImportTemplateSheetXML 處理員工 import 範本 sheet xml。
func employeeImportTemplateSheetXML() string {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1">`)
	for i := range employeeImportTemplateHeaders() {
		col := string(rune('A' + i))
		buf.WriteString(`<c r="`)
		buf.WriteString(col)
		buf.WriteString(`1" t="s"><v>`)
		buf.WriteString(strconv.Itoa(i))
		buf.WriteString(`</v></c>`)
	}
	buf.WriteString(`</row></sheetData></worksheet>`)
	return buf.String()
}

// employeeImportTemplateSharedStringsXML 處理員工 import 範本 shared 字串 xml。
func employeeImportTemplateSharedStringsXML() string {
	headers := employeeImportTemplateHeaders()
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="`)
	buf.WriteString(strconv.Itoa(len(headers)))
	buf.WriteString(`" uniqueCount="`)
	buf.WriteString(strconv.Itoa(len(headers)))
	buf.WriteString(`">`)
	for _, header := range headers {
		buf.WriteString(`<si><t>`)
		_ = xml.EscapeText(&buf, []byte(header))
		buf.WriteString(`</t></si>`)
	}
	buf.WriteString(`</sst>`)
	return buf.String()
}

const xlsxContentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>
</Types>`

const xlsxRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`

const xlsxWorkbookXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheets><sheet name="Employees" sheetId="1" r:id="rId1"/></sheets>
</workbook>`

const xlsxWorkbookRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/>
</Relationships>`

// parseEmployeeImport 解析員工 import。
func parseEmployeeImport(filename string, raw []byte) ([]EmployeeImportRow, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".xlsx":
		return parseEmployeeXLSX(raw)
	case ".csv":
		return parseEmployeeCSV(raw)
	default:
		return nil, fmt.Errorf("employee import supports csv and xlsx files")
	}
}

// importContentType 匯入 content type。
func importContentType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	default:
		return "text/csv; charset=utf-8"
	}
}

// parseEmployeeCSV 解析員工 CSV。
func parseEmployeeCSV(raw []byte) ([]EmployeeImportRow, error) {
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})))
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}
	return employeeRowsFromRecords(records)
}

// parseEmployeeXLSX 解析員工 XLSX。
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

// employeeRowsFromRecords 處理員工列 來源 records。
func employeeRowsFromRecords(records [][]string) ([]EmployeeImportRow, error) {
	if len(records) < 2 {
		return nil, fmt.Errorf("import file must include header and at least one data row")
	}
	if err := validateEmployeeImportHeader(records[0]); err != nil {
		return nil, err
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

// validateEmployeeImportHeader 驗證員工 import header。
func validateEmployeeImportHeader(record []string) error {
	headers := employeeImportTemplateHeaders()
	if employeeImportHeaderMatches(record, headers) {
		return nil
	}
	legacyHeaders := headers[:employeeImportColumnAccountPolicy]
	if employeeImportHeaderMatches(record, legacyHeaders) {
		return nil
	}
	if len(record) < len(headers) {
		return fmt.Errorf("import file header must include %d columns", len(headers))
	}
	for i, want := range headers {
		if got := strings.TrimSpace(record[i]); got != want {
			return fmt.Errorf("import file header column %d must be %q", i+1, want)
		}
	}
	return nil
}

// employeeImportHeaderMatches 處理員工 import header matches。
func employeeImportHeaderMatches(record []string, headers []string) bool {
	if len(record) < len(headers) {
		return false
	}
	for i, want := range headers {
		if strings.TrimSpace(record[i]) != want {
			return false
		}
	}
	return true
}

type xlsxSST struct {
	Items []struct {
		Text string `xml:"t"`
	} `xml:"si"`
}

// readXLSXSharedStrings 讀取 XLSX shared 字串。
func readXLSXSharedStrings(file *zip.File) ([]string, error) {
	if file == nil {
		return nil, nil
	}
	raw, err := readLimitedXLSXFile(file, maxEmployeeImportXLSXEntryBytes)
	if err != nil {
		return nil, err
	}
	var sst xlsxSST
	if err := xml.Unmarshal(raw, &sst); err != nil {
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

// readXLSXSheet 讀取 XLSX sheet。
func readXLSXSheet(file *zip.File, shared []string) ([][]string, error) {
	raw, err := readLimitedXLSXFile(file, maxEmployeeImportXLSXEntryBytes)
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

// readLimitedXLSXFile 讀取 limited XLSX 檔案。
func readLimitedXLSXFile(file *zip.File, maxBytes int64) ([]byte, error) {
	if file == nil {
		return nil, nil
	}
	if file.UncompressedSize64 > uint64(maxBytes) {
		return nil, fmt.Errorf("xlsx entry %s exceeds %d bytes", file.Name, maxBytes)
	}
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	limited := &io.LimitedReader{R: rc, N: maxBytes + 1}
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("xlsx entry %s exceeds %d bytes", file.Name, maxBytes)
	}
	return raw, nil
}

// xlsxColumnIndex 處理 XLSX column index。
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

// normalizeImportDate 正規化import 日期。
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

// padRecord 處理 pad record。
func padRecord(record []string, size int) []string {
	if len(record) >= size {
		return record
	}
	out := make([]string, size)
	copy(out, record)
	return out
}

const (
	maxEmployeeImportBytes          = 10 << 20
	maxEmployeeImportXLSXEntryBytes = maxEmployeeImportBytes
	maxEmployeeImportRows           = 500
)

const (
	employeeImportModeCreate = "create"
	employeeImportModeUpdate = "update"
	employeeImportModeUpsert = "upsert"
)

// normalizeEmployeeImportMode 正規化員工 import mode。
func normalizeEmployeeImportMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", employeeImportModeCreate:
		return employeeImportModeCreate, nil
	case employeeImportModeUpdate:
		return employeeImportModeUpdate, nil
	case employeeImportModeUpsert:
		return employeeImportModeUpsert, nil
	default:
		return "", BadRequest("employee import mode must be create, update, or upsert")
	}
}

// validateEmployeeImportFailurePolicy 驗證員工 import failure 政策。
func validateEmployeeImportFailurePolicy(policy string) error {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", "all_or_nothing":
		return nil
	default:
		return BadRequest("employee import failure_policy must be all_or_nothing")
	}
}

// safeImportFilename 處理 safe import filename。
func safeImportFilename(filename string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "employees.csv"
	}
	filename = filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
	if filename == "." || filename == "/" || filename == "" {
		return "employees.csv"
	}
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', 0:
			return '_'
		default:
			return r
		}
	}, filename)
}

// employeeImportObjectKey 處理員工 import 物件 key。
func employeeImportObjectKey(tenantID, sessionID, filename string) string {
	return "employee-imports/" + tenantID + "/" + sessionID + "/" + utils.NewID("file") + "-" + safeImportFilename(filename)
}

// employeeImportSHA256 處理員工 import sha 256。
func employeeImportSHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// employeeImportAuditDetails 處理員工 import 稽覈 details。
func employeeImportAuditDetails(session EmployeeImportSession) map[string]any {
	details := utils.CopyStringMap(session.Summary)
	if details == nil {
		details = map[string]any{}
	}
	details["session_id"] = session.ID
	details["filename"] = session.Filename
	details["object_provider"] = session.ObjectProvider
	details["object_bucket"] = session.ObjectBucket
	details["object_key"] = session.ObjectKey
	details["content_type"] = session.ContentType
	details["size_bytes"] = session.SizeBytes
	details["sha256"] = session.SHA256
	details["created_by_account_id"] = session.CreatedByAccountID
	if session.ConfirmedByAccountID != "" {
		details["confirmed_by_account_id"] = session.ConfirmedByAccountID
	}
	return details
}

// PreviewEmployeeImport 預覽員工 import 的服務流程。
func (c HRService) PreviewEmployeeImport(ctx RequestContext, input EmployeeImportPreviewInput) (EmployeeImportSession, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return EmployeeImportSession{}, err
	}
	req := CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionImport}
	decision, err := c.evaluateAuthz(ctx, account, req)
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
	authzAudit := AuthzAudit{service: c.Service, target: AuditTarget{Event: "hr.employee.import.preview", Resource: string(ResourceEmployeeImport)}, decision: decision}
	filename := safeImportFilename(input.Filename)
	raw := []byte(input.Content)
	if len(raw) > maxEmployeeImportBytes {
		return EmployeeImportSession{}, BadRequest("employee import file exceeds 10MB limit")
	}
	contentType := importContentType(filename)
	sha256Value := employeeImportSHA256(raw)
	rows, err := parseEmployeeImport(filename, raw)
	if err != nil {
		return EmployeeImportSession{}, BadRequest(err.Error())
	}
	if len(rows) > maxEmployeeImportRows {
		return EmployeeImportSession{}, BadRequest(fmt.Sprintf("employee import supports at most %d rows", maxEmployeeImportRows))
	}
	sessionID := utils.NewID("eimp")
	objectKey := employeeImportObjectKey(ctx.TenantID, sessionID, filename)
	if err := c.objectStore.PutObject(goContext(ctx), objectKey, contentType, raw); err != nil {
		c.logWarn(ctx, "store employee import file failed", "object_key", objectKey, "error", err)
		return EmployeeImportSession{}, domain.E(502, "object_store_error", "employee import file storage failed")
	}
	objectCommitted := false
	defer func() {
		if !objectCommitted {
			c.deleteObjectIfSupported(ctx, objectKey)
		}
	}()
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
		ID:                 sessionID,
		TenantID:           ctx.TenantID,
		Filename:           filename,
		ObjectProvider:     objectStoreProvider(c.objectStore),
		ObjectBucket:       objectStoreBucket(c.objectStore),
		ObjectKey:          objectKey,
		ContentType:        contentType,
		SizeBytes:          int64(len(raw)),
		SHA256:             sha256Value,
		Status:             "previewed",
		Rows:               rows,
		CreatedByAccountID: ctx.AccountID,
		Summary: map[string]any{
			"total":       len(rows),
			"valid":       valid,
			"invalid":     len(rows) - valid,
			"confirmable": valid,
			"error_count": len(rowErrors),
			"filename":    filename,
			"size_bytes":  len(raw),
			"sha256":      sha256Value,
		},
		CreatedAt: c.Now(),
		ExpiresAt: c.Now().Add(24 * time.Hour),
	}
	if err := c.withTransaction(ctx, func(tx HRService) error {
		if err := tx.store.UpsertEmployeeImportSession(goContext(ctx), session); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.import.preview", string(ResourceEmployeeImport), session.ID, string(SeverityMedium), employeeImportAuditDetails(session)); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		return EmployeeImportSession{}, err
	}
	objectCommitted = true
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

// ConfirmEmployeeImport 確認員工 import 的服務流程。
func (c HRService) ConfirmEmployeeImport(ctx RequestContext, sessionID string, input EmployeeImportConfirmInput) (EmployeeImportSession, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return EmployeeImportSession{}, err
	}
	req := CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: sessionID, Action: ActionImport}
	decision, err := c.evaluateAuthz(ctx, account, req)
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
	authzAudit := AuthzAudit{service: c.Service, target: AuditTarget{Event: "hr.employee.import.confirm", Resource: string(ResourceEmployeeImport), Target: sessionID}, decision: decision}
	mode, err := normalizeEmployeeImportMode(input.Mode)
	if err != nil {
		return EmployeeImportSession{}, err
	}
	if err := validateEmployeeImportFailurePolicy(input.FailurePolicy); err != nil {
		return EmployeeImportSession{}, err
	}
	var session EmployeeImportSession
	confirmedCount := 0
	createdCount := 0
	updatedCount := 0
	provisionQueued := false
	var validationErr error
	if err := c.withTransaction(ctx, func(tx HRService) error {
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
			previous Employee
			update   bool
		}
		employees := make([]importEmployeeWrite, 0, len(next.Rows))
		reservedEmployeeNos := map[string]struct{}{}
		batch := newEmployeeImportBatchIndex()
		for i, row := range next.Rows {
			row.Errors = nil
			employee, previous, update, errors, err := tx.prepareEmployeeImportWrite(ctx, row, batch, mode, reservedEmployeeNos)
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
			scopeErrors, err := tx.employeeImportScopeErrors(ctx, account, row.RowNumber, employee, previous, update, decision)
			if err != nil {
				return err
			}
			if len(scopeErrors) > 0 {
				row.Errors = scopeErrors
				row.Valid = false
				rowErrors = append(rowErrors, scopeErrors...)
				results = append(results, BatchEmployeeResult{RowNumber: row.RowNumber, Success: false, Code: "import_validation_failed", Message: firstRowErrorMessage(scopeErrors)})
				next.Rows[i] = row
				continue
			}
			row.Valid = true
			next.Rows[i] = row
			employees = append(employees, importEmployeeWrite{row: row, employee: employee, previous: previous, update: update})
		}
		if len(rowErrors) > 0 {
			tx.logWarn(ctx, "employee import confirmation failed validation",
				"session_id", next.ID,
				"total_rows", len(next.Rows),
				"error_count", len(rowErrors),
			)
			next.Status = "failed_validation"
			next.ConfirmedByAccountID = ctx.AccountID
			if next.Summary == nil {
				next.Summary = map[string]any{}
			}
			next.Summary["total"] = len(next.Rows)
			next.Summary["confirmed"] = 0
			next.Summary["created"] = 0
			next.Summary["updated"] = 0
			next.Summary["failed"] = len(next.Rows)
			next.Summary["results"] = results
			next.Summary["row_errors"] = rowErrors
			next.Summary["error_count"] = len(rowErrors)
			next.Summary["mode"] = mode
			next.Summary["failure_policy"] = "all_or_nothing"
			if err := tx.store.UpsertEmployeeImportSession(goContext(ctx), next); err != nil {
				return err
			}
			if err := tx.audit(ctx, "hr.employee.import.confirm_failed", string(ResourceEmployeeImport), next.ID, string(SeverityHigh), employeeImportAuditDetails(next)); err != nil {
				return err
			}
			if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
				return err
			}
			session = next
			validationErr = ImportValidationFailed("employee import contains invalid rows", rowErrors)
			return nil
		}
		for _, item := range employees {
			employee := item.employee
			accountPolicy := string(EmployeeAccountPolicyNone)
			if !item.update {
				var err error
				accountPolicy, _, err = tx.applyEmployeeCreateAccountPolicy(ctx, &employee, item.row.Employee)
				if err != nil {
					return err
				}
			}
			if err := tx.store.UpsertEmployee(goContext(ctx), employee); err != nil {
				return err
			}
			previous := item.previous
			if err := tx.touchEmployeeAuthzIfNeeded(ctx, previous, employee, string(EventEmployeeAuthzSubjectImport)); err != nil {
				return err
			}
			if err := tx.linkEmployeeAccount(ctx, employee); err != nil {
				return err
			}
			if employee.AccountID != "" && accountPolicy != string(EmployeeAccountPolicyNone) {
				sendInvite := accountPolicy == string(EmployeeAccountPolicyCreatePendingInvite)
				if err := tx.provisionEmployeeIdentityFromAccountID(ctx, employee, employee.AccountID, sendInvite); err != nil {
					return err
				}
				provisionQueued = true
			}
			eventType := string(EventEmployeeCreated)
			action := "created"
			if item.update {
				eventType = string(EventEmployeeUpdated)
				action = "updated"
				updatedCount++
			} else {
				createdCount++
			}
			if err := tx.appendEmployeeEvent(ctx, eventType, employee.ID, map[string]any{"employee_id": employee.ID, "import_session_id": next.ID, "action": action}); err != nil {
				return err
			}
			results = append(results, BatchEmployeeResult{RowNumber: item.row.RowNumber, EmployeeID: employee.ID, Success: true, Message: action})
		}
		now := tx.Now()
		next.Status = "confirmed"
		next.ConfirmedAt = &now
		if next.Summary == nil {
			next.Summary = map[string]any{}
		}
		next.Summary["total"] = len(next.Rows)
		next.Summary["confirmed"] = len(employees)
		next.Summary["created"] = createdCount
		next.Summary["updated"] = updatedCount
		next.Summary["failed"] = 0
		next.Summary["results"] = results
		next.Summary["row_errors"] = rowErrors
		next.Summary["error_count"] = len(rowErrors)
		next.Summary["mode"] = mode
		next.Summary["failure_policy"] = "all_or_nothing"
		next.ConfirmedByAccountID = ctx.AccountID
		if err := tx.store.UpsertEmployeeImportSession(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeImported), next.ID, map[string]any{"session_id": next.ID, "success": len(employees), "created": createdCount, "updated": updatedCount, "failed": len(next.Rows) - len(employees), "mode": mode}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.import.confirm", string(ResourceEmployeeImport), next.ID, string(SeverityHigh), employeeImportAuditDetails(next)); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		confirmedCount = len(employees)
		session = next
		return nil
	}); err != nil {
		return EmployeeImportSession{}, err
	}
	if validationErr != nil {
		return session, validationErr
	}
	if provisionQueued {
		c.runIdentityProvisioningFastPath(ctx)
	}
	c.logInfo(ctx, "employee import confirmed",
		"session_id", session.ID,
		"total_rows", len(session.Rows),
		"confirmed_count", confirmedCount,
		"failed_count", len(session.Rows)-confirmedCount,
		"mode", mode,
	)
	return session, nil
}

// employeeImportScopeErrors 處理員工 import 範圍錯誤的服務流程。
func (c HRService) employeeImportScopeErrors(ctx RequestContext, account Account, rowNumber int, employee Employee, previous Employee, update bool, decision CheckResult) ([]RowError, error) {
	targets := []Employee{employee}
	if update {
		targets = append(targets, previous)
	}
	visible, err := c.filterEmployeesByDecision(ctx, account, targets, decision)
	if err != nil {
		return nil, err
	}
	if len(visible) == len(targets) {
		return nil, nil
	}
	return []RowError{{
		Row:     rowNumber,
		Field:   "authz_scope",
		Code:    "out_of_scope",
		Message: "employee import row is outside authorized scope",
	}}, nil
}

type employeeImportBatchIndex struct {
	employeeNos   map[string]int
	companyEmails map[string]int
	accountIDs    map[string]int
}

// newEmployeeImportBatchIndex 建立員工 import 批次 index。
func newEmployeeImportBatchIndex() *employeeImportBatchIndex {
	return &employeeImportBatchIndex{
		employeeNos:   map[string]int{},
		companyEmails: map[string]int{},
		accountIDs:    map[string]int{},
	}
}

// terminalEmployeeImportStatus 處理 terminal 員工 import 狀態。
func terminalEmployeeImportStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "confirmed", "partially_confirmed", "failed", "failed_validation":
		return true
	default:
		return false
	}
}

// prepareEmployeeImportWrite 處理 prepare 員工 import write 的服務流程。
func (c HRService) prepareEmployeeImportWrite(ctx RequestContext, row EmployeeImportRow, batch *employeeImportBatchIndex, mode string, reservedEmployeeNos map[string]struct{}) (Employee, Employee, bool, []RowError, error) {
	batchErrors := employeeImportBatchErrors(row, batch)
	if mode == employeeImportModeUpdate {
		return c.prepareEmployeeImportUpdate(ctx, row, batchErrors)
	}
	if mode == employeeImportModeUpsert {
		existing, ok, err := c.employeeImportExistingByEmployeeNo(ctx, row.Employee)
		if err != nil {
			return Employee{}, Employee{}, false, nil, err
		}
		if ok {
			return c.prepareEmployeeImportUpdateWithExisting(ctx, row, existing, batchErrors)
		}
	}
	employee, err := c.employeeFromImportInput(ctx, row.Employee, reservedEmployeeNos)
	if err != nil {
		errors, ok := employeeImportErrorsFromError(row.RowNumber, err)
		if !ok {
			return Employee{}, Employee{}, false, nil, err
		}
		return Employee{}, Employee{}, false, append(errors, batchErrors...), nil
	}
	return employee, Employee{}, false, batchErrors, nil
}

// prepareEmployeeImportUpdate 處理 prepare 員工 import update 的服務流程。
func (c HRService) prepareEmployeeImportUpdate(ctx RequestContext, row EmployeeImportRow, batchErrors []RowError) (Employee, Employee, bool, []RowError, error) {
	existing, ok, err := c.employeeImportExistingByEmployeeNo(ctx, row.Employee)
	if err != nil {
		return Employee{}, Employee{}, false, nil, err
	}
	if strings.TrimSpace(row.Employee.EmployeeNo) == "" {
		errors := []RowError{{Row: row.RowNumber, Field: "employee_no", Code: "required", Message: "employee_no is required for update imports"}}
		return Employee{}, Employee{}, false, append(errors, batchErrors...), nil
	}
	if !ok {
		errors := []RowError{{Row: row.RowNumber, Field: "employee_no", Code: "not_found", Message: "employee_no was not found for update import"}}
		return Employee{}, Employee{}, false, append(errors, batchErrors...), nil
	}
	return c.prepareEmployeeImportUpdateWithExisting(ctx, row, existing, batchErrors)
}

// prepareEmployeeImportUpdateWithExisting 處理 prepare 員工 import update with existing 的服務流程。
func (c HRService) prepareEmployeeImportUpdateWithExisting(ctx RequestContext, row EmployeeImportRow, existing Employee, batchErrors []RowError) (Employee, Employee, bool, []RowError, error) {
	candidate, err := c.employeeCreateCandidate(ctx, row.Employee)
	if err != nil {
		errors, ok := employeeImportErrorsFromError(row.RowNumber, err)
		if !ok {
			return Employee{}, Employee{}, false, nil, err
		}
		return Employee{}, Employee{}, false, append(errors, batchErrors...), nil
	}
	next := employeeImportUpdateEmployee(existing, candidate, row.Employee)
	if err := c.ensureEmployeePosition(ctx, &next, true); err != nil {
		errors, ok := employeeImportErrorsFromError(row.RowNumber, err)
		if !ok {
			return Employee{}, Employee{}, false, nil, err
		}
		return Employee{}, Employee{}, false, append(errors, batchErrors...), nil
	}
	if err := c.validateEmployee(ctx, next, "update", employeeValidationImportMinimal); err != nil {
		errors, ok := employeeImportErrorsFromError(row.RowNumber, err)
		if !ok {
			return Employee{}, Employee{}, false, nil, err
		}
		return Employee{}, Employee{}, false, append(errors, batchErrors...), nil
	}
	return next, existing, true, batchErrors, nil
}

// employeeImportExistingByEmployeeNo 處理員工 import existing by 員工 no 的服務流程。
func (c HRService) employeeImportExistingByEmployeeNo(ctx RequestContext, input CreateEmployeeInput) (Employee, bool, error) {
	employeeNo := strings.TrimSpace(input.EmployeeNo)
	if employeeNo == "" {
		return Employee{}, false, nil
	}
	return c.store.GetEmployeeByEmployeeNo(goContext(ctx), ctx.TenantID, employeeNo)
}

// employeeImportUpdateEmployee 處理員工 import update 員工。
func employeeImportUpdateEmployee(existing Employee, candidate Employee, input CreateEmployeeInput) Employee {
	next := existing
	if strings.TrimSpace(input.EmployeeNo) != "" {
		next.EmployeeNo = candidate.EmployeeNo
	}
	if strings.TrimSpace(input.Name) != "" {
		next.Name = candidate.Name
	}
	if strings.TrimSpace(input.CompanyEmail) != "" {
		next.CompanyEmail = candidate.CompanyEmail
	}
	if strings.TrimSpace(input.Phone) != "" {
		next.Phone = candidate.Phone
	}
	if strings.TrimSpace(input.OrgUnitID) != "" {
		next.OrgUnitID = candidate.OrgUnitID
	}
	if strings.TrimSpace(input.ManagerEmployeeID) != "" {
		next.ManagerEmployeeID = candidate.ManagerEmployeeID
	}
	if strings.TrimSpace(input.PositionID) != "" {
		next.PositionID = candidate.PositionID
	}
	if strings.TrimSpace(input.Position) != "" {
		next.Position = candidate.Position
	}
	if strings.TrimSpace(input.Category) != "" {
		next.Category = candidate.Category
	}
	if strings.TrimSpace(input.Status) != "" || strings.TrimSpace(input.EmploymentStatus) != "" {
		next.Status = candidate.Status
		next.EmploymentStatus = candidate.EmploymentStatus
	}
	if strings.TrimSpace(input.HireDate) != "" {
		next.HireDate = candidate.HireDate
	}
	next.BasicInfo = mergeEmployeeImportMap(next.BasicInfo, candidate.BasicInfo)
	next.EmploymentInfo = mergeEmployeeImportMap(next.EmploymentInfo, candidate.EmploymentInfo)
	next.ContactInfo = mergeEmployeeImportMap(next.ContactInfo, candidate.ContactInfo)
	next.UpdatedAt = candidate.UpdatedAt
	next.Category = normalizeEmployeeCategory(next.Category)
	if next.EmploymentStatus == "" {
		next.EmploymentStatus = next.Status
	}
	if next.Status == "" {
		next.Status = next.EmploymentStatus
	}
	next.EmploymentStatus = normalizeEmployeeStatus(next.EmploymentStatus)
	next.Status = normalizeEmployeeStatus(next.Status)
	return next
}

// mergeEmployeeImportMap 合併員工 import map。
func mergeEmployeeImportMap(existing map[string]any, updates map[string]any) map[string]any {
	out := utils.CopyStringMap(existing)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range updates {
		if strings.TrimSpace(fmt.Sprint(value)) == "" {
			continue
		}
		out[key] = value
	}
	return out
}

// validateEmployeeImportRow 驗證員工 import 列的服務流程。
func (c HRService) validateEmployeeImportRow(ctx RequestContext, row EmployeeImportRow, batch *employeeImportBatchIndex) ([]RowError, error) {
	employee, err := c.employeeCreateCandidate(ctx, row.Employee)
	if err != nil {
		errors, ok := employeeImportErrorsFromError(row.RowNumber, err)
		if ok {
			return append(errors, employeeImportBatchErrors(row, batch)...), nil
		}
		return nil, err
	}
	err = c.validateEmployee(ctx, employee, "create", employeeValidationImportMinimal)
	errors, ok := employeeImportErrorsFromError(row.RowNumber, err)
	if err != nil && !ok {
		return nil, err
	}
	errors = append(errors, employeeImportAccountPolicyErrors(row)...)
	errors = append(errors, employeeImportBatchErrors(row, batch)...)
	return errors, nil
}

// employeeImportAccountPolicyErrors 處理員工 import 帳號政策錯誤。
func employeeImportAccountPolicyErrors(row EmployeeImportRow) []RowError {
	policy := strings.TrimSpace(row.Employee.AccountPolicy)
	if policy == "" || validEmployeeAccountPolicy(policy) {
		return nil
	}
	return []RowError{{Row: row.RowNumber, Field: "account_policy", Code: "invalid", Message: "account_policy must be one of none, link_existing, create_pending_invite, create_active"}}
}

// employeeImportBatchErrors 處理員工 import 批次錯誤。
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

// employeeImportErrorsFromError 處理員工 import 錯誤 來源 錯誤。
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

// firstRowErrorMessage 取得第一個列錯誤 message。
func firstRowErrorMessage(errors []RowError) string {
	if len(errors) == 0 {
		return "employee import row failed"
	}
	return errors[0].Message
}

const maxEmployeeExportRows = 5000

// ExportEmployeesCSV 匯出員工 CSV 的服務流程。
