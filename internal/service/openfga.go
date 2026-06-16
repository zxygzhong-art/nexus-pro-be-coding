package service

import (
	"context"

	authzpkg "nexus-pro-be/internal/domain/authz"
)

type RelationshipChecker interface {
	CheckRelationship(ctx context.Context, check authzpkg.RelationshipCheck) (bool, error)
}
