package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

// WorkflowStore persists form templates and form instances.
type WorkflowStore interface {
	UpsertFormTemplate(context.Context, domain.FormTemplate) error
	GetFormTemplate(ctx context.Context, tenantID, id string) (domain.FormTemplate, bool, error)
	GetFormTemplateByKey(ctx context.Context, tenantID, key string) (domain.FormTemplate, bool, error)
	ListFormTemplates(ctx context.Context, tenantID string) ([]domain.FormTemplate, error)

	UpsertFormInstance(context.Context, domain.FormInstance) error
	GetFormInstance(ctx context.Context, tenantID, id string) (domain.FormInstance, bool, error)
	ListFormInstances(ctx context.Context, tenantID string) ([]domain.FormInstance, error)
}
