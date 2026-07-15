package v1_test

import (
	"errors"
	"testing"

	v1 "nexus-pro-be/internal/api/v1"
	"nexus-pro-be/internal/domain"
)

// TestBusinessRouteErrorMapsGenericCodes verifies module ownership without changing HTTP semantics.
func TestBusinessRouteErrorMapsGenericCodes(t *testing.T) {
	tests := []struct {
		name     string
		resource string
		err      error
		want     domain.ErrorCode
	}{
		{name: "attendance bad request", resource: "attendance.clock", err: domain.BadRequest("direction is required"), want: domain.ErrorCodeAttendanceBadRequest},
		{name: "workflow not found", resource: "workflow.form_instance", err: domain.NotFound("form instance", "form-1"), want: domain.ErrorCodeWorkflowNotFound},
		{name: "agent conflict", resource: "agent.model", err: domain.Conflict("model changed"), want: domain.ErrorCodeAgentConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapped := v1.BusinessRouteError(tt.resource, tt.err)
			appErr, ok := domain.AsAppError(mapped)
			if !ok || appErr.NumericCode() != tt.want {
				t.Fatalf("expected code %d, got %#v", tt.want, mapped)
			}
		})
	}
}

// TestBusinessRouteErrorPreservesSpecificAndUnknownErrors verifies specific codes and raw failures keep their safety behavior.
func TestBusinessRouteErrorPreservesSpecificAndUnknownErrors(t *testing.T) {
	specific := domain.Conflict("model is in use").WithReasonCode("agent_model_in_use")
	mapped := v1.BusinessRouteError("agent.model", specific)
	appErr, ok := domain.AsAppError(mapped)
	if !ok || appErr.NumericCode() != domain.ErrorCodeAgentModelInUse {
		t.Fatalf("specific code was overwritten: %#v", mapped)
	}

	raw := errors.New("database unavailable")
	if got := v1.BusinessRouteError("agent.model", raw); got != raw {
		t.Fatalf("raw system error must remain unchanged, got %#v", got)
	}
}
