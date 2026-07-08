package workflows

import (
	"nexus-pro-be/internal/domain"

	"go.temporal.io/sdk/temporal"
)

// nonRetryableActivityError marks client/data errors that should not be retried by Temporal.
func nonRetryableActivityError(err error) error {
	if err == nil {
		return nil
	}
	appErr, ok := domain.AsAppError(err)
	if !ok {
		return err
	}
	switch appErr.Status {
	case 400, 404:
		return temporal.NewNonRetryableApplicationError(appErr.Message, appErr.Code, err)
	default:
		return err
	}
}
