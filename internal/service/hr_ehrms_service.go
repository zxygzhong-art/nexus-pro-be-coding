package service

import (
	"fmt"
	"sort"
	"strings"
	"time"

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
	ehrmsFieldLeaveGroup      = "休假群組"
	ehrmsFieldCompanyEmail    = "公司信箱"
	ehrmsFieldParentDeptCode  = "上級部門代碼"
	ehrmsFieldDeptClosed      = "部門已關閉"
)

type ehrmsEmployeeWrite struct {
	rowNumber int
	employee  Employee
	previous  Employee
	update    bool
}

// SyncEHRMSOrgUnits synchronizes the tenant-wide org catalog only for tenant-wide grants.
func (c HRService) SyncEHRMSOrgUnits(ctx RequestContext) (EHRMSOrgUnitSyncResponse, error) {
	if c.ehrmsClient == nil {
		return EHRMSOrgUnitSyncResponse{}, BadRequest("eHRMS is not configured")
	}
	_, decision, authzAudit, err := c.Service.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceOrgUnit, Action: ActionCreate},
		AuditTarget{Event: "hr.org_unit.ehrms.sync", Resource: string(ResourceOrgUnit)},
	)
	if err != nil {
		return EHRMSOrgUnitSyncResponse{}, err
	}
	if err := requireTenantWideEHRMSSyncScope(decision); err != nil {
		return EHRMSOrgUnitSyncResponse{}, err
	}
	departmentRecords, err := c.ehrmsClient.ListDepartments(goContext(ctx))
	if err != nil {
		c.logWarn(ctx, "eHRMS department fetch failed", "error", err)
		return EHRMSOrgUnitSyncResponse{}, ehrmsFetchError("departments", err)
	}
	departments := EHRMSOrgUnitsFromDepartments(ctx.TenantID, departmentRecords, c.Now())
	response := EHRMSOrgUnitSyncResponse{Fetched: len(departmentRecords)}
	if err := c.withTransaction(ctx, func(tx HRService) error {
		upserted, err := tx.UpsertEHRMSOrgUnits(ctx, departments)
		if err != nil {
			return err
		}
		response.Upserted = upserted
		if err := tx.audit(ctx, "hr.org_unit.ehrms.sync", string(ResourceOrgUnit), "ehrms", string(SeverityHigh), map[string]any{
			"source":   "ehrms",
			"fetched":  response.Fetched,
			"upserted": upserted,
		}); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		return EHRMSOrgUnitSyncResponse{}, err
	}
	c.logInfo(ctx, "eHRMS org unit sync completed", "fetched", response.Fetched, "upserted", response.Upserted)
	return response, nil
}

// SyncEHRMSPositions synchronizes the tenant-wide position catalog only for tenant-wide grants.
func (c HRService) SyncEHRMSPositions(ctx RequestContext) (EHRMSPositionSyncResponse, error) {
	if c.ehrmsClient == nil {
		return EHRMSPositionSyncResponse{}, BadRequest("eHRMS is not configured")
	}
	_, decision, authzAudit, err := c.Service.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourcePosition, Action: ActionCreate},
		AuditTarget{Event: "hr.position.ehrms.sync", Resource: string(ResourcePosition)},
	)
	if err != nil {
		return EHRMSPositionSyncResponse{}, err
	}
	if err := requireTenantWideEHRMSSyncScope(decision); err != nil {
		return EHRMSPositionSyncResponse{}, err
	}
	positionRecords, err := c.ehrmsClient.ListPositions(goContext(ctx))
	if err != nil {
		c.logWarn(ctx, "eHRMS position fetch failed", "error", err)
		return EHRMSPositionSyncResponse{}, ehrmsFetchError("positions", err)
	}
	positions := EHRMSPositionsFromRecords(ctx.TenantID, positionRecords, c.Now())
	response := EHRMSPositionSyncResponse{Fetched: len(positionRecords)}
	if err := c.withTransaction(ctx, func(tx HRService) error {
		upserted, err := tx.UpsertEHRMSPositions(ctx, positions)
		if err != nil {
			return err
		}
		response.Upserted = upserted
		if err := tx.audit(ctx, "hr.position.ehrms.sync", string(ResourcePosition), "ehrms", string(SeverityHigh), map[string]any{
			"source":   "ehrms",
			"fetched":  response.Fetched,
			"upserted": upserted,
		}); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		return EHRMSPositionSyncResponse{}, err
	}
	c.logInfo(ctx, "eHRMS position sync completed", "fetched", response.Fetched, "upserted", response.Upserted)
	return response, nil
}

// SyncEHRMSEmployees synchronizes tenant-wide employee data only for tenant-wide grants.
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
	if err := requireTenantWideEHRMSSyncScope(decision); err != nil {
		return EHRMSEmployeeSyncResponse{}, err
	}
	departmentRecords, err := c.ehrmsClient.ListDepartments(goContext(ctx))
	if err != nil {
		c.logWarn(ctx, "eHRMS department fetch failed", "error", err)
		return EHRMSEmployeeSyncResponse{}, ehrmsFetchError("departments", err)
	}
	positionRecords, err := c.ehrmsClient.ListPositions(goContext(ctx))
	if err != nil {
		c.logWarn(ctx, "eHRMS position fetch failed", "error", err)
		return EHRMSEmployeeSyncResponse{}, ehrmsFetchError("positions", err)
	}
	records, err := c.ehrmsClient.ListEmployees(goContext(ctx))
	if err != nil {
		c.logWarn(ctx, "eHRMS employee fetch failed", "error", err)
		return EHRMSEmployeeSyncResponse{}, ehrmsFetchError("employees", err)
	}
	response := EHRMSEmployeeSyncResponse{Fetched: len(records), Mode: mode}
	now := c.Now()
	departments := EHRMSOrgUnitsFromDepartments(ctx.TenantID, departmentRecords, now)
	positions := EHRMSPositionsFromRecords(ctx.TenantID, positionRecords, now)
	provisionQueued := false
	if err := c.withTransaction(ctx, func(tx HRService) error {
		c.logInfo(ctx, "eHRMS sync step started", "step", "departments", "fetched", len(departmentRecords))
		if _, err := tx.UpsertEHRMSOrgUnits(ctx, departments); err != nil {
			return err
		}
		c.logInfo(ctx, "eHRMS sync step completed", "step", "departments", "upserted", len(departments))
		c.logInfo(ctx, "eHRMS sync step started", "step", "positions", "fetched", len(positionRecords))
		if _, err := tx.UpsertEHRMSPositions(ctx, positions); err != nil {
			return err
		}
		c.logInfo(ctx, "eHRMS sync step completed", "step", "positions", "upserted", len(positions))
		c.logInfo(ctx, "eHRMS sync step started", "step", "employees", "fetched", len(records))
		writes, rowErrors, skippedResults, err := tx.prepareEHRMSSyncWrites(ctx, account, decision, records, mode)
		if err != nil {
			return err
		}
		created, updated := 0, 0
		results := make([]BatchEmployeeResult, 0, len(writes)+len(skippedResults))
		results = append(results, skippedResults...)
		for _, item := range writes {
			accountCreated, err := tx.ensureEHRMSEmployeeAccount(ctx, &item.employee)
			if err != nil {
				return err
			}
			if err := tx.store.UpsertEmployee(goContext(ctx), item.employee); err != nil {
				return err
			}
			if err := tx.touchEmployeeAuthzIfNeeded(ctx, item.previous, item.employee, string(EventEmployeeAuthzSubjectImport)); err != nil {
				return err
			}
			if err := tx.linkEmployeeAccount(ctx, item.employee); err != nil {
				return err
			}
			if accountCreated && item.employee.AccountID != "" {
				if err := tx.provisionEmployeeIdentityFromAccountID(ctx, item.employee, item.employee.AccountID, true); err != nil {
					return err
				}
				provisionQueued = true
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
			eventPayload := map[string]any{
				"employee_id": item.employee.ID,
				"employee_no": item.employee.EmployeeNo,
				"source":      "ehrms",
				"action":      action,
			}
			if item.employee.AccountID != "" {
				eventPayload["account_id"] = item.employee.AccountID
				eventPayload["account_policy"] = string(EmployeeAccountPolicyCreatePendingInvite)
			}
			if err := tx.appendEmployeeEvent(ctx, eventType, item.employee.ID, eventPayload); err != nil {
				return err
			}
			if accountCreated && item.employee.AccountID != "" {
				if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeInvited), item.employee.ID, map[string]any{
					"employee_id":    item.employee.ID,
					"account_id":     item.employee.AccountID,
					"account_policy": string(EmployeeAccountPolicyCreatePendingInvite),
					"source":         "ehrms",
				}); err != nil {
					return err
				}
			}
			results = append(results, BatchEmployeeResult{RowNumber: item.rowNumber, EmployeeID: item.employee.ID, Success: true, Action: action, Message: action})
		}
		sort.SliceStable(results, func(i, j int) bool {
			return results[i].RowNumber < results[j].RowNumber
		})
		response.Created = created
		response.Updated = updated
		response.Failed = len(skippedResults)
		response.RowErrors = rowErrors
		response.DepartmentsUpserted = len(departments)
		response.PositionsUpserted = len(positions)
		response.Results = results
		c.logInfo(ctx, "eHRMS sync step completed",
			"step", "employees",
			"created", created,
			"updated", updated,
			"failed", len(skippedResults),
			"mode", mode,
		)
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeImported), "ehrms", map[string]any{
			"source":               "ehrms",
			"fetched":              response.Fetched,
			"created":              created,
			"updated":              updated,
			"failed":               len(skippedResults),
			"departments_upserted": len(departments),
			"positions_upserted":   len(positions),
			"mode":                 mode,
		}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.ehrms.sync", string(ResourceEmployeeImport), "ehrms", string(SeverityHigh), map[string]any{
			"source":               "ehrms",
			"fetched":              response.Fetched,
			"created":              created,
			"updated":              updated,
			"skipped":              response.Skipped,
			"failed":               response.Failed,
			"departments_upserted": len(departments),
			"positions_upserted":   len(positions),
			"mode":                 mode,
		}); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		return EHRMSEmployeeSyncResponse{}, err
	}
	if provisionQueued {
		c.runIdentityProvisioningFastPath(ctx)
	}
	c.logInfo(ctx, "eHRMS employee sync completed",
		"fetched", response.Fetched,
		"created", response.Created,
		"updated", response.Updated,
		"skipped", response.Skipped,
		"departments_upserted", response.DepartmentsUpserted,
		"positions_upserted", response.PositionsUpserted,
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
	seenCompanyEmails := map[string]int{}
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
		errors = append(errors, ehrmsBatchErrors(rowNumber, employee, seenEmployeeNos, seenNationalIDs, seenCompanyEmails)...)
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
			employee = EHRMSMergeEmployee(existing, employee)
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
			c.logWarn(ctx, "eHRMS employee sync row skipped",
				"row", rowNumber,
				"employee_no", employee.EmployeeNo,
				"code", errors[0].Code,
				"field", errors[0].Field,
				"message", firstRowErrorMessage(errors),
				"error_count", len(errors),
			)
			results = append(results, BatchEmployeeResult{
				RowNumber: rowNumber,
				Success:   false,
				Action:    "failed",
				Code:      errors[0].Code,
				Message:   firstRowErrorMessage(errors),
			})
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
	positionCode := ehrmsValue(record, ehrmsFieldPositionCode)
	companyEmail := strings.ToLower(strings.TrimSpace(ehrmsValue(record, ehrmsFieldCompanyEmail)))
	input := CreateEmployeeInput{
		EmployeeNo:       ehrmsValue(record, ehrmsFieldEmployeeNo),
		Name:             ehrmsValue(record, ehrmsFieldName),
		CompanyEmail:     companyEmail,
		OrgUnitID:        ehrmsValue(record, ehrmsFieldDepartmentCode),
		PositionID:       positionCode,
		Position:         utils.FirstNonEmpty(ehrmsValue(record, ehrmsFieldPositionName), positionCode),
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
			"company_email":      companyEmail,
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
			"leave_group":              ehrmsValue(record, ehrmsFieldLeaveGroup),
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
	if err := c.ensureEmployeePosition(ctx, &employee, positionCode == ""); err != nil {
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

// UpsertEHRMSOrgUnits persists normalized upstream departments while preserving local ownership fields.
func (c HRService) UpsertEHRMSOrgUnits(ctx RequestContext, departments []OrgUnit) (int, error) {
	departments, err := c.attachEHRMSRootsToCanonicalRoot(ctx, departments)
	if err != nil {
		return 0, err
	}
	for _, unit := range departments {
		before, ok, err := c.store.GetOrgUnit(goContext(ctx), ctx.TenantID, unit.ID)
		if err != nil {
			return 0, err
		}
		if ok && unit.ManagerPositionID == "" {
			unit.ManagerPositionID = before.ManagerPositionID
		}
		if err := c.store.UpsertOrgUnit(goContext(ctx), unit); err != nil {
			return 0, err
		}
		if !ok {
			before = OrgUnit{}
		}
		if err := c.Service.syncOrgUnitRelationshipTuples(ctx, before, unit); err != nil {
			return 0, err
		}
	}
	return len(departments), nil
}

// attachEHRMSRootsToCanonicalRoot 將 eHRMS 的多個根部門收斂到租戶唯一根節點下。
func (c HRService) attachEHRMSRootsToCanonicalRoot(ctx RequestContext, departments []OrgUnit) ([]OrgUnit, error) {
	if len(departments) == 0 {
		return departments, nil
	}
	existing, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	rootID := ""
	rootClosed := false
	for _, unit := range existing {
		if strings.TrimSpace(unit.ParentID) == "" {
			rootID = unit.ID
			rootClosed = unit.Closed
			break
		}
	}
	if rootID == "" {
		for _, unit := range departments {
			if strings.TrimSpace(unit.ParentID) == "" {
				rootID = unit.ID
				rootClosed = unit.Closed
				break
			}
		}
	}
	if rootID == "" {
		return nil, Conflict("organization must have exactly one top-level org unit")
	}
	out := make([]OrgUnit, 0, len(departments))
	for _, unit := range departments {
		if unit.ID != rootID && (len(unit.Path) == 0 || unit.Path[0] != rootID) {
			unit.Path = append([]string{rootID}, unit.Path...)
		}
		if unit.ID != rootID && strings.TrimSpace(unit.ParentID) == "" {
			unit.ParentID = rootID
		}
		if unit.ID != rootID && rootClosed {
			unit.Closed = true
		}
		out = append(out, unit)
	}
	return out, nil
}

// UpsertEHRMSPositions persists normalized upstream positions while preserving local organization assignments.
func (c HRService) UpsertEHRMSPositions(ctx RequestContext, positions []Position) (int, error) {
	for _, position := range positions {
		before, ok, err := c.store.GetPosition(goContext(ctx), ctx.TenantID, position.ID)
		if err != nil {
			return 0, err
		}
		if ok && position.OrgUnitID == "" {
			position.OrgUnitID = before.OrgUnitID
		}
		if err := c.store.UpsertPosition(goContext(ctx), position); err != nil {
			return 0, err
		}
	}
	return len(positions), nil
}

// EHRMSOrgUnitsFromDepartments maps upstream department records into the canonical organization hierarchy.
func EHRMSOrgUnitsFromDepartments(tenantID string, records []EHRMSDepartmentRecord, now time.Time) []OrgUnit {
	unitsByID := make(map[string]OrgUnit, len(records))
	for _, record := range records {
		code := ehrmsValue(record, ehrmsFieldDepartmentCode)
		if code == "" {
			continue
		}
		rawName := utils.FirstNonEmpty(ehrmsValue(record, ehrmsFieldDepartmentName), ehrmsValue(record, ehrmsFieldDepartmentEN), code)
		rawNameEN := ehrmsValue(record, ehrmsFieldDepartmentEN)
		name, nameClosed := EHRMSCleanDepartmentName(rawName)
		nameEN, nameENClosed := EHRMSCleanDepartmentName(rawNameEN)
		closed := ehrmsBool(record, ehrmsFieldDeptClosed) || nameClosed || nameENClosed
		if name == "" {
			name = code
		}
		unitsByID[code] = OrgUnit{
			ID:        code,
			TenantID:  tenantID,
			Code:      code,
			Name:      name,
			NameEN:    nameEN,
			ParentID:  ehrmsValue(record, ehrmsFieldParentDeptCode),
			Closed:    closed,
			CreatedAt: now,
			UpdatedAt: now,
			Source:    "ehrms",
		}
	}
	for code := range unitsByID {
		unit := unitsByID[code]
		if unit.ParentID != "" {
			if _, ok := unitsByID[unit.ParentID]; !ok {
				unit.ParentID = ""
			}
		}
		unit.Path = ehrmsOrgUnitPath(code, unitsByID)
		unitsByID[code] = unit
	}
	for _, unit := range ehrmsSortedOrgUnits(unitsByID) {
		if parent, ok := unitsByID[unit.ParentID]; ok && parent.Closed {
			unit.Closed = true
			unitsByID[unit.ID] = unit
		}
	}
	return ehrmsSortedOrgUnits(unitsByID)
}

// EHRMSPositionsFromRecords maps upstream position records into the canonical position catalog.
func EHRMSPositionsFromRecords(tenantID string, records []EHRMSPositionRecord, now time.Time) []Position {
	byCode := map[string]Position{}
	for _, record := range records {
		code := ehrmsValue(record, ehrmsFieldPositionCode)
		if code == "" {
			continue
		}
		name := utils.FirstNonEmpty(ehrmsValue(record, ehrmsFieldPositionName), ehrmsValue(record, ehrmsFieldPositionEN), code)
		if existing, ok := byCode[code]; ok && strings.TrimSpace(existing.Name) != "" {
			continue
		}
		byCode[code] = Position{
			ID:        code,
			TenantID:  tenantID,
			Code:      code,
			Name:      name,
			NameEN:    ehrmsValue(record, ehrmsFieldPositionEN),
			Status:    string(PositionStatusActive),
			Source:    "ehrms",
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	ids := make([]string, 0, len(byCode))
	for id := range byCode {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	positions := make([]Position, 0, len(ids))
	for _, id := range ids {
		positions = append(positions, byCode[id])
	}
	return positions
}

// ehrmsOrgUnits 從員工資料彙整組織單位（測試與相容保留）。
func ehrmsOrgUnits(tenantID string, records []EHRMSEmployeeRecord, now time.Time) []OrgUnit {
	deptRecords := make([]EHRMSDepartmentRecord, 0, len(records))
	codes := map[string]struct{}{}
	for _, record := range records {
		code := ehrmsValue(record, ehrmsFieldDepartmentCode)
		if code == "" {
			continue
		}
		codes[code] = struct{}{}
		deptRecords = append(deptRecords, EHRMSDepartmentRecord{
			ehrmsFieldDepartmentCode: code,
			ehrmsFieldDepartmentName: ehrmsValue(record, ehrmsFieldDepartmentName),
			ehrmsFieldDepartmentEN:   ehrmsValue(record, ehrmsFieldDepartmentEN),
		})
	}
	for i, record := range deptRecords {
		code := ehrmsValue(record, ehrmsFieldDepartmentCode)
		parent := EHRMSInferParentDeptCode(code, codes)
		if parent != "" {
			deptRecords[i][ehrmsFieldParentDeptCode] = parent
		}
	}
	return EHRMSOrgUnitsFromDepartments(tenantID, deptRecords, now)
}

// ehrmsPositions 從員工資料彙整崗位目錄（測試與相容保留）。
func ehrmsPositions(tenantID string, records []EHRMSEmployeeRecord, now time.Time) []Position {
	positionRecords := make([]EHRMSPositionRecord, 0, len(records))
	for _, record := range records {
		code := ehrmsValue(record, ehrmsFieldPositionCode)
		if code == "" {
			continue
		}
		positionRecords = append(positionRecords, EHRMSPositionRecord{
			ehrmsFieldPositionCode: code,
			ehrmsFieldPositionName: ehrmsValue(record, ehrmsFieldPositionName),
			ehrmsFieldPositionEN:   ehrmsValue(record, ehrmsFieldPositionEN),
		})
	}
	return EHRMSPositionsFromRecords(tenantID, positionRecords, now)
}

// EHRMSInferParentDeptCode selects the longest existing department-code prefix as the parent.
func EHRMSInferParentDeptCode(code string, codes map[string]struct{}) string {
	for length := len(code) - 1; length > 0; length-- {
		prefix := code[:length]
		if _, ok := codes[prefix]; ok {
			return prefix
		}
	}
	return ""
}

// ehrmsOrgUnitPath 依 parent 鏈建立 org unit path。
func ehrmsOrgUnitPath(code string, unitsByID map[string]OrgUnit) []string {
	path := []string{}
	current := code
	seen := map[string]struct{}{}
	for current != "" {
		if _, ok := seen[current]; ok {
			break
		}
		seen[current] = struct{}{}
		path = append([]string{current}, path...)
		unit, ok := unitsByID[current]
		if !ok {
			break
		}
		current = strings.TrimSpace(unit.ParentID)
	}
	if len(path) == 0 {
		return []string{code}
	}
	return path
}

// ehrmsSortedOrgUnits 依 path 深度排序，確保上級組織先 upsert。
func ehrmsSortedOrgUnits(unitsByID map[string]OrgUnit) []OrgUnit {
	units := make([]OrgUnit, 0, len(unitsByID))
	for _, unit := range unitsByID {
		units = append(units, unit)
	}
	sort.Slice(units, func(i, j int) bool {
		if len(units[i].Path) != len(units[j].Path) {
			return len(units[i].Path) < len(units[j].Path)
		}
		return units[i].ID < units[j].ID
	})
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

// EHRMSMergeEmployee merges upstream employee data without overwriting self-managed profile fields.
func EHRMSMergeEmployee(existing Employee, candidate Employee) Employee {
	next := existing
	selfNameEN := next.BasicInfo["name_en"]
	nameENSource := stringFromAny(next.BasicInfo["name_en_source"])
	next.EmployeeNo = candidate.EmployeeNo
	next.Name = candidate.Name
	next.CompanyEmail = candidate.CompanyEmail
	next.OrgUnitID = candidate.OrgUnitID
	next.Position = candidate.Position
	next.PositionID = candidate.PositionID
	next.Category = candidate.Category
	next.Status = candidate.Status
	next.EmploymentStatus = candidate.EmploymentStatus
	next.HireDate = candidate.HireDate
	next.ResignDate = candidate.ResignDate
	next.BasicInfo = mergeEmployeeImportMap(next.BasicInfo, candidate.BasicInfo)
	if nameENSource == "self" {
		next.BasicInfo["name_en"] = selfNameEN
		next.BasicInfo["name_en_source"] = "self"
	}
	next.EmploymentInfo = mergeEmployeeImportMap(next.EmploymentInfo, candidate.EmploymentInfo)
	next.EducationMilitaryInfo = mergeEmployeeImportMap(next.EducationMilitaryInfo, candidate.EducationMilitaryInfo)
	if next.BasicInfo == nil {
		next.BasicInfo = map[string]any{}
	}
	// EHRMS email 以 upstream 為準（含空值覆蓋），不受 merge 跳過空字串影響。
	next.BasicInfo["company_email"] = candidate.CompanyEmail
	next.UpdatedAt = candidate.UpdatedAt
	return next
}

// ensureEHRMSEmployeeAccount 依公司信箱建立 pending_invite 帳號，並供 Keycloak invite 開通。
func (c HRService) ensureEHRMSEmployeeAccount(ctx RequestContext, employee *Employee) (bool, error) {
	email := strings.ToLower(strings.TrimSpace(employee.CompanyEmail))
	employee.CompanyEmail = email
	if email == "" || employeeTerminalStatus(employeeStatus(*employee)) {
		return false, nil
	}
	if employee.AccountID != "" {
		account, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, employee.AccountID)
		if err != nil {
			return false, err
		}
		if ok {
			before := account
			account.Email = email
			account.DisplayName = utils.FirstNonEmpty(account.DisplayName, employee.Name)
			account.EmployeeID = employee.ID
			if err := c.ensureAccountEmailAvailableForAccount(ctx, email, account.ID); err != nil {
				return false, err
			}
			if err := c.store.UpsertAccount(goContext(ctx), account); err != nil {
				return false, err
			}
			return false, c.Service.syncAccountTenantMembershipTuple(ctx, before, account)
		}
	}
	if existing, ok, err := c.findAccountByEmail(ctx, email); err != nil {
		return false, err
	} else if ok {
		before := existing
		existing.EmployeeID = employee.ID
		existing.DisplayName = utils.FirstNonEmpty(existing.DisplayName, employee.Name)
		existing.Email = email
		if err := c.store.UpsertAccount(goContext(ctx), existing); err != nil {
			return false, err
		}
		if err := c.Service.syncAccountTenantMembershipTuple(ctx, before, existing); err != nil {
			return false, err
		}
		employee.AccountID = existing.ID
		return false, nil
	}
	if err := c.ensureAccountEmailAvailable(ctx, email); err != nil {
		return false, err
	}
	account := Account{
		ID:          utils.NewID("acct"),
		TenantID:    ctx.TenantID,
		DisplayName: employee.Name,
		Email:       email,
		EmployeeID:  employee.ID,
		Status:      string(AccountStatusPendingInvite),
		CreatedAt:   c.Now(),
	}
	if err := c.store.UpsertAccount(goContext(ctx), account); err != nil {
		return false, err
	}
	if err := c.Service.syncAccountTenantMembershipTuple(ctx, Account{}, account); err != nil {
		return false, err
	}
	employee.AccountID = account.ID
	return true, nil
}

func (c HRService) findAccountByEmail(ctx RequestContext, email string) (Account, bool, error) {
	accounts, err := c.store.ListAccounts(goContext(ctx), ctx.TenantID)
	if err != nil {
		return Account{}, false, err
	}
	for _, account := range accounts {
		if strings.EqualFold(strings.TrimSpace(account.Email), email) {
			return account, true, nil
		}
	}
	return Account{}, false, nil
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
func ehrmsBatchErrors(rowNumber int, employee Employee, employeeNos map[string]int, nationalIDs map[string]int, companyEmails map[string]int) []RowError {
	errors := make([]RowError, 0, 3)
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
	if email := strings.ToLower(strings.TrimSpace(employee.CompanyEmail)); email != "" {
		if firstRow, ok := companyEmails[email]; ok {
			errors = append(errors, RowError{Row: rowNumber, Field: "company_email", Code: "duplicate_in_file", Message: fmt.Sprintf("company_email is duplicated with row %d", firstRow)})
		} else {
			companyEmails[email] = rowNumber
		}
	}
	return errors
}

func ehrmsBool(record map[string]string, key string) bool {
	value := strings.TrimSpace(strings.ToLower(ehrmsValue(record, key)))
	switch value {
	case "1", "true", "t", "yes", "y", "v", "是":
		return true
	default:
		return false
	}
}

// EHRMSCleanDepartmentName strips upstream closed markers and reports the resulting closed state.
func EHRMSCleanDepartmentName(name string) (string, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", false
	}
	closed := false
	suffixes := []string{"(已關閉)", "（已關閉）", "(已关闭)", "（已关闭）"}
	for {
		changed := false
		for _, suffix := range suffixes {
			if strings.HasSuffix(trimmed, suffix) {
				trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, suffix))
				closed = true
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return trimmed, closed
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
func ehrmsValue(record map[string]string, key string) string {
	if len(record) == 0 {
		return ""
	}
	return normalizeEHRMSPlaceholder(record[key])
}

// normalizeEHRMSPlaceholder 將上游佔位值視為空值。
func normalizeEHRMSPlaceholder(value string) string {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "", "-", "—", "n/a", "na", "null", "none":
		return ""
	}
	return value
}
