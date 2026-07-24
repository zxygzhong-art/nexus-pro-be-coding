package service

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

const (
	ehrmsFieldEmployeeNo           = "員工編號"
	ehrmsFieldName                 = "中文姓名"
	ehrmsFieldNameEN               = "英文姓名"
	ehrmsFieldFirstName            = "First Name"
	ehrmsFieldLastName             = "Last Name"
	ehrmsFieldGender               = "性別"
	ehrmsFieldBirthDate            = "生日"
	ehrmsFieldHireDate             = "到職日期"
	ehrmsFieldQuitDate             = "離職日期"
	ehrmsFieldTenureStartDate      = "年資起始日"
	ehrmsFieldProbationEnd         = "試用期滿日"
	ehrmsFieldEmployeeStatus       = "在職狀態"
	ehrmsFieldNationality          = "國籍名稱"
	ehrmsFieldNationalID           = "身份證號"
	ehrmsFieldPassportNo           = "護照號碼"
	ehrmsFieldPassportName         = "護照姓名"
	ehrmsFieldEntryDate            = "入境日期"
	ehrmsFieldARCNo                = "居留證號"
	ehrmsFieldARCExpiryDate        = "居留證到期日"
	ehrmsFieldTaxID                = "稅籍編號"
	ehrmsFieldWorkPermitNo         = "工作證號"
	ehrmsFieldWorkPermitExpiryDate = "工作證到期日"
	ehrmsFieldContractExpiryDate   = "契約到期日"
	ehrmsFieldBroker               = "仲介單位"
	ehrmsFieldEmergencyPhone       = "緊急聯絡人電話"
	ehrmsFieldEmergencyContact     = "緊急聯絡人姓名"
	ehrmsFieldEmergencyRelation    = "緊急聯絡人關係"
	ehrmsFieldIdentityType         = "身份類別名稱"
	ehrmsFieldEducation            = "最高學歷"
	ehrmsFieldSchoolName           = "學校名稱(中文)"
	ehrmsFieldDepartmentCode       = "部門代碼"
	ehrmsFieldDepartmentName       = "部門中文名稱"
	ehrmsFieldDepartmentEN         = "部門英文名稱"
	ehrmsFieldPositionCode         = "職務代碼"
	ehrmsFieldPositionName         = "職務中文名稱"
	ehrmsFieldPositionEN           = "職務英文名稱"
	ehrmsFieldCardNo               = "卡號"
	ehrmsFieldClockRequired        = "上下班刷卡"
	ehrmsFieldShiftName            = "員工班別名稱"
	ehrmsFieldShiftType            = "員工班別屬性"
	ehrmsFieldDirectIndirect       = "直接/間接員工"
	ehrmsFieldLeaveGroup           = "休假羣組"
	ehrmsFieldCompanyEmail         = "公司信箱"
	ehrmsFieldParentDeptCode       = "上級部門代碼"
	ehrmsFieldDeptClosed           = "部門已關閉"
	ehrmsFieldManagerJobCode       = "主管職務代碼"
	ehrmsFieldManagerJobName       = "主管職務中文名稱"
	ehrmsFieldManagerJobNameEN     = "主管職務英文名稱"
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
	// Manager-position FKs require the position catalog first; also absorb manager jobs missing from /positions.
	positionRecords, err := c.ehrmsClient.ListPositions(goContext(ctx))
	if err != nil {
		c.logWarn(ctx, "eHRMS position fetch failed during org sync", "error", err)
		return EHRMSOrgUnitSyncResponse{}, ehrmsFetchError("positions", err)
	}
	now := c.Now()
	departments := filterOpenEHRMSOrgUnits(EHRMSOrgUnitsFromDepartments(ctx.TenantID, departmentRecords, now))
	// Manager-position FKs only need jobs referenced by open departments being upserted.
	positions := mergeEHRMSPositionsWithDepartmentManagers(
		EHRMSPositionsFromRecords(ctx.TenantID, positionRecords, now),
		ctx.TenantID,
		filterEHRMSDepartmentRecordsByOrgUnits(departmentRecords, departments),
		now,
	)
	response := EHRMSOrgUnitSyncResponse{Fetched: len(departmentRecords)}
	if err := c.withTransaction(ctx, func(tx HRService) error {
		if _, err := tx.UpsertEHRMSPositions(ctx, positions); err != nil {
			return err
		}
		upserted, err := tx.UpsertEHRMSOrgUnits(ctx, departments)
		if err != nil {
			return err
		}
		response.Upserted = upserted
		if err := tx.resyncEmployeeManagerRelationshipTuples(ctx); err != nil {
			return err
		}
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

// SyncEHRMSEmployees synchronizes tenant-wide employee data only for tenant-wide grants.
// Only employees whose department code already exists in the local org catalog are synced;
// unknown upstream departments are ignored (run SyncEHRMSOrgUnits first to admit them).
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
	localUnits, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return EHRMSEmployeeSyncResponse{}, err
	}
	localDeptCodes := ehrmsOrgUnitCodeSet(localUnits)
	departmentRecords, err := c.ehrmsClient.ListDepartments(goContext(ctx))
	if err != nil {
		c.logWarn(ctx, "eHRMS department fetch failed", "error", err)
		return EHRMSEmployeeSyncResponse{}, ehrmsFetchError("departments", err)
	}
	departmentRecords = filterEHRMSDepartmentRecordsByCodes(departmentRecords, localDeptCodes)
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
	records = filterEHRMSEmployeesByDepartmentCodes(records, localDeptCodes)
	response := EHRMSEmployeeSyncResponse{Fetched: len(records), Mode: mode}
	now := c.Now()
	departments := EHRMSOrgUnitsFromDepartments(ctx.TenantID, departmentRecords, now)
	positions := mergeEHRMSPositionsWithDepartmentManagers(
		EHRMSPositionsFromRecords(ctx.TenantID, positionRecords, now),
		ctx.TenantID,
		departmentRecords,
		now,
	)
	provisionQueued := false
	syncStep := "positions"
	if err := c.withTransaction(ctx, func(tx HRService) error {
		// Positions first so department manager_position_id and employee position_id FKs resolve.
		if _, err := tx.UpsertEHRMSPositions(ctx, positions); err != nil {
			return err
		}
		syncStep = "departments"
		if _, err := tx.UpsertEHRMSOrgUnits(ctx, departments); err != nil {
			return err
		}
		syncStep = "employees"
		writes, rowErrors, skippedResults, err := tx.prepareEHRMSSyncWrites(ctx, account, decision, records, mode)
		if err != nil {
			return err
		}
		created, updated, skipped := 0, 0, 0
		failed := 0
		for _, item := range skippedResults {
			if item.Action == "skipped" {
				skipped++
			} else {
				failed++
			}
		}
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
		response.Skipped = skipped
		response.Failed = failed
		response.RowErrors = rowErrors
		response.DepartmentsUpserted = len(departments)
		response.PositionsUpserted = len(positions)
		response.Results = results
		syncStep = "finalize"
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeImported), "ehrms", map[string]any{
			"source":               "ehrms",
			"fetched":              response.Fetched,
			"created":              created,
			"updated":              updated,
			"skipped":              skipped,
			"failed":               failed,
			"departments_upserted": len(departments),
			"positions_upserted":   len(positions),
			"mode":                 mode,
		}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.ehrms.sync", string(ResourceEmployeeSync), "ehrms", string(SeverityHigh), map[string]any{
			"source":               "ehrms",
			"fetched":              response.Fetched,
			"created":              created,
			"updated":              updated,
			"skipped":              skipped,
			"failed":               failed,
			"departments_upserted": len(departments),
			"positions_upserted":   len(positions),
			"mode":                 mode,
		}); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		c.logEHRMSSyncSummary(ctx,
			0, len(positions),
			0, len(departments),
			0, len(records),
			syncStep, err,
		)
		return EHRMSEmployeeSyncResponse{}, err
	}
	if provisionQueued {
		c.runIdentityProvisioningFastPath(ctx)
	}
	employeeSucceeded := response.Created + response.Updated
	c.logEHRMSSyncSummary(ctx,
		response.PositionsUpserted, len(positions)-response.PositionsUpserted,
		response.DepartmentsUpserted, len(departments)-response.DepartmentsUpserted,
		employeeSucceeded, response.Fetched-employeeSucceeded,
		"", nil,
	)
	return response, nil
}

func (c HRService) logEHRMSSyncSummary(
	ctx RequestContext,
	positionsSucceeded, positionsFailed int,
	departmentsSucceeded, departmentsFailed int,
	employeesSucceeded, employeesFailed int,
	failedStep string,
	syncErr error,
) {
	args := []any{
		"positions_success", positionsSucceeded,
		"positions_failed", positionsFailed,
		"departments_success", departmentsSucceeded,
		"departments_failed", departmentsFailed,
		"employees_success", employeesSucceeded,
		"employees_failed", employeesFailed,
	}
	if failedStep != "" {
		args = append(args, "failed_step", failedStep)
	}
	if syncErr != nil {
		args = append(args, "error", syncErr)
	}
	if positionsFailed+departmentsFailed+employeesFailed > 0 || syncErr != nil {
		c.logWarn(ctx, "eHRMS sync summary", args...)
		return
	}
	c.logInfo(ctx, "eHRMS sync summary", args...)
}

// prepareEHRMSSyncWrites resolves tenant-local catalog identities once before mapping employee rows.
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
		employee, errors, err := c.ehrmsEmployeeCandidate(ctx, record, rowNumber, lookup)
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
		case employeeSyncModeCreate:
			if ok {
				errors = append(errors, RowError{Row: rowNumber, Field: "employee_no", Code: "unique", Message: "employee_no already exists"})
			}
		case employeeSyncModeUpdate:
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
			scopeErrors, err := c.employeeSyncScopeErrors(ctx, account, rowNumber, employee, existing, update, decision)
			if err != nil {
				return nil, nil, nil, err
			}
			errors = append(errors, scopeErrors...)
		}
		if len(errors) > 0 {
			rowErrors = append(rowErrors, errors...)
			action := "failed"
			if errors[0].Field == "position_code" && (errors[0].Code == "not_found" || errors[0].Code == "required") {
				action = "skipped"
			}
			results = append(results, BatchEmployeeResult{
				RowNumber: rowNumber,
				Success:   false,
				Action:    action,
				Code:      errors[0].Code,
				Message:   firstEmployeeRowErrorMessage(errors),
			})
			continue
		}
		if update {
			employee = c.appendHistoryForChangedEmployment(existing, employee, "eHRMS sync")
		}
		if len(employee.InternalExperiences) == 0 {
			employee.InternalExperiences = append(employee.InternalExperiences, c.newEmployeeExperience(employee, "eHRMS sync"))
		}
		writes = append(writes, ehrmsEmployeeWrite{rowNumber: rowNumber, employee: employee, previous: existing, update: update})
	}
	return writes, rowErrors, results, nil
}

// ehrmsEmployeeCandidate maps upstream business codes to tenant-scoped internal references.
// Employees are only accepted when 職務代碼 already exists in the local positions catalog.
func (c HRService) ehrmsEmployeeCandidate(ctx RequestContext, record EHRMSEmployeeRecord, rowNumber int, lookup ehrmsValidationLookup) (Employee, []RowError, error) {
	status := normalizeEmployeeStatus(ehrmsValue(record, ehrmsFieldEmployeeStatus))
	departmentCode := ehrmsValue(record, ehrmsFieldDepartmentCode)
	positionCode := ehrmsValue(record, ehrmsFieldPositionCode)
	if positionCode == "" {
		return Employee{}, []RowError{{
			Row: rowNumber, Field: "position_code", Code: "required",
			Message: "position_code is required and must exist in local positions catalog",
		}}, nil
	}
	positionID, ok := lookup.positionIDsByCode[ehrmsExternalCodeKey(positionCode)]
	if !ok {
		return Employee{}, []RowError{{
			Row: rowNumber, Field: "position_code", Code: "not_found",
			Message: "position_code does not exist in local positions catalog",
		}}, nil
	}
	orgUnitID := utils.FirstNonEmpty(lookup.orgUnitIDsByCode[ehrmsExternalCodeKey(departmentCode)], ehrmsOrgUnitID(ctx.TenantID, departmentCode))
	companyEmail := strings.ToLower(strings.TrimSpace(ehrmsValue(record, ehrmsFieldCompanyEmail)))
	hireDate := normalizeEmployeeDate(utils.FirstNonEmpty(
		ehrmsValue(record, ehrmsFieldHireDate),
		ehrmsValue(record, ehrmsFieldTenureStartDate),
	))
	resignDate := normalizeEmployeeDate(ehrmsValue(record, ehrmsFieldQuitDate))
	tenureStartDate := normalizeEmployeeDate(ehrmsValue(record, ehrmsFieldTenureStartDate))
	probationEndDate := normalizeEmployeeDate(ehrmsValue(record, ehrmsFieldProbationEnd))
	input := CreateEmployeeInput{
		EmployeeNo:       ehrmsValue(record, ehrmsFieldEmployeeNo),
		Name:             ehrmsValue(record, ehrmsFieldName),
		CompanyEmail:     companyEmail,
		OrgUnitID:        orgUnitID,
		PositionID:       positionID,
		Position:         utils.FirstNonEmpty(ehrmsValue(record, ehrmsFieldPositionName), positionCode),
		Category:         ehrmsEmployeeCategory(record),
		Status:           status,
		EmploymentStatus: status,
		HireDate:         hireDate,
		ResignDate:       resignDate,
		BasicInfo: map[string]any{
			domain.EmployeeBasicInfoKeyName:            ehrmsValue(record, ehrmsFieldName),
			domain.EmployeeBasicInfoKeyNameEN:          ehrmsValue(record, ehrmsFieldNameEN),
			"first_name":                               ehrmsValue(record, ehrmsFieldFirstName),
			"last_name":                                ehrmsValue(record, ehrmsFieldLastName),
			domain.EmployeeBasicInfoKeyGender:          ehrmsValue(record, ehrmsFieldGender),
			domain.EmployeeBasicInfoKeyBirthDate:       normalizeEmployeeDate(ehrmsValue(record, ehrmsFieldBirthDate)),
			domain.EmployeeBasicInfoKeyNationalityType: ehrmsValue(record, ehrmsFieldNationality),
			// 保留 legacy 鍵：前端仍直接讀取原始國籍名稱。
			domain.EmployeeBasicInfoKeyNationality:          ehrmsValue(record, ehrmsFieldNationality),
			domain.EmployeeBasicInfoKeyNationalID:           ehrmsValue(record, ehrmsFieldNationalID),
			domain.EmployeeBasicInfoKeyPassportNo:           ehrmsValue(record, ehrmsFieldPassportNo),
			domain.EmployeeBasicInfoKeyPassportName:         ehrmsValue(record, ehrmsFieldPassportName),
			domain.EmployeeBasicInfoKeyEntryDate:            normalizeEmployeeDate(ehrmsValue(record, ehrmsFieldEntryDate)),
			domain.EmployeeBasicInfoKeyARCNo:                ehrmsValue(record, ehrmsFieldARCNo),
			domain.EmployeeBasicInfoKeyARCExpiryDate:        normalizeEmployeeDate(ehrmsValue(record, ehrmsFieldARCExpiryDate)),
			domain.EmployeeBasicInfoKeyTaxID:                ehrmsValue(record, ehrmsFieldTaxID),
			domain.EmployeeBasicInfoKeyWorkPermitNo:         ehrmsValue(record, ehrmsFieldWorkPermitNo),
			domain.EmployeeBasicInfoKeyWorkPermitExpiryDate: normalizeEmployeeDate(ehrmsValue(record, ehrmsFieldWorkPermitExpiryDate)),
			domain.EmployeeBasicInfoKeyContractExpiryDate:   normalizeEmployeeDate(ehrmsValue(record, ehrmsFieldContractExpiryDate)),
			domain.EmployeeBasicInfoKeyBroker:               ehrmsValue(record, ehrmsFieldBroker),
			"identity_type_name":                            ehrmsValue(record, ehrmsFieldIdentityType),
			domain.EmployeeBasicInfoKeyCompanyEmail:         companyEmail,
			"source":                                        "ehrms",
		},
		EmploymentInfo: map[string]any{
			domain.EmployeeEmploymentInfoKeyOrgUnitID: orgUnitID,
			"org_unit_code": departmentCode,
			domain.EmployeeEmploymentInfoKeyOrgUnitName: ehrmsValue(record, ehrmsFieldDepartmentName),
			"org_unit_name_en":                          ehrmsValue(record, ehrmsFieldDepartmentEN),
			"position":                                  utils.FirstNonEmpty(ehrmsValue(record, ehrmsFieldPositionName), ehrmsValue(record, ehrmsFieldPositionCode)),
			"position_id":                               positionID,
			"position_code":                             ehrmsValue(record, ehrmsFieldPositionCode),
			"position_name_en":                          ehrmsValue(record, ehrmsFieldPositionEN),
			"category":                                  ehrmsEmployeeCategory(record),
			"employment_status":                         status,
			"hire_date":                                 hireDate,
			"resign_date":                               resignDate,
			"tenure_start_date":                         tenureStartDate,
			"probation_end_date":                        probationEndDate,
			"card_no":                                   ehrmsValue(record, ehrmsFieldCardNo),
			"clock_required":                            ehrmsValue(record, ehrmsFieldClockRequired),
			domain.EmployeeEmploymentInfoKeyShift:       ehrmsValue(record, ehrmsFieldShiftName),
			domain.EmployeeEmploymentInfoKeyShiftType: ehrmsValue(record, ehrmsFieldShiftType),
			"direct_indirect_employee":                ehrmsValue(record, ehrmsFieldDirectIndirect),
			"leave_group":                             ehrmsValue(record, ehrmsFieldLeaveGroup),
			"source":                                  "ehrms",
		},
		EducationMilitaryInfo: map[string]any{
			"highest_education": ehrmsValue(record, ehrmsFieldEducation),
			"school_name":       ehrmsValue(record, ehrmsFieldSchoolName),
		},
		ContactInfo: map[string]any{
			"emergency_contact_phone":    ehrmsValue(record, ehrmsFieldEmergencyPhone),
			"emergency_contact_name":     ehrmsValue(record, ehrmsFieldEmergencyContact),
			"emergency_contact_relation": ehrmsValue(record, ehrmsFieldEmergencyRelation),
		},
	}
	employee, err := c.employeeCreateCandidate(ctx, input)
	if err != nil {
		errors, ok := employeeRowErrorsFromError(rowNumber, err)
		if ok {
			return Employee{}, errors, nil
		}
		return Employee{}, nil, err
	}
	// Persist a globally unique internal ID. The upstream employee number stays
	// tenant-scoped in external_employee_id and employee_no.
	if employeeNo := strings.TrimSpace(employee.EmployeeNo); employeeNo != "" {
		now := c.Now()
		employee.ID = ehrmsEmployeeStableID(ctx.TenantID, employeeNo)
		employee.ExternalSource = ehrmsAttendanceSource
		employee.ExternalEmployeeID = employeeNo
		employee.SourcePayload = ehrmsStringRecordPayload(record)
		employee.SourceUpdatedAt = ehrmsRecordUpdatedAt(record)
		employee.LastSyncedAt = &now
	}
	if err := c.ensureEmployeePosition(ctx, &employee, positionCode == ""); err != nil {
		errors, ok := employeeRowErrorsFromError(rowNumber, err)
		if ok {
			return Employee{}, errors, nil
		}
		return Employee{}, nil, err
	}
	return employee, nil, nil
}

func ehrmsEmployeeStableID(tenantID, employeeNo string) string {
	return ehrmsStableID(
		"emp",
		strings.TrimSpace(tenantID),
		strings.ToLower(strings.TrimSpace(employeeNo)),
	)
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
	positionCode := strings.TrimSpace(stringFromMap(employee.EmploymentInfo, "position_code"))
	if positionCode == "" || lookup.positionIDsByCode[ehrmsExternalCodeKey(positionCode)] == "" {
		fields = append(fields, FieldError{
			Tab: "employment_info", Field: "position_code", Code: "not_found",
			Message: "position_code does not exist in local positions catalog",
		})
	} else if positionID := strings.TrimSpace(employee.PositionID); positionID != "" {
		if expected := lookup.positionIDsByCode[ehrmsExternalCodeKey(positionCode)]; expected != positionID {
			fields = append(fields, FieldError{
				Tab: "employment_info", Field: "position_id", Code: "invalid",
				Message: "position_id does not match local positions catalog",
			})
		}
	}
	if len(fields) == 0 {
		fields = append(fields, lookup.unique.fieldErrors(employee)...)
	}
	return fieldErrorsToRowErrors(rowNumber, fields), nil
}

type ehrmsValidationLookup struct {
	orgUnitIDs        map[string]struct{}
	orgUnitIDsByCode  map[string]string
	positionIDsByCode map[string]string
	unique            employeeUniqueIndex
}

// ehrmsValidationLookup builds validation and business-code reference indexes for one tenant.
func (c HRService) ehrmsValidationLookup(ctx RequestContext) (ehrmsValidationLookup, error) {
	employees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
	if err != nil {
		return ehrmsValidationLookup{}, err
	}
	units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return ehrmsValidationLookup{}, err
	}
	positions, err := c.store.ListPositions(goContext(ctx), ctx.TenantID)
	if err != nil {
		return ehrmsValidationLookup{}, err
	}
	orgUnitIDs := make(map[string]struct{}, len(units))
	orgUnitIDsByCode := make(map[string]string, len(units))
	for _, unit := range units {
		orgUnitIDs[unit.ID] = struct{}{}
		key := ehrmsExternalCodeKey(unit.Code)
		if key != "" {
			orgUnitIDsByCode[key] = unit.ID
		}
	}
	positionIDsByCode := make(map[string]string, len(positions))
	for _, position := range positions {
		key := ehrmsExternalCodeKey(position.Code)
		if key != "" {
			positionIDsByCode[key] = position.ID
		}
	}
	return ehrmsValidationLookup{
		orgUnitIDs:        orgUnitIDs,
		orgUnitIDsByCode:  orgUnitIDsByCode,
		positionIDsByCode: positionIDsByCode,
		unique:            newEmployeeUniqueIndex(employees),
	}, nil
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
	departments, err := c.reconcileEHRMSOrgUnitIDs(ctx, departments)
	if err != nil {
		return 0, err
	}
	for _, unit := range departments {
		before, ok, err := c.store.GetOrgUnit(goContext(ctx), ctx.TenantID, unit.ID)
		if err != nil {
			return 0, err
		}
		if ok {
			unit.CreatedAt = before.CreatedAt
			unit.ShowInOrgChart = before.ShowInOrgChart
		} else {
			unit.ShowInOrgChart = true
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

// reconcileEHRMSOrgUnitIDs preserves same-tenant legacy IDs while remapping incoming hierarchy references.
func (c HRService) reconcileEHRMSOrgUnitIDs(ctx RequestContext, departments []OrgUnit) ([]OrgUnit, error) {
	existing, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	existingByCode := make(map[string]OrgUnit, len(existing))
	for _, unit := range existing {
		key := ehrmsExternalCodeKey(unit.Code)
		if key == "" {
			continue
		}
		current, ok := existingByCode[key]
		if !ok || (current.Source != "ehrms" && unit.Source == "ehrms") {
			existingByCode[key] = unit
		}
	}
	replacements := make(map[string]string, len(departments))
	for _, unit := range departments {
		if previous, ok := existingByCode[ehrmsExternalCodeKey(unit.Code)]; ok {
			replacements[unit.ID] = previous.ID
		}
	}
	out := make([]OrgUnit, 0, len(departments))
	for _, unit := range departments {
		if replacement := replacements[unit.ID]; replacement != "" {
			unit.ID = replacement
		}
		if replacement := replacements[unit.ParentID]; replacement != "" {
			unit.ParentID = replacement
		}
		for index, pathID := range unit.Path {
			if replacement := replacements[pathID]; replacement != "" {
				unit.Path[index] = replacement
			}
		}
		out = append(out, unit)
	}
	return out, nil
}

// UpsertEHRMSPositions persists normalized upstream positions by stable tenant-local code.
func (c HRService) UpsertEHRMSPositions(ctx RequestContext, positions []Position) (int, error) {
	for _, position := range positions {
		before, ok, err := c.store.GetPositionByCode(goContext(ctx), ctx.TenantID, position.Code)
		if err != nil {
			return 0, err
		}
		if ok {
			position.ID = before.ID
			position.CreatedAt = before.CreatedAt
		}
		if err := c.store.UpsertPosition(goContext(ctx), position); err != nil {
			return 0, err
		}
	}
	return len(positions), nil
}

// filterOpenEHRMSOrgUnits keeps only departments that are not closed (including parent-propagated closed).
func filterOpenEHRMSOrgUnits(units []OrgUnit) []OrgUnit {
	if len(units) == 0 {
		return units
	}
	out := make([]OrgUnit, 0, len(units))
	for _, unit := range units {
		if unit.Closed {
			continue
		}
		out = append(out, unit)
	}
	return out
}

// ehrmsOrgUnitCodeSet indexes local org-unit business codes for eHRMS sync filters.
func ehrmsOrgUnitCodeSet(units []OrgUnit) map[string]struct{} {
	out := make(map[string]struct{}, len(units))
	for _, unit := range units {
		if key := ehrmsExternalCodeKey(unit.Code); key != "" {
			out[key] = struct{}{}
		}
	}
	return out
}

// filterEHRMSDepartmentRecordsByCodes keeps upstream department rows whose codes are in the allow-list.
func filterEHRMSDepartmentRecordsByCodes(records []EHRMSDepartmentRecord, codes map[string]struct{}) []EHRMSDepartmentRecord {
	if len(records) == 0 || len(codes) == 0 {
		return nil
	}
	out := make([]EHRMSDepartmentRecord, 0, len(codes))
	for _, record := range records {
		code := ehrmsExternalCodeKey(ehrmsValue(record, ehrmsFieldDepartmentCode))
		if _, ok := codes[code]; !ok {
			continue
		}
		out = append(out, record)
	}
	return out
}

// filterEHRMSDepartmentRecordsByOrgUnits keeps upstream department rows whose codes match the given org units.
func filterEHRMSDepartmentRecordsByOrgUnits(records []EHRMSDepartmentRecord, units []OrgUnit) []EHRMSDepartmentRecord {
	return filterEHRMSDepartmentRecordsByCodes(records, ehrmsOrgUnitCodeSet(units))
}

// filterEHRMSEmployeesByDepartmentCodes keeps upstream employees assigned to allow-listed department codes.
func filterEHRMSEmployeesByDepartmentCodes(records []EHRMSEmployeeRecord, codes map[string]struct{}) []EHRMSEmployeeRecord {
	if len(records) == 0 || len(codes) == 0 {
		return nil
	}
	out := make([]EHRMSEmployeeRecord, 0, len(records))
	for _, record := range records {
		code := ehrmsExternalCodeKey(ehrmsValue(record, ehrmsFieldDepartmentCode))
		if _, ok := codes[code]; !ok {
			continue
		}
		out = append(out, record)
	}
	return out
}

// EHRMSOrgUnitsFromDepartments maps upstream department records into the canonical organization hierarchy.
// manager_position_id uses manager_job_code; empty or same-as-parent job codes stay empty so runtime inherits.
func EHRMSOrgUnitsFromDepartments(tenantID string, records []EHRMSDepartmentRecord, now time.Time) []OrgUnit {
	unitsByCode := make(map[string]OrgUnit, len(records))
	parentCodes := make(map[string]string, len(records))
	managerJobByCode := make(map[string]string, len(records))
	for _, record := range records {
		code := ehrmsValue(record, ehrmsFieldDepartmentCode)
		if code == "" {
			continue
		}
		codeKey := ehrmsExternalCodeKey(code)
		rawName := utils.FirstNonEmpty(ehrmsValue(record, ehrmsFieldDepartmentName), ehrmsValue(record, ehrmsFieldDepartmentEN), code)
		rawNameEN := ehrmsValue(record, ehrmsFieldDepartmentEN)
		name, nameClosed := EHRMSCleanDepartmentName(rawName)
		nameEN, nameENClosed := EHRMSCleanDepartmentName(rawNameEN)
		closed := ehrmsBool(record, ehrmsFieldDeptClosed) || nameClosed || nameENClosed
		if name == "" {
			name = code
		}
		unitsByCode[codeKey] = OrgUnit{
			ID:             ehrmsOrgUnitID(tenantID, code),
			TenantID:       tenantID,
			Code:           code,
			Name:           name,
			NameEN:         nameEN,
			Closed:         closed,
			ShowInOrgChart: true,
			CreatedAt:      now,
			UpdatedAt:      now,
			Source:         "ehrms",
		}
		parentCodes[codeKey] = ehrmsExternalCodeKey(ehrmsValue(record, ehrmsFieldParentDeptCode))
		managerJobByCode[codeKey] = strings.TrimSpace(utils.FirstNonEmpty(
			ehrmsValue(record, ehrmsFieldManagerJobCode),
			ehrmsValue(record, "manager_job_code"),
		))
	}
	unitsByID := make(map[string]OrgUnit, len(unitsByCode))
	for codeKey, unit := range unitsByCode {
		if parent, ok := unitsByCode[parentCodes[codeKey]]; ok {
			unit.ParentID = parent.ID
		}
		unit.ManagerPositionID = ehrmsCollapsedManagerPositionID(
			tenantID, managerJobByCode[codeKey], managerJobByCode[parentCodes[codeKey]],
		)
		unitsByID[unit.ID] = unit
	}
	for id, unit := range unitsByID {
		unit.Path = ehrmsOrgUnitPath(id, unitsByID)
		unitsByID[id] = unit
	}
	for _, unit := range ehrmsSortedOrgUnits(unitsByID) {
		if parent, ok := unitsByID[unit.ParentID]; ok && parent.Closed {
			unit.Closed = true
			unitsByID[unit.ID] = unit
		}
	}
	return ehrmsSortedOrgUnits(unitsByID)
}

// ehrmsCollapsedManagerPositionID keeps only distinct manager jobs; same-as-parent or empty inherit at runtime.
func ehrmsCollapsedManagerPositionID(tenantID, ownJobCode, parentJobCode string) string {
	ownJobCode = strings.TrimSpace(ownJobCode)
	if ownJobCode == "" {
		return ""
	}
	if ehrmsExternalCodeKey(ownJobCode) == ehrmsExternalCodeKey(parentJobCode) {
		return ""
	}
	return ehrmsPositionID(tenantID, ownJobCode)
}

// mergeEHRMSPositionsWithDepartmentManagers ensures manager job codes referenced by departments exist locally.
func mergeEHRMSPositionsWithDepartmentManagers(positions []Position, tenantID string, departments []EHRMSDepartmentRecord, now time.Time) []Position {
	byCode := make(map[string]Position, len(positions))
	for _, position := range positions {
		byCode[ehrmsExternalCodeKey(position.Code)] = position
	}
	for _, record := range departments {
		code := strings.TrimSpace(utils.FirstNonEmpty(
			ehrmsValue(record, ehrmsFieldManagerJobCode),
			ehrmsValue(record, "manager_job_code"),
		))
		if code == "" {
			continue
		}
		codeKey := ehrmsExternalCodeKey(code)
		if existing, ok := byCode[codeKey]; ok && strings.TrimSpace(existing.Name) != "" {
			continue
		}
		name := utils.FirstNonEmpty(
			ehrmsValue(record, ehrmsFieldManagerJobName),
			ehrmsValue(record, "manager_job_title"),
			ehrmsValue(record, ehrmsFieldManagerJobNameEN),
			ehrmsValue(record, "manager_job_title_en"),
			code,
		)
		byCode[codeKey] = Position{
			ID:       ehrmsPositionID(tenantID, code),
			TenantID: tenantID,
			Code:     code,
			Name:     name,
			NameEN: utils.FirstNonEmpty(
				ehrmsValue(record, ehrmsFieldManagerJobNameEN),
				ehrmsValue(record, "manager_job_title_en"),
			),
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
	out := make([]Position, 0, len(ids))
	for _, id := range ids {
		out = append(out, byCode[id])
	}
	return out
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
		codeKey := ehrmsExternalCodeKey(code)
		if existing, ok := byCode[codeKey]; ok && strings.TrimSpace(existing.Name) != "" {
			continue
		}
		byCode[codeKey] = Position{
			ID:        ehrmsPositionID(tenantID, code),
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

// ehrmsExternalCodeKey normalizes external catalog codes for deterministic identity mapping.
func ehrmsExternalCodeKey(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

// ehrmsOrgUnitID keeps upstream department codes tenant-local while producing globally safe IDs.
func ehrmsOrgUnitID(tenantID, code string) string {
	code = ehrmsExternalCodeKey(code)
	if code == "" {
		return ""
	}
	return ehrmsStableID("ehrms-ou", strings.TrimSpace(tenantID), code)
}

// ehrmsPositionID keeps upstream position codes tenant-local while producing globally safe IDs.
func ehrmsPositionID(tenantID, code string) string {
	code = ehrmsExternalCodeKey(code)
	if code == "" {
		return ""
	}
	return ehrmsStableID("ehrms-pos", strings.TrimSpace(tenantID), code)
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
	case "", employeeSyncModeUpsert:
		return employeeSyncModeUpsert, nil
	case employeeSyncModeCreate:
		return employeeSyncModeCreate, nil
	case employeeSyncModeUpdate:
		return employeeSyncModeUpdate, nil
	default:
		return "", BadRequest("eHRMS sync mode must be create, update, or upsert")
	}
}

// EHRMSMergeEmployee merges upstream employee data without overwriting self-managed profile fields.
func EHRMSMergeEmployee(existing Employee, candidate Employee) Employee {
	next := existing
	selfNameEN := next.BasicInfo[domain.EmployeeBasicInfoKeyNameEN]
	nameENSource := stringFromAny(next.BasicInfo["name_en_source"])
	next.EmployeeNo = candidate.EmployeeNo
	next.ExternalSource = candidate.ExternalSource
	next.ExternalEmployeeID = candidate.ExternalEmployeeID
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
	next.BasicInfo = mergeEmployeeMaps(next.BasicInfo, candidate.BasicInfo)
	if nameENSource == "self" {
		next.BasicInfo[domain.EmployeeBasicInfoKeyNameEN] = selfNameEN
		next.BasicInfo["name_en_source"] = "self"
	}
	next.EmploymentInfo = mergeEmployeeMaps(next.EmploymentInfo, candidate.EmploymentInfo)
	next.EducationMilitaryInfo = mergeEmployeeMaps(next.EducationMilitaryInfo, candidate.EducationMilitaryInfo)
	next.ContactInfo = mergeEmployeeMaps(next.ContactInfo, candidate.ContactInfo)
	next.SourcePayload = candidate.SourcePayload
	if candidate.SourceUpdatedAt != nil {
		next.SourceUpdatedAt = candidate.SourceUpdatedAt
	}
	next.LastSyncedAt = candidate.LastSyncedAt
	if next.BasicInfo == nil {
		next.BasicInfo = map[string]any{}
	}
	// EHRMS email 以 upstream 為準（含空值覆蓋），不受 merge 跳過空字串影響。
	next.BasicInfo[domain.EmployeeBasicInfoKeyCompanyEmail] = candidate.CompanyEmail
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
	suffixes := []string{"(已關閉)", "（已關閉）", "(已關閉)", "（已關閉）"}
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

func ehrmsStringRecordPayload(record map[string]string) map[string]any {
	if len(record) == 0 {
		return nil
	}
	payload := make(map[string]any, len(record))
	for key, value := range record {
		payload[key] = value
	}
	return payload
}

func ehrmsRecordUpdatedAt(record map[string]string) *time.Time {
	for _, key := range []string{
		"source_updated_at", "updated_at", "update_time", "modified_at",
		"異動時間", "更新时间", "更新時間",
	} {
		value := normalizeEHRMSPlaceholder(record[key])
		if value == "" {
			continue
		}
		for _, layout := range []string{
			time.RFC3339Nano, time.RFC3339,
			"2006-01-02 15:04:05", "2006/01/02 15:04:05",
			time.DateOnly,
		} {
			if parsed, err := time.ParseInLocation(layout, value, attendanceClockLocation); err == nil {
				return &parsed
			}
		}
	}
	return nil
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

var ehrmsNumberPattern = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)

const (
	leaveTypeUnknownCode             = "unknown_leave_type"
	ehrmsAttendanceFetchConcurrency  = 10
	ehrmsAttendanceProgressLogEvery  = 25
	ehrmsPersistenceProgressLogEvery = 500

	ehrmsAttendanceFieldEmployeeNo    = "員工編號"
	ehrmsAttendanceFieldDate          = "日期"
	ehrmsAttendanceFieldShiftStart    = "班別開始"
	ehrmsAttendanceFieldShiftEnd      = "班別結束"
	ehrmsAttendanceFieldShiftHours    = "班別工時"
	ehrmsAttendanceFieldDailyHours    = "應出勤工時"
	ehrmsAttendanceFieldClockHours    = "刷卡工時"
	ehrmsAttendanceFieldClockStart    = "clock_start"
	ehrmsAttendanceFieldClockEnd      = "clock_end"
	ehrmsAttendanceFieldLeaveType     = "leave_type"
	ehrmsAttendanceFieldLeaveStart    = "leave_start"
	ehrmsAttendanceFieldLeaveEnd      = "leave_end"
	ehrmsAttendanceFieldLeaveHours    = "leave_hours"
	ehrmsAttendanceFieldLeaveCounted  = "leave_counted"
	ehrmsAttendanceFieldLeave2Type    = "leave2_type"
	ehrmsAttendanceFieldLeave2Start   = "leave2_start"
	ehrmsAttendanceFieldLeave2End     = "leave2_end"
	ehrmsAttendanceFieldLeave2Hours   = "leave2_hours"
	ehrmsAttendanceFieldLeave2Counted = "leave2_counted"
	ehrmsAttendanceSource             = "ehrms"

	ehrmsLeaveBalanceFieldEmployeeNo   = "員工編號"
	ehrmsLeaveBalanceFieldYear         = "年度"
	ehrmsLeaveBalanceFieldLeaveType    = "假別"
	ehrmsLeaveBalanceFieldUnit         = "單位"
	ehrmsLeaveBalanceFieldQuota        = "額度"
	ehrmsLeaveBalanceFieldUsed         = "已使用"
	ehrmsLeaveBalanceFieldRemaining    = "餘額"
	ehrmsLeaveBalanceFieldLeaveCode    = "假別代碼"
	ehrmsLeaveBalanceFieldCategoryCode = "假別類別代碼"

	ehrmsLeaveDetailFieldEmployeeNo   = "員工編號"
	ehrmsLeaveDetailFieldDate         = "日期"
	ehrmsLeaveDetailFieldLeaveType    = "假別"
	ehrmsLeaveDetailFieldStart        = "開始時間"
	ehrmsLeaveDetailFieldEnd          = "結束時間"
	ehrmsLeaveDetailFieldHours        = "時數"
	ehrmsLeaveDetailFieldLeaveCode    = "假別代碼"
	ehrmsLeaveDetailFieldCategoryCode = "假別類別代碼"
	ehrmsLeaveDetailFieldLeaveItem    = "假勤項目"
	ehrmsLeaveDetailFieldRemark       = "備註"
	ehrmsLeaveDetailFieldSource       = "資料來源"
	ehrmsLeaveDetailFieldDeductItem   = "扣除項目"
	ehrmsLeaveDetailFieldDeductHours  = "扣除時間"

	ehrmsLeaveTypeFieldCode     = "假別代碼"
	ehrmsLeaveTypeFieldKind     = "節點類型"
	ehrmsLeaveTypeFieldParent   = "上級假別代碼"
	ehrmsLeaveTypeFieldName     = "假別名稱"
	ehrmsLeaveTypeFieldNameEN   = "英文名稱"
	ehrmsLeaveTypeFieldMaxValue = "最大值"
	ehrmsLeaveTypeFieldUnit     = "單位"
	ehrmsLeaveTypeFieldCategory = "假別類別"
)

var ehrmsAttendanceOnlyLeaveTypes = map[string]struct{}{
	"absenteeism":      {},
	"attendance hours": {},
	"holiday overtime": {},
	"missing punch":    {},
	"overtime":         {},
	"例假日加班":            {},
	"出勤時數":             {},
	"加班":               {},
	"忘刷忘帶卡":            {},
}

// SyncEHRMSLeaveTypes synchronizes only the tenant leave catalog from EHRMS.
func (c AttendanceService) SyncEHRMSLeaveTypes(ctx RequestContext) (EHRMSLeaveTypeSyncResponse, error) {
	if c.ehrmsClient == nil {
		return EHRMSLeaveTypeSyncResponse{}, BadRequest("eHRMS is not configured")
	}
	_, decision, authzAudit, err := c.Service.Authorize(ctx,
		CheckRequest{ApplicationCode: AppAttendance, ResourceType: ResourceAttendanceClock, Action: ActionImport},
		AuditTarget{Event: "attendance.leave_type.ehrms.sync", Resource: string(ResourceLeave)},
	)
	if err != nil {
		return EHRMSLeaveTypeSyncResponse{}, err
	}
	if err := requireTenantWideEHRMSSyncScope(decision); err != nil {
		return EHRMSLeaveTypeSyncResponse{}, err
	}
	rows, err := c.ehrmsClient.ListLeaveTypes(goContext(ctx))
	if err != nil {
		c.logWarn(ctx, "eHRMS leave types fetch failed", "error", err)
		return EHRMSLeaveTypeSyncResponse{}, ehrmsFetchError("leave types", err)
	}
	response := EHRMSLeaveTypeSyncResponse{Fetched: len(rows)}
	if err := c.withTransaction(ctx, func(tx AttendanceService) error {
		upserted, deactivated, syncErr := tx.syncEHRMSLeaveTypes(ctx, rows)
		if syncErr != nil {
			return syncErr
		}
		response.Upserted = upserted
		response.Deactivated = deactivated
		if err := tx.audit(ctx, "attendance.leave_type.ehrms.sync", string(ResourceLeave), "ehrms", string(SeverityHigh), map[string]any{
			"source": "ehrms", "fetched": response.Fetched, "upserted": upserted, "deactivated": deactivated,
		}); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		return EHRMSLeaveTypeSyncResponse{}, err
	}
	c.logInfo(ctx, "eHRMS leave type sync completed", "fetched", response.Fetched, "upserted", response.Upserted, "deactivated", response.Deactivated)
	return response, nil
}

// SyncEHRMSAttendance synchronizes tenant-wide attendance, leave balances, and
// leave details for one bounded window. Manual calls also refresh leave types;
// scheduled attendance-only calls keep the daily catalog cadence independent.
func (c AttendanceService) SyncEHRMSAttendance(ctx RequestContext, input EHRMSAttendanceSyncInput) (EHRMSAttendanceSyncResponse, error) {
	syncStartedAt := time.Now()
	if c.ehrmsClient == nil {
		return EHRMSAttendanceSyncResponse{}, BadRequest("eHRMS is not configured")
	}
	mode, err := normalizeEHRMSSyncMode(input.Mode)
	if err != nil {
		return EHRMSAttendanceSyncResponse{}, err
	}
	now := c.Now()
	syncStart, syncEnd, syncYear, err := normalizeEHRMSAttendanceSyncBounds(input, now)
	if err != nil {
		return EHRMSAttendanceSyncResponse{}, err
	}
	_, decision, authzAudit, err := c.Service.Authorize(ctx,
		CheckRequest{ApplicationCode: AppAttendance, ResourceType: ResourceAttendanceClock, Action: ActionImport},
		AuditTarget{Event: "attendance.ehrms.sync", Resource: string(ResourceAttendanceClock)},
	)
	if err != nil {
		return EHRMSAttendanceSyncResponse{}, err
	}
	if err := requireTenantWideEHRMSSyncScope(decision); err != nil {
		return EHRMSAttendanceSyncResponse{}, err
	}
	c.logInfo(ctx, "eHRMS attendance and leave sync started",
		"mode", mode,
		"start", syncStart,
		"end", syncEnd,
		"include_leave_types", !input.SkipLeaveTypes,
		"max_concurrency", ehrmsAttendanceFetchConcurrency,
	)
	var leaveTypeRows []domain.EHRMSLeaveTypeRecord
	if !input.SkipLeaveTypes {
		leaveTypeRows, err = c.ehrmsClient.ListLeaveTypes(goContext(ctx))
		if err != nil {
			c.logWarn(ctx, "eHRMS leave types fetch failed", "error", err)
			return EHRMSAttendanceSyncResponse{}, ehrmsFetchError("leave types", err)
		}
		c.logInfo(ctx, "eHRMS leave types fetched", "records", len(leaveTypeRows))
	}
	employees, err := c.Service.listBusinessEmployees(ctx)
	if err != nil {
		return EHRMSAttendanceSyncResponse{}, err
	}
	fetched, err := c.fetchEHRMSAttendanceAndLeaveData(ctx, employees, syncStart, syncEnd, syncYear)
	if err != nil {
		return EHRMSAttendanceSyncResponse{}, err
	}
	response := EHRMSAttendanceSyncResponse{
		Fetched:              len(fetched.attendance),
		Failed:               fetched.attendanceFetchFailed,
		LeaveTypesFetched:    len(leaveTypeRows),
		LeaveBalancesFetched: len(fetched.leaveBalances),
		LeaveBalancesFailed:  fetched.leaveBalanceFetchFailed,
		LeaveDetailsFetched:  len(fetched.leaveDetails),
		LeaveDetailsFailed:   fetched.leaveDetailFetchFailed,
		Mode:                 mode,
		Start:                syncStart,
		End:                  syncEnd,
		RowErrors:            append([]RowError(nil), fetched.rowErrors...),
	}
	persistenceStartedAt := time.Now()
	c.logInfo(ctx, "eHRMS attendance and leave persistence started",
		"attendance_records", len(fetched.attendance),
		"leave_types", len(leaveTypeRows),
		"leave_balances", len(fetched.leaveBalances),
		"leave_details", len(fetched.leaveDetails),
	)
	if err := c.withTransaction(ctx, func(tx AttendanceService) error {
		if !input.SkipLeaveTypes {
			upserted, deactivated, syncErr := tx.syncEHRMSLeaveTypes(ctx, leaveTypeRows)
			if syncErr != nil {
				return syncErr
			}
			response.LeaveTypesUpserted = upserted
			response.LeaveTypesDeactivated = deactivated
			c.logInfo(ctx, "eHRMS persistence stage completed",
				"stage", "leave_types",
				"upserted", upserted,
				"deactivated", deactivated,
			)
		}

		type attendanceSyncDay struct {
			employeeID string
			workDate   string
		}
		linkableAttendanceDays := map[int]attendanceSyncDay{}
		for idx, record := range fetched.attendance {
			result := tx.syncEHRMSAttendanceRecord(ctx, record, idx+1, mode, syncStart)
			response.Results = append(response.Results, result.result)
			response.RowErrors = append(response.RowErrors, result.rowErrors...)
			switch result.action {
			case "created":
				response.Created++
				linkableAttendanceDays[idx] = attendanceSyncDay{
					employeeID: result.result.EmployeeID,
					workDate:   normalizeEHRMSAttendanceDate(ehrmsAttendanceValue(record, ehrmsAttendanceFieldDate)),
				}
			case "updated":
				response.Updated++
				linkableAttendanceDays[idx] = attendanceSyncDay{
					employeeID: result.result.EmployeeID,
					workDate:   normalizeEHRMSAttendanceDate(ehrmsAttendanceValue(record, ehrmsAttendanceFieldDate)),
				}
			case "skipped":
				response.Skipped++
			case "failed":
				response.Failed++
			}
			c.logEHRMSPersistenceProgress(ctx, "attendance", idx+1, len(fetched.attendance))
		}

		for idx, record := range fetched.leaveBalances {
			result := tx.syncEHRMSLeaveBalanceRecord(ctx, record, idx+1)
			response.Results = append(response.Results, result.result)
			response.RowErrors = append(response.RowErrors, result.rowErrors...)
			switch result.action {
			case "upserted":
				response.LeaveBalancesUpserted++
			case "skipped":
				response.LeaveBalancesSkipped++
			case "failed":
				response.LeaveBalancesFailed++
			}
			c.logEHRMSPersistenceProgress(ctx, "leave_balances", idx+1, len(fetched.leaveBalances))
		}

		seenEHRMSLeaveRecords := map[string]struct{}{}
		for idx, record := range fetched.leaveDetails {
			result := tx.syncEHRMSLeaveDetailRecord(ctx, record, idx+1, mode)
			response.Results = append(response.Results, result.result)
			response.RowErrors = append(response.RowErrors, result.rowErrors...)
			switch result.action {
			case "created":
				response.LeaveDetailsCreated++
			case "updated":
				response.LeaveDetailsUpdated++
			case "skipped":
				response.LeaveDetailsSkipped++
			case "failed":
				response.LeaveDetailsFailed++
			}
			if result.balanceUnmatched {
				response.LeaveDetailsUnmatched++
			}
			if result.leaveRecordID != "" {
				seenEHRMSLeaveRecords[result.leaveRecordID] = struct{}{}
			}
			if result.action == "failed" {
				if employeeID := fetched.leaveSyncEmployees[result.employeeNo]; employeeID != "" {
					fetched.unsafeLeaveSweepEmployees[employeeID] = struct{}{}
				}
			}
			c.logEHRMSPersistenceProgress(ctx, "leave_details", idx+1, len(fetched.leaveDetails))
		}
		if err := tx.tombstoneMissingEHRMSLeaveRecords(
			ctx,
			fetched.leaveSyncEmployees,
			fetched.unsafeLeaveSweepEmployees,
			seenEHRMSLeaveRecords,
			syncStart,
			syncEnd,
		); err != nil {
			return err
		}

		for idx, record := range fetched.attendance {
			day, linkable := linkableAttendanceDays[idx]
			if !linkable {
				continue
			}
			if err := tx.syncEHRMSAttendanceLeaveSegments(ctx, record, idx+1); err != nil {
				return err
			}
			if err := tx.aggregateEHRMSAttendanceLeaveMinutes(ctx, day.employeeID, day.workDate); err != nil {
				return err
			}
		}
		for _, day := range linkableAttendanceDays {
			if _, err := tx.loadAttendanceDayProjection(ctx, day.employeeID, day.workDate, now); err != nil {
				return err
			}
		}
		for _, day := range linkableAttendanceDays {
			if err := tx.reconcileAttendanceDailyRecord(ctx, day.employeeID, day.workDate); err != nil {
				return err
			}
		}
		if err := tx.audit(ctx, "attendance.ehrms.sync", string(ResourceAttendanceClock), "ehrms", string(SeverityHigh), map[string]any{
			"source":                  ehrmsAttendanceSource,
			"fetched":                 response.Fetched,
			"created":                 response.Created,
			"updated":                 response.Updated,
			"skipped":                 response.Skipped,
			"failed":                  response.Failed,
			"leave_types_fetched":     response.LeaveTypesFetched,
			"leave_types_upserted":    response.LeaveTypesUpserted,
			"leave_types_deactivated": response.LeaveTypesDeactivated,
			"leave_balances_fetched":  response.LeaveBalancesFetched,
			"leave_balances_upserted": response.LeaveBalancesUpserted,
			"leave_balances_skipped":  response.LeaveBalancesSkipped,
			"leave_balances_failed":   response.LeaveBalancesFailed,
			"leave_details_fetched":   response.LeaveDetailsFetched,
			"leave_details_created":   response.LeaveDetailsCreated,
			"leave_details_updated":   response.LeaveDetailsUpdated,
			"leave_details_skipped":   response.LeaveDetailsSkipped,
			"leave_details_failed":    response.LeaveDetailsFailed,
			"leave_details_unmatched": response.LeaveDetailsUnmatched,
			"mode":                    mode,
			"start":                   syncStart,
			"end":                     syncEnd,
			"queried_employees":       fetched.queriedEmployees,
		}); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		c.logWarn(ctx, "eHRMS attendance and leave persistence failed",
			"elapsed_ms", time.Since(persistenceStartedAt).Milliseconds(),
			"error", err,
		)
		return EHRMSAttendanceSyncResponse{}, err
	}
	c.logInfo(ctx, "eHRMS attendance and leave sync completed",
		"fetched", response.Fetched,
		"created", response.Created,
		"updated", response.Updated,
		"skipped", response.Skipped,
		"failed", response.Failed,
		"leave_types_upserted", response.LeaveTypesUpserted,
		"leave_balances_fetched", response.LeaveBalancesFetched,
		"leave_balances_upserted", response.LeaveBalancesUpserted,
		"leave_balances_failed", response.LeaveBalancesFailed,
		"leave_details_fetched", response.LeaveDetailsFetched,
		"leave_details_created", response.LeaveDetailsCreated,
		"leave_details_updated", response.LeaveDetailsUpdated,
		"leave_details_failed", response.LeaveDetailsFailed,
		"leave_details_unmatched", response.LeaveDetailsUnmatched,
		"mode", mode,
		"start", syncStart,
		"end", syncEnd,
		"queried_employees", fetched.queriedEmployees,
		"persistence_elapsed_ms", time.Since(persistenceStartedAt).Milliseconds(),
		"elapsed_ms", time.Since(syncStartedAt).Milliseconds(),
	)
	return response, nil
}

func normalizeEHRMSAttendanceSyncBounds(input EHRMSAttendanceSyncInput, now time.Time) (string, string, string, error) {
	localNow := now.In(attendanceClockLocation)
	start := strings.TrimSpace(input.Start)
	end := strings.TrimSpace(input.End)
	if start == "" && end == "" {
		startAt := time.Date(localNow.Year(), time.January, 1, 0, 0, 0, 0, attendanceClockLocation)
		return startAt.Format(time.DateOnly), startAt.AddDate(1, 0, 0).Format(time.DateOnly), strconv.Itoa(startAt.Year()), nil
	}
	if start == "" || end == "" {
		return "", "", "", BadRequest("eHRMS attendance sync start and end must be provided together")
	}
	startAt, startErr := time.ParseInLocation(time.DateOnly, start, attendanceClockLocation)
	endAt, endErr := time.ParseInLocation(time.DateOnly, end, attendanceClockLocation)
	if startErr != nil || endErr != nil || !endAt.After(startAt) {
		return "", "", "", BadRequest("eHRMS attendance sync requires YYYY-MM-DD bounds with end after start")
	}
	if endAt.After(startAt.AddDate(1, 0, 0)) {
		return "", "", "", BadRequest("eHRMS attendance sync range cannot exceed one year")
	}
	return startAt.Format(time.DateOnly), endAt.Format(time.DateOnly), strconv.Itoa(startAt.Year()), nil
}

type ehrmsAttendanceFetchJob struct {
	order      int
	employeeID string
	employeeNo string
}

type ehrmsAttendanceFetchResult struct {
	order         int
	employeeID    string
	employeeNo    string
	attendance    []domain.EHRMSAttendanceRecord
	leaveBalance  []domain.EHRMSLeaveBalanceRecord
	leaveDetails  []domain.EHRMSLeaveDetailRecord
	attendanceErr error
	balanceErr    error
	detailErr     error
}

type ehrmsAttendanceAndLeaveFetch struct {
	attendance                []domain.EHRMSAttendanceRecord
	leaveBalances             []domain.EHRMSLeaveBalanceRecord
	leaveDetails              []domain.EHRMSLeaveDetailRecord
	leaveSyncEmployees        map[string]string
	unsafeLeaveSweepEmployees map[string]struct{}
	queriedEmployees          int
	attendanceFetchFailed     int
	leaveBalanceFetchFailed   int
	leaveDetailFetchFailed    int
	rowErrors                 []RowError
}

func (c AttendanceService) fetchEHRMSAttendanceAndLeaveData(
	ctx RequestContext,
	employees []Employee,
	syncStart string,
	syncEnd string,
	syncYear string,
) (ehrmsAttendanceAndLeaveFetch, error) {
	fetchStartedAt := time.Now()
	fetched := ehrmsAttendanceAndLeaveFetch{
		leaveSyncEmployees:        map[string]string{},
		unsafeLeaveSweepEmployees: map[string]struct{}{},
	}
	jobs := make([]ehrmsAttendanceFetchJob, 0, len(employees))
	for _, employee := range employees {
		employeeNo := strings.TrimSpace(employee.EmployeeNo)
		if employeeNo == "" {
			continue
		}
		jobs = append(jobs, ehrmsAttendanceFetchJob{
			order:      len(jobs),
			employeeID: employee.ID,
			employeeNo: employeeNo,
		})
	}
	workerCount := ehrmsAttendanceFetchConcurrency
	if len(jobs) < workerCount {
		workerCount = len(jobs)
	}
	c.logInfo(ctx, "eHRMS employee-scoped fetch started",
		"total_employees", len(jobs),
		"workers", workerCount,
		"start", syncStart,
	)
	if len(jobs) == 0 {
		c.logInfo(ctx, "eHRMS employee-scoped fetch completed",
			"completed_employees", 0,
			"total_employees", 0,
			"elapsed_ms", time.Since(fetchStartedAt).Milliseconds(),
		)
		return fetched, nil
	}

	fetchCtx := goContext(ctx)
	jobCh := make(chan ehrmsAttendanceFetchJob)
	resultCh := make(chan ehrmsAttendanceFetchResult, len(jobs))

	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for {
				select {
				case <-fetchCtx.Done():
					return
				case job, ok := <-jobCh:
					if !ok {
						return
					}
					query := domain.EHRMSAttendanceQuery{
						EmployeeID: job.employeeNo,
						Start:      syncStart,
						End:        syncEnd,
						Year:       syncYear,
					}
					attendance, attendanceErr := c.ehrmsClient.ListAttendance(fetchCtx, query)
					leaveBalances, balanceErr := c.ehrmsClient.ListLeaveBalances(fetchCtx, query)
					leaveDetails, detailErr := c.ehrmsClient.ListLeaveDetails(fetchCtx, query)
					if attendanceErr == nil {
						attendance = filterEHRMSAttendanceRecordsByDateRange(attendance, syncStart, syncEnd)
					}
					if detailErr == nil {
						leaveDetails = filterEHRMSLeaveDetailRecordsByDateRange(leaveDetails, syncStart, syncEnd)
					}
					resultCh <- ehrmsAttendanceFetchResult{
						order:         job.order,
						employeeID:    job.employeeID,
						employeeNo:    job.employeeNo,
						attendance:    attendance,
						leaveBalance:  leaveBalances,
						leaveDetails:  leaveDetails,
						attendanceErr: attendanceErr,
						balanceErr:    balanceErr,
						detailErr:     detailErr,
					}
				}
			}
		}()
	}
	go func() {
		defer close(jobCh)
		for _, job := range jobs {
			select {
			case jobCh <- job:
			case <-fetchCtx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(resultCh)
	}()

	orderedResults := make([]ehrmsAttendanceFetchResult, len(jobs))
	completedEmployees := 0
	var firstAttendanceErr error
	for result := range resultCh {
		completedEmployees++
		orderedResults[result.order] = result
		if result.attendanceErr != nil {
			if firstAttendanceErr == nil {
				firstAttendanceErr = result.attendanceErr
			}
			fetched.attendanceFetchFailed++
			fetched.rowErrors = append(fetched.rowErrors, RowError{
				Row: result.order + 1, Field: "attendance", Code: "upstream_fetch_failed", Message: "eHRMS attendance request failed",
			})
			c.logWarn(ctx, "eHRMS employee fetch failed", "source", "attendance", "employee_id", result.employeeID, "error", ehrmsFetchError("attendance", result.attendanceErr))
		} else {
			fetched.queriedEmployees++
		}
		if result.balanceErr != nil {
			fetched.leaveBalanceFetchFailed++
			fetched.rowErrors = append(fetched.rowErrors, RowError{
				Row: result.order + 1, Field: "leave_balance", Code: "upstream_fetch_failed", Message: "eHRMS leave balance request failed",
			})
			c.logWarn(ctx, "eHRMS employee fetch failed", "source", "leave_balance", "employee_id", result.employeeID, "error", ehrmsFetchError("leave balances", result.balanceErr))
		}
		if result.detailErr != nil {
			fetched.leaveDetailFetchFailed++
			fetched.unsafeLeaveSweepEmployees[result.employeeID] = struct{}{}
			fetched.rowErrors = append(fetched.rowErrors, RowError{
				Row: result.order + 1, Field: "leave_detail", Code: "upstream_fetch_failed", Message: "eHRMS leave detail request failed",
			})
			c.logWarn(ctx, "eHRMS employee fetch failed", "source", "leave_detail", "employee_id", result.employeeID, "error", ehrmsFetchError("leave details", result.detailErr))
		} else {
			fetched.leaveSyncEmployees[result.employeeNo] = result.employeeID
		}
		if completedEmployees%ehrmsAttendanceProgressLogEvery == 0 || completedEmployees == len(jobs) {
			c.logInfo(ctx, "eHRMS employee-scoped fetch progress",
				"completed_employees", completedEmployees,
				"total_employees", len(jobs),
				"progress_percent", completedEmployees*100/len(jobs),
				"attendance_fetch_failed", fetched.attendanceFetchFailed,
				"leave_balance_fetch_failed", fetched.leaveBalanceFetchFailed,
				"leave_detail_fetch_failed", fetched.leaveDetailFetchFailed,
				"elapsed_ms", time.Since(fetchStartedAt).Milliseconds(),
			)
		}
	}
	if err := fetchCtx.Err(); err != nil {
		return ehrmsAttendanceAndLeaveFetch{}, err
	}
	if fetched.queriedEmployees == 0 && firstAttendanceErr != nil {
		return ehrmsAttendanceAndLeaveFetch{}, ehrmsFetchError("attendance", firstAttendanceErr)
	}
	for _, result := range orderedResults {
		if result.attendanceErr == nil {
			fetched.attendance = append(fetched.attendance, result.attendance...)
		}
		if result.balanceErr == nil {
			fetched.leaveBalances = append(fetched.leaveBalances, result.leaveBalance...)
		}
		if result.detailErr == nil {
			fetched.leaveDetails = append(fetched.leaveDetails, result.leaveDetails...)
		}
	}
	c.logInfo(ctx, "eHRMS employee-scoped fetch completed",
		"completed_employees", completedEmployees,
		"total_employees", len(jobs),
		"attendance_records", len(fetched.attendance),
		"leave_balances", len(fetched.leaveBalances),
		"leave_details", len(fetched.leaveDetails),
		"attendance_fetch_failed", fetched.attendanceFetchFailed,
		"leave_balance_fetch_failed", fetched.leaveBalanceFetchFailed,
		"leave_detail_fetch_failed", fetched.leaveDetailFetchFailed,
		"elapsed_ms", time.Since(fetchStartedAt).Milliseconds(),
	)
	return fetched, nil
}

func filterEHRMSAttendanceRecordsByDateRange(records []domain.EHRMSAttendanceRecord, start, end string) []domain.EHRMSAttendanceRecord {
	out := make([]domain.EHRMSAttendanceRecord, 0, len(records))
	for _, record := range records {
		date := normalizeEHRMSAttendanceDate(ehrmsAttendanceValue(record, ehrmsAttendanceFieldDate))
		if date == "" || ehrmsDateInHalfOpenRange(date, start, end) {
			out = append(out, record)
		}
	}
	return out
}

func filterEHRMSLeaveDetailRecordsByDateRange(records []domain.EHRMSLeaveDetailRecord, start, end string) []domain.EHRMSLeaveDetailRecord {
	out := make([]domain.EHRMSLeaveDetailRecord, 0, len(records))
	for _, record := range records {
		if ehrmsLeaveDetailOverlapsHalfOpenRange(record, start, end) {
			out = append(out, record)
		}
	}
	return out
}

func ehrmsLeaveDetailOverlapsHalfOpenRange(record domain.EHRMSLeaveDetailRecord, start, end string) bool {
	rangeStart, startErr := time.ParseInLocation(time.DateOnly, start, attendanceClockLocation)
	rangeEnd, endErr := time.ParseInLocation(time.DateOnly, end, attendanceClockLocation)
	workDate := normalizeEHRMSAttendanceDate(ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldDate))
	recordStart, recordStartOK := parseEHRMSLeaveDetailDateTime(workDate, ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldStart))
	recordEnd, recordEndOK := parseEHRMSLeaveDetailDateTime(workDate, ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldEnd))
	if startErr == nil && endErr == nil && recordStartOK && recordEndOK {
		return recordEnd.After(rangeStart) && recordStart.Before(rangeEnd)
	}
	if workDate == "" {
		return true
	}
	return ehrmsDateInHalfOpenRange(workDate, start, end)
}

func ehrmsDateInHalfOpenRange(value, start, end string) bool {
	date := normalizeEHRMSAttendanceDate(value)
	return date != "" && date >= start && date < end
}

func (c AttendanceService) logEHRMSPersistenceProgress(ctx RequestContext, stage string, completed, total int) {
	if total == 0 || (completed%ehrmsPersistenceProgressLogEvery != 0 && completed != total) {
		return
	}
	c.logInfo(ctx, "eHRMS persistence progress",
		"stage", stage,
		"completed", completed,
		"total", total,
		"progress_percent", completed*100/total,
	)
}

type ehrmsAttendanceSyncResult struct {
	action           string
	result           BatchEmployeeResult
	rowErrors        []RowError
	employeeNo       string
	leaveRecordID    string
	balanceUnmatched bool
}

// syncEHRMSLeaveTypes upserts the EHRMS /leave-types catalog and deactivates missing EHRMS rows.
// max_value is converted into max_balance_minutes (and requires_balance) using the same unit rules as leave balances.
func (c AttendanceService) syncEHRMSLeaveTypes(ctx RequestContext, rows []domain.EHRMSLeaveTypeRecord) (upserted int, deactivated int, err error) {
	dayHours := 8.0
	if policy, loadErr := c.loadAttendancePolicyResponse(ctx); loadErr == nil {
		dayHours = standardDayHours(policy.WorkTime)
	}
	now := c.Now()
	activeIDs := make([]string, 0, len(rows))
	codeCounts := make(map[string]int, len(rows))
	categoryIDs := make(map[string]string, len(rows))
	parentUnits := make(map[string]string, len(rows))
	for _, record := range rows {
		code := ehrmsLeaveTypeCode(record)
		if code == "" {
			continue
		}
		codeCounts[code]++
		if normalizedEHRMSLeaveTypeKind(record) == "category" {
			id := domain.StableLeaveTypeID(code)
			if codeCounts[code] > 1 {
				id = "category:" + code
			}
			categoryIDs[code] = id
			parentUnits[code] = strings.TrimSpace(ehrmsLeaveTypeValue(record, ehrmsLeaveTypeFieldUnit))
		}
	}
	// A category can occur before its same-code item, so finalize collision IDs
	// after all code counts are known.
	for code := range categoryIDs {
		if codeCounts[code] > 1 {
			categoryIDs[code] = "category:" + code
		}
	}
	orderedRows := make([]domain.EHRMSLeaveTypeRecord, 0, len(rows))
	for _, record := range rows {
		if normalizedEHRMSLeaveTypeKind(record) != "item" {
			orderedRows = append(orderedRows, record)
		}
	}
	for _, record := range rows {
		if normalizedEHRMSLeaveTypeKind(record) == "item" {
			orderedRows = append(orderedRows, record)
		}
	}
	seenNodes := make(map[string]struct{}, len(rows))
	for idx, record := range orderedRows {
		code := ehrmsLeaveTypeCode(record)
		if code == "" {
			continue
		}
		nameZH := strings.TrimSpace(ehrmsLeaveTypeValue(record, ehrmsLeaveTypeFieldName))
		if nameZH == "" {
			nameZH = code
		}
		kind := normalizedEHRMSLeaveTypeKind(record)
		nodeKey := kind + "\x00" + code
		if _, duplicate := seenNodes[nodeKey]; duplicate {
			continue
		}
		seenNodes[nodeKey] = struct{}{}
		parentCode := strings.ToLower(strings.TrimSpace(ehrmsLeaveTypeValue(record, ehrmsLeaveTypeFieldParent)))
		parentID := ""
		if kind != "item" {
			parentCode = ""
		} else if resolvedParentID, found := categoryIDs[parentCode]; found {
			parentID = resolvedParentID
		} else {
			parentCode = ""
		}
		unit := strings.TrimSpace(ehrmsLeaveTypeValue(record, ehrmsLeaveTypeFieldUnit))
		if unit == "" && parentCode != "" {
			unit = parentUnits[parentCode]
		}
		maxValueRaw := ehrmsLeaveTypeValue(record, ehrmsLeaveTypeFieldMaxValue)
		maxHours, ok := parseEHRMSLeaveTypeMaxValue(maxValueRaw, unit, dayHours)
		if !ok {
			maxHours = 0
		}
		maxMinutes := leaveMinutes(maxHours)
		id := domain.StableLeaveTypeID(code)
		if kind != "item" && codeCounts[code] > 1 {
			id = kind + ":" + code
		}
		item := domain.LeaveType{
			ID:                id,
			TenantID:          ctx.TenantID,
			Code:              code,
			Kind:              kind,
			ParentID:          parentID,
			ParentCode:        parentCode,
			NameZH:            nameZH,
			NameEN:            strings.TrimSpace(ehrmsLeaveTypeValue(record, ehrmsLeaveTypeFieldNameEN)),
			Category:          ehrmsLeaveTypeCategory(record),
			RequiresBalance:   maxMinutes > 0,
			MaxBalanceMinutes: maxMinutes,
			Unit:              unit,
			Enabled:           true,
			DisplayOrder:      idx + 1,
			RawPayload:        ehrmsStringPayload(map[string]string(record)),
			LastSyncedAt:      &now,
			UpdatedAt:         now,
		}
		if err := c.store.UpsertLeaveType(goContext(ctx), item); err != nil {
			return upserted, 0, err
		}
		activeIDs = append(activeIDs, id)
		upserted++
	}
	if len(rows) == 0 {
		// Empty upstream catalog is treated as "no change" for deactivation to avoid
		// wiping the local catalog on a blank/misconfigured response.
		return upserted, 0, nil
	}
	n, err := c.store.DeactivateMissingLeaveTypes(goContext(ctx), ctx.TenantID, activeIDs, now)
	if err != nil {
		return upserted, 0, err
	}
	return upserted, int(n), nil
}

func ehrmsLeaveTypeCode(record domain.EHRMSLeaveTypeRecord) string {
	code := strings.ToLower(strings.TrimSpace(ehrmsLeaveTypeValue(record, ehrmsLeaveTypeFieldCode)))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(ehrmsLeaveTypeValue(record, ehrmsLeaveTypeFieldName)))
	}
	return code
}

func normalizedEHRMSLeaveTypeKind(record domain.EHRMSLeaveTypeRecord) string {
	switch strings.ToLower(strings.TrimSpace(ehrmsLeaveTypeValue(record, ehrmsLeaveTypeFieldKind))) {
	case "category":
		return "category"
	case "special_group":
		return "special_group"
	default:
		return "item"
	}
}

func ehrmsLeaveTypeCategory(record domain.EHRMSLeaveTypeRecord) string {
	value := strings.ToLower(strings.TrimSpace(ehrmsLeaveTypeValue(record, ehrmsLeaveTypeFieldCategory)))
	if value == "statutory" || strings.Contains(value, "法定") {
		return "statutory"
	}
	return "company"
}

func ehrmsLeaveTypeValue(record domain.EHRMSLeaveTypeRecord, field string) string {
	if len(record) == 0 {
		return ""
	}
	if value := strings.TrimSpace(record[field]); value != "" {
		return value
	}
	switch field {
	case ehrmsLeaveTypeFieldCode:
		return utils.FirstNonEmpty(record["code"], record["leave_code"])
	case ehrmsLeaveTypeFieldKind:
		return strings.TrimSpace(record["kind"])
	case ehrmsLeaveTypeFieldParent:
		return strings.TrimSpace(record["parent_code"])
	case ehrmsLeaveTypeFieldName:
		return utils.FirstNonEmpty(record["name"], record["name_zh"], record["leave_type"], record["假別"])
	case ehrmsLeaveTypeFieldNameEN:
		return strings.TrimSpace(record["name_en"])
	case ehrmsLeaveTypeFieldMaxValue:
		return utils.FirstNonEmpty(record["max_value"], record["maxValue"], record["最大額度"], record["額度上限"])
	case ehrmsLeaveTypeFieldUnit:
		return strings.TrimSpace(record["unit"])
	case ehrmsLeaveTypeFieldCategory:
		return strings.TrimSpace(record["category"])
	default:
		return ""
	}
}

func (c AttendanceService) syncEHRMSAttendanceRecord(ctx RequestContext, record domain.EHRMSAttendanceRecord, rowNumber int, mode string, start string) ehrmsAttendanceSyncResult {
	summary, employeeNo, errors := c.ehrmsAttendanceSummaryCandidate(ctx, record, rowNumber)
	if len(errors) > 0 {
		return ehrmsAttendanceFailed(rowNumber, errors)
	}
	if start != "" && summary.WorkDate < start {
		return ehrmsAttendanceSkipped(rowNumber, "", "before_start", "attendance summary is before start date")
	}
	employee, ok, err := c.store.GetEmployeeByEmployeeNo(goContext(ctx), ctx.TenantID, employeeNo)
	if err != nil {
		return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "employee_no", Code: "store_error", Message: err.Error()}})
	}
	if !ok {
		return ehrmsAttendanceSkipped(rowNumber, "", "employee_not_found", "employee_no was not found for eHRMS attendance sync")
	}
	summary.EmployeeID = employee.ID
	existing, ok, err := c.store.GetAttendanceDailySummaryByExternalRef(goContext(ctx), ctx.TenantID, summary.ExternalRef)
	if err != nil {
		return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "external_ref", Code: "store_error", Message: err.Error()}})
	}
	if !ok {
		existing, ok, err = c.store.GetAttendanceDailySummaryByEmployeeDate(goContext(ctx), ctx.TenantID, employee.ID, summary.WorkDate)
		if err != nil {
			return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "work_date", Code: "store_error", Message: err.Error()}})
		}
	}
	update := ok
	switch mode {
	case employeeSyncModeCreate:
		if update {
			return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "work_date", Code: "unique", Message: "attendance daily summary already exists"}})
		}
	case employeeSyncModeUpdate:
		if !update {
			return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "work_date", Code: "not_found", Message: "attendance daily summary was not found for eHRMS sync"}})
		}
	}
	if update {
		summary.CreatedAt = existing.CreatedAt
	}
	if err := c.store.UpsertAttendanceDailySummary(goContext(ctx), summary); err != nil {
		return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "work_date", Code: "store_error", Message: err.Error()}})
	}
	if err := c.upsertEHRMSAttendanceDailyRecord(ctx, summary, 0); err != nil {
		return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "work_date", Code: "store_error", Message: err.Error()}})
	}
	action := "created"
	if update {
		action = "updated"
	}
	return ehrmsAttendanceSyncResult{action: action, result: BatchEmployeeResult{RowNumber: rowNumber, EmployeeID: employee.ID, Success: true, Action: action, Message: action}}
}

// syncEHRMSLeaveBalanceRecord excludes attendance metrics before persisting an employee leave balance.
func (c AttendanceService) syncEHRMSLeaveBalanceRecord(ctx RequestContext, record domain.EHRMSLeaveBalanceRecord, rowNumber int) ehrmsAttendanceSyncResult {
	leaveTypeRaw := ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldLeaveType)
	if isEHRMSAttendanceOnlyLeaveType(leaveTypeRaw) {
		return ehrmsAttendanceSkipped(rowNumber, "", "non_leave_balance_type", "attendance-only type was excluded from leave balance sync")
	}
	balance, employeeNo, errors := c.ehrmsLeaveBalanceCandidate(ctx, record, rowNumber)
	if len(errors) > 0 {
		return ehrmsAttendanceFailed(rowNumber, errors)
	}
	employee, ok, err := c.store.GetEmployeeByEmployeeNo(goContext(ctx), ctx.TenantID, employeeNo)
	if err != nil {
		return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "employee_no", Code: "store_error", Message: err.Error()}})
	}
	if !ok {
		return ehrmsAttendanceSkipped(rowNumber, "", "employee_not_found", "employee_no was not found for eHRMS leave balance sync")
	}
	balance.EmployeeID = employee.ID
	balance.ID = ehrmsLeaveBalanceStableID(ctx.TenantID, employee.EmployeeNo, balance)
	if err := c.store.UpsertLeaveBalance(goContext(ctx), balance); err != nil {
		return ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "leave_type", Code: "store_error", Message: err.Error()}})
	}
	return ehrmsAttendanceSyncResult{action: "upserted", result: BatchEmployeeResult{RowNumber: rowNumber, EmployeeID: employee.ID, Success: true, Action: "upserted", Message: "upserted"}}
}

// ehrmsLeaveBalanceStableID identifies the one annual balance for an
// employee/type/year tuple.
func ehrmsLeaveBalanceStableID(tenantID, employeeNo string, balance LeaveBalance) string {
	return ehrmsStableID(
		"ehrms-lb",
		strings.TrimSpace(tenantID),
		strings.ToLower(strings.TrimSpace(employeeNo)),
		strings.TrimSpace(balance.LeaveTypeID),
		strconv.Itoa(balance.EntitlementYear),
	)
}

// syncEHRMSLeaveDetailRecord persists an eHRMS fact independently from Nexus
// workflow requests, then reconciles exact dual-entry matches.
func (c AttendanceService) syncEHRMSLeaveDetailRecord(ctx RequestContext, record domain.EHRMSLeaveDetailRecord, rowNumber int, mode string) ehrmsAttendanceSyncResult {
	leaveTypeRaw := ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldLeaveType)
	if isEHRMSAttendanceOnlyLeaveType(leaveTypeRaw) {
		return ehrmsAttendanceSkipped(rowNumber, "", "non_leave_detail_type", "attendance-only type was excluded from leave detail sync")
	}
	external, employeeNo, _, errors := c.ehrmsLeaveDetailCandidate(ctx, record, rowNumber)
	if len(errors) > 0 {
		result := ehrmsAttendanceFailed(rowNumber, errors)
		result.employeeNo = employeeNo
		result.leaveRecordID = external.ID
		return result
	}
	employee, ok, err := c.store.GetEmployeeByEmployeeNo(goContext(ctx), ctx.TenantID, employeeNo)
	if err != nil {
		result := ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "employee_no", Code: "store_error", Message: err.Error()}})
		result.employeeNo = employeeNo
		result.leaveRecordID = external.ID
		return result
	}
	if !ok {
		result := ehrmsAttendanceSkipped(rowNumber, "", "employee_not_found", "employee_no was not found for eHRMS leave detail sync")
		result.employeeNo = employeeNo
		result.leaveRecordID = external.ID
		return result
	}
	external.EmployeeID = employee.ID
	balance, balanceFound, err := c.store.GetLeaveBalanceForOverlay(goContext(ctx), ctx.TenantID, employee.ID, external.LeaveTypeID, external.StartAt)
	if err != nil {
		result := ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "leave_detail", Code: "store_error", Message: err.Error()}})
		result.employeeNo = employeeNo
		result.leaveRecordID = external.ID
		return result
	}
	if balanceFound {
		external.BalanceID = balance.ID
		external.EntitlementYear = balance.EntitlementYear
		external.BalanceMatchStatus = "matched"
		external.BalanceMatchReason = ""
	}
	existing, exists, err := c.store.GetLeaveRecord(goContext(ctx), ctx.TenantID, external.ID)
	if err != nil {
		result := ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "leave_detail", Code: "store_error", Message: err.Error()}})
		result.employeeNo = employeeNo
		result.leaveRecordID = external.ID
		return result
	}
	switch mode {
	case employeeSyncModeCreate:
		if exists {
			result := ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "leave_detail", Code: "unique", Message: "leave detail already exists"}})
			result.employeeNo = employeeNo
			result.leaveRecordID = existing.ID
			return result
		}
	case employeeSyncModeUpdate:
		if !exists {
			result := ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "leave_detail", Code: "not_found", Message: "leave detail was not found for eHRMS sync"}})
			result.employeeNo = employeeNo
			result.leaveRecordID = external.ID
			return result
		}
	}
	if exists {
		external.EventDate = existing.EventDate
		external.MatchedRecordID = existing.MatchedRecordID
		external.ReconciliationStatus = existing.ReconciliationStatus
	}
	if err := c.store.UpsertLeaveRecord(goContext(ctx), external); err != nil {
		result := ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "leave_detail", Code: "store_error", Message: err.Error()}})
		result.employeeNo = employeeNo
		result.leaveRecordID = external.ID
		return result
	}
	if err := c.reconcileEHRMSLeaveRecord(ctx, external); err != nil {
		result := ehrmsAttendanceFailed(rowNumber, []RowError{{Row: rowNumber, Field: "leave_detail", Code: "reconciliation_error", Message: err.Error()}})
		result.employeeNo = employeeNo
		result.leaveRecordID = external.ID
		return result
	}
	action := "created"
	if exists {
		action = "updated"
	}
	return ehrmsAttendanceSyncResult{
		action: action, employeeNo: employeeNo, leaveRecordID: external.ID,
		balanceUnmatched: external.BalanceMatchStatus == "unmatched",
		result: BatchEmployeeResult{
			RowNumber: rowNumber, EmployeeID: employee.ID, Success: true, Action: action,
			Code: ehrmsLeaveDetailResultCode(external), Message: ehrmsLeaveDetailResultMessage(action, external),
		},
	}
}

func ehrmsLeaveDetailResultCode(record LeaveRecord) string {
	if record.BalanceMatchStatus == "unmatched" {
		return "balance_unmatched"
	}
	return ""
}

func ehrmsLeaveDetailResultMessage(action string, record LeaveRecord) string {
	if record.BalanceMatchStatus == "unmatched" {
		return action + "; annual balance pending"
	}
	return action
}

// tombstoneMissingEHRMSLeaveRecords closes a successful employee-scoped
// snapshot sync. A malformed row disables deletion for only that employee so a
// partial import cannot erase previously valid facts.
func (c AttendanceService) tombstoneMissingEHRMSLeaveRecords(ctx RequestContext, scopedEmployees map[string]string, unsafeEmployees map[string]struct{}, seen map[string]struct{}, syncStart, syncEnd string) error {
	scopedEmployeeIDs := map[string]struct{}{}
	for _, employeeID := range scopedEmployees {
		scopedEmployeeIDs[employeeID] = struct{}{}
	}
	startAt, _ := time.ParseInLocation(time.DateOnly, syncStart, attendanceClockLocation)
	endAt, _ := time.ParseInLocation(time.DateOnly, syncEnd, attendanceClockLocation)
	items, err := c.store.ListLeaveRecords(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Source != ehrmsAttendanceSource || item.DeletedAt != nil {
			continue
		}
		if _, ok := scopedEmployeeIDs[item.EmployeeID]; !ok {
			continue
		}
		if _, unsafe := unsafeEmployees[item.EmployeeID]; unsafe {
			continue
		}
		if (!startAt.IsZero() && !item.EndAt.After(startAt)) ||
			(!endAt.IsZero() && !item.StartAt.Before(endAt)) {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		now := c.Now()
		item.Status = "cancelled"
		item.DeletedAt = &now
		if err := c.store.UpsertLeaveRecord(goContext(ctx), item); err != nil {
			return err
		}
		if err := c.reconcileEHRMSLeaveRecord(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (c AttendanceService) ehrmsAttendanceSummaryCandidate(ctx RequestContext, record domain.EHRMSAttendanceRecord, rowNumber int) (AttendanceDailySummary, string, []RowError) {
	errors := make([]RowError, 0)
	employeeNo := ehrmsAttendanceValue(record, ehrmsAttendanceFieldEmployeeNo)
	workDate := normalizeEHRMSAttendanceDate(ehrmsAttendanceValue(record, ehrmsAttendanceFieldDate))
	if employeeNo == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "employee_no", Code: "required", Message: "employee_no is required"})
	}
	if workDate == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "date", Code: "invalid", Message: "date must be YYYY-MM-DD"})
	}
	shiftStart := normalizeEHRMSAttendanceTime(ehrmsAttendanceValue(record, ehrmsAttendanceFieldShiftStart))
	if ehrmsAttendanceValue(record, ehrmsAttendanceFieldShiftStart) != "" && shiftStart == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "shift_start", Code: "invalid", Message: "shift_start must be HH:MM"})
	}
	shiftEnd := normalizeEHRMSAttendanceTime(ehrmsAttendanceValue(record, ehrmsAttendanceFieldShiftEnd))
	if ehrmsAttendanceValue(record, ehrmsAttendanceFieldShiftEnd) != "" && shiftEnd == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "shift_end", Code: "invalid", Message: "shift_end must be HH:MM"})
	}
	shiftHours, ok := parseEHRMSAttendanceHours(ehrmsAttendanceValue(record, ehrmsAttendanceFieldShiftHours))
	if !ok {
		errors = append(errors, RowError{Row: rowNumber, Field: "shift_hours", Code: "invalid", Message: "shift_hours must be a number"})
	}
	dailyHours, ok := parseEHRMSAttendanceHours(ehrmsAttendanceValue(record, ehrmsAttendanceFieldDailyHours))
	if !ok {
		errors = append(errors, RowError{Row: rowNumber, Field: "daily_hours", Code: "invalid", Message: "daily_hours must be a number"})
	}
	clockHours, ok := parseEHRMSAttendanceHours(ehrmsAttendanceValue(record, ehrmsAttendanceFieldClockHours))
	if !ok {
		errors = append(errors, RowError{Row: rowNumber, Field: "clock_hours", Code: "invalid", Message: "clock_hours must be a number"})
	}
	clockStart := ehrmsAttendanceTimeField(record, ehrmsAttendanceFieldClockStart, rowNumber, &errors)
	clockEnd := ehrmsAttendanceTimeField(record, ehrmsAttendanceFieldClockEnd, rowNumber, &errors)
	now := c.Now()
	return AttendanceDailySummary{
		TenantID:    ctx.TenantID,
		WorkDate:    workDate,
		ShiftStart:  shiftStart,
		ShiftEnd:    shiftEnd,
		ShiftHours:  shiftHours,
		DailyHours:  dailyHours,
		ClockHours:  clockHours,
		ClockStart:  clockStart,
		ClockEnd:    clockEnd,
		Payload:     ehrmsAttendancePayload(record),
		Source:      ehrmsAttendanceSource,
		ExternalRef: fmt.Sprintf("%s:%s", employeeNo, workDate),
		CreatedAt:   now,
		UpdatedAt:   now,
	}, employeeNo, errors
}

func (c AttendanceService) ehrmsLeaveBalanceCandidate(ctx RequestContext, record domain.EHRMSLeaveBalanceRecord, rowNumber int) (LeaveBalance, string, []RowError) {
	errors := make([]RowError, 0)
	employeeNo := ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldEmployeeNo)
	leaveTypeRaw := ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldLeaveType)
	externalLeaveCode := ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldLeaveCode)
	externalCategoryCode := ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldCategoryCode)
	entitlementYear, yearOK := parseEHRMSLeaveBalanceYear(ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldYear))
	if !yearOK {
		errors = append(errors, RowError{Row: rowNumber, Field: "entitlement_year", Code: "invalid", Message: "entitlement_year must be a valid year"})
	}
	asOf := c.Now()
	if yearOK {
		asOf = time.Date(entitlementYear, time.January, 1, 0, 0, 0, 0, attendanceClockLocation)
	}
	leaveType, leaveTypeID, leaveTypeFound, mappingErr := c.resolveEHRMSLeaveType(ctx, externalLeaveCode, externalCategoryCode, leaveTypeRaw, asOf)
	if mappingErr != nil {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave_type", Code: "store_error", Message: mappingErr.Error()})
	} else if leaveTypeRaw != "" && !leaveTypeFound {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave_type", Code: leaveTypeUnknownCode, Message: "leave_type is not in the tenant leave catalog"})
	}
	if employeeNo == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "employee_no", Code: "required", Message: "employee_no is required"})
	}
	if leaveTypeRaw == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave_type", Code: "required", Message: "leave_type is required"})
	}
	dayHours := 8.0
	if policy, err := c.loadAttendancePolicyResponse(ctx); err == nil {
		dayHours = standardDayHours(policy.WorkTime)
	}
	unit := strings.ToLower(ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldUnit))
	quota, ok := parseEHRMSLeaveBalanceNumber(ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldQuota), unit, dayHours)
	if !ok || quota < 0 {
		errors = append(errors, RowError{Row: rowNumber, Field: "quota", Code: "invalid", Message: "quota must be a non-negative number"})
	}
	used, ok := parseEHRMSLeaveBalanceNumber(ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldUsed), unit, dayHours)
	if !ok || used < 0 {
		errors = append(errors, RowError{Row: rowNumber, Field: "used", Code: "invalid", Message: "used must be a non-negative number"})
	}
	remainingRaw := ehrmsLeaveBalanceValue(record, ehrmsLeaveBalanceFieldRemaining)
	remaining, ok := parseEHRMSLeaveBalanceNumber(remainingRaw, unit, dayHours)
	if !ok {
		errors = append(errors, RowError{Row: rowNumber, Field: "remaining", Code: "invalid", Message: "remaining must be a number"})
	}
	if strings.TrimSpace(remainingRaw) == "" && quota > 0 {
		remaining = quota - used
	}
	now := c.Now()
	return LeaveBalance{
		TenantID:         ctx.TenantID,
		LeaveType:        leaveType,
		LeaveTypeID:      leaveTypeID,
		RemainingMinutes: leaveMinutes(remaining),
		GrantedMinutes:   leaveMinutes(quota),
		UsedMinutes:      leaveMinutes(used),
		Source:           ehrmsAttendanceSource,
		SourcePayload:    ehrmsStringRecordPayload(record),
		SourceUpdatedAt:  ehrmsRecordUpdatedAt(record),
		EntitlementYear:  entitlementYear,
		LastSyncedAt:     &now,
		UpdatedAt:        now,
	}, employeeNo, errors
}

func (c AttendanceService) ehrmsLeaveDetailCandidate(ctx RequestContext, record domain.EHRMSLeaveDetailRecord, rowNumber int) (LeaveRecord, string, string, []RowError) {
	errors := make([]RowError, 0)
	employeeNo := ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldEmployeeNo)
	workDate := normalizeEHRMSAttendanceDate(ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldDate))
	leaveTypeRaw := ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldLeaveType)
	externalLeaveCode := ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldLeaveCode)
	externalCategoryCode := ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldCategoryCode)
	asOf := c.Now()
	if parsed, err := time.Parse(time.DateOnly, workDate); err == nil {
		asOf = parsed
	}
	_, leaveTypeID, leaveTypeFound, mappingErr := c.resolveEHRMSLeaveType(ctx, externalLeaveCode, externalCategoryCode, leaveTypeRaw, asOf)
	if mappingErr != nil {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave_type", Code: "store_error", Message: mappingErr.Error()})
	} else if leaveTypeRaw != "" && !leaveTypeFound {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave_type", Code: leaveTypeUnknownCode, Message: "leave_type is not in the tenant leave catalog"})
	}
	if employeeNo == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "employee_no", Code: "required", Message: "employee_no is required"})
	}
	if workDate == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "date", Code: "invalid", Message: "date must be YYYY-MM-DD"})
	}
	if leaveTypeRaw == "" {
		errors = append(errors, RowError{Row: rowNumber, Field: "leave_type", Code: "required", Message: "leave_type is required"})
	}
	startAt, ok := parseEHRMSLeaveDetailDateTime(workDate, ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldStart))
	if !ok {
		errors = append(errors, RowError{Row: rowNumber, Field: "start", Code: "invalid", Message: "start must be HH:MM or datetime"})
	}
	endAt, ok := parseEHRMSLeaveDetailDateTime(workDate, ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldEnd))
	if !ok {
		errors = append(errors, RowError{Row: rowNumber, Field: "end", Code: "invalid", Message: "end must be HH:MM or datetime"})
	}
	if !startAt.IsZero() && !endAt.IsZero() && !endAt.After(startAt) {
		errors = append(errors, RowError{Row: rowNumber, Field: "end", Code: "invalid", Message: "end must be after start"})
	}
	hours, ok := parseEHRMSAttendanceHours(ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldHours))
	if !ok || hours <= 0 {
		errors = append(errors, RowError{Row: rowNumber, Field: "hours", Code: "invalid", Message: "hours must be greater than zero"})
	}
	netMinutes := leaveMinutes(hours)
	grossMinutes := int(endAt.Sub(startAt).Minutes())
	deductMinutes := parseEHRMSDeductMinutes(ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldDeductHours))
	if deductMinutes == 0 && grossMinutes > netMinutes {
		deductMinutes = grossMinutes - netMinutes
	}
	if grossMinutes < netMinutes {
		errors = append(errors, RowError{Row: rowNumber, Field: "hours", Code: "invalid", Message: "hours cannot exceed the leave interval"})
	}
	if deductMinutes+netMinutes > grossMinutes {
		errors = append(errors, RowError{Row: rowNumber, Field: "deduct_hours", Code: "invalid", Message: "deduct_hours plus hours cannot exceed the leave interval"})
	}
	now := c.Now()
	recordIdentity := ehrmsLeaveDetailIdentity(record, employeeNo, leaveTypeID, externalLeaveCode, externalCategoryCode, startAt, endAt)
	entitlementYear := startAt.In(attendanceClockLocation).Year()
	return LeaveRecord{
		ID: ehrmsStableID("elr", ctx.TenantID, recordIdentity), TenantID: ctx.TenantID,
		Source: ehrmsAttendanceSource, ExternalRef: recordIdentity, LeaveTypeID: leaveTypeID,
		EntitlementYear: entitlementYear, EventDate: now,
		StartAt: startAt, EndAt: endAt, NetMinutes: netMinutes,
		Remark: ehrmsLeaveDetailValue(record, ehrmsLeaveDetailFieldRemark),
		Status: "active", ReconciliationStatus: "unmatched",
		BalanceMatchStatus: "unmatched", BalanceMatchReason: "annual_balance_not_found",
		SourcePayload: ehrmsStringRecordPayload(record), SourceUpdatedAt: ehrmsRecordUpdatedAt(record),
		LastSeenAt: &now, UpdatedAt: now,
	}, employeeNo, workDate, errors
}

func ehrmsLeaveDetailIdentity(record domain.EHRMSLeaveDetailRecord, employeeNo, leaveTypeID, externalLeaveCode, externalCategoryCode string, startAt, endAt time.Time) string {
	for _, key := range []string{"record_id", "leave_id", "request_id", "id", "請假單號", "單號", "流水號"} {
		if value := strings.TrimSpace(record[key]); value != "" {
			return ehrmsStableID("ehrms-leave", employeeNo, "upstream", value)
		}
	}
	return ehrmsStableID(
		"ehrms-leave", employeeNo, leaveTypeID, externalLeaveCode, externalCategoryCode,
		startAt.Format(time.RFC3339), endAt.Format(time.RFC3339),
	)
}

func ehrmsAttendanceFailed(rowNumber int, errors []RowError) ehrmsAttendanceSyncResult {
	return ehrmsAttendanceSyncResult{
		action:    "failed",
		rowErrors: errors,
		result:    BatchEmployeeResult{RowNumber: rowNumber, Success: false, Code: "import_validation_failed", Message: firstEmployeeRowErrorMessage(errors)},
	}
}

func ehrmsAttendanceSkipped(rowNumber int, employeeID string, code string, message string) ehrmsAttendanceSyncResult {
	return ehrmsAttendanceSyncResult{
		action: "skipped",
		result: BatchEmployeeResult{RowNumber: rowNumber, EmployeeID: employeeID, Success: true, Action: "skipped", Code: code, Message: message},
	}
}

func normalizeEHRMSAttendanceDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := time.Parse(time.DateOnly, value); err == nil {
		return parsed.Format(time.DateOnly)
	}
	if parsed, err := time.Parse("2006/01/02", value); err == nil {
		return parsed.Format(time.DateOnly)
	}
	if parsed, err := utils.ParseDate(value); err == nil {
		return parsed.UTC().Format(time.DateOnly)
	}
	return ""
}

func normalizeEHRMSAttendanceTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, layout := range []string{"15:04", "15:04:05", "2006-01-02 15:04", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("15:04")
		}
	}
	return ""
}

func parseEHRMSAttendanceHours(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, true
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func parseEHRMSLeaveBalanceNumber(value string, unit string, configuredDayHours ...float64) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, true
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "days", "day", "天", "日":
		dayHours := 8.0
		if len(configuredDayHours) > 0 && configuredDayHours[0] > 0 {
			dayHours = configuredDayHours[0]
		}
		return n * dayHours, true
	default:
		return n, true
	}
}

// parseEHRMSLeaveTypeMaxValue accepts catalog values such as
// "112小時(後1年)" while keeping balance endpoints strict-number only.
func parseEHRMSLeaveTypeMaxValue(value string, unit string, configuredDayHours ...float64) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, true
	}
	end := 0
	for end < len(value) {
		char := value[end]
		if (char < '0' || char > '9') && char != '.' {
			break
		}
		end++
	}
	if end == 0 {
		return 0, false
	}
	return parseEHRMSLeaveBalanceNumber(value[:end], unit, configuredDayHours...)
}

func parseEHRMSLeaveDetailDateTime(workDate string, value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if workDate == "" || value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006/01/02 15:04:05", "2006/01/02 15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, attendanceClockLocation); err == nil {
			return parsed, true
		}
	}
	for _, layout := range []string{"15:04:05", "15:04"} {
		if parsed, err := time.ParseInLocation(time.DateOnly+" "+layout, workDate+" "+value, attendanceClockLocation); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func parseEHRMSDeductMinutes(value string) int {
	match := ehrmsNumberPattern.FindString(strings.TrimSpace(value))
	if match == "" {
		return 0
	}
	number, err := strconv.ParseFloat(match, 64)
	if err != nil || number < 0 {
		return 0
	}
	lower := strings.ToLower(value)
	if strings.Contains(lower, "hour") || strings.Contains(value, "小時") || strings.Contains(value, "小时") {
		number *= 60
	}
	return int(math.Round(number))
}

func ehrmsPayloadHash(payload map[string]any) string {
	raw, _ := json.Marshal(payload)
	sum := sha1.Sum(raw)
	return fmt.Sprintf("%x", sum[:])
}

func ehrmsAttendanceTimeField(record domain.EHRMSAttendanceRecord, key string, rowNumber int, errors *[]RowError) string {
	raw := ehrmsAttendanceValue(record, key)
	value := normalizeEHRMSAttendanceTime(raw)
	if raw != "" && value == "" {
		*errors = append(*errors, RowError{Row: rowNumber, Field: key, Code: "invalid", Message: key + " must be HH:MM"})
	}
	return value
}

func ehrmsAttendancePayload(record domain.EHRMSAttendanceRecord) map[string]any {
	return ehrmsStringPayload(map[string]string(record))
}

func ehrmsStringPayload(record map[string]string) map[string]any {
	if len(record) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(record))
	for key, value := range record {
		out[key] = normalizeEHRMSPlaceholder(value)
	}
	return out
}

func ehrmsAttendanceValue(record domain.EHRMSAttendanceRecord, key string) string {
	if len(record) == 0 {
		return ""
	}
	if value := strings.TrimSpace(record[key]); value != "" {
		return value
	}
	switch key {
	case ehrmsAttendanceFieldEmployeeNo:
		return strings.TrimSpace(record["emp_id"])
	case ehrmsAttendanceFieldDate:
		return strings.TrimSpace(record["date"])
	case ehrmsAttendanceFieldShiftStart:
		return strings.TrimSpace(record["shift_start"])
	case ehrmsAttendanceFieldShiftEnd:
		return strings.TrimSpace(record["shift_end"])
	case ehrmsAttendanceFieldShiftHours:
		return strings.TrimSpace(record["shift_hours"])
	case ehrmsAttendanceFieldDailyHours:
		return strings.TrimSpace(record["daily_hours"])
	case ehrmsAttendanceFieldClockHours:
		return strings.TrimSpace(record["clock_hours"])
	case ehrmsAttendanceFieldClockStart:
		return strings.TrimSpace(record["clock_start"])
	case ehrmsAttendanceFieldClockEnd:
		return strings.TrimSpace(record["clock_end"])
	case ehrmsAttendanceFieldLeaveType:
		return strings.TrimSpace(record["leave_type"])
	case ehrmsAttendanceFieldLeaveStart:
		return strings.TrimSpace(record["leave_start"])
	case ehrmsAttendanceFieldLeaveEnd:
		return strings.TrimSpace(record["leave_end"])
	case ehrmsAttendanceFieldLeaveHours:
		return strings.TrimSpace(record["leave_hours"])
	case ehrmsAttendanceFieldLeaveCounted:
		return strings.TrimSpace(record["leave_counted"])
	case ehrmsAttendanceFieldLeave2Type:
		return strings.TrimSpace(record["leave2_type"])
	case ehrmsAttendanceFieldLeave2Start:
		return strings.TrimSpace(record["leave2_start"])
	case ehrmsAttendanceFieldLeave2End:
		return strings.TrimSpace(record["leave2_end"])
	case ehrmsAttendanceFieldLeave2Hours:
		return strings.TrimSpace(record["leave2_hours"])
	case ehrmsAttendanceFieldLeave2Counted:
		return strings.TrimSpace(record["leave2_counted"])
	default:
		return ""
	}
}

func ehrmsLeaveBalanceValue(record domain.EHRMSLeaveBalanceRecord, key string) string {
	if len(record) == 0 {
		return ""
	}
	if value := strings.TrimSpace(record[key]); value != "" {
		return value
	}
	switch key {
	case ehrmsLeaveBalanceFieldEmployeeNo:
		return strings.TrimSpace(record["emp_id"])
	case ehrmsLeaveBalanceFieldYear:
		return strings.TrimSpace(record["year"])
	case ehrmsLeaveBalanceFieldLeaveType:
		return strings.TrimSpace(record["leave_type"])
	case ehrmsLeaveBalanceFieldUnit:
		return strings.TrimSpace(record["unit"])
	case ehrmsLeaveBalanceFieldQuota:
		return strings.TrimSpace(record["quota"])
	case ehrmsLeaveBalanceFieldUsed:
		return strings.TrimSpace(record["used"])
	case ehrmsLeaveBalanceFieldRemaining:
		return strings.TrimSpace(record["remaining"])
	case ehrmsLeaveBalanceFieldLeaveCode:
		return strings.TrimSpace(record["leave_code"])
	case ehrmsLeaveBalanceFieldCategoryCode:
		return strings.TrimSpace(record["leave_category_code"])
	default:
		return ""
	}
}

func parseEHRMSLeaveBalanceYear(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	year, err := strconv.Atoi(value)
	if err != nil || year < 1 {
		return 0, false
	}
	return year, true
}

// isEHRMSAttendanceOnlyLeaveType rejects upstream attendance metrics that share the leave feeds.
func isEHRMSAttendanceOnlyLeaveType(value string) bool {
	_, excluded := ehrmsAttendanceOnlyLeaveTypes[strings.ToLower(strings.TrimSpace(value))]
	return excluded
}

func ehrmsLeaveDetailValue(record domain.EHRMSLeaveDetailRecord, key string) string {
	if len(record) == 0 {
		return ""
	}
	if value := strings.TrimSpace(record[key]); value != "" {
		return value
	}
	switch key {
	case ehrmsLeaveDetailFieldEmployeeNo:
		return strings.TrimSpace(record["emp_id"])
	case ehrmsLeaveDetailFieldDate:
		return strings.TrimSpace(record["date"])
	case ehrmsLeaveDetailFieldLeaveType:
		return strings.TrimSpace(record["leave_type"])
	case ehrmsLeaveDetailFieldStart:
		return strings.TrimSpace(record["start"])
	case ehrmsLeaveDetailFieldEnd:
		return strings.TrimSpace(record["end"])
	case ehrmsLeaveDetailFieldHours:
		return strings.TrimSpace(record["hours"])
	case ehrmsLeaveDetailFieldLeaveCode:
		return strings.TrimSpace(record["leave_code"])
	case ehrmsLeaveDetailFieldCategoryCode:
		return strings.TrimSpace(record["leave_category_code"])
	case ehrmsLeaveDetailFieldLeaveItem:
		return strings.TrimSpace(record["leave_item"])
	case ehrmsLeaveDetailFieldRemark:
		return strings.TrimSpace(record["remark"])
	case ehrmsLeaveDetailFieldSource:
		return strings.TrimSpace(record["source"])
	case ehrmsLeaveDetailFieldDeductItem:
		return strings.TrimSpace(record["deduct_item"])
	case ehrmsLeaveDetailFieldDeductHours:
		return strings.TrimSpace(record["deduct_hours"])
	default:
		return ""
	}
}

func ehrmsStableID(prefix string, parts ...string) string {
	h := sha1.Sum([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("%s-%x", strings.TrimSpace(prefix), h[:10])
}

// ehrmsFetchError 隱藏上游錯誤細節，並保留 scheduler 所需的暫時錯誤分類。
func ehrmsFetchError(label string, err error) *domain.AppError {
	appErr := domain.BadRequest("fetch eHRMS " + label + " failed")
	var temporary interface{ Temporary() bool }
	if errors.As(err, &temporary) && temporary.Temporary() {
		appErr.ReasonCode = "ehrms_temporary_failure"
	} else {
		appErr.ReasonCode = "ehrms_permanent_failure"
	}
	return appErr
}

// requireTenantWideEHRMSSyncScope prevents scoped grants from triggering tenant-wide upstream writes.
func requireTenantWideEHRMSSyncScope(decision CheckResult) error {
	scope := decision.EffectiveScope
	if scope == "" {
		scope = decision.Scope
	}
	switch scope {
	case "", ScopeAll, ScopeTenant, ScopeSystem:
		return nil
	default:
		return ForbiddenDataScope("tenant-wide eHRMS sync requires all-tenant access")
	}
}
