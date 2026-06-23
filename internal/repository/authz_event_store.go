package repository

import (
	"context"

	"nexus-pro-be/internal/domain"
)

type AuthzEventStore interface {
	GetPermissionVersion(ctx context.Context, tenantID string) (int64, error)
	IncrementPermissionVersion(ctx context.Context, tenantID string) (int64, error)
	UpsertAuthzRelationshipTuple(context.Context, domain.AuthzRelationshipTuple) error
	DeleteAuthzRelationshipTuple(context.Context, domain.AuthzRelationshipTuple) error
	ListAuthzRelationshipTuplesForObject(ctx context.Context, tenantID, objectType, objectID string) ([]domain.AuthzRelationshipTuple, error)
	AppendAuthzOutboxEvent(context.Context, domain.AuthzOutboxEvent) error
	ListAuthzOutboxEvents(ctx context.Context, tenantID string) ([]domain.AuthzOutboxEvent, error)
	UpdateAuthzOutboxEvent(context.Context, domain.AuthzOutboxEvent) error
}
