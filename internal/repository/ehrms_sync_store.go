package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

// EHRMSSyncStore 持久化同步運行狀態並提供同租戶同步互斥。
type EHRMSSyncStore interface {
	UpsertEHRMSSyncRun(context.Context, domain.EHRMSSyncRun) error
	UpsertEHRMSSyncRunStep(context.Context, domain.EHRMSSyncRunStep) error
	GetEHRMSSyncRun(context.Context, string, string) (domain.EHRMSSyncRun, bool, error)
	ListEHRMSSyncRuns(context.Context, string, domain.PageRequest) ([]domain.EHRMSSyncRun, int, error)
	ListEHRMSSyncRunSteps(context.Context, string, string) ([]domain.EHRMSSyncRunStep, error)
	WithEHRMSSyncLock(context.Context, string, string, func() error) (bool, error)
}
