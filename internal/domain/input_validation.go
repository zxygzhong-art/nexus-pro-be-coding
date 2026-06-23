package domain

import "strings"

// Validate enforces employee creation rules shared by API and service paths.
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

// Validate enforces direct employee status update rules.
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

// Validate enforces employee status transition rules.
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
