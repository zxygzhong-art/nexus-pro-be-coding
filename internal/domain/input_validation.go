package domain

import "strings"

type ValidatedInput interface {
	Validate() error
}

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
	if len(fields) > 0 {
		return ValidationFailed("employee input validation failed", fields)
	}
	return nil
}

func (in UpdateEmployeeStatusInput) Validate() error {
	status := strings.TrimSpace(in.Status)
	if status == "" {
		return ValidationFailed("employee status validation failed", []FieldError{{Tab: "employment_info", Field: "status", Code: "required", Message: "status is required"}})
	}
	parsed, ok := ParseEmployeeStatus(status)
	if !ok || !parsed.Valid(false) {
		return ValidationFailed("employee status validation failed", []FieldError{{Tab: "employment_info", Field: "status", Code: "invalid", Message: "status must be one of active, probation, leave_suspended, onboarding or resigned"}})
	}
	return nil
}

func (in StatusTransitionInput) Validate() error {
	status := strings.TrimSpace(in.Status)
	if status == "" {
		return ValidationFailed("employee status transition validation failed", []FieldError{{Tab: "employment_info", Field: "status", Code: "required", Message: "status is required"}})
	}
	parsed, ok := ParseEmployeeStatus(status)
	if !ok || !parsed.Valid(false) {
		return ValidationFailed("employee status transition validation failed", []FieldError{{Tab: "employment_info", Field: "status", Code: "invalid", Message: "status must be one of active, probation, leave_suspended, onboarding or resigned"}})
	}
	return nil
}
