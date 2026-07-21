package repository

import (
	"context"
	"time"

	"nexus-pro-api/internal/domain"
)

// WorkflowStore 定義流程與運行時儲存層的行為契約。
type WorkflowStore interface {
	UpsertFormDefinitionDraft(context.Context, domain.FormDefinitionDraft) error
	GetFormDefinitionDraft(ctx context.Context, tenantID, id string) (domain.FormDefinitionDraft, bool, error)
	GetFormDefinitionDraftByAgentCall(ctx context.Context, tenantID, agentRunID, toolCallID string) (domain.FormDefinitionDraft, bool, error)
	ListFormDefinitionDrafts(ctx context.Context, tenantID, ownerAccountID, status string) ([]domain.FormDefinitionDraft, error)

	UpsertFormTemplate(context.Context, domain.FormTemplate) error
	GetFormTemplate(ctx context.Context, tenantID, id string) (domain.FormTemplate, bool, error)
	GetFormTemplateByKey(ctx context.Context, tenantID, key string) (domain.FormTemplate, bool, error)
	ListFormTemplates(ctx context.Context, tenantID string) ([]domain.FormTemplate, error)
	InsertFormTemplateVersion(context.Context, domain.FormTemplateVersion) error
	GetFormTemplateVersion(ctx context.Context, tenantID, id string) (domain.FormTemplateVersion, bool, error)
	GetFormTemplateVersionByNumber(ctx context.Context, tenantID, templateID string, version int) (domain.FormTemplateVersion, bool, error)

	UpsertFormInstance(context.Context, domain.FormInstance) error
	GetFormInstance(ctx context.Context, tenantID, id string) (domain.FormInstance, bool, error)
	ListFormInstances(ctx context.Context, tenantID string) ([]domain.FormInstance, error)
	ListFormInstancesByQuery(ctx context.Context, tenantID string, query domain.FormInstanceQuery) ([]domain.FormInstance, error)
	ListFormInstancePageByQuery(ctx context.Context, tenantID string, query domain.FormInstanceQuery, page domain.PageRequest) ([]domain.FormInstance, int, error)
	ReplaceFormInstanceFieldValues(ctx context.Context, tenantID, formInstanceID string, values []domain.FormInstanceFieldValue) error
	ListFormInstanceFieldValues(ctx context.Context, tenantID, formInstanceID string) ([]domain.FormInstanceFieldValue, error)
	DeleteFormInstance(ctx context.Context, tenantID, id string) error

	UpsertFormFileAsset(context.Context, domain.FormInstanceFile) error
	InsertFormInstanceFile(context.Context, domain.FormInstanceFile) error
	GetFormInstanceFile(ctx context.Context, tenantID, formInstanceID, fileID string) (domain.FormInstanceFile, bool, error)
	ListFormInstanceFiles(ctx context.Context, tenantID, formInstanceID string) ([]domain.FormInstanceFile, error)
	ListFormInstanceFilesByField(ctx context.Context, tenantID, formInstanceID, fieldID string) ([]domain.FormInstanceFile, error)
	CountFormInstanceFilesByField(ctx context.Context, tenantID, formInstanceID, fieldID string) (int, error)
	MarkFormInstanceFilesAttached(ctx context.Context, tenantID, formInstanceID string, updatedAt time.Time) error
	DeleteDraftFormInstanceFile(ctx context.Context, tenantID, formInstanceID, fileID string) (bool, error)
	DeleteFormFileAsset(ctx context.Context, tenantID, fileID string) error

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
