package service

import "strings"

func (c *Service) employeeFromCreateInput(ctx RequestContext, input CreateEmployeeInput, reservedEmployeeNos ...map[string]struct{}) (Employee, error) {
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
	if err := c.validateEmployee(ctx, employee, "create"); err != nil {
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

func (c *Service) employeeCreateCandidate(ctx RequestContext, input CreateEmployeeInput) (Employee, error) {
	now := c.Now()
	hireDate, err := optionalDateTime(input.HireDate)
	if err != nil {
		return Employee{}, BadRequest("hire_date must be RFC3339 or YYYY-MM-DD")
	}
	resignDate, err := optionalDateTime(input.ResignDate)
	if err != nil {
		return Employee{}, BadRequest("resign_date must be RFC3339 or YYYY-MM-DD")
	}
	status := normalizeEmployeeStatus(firstNonEmpty(input.EmploymentStatus, input.Status, string(EmployeeStatusActive)))
	employee := Employee{
		ID:                    newID("emp"),
		TenantID:              ctx.TenantID,
		EmployeeNo:            strings.TrimSpace(input.EmployeeNo),
		Name:                  strings.TrimSpace(input.Name),
		CompanyEmail:          strings.TrimSpace(input.CompanyEmail),
		PersonalEmail:         strings.TrimSpace(input.PersonalEmail),
		Phone:                 strings.TrimSpace(input.Phone),
		OrgUnitID:             strings.TrimSpace(input.OrgUnitID),
		AccountID:             strings.TrimSpace(input.AccountID),
		ManagerEmployeeID:     strings.TrimSpace(input.ManagerEmployeeID),
		Position:              strings.TrimSpace(input.Position),
		Category:              normalizeEmployeeCategory(input.Category),
		Status:                status,
		EmploymentStatus:      status,
		HireDate:              hireDate,
		ResignDate:            resignDate,
		BasicInfo:             copyStringMap(input.BasicInfo),
		EmploymentInfo:        copyStringMap(input.EmploymentInfo),
		EducationMilitaryInfo: copyStringMap(input.EducationMilitaryInfo),
		ContactInfo:           copyStringMap(input.ContactInfo),
		InsuranceInfo:         copyStringMap(input.InsuranceInfo),
		InternalExperiences:   copyEmployeeExperiences(input.InternalExperiences),
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	return c.deriveEmployeeHotFields(employee), nil
}

func (c *Service) applyEmployeePatch(ctx RequestContext, employee *Employee, input UpdateEmployeeInput) error {
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
	if input.Position != nil {
		employee.Position = strings.TrimSpace(*input.Position)
	}
	if input.Category != nil {
		employee.Category = normalizeEmployeeCategory(*input.Category)
	}
	if input.Status != nil {
		employee.Status = normalizeEmployeeStatus(*input.Status)
	}
	if input.EmploymentStatus != nil {
		employee.EmploymentStatus = normalizeEmployeeStatus(*input.EmploymentStatus)
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
	employee.EducationMilitaryInfo = mergeMap(employee.EducationMilitaryInfo, input.EducationMilitaryInfo)
	employee.ContactInfo = mergeMap(employee.ContactInfo, input.ContactInfo)
	employee.InsuranceInfo = mergeMap(employee.InsuranceInfo, input.InsuranceInfo)
	if input.InternalExperiences != nil {
		employee.InternalExperiences = copyEmployeeExperiences(input.InternalExperiences)
	}
	*employee = c.deriveEmployeeHotFields(*employee)
	employee.UpdatedAt = c.Now()
	return c.validateEmployee(ctx, *employee, "update")
}

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
		fields = append(fields, FieldError{Tab: tab, Field: field, Code: "field_policy_denied", Message: field + " cannot be updated by current permission policy"})
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

func addPatchMapFields(fields *[]FieldError, policies map[string]string, tab string, values map[string]any) {
	if len(values) == 0 {
		return
	}
	if effect := policies[tab]; effect == "deny" || effect == "hide" || effect == "readonly" {
		*fields = append(*fields, FieldError{Tab: tab, Field: tab, Code: "field_policy_denied", Message: tab + " cannot be updated by current permission policy"})
	}
	for field := range values {
		effect := policies[field]
		if effect != "deny" && effect != "hide" && effect != "readonly" {
			continue
		}
		*fields = append(*fields, FieldError{Tab: tab, Field: field, Code: "field_policy_denied", Message: field + " cannot be updated by current permission policy"})
	}
}

func (c *Service) deriveEmployeeHotFields(employee Employee) Employee {
	employee.CompanyEmail = firstNonEmpty(employee.CompanyEmail, employeeHotValue(employee, "company_email"))
	employee.PersonalEmail = firstNonEmpty(employee.PersonalEmail, employeeHotValue(employee, "personal_email"))
	employee.Phone = firstNonEmpty(employee.Phone, employeeHotValue(employee, "phone"))
	employee.OrgUnitID = firstNonEmpty(employee.OrgUnitID, employeeHotValue(employee, "org_unit_id"))
	employee.ManagerEmployeeID = firstNonEmpty(employee.ManagerEmployeeID, employeeHotValue(employee, "manager_employee_id"))
	employee.Position = firstNonEmpty(employee.Position, employeeHotValue(employee, "position"))
	employee.Category = normalizeEmployeeCategory(firstNonEmpty(employee.Category, employeeHotValue(employee, "category")))
	employee.Name = firstNonEmpty(employee.Name, employeeHotValue(employee, "name"), strings.TrimSpace(stringFromMap(employee.BasicInfo, "first_name")+" "+stringFromMap(employee.BasicInfo, "last_name")))
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

func (c *Service) validateEmployee(ctx RequestContext, employee Employee, mode string) error {
	fields := make([]FieldError, 0)
	if strings.TrimSpace(employee.Name) == "" {
		fields = append(fields, FieldError{Tab: "basic_info", Field: "name", Code: "required", Message: "name is required"})
	}
	if strings.TrimSpace(employee.CompanyEmail) == "" {
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
	if status == string(EmployeeStatusResigned) {
		if employee.ResignDate == nil && stringFromMap(employee.EmploymentInfo, "resign_date") == "" {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "resign_date", Code: "required", Message: "resign_date is required"})
		}
		if stringFromMap(employee.EmploymentInfo, "resign_reason") == "" && mode == "transition" {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "resign_reason", Code: "required", Message: "resign_reason is required"})
		}
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

func (c *Service) employeeUniqueFieldErrors(ctx RequestContext, store employeeUniqueLookupStore, employee Employee) ([]FieldError, error) {
	fields := make([]FieldError, 0, 3)
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
	if employee.AccountID != "" {
		existing, ok, err := store.GetEmployeeByAccountID(goCtx, ctx.TenantID, employee.AccountID)
		if err != nil {
			return nil, err
		}
		if ok && existing.ID != employee.ID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "account_id", Code: "unique", Message: "account_id already linked"})
		}
	}
	return fields, nil
}

func (c *Service) employeeUniqueFieldErrorsFromList(ctx RequestContext, employee Employee) ([]FieldError, error) {
	existingEmployees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
	if err != nil {
		return nil, err
	}
	fields := make([]FieldError, 0, 3)
	for _, existing := range existingEmployees {
		if existing.ID == employee.ID {
			continue
		}
		if employee.EmployeeNo != "" && existing.EmployeeNo == employee.EmployeeNo {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "employee_no", Code: "unique", Message: "employee_no already exists"})
		}
		if employee.CompanyEmail != "" && existing.CompanyEmail == employee.CompanyEmail {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "company_email", Code: "unique", Message: "company_email already exists"})
		}
		if employee.AccountID != "" && existing.AccountID == employee.AccountID {
			fields = append(fields, FieldError{Tab: "basic_info", Field: "account_id", Code: "unique", Message: "account_id already linked"})
		}
	}
	return fields, nil
}

func (c *Service) generateEmployeeNo(ctx RequestContext, reservedEmployeeNos ...map[string]struct{}) (string, error) {
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

func stringFromMap(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	if v, ok := values[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func mergeMap(base map[string]any, patch map[string]any) map[string]any {
	if len(base) == 0 && len(patch) == 0 {
		return nil
	}
	out := copyStringMap(base)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range patch {
		out[key] = value
	}
	return out
}

func (c *Service) newEmployeeExperience(employee Employee, reason string) EmployeeExperience {
	return EmployeeExperience{
		ID:                newID("ehist"),
		StartDate:         employee.HireDate,
		Reason:            firstNonEmpty(reason, "資料更新"),
		OrgUnitID:         employee.OrgUnitID,
		ManagerEmployeeID: employee.ManagerEmployeeID,
		Position:          employee.Position,
		Category:          employee.Category,
		Current:           true,
		CreatedAt:         c.Now(),
	}
}

func (c *Service) appendHistoryForChangedEmployment(before, after Employee, reason string) Employee {
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

func (c *Service) touchEmployeeAuthzIfNeeded(ctx RequestContext, before, after Employee, eventType string) error {
	if before.OrgUnitID == after.OrgUnitID && before.AccountID == after.AccountID && before.ManagerEmployeeID == after.ManagerEmployeeID {
		return nil
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

func (c *Service) linkEmployeeAccount(ctx RequestContext, employee Employee) error {
	if employee.AccountID == "" {
		return nil
	}
	account, ok, err := c.store.GetAccount(goContext(ctx), ctx.TenantID, employee.AccountID)
	if err != nil {
		return err
	}
	if ok {
		account.EmployeeID = employee.ID
		account.DisplayName = firstNonEmpty(account.DisplayName, employee.Name)
		account.Email = firstNonEmpty(account.Email, employee.CompanyEmail)
		return c.store.UpsertAccount(goContext(ctx), account)
	}
	return nil
}

func (c *Service) appendEmployeeEvent(ctx RequestContext, eventType, target string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["target"] = target
	return c.store.AppendAuthzOutboxEvent(goContext(ctx), AuthzOutboxEvent{
		ID:         newID("outbox"),
		TenantID:   ctx.TenantID,
		EventType:  eventType,
		Payload:    payload,
		Status:     "pending",
		RetryCount: 0,
		CreatedAt:  c.Now(),
	})
}

func domainValidation(message string, fields ...FieldError) error {
	return ValidationFailed(message, fields)
}
