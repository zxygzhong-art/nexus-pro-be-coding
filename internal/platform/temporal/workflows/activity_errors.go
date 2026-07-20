package workflows

import (
	"nexus-pro-api/internal/domain"

	"go.temporal.io/sdk/temporal"
)

// NonRetryableActivityError converts permanent domain failures into non-retryable Temporal errors.
func NonRetryableActivityError(err error) error {
	if err == nil {
		return nil
	}
	appErr, ok := domain.AsAppError(err)
	if !ok {
		return err
	}
	switch appErr.Status {
	case 400, 401, 403, 404, 409, 422:
		return temporal.NewNonRetryableApplicationError(appErr.Message, appErr.Code, err)
	default:
		return err
	}
}
