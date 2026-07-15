package workflows_test

import (
	"errors"
	"testing"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/workflows"

	"go.temporal.io/sdk/temporal"
)

func TestNonRetryableActivityErrorMarksNotFound(t *testing.T) {
	wrapped := workflows.NonRetryableActivityError(domain.NotFound("form instance", "fi-1"))
	var appErr *temporal.ApplicationError
	if !errors.As(wrapped, &appErr) || !appErr.NonRetryable() {
		t.Fatalf("expected non-retryable application error, got %T %v", wrapped, wrapped)
	}
}

func TestNonRetryableActivityErrorMarksBusinessRejections(t *testing.T) {
	for _, original := range []error{
		domain.Forbidden("current account is not an active assignee"),
		domain.Conflict("workflow stage changed"),
	} {
		wrapped := workflows.NonRetryableActivityError(original)
		var appErr *temporal.ApplicationError
		if !errors.As(wrapped, &appErr) || !appErr.NonRetryable() {
			t.Fatalf("expected non-retryable business error for %v, got %T %v", original, wrapped, wrapped)
		}
	}
}

func TestNonRetryableActivityErrorPassesThroughOtherErrors(t *testing.T) {
	original := errors.New("transient")
	if workflows.NonRetryableActivityError(original) != original {
		t.Fatal("expected transient errors to pass through unchanged")
	}
}
