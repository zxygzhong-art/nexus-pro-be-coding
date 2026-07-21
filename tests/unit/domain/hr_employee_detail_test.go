package domain_test

import (
	"encoding/json"
	"testing"

	"nexus-pro-api/internal/domain"
)

func TestEmployeeDetailEmptyInternalExperiencesMarshalAsArray(t *testing.T) {
	detail := domain.EmployeeDetailFromEmployee(domain.Employee{
		ID:       "emp-1",
		TenantID: "tenant-1",
		Name:     "Employee One",
		Status:   "active",
	})

	payload, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Sections struct {
			InternalExperiences json.RawMessage `json:"internal_experiences"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	if string(response.Sections.InternalExperiences) != "[]" {
		t.Fatalf("expected employee detail sections to encode empty internal_experiences as [], got %s", payload)
	}
}
