package domain_test

import (
	"testing"

	"nexus-pro-api/internal/domain"
)

// TestEmployeeBasicInfoOnlyUpdateValidation 驗證員工管理編輯欄位邊界。
func TestEmployeeBasicInfoOnlyUpdateValidation(t *testing.T) {
	employeeNo := "E002"
	name := "After Name"
	email := "after@example.com"
	allowed := domain.UpdateEmployeeInput{
		EmployeeNo:   &employeeNo,
		Name:         &name,
		CompanyEmail: &email,
		BasicInfo:    map[string]any{"nationality_type": "local", "national_id": "B123456789"},
	}
	if err := allowed.ValidateBasicInfoOnly(); err != nil {
		t.Fatalf("expected basic information update to pass, got %v", err)
	}

	position := "Manager"
	phone := "0912345678"
	err := (domain.UpdateEmployeeInput{Position: &position, Phone: &phone}).ValidateBasicInfoOnly()
	appErr, ok := domain.AsAppError(err)
	if !ok || len(appErr.FieldErrors) != 2 || appErr.FieldErrors[0].Code != "basic_info_only" {
		t.Fatalf("expected basic_info_only field errors, got %+v", err)
	}
}
