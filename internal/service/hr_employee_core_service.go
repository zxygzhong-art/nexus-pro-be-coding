package service

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

const (
	defaultEmployeePage     = 1
	defaultEmployeePageSize = 20
	maxEmployeePageSize     = 100
)

// listEmployeesForQuery 列出員工 for 查詢的服務流程。
func (c HRService) listEmployeesForQuery(ctx RequestContext, query EmployeeQuery) ([]Employee, error) {
	return c.store.ListEmployeesByQuery(goContext(ctx), ctx.TenantID, query)
}

// employeeQueryWithDecisionScope 處理員工查詢 with 決策範圍的服務流程。
func (c HRService) employeeQueryWithDecisionScope(ctx RequestContext, account Account, query EmployeeQuery, decision CheckResult) (EmployeeQuery, error) {
	query = normalizeEmployeeQuery(query)
	scope, err := c.employeeScopeConstraint(ctx, account, decision)
	if err != nil {
		return EmployeeQuery{}, err
	}
	query.Scope = scope
	return query, nil
}

// employeeScopeConstraint 處理員工範圍 constraint 的服務流程。
func (c HRService) employeeScopeConstraint(ctx RequestContext, account Account, decision CheckResult) (domain.EmployeeScopeConstraint, error) {
	if decisionUsesOpenFGAScopeCheck(decision) {
		return domain.EmployeeScopeConstraint{}, nil
	}
	switch decision.Scope {
	case "", ScopeAll, ScopeTenant, ScopeSystem:
		return domain.EmployeeScopeConstraint{}, nil
	case ScopeSelf, ScopeOwn:
		ids := stringSliceFromAny(decision.Conditions["employee_ids"])
		if len(ids) == 0 && account.EmployeeID != "" {
			ids = []string{account.EmployeeID}
		}
		return employeeScopeByIDs(ids), nil
	case ScopeDepartment, ScopeAssignedOrgUnits:
		return employeeScopeByOrgUnits(stringSliceFromAny(decision.Conditions["org_unit_ids"])), nil
	case ScopeDepartmentSubtree:
		orgIDs := stringSliceFromAny(decision.Conditions["org_unit_ids"])
		if len(orgIDs) == 0 && account.EmployeeID != "" {
			employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, account.EmployeeID)
			if err != nil {
				return domain.EmployeeScopeConstraint{}, err
			}
			if ok && employee.OrgUnitID != "" {
				units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
				if err != nil {
					return domain.EmployeeScopeConstraint{}, err
				}
				orgIDs = orgUnitIDsInSubtree(units, []string{employee.OrgUnitID})
			}
		}
		return employeeScopeByOrgUnits(orgIDs), nil
	case ScopeDirectReports:
		return employeeScopeByIDs(stringSliceFromAny(decision.Conditions["employee_ids"])), nil
	case ScopeCustomCondition:
		return employeeScopeFromConditions(decision.Conditions), nil
	default:
		return domain.EmployeeScopeConstraint{DenyAll: true}, nil
	}
}

// employeeScopeByIDs 處理員工範圍 by IDs。
func employeeScopeByIDs(ids []string) domain.EmployeeScopeConstraint {
	ids = uniqueStrings(ids)
	if len(ids) == 0 {
		return domain.EmployeeScopeConstraint{DenyAll: true}
	}
	return domain.EmployeeScopeConstraint{EmployeeIDs: ids}
}

// employeeScopeByOrgUnits 處理員工範圍 by 組織單位。
func employeeScopeByOrgUnits(ids []string) domain.EmployeeScopeConstraint {
	ids = uniqueStrings(ids)
	if len(ids) == 0 {
		return domain.EmployeeScopeConstraint{DenyAll: true}
	}
	return domain.EmployeeScopeConstraint{OrgUnitIDs: ids}
}

// employeeScopeFromConditions 處理員工範圍 來源 conditions。
func employeeScopeFromConditions(conditions map[string]any) domain.EmployeeScopeConstraint {
	scope := domain.EmployeeScopeConstraint{
		EmployeeIDs: uniqueStrings(stringSliceFromAny(conditions["employee_ids"])),
		OrgUnitIDs:  uniqueStrings(stringSliceFromAny(conditions["org_unit_ids"])),
		Statuses:    uniqueStrings(stringSliceFromAny(conditions["employee_statuses"])),
	}
	if len(scope.Statuses) == 0 {
		scope.Statuses = uniqueStrings(stringSliceFromAny(conditions["statuses"]))
	}
	if len(scope.EmployeeIDs) == 0 && len(scope.OrgUnitIDs) == 0 && len(scope.Statuses) == 0 {
		scope.DenyAll = true
	}
	return scope
}

// normalizeEmployeeQuery 正規化員工查詢。
func normalizeEmployeeQuery(query EmployeeQuery) EmployeeQuery {
	if query.Page <= 0 {
		query.Page = defaultEmployeePage
	}
	if query.PageSize <= 0 {
		query.PageSize = defaultEmployeePageSize
	}
	if query.PageSize > maxEmployeePageSize {
		query.PageSize = maxEmployeePageSize
	}
	if query.Sort == "" {
		query.Sort = "created_at_asc"
	}
	query.EmploymentStatus = normalizeEmployeeStatus(query.EmploymentStatus)
	query.Category = normalizeEmployeeCategory(query.Category)
	return query
}

// sortEmployees 排序員工。
func sortEmployees(items []Employee, sortKey string) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch sortKey {
		case "created_at_desc":
			if a.CreatedAt.Equal(b.CreatedAt) {
				return a.ID > b.ID
			}
			return a.CreatedAt.After(b.CreatedAt)
		case "hire_date_desc":
			return timeValue(a.HireDate).After(timeValue(b.HireDate))
		case "hire_date_asc":
			return timeValue(a.HireDate).Before(timeValue(b.HireDate))
		default:
			if a.CreatedAt.Equal(b.CreatedAt) {
				return a.ID < b.ID
			}
			return a.CreatedAt.Before(b.CreatedAt)
		}
	})
}

// employeeStatus 處理員工狀態。
func employeeStatus(item Employee) string {
	return utils.FirstNonEmpty(item.EmploymentStatus, item.Status)
}

// normalizeEmployeeStatus 正規化員工狀態。
func normalizeEmployeeStatus(value string) string {
	return NormalizeEmployeeStatus(value)
}

// normalizeEmployeeCategory 正規化員工分類。
func normalizeEmployeeCategory(value string) string {
	return NormalizeEmployeeCategory(value)
}

// normalizeEmployeeAccountPolicy 正規化員工帳號政策。
func normalizeEmployeeAccountPolicy(value string) string {
	return NormalizeEmployeeAccountPolicy(value)
}

// validEmployeeStatus 處理有效員工狀態。
func validEmployeeStatus(value string, includeDeleted bool) bool {
	status, ok := ParseEmployeeStatus(value)
	return ok && status.Valid(includeDeleted)
}

// validEmployeeCategory 處理有效員工分類。
func validEmployeeCategory(value string) bool {
	category, ok := ParseEmployeeCategory(value)
	return ok && category.Valid()
}

// validEmployeeAccountPolicy 處理有效員工帳號政策。
func validEmployeeAccountPolicy(value string) bool {
	_, ok := ParseEmployeeAccountPolicy(value)
	return ok
}

// employeeTerminalStatus 處理員工 terminal 狀態。
func employeeTerminalStatus(status string) bool {
	status = normalizeEmployeeStatus(status)
	return status == string(EmployeeStatusResigned) || status == string(EmployeeStatusDeleted)
}

// sameMonth 處理 same 月份。
func sameMonth(t time.Time, ref time.Time) bool {
	t = t.UTC()
	ref = ref.UTC()
	return t.Year() == ref.Year() && t.Month() == ref.Month()
}

// timeValue 處理時間 value。
func timeValue(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// formatDate 處理 format 日期。
func formatDate(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// uniqueSorted 處理 unique sorted。
func uniqueSorted(values []string) []string {
	return uniqueStrings(values)
}

// employeeStringValues 處理員工字串 values。
func employeeStringValues(items []Employee, fn func(Employee) string) []string {
	out := make([]string, 0)
	for _, item := range items {
		if value := strings.TrimSpace(fn(item)); value != "" {
			out = append(out, value)
		}
	}
	return out
}

const employeeNoPrefix = "IKL"

const (
	employeeValidationFullForm      = "full_form"
	employeeValidationImportMinimal = "import_minimal"
)

type employeeUniqueLookupStore interface {
	GetEmployeeByEmployeeNo(context.Context, string, string) (Employee, bool, error)
	GetEmployeeByCompanyEmail(context.Context, string, string) (Employee, bool, error)
	GetEmployeeByPersonalEmail(context.Context, string, string) (Employee, bool, error)
	GetEmployeeByAccountID(context.Context, string, string) (Employee, bool, error)
	GetEmployeeByBasicInfoField(context.Context, string, string, string) (Employee, bool, error)
}

// employeeFromCreateInput 處理員工 來源 create 輸入的服務流程。
func (c HRService) employeeFromCreateInput(ctx RequestContext, input CreateEmployeeInput, reservedEmployeeNos ...map[string]struct{}) (Employee, error) {
	return c.employeeFromCreateInputWithProfile(ctx, input, employeeValidationFullForm, reservedEmployeeNos...)
}

// employeeFromImportInput 處理員工 來源 import 輸入的服務流程。
func (c HRService) employeeFromImportInput(ctx RequestContext, input CreateEmployeeInput, reservedEmployeeNos ...map[string]struct{}) (Employee, error) {
	return c.employeeFromCreateInputWithProfile(ctx, input, employeeValidationImportMinimal, reservedEmployeeNos...)
}

// employeeFromCreateInputWithProfile 處理員工 來源 create 輸入 with 資料檔的服務流程。
func (c HRService) employeeFromCreateInputWithProfile(ctx RequestContext, input CreateEmployeeInput, profile string, reservedEmployeeNos ...map[string]struct{}) (Employee, error) {
	employee, err := c.employeeCreateCandidate(ctx, input)
	if err != nil {
		return Employee{}, err
	}
	if employee.EmployeeNo == "" {
		employeeNo, err := c.generateEmployeeNo(ctx, reservedEmployeeNos...)
		if err != nil {
			return Employee{}, err
		}
		employee.EmployeeNo = employeeNo
	}
	if err := c.ensureEmployeePosition(ctx, &employee, true); err != nil {
		return Employee{}, err
	}
	if err := c.validateEmployee(ctx, employee, "create", profile); err != nil {
		return Employee{}, err
	}
	if err := reserveEmployeeNo(employee.EmployeeNo, reservedEmployeeNos...); err != nil {
		return Employee{}, err
	}
	if len(employee.InternalExperiences) == 0 {
		employee.InternalExperiences = append(employee.InternalExperiences, c.newEmployeeExperience(employee, "新進"))
	}
	return employee, nil
}

// employeeCreateCandidate 處理員工 create 候選的服務流程。
func (c HRService) employeeCreateCandidate(ctx RequestContext, input CreateEmployeeInput) (Employee, error) {
	now := c.Now()
	hireDate, err := optionalDateTime(input.HireDate)
	if err != nil {
		return Employee{}, BadRequest("hire_date must be RFC3339 or YYYY-MM-DD")
	}
	resignDate, err := optionalDateTime(input.ResignDate)
	if err != nil {
		return Employee{}, BadRequest("resign_date must be RFC3339 or YYYY-MM-DD")
	}
	status := normalizeEmployeeStatus(utils.FirstNonEmpty(input.EmploymentStatus, input.Status, string(EmployeeStatusActive)))
	employee := Employee{
		ID:                    utils.NewID("emp"),
		TenantID:              ctx.TenantID,
		EmployeeNo:            strings.TrimSpace(input.EmployeeNo),
		Name:                  strings.TrimSpace(input.Name),
		CompanyEmail:          strings.TrimSpace(input.CompanyEmail),
		PersonalEmail:         strings.TrimSpace(input.PersonalEmail),
		Phone:                 strings.TrimSpace(input.Phone),
		OrgUnitID:             strings.TrimSpace(input.OrgUnitID),
		AccountID:             strings.TrimSpace(input.AccountID),
		ManagerEmployeeID:     strings.TrimSpace(input.ManagerEmployeeID),
		PositionID:            strings.TrimSpace(input.PositionID),
		Position:              strings.TrimSpace(input.Position),
		Category:              normalizeEmployeeCategory(input.Category),
		Status:                status,
		EmploymentStatus:      status,
		HireDate:              hireDate,
		ResignDate:            resignDate,
		BasicInfo:             utils.CopyStringMap(input.BasicInfo),
		EmploymentInfo:        utils.CopyStringMap(input.EmploymentInfo),
		EducationMilitaryInfo: utils.CopyStringMap(input.EducationMilitaryInfo),
		ContactInfo:           utils.CopyStringMap(input.ContactInfo),
		InsuranceInfo:         utils.CopyStringMap(input.InsuranceInfo),
		InternalExperiences:   utils.CopyEmployeeExperiences(input.InternalExperiences),
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	return c.deriveEmployeeHotFields(employee), nil
}

// applyEmployeePatch 處理 apply 員工 patch 的服務流程。
func (c HRService) applyEmployeePatch(ctx RequestContext, employee *Employee, input UpdateEmployeeInput) error {
	return c.applyEmployeePatchWithPositionCreation(ctx, employee, input, true)
}

// applyEmployeePatchWithPositionCreation 處理 apply 員工 patch 的服務流程。
func (c HRService) applyEmployeePatchWithPositionCreation(ctx RequestContext, employee *Employee, input UpdateEmployeeInput, createMissingPosition bool) error {
	if input.Status != nil || input.EmploymentStatus != nil {
		return domainValidation("employee status must be changed through status-transition", FieldError{Tab: employeeTabEmploymentInfo, Field: "status", Code: "transition_required", Message: "status must be changed through status-transition"})
	}
	if input.EmployeeNo != nil {
		employee.EmployeeNo = strings.TrimSpace(*input.EmployeeNo)
	}
	if input.Name != nil {
		employee.Name = strings.TrimSpace(*input.Name)
	}
	if input.CompanyEmail != nil {
		employee.CompanyEmail = strings.TrimSpace(*input.CompanyEmail)
	}
	if input.PersonalEmail != nil {
		employee.PersonalEmail = strings.TrimSpace(*input.PersonalEmail)
	}
	if input.Phone != nil {
		employee.Phone = strings.TrimSpace(*input.Phone)
	}
	if input.OrgUnitID != nil {
		employee.OrgUnitID = strings.TrimSpace(*input.OrgUnitID)
	}
	if input.AccountID != nil {
		employee.AccountID = strings.TrimSpace(*input.AccountID)
	}
	if input.ManagerEmployeeID != nil {
		employee.ManagerEmployeeID = strings.TrimSpace(*input.ManagerEmployeeID)
	}
	if input.PositionID != nil {
		employee.PositionID = strings.TrimSpace(*input.PositionID)
	}
	if input.Position != nil {
		employee.Position = strings.TrimSpace(*input.Position)
	}
	if input.Category != nil {
		employee.Category = normalizeEmployeeCategory(*input.Category)
	}
	if input.HireDate != nil {
		t, err := optionalDateTime(*input.HireDate)
		if err != nil {
			return BadRequest("hire_date must be RFC3339 or YYYY-MM-DD")
		}
		employee.HireDate = t
	}
	if input.ResignDate != nil {
		t, err := optionalDateTime(*input.ResignDate)
		if err != nil {
			return BadRequest("resign_date must be RFC3339 or YYYY-MM-DD")
		}
		employee.ResignDate = t
	}
	employee.BasicInfo = mergeMap(employee.BasicInfo, input.BasicInfo)
	employee.EmploymentInfo = mergeMap(employee.EmploymentInfo, input.EmploymentInfo)
	if input.Position != nil && input.PositionID == nil {
		employee.PositionID = ""
		if employee.EmploymentInfo == nil {
			employee.EmploymentInfo = map[string]any{}
		}
		delete(employee.EmploymentInfo, "position_id")
		employee.EmploymentInfo["position"] = employee.Position
	}
	employee.EducationMilitaryInfo = mergeMap(employee.EducationMilitaryInfo, input.EducationMilitaryInfo)
	employee.ContactInfo = mergeMap(employee.ContactInfo, input.ContactInfo)
	employee.InsuranceInfo = mergeMap(employee.InsuranceInfo, input.InsuranceInfo)
	if input.InternalExperiences != nil {
		employee.InternalExperiences = utils.CopyEmployeeExperiences(input.InternalExperiences)
	}
	*employee = c.deriveEmployeeHotFields(*employee)
	if err := c.ensureEmployeePosition(ctx, employee, createMissingPosition); err != nil {
		return err
	}
	employee.UpdatedAt = c.Now()
	return c.validateEmployee(ctx, *employee, "update", employeeValidationFullForm)
}

// forbiddenEmployeePatchFields 處理禁止員工 patch 欄位。
func forbiddenEmployeePatchFields(input UpdateEmployeeInput, policies map[string]string) []FieldError {
	if len(policies) == 0 {
		return nil
	}
	fields := make([]FieldError, 0)
	add := func(tab, field string) {
		effect := policies[field]
		if effect != "deny" && effect != "hide" && effect != "readonly" {
			return
		}
		fields = append(fields, FieldError{Tab: tab, Field: field, Code: "field_denied", Message: field + " cannot be updated by current permission policy"})
	}
	for _, field := range employeeScalarPatchPolicyFields {
		if field.present(input) {
			add(field.tab, field.field)
		}
	}
	for _, field := range employeeMapPatchPolicyFields {
		addPatchMapFields(&fields, policies, field.tab, field.values(input))
	}
	return fields
}

// addPatchMapFields 處理 add patch map 欄位。
func addPatchMapFields(fields *[]FieldError, policies map[string]string, tab string, values map[string]any) {
	if len(values) == 0 {
		return
	}
	if effect := policies[tab]; effect == "deny" || effect == "hide" || effect == "readonly" {
		*fields = append(*fields, FieldError{Tab: tab, Field: tab, Code: "field_denied", Message: tab + " cannot be updated by current permission policy"})
	}
	for field := range values {
		effect := policies[field]
		if effect != "deny" && effect != "hide" && effect != "readonly" {
			continue
		}
		*fields = append(*fields, FieldError{Tab: tab, Field: field, Code: "field_denied", Message: field + " cannot be updated by current permission policy"})
	}
}

// deriveEmployeeHotFields 推導員工 hot 欄位的服務流程。
func (c HRService) deriveEmployeeHotFields(employee Employee) Employee {
	employee.CompanyEmail = utils.FirstNonEmpty(employee.CompanyEmail, employeeHotValue(employee, "company_email"))
	employee.PersonalEmail = utils.FirstNonEmpty(employee.PersonalEmail, employeeHotValue(employee, "personal_email"))
	employee.Phone = utils.FirstNonEmpty(employee.Phone, employeeHotValue(employee, "phone"))
	employee.OrgUnitID = utils.FirstNonEmpty(employee.OrgUnitID, employeeHotValue(employee, "org_unit_id"))
	employee.ManagerEmployeeID = utils.FirstNonEmpty(employee.ManagerEmployeeID, employeeHotValue(employee, "manager_employee_id"))
	employee.PositionID = utils.FirstNonEmpty(employee.PositionID, employeeHotValue(employee, "position_id"))
	employee.Position = utils.FirstNonEmpty(employee.Position, employeeHotValue(employee, "position"))
	employee.Category = normalizeEmployeeCategory(utils.FirstNonEmpty(employee.Category, employeeHotValue(employee, "category")))
	employee.Name = utils.FirstNonEmpty(employee.Name, employeeHotValue(employee, "name"), strings.TrimSpace(stringFromMap(employee.BasicInfo, "first_name")+" "+stringFromMap(employee.BasicInfo, "last_name")))
	if employee.EmploymentStatus == "" {
		employee.EmploymentStatus = employee.Status
	}
	if employee.Status == "" {
		employee.Status = employee.EmploymentStatus
	}
	employee.EmploymentStatus = normalizeEmployeeStatus(employee.EmploymentStatus)
	employee.Status = normalizeEmployeeStatus(employee.Status)
	return employee
}

// validateEmployee 驗證員工的服務流程。
func (c HRService) validateEmployee(ctx RequestContext, employee Employee, mode string, profile ...string) error {
	validationProfile := employeeValidationFullForm
	if len(profile) > 0 && strings.TrimSpace(profile[0]) != "" {
		validationProfile = strings.TrimSpace(profile[0])
	}
	fields := make([]FieldError, 0)
	if strings.TrimSpace(employee.Name) == "" {
		fields = append(fields, FieldError{Tab: "basic_info", Field: "name", Code: "required", Message: "name is required"})
	}
	if validationProfile != employeeValidationImportMinimal && strings.TrimSpace(employee.CompanyEmail) == "" {
		fields = append(fields, FieldError{Tab: "basic_info", Field: "company_email", Code: "required", Message: "company_email is required"})
	}
	if employee.Category != "" && !validEmployeeCategory(employee.Category) {
		fields = append(fields, FieldError{Tab: "employment_info", Field: "category", Code: "invalid", Message: "category must be one of full_time, part_time, intern, contractor, other"})
	}
	if !validEmployeeStatus(employeeStatus(employee), true) {
		fields = append(fields, FieldError{Tab: "employment_info", Field: "employment_status", Code: "invalid", Message: "employment_status must be one of active, probation, leave_suspended, onboarding, resigned, deleted"})
	}
	if strings.TrimSpace(employee.OrgUnitID) != "" {
		if _, ok, err := c.store.GetOrgUnit(goContext(ctx), ctx.TenantID, employee.OrgUnitID); err != nil {
			return err
		} else if !ok {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "org_unit_id", Code: "not_found", Message: "org unit not found"})
		}
	}
	if strings.TrimSpace(employee.AccountID) != "" {
		account, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, employee.AccountID)
		if err != nil {
			return err
		}
		if !ok {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "account_id", Code: "not_found", Message: "account not found"})
		} else {
			if account.EmployeeID != "" && account.EmployeeID != employee.ID {
				fields = append(fields, FieldError{Tab: "employment_info", Field: "account_id", Code: "unique", Message: "account_id already linked"})
			}
			if account.Status == string(AccountStatusDisabled) && !employeeTerminalStatus(employeeStatus(employee)) {
				fields = append(fields, FieldError{Tab: "employment_info", Field: "account_id", Code: "invalid", Message: "disabled account cannot be linked to a non-terminal employee"})
			}
		}
	}
	if strings.TrimSpace(employee.ManagerEmployeeID) != "" {
		if employee.ManagerEmployeeID == employee.ID {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "manager_employee_id", Code: "invalid", Message: "manager cannot be the employee itself"})
		} else if _, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, employee.ManagerEmployeeID); err != nil {
			return err
		} else if !ok {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "manager_employee_id", Code: "not_found", Message: "manager employee not found"})
		}
	}
	if identityType := stringFromMap(employee.BasicInfo, "nationality_type"); identityType == "foreign" {
		for _, field := range []string{"passport_no", "passport_name", "entry_date", "arc_no", "arc_expiry_date", "tax_id", "work_permit_no", "work_permit_expiry_date", "contract_expiry_date", "broker"} {
			if stringFromMap(employee.BasicInfo, field) == "" {
				fields = append(fields, FieldError{Tab: "basic_info", Field: field, Code: "required", Message: field + " is required for foreign employees"})
			}
		}
	} else if idNo := stringFromMap(employee.BasicInfo, "national_id"); idNo == "" && stringFromMap(employee.BasicInfo, "nationality_type") != "" {
		fields = append(fields, FieldError{Tab: "basic_info", Field: "national_id", Code: "required", Message: "national_id is required for local employees"})
	}
	status := employeeStatus(employee)
	if status == string(EmployeeStatusResigned) && validationProfile != employeeValidationImportMinimal {
		if employee.ResignDate == nil && stringFromMap(employee.EmploymentInfo, "resign_date") == "" {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "resign_date", Code: "required", Message: "resign_date is required"})
		}
		if stringFromMap(employee.EmploymentInfo, "resign_reason") == "" && mode == "transition" {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "resign_reason", Code: "required", Message: "resign_reason is required"})
		}
	}
	if validationProfile == employeeValidationFullForm {
		fields = append(fields, fullFormEmployeeFieldErrors(employee)...)
	}
	if len(fields) == 0 {
		if uniqueLookup, ok := c.store.(employeeUniqueLookupStore); ok {
			uniqueFields, err := c.employeeUniqueFieldErrors(ctx, uniqueLookup, employee)
			if err != nil {
				return err
			}
			fields = append(fields, uniqueFields...)
		} else if uniqueFields, err := c.employeeUniqueFieldErrorsFromList(ctx, employee); err != nil {
			return err
		} else {
			fields = append(fields, uniqueFields...)
		}
	}
	if len(fields) > 0 {
		return domainValidation("employee validation failed", fields...)
	}
	return nil
}

// fullFormEmployeeFieldErrors 處理 full 表單員工欄位錯誤。
func fullFormEmployeeFieldErrors(employee Employee) []FieldError {
	fields := make([]FieldError, 0)
	addRequired := func(tab, field, message string) {
		fields = append(fields, FieldError{Tab: tab, Field: field, Code: "required", Message: message})
	}
	if strings.TrimSpace(employee.OrgUnitID) == "" {
		addRequired(employeeTabEmploymentInfo, "org_unit_id", "org_unit_id is required")
	}
	if strings.TrimSpace(employee.Position) == "" {
		addRequired(employeeTabEmploymentInfo, "position", "position is required")
	}
	if strings.TrimSpace(employee.Category) == "" {
		addRequired(employeeTabEmploymentInfo, "category", "category is required")
	}
	if strings.TrimSpace(employeeStatus(employee)) == "" {
		addRequired(employeeTabEmploymentInfo, "employment_status", "employment_status is required")
	}
	if employee.HireDate == nil && stringFromMap(employee.EmploymentInfo, "hire_date") == "" {
		addRequired(employeeTabEmploymentInfo, "hire_date", "hire_date is required")
	}
	if identityType := stringFromMap(employee.BasicInfo, "nationality_type"); identityType == "" {
		addRequired(employeeTabBasicInfo, "nationality_type", "nationality_type is required")
	}
	requireAny(&fields, employeeTabEducationMilitaryInfo, employee.EducationMilitaryInfo, "highest_education", "highest_education is required", "highest_education", "education_level", "degree")
	requireAny(&fields, employeeTabEducationMilitaryInfo, employee.EducationMilitaryInfo, "school", "school is required", "school", "school_name")
	requireAny(&fields, employeeTabContactInfo, employee.ContactInfo, "mobile_phone", "mobile_phone is required", "mobile_phone", "phone")
	requireAny(&fields, employeeTabContactInfo, employee.ContactInfo, "address", "address is required", "address", "communication_address")
	requireAny(&fields, employeeTabContactInfo, employee.ContactInfo, "emergency_contact_relation", "emergency_contact_relation is required", "emergency_contact_relation", "emergency_relation")
	requireAny(&fields, employeeTabContactInfo, employee.ContactInfo, "emergency_contact_name", "emergency_contact_name is required", "emergency_contact_name", "emergency_name")
	requireAny(&fields, employeeTabContactInfo, employee.ContactInfo, "emergency_contact_phone", "emergency_contact_phone is required", "emergency_contact_phone", "emergency_phone")
	requireAny(&fields, employeeTabInsuranceInfo, employee.InsuranceInfo, "labor_insurance_date", "labor_insurance_date is required", "labor_insurance_date")
	requireAny(&fields, employeeTabInsuranceInfo, employee.InsuranceInfo, "labor_insurance_level", "labor_insurance_level is required", "labor_insurance_level")
	requirePositiveNumber(&fields, employee.InsuranceInfo, "labor_insurance_salary", "labor_insurance_salary must be positive")
	requireAny(&fields, employeeTabInsuranceInfo, employee.InsuranceInfo, "health_insurance_date", "health_insurance_date is required", "health_insurance_date")
	requireAny(&fields, employeeTabInsuranceInfo, employee.InsuranceInfo, "health_insurance_level", "health_insurance_level is required", "health_insurance_level")
	requirePositiveNumber(&fields, employee.InsuranceInfo, "health_insurance_amount", "health_insurance_amount must be positive")
	return fields
}

// requireAny 處理 require any。
func requireAny(fields *[]FieldError, tab string, values map[string]any, field, message string, keys ...string) {
	for _, key := range keys {
		if mapAnyString(values, key) != "" {
			return
		}
	}
	*fields = append(*fields, FieldError{Tab: tab, Field: field, Code: "required", Message: message})
}

// requirePositiveNumber 處理 require 正數數字。
func requirePositiveNumber(fields *[]FieldError, values map[string]any, field, message string) {
	raw := mapAnyString(values, field)
	if raw == "" {
		*fields = append(*fields, FieldError{Tab: employeeTabInsuranceInfo, Field: field, Code: "required", Message: message})
		return
	}
	number, err := strconv.ParseFloat(raw, 64)
	if err != nil || number <= 0 {
		*fields = append(*fields, FieldError{Tab: employeeTabInsuranceInfo, Field: field, Code: "invalid", Message: message})
	}
}

// employeeUniqueFieldErrors 處理員工 unique 欄位錯誤的服務流程。
func (c HRService) employeeUniqueFieldErrors(ctx RequestContext, store employeeUniqueLookupStore, employee Employee) ([]FieldError, error) {
	fields := make([]FieldError, 0, 8)
	goCtx := goContext(ctx)
	if employee.EmployeeNo != "" {
		existing, ok, err := store.GetEmployeeByEmployeeNo(goCtx, ctx.TenantID, employee.EmployeeNo)
		if err != nil {
			return nil, err
		}
		if ok && existing.ID != employee.ID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "employee_no", Code: "unique", Message: "employee_no already exists"})
		}
	}
	if employee.CompanyEmail != "" {
		existing, ok, err := store.GetEmployeeByCompanyEmail(goCtx, ctx.TenantID, employee.CompanyEmail)
		if err != nil {
			return nil, err
		}
		if ok && existing.ID != employee.ID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "company_email", Code: "unique", Message: "company_email already exists"})
		}
	}
	if employee.PersonalEmail != "" {
		existing, ok, err := store.GetEmployeeByPersonalEmail(goCtx, ctx.TenantID, employee.PersonalEmail)
		if err != nil {
			return nil, err
		}
		if ok && existing.ID != employee.ID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "personal_email", Code: "unique", Message: "personal_email already exists"})
		}
	}
	if employee.AccountID != "" {
		existing, ok, err := store.GetEmployeeByAccountID(goCtx, ctx.TenantID, employee.AccountID)
		if err != nil {
			return nil, err
		}
		if ok && existing.ID != employee.ID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "account_id", Code: "unique", Message: "account_id already linked"})
		}
	}
	for _, field := range employeeUniqueBasicInfoFields {
		value := stringFromMap(employee.BasicInfo, field)
		if value == "" {
			continue
		}
		existing, ok, err := store.GetEmployeeByBasicInfoField(goCtx, ctx.TenantID, field, value)
		if err != nil {
			return nil, err
		}
		if ok && existing.ID != employee.ID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: field, Code: "unique", Message: field + " already exists"})
		}
	}
	return fields, nil
}

// employeeUniqueFieldErrorsFromList 處理員工 unique 欄位錯誤 來源 列表的服務流程。
func (c HRService) employeeUniqueFieldErrorsFromList(ctx RequestContext, employee Employee) ([]FieldError, error) {
	existingEmployees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	fields := make([]FieldError, 0, 8)
	for _, existing := range existingEmployees {
		if existing.ID == employee.ID {
			continue
		}
		if employee.EmployeeNo != "" && existing.EmployeeNo == employee.EmployeeNo {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "employee_no", Code: "unique", Message: "employee_no already exists"})
		}
		if employee.CompanyEmail != "" && strings.EqualFold(existing.CompanyEmail, employee.CompanyEmail) {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "company_email", Code: "unique", Message: "company_email already exists"})
		}
		if employee.PersonalEmail != "" && strings.EqualFold(existing.PersonalEmail, employee.PersonalEmail) {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "personal_email", Code: "unique", Message: "personal_email already exists"})
		}
		if employee.AccountID != "" && existing.AccountID == employee.AccountID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "account_id", Code: "unique", Message: "account_id already linked"})
		}
		for _, field := range employeeUniqueBasicInfoFields {
			value := stringFromMap(employee.BasicInfo, field)
			if value == "" {
				continue
			}
			if strings.EqualFold(stringFromMap(existing.BasicInfo, field), value) {
				fields = append(fields, FieldError{Tab: "basic_info", Field: field, Code: "unique", Message: field + " already exists"})
			}
		}
	}
	return fields, nil
}

var employeeUniqueBasicInfoFields = []string{
	"national_id",
	"passport_no",
	"arc_no",
	"tax_id",
	"work_permit_no",
}

// generateEmployeeNo 處理 generate 員工 no 的服務流程。
func (c HRService) generateEmployeeNo(ctx RequestContext, reservedEmployeeNos ...map[string]struct{}) (string, error) {
	for {
		employeeNo, err := c.store.NextEmployeeNo(goContext(ctx), ctx.TenantID, employeeNoPrefix)
		if err != nil {
			return "", err
		}
		if !employeeNoReserved(employeeNo, reservedEmployeeNos...) {
			return employeeNo, nil
		}
	}
}

// employeeNoReserved 處理員工 no reserved。
func employeeNoReserved(employeeNo string, reservedEmployeeNos ...map[string]struct{}) bool {
	employeeNo = strings.TrimSpace(employeeNo)
	if employeeNo == "" {
		return false
	}
	for _, reserved := range reservedEmployeeNos {
		if reserved == nil {
			continue
		}
		if _, ok := reserved[employeeNo]; ok {
			return true
		}
	}
	return false
}

// reserveEmployeeNo 保留員工 no。
func reserveEmployeeNo(employeeNo string, reservedEmployeeNos ...map[string]struct{}) error {
	employeeNo = strings.TrimSpace(employeeNo)
	if employeeNo == "" {
		return nil
	}
	for _, reserved := range reservedEmployeeNos {
		if reserved == nil {
			continue
		}
		if _, ok := reserved[employeeNo]; ok {
			return domainValidation("employee validation failed", FieldError{Tab: "basic_info", Field: "employee_no", Code: "duplicate_in_import", Message: "employee_no is duplicated in import batch"})
		}
		reserved[employeeNo] = struct{}{}
	}
	return nil
}

// stringFromMap 處理字串 來源 map。
func stringFromMap(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	if v, ok := values[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// mapAnyString 映射any 字串。
func mapAnyString(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	switch v := values[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	default:
		return ""
	}
}

// mergeMap 合併 map。
func mergeMap(base map[string]any, patch map[string]any) map[string]any {
	if len(base) == 0 && len(patch) == 0 {
		return nil
	}
	out := utils.CopyStringMap(base)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range patch {
		out[key] = value
	}
	return out
}

// newEmployeeExperience 建立員工經歷的服務流程。
func (c HRService) newEmployeeExperience(employee Employee, reason string) EmployeeExperience {
	return EmployeeExperience{
		ID:                utils.NewID("ehist"),
		StartDate:         employee.HireDate,
		Reason:            utils.FirstNonEmpty(reason, "資料更新"),
		OrgUnitID:         employee.OrgUnitID,
		ManagerEmployeeID: employee.ManagerEmployeeID,
		Position:          employee.Position,
		Category:          employee.Category,
		Status:            employeeStatus(employee),
		Current:           true,
		CreatedAt:         c.Now(),
	}
}

// appendHistoryForChangedEmployment 附加歷史 for changed 任職的服務流程。
func (c HRService) appendHistoryForChangedEmployment(before, after Employee, reason string) Employee {
	if before.OrgUnitID == after.OrgUnitID && before.ManagerEmployeeID == after.ManagerEmployeeID && before.Position == after.Position && before.Category == after.Category && before.Status == after.Status && before.EmploymentStatus == after.EmploymentStatus {
		return after
	}
	for i := range after.InternalExperiences {
		after.InternalExperiences[i].Current = false
		if after.InternalExperiences[i].EndDate == nil {
			t := c.Now()
			after.InternalExperiences[i].EndDate = &t
		}
	}
	after.InternalExperiences = append(after.InternalExperiences, c.newEmployeeExperience(after, reason))
	return after
}

// touchEmployeeAuthzIfNeeded 處理 touch 員工授權 if needed 的服務流程。
func (c HRService) touchEmployeeAuthzIfNeeded(ctx RequestContext, before, after Employee, eventType string) error {
	if before.OrgUnitID == after.OrgUnitID && before.AccountID == after.AccountID && before.ManagerEmployeeID == after.ManagerEmployeeID {
		return nil
	}
	if err := c.syncEmployeeRelationshipTuples(ctx, before, after); err != nil {
		return err
	}
	return c.touchAuthzConfig(ctx, eventType, map[string]any{
		"employee_id":                after.ID,
		"before_org_unit_id":         before.OrgUnitID,
		"after_org_unit_id":          after.OrgUnitID,
		"before_account_id":          before.AccountID,
		"after_account_id":           after.AccountID,
		"before_manager_employee_id": before.ManagerEmployeeID,
		"after_manager_employee_id":  after.ManagerEmployeeID,
	})
}

// linkEmployeeAccount 處理 link 員工帳號的服務流程。
func (c HRService) linkEmployeeAccount(ctx RequestContext, employee Employee) error {
	if employee.AccountID == "" {
		return nil
	}
	account, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, employee.AccountID)
	if err != nil {
		return err
	}
	if ok {
		before := account
		account.EmployeeID = employee.ID
		account.DisplayName = utils.FirstNonEmpty(account.DisplayName, employee.Name)
		account.Email = utils.FirstNonEmpty(account.Email, employee.CompanyEmail)
		if err := c.store.UpsertAccount(goContext(ctx), account); err != nil {
			return err
		}
		return c.Service.syncAccountTenantMembershipTuple(ctx, before, account)
	}
	return nil
}

// applyEmployeeCreateAccountPolicy 處理 apply 員工 create 帳號政策的服務流程。
func (c HRService) applyEmployeeCreateAccountPolicy(ctx RequestContext, employee *Employee, input CreateEmployeeInput) (string, bool, error) {
	policy, err := employeeCreateAccountPolicy(input)
	if err != nil {
		return "", false, err
	}
	switch policy {
	case string(EmployeeAccountPolicyNone):
		return policy, false, nil
	case string(EmployeeAccountPolicyLinkExisting):
		if strings.TrimSpace(employee.AccountID) == "" {
			return "", false, domainValidation("employee account policy validation failed", FieldError{Tab: employeeTabEmploymentInfo, Field: "account_id", Code: "required", Message: "account_id is required when account_policy is link_existing"})
		}
		return policy, false, c.ensureEmployeeLinkedAccount(ctx, *employee)
	case string(EmployeeAccountPolicyCreatePendingInvite), string(EmployeeAccountPolicyCreateActive):
		if strings.TrimSpace(input.AccountID) != "" {
			return "", false, domainValidation("employee account policy validation failed", FieldError{Tab: employeeTabEmploymentInfo, Field: "account_id", Code: "invalid", Message: "account_id is only allowed when account_policy is link_existing"})
		}
		if employeeTerminalStatus(employeeStatus(*employee)) {
			return "", false, domainValidation("employee account policy validation failed", FieldError{Tab: employeeTabEmploymentInfo, Field: "account_policy", Code: "invalid", Message: "terminal employee cannot create a login account"})
		}
		email := strings.TrimSpace(employee.CompanyEmail)
		if email == "" {
			return "", false, domainValidation("employee account policy validation failed", FieldError{Tab: employeeTabBasicInfo, Field: "company_email", Code: "required", Message: "company_email is required to create an account"})
		}
		if err := c.ensureAccountEmailAvailable(ctx, email); err != nil {
			return "", false, err
		}
		accountStatus := string(AccountStatusPendingInvite)
		if policy == string(EmployeeAccountPolicyCreateActive) {
			accountStatus = string(AccountStatusActive)
		}
		account := Account{
			ID:          utils.NewID("acct"),
			TenantID:    ctx.TenantID,
			DisplayName: employee.Name,
			Email:       email,
			EmployeeID:  employee.ID,
			Status:      accountStatus,
			CreatedAt:   c.Now(),
		}
		if err := c.store.UpsertAccount(goContext(ctx), account); err != nil {
			return "", false, err
		}
		if err := c.Service.syncAccountTenantMembershipTuple(ctx, Account{}, account); err != nil {
			return "", false, err
		}
		employee.AccountID = account.ID
		return policy, true, nil
	default:
		return "", false, BadRequest("invalid account_policy")
	}
}

// employeeCreateAccountPolicy 處理員工 create 帳號政策。
func employeeCreateAccountPolicy(input CreateEmployeeInput) (string, error) {
	rawPolicy := strings.TrimSpace(input.AccountPolicy)
	if rawPolicy == "" {
		if strings.TrimSpace(input.AccountID) != "" {
			return string(EmployeeAccountPolicyLinkExisting), nil
		}
		return string(EmployeeAccountPolicyNone), nil
	}
	policy := normalizeEmployeeAccountPolicy(rawPolicy)
	if !validEmployeeAccountPolicy(policy) {
		return "", BadRequest("invalid account_policy")
	}
	return policy, nil
}

// ensureEmployeeLinkedAccount 確保員工 linked 帳號的服務流程。
func (c HRService) ensureEmployeeLinkedAccount(ctx RequestContext, employee Employee) error {
	account, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, employee.AccountID)
	if err != nil {
		return err
	}
	if !ok {
		return domainValidation("employee account validation failed", FieldError{Tab: employeeTabEmploymentInfo, Field: "account_id", Code: "not_found", Message: "account not found"})
	}
	if account.EmployeeID != "" && account.EmployeeID != employee.ID {
		return domainValidation("employee account validation failed", FieldError{Tab: employeeTabEmploymentInfo, Field: "account_id", Code: "unique", Message: "account_id already linked"})
	}
	if account.Status == string(AccountStatusDisabled) && !employeeTerminalStatus(employeeStatus(employee)) {
		return domainValidation("employee account validation failed", FieldError{Tab: employeeTabEmploymentInfo, Field: "account_id", Code: "invalid", Message: "disabled account cannot be linked to a non-terminal employee"})
	}
	return nil
}

// ensureAccountEmailAvailable 確保帳號 email available 的服務流程。
func (c HRService) ensureAccountEmailAvailable(ctx RequestContext, email string) error {
	return c.ensureAccountEmailAvailableForAccount(ctx, email, "")
}

// ensureAccountEmailAvailableForAccount 確保帳號 email available for 帳號的服務流程。
func (c HRService) ensureAccountEmailAvailableForAccount(ctx RequestContext, email, allowedAccountID string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	accounts, err := c.store.ListAccounts(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if strings.EqualFold(strings.TrimSpace(account.Email), email) && account.ID != strings.TrimSpace(allowedAccountID) {
			return domainValidation("employee account policy validation failed", FieldError{Tab: employeeTabBasicInfo, Field: "company_email", Code: "unique", Message: "account email already exists"})
		}
	}
	return nil
}

// appendEmployeeEvent 附加員工事件的服務流程。
func (c HRService) appendEmployeeEvent(ctx RequestContext, eventType, target string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["target"] = target
	aggregateType := string(ResourceEmployee)
	if eventType == string(EventEmployeeImported) {
		aggregateType = string(ResourceEmployeeImport)
	}
	return c.store.AppendOutboxEvent(goContext(ctx), OutboxEvent{
		ID:            utils.NewID("outbox"),
		TenantID:      ctx.TenantID,
		EventType:     eventType,
		AggregateType: aggregateType,
		AggregateID:   target,
		Payload:       payload,
		Status:        "pending",
		RetryCount:    0,
		CreatedAt:     c.Now(),
	})
}

// domainValidation 處理 domain 驗證。
func domainValidation(message string, fields ...FieldError) error {
	return ValidationFailed(message, fields)
}

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
	"position_id": {
		{tab: employeeTabEmploymentInfo, key: "position_id"},
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
	{tab: employeeTabEmploymentInfo, field: "position_id", present: func(input UpdateEmployeeInput) bool { return input.PositionID != nil }},
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
	field  string
	header string
	value  func(Employee) string
}

var employeeExportColumns = []employeeExportColumn{
	{field: "employee_no", header: "員工編號", value: func(employee Employee) string { return employee.EmployeeNo }},
	{field: "name", header: "姓名", value: func(employee Employee) string { return employee.Name }},
	{field: "company_email", header: "Email", value: func(employee Employee) string { return employee.CompanyEmail }},
	{field: "org_unit_id", header: "部門", value: func(employee Employee) string { return employee.OrgUnitID }},
	{field: "position", header: "職位", value: func(employee Employee) string { return employee.Position }},
	{field: "category", header: "類別", value: func(employee Employee) string { return employee.Category }},
	{field: "phone", header: "電話", value: func(employee Employee) string { return employee.Phone }},
	{field: "employment_status", header: "狀態", value: func(employee Employee) string { return employeeStatus(employee) }},
	{field: "hire_date", header: "到職日期", value: func(employee Employee) string { return formatDate(employee.HireDate) }},
	{field: "manager_employee_id", header: "主管員工ID", value: func(employee Employee) string { return employee.ManagerEmployeeID }},
}

// employeeExportColumnsForPolicy 處理員工 export columns for 政策。
func employeeExportColumnsForPolicy(policies map[string]string) []employeeExportColumn {
	out := make([]employeeExportColumn, 0, len(employeeExportColumns))
	for _, column := range employeeExportColumns {
		if employeeFieldPolicyHidden(policies[column.field]) {
			continue
		}
		out = append(out, column)
	}
	return out
}

// employeeFieldPolicyHidden 處理員工欄位政策 hidden。
func employeeFieldPolicyHidden(effect string) bool {
	return effect == "hide" || effect == "deny"
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
	employeeImportColumnAccountPolicy
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
	{header: "帳號策略"},
}

// employeeImportColumnCount 處理員工 import column count。
func employeeImportColumnCount() int {
	return len(employeeImportColumns)
}

// restrictedEmployeeFieldPolicies 處理 restricted 員工欄位政策。
func restrictedEmployeeFieldPolicies(policies map[string]string) map[string][]string {
	out := map[string][]string{}
	for field, effect := range policies {
		switch effect {
		case "mask", "hide", "deny":
			out[effect] = append(out[effect], field)
		}
	}
	for effect, fields := range out {
		out[effect] = uniqueSorted(fields)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// auditSensitiveEmployeeRead 寫入員工敏感欄位明文讀取稽核。
func (c HRService) auditSensitiveEmployeeRead(ctx RequestContext, decision CheckResult, items []Employee, target string) error {
	fields := visibleSensitiveEmployeeFields(decision, items)
	if len(fields) == 0 {
		return nil
	}
	details := auditDecisionDetails(ctx, decision, map[string]any{
		"fields": fields,
		"count":  len(items),
	})
	resource := string(ResourceEmployeeCollection)
	if strings.TrimSpace(target) != "" {
		resource = string(ResourceEmployee)
		details["resource_id"] = target
	} else {
		ids, truncated := employeeAuditResourceIDs(items, 50)
		details["resource_ids"] = ids
		if truncated {
			details["resource_ids_truncated"] = true
		}
	}
	return c.audit(ctx, "hr.employee.sensitive_field.read", resource, target, string(SeverityHigh), details)
}

// visibleSensitiveEmployeeFields 回傳本次響應中以明文返回的預設敏感欄位。
func visibleSensitiveEmployeeFields(decision CheckResult, items []Employee) []string {
	defaults := defaultFieldPolicies(AppHR, ResourceEmployee)
	fields := make([]string, 0, len(defaults))
	for field, defaultEffect := range defaults {
		if !employeeFieldPolicyRestrictsRead(defaultEffect) {
			continue
		}
		if employeeFieldPolicyRestrictsRead(decision.FieldPolicies[field]) {
			continue
		}
		if employeeItemsHaveFieldValue(items, field) {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	return fields
}

func employeeFieldPolicyRestrictsRead(effect string) bool {
	switch strings.TrimSpace(effect) {
	case "mask", "hide", "deny":
		return true
	default:
		return false
	}
}

func employeeItemsHaveFieldValue(items []Employee, field string) bool {
	for _, item := range items {
		if employeeFieldHasValue(item, field) {
			return true
		}
	}
	return false
}

func employeeFieldHasValue(item Employee, field string) bool {
	switch field {
	case "personal_email":
		return stringHasVisibleValue(item.PersonalEmail)
	case "phone":
		return stringHasVisibleValue(item.Phone)
	case "insurance_info":
		return len(item.InsuranceInfo) > 0
	default:
		return mapFieldHasVisibleValue(item.BasicInfo, field) ||
			mapFieldHasVisibleValue(item.ContactInfo, field) ||
			mapFieldHasVisibleValue(item.InsuranceInfo, field) ||
			mapFieldHasVisibleValue(item.EmploymentInfo, field) ||
			mapFieldHasVisibleValue(item.EducationMilitaryInfo, field)
	}
}

func mapFieldHasVisibleValue(values map[string]any, field string) bool {
	if len(values) == 0 {
		return false
	}
	value, ok := values[field]
	if !ok {
		return false
	}
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return stringHasVisibleValue(v)
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	default:
		return true
	}
}

func stringHasVisibleValue(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "****"
}

func employeeAuditResourceIDs(items []Employee, limit int) ([]string, bool) {
	if limit <= 0 {
		limit = 50
	}
	ids := make([]string, 0, minInt(len(items), limit))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		ids = append(ids, item.ID)
		if len(ids) >= limit {
			return ids, len(items) > len(ids)
		}
	}
	return ids, false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// employeeImportInputFromRecord 處理員工 import 輸入 來源 record。
func employeeImportInputFromRecord(record []string) map[string]string {
	input := make(map[string]string, len(employeeImportColumns))
	for i, column := range employeeImportColumns {
		input[column.header] = record[i]
	}
	return input
}

// employeeCreateInputFromImportRecord 處理員工 create 輸入 來源 import record。
func employeeCreateInputFromImportRecord(record []string) CreateEmployeeInput {
	email := strings.TrimSpace(record[employeeImportColumnEmail])
	name := strings.TrimSpace(record[employeeImportColumnName])
	orgUnitID := strings.TrimSpace(record[employeeImportColumnOrgUnit])
	managerEmployeeID := strings.TrimSpace(record[employeeImportColumnManagerEmployeeID])
	position := strings.TrimSpace(record[employeeImportColumnPosition])
	category := normalizeEmployeeCategory(record[employeeImportColumnCategory])
	phone := strings.TrimSpace(record[employeeImportColumnPhone])
	status := normalizeEmployeeStatus(record[employeeImportColumnStatus])
	accountPolicy := normalizeEmployeeAccountPolicy(record[employeeImportColumnAccountPolicy])
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
		AccountPolicy:     accountPolicy,
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

// employeeHotValue 處理員工 hot value。
func employeeHotValue(employee Employee, field string) string {
	for _, source := range employeeHotFieldSources[field] {
		value := employeeSourceValue(employee, source)
		if value != "" {
			return value
		}
	}
	return ""
}

// employeeSourceValue 處理員工 source value。
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

const maxEmployeeAvatarBytes = 2 << 20

// PreviewCreateEmployee 預覽create 員工的服務流程。
