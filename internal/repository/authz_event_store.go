package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

// AuthzEventStore 定義授權事件儲存層的行為契約。
type AuthzEventStore interface {
	GetPermissionVersion(ctx context.Context, tenantID string) (int64, error)
	IncrementPermissionVersion(ctx context.Context, tenantID string) (int64, error)
	UpsertAuthzRelationshipTuple(context.Context, domain.AuthzRelationshipTuple) error
	DeleteAuthzRelationshipTuple(context.Context, domain.AuthzRelationshipTuple) error
	ListAuthzRelationshipTuplesForObject(ctx context.Context, tenantID, objectType, objectID string) ([]domain.AuthzRelationshipTuple, error)
}
