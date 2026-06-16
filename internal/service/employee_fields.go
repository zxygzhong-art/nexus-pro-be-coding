package service

import "strings"

type employeeFieldSource struct {
	tab string
	key string
}

const (
	employeeTabBasicInfo             = "basic_info"
	employeeTabEmploymentInfo        = "employment_info"
	employeeTabEducationMilitaryInfo = "education_military_info"
	employeeTabContactInfo           = "contact_info"
	employeeTabInsuranceInfo         = "insurance_info"
)

var employeeHotFieldSources = map[string][]employeeFieldSource{
	"company_email": {
		{tab: employeeTabBasicInfo, key: "company_email"},
		{tab: employeeTabBasicInfo, key: "email"},
	},
	"personal_email": {
		{tab: employeeTabBasicInfo, key: "personal_email"},
	},
	"phone": {
		{tab: employeeTabContactInfo, key: "mobile_phone"},
		{tab: employeeTabContactInfo, key: "phone"},
	},
	"org_unit_id": {
		{tab: employeeTabEmploymentInfo, key: "org_unit_id"},
		{tab: employeeTabEmploymentInfo, key: "department_id"},
	},
	"manager_employee_id": {
		{tab: employeeTabEmploymentInfo, key: "manager_employee_id"},
		{tab: employeeTabBasicInfo, key: "manager_employee_id"},
	},
	"position": {
		{tab: employeeTabEmploymentInfo, key: "position"},
		{tab: employeeTabEmploymentInfo, key: "job_title"},
	},
	"category": {
		{tab: employeeTabEmploymentInfo, key: "category"},
	},
	"name": {
		{tab: employeeTabBasicInfo, key: "name"},
	},
}

type employeePatchPolicyField struct {
	tab     string
	field   string
	present func(UpdateEmployeeInput) bool
}

var employeeScalarPatchPolicyFields = []employeePatchPolicyField{
	{tab: employeeTabBasicInfo, field: "employee_no", present: func(input UpdateEmployeeInput) bool { return input.EmployeeNo != nil }},
	{tab: employeeTabBasicInfo, field: "name", present: func(input UpdateEmployeeInput) bool { return input.Name != nil }},
	{tab: employeeTabBasicInfo, field: "company_email", present: func(input UpdateEmployeeInput) bool { return input.CompanyEmail != nil }},
	{tab: employeeTabBasicInfo, field: "personal_email", present: func(input UpdateEmployeeInput) bool { return input.PersonalEmail != nil }},
	{tab: employeeTabContactInfo, field: "phone", present: func(input UpdateEmployeeInput) bool { return input.Phone != nil }},
	{tab: employeeTabEmploymentInfo, field: "org_unit_id", present: func(input UpdateEmployeeInput) bool { return input.OrgUnitID != nil }},
	{tab: employeeTabBasicInfo, field: "account_id", present: func(input UpdateEmployeeInput) bool { return input.AccountID != nil }},
	{tab: employeeTabEmploymentInfo, field: "manager_employee_id", present: func(input UpdateEmployeeInput) bool { return input.ManagerEmployeeID != nil }},
	{tab: employeeTabEmploymentInfo, field: "position", present: func(input UpdateEmployeeInput) bool { return input.Position != nil }},
	{tab: employeeTabEmploymentInfo, field: "category", present: func(input UpdateEmployeeInput) bool { return input.Category != nil }},
	{tab: employeeTabEmploymentInfo, field: "status", present: func(input UpdateEmployeeInput) bool { return input.Status != nil }},
	{tab: employeeTabEmploymentInfo, field: "employment_status", present: func(input UpdateEmployeeInput) bool { return input.EmploymentStatus != nil }},
	{tab: employeeTabEmploymentInfo, field: "hire_date", present: func(input UpdateEmployeeInput) bool { return input.HireDate != nil }},
	{tab: employeeTabEmploymentInfo, field: "resign_date", present: func(input UpdateEmployeeInput) bool { return input.ResignDate != nil }},
	{tab: employeeTabEmploymentInfo, field: "internal_experiences", present: func(input UpdateEmployeeInput) bool { return input.InternalExperiences != nil }},
}

type employeePatchMapPolicyField struct {
	tab    string
	values func(UpdateEmployeeInput) map[string]any
}

var employeeMapPatchPolicyFields = []employeePatchMapPolicyField{
	{tab: employeeTabBasicInfo, values: func(input UpdateEmployeeInput) map[string]any { return input.BasicInfo }},
	{tab: employeeTabEmploymentInfo, values: func(input UpdateEmployeeInput) map[string]any { return input.EmploymentInfo }},
	{tab: employeeTabEducationMilitaryInfo, values: func(input UpdateEmployeeInput) map[string]any { return input.EducationMilitaryInfo }},
	{tab: employeeTabContactInfo, values: func(input UpdateEmployeeInput) map[string]any { return input.ContactInfo }},
	{tab: employeeTabInsuranceInfo, values: func(input UpdateEmployeeInput) map[string]any { return input.InsuranceInfo }},
}

type employeeExportColumn struct {
	header string
	value  func(Employee) string
}

var employeeExportColumns = []employeeExportColumn{
	{header: "員工編號", value: func(employee Employee) string { return employee.EmployeeNo }},
	{header: "姓名", value: func(employee Employee) string { return employee.Name }},
	{header: "Email", value: func(employee Employee) string { return employee.CompanyEmail }},
	{header: "部門", value: func(employee Employee) string { return employee.OrgUnitID }},
	{header: "職位", value: func(employee Employee) string { return employee.Position }},
	{header: "類別", value: func(employee Employee) string { return employee.Category }},
	{header: "電話", value: func(employee Employee) string { return employee.Phone }},
	{header: "狀態", value: func(employee Employee) string { return employeeStatus(employee) }},
	{header: "到職日期", value: func(employee Employee) string { return formatDate(employee.HireDate) }},
	{header: "主管員工ID", value: func(employee Employee) string { return employee.ManagerEmployeeID }},
}

const (
	employeeImportColumnEmployeeNo = iota
	employeeImportColumnName
	employeeImportColumnEmail
	employeeImportColumnOrgUnit
	employeeImportColumnPosition
	employeeImportColumnCategory
	employeeImportColumnPhone
	employeeImportColumnStatus
	employeeImportColumnHireDate
	employeeImportColumnManagerEmployeeID
)

type employeeImportColumn struct {
	header string
}

var employeeImportColumns = []employeeImportColumn{
	{header: "員工編號"},
	{header: "姓名"},
	{header: "Email"},
	{header: "部門"},
	{header: "職位"},
	{header: "類別"},
	{header: "電話"},
	{header: "狀態"},
	{header: "到職日期"},
	{header: "主管員工ID"},
}

func employeeImportColumnCount() int {
	return len(employeeImportColumns)
}

func employeeImportInputFromRecord(record []string) map[string]string {
	input := make(map[string]string, len(employeeImportColumns))
	for i, column := range employeeImportColumns {
		input[column.header] = record[i]
	}
	return input
}

func employeeCreateInputFromImportRecord(record []string) CreateEmployeeInput {
	email := strings.TrimSpace(record[employeeImportColumnEmail])
	name := strings.TrimSpace(record[employeeImportColumnName])
	orgUnitID := strings.TrimSpace(record[employeeImportColumnOrgUnit])
	managerEmployeeID := strings.TrimSpace(record[employeeImportColumnManagerEmployeeID])
	position := strings.TrimSpace(record[employeeImportColumnPosition])
	category := normalizeEmployeeCategory(record[employeeImportColumnCategory])
	phone := strings.TrimSpace(record[employeeImportColumnPhone])
	status := normalizeEmployeeStatus(record[employeeImportColumnStatus])
	return CreateEmployeeInput{
		EmployeeNo:        strings.TrimSpace(record[employeeImportColumnEmployeeNo]),
		Name:              name,
		CompanyEmail:      email,
		OrgUnitID:         orgUnitID,
		ManagerEmployeeID: managerEmployeeID,
		Position:          position,
		Category:          category,
		Phone:             phone,
		EmploymentStatus:  status,
		Status:            status,
		HireDate:          normalizeImportDate(record[employeeImportColumnHireDate]),
		BasicInfo:         map[string]any{"company_email": email, "name": name},
		EmploymentInfo: map[string]any{
			"org_unit_id":         orgUnitID,
			"manager_employee_id": managerEmployeeID,
			"position":            position,
			"category":            category,
		},
		ContactInfo: map[string]any{"mobile_phone": phone},
	}
}

func employeeHotValue(employee Employee, field string) string {
	for _, source := range employeeHotFieldSources[field] {
		value := employeeSourceValue(employee, source)
		if value != "" {
			return value
		}
	}
	return ""
}

func employeeSourceValue(employee Employee, source employeeFieldSource) string {
	switch source.tab {
	case employeeTabBasicInfo:
		return stringFromMap(employee.BasicInfo, source.key)
	case employeeTabEmploymentInfo:
		return stringFromMap(employee.EmploymentInfo, source.key)
	case employeeTabEducationMilitaryInfo:
		return stringFromMap(employee.EducationMilitaryInfo, source.key)
	case employeeTabContactInfo:
		return stringFromMap(employee.ContactInfo, source.key)
	case employeeTabInsuranceInfo:
		return stringFromMap(employee.InsuranceInfo, source.key)
	default:
		return ""
	}
}
