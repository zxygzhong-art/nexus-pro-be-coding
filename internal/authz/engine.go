package authz

import (
	"context"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/models"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/principal"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/reqctx"
)

// SubjectRef identifies an authorization subject (user | group | assumable_role).
type SubjectRef struct {
	Type string
	ID   string
}

// AssignmentInfo is a flattened, validity-filtered permission-set assignment.
type AssignmentInfo struct {
	PermissionSetID string
	SubjectType     string // user | group | assumable_role
	Effect          string // allow | deny
	DataScopeType   string // resolved from data_scope_id (empty if none)
	SourceLabel     string // e.g. group.hr-admin, user.acct-x, assumable_role.x
}

// DataSource is the narrow data access the engine needs. The repository layer
// implements it against a tenant-scoped *gorm.DB; tests supply a fake.
type DataSource interface {
	Assignments(ctx context.Context, subjects []SubjectRef) ([]AssignmentInfo, error)
	PermissionsForSets(ctx context.Context, setIDs []string) (map[string][]models.Permission, error)
	FieldPolicies(ctx context.Context, applicationCode, resourceType string) ([]models.FieldPolicy, error)
	Menus(ctx context.Context, applicationCode string) ([]models.MenuItem, error)
	PermissionsByIDs(ctx context.Context, ids []string) (map[string]models.Permission, error)
	ActiveSession(ctx context.Context, sessionID string) (*models.AssumableRoleSession, error)
	Boundary(ctx context.Context, id string) (*models.PermissionBoundary, error)
}

// LocalEngine computes decisions from the IAM tables. It implements the
// adapters/authorizer.Authorizer interface.
type LocalEngine struct {
	src DataSource
}

// NewLocalEngine builds an engine over the given data source.
func NewLocalEngine(src DataSource) *LocalEngine {
	return &LocalEngine{src: src}
}

// Check evaluates a single authorization request for the principal in ctx.
func (e *LocalEngine) Check(ctx context.Context, req Request) (Decision, error) {
	p, ok := reqctx.Principal(ctx)
	if !ok {
		return Decision{Allowed: false, Reason: "no principal in context"}, nil
	}
	return e.evaluate(ctx, p, req)
}

// BatchCheck evaluates many requests, preserving order.
func (e *LocalEngine) BatchCheck(ctx context.Context, reqs []Request) ([]Decision, error) {
	out := make([]Decision, len(reqs))
	for i, r := range reqs {
		d, err := e.Check(ctx, r)
		if err != nil {
			return nil, err
		}
		out[i] = d
	}
	return out, nil
}

// subjectsFor builds the subject list for a principal, including an active
// assumed-role session if present.
func (e *LocalEngine) subjectsFor(ctx context.Context, p principal.Principal) ([]SubjectRef, *models.AssumableRoleSession) {
	subjects := []SubjectRef{{Type: "user", ID: p.AccountID}}
	for _, g := range p.GroupIDs {
		subjects = append(subjects, SubjectRef{Type: "group", ID: g})
	}
	var session *models.AssumableRoleSession
	if p.AssumedRoleSessionID != "" {
		if s, err := e.src.ActiveSession(ctx, p.AssumedRoleSessionID); err == nil && s != nil {
			session = s
			subjects = append(subjects, SubjectRef{Type: "assumable_role", ID: s.AssumableRoleID})
		}
	}
	return subjects, session
}
