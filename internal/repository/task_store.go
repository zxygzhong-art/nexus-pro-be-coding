package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

// TaskStore 定義任務儲存層的行為契約。
type TaskStore interface {
	UpsertPlatformTaskItem(context.Context, domain.PlatformTaskRecordItem) error
	GetPlatformTaskItem(ctx context.Context, tenantID, accountID, id string) (domain.PlatformTaskRecordItem, bool, error)
	ListPlatformTaskItems(ctx context.Context, tenantID, accountID string) ([]domain.PlatformTaskRecordItem, error)
	DeletePlatformTaskItem(ctx context.Context, tenantID, accountID, id string) error
	UpsertPlatformTaskTodo(context.Context, domain.PlatformTaskTodoRecord) error
	GetPlatformTaskTodo(ctx context.Context, tenantID, accountID, id string) (domain.PlatformTaskTodoRecord, bool, error)
	ListPlatformTaskTodos(ctx context.Context, tenantID, accountID string) ([]domain.PlatformTaskTodoRecord, error)
	DeletePlatformTaskTodo(ctx context.Context, tenantID, accountID, id string) error
}
