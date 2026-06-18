package service

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

const maxEmployeeAvatarBytes = 2 << 20

func (c HRService) PreviewCreateEmployee(ctx RequestContext, input CreateEmployeeInput) (EmployeePreviewResponse, error) {
	_, _, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionCreate},
		AuditTarget{Event: "hr.employee.preview_create", Resource: string(ResourceEmployee)},
	)
	if err != nil {
		return EmployeePreviewResponse{}, err
	}
	employee, err := c.employeeCreateCandidate(ctx, input)
	if err != nil {
		return EmployeePreviewResponse{}, err
	}
	if len(employee.InternalExperiences) == 0 {
		employee.InternalExperiences = append(employee.InternalExperiences, c.newEmployeeExperience(employee, "新進"))
	}
	resp := employeePreviewResponse(employee, nil)
	if err := c.validateEmployee(ctx, employee, "create", employeeValidationFullForm); err != nil {
		if appErr, ok := domain.AsAppError(err); ok && appErr.Code == "validation_failed" {
			resp.FieldErrors = appErr.FieldErrors
			resp.Valid = false
		} else {
			return EmployeePreviewResponse{}, err
		}
	}
	if err := authzAudit.Commit(ctx); err != nil {
		return EmployeePreviewResponse{}, err
	}
	return resp, nil
}

func (c HRService) PreviewUpdateEmployee(ctx RequestContext, id string, input UpdateEmployeeInput) (EmployeePreviewResponse, error) {
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionUpdate},
		AuditTarget{Event: "hr.employee.preview_update", Resource: string(ResourceEmployee), Target: id},
	)
	if err != nil {
		return EmployeePreviewResponse{}, err
	}
	employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return EmployeePreviewResponse{}, err
	}
	if !ok {
		return EmployeePreviewResponse{}, NotFound("employee", id)
	}
	visible, err := c.filterEmployeesByDecision(ctx, account, []Employee{employee}, decision)
	if err != nil {
		return EmployeePreviewResponse{}, err
	}
	if len(visible) == 0 {
		return EmployeePreviewResponse{}, forbiddenDataScope("employee is outside data scope")
	}
	if fields := forbiddenEmployeePatchFields(input, decision.FieldPolicies); len(fields) > 0 {
		return EmployeePreviewResponse{}, domainValidation("employee field policy denied update", fields...)
	}
	before := employee
	err = c.applyEmployeePatch(ctx, &employee, input)
	resp := employeePreviewResponse(employee, employeeDiff(before, employee))
	if err != nil {
		if appErr, ok := domain.AsAppError(err); ok && appErr.Code == "validation_failed" {
			resp.FieldErrors = appErr.FieldErrors
			resp.Valid = false
		} else {
			return EmployeePreviewResponse{}, err
		}
	}
	if err := authzAudit.Commit(ctx); err != nil {
		return EmployeePreviewResponse{}, err
	}
	return resp, nil
}

func (c HRService) UpdateEmployeeAvatar(ctx RequestContext, id string, input EmployeeAvatarInput) (Employee, error) {
	contentType, err := validateEmployeeAvatarInput(input)
	if err != nil {
		return Employee{}, err
	}
	input.ContentType = contentType
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionUpdate},
		AuditTarget{Event: "hr.employee.avatar.update", Resource: string(ResourceEmployee), Target: id},
	)
	if err != nil {
		return Employee{}, err
	}
	if employeeAvatarDenied(decision.FieldPolicies) {
		return Employee{}, domainValidation("employee field policy denied update", FieldError{Tab: employeeTabBasicInfo, Field: "avatar", Code: "field_denied", Message: "avatar cannot be updated by current permission policy"})
	}
	var employee Employee
	var oldKey string
	var newKey string
	newObjectWritten := false
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
		oldKey = stringFromMap(next.BasicInfo, "avatar_object_key")
		newKey = employeeAvatarObjectKey(ctx.TenantID, next.ID, input.Filename, input.ContentType)
		if err := tx.objectStore.PutObject(goContext(ctx), newKey, input.ContentType, input.Content); err != nil {
			return BadRequest("store avatar: " + err.Error())
		}
		newObjectWritten = true
		next.BasicInfo = mergeMap(next.BasicInfo, map[string]any{
			"avatar": map[string]any{
				"object_key":    newKey,
				"content_type":  input.ContentType,
				"original_name": strings.TrimSpace(input.Filename),
			},
			"avatar_object_key":   newKey,
			"avatar_content_type": input.ContentType,
		})
		next.UpdatedAt = tx.Now()
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.avatar.update", string(ResourceEmployee), next.ID, string(SeverityMedium), auditDecisionDetails(ctx, decision, map[string]any{"object_key": newKey})); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx); err != nil {
			return err
		}
		employee = next
		return nil
	}); err != nil {
		if newObjectWritten && newKey != "" {
			c.deleteObjectIfSupported(ctx, newKey)
		}
		return Employee{}, err
	}
	if oldKey != "" && oldKey != newKey {
		c.deleteObjectIfSupported(ctx, oldKey)
	}
	return employee, nil
}

func (c HRService) DeleteEmployeeAvatar(ctx RequestContext, id string) (Employee, error) {
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionUpdate},
		AuditTarget{Event: "hr.employee.avatar.delete", Resource: string(ResourceEmployee), Target: id},
	)
	if err != nil {
		return Employee{}, err
	}
	if employeeAvatarDenied(decision.FieldPolicies) {
		return Employee{}, domainValidation("employee field policy denied update", FieldError{Tab: employeeTabBasicInfo, Field: "avatar", Code: "field_denied", Message: "avatar cannot be updated by current permission policy"})
	}
	var employee Employee
	var oldKey string
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
		oldKey = stringFromMap(next.BasicInfo, "avatar_object_key")
		next.BasicInfo = utils.CopyStringMap(next.BasicInfo)
		delete(next.BasicInfo, "avatar")
		delete(next.BasicInfo, "avatar_object_key")
		delete(next.BasicInfo, "avatar_content_type")
		next.UpdatedAt = tx.Now()
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.avatar.delete", string(ResourceEmployee), next.ID, string(SeverityMedium), auditDecisionDetails(ctx, decision, map[string]any{"object_key": oldKey})); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx); err != nil {
			return err
		}
		employee = next
		return nil
	}); err != nil {
		return Employee{}, err
	}
	if oldKey != "" {
		c.deleteObjectIfSupported(ctx, oldKey)
	}
	return employee, nil
}

func (c HRService) EmployeeImportTemplate(ctx RequestContext, format string) ([]byte, string, string, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return nil, "", "", err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionRead})
	if err != nil {
		return nil, "", "", err
	}
	if !decision.Allowed {
		return nil, "", "", forbiddenAuthz(decision)
	}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "csv":
		raw, err := employeeImportTemplateCSV()
		return raw, "employee-import-template.csv", "text/csv; charset=utf-8", err
	case "xlsx":
		raw, err := employeeImportTemplateXLSX()
		return raw, "employee-import-template.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", err
	default:
		return nil, "", "", BadRequest("format must be csv or xlsx")
	}
}

func employeePreviewResponse(employee Employee, diff map[string]any) EmployeePreviewResponse {
	return EmployeePreviewResponse{Employee: employee, Diff: diff, Valid: true}
}

func employeeQueryApprovalFilters(query EmployeeQuery) map[string]any {
	out := map[string]any{}
	if query.Keyword != "" {
		out["keyword"] = query.Keyword
	}
	if query.DepartmentID != "" {
		out["department_id"] = query.DepartmentID
	}
	if query.EmploymentStatus != "" {
		out["employment_status"] = query.EmploymentStatus
	}
	if query.Category != "" {
		out["category"] = query.Category
	}
	return out
}

func employeeDiff(before, after Employee) map[string]any {
	diff := map[string]any{}
	add := func(field string, oldValue, newValue any) {
		if !reflect.DeepEqual(oldValue, newValue) {
			diff[field] = map[string]any{"before": oldValue, "after": newValue}
		}
	}
	add("employee_no", before.EmployeeNo, after.EmployeeNo)
	add("name", before.Name, after.Name)
	add("company_email", before.CompanyEmail, after.CompanyEmail)
	add("personal_email", before.PersonalEmail, after.PersonalEmail)
	add("phone", before.Phone, after.Phone)
	add("org_unit_id", before.OrgUnitID, after.OrgUnitID)
	add("account_id", before.AccountID, after.AccountID)
	add("manager_employee_id", before.ManagerEmployeeID, after.ManagerEmployeeID)
	add("position", before.Position, after.Position)
	add("category", before.Category, after.Category)
	add("status", before.Status, after.Status)
	add("employment_status", before.EmploymentStatus, after.EmploymentStatus)
	add("basic_info", before.BasicInfo, after.BasicInfo)
	add("employment_info", before.EmploymentInfo, after.EmploymentInfo)
	add("education_military_info", before.EducationMilitaryInfo, after.EducationMilitaryInfo)
	add("contact_info", before.ContactInfo, after.ContactInfo)
	add("insurance_info", before.InsuranceInfo, after.InsuranceInfo)
	add("internal_experiences", before.InternalExperiences, after.InternalExperiences)
	if len(diff) == 0 {
		return nil
	}
	return diff
}

func validateEmployeeAvatarInput(input EmployeeAvatarInput) (string, error) {
	if len(input.Content) == 0 {
		return "", BadRequest("avatar file is required")
	}
	if len(input.Content) > maxEmployeeAvatarBytes {
		return "", BadRequest("avatar file exceeds 2MB limit")
	}
	declared := normalizedEmployeeAvatarContentType(input.ContentType)
	detected := detectEmployeeAvatarContentType(input.Content)
	if detected == "" {
		return "", BadRequest("avatar file must be a valid image/png, image/jpeg or image/webp")
	}
	if declared != "" && declared != detected {
		return "", BadRequest("avatar content_type does not match file content")
	}
	return detected, nil
}

func normalizedEmployeeAvatarContentType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if idx := strings.Index(value, ";"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	return value
}

func detectEmployeeAvatarContentType(raw []byte) string {
	switch {
	case bytes.HasPrefix(raw, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}):
		return "image/png"
	case len(raw) >= 3 && raw[0] == 0xff && raw[1] == 0xd8 && raw[2] == 0xff:
		return "image/jpeg"
	case len(raw) >= 12 && bytes.Equal(raw[0:4], []byte("RIFF")) && bytes.Equal(raw[8:12], []byte("WEBP")):
		return "image/webp"
	default:
		return ""
	}
}

func employeeAvatarDenied(policies map[string]string) bool {
	for _, field := range []string{"basic_info", "avatar", "avatar_object_key", "avatar_content_type"} {
		switch policies[field] {
		case "deny", "hide", "readonly":
			return true
		}
	}
	return false
}

func employeeAvatarObjectKey(tenantID, employeeID, filename, contentType string) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	if ext == "" {
		switch strings.ToLower(strings.TrimSpace(contentType)) {
		case "image/png":
			ext = ".png"
		case "image/jpeg":
			ext = ".jpg"
		case "image/webp":
			ext = ".webp"
		}
	}
	return "employee-avatars/" + tenantID + "/" + employeeID + "/" + utils.NewID("avatar") + ext
}

func (c *Service) deleteObjectIfSupported(ctx RequestContext, key string) {
	deleter, ok := c.objectStore.(objectDeleter)
	if !ok {
		return
	}
	_ = deleter.DeleteObject(goContext(ctx), key)
}

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

func employeeImportTemplateHeaders() []string {
	headers := make([]string, 0, len(employeeImportColumns))
	for _, column := range employeeImportColumns {
		headers = append(headers, column.header)
	}
	return headers
}

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
