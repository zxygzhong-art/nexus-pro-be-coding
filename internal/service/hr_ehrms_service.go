package service

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

const (
	ehrmsFieldEmployeeNo      = "員工編號"
	ehrmsFieldName            = "中文姓名"
	ehrmsFieldNameEN          = "英文姓名"
	ehrmsFieldFirstName       = "First Name"
	ehrmsFieldLastName        = "Last Name"
	ehrmsFieldGender          = "性別"
	ehrmsFieldBirthDate       = "生日"
	ehrmsFieldHireDate        = "到職日期"
	ehrmsFieldTenureStartDate = "年資起始日"
	ehrmsFieldProbationEnd    = "試用期滿日"
	ehrmsFieldEmployeeStatus  = "在職狀態"
	ehrmsFieldNationality     = "國籍名稱"
	ehrmsFieldNationalID      = "身份證號"
	ehrmsFieldPassportNo      = "護照號碼"
	ehrmsFieldIdentityType    = "身份類別名稱"
	ehrmsFieldEducation       = "最高學歷"
	ehrmsFieldSchoolName      = "學校名稱(中文)"
	ehrmsFieldDepartmentCode  = "部門代碼"
	ehrmsFieldDepartmentName  = "部門中文名稱"
	ehrmsFieldDepartmentEN    = "部門英文名稱"
	ehrmsFieldPositionCode    = "職務代碼"
	ehrmsFieldPositionName    = "職務中文名稱"
	ehrmsFieldPositionEN      = "職務英文名稱"
	ehrmsFieldCardNo          = "卡號"
	ehrmsFieldClockRequired   = "上下班刷卡"
	ehrmsFieldShiftName       = "員工班別名稱"
	ehrmsFieldShiftType       = "員工班別屬性"
	ehrmsFieldDirectIndirect  = "直接/間接員工"
)

type ehrmsEmployeeWrite struct {
	rowNumber int
	employee  Employee
	previous  Employee
	update    bool
}

// SyncEHRMSEmployees 同步 eHRMS 員工的服務流程。
func (c HRService) SyncEHRMSEmployees(ctx RequestContext, input EHRMSEmployeeSyncInput) (EHRMSEmployeeSyncResponse, error) {
	if c.ehrmsClient == nil {
		return EHRMSEmployeeSyncResponse{}, BadRequest("eHRMS is not configured")
	}
	mode, err := normalizeEHRMSSyncMode(input.Mode)
	if err != nil {
		return EHRMSEmployeeSyncResponse{}, err
	}
	req := CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionImport}
	account, decision, authzAudit, err := c.Service.Authorize(ctx, req, AuditTarget{Event: "hr.employee.ehrms.sync", Resource: string(ResourceEmployee)})
	if err != nil {
		return EHRMSEmployeeSyncResponse{}, err
	}
	records, err := c.ehrmsClient.ListEmployees(goContext(ctx))
	if err != nil {
		c.logWarn(ctx, "eHRMS employee fetch failed", "error", err)
		return EHRMSEmployeeSyncResponse{}, BadRequest("fetch eHRMS employees failed")
	}
	response := EHRMSEmployeeSyncResponse{Fetched: len(records), Mode: mode}
	departments := ehrmsOrgUnits(ctx.TenantID, records, c.Now())
	var validationErr error
	if err := c.withTransaction(ctx, func(tx HRService) error {
		for _, unit := range departments {
			if err := tx.store.UpsertOrgUnit(goContext(ctx), unit); err != nil {
				return err
			}
		}
		writes, rowErrors, results, err := tx.prepareEHRMSSyncWrites(ctx, account, decision, records, mode)
		if err != nil {
			return err
		}
		if len(rowErrors) > 0 {
			response.Failed = len(records)
			response.RowErrors = rowErrors
			response.Results = results
			validationErr = domain.ImportValidationFailed("eHRMS employee sync contains invalid rows", rowErrors)
			return validationErr
		}
		created, updated := 0, 0
		results = make([]BatchEmployeeResult, 0, len(writes))
		for _, item := range writes {
			if err := tx.store.UpsertEmployee(goContext(ctx), item.employee); err != nil {
				return err
			}
			if err := tx.touchEmployeeAuthzIfNeeded(ctx, item.previous, item.employee, string(EventEmployeeAuthzSubjectImport)); err != nil {
				return err
			}
			if err := tx.linkEmployeeAccount(ctx, item.employee); err != nil {
				return err
			}
			eventType := string(EventEmployeeCreated)
			action := "created"
			if item.update {
				eventType = string(EventEmployeeUpdated)
				action = "updated"
				updated++
			} else {
				created++
			}
			if err := tx.appendEmployeeEvent(ctx, eventType, item.employee.ID, map[string]any{
				"employee_id": item.employee.ID,
				"employee_no": item.employee.EmployeeNo,
				"source":      "ehrms",
				"action":      action,
			}); err != nil {
				return err
			}
			results = append(results, BatchEmployeeResult{RowNumber: item.rowNumber, EmployeeID: item.employee.ID, Success: true, Action: action, Message: action})
		}
		response.Created = created
		response.Updated = updated
		response.DepartmentsUpserted = len(departments)
		response.Results = results
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeImported), "ehrms", map[string]any{
			"source":               "ehrms",
			"fetched":              response.Fetched,
			"created":              created,
			"updated":              updated,
			"departments_upserted": len(departments),
			"mode":                 mode,
		}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.ehrms.sync", string(ResourceEmployeeImport), "ehrms", string(SeverityHigh), map[string]any{
			"source":               "ehrms",
			"fetched":              response.Fetched,
			"created":              created,
			"updated":              updated,
			"failed":               response.Failed,
			"departments_upserted": len(departments),
			"mode":                 mode,
		}); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		if validationErr != nil {
			return response, validationErr
		}
		return EHRMSEmployeeSyncResponse{}, err
	}
	if validationErr != nil {
		return response, validationErr
	}
	c.logInfo(ctx, "eHRMS employee sync completed",
		"fetched", response.Fetched,
		"created", response.Created,
		"updated", response.Updated,
		"departments_upserted", response.DepartmentsUpserted,
		"mode", mode,
	)
	return response, nil
}

// prepareEHRMSSyncWrites 處理 prepare eHRMS sync writes 的服務流程。
func (c HRService) prepareEHRMSSyncWrites(ctx RequestContext, account Account, decision CheckResult, records []EHRMSEmployeeRecord, mode string) ([]ehrmsEmployeeWrite, []RowError, []BatchEmployeeResult, error) {
	writes := make([]ehrmsEmployeeWrite, 0, len(records))
	rowErrors := make([]RowError, 0)
	results := make([]BatchEmployeeResult, 0)
	seenEmployeeNos := map[string]int{}
	seenNationalIDs := map[string]int{}
	lookup, err := c.ehrmsValidationLookup(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	for idx, record := range records {
		rowNumber := idx + 1
		employee, errors, err := c.ehrmsEmployeeCandidate(ctx, record, rowNumber)
		if err != nil {
			return nil, nil, nil, err
		}
		errors = append(errors, ehrmsBatchErrors(rowNumber, employee, seenEmployeeNos, seenNationalIDs)...)
		existing, ok, err := c.store.GetEmployeeByEmployeeNo(goContext(ctx), ctx.TenantID, employee.EmployeeNo)
		if err != nil {
			return nil, nil, nil, err
		}
		update := ok
		switch mode {
		case employeeImportModeCreate:
			if ok {
				errors = append(errors, RowError{Row: rowNumber, Field: "employee_no", Code: "unique", Message: "employee_no already exists"})
			}
		case employeeImportModeUpdate:
			if !ok {
				errors = append(errors, RowError{Row: rowNumber, Field: "employee_no", Code: "not_found", Message: "employee_no was not found for eHRMS sync"})
			}
		}
		if ok {
			employee = ehrmsMergeEmployee(existing, employee)
		}
		if len(errors) == 0 {
			validateErrors, err := c.validateEHRMSEmployee(ctx, employee, rowNumber, lookup)
			if err != nil {
				return nil, nil, nil, err
			}
			errors = append(errors, validateErrors...)
		}
		if len(errors) == 0 {
			scopeErrors, err := c.employeeImportScopeErrors(ctx, account, rowNumber, employee, existing, update, decision)
			if err != nil {
				return nil, nil, nil, err
			}
			errors = append(errors, scopeErrors...)
		}
		if len(errors) > 0 {
			rowErrors = append(rowErrors, errors...)
			results = append(results, BatchEmployeeResult{RowNumber: rowNumber, Success: false, Code: "import_validation_failed", Message: firstRowErrorMessage(errors)})
			continue
		}
		if len(employee.InternalExperiences) == 0 {
			employee.InternalExperiences = append(employee.InternalExperiences, c.newEmployeeExperience(employee, "eHRMS sync"))
		}
		writes = append(writes, ehrmsEmployeeWrite{rowNumber: rowNumber, employee: employee, previous: existing, update: update})
	}
	return writes, rowErrors, results, nil
}

// ehrmsEmployeeCandidate 處理 eHRMS 員工候選的服務流程。
func (c HRService) ehrmsEmployeeCandidate(ctx RequestContext, record EHRMSEmployeeRecord, rowNumber int) (Employee, []RowError, error) {
	status := normalizeEmployeeStatus(ehrmsValue(record, ehrmsFieldEmployeeStatus))
	input := CreateEmployeeInput{
		EmployeeNo:       ehrmsValue(record, ehrmsFieldEmployeeNo),
		Name:             ehrmsValue(record, ehrmsFieldName),
		OrgUnitID:        ehrmsValue(record, ehrmsFieldDepartmentCode),
		Position:         utils.FirstNonEmpty(ehrmsValue(record, ehrmsFieldPositionName), ehrmsValue(record, ehrmsFieldPositionCode)),
		Category:         ehrmsEmployeeCategory(record),
		Status:           status,
		EmploymentStatus: status,
		HireDate:         normalizeImportDate(ehrmsValue(record, ehrmsFieldHireDate)),
		BasicInfo: map[string]any{
			"name":               ehrmsValue(record, ehrmsFieldName),
			"name_en":            ehrmsValue(record, ehrmsFieldNameEN),
			"first_name":         ehrmsValue(record, ehrmsFieldFirstName),
			"last_name":          ehrmsValue(record, ehrmsFieldLastName),
			"gender":             ehrmsValue(record, ehrmsFieldGender),
			"birth_date":         normalizeImportDate(ehrmsValue(record, ehrmsFieldBirthDate)),
			"nationality":        ehrmsValue(record, ehrmsFieldNationality),
			"national_id":        ehrmsValue(record, ehrmsFieldNationalID),
			"passport_no":        ehrmsValue(record, ehrmsFieldPassportNo),
			"identity_type_name": ehrmsValue(record, ehrmsFieldIdentityType),
			"source":             "ehrms",
		},
		EmploymentInfo: map[string]any{
			"org_unit_id":              ehrmsValue(record, ehrmsFieldDepartmentCode),
			"org_unit_code":            ehrmsValue(record, ehrmsFieldDepartmentCode),
			"org_unit_name":            ehrmsValue(record, ehrmsFieldDepartmentName),
			"org_unit_name_en":         ehrmsValue(record, ehrmsFieldDepartmentEN),
			"position":                 utils.FirstNonEmpty(ehrmsValue(record, ehrmsFieldPositionName), ehrmsValue(record, ehrmsFieldPositionCode)),
			"position_code":            ehrmsValue(record, ehrmsFieldPositionCode),
			"position_name_en":         ehrmsValue(record, ehrmsFieldPositionEN),
			"category":                 ehrmsEmployeeCategory(record),
			"employment_status":        status,
			"hire_date":                normalizeImportDate(ehrmsValue(record, ehrmsFieldHireDate)),
			"tenure_start_date":        normalizeImportDate(ehrmsValue(record, ehrmsFieldTenureStartDate)),
			"probation_end_date":       normalizeImportDate(ehrmsValue(record, ehrmsFieldProbationEnd)),
			"card_no":                  ehrmsValue(record, ehrmsFieldCardNo),
			"clock_required":           ehrmsValue(record, ehrmsFieldClockRequired),
			"shift_name":               ehrmsValue(record, ehrmsFieldShiftName),
			"shift_type":               ehrmsValue(record, ehrmsFieldShiftType),
			"direct_indirect_employee": ehrmsValue(record, ehrmsFieldDirectIndirect),
			"source":                   "ehrms",
		},
		EducationMilitaryInfo: map[string]any{
			"highest_education": ehrmsValue(record, ehrmsFieldEducation),
			"school_name":       ehrmsValue(record, ehrmsFieldSchoolName),
		},
	}
	employee, err := c.employeeCreateCandidate(ctx, input)
	if err != nil {
		errors, ok := employeeImportErrorsFromError(rowNumber, err)
		if ok {
			return Employee{}, errors, nil
		}
		return Employee{}, nil, err
	}
	return employee, nil, nil
}

// validateEHRMSEmployee 驗證 eHRMS 員工的服務流程。
func (c HRService) validateEHRMSEmployee(ctx RequestContext, employee Employee, rowNumber int, lookup ehrmsValidationLookup) ([]RowError, error) {
	fields := make([]FieldError, 0)
	if strings.TrimSpace(employee.EmployeeNo) == "" {
		fields = append(fields, FieldError{Tab: "basic_info", Field: "employee_no", Code: "required", Message: "employee_no is required"})
	}
	if strings.TrimSpace(employee.Name) == "" {
		fields = append(fields, FieldError{Tab: "basic_info", Field: "name", Code: "required", Message: "name is required"})
	}
	if employee.Category != "" && !validEmployeeCategory(employee.Category) {
		fields = append(fields, FieldError{Tab: "employment_info", Field: "category", Code: "invalid", Message: "category must be one of full_time, part_time, intern, contractor, other"})
	}
	if !validEmployeeStatus(employeeStatus(employee), true) {
		fields = append(fields, FieldError{Tab: "employment_info", Field: "employment_status", Code: "invalid", Message: "employment_status must be one of active, probation, leave_suspended, onboarding, resigned, deleted"})
	}
	if strings.TrimSpace(employee.OrgUnitID) != "" {
		if _, ok := lookup.orgUnitIDs[employee.OrgUnitID]; !ok {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "org_unit_id", Code: "not_found", Message: "org unit not found"})
		}
	}
	if len(fields) == 0 {
		fields = append(fields, lookup.unique.fieldErrors(employee)...)
	}
	return fieldErrorsToRowErrors(rowNumber, fields), nil
}

type ehrmsValidationLookup struct {
	orgUnitIDs map[string]struct{}
	unique     employeeUniqueIndex
}

// ehrmsValidationLookup 處理 eHRMS 驗證 lookup 的服務流程。
func (c HRService) ehrmsValidationLookup(ctx RequestContext) (ehrmsValidationLookup, error) {
	employees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
	if err != nil {
		return ehrmsValidationLookup{}, err
	}
	units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return ehrmsValidationLookup{}, err
	}
	orgUnitIDs := make(map[string]struct{}, len(units))
	for _, unit := range units {
		orgUnitIDs[unit.ID] = struct{}{}
	}
	return ehrmsValidationLookup{orgUnitIDs: orgUnitIDs, unique: newEmployeeUniqueIndex(employees)}, nil
}

type employeeUniqueIndex struct {
	employeeNo    map[string]Employee
	companyEmail  map[string]Employee
	personalEmail map[string]Employee
	accountID     map[string]Employee
	basicInfo     map[string]map[string]Employee
}

// newEmployeeUniqueIndex 建立員工 unique index。
func newEmployeeUniqueIndex(employees []Employee) employeeUniqueIndex {
	idx := employeeUniqueIndex{
		employeeNo:    map[string]Employee{},
		companyEmail:  map[string]Employee{},
		personalEmail: map[string]Employee{},
		accountID:     map[string]Employee{},
		basicInfo:     map[string]map[string]Employee{},
	}
	for _, employee := range employees {
		if employee.EmployeeNo != "" {
			idx.employeeNo[employee.EmployeeNo] = employee
		}
		if employee.CompanyEmail != "" {
			idx.companyEmail[strings.ToLower(employee.CompanyEmail)] = employee
		}
		if employee.PersonalEmail != "" {
			idx.personalEmail[strings.ToLower(employee.PersonalEmail)] = employee
		}
		if employee.AccountID != "" {
			idx.accountID[employee.AccountID] = employee
		}
		for _, field := range employeeUniqueBasicInfoFields {
			value := stringFromMap(employee.BasicInfo, field)
			if value == "" {
				continue
			}
			if idx.basicInfo[field] == nil {
				idx.basicInfo[field] = map[string]Employee{}
			}
			idx.basicInfo[field][strings.ToLower(value)] = employee
		}
	}
	return idx
}

// fieldErrors 處理欄位錯誤。
func (idx employeeUniqueIndex) fieldErrors(employee Employee) []FieldError {
	fields := make([]FieldError, 0, 8)
	if existing, ok := idx.employeeNo[employee.EmployeeNo]; employee.EmployeeNo != "" && ok && existing.ID != employee.ID {
		fields = append(fields, FieldError{Tab: "basic_info", Field: "employee_no", Code: "unique", Message: "employee_no already exists"})
	}
	if key := strings.ToLower(employee.CompanyEmail); key != "" {
		if existing, ok := idx.companyEmail[key]; ok && existing.ID != employee.ID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "company_email", Code: "unique", Message: "company_email already exists"})
		}
	}
	if key := strings.ToLower(employee.PersonalEmail); key != "" {
		if existing, ok := idx.personalEmail[key]; ok && existing.ID != employee.ID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "personal_email", Code: "unique", Message: "personal_email already exists"})
		}
	}
	if existing, ok := idx.accountID[employee.AccountID]; employee.AccountID != "" && ok && existing.ID != employee.ID {
		fields = append(fields, FieldError{Tab: "basic_info", Field: "account_id", Code: "unique", Message: "account_id already linked"})
	}
	for _, field := range employeeUniqueBasicInfoFields {
		value := strings.ToLower(stringFromMap(employee.BasicInfo, field))
		if value == "" || idx.basicInfo[field] == nil {
			continue
		}
		if existing, ok := idx.basicInfo[field][value]; ok && existing.ID != employee.ID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: field, Code: "unique", Message: field + " already exists"})
		}
	}
	return fields
}

// ehrmsOrgUnits 處理 eHRMS 組織單位。
func ehrmsOrgUnits(tenantID string, records []EHRMSEmployeeRecord, now time.Time) []OrgUnit {
	unitsByID := map[string]OrgUnit{}
	for _, record := range records {
		code := ehrmsValue(record, ehrmsFieldDepartmentCode)
		if code == "" {
			continue
		}
		name := utils.FirstNonEmpty(ehrmsValue(record, ehrmsFieldDepartmentName), ehrmsValue(record, ehrmsFieldDepartmentEN), code)
		unitsByID[code] = OrgUnit{ID: code, TenantID: tenantID, Code: code, Name: name, Path: []string{code}, CreatedAt: now}
	}
	ids := make([]string, 0, len(unitsByID))
	for id := range unitsByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	units := make([]OrgUnit, 0, len(ids))
	for _, id := range ids {
		units = append(units, unitsByID[id])
	}
	return units
}

// normalizeEHRMSSyncMode 正規化eHRMS sync mode。
func normalizeEHRMSSyncMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", employeeImportModeUpsert:
		return employeeImportModeUpsert, nil
	case employeeImportModeCreate:
		return employeeImportModeCreate, nil
	case employeeImportModeUpdate:
		return employeeImportModeUpdate, nil
	default:
		return "", BadRequest("eHRMS sync mode must be create, update, or upsert")
	}
}

// ehrmsMergeEmployee 處理 eHRMS merge 員工。
func ehrmsMergeEmployee(existing Employee, candidate Employee) Employee {
	next := existing
	next.EmployeeNo = candidate.EmployeeNo
	next.Name = candidate.Name
	next.OrgUnitID = candidate.OrgUnitID
	next.Position = candidate.Position
	next.Category = candidate.Category
	next.Status = candidate.Status
	next.EmploymentStatus = candidate.EmploymentStatus
	next.HireDate = candidate.HireDate
	next.BasicInfo = mergeEmployeeImportMap(next.BasicInfo, candidate.BasicInfo)
	next.EmploymentInfo = mergeEmployeeImportMap(next.EmploymentInfo, candidate.EmploymentInfo)
	next.EducationMilitaryInfo = mergeEmployeeImportMap(next.EducationMilitaryInfo, candidate.EducationMilitaryInfo)
	next.UpdatedAt = candidate.UpdatedAt
	return next
}

// ehrmsEmployeeCategory 處理 eHRMS 員工分類。
func ehrmsEmployeeCategory(record EHRMSEmployeeRecord) string {
	switch ehrmsValue(record, ehrmsFieldIdentityType) {
	case "時薪員工":
		return string(EmployeeCategoryPartTime)
	case "約聘類員工":
		return string(EmployeeCategoryContractor)
	case "外籍員工", "一般員工":
		return string(EmployeeCategoryFullTime)
	default:
		return ""
	}
}

// ehrmsBatchErrors 處理 eHRMS 批次錯誤。
func ehrmsBatchErrors(rowNumber int, employee Employee, employeeNos map[string]int, nationalIDs map[string]int) []RowError {
	errors := make([]RowError, 0, 2)
	if employeeNo := strings.TrimSpace(employee.EmployeeNo); employeeNo != "" {
		if firstRow, ok := employeeNos[employeeNo]; ok {
			errors = append(errors, RowError{Row: rowNumber, Field: "employee_no", Code: "duplicate_in_file", Message: fmt.Sprintf("employee_no is duplicated with row %d", firstRow)})
		} else {
			employeeNos[employeeNo] = rowNumber
		}
	}
	if nationalID := strings.TrimSpace(stringFromMap(employee.BasicInfo, "national_id")); nationalID != "" {
		if firstRow, ok := nationalIDs[nationalID]; ok {
			errors = append(errors, RowError{Row: rowNumber, Field: "national_id", Code: "duplicate_in_file", Message: fmt.Sprintf("national_id is duplicated with row %d", firstRow)})
		} else {
			nationalIDs[nationalID] = rowNumber
		}
	}
	return errors
}

// fieldErrorsToRowErrors 處理欄位錯誤 to 列錯誤。
func fieldErrorsToRowErrors(rowNumber int, fields []FieldError) []RowError {
	if len(fields) == 0 {
		return nil
	}
	out := make([]RowError, 0, len(fields))
	for _, field := range fields {
		out = append(out, RowError{Row: rowNumber, Field: field.Field, Code: field.Code, Message: field.Message})
	}
	return out
}

// ehrmsValue 處理 eHRMS value。
func ehrmsValue(record EHRMSEmployeeRecord, key string) string {
	if len(record) == 0 {
		return ""
	}
	return strings.TrimSpace(record[key])
}
