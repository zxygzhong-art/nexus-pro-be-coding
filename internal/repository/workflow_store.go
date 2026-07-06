package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

// WorkflowStore 定義流程與運行時儲存層的行為契約。
type WorkflowStore interface {
	UpsertFormTemplate(context.Context, domain.FormTemplate) error
	GetFormTemplate(ctx context.Context, tenantID, id string) (domain.FormTemplate, bool, error)
	GetFormTemplateByKey(ctx context.Context, tenantID, key string) (domain.FormTemplate, bool, error)
	ListFormTemplates(ctx context.Context, tenantID string) ([]domain.FormTemplate, error)

	UpsertFormInstance(context.Context, domain.FormInstance) error
	GetFormInstance(ctx context.Context, tenantID, id string) (domain.FormInstance, bool, error)
	ListFormInstances(ctx context.Context, tenantID string) ([]domain.FormInstance, error)
	ListFormInstancesByQuery(ctx context.Context, tenantID string, query domain.FormInstanceQuery) ([]domain.FormInstance, error)
	ListFormInstancePageByQuery(ctx context.Context, tenantID string, query domain.FormInstanceQuery, page domain.PageRequest) ([]domain.FormInstance, int, error)
	DeleteFormInstance(ctx context.Context, tenantID, id string) error

	UpsertWorkflowRun(context.Context, domain.WorkflowRun) error
	GetWorkflowRun(ctx context.Context, tenantID, id string) (domain.WorkflowRun, bool, error)
	GetWorkflowRunByFormInstance(ctx context.Context, tenantID, formInstanceID string) (domain.WorkflowRun, bool, error)
	ListWorkflowRunsByFormInstance(ctx context.Context, tenantID, formInstanceID string) ([]domain.WorkflowRun, error)

	UpsertWorkflowStageInstance(context.Context, domain.WorkflowStageInstance) error
	GetWorkflowStageInstance(ctx context.Context, tenantID, id string) (domain.WorkflowStageInstance, bool, error)
	ListWorkflowStageInstancesByRun(ctx context.Context, tenantID, runID string) ([]domain.WorkflowStageInstance, error)

	UpsertWorkflowStageAssignee(context.Context, domain.WorkflowStageAssignee) error
	ListWorkflowStageAssignees(ctx context.Context, tenantID, stageInstanceID string) ([]domain.WorkflowStageAssignee, error)
	ListPendingAssigneeStageInstanceIDs(ctx context.Context, tenantID, accountID string) ([]string, error)

	InsertWorkflowAction(context.Context, domain.WorkflowAction) error
	ListWorkflowActionsByRun(ctx context.Context, tenantID, runID string) ([]domain.WorkflowAction, error)
}
