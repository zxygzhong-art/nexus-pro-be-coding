package domain_test

import (
	"testing"

	"nexus-pro-be/internal/domain"
)

func TestValidateFormApprovalWorkflowStartRequiresIDs(t *testing.T) {
	if err := domain.ValidateFormApprovalWorkflowStart(domain.FormApprovalWorkflowStart{}); err == nil {
		t.Fatal("expected validation error for empty start input")
	}
	if err := domain.ValidateFormApprovalWorkflowStart(domain.FormApprovalWorkflowStart{
		TenantID: "demo",
	}); err == nil {
		t.Fatal("expected validation error for missing form_instance_id")
	}
	if err := domain.ValidateFormApprovalWorkflowStart(domain.FormApprovalWorkflowStart{
		FormInstanceID: "fi-1",
	}); err == nil {
		t.Fatal("expected validation error for missing tenant_id")
	}
	if err := domain.ValidateFormApprovalWorkflowStart(domain.FormApprovalWorkflowStart{
		TenantID:       "demo",
		FormInstanceID: "fi-1",
	}); err != nil {
		t.Fatalf("expected valid start input, got %v", err)
	}
}
