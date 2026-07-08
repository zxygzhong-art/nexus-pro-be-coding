package workflows

import (
	"errors"
	"testing"

	"nexus-pro-be/internal/domain"

	"go.temporal.io/sdk/temporal"
)

func TestNonRetryableActivityErrorMarksNotFound(t *testing.T) {
	wrapped := nonRetryableActivityError(domain.NotFound("form instance", "fi-1"))
	var appErr *temporal.ApplicationError
	if !errors.As(wrapped, &appErr) || !appErr.NonRetryable() {
		t.Fatalf("expected non-retryable application error, got %T %v", wrapped, wrapped)
	}
}

func TestNonRetryableActivityErrorPassesThroughOtherErrors(t *testing.T) {
	original := errors.New("transient")
	if nonRetryableActivityError(original) != original {
		t.Fatal("expected transient errors to pass through unchanged")
	}
}
