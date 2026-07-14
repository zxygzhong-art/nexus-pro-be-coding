package domain

import "strings"

// Validate rejects oversized self-service profile values while allowing fields to be cleared.
func (in UpdateMeProfileInput) Validate() error {
	if in.EnglishName == nil && in.MobilePhone == nil && in.Extension == nil && in.Slack == nil && in.EmergencyContactName == nil {
		return ValidationFailed("profile validation failed", []FieldError{{Field: "profile", Code: "at_least_one", Message: "at least one profile field is required"}})
	}
	fields := make([]FieldError, 0)
	validateOptionalProfileLength := func(field string, value *string, max int) {
		if value != nil && len([]rune(strings.TrimSpace(*value))) > max {
			fields = append(fields, FieldError{Field: field, Code: "max_length", Message: field + " exceeds maximum length"})
		}
	}
	validateOptionalProfileLength("english_name", in.EnglishName, 100)
	validateOptionalProfileLength("mobile_phone", in.MobilePhone, 32)
	validateOptionalProfileLength("extension", in.Extension, 16)
	validateOptionalProfileLength("slack", in.Slack, 80)
	validateOptionalProfileLength("emergency_contact_name", in.EmergencyContactName, 100)
	if len(fields) > 0 {
		return ValidationFailed("profile validation failed", fields)
	}
	return nil
}

// Validate 驗證目前流程。
func (in CreateEmployeeInput) Validate() error {
	fields := make([]FieldError, 0)
	if strings.TrimSpace(in.Name) == "" {
		fields = append(fields, FieldError{Tab: "basic_info", Field: "name", Code: "required", Message: "name is required"})
	}
	if strings.TrimSpace(in.CompanyEmail) == "" {
		fields = append(fields, FieldError{Tab: "basic_info", Field: "company_email", Code: "required", Message: "company_email is required"})
	}
	if in.Status != "" {
		if status, ok := ParseEmployeeStatus(in.Status); !ok || !status.Valid(true) {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "status", Code: "invalid", Message: "status must be one of active, probation, leave_suspended, onboarding, resigned, deleted"})
		}
	}
	if in.EmploymentStatus != "" {
		if status, ok := ParseEmployeeStatus(in.EmploymentStatus); !ok || !status.Valid(true) {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "employment_status", Code: "invalid", Message: "employment_status must be one of active, probation, leave_suspended, onboarding, resigned, deleted"})
		}
	}
	if in.Category != "" {
		if category, ok := ParseEmployeeCategory(in.Category); !ok || !category.Valid() {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "category", Code: "invalid", Message: "category must be one of full_time, part_time, intern, contractor, other"})
		}
	}
	if strings.TrimSpace(in.AccountPolicy) != "" {
		if policy, ok := ParseEmployeeAccountPolicy(in.AccountPolicy); !ok {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "account_policy", Code: "invalid", Message: "account_policy must be one of none, link_existing, create_pending_invite, create_active"})
		} else {
			switch policy {
			case EmployeeAccountPolicyLinkExisting:
				if strings.TrimSpace(in.AccountID) == "" {
					fields = append(fields, FieldError{Tab: "employment_info", Field: "account_id", Code: "required", Message: "account_id is required when account_policy is link_existing"})
				}
			case EmployeeAccountPolicyNone, EmployeeAccountPolicyCreatePendingInvite, EmployeeAccountPolicyCreateActive:
				if strings.TrimSpace(in.AccountID) != "" {
					fields = append(fields, FieldError{Tab: "employment_info", Field: "account_id", Code: "invalid", Message: "account_id is only allowed when account_policy is link_existing"})
				}
			}
		}
	}
	if identityType := mapString(in.BasicInfo, "nationality_type"); identityType == "foreign" {
		for _, field := range []string{"passport_no", "passport_name", "entry_date", "arc_no", "arc_expiry_date", "tax_id", "work_permit_no", "work_permit_expiry_date", "contract_expiry_date", "broker"} {
			if mapString(in.BasicInfo, field) == "" {
				fields = append(fields, FieldError{Tab: "basic_info", Field: field, Code: "required", Message: field + " is required for foreign employees"})
			}
		}
	} else if identityType != "" && mapString(in.BasicInfo, "national_id") == "" {
		fields = append(fields, FieldError{Tab: "basic_info", Field: "national_id", Code: "required", Message: "national_id is required for local employees"})
	}
	if status, ok := ParseEmployeeStatus(firstNonEmpty(in.EmploymentStatus, in.Status)); ok && status == EmployeeStatusResigned {
		if strings.TrimSpace(in.ResignDate) == "" && mapString(in.EmploymentInfo, "resign_date") == "" {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "resign_date", Code: "required", Message: "resign_date is required"})
		}
	}
	if len(fields) > 0 {
		return ValidationFailed("employee input validation failed", fields)
	}
	return nil
}

// ValidateBasicInfoOnly 驗證員工管理編輯只包含基本資料欄位。
func (in UpdateEmployeeInput) ValidateBasicInfoOnly() error {
	fields := make([]FieldError, 0)
	add := func(tab, field string, present bool) {
		if present {
			fields = append(fields, FieldError{Tab: tab, Field: field, Code: "basic_info_only", Message: field + " cannot be updated from employee edit"})
		}
	}
	add("basic_info", "personal_email", in.PersonalEmail != nil)
	add("contact_info", "phone", in.Phone != nil)
	add("basic_info", "account_id", in.AccountID != nil)
	add("employment_info", "org_unit_id", in.OrgUnitID != nil)
	add("employment_info", "manager_employee_id", in.ManagerEmployeeID != nil)
	add("employment_info", "position_id", in.PositionID != nil)
	add("employment_info", "position", in.Position != nil)
	add("employment_info", "category", in.Category != nil)
	add("employment_info", "status", in.Status != nil)
	add("employment_info", "employment_status", in.EmploymentStatus != nil)
	add("employment_info", "hire_date", in.HireDate != nil)
	add("employment_info", "resign_date", in.ResignDate != nil)
	add("employment_info", "employment_info", in.EmploymentInfo != nil)
	add("education_military_info", "education_military_info", in.EducationMilitaryInfo != nil)
	add("contact_info", "contact_info", in.ContactInfo != nil)
	add("insurance_info", "insurance_info", in.InsuranceInfo != nil)
	add("employment_info", "internal_experiences", in.InternalExperiences != nil)
	if len(fields) > 0 {
		return ValidationFailed("employee edit only supports basic information", fields)
	}
	return nil
}

// Validate 驗證目前流程。
func (in UpdateEmployeeStatusInput) Validate() error {
	if err := validateEmployeeStatusInput(in.Status, "employee status validation failed"); err != nil {
		return err
	}
	parsed, _ := ParseEmployeeStatus(in.Status)
	if parsed == EmployeeStatusLeaveSuspended {
		return ValidationFailed("employee status validation failed", []FieldError{{Tab: "employment_info", Field: "status", Code: "transition_required", Message: "leave_suspended requires status-transition"}})
	}
	return nil
}

// Validate 驗證目前流程。
func (in StatusTransitionInput) Validate() error {
	if err := validateEmployeeStatusInput(in.Status, "employee status transition validation failed"); err != nil {
		return err
	}
	parsed, _ := ParseEmployeeStatus(in.Status)
	fields := make([]FieldError, 0, 3)
	switch parsed {
	case EmployeeStatusLeaveSuspended:
		if strings.TrimSpace(in.Reason) == "" {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "reason", Code: "required", Message: "reason is required"})
		}
		if strings.TrimSpace(in.StartDate) == "" {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "start_date", Code: "required", Message: "start_date is required"})
		}
		if strings.TrimSpace(in.EndDate) == "" {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "end_date", Code: "required", Message: "end_date is required"})
		}
	case EmployeeStatusResigned:
		if strings.TrimSpace(in.Reason) == "" {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "reason", Code: "required", Message: "reason is required"})
		}
		if strings.TrimSpace(in.EndDate) == "" {
			fields = append(fields, FieldError{Tab: "employment_info", Field: "end_date", Code: "required", Message: "end_date is required"})
		}
	}
	if len(fields) > 0 {
		return ValidationFailed("employee status transition validation failed", fields)
	}
	return nil
}

// validateEmployeeStatusInput 驗證員工狀態輸入。
func validateEmployeeStatusInput(rawStatus, message string) error {
	status := strings.TrimSpace(rawStatus)
	if status == "" {
		return ValidationFailed(message, []FieldError{{Tab: "employment_info", Field: "status", Code: "required", Message: "status is required"}})
	}
	parsed, ok := ParseEmployeeStatus(status)
	if !ok || !parsed.Valid(false) {
		return ValidationFailed(message, []FieldError{{Tab: "employment_info", Field: "status", Code: "invalid", Message: "status must be one of active, probation, leave_suspended, onboarding or resigned"}})
	}
	return nil
}

// mapString 映射字串。
func mapString(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	value, ok := values[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

// firstNonEmpty 取得第一個non 空值。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
