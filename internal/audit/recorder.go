// Package audit records authorization decisions and high-risk actions. It writes
// each entry in its own tenant-scoped transaction so the audit survives even when
// the originating request is denied and its transaction is rolled back.
package audit

import (
	"context"
	"encoding/json"
	"time"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/db"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/models"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/idgen"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Entry is a single audit record.
type Entry struct {
	TenantID             string
	ApplicationCode      string
	ActorAccountID       string
	Action               string
	ResourceType         string
	ResourceID           string
	Decision             string // allow | deny | approval_required
	MatchedPermissions   []string
	MatchedSources       []string
	AssumedRoleSessionID string
	PermissionBoundary   string
	DataScope            string
	FieldPolicies        map[string]string
	RequestID            string
	TraceID              string
	Metadata             map[string]any
}

// Recorder persists audit entries.
type Recorder struct {
	gdb *gorm.DB
}

// NewRecorder builds a Recorder over the root DB.
func NewRecorder(gdb *gorm.DB) *Recorder { return &Recorder{gdb: gdb} }

// Record writes one audit entry in its own transaction.
func (r *Recorder) Record(ctx context.Context, e Entry) error {
	log := models.AuditLog{
		ID:                   idgen.New("audit"),
		TenantID:             e.TenantID,
		ApplicationCode:      e.ApplicationCode,
		ActorAccountID:       e.ActorAccountID,
		Action:               e.Action,
		ResourceType:         e.ResourceType,
		ResourceID:           e.ResourceID,
		AuthzDecision:        e.Decision,
		MatchedPermissions:   toJSON(e.MatchedPermissions),
		MatchedSources:       toJSON(e.MatchedSources),
		AssumedRoleSessionID: e.AssumedRoleSessionID,
		PermissionBoundary:   e.PermissionBoundary,
		DataScope:            e.DataScope,
		FieldPolicies:        toJSON(e.FieldPolicies),
		RequestID:            e.RequestID,
		TraceID:              e.TraceID,
		Metadata:             toJSON(e.Metadata),
		CreatedAt:            time.Now().UTC(),
	}
	return db.WithTenant(ctx, r.gdb, e.TenantID, func(tx *gorm.DB) error {
		return tx.Create(&log).Error
	})
}

func toJSON(v any) datatypes.JSON {
	if v == nil {
		return datatypes.JSON("null")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON("null")
	}
	return datatypes.JSON(b)
}
