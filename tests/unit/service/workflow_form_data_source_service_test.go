package service_test

import (
	"testing"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// TestValidateFormFieldBindingRejectsUnsupportedSources verifies persisted bindings are allowlisted.
func TestValidateFormFieldBindingRejectsUnsupportedSources(t *testing.T) {
	errors := service.ValidateFormFieldBinding("employee", "select", domain.PlatformFormBuilderFieldBinding{
		SourceID: "arbitrary_url", ValueField: "id", LabelField: "name",
	})
	if len(errors) != 1 || errors[0].Code != "invalid" {
		t.Fatalf("expected unsupported source validation error, got %+v", errors)
	}
}

// TestValidateAndResolveBoundSubmissionValue ensures object values are server-owned and collection values are allowlisted.
func TestValidateAndResolveBoundSubmissionValue(t *testing.T) {
	catalog := domain.FormDataSourceCatalogResponse{DataSources: []domain.FormDataSource{
		{ID: "current_user", Kind: "object", Records: []map[string]interface{}{{"employee_no": "E-001"}}},
		{ID: "departments", Kind: "collection", Records: []map[string]interface{}{{"id": "dept-1", "name": "產品部"}}},
	}}

	resolved, exists, message := service.ValidateAndResolveBoundSubmissionValue(catalog, domain.PlatformFormBuilderFieldBinding{
		SourceID: "current_user", ValueField: "employee_no",
	}, "tampered", true)
	if message != "" || !exists || resolved != "E-001" {
		t.Fatalf("expected server value to replace client value, got value=%v exists=%v message=%q", resolved, exists, message)
	}

	_, _, message = service.ValidateAndResolveBoundSubmissionValue(catalog, domain.PlatformFormBuilderFieldBinding{
		SourceID: "departments", ValueField: "id", LabelField: "name",
	}, "dept-unknown", true)
	if message == "" {
		t.Fatal("expected unknown collection option to be rejected")
	}
}
