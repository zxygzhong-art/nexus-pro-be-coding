package service_test

import (
	"testing"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// TestEHRMSMergePreservesSelfManagedEnglishName verifies upstream sync cannot undo a user-selected English name.
func TestEHRMSMergePreservesSelfManagedEnglishName(t *testing.T) {
	existing := domain.Employee{BasicInfo: map[string]any{"name_en": "Alice", "name_en_source": "self"}}
	candidate := domain.Employee{BasicInfo: map[string]any{"name_en": "Upstream Alice", "source": "ehrms"}}

	merged := service.EHRMSMergeEmployee(existing, candidate)
	if merged.BasicInfo["name_en"] != "Alice" || merged.BasicInfo["name_en_source"] != "self" {
		t.Fatalf("self-managed English name was overwritten: %+v", merged.BasicInfo)
	}
}
