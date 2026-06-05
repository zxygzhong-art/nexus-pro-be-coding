package service

import (
	"context"
	"encoding/json"
	"time"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/audit"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/models"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/apperror"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/idgen"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/principal"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/repository"
	"gorm.io/datatypes"
)

// AssumeResult is the outcome of assuming a role (§9).
type AssumeResult struct {
	SessionID          string
	AssumedRole        string
	ExpiresAt          time.Time
	PermissionBoundary string
	RequiresApproval   bool
}

// AssumableRoleService handles assuming a role and creating a controlled session.
// TrustPolicy / MFA / approval enforcement is left as a documented extension
// point; this scaffold validates the role exists and creates the session.
type AssumableRoleService struct {
	repo *repository.Repository
	rec  *audit.Recorder
}

// NewAssumableRoleService builds the service.
func NewAssumableRoleService(repo *repository.Repository, rec *audit.Recorder) *AssumableRoleService {
	return &AssumableRoleService{repo: repo, rec: rec}
}

// Assume creates an active session for the principal assuming the given role.
func (s *AssumableRoleService) Assume(ctx context.Context, p principal.Principal, roleID, reason string, durationMinutes int, sessionPolicy map[string]any) (AssumeResult, error) {
	role, err := s.repo.GetAssumableRole(ctx, roleID)
	if err != nil {
		return AssumeResult{}, apperror.NotFound("assumable role not found")
	}

	// Clamp the session duration to the role's maximum.
	dur := durationMinutes
	if dur <= 0 || dur > role.MaxSessionMinutes {
		dur = role.MaxSessionMinutes
	}
	expires := time.Now().UTC().Add(time.Duration(dur) * time.Minute)

	session := &models.AssumableRoleSession{
		ID:                   idgen.NewWith("ars", "_"),
		TenantID:             p.TenantID,
		AccountID:            p.AccountID,
		AssumableRoleID:      role.ID,
		PermissionBoundaryID: role.PermissionBoundaryID,
		SessionPolicy:        toJSON(sessionPolicy),
		Reason:               reason,
		Status:               "active",
		ExpiresAt:            expires,
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return AssumeResult{}, err
	}

	_ = s.rec.Record(ctx, audit.Entry{
		TenantID:             p.TenantID,
		ApplicationCode:      "iam",
		ActorAccountID:       p.AccountID,
		Action:               "assume",
		ResourceType:         "assumable_role",
		ResourceID:           role.ID,
		Decision:             "allow",
		AssumedRoleSessionID: session.ID,
		PermissionBoundary:   role.PermissionBoundaryID,
		Metadata:             map[string]any{"reason": reason, "duration_minutes": dur},
	})

	return AssumeResult{
		SessionID:          session.ID,
		AssumedRole:        role.ID,
		ExpiresAt:          expires,
		PermissionBoundary: role.PermissionBoundaryID,
		RequiresApproval:   role.RequiresApproval,
	}, nil
}

func toJSON(m map[string]any) datatypes.JSON {
	if m == nil {
		return datatypes.JSON("{}")
	}
	b, err := json.Marshal(m)
	if err != nil {
		return datatypes.JSON("{}")
	}
	return datatypes.JSON(b)
}
