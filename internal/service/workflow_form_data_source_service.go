package service

import (
	"sort"
	"strings"

	"nexus-pro-api/internal/domain"
)

const (
	formDataSourceCurrentUser = "current_user"
	formDataSourceDepartments = "departments"
	formDataSourceEmployees   = "employees"
	formDataSourcePositions   = "positions"
	formDataSourceLeaveTypes  = "leave_types"
)

var formDataSourceAllowedFields = map[string]map[string]struct{}{
	formDataSourceCurrentUser: {"display_name": {}, "email": {}, "employee_id": {}, "employee_no": {}, "department_id": {}, "department_name": {}, "position_id": {}, "position_name": {}},
	formDataSourceDepartments: {"id": {}, "name": {}, "code": {}},
	formDataSourceEmployees:   {"id": {}, "name": {}, "employee_no": {}, "email": {}, "department_id": {}, "department_name": {}, "position_id": {}, "position_name": {}},
	formDataSourcePositions:   {"id": {}, "name": {}, "code": {}, "department_id": {}},
	formDataSourceLeaveTypes:  {"code": {}, "name": {}, "unit": {}},
}

// ValidateFormFieldBinding validates that a persisted form binding uses an allowlisted data source.
func ValidateFormFieldBinding(fieldID, fieldType string, binding domain.PlatformFormBuilderFieldBinding) []domain.FieldError {
	prefix := "fields." + fieldID + ".binding"
	sourceID := strings.TrimSpace(binding.SourceID)
	allowedFields, sourceOK := formDataSourceAllowedFields[sourceID]
	if !sourceOK {
		return []domain.FieldError{{Field: prefix + ".source_id", Code: "invalid", Message: "unsupported form data source"}}
	}
	if _, ok := allowedFields[strings.TrimSpace(binding.ValueField)]; !ok {
		return []domain.FieldError{{Field: prefix + ".value_field", Code: "invalid", Message: "unsupported data source value field"}}
	}
	isObject := sourceID == formDataSourceCurrentUser
	if isObject && strings.TrimSpace(fieldType) != "autofill" {
		return []domain.FieldError{{Field: prefix, Code: "invalid", Message: "current user bindings require an autofill field"}}
	}
	if !isObject {
		if _, ok := allowedFields[strings.TrimSpace(binding.LabelField)]; !ok {
			return []domain.FieldError{{Field: prefix + ".label_field", Code: "invalid", Message: "collection binding requires a valid label field"}}
		}
		if !isFormDataSourceOptionField(fieldType) {
			return []domain.FieldError{{Field: prefix, Code: "invalid", Message: "collection bindings require an option field"}}
		}
	}
	return nil
}

// isFormDataSourceOptionField 限制 collection 綁定只用於選項型欄位。
func isFormDataSourceOptionField(fieldType string) bool {
	switch strings.TrimSpace(fieldType) {
	case "select", "radio", "multilist", "country", "autofill-select":
		return true
	default:
		return false
	}
}

// FormDataSources 回傳目前租戶可供表單設計與執行階段使用的資料源目錄。
func (c WorkflowService) FormDataSources(ctx RequestContext) (domain.FormDataSourceCatalogResponse, error) {
	if _, _, err := c.RequireWorkflowAuthz(ctx, ResourceFormInstance, ActionRead, ""); err != nil {
		return domain.FormDataSourceCatalogResponse{}, err
	}
	return c.loadFormDataSources(ctx)
}

// loadFormDataSources 從租戶資料重建受控目錄，避免執行階段信任前端快照。
func (c WorkflowService) loadFormDataSources(ctx RequestContext) (domain.FormDataSourceCatalogResponse, error) {
	account, ok, err := c.Service.store.GetAccount(goContext(ctx), ctx.TenantID, ctx.AccountID)
	if err != nil {
		return domain.FormDataSourceCatalogResponse{}, err
	}
	if !ok {
		return domain.FormDataSourceCatalogResponse{}, NotFound("account", ctx.AccountID)
	}
	units, err := c.Service.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return domain.FormDataSourceCatalogResponse{}, err
	}
	positions, err := c.Service.store.ListPositions(goContext(ctx), ctx.TenantID)
	if err != nil {
		return domain.FormDataSourceCatalogResponse{}, err
	}
	employees, err := c.Service.store.ListEmployees(goContext(ctx), ctx.TenantID)
	if err != nil {
		return domain.FormDataSourceCatalogResponse{}, err
	}
	policy, err := c.Service.Attendance().loadAttendancePolicyResponse(ctx)
	if err != nil {
		return domain.FormDataSourceCatalogResponse{}, err
	}

	unitNames := make(map[string]string, len(units))
	departmentRecords := make([]map[string]interface{}, 0, len(units))
	for _, unit := range units {
		unitNames[unit.ID] = unit.Name
		if unit.Closed {
			continue
		}
		departmentRecords = append(departmentRecords, map[string]interface{}{
			"id": unit.ID, "name": unit.Name, "code": unit.Code,
		})
	}
	positionNames := make(map[string]string, len(positions))
	positionRecords := make([]map[string]interface{}, 0, len(positions))
	for _, position := range positions {
		positionNames[position.ID] = position.Name
		if strings.EqualFold(position.Status, string(domain.PositionStatusDisabled)) {
			continue
		}
		positionRecords = append(positionRecords, map[string]interface{}{
			"id": position.ID, "name": position.Name, "code": position.Code, "department_id": position.OrgUnitID,
		})
	}
	employeeRecords := make([]map[string]interface{}, 0, len(employees))
	var currentEmployee domain.Employee
	for _, employee := range employees {
		if employee.AccountID == account.ID {
			currentEmployee = employee
		}
		if strings.EqualFold(employee.Status, string(domain.EmployeeStatusDeleted)) || strings.EqualFold(employee.Status, string(domain.EmployeeStatusResigned)) {
			continue
		}
		employeeRecords = append(employeeRecords, map[string]interface{}{
			"id": employee.ID, "name": employee.Name, "employee_no": employee.EmployeeNo,
			"email": employee.CompanyEmail, "department_id": employee.OrgUnitID,
			"department_name": unitNames[employee.OrgUnitID], "position_id": employee.PositionID,
			"position_name": firstNonEmptyString(positionNames[employee.PositionID], employee.Position),
		})
	}
	leaveTypeRecords := make([]map[string]interface{}, 0, len(policy.LeaveTypes))
	for _, leaveType := range policy.LeaveTypes {
		if !leaveType.Active {
			continue
		}
		leaveTypeRecords = append(leaveTypeRecords, map[string]interface{}{
			"code": leaveType.Code, "name": leaveType.Name, "unit": leaveType.Unit,
		})
	}

	currentUserRecord := map[string]interface{}{
		"id": account.ID, "display_name": account.DisplayName, "email": account.Email,
		"employee_id": currentEmployee.ID, "employee_no": currentEmployee.EmployeeNo,
		"department_id": currentEmployee.OrgUnitID, "department_name": unitNames[currentEmployee.OrgUnitID],
		"position_id":   currentEmployee.PositionID,
		"position_name": firstNonEmptyString(positionNames[currentEmployee.PositionID], currentEmployee.Position),
	}
	sortFormDataSourceRecords(departmentRecords, "name")
	sortFormDataSourceRecords(positionRecords, "name")
	sortFormDataSourceRecords(employeeRecords, "name")
	sortFormDataSourceRecords(leaveTypeRecords, "name")

	return domain.FormDataSourceCatalogResponse{DataSources: []domain.FormDataSource{
		{ID: formDataSourceCurrentUser, Label: "目前登入者", Kind: "object", Fields: currentUserDataSourceFields(), Records: []map[string]interface{}{currentUserRecord}},
		{ID: formDataSourceDepartments, Label: "部門", Kind: "collection", Fields: departmentDataSourceFields(), Records: departmentRecords},
		{ID: formDataSourceEmployees, Label: "員工", Kind: "collection", Fields: employeeDataSourceFields(), Records: employeeRecords},
		{ID: formDataSourcePositions, Label: "崗位", Kind: "collection", Fields: positionDataSourceFields(), Records: positionRecords},
		{ID: formDataSourceLeaveTypes, Label: "假期類別", Kind: "collection", Fields: leaveTypeDataSourceFields(), Records: leaveTypeRecords},
	}}, nil
}

// formDataSourceByID 尋找提交校驗所需的即時資料源。
func formDataSourceByID(catalog domain.FormDataSourceCatalogResponse, sourceID string) (domain.FormDataSource, bool) {
	for _, source := range catalog.DataSources {
		if source.ID == sourceID {
			return source, true
		}
	}
	return domain.FormDataSource{}, false
}

// sortFormDataSourceRecords 讓設計器與執行階段選項順序穩定。
func sortFormDataSourceRecords(records []map[string]interface{}, key string) {
	sort.SliceStable(records, func(i, j int) bool {
		return strings.ToLower(dataSourceString(records[i][key])) < strings.ToLower(dataSourceString(records[j][key]))
	})
}

// dataSourceString 將可暴露欄位正規化為字串值。
func dataSourceString(value interface{}) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func currentUserDataSourceFields() []domain.FormDataSourceField {
	return []domain.FormDataSourceField{
		{Key: "display_name", Label: "姓名", Type: "string"}, {Key: "email", Label: "公司 Email", Type: "string"},
		{Key: "employee_id", Label: "員工 ID", Type: "string"}, {Key: "employee_no", Label: "員工編號", Type: "string"},
		{Key: "department_id", Label: "部門 ID", Type: "string"}, {Key: "department_name", Label: "部門名稱", Type: "string"},
		{Key: "position_id", Label: "崗位 ID", Type: "string"}, {Key: "position_name", Label: "崗位名稱", Type: "string"},
	}
}

func departmentDataSourceFields() []domain.FormDataSourceField {
	return []domain.FormDataSourceField{{Key: "id", Label: "部門 ID", Type: "string"}, {Key: "name", Label: "部門名稱", Type: "string"}, {Key: "code", Label: "部門代碼", Type: "string"}}
}

func employeeDataSourceFields() []domain.FormDataSourceField {
	return []domain.FormDataSourceField{
		{Key: "id", Label: "員工 ID", Type: "string"}, {Key: "name", Label: "員工姓名", Type: "string"},
		{Key: "employee_no", Label: "員工編號", Type: "string"}, {Key: "email", Label: "公司 Email", Type: "string"},
		{Key: "department_id", Label: "部門 ID", Type: "string"}, {Key: "department_name", Label: "部門名稱", Type: "string"},
		{Key: "position_id", Label: "崗位 ID", Type: "string"}, {Key: "position_name", Label: "崗位名稱", Type: "string"},
	}
}

func positionDataSourceFields() []domain.FormDataSourceField {
	return []domain.FormDataSourceField{{Key: "id", Label: "崗位 ID", Type: "string"}, {Key: "name", Label: "崗位名稱", Type: "string"}, {Key: "code", Label: "崗位代碼", Type: "string"}, {Key: "department_id", Label: "部門 ID", Type: "string"}}
}

func leaveTypeDataSourceFields() []domain.FormDataSourceField {
	return []domain.FormDataSourceField{{Key: "code", Label: "假別代碼", Type: "string"}, {Key: "name", Label: "假別名稱", Type: "string"}, {Key: "unit", Label: "計算單位", Type: "string"}}
}
