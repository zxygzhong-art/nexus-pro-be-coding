package service

import (
	"errors"

	"nexus-pro-be/internal/domain"
)

// ehrmsFetchError 隱藏上游錯誤細節，並保留 scheduler 所需的暫時錯誤分類。
func ehrmsFetchError(label string, err error) *domain.AppError {
	appErr := domain.BadRequest("fetch eHRMS " + label + " failed")
	var temporary interface{ Temporary() bool }
	if errors.As(err, &temporary) && temporary.Temporary() {
		appErr.ReasonCode = "ehrms_temporary_failure"
	} else {
		appErr.ReasonCode = "ehrms_permanent_failure"
	}
	return appErr
}
