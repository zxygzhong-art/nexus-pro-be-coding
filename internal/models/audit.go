package models

import (
	"time"

	"gorm.io/datatypes"
)

// AuditLog is an append-only record of a permission decision or high-risk action.
type AuditLog struct {
	ID                   string         `gorm:"primaryKey;type:text" json:"id"`
	TenantID             string         `gorm:"column:tenant_id" json:"tenant_id"`
	ApplicationCode      string         `gorm:"column:application_code" json:"application_code"`
	ActorAccountID       string         `gorm:"column:actor_account_id" json:"actor_account_id"`
	Action               string         `json:"action"`
	ResourceType         string         `gorm:"column:resource_type" json:"resource_type"`
	ResourceID           string         `gorm:"column:resource_id" json:"resource_id"`
	AuthzDecision        string         `gorm:"column:authz_decision" json:"authz_decision"`
	MatchedPermissions   datatypes.JSON `gorm:"column:matched_permissions" json:"matched_permissions"`
	MatchedSources       datatypes.JSON `gorm:"column:matched_sources" json:"matched_sources"`
	AssumedRoleSessionID string         `gorm:"column:assumed_role_session_id" json:"assumed_role_session_id"`
	PermissionBoundary   string         `gorm:"column:permission_boundary" json:"permission_boundary"`
	DataScope            string         `gorm:"column:data_scope" json:"data_scope"`
	FieldPolicies        datatypes.JSON `gorm:"column:field_policies" json:"field_policies"`
	RequestID            string         `gorm:"column:request_id" json:"request_id"`
	TraceID              string         `gorm:"column:trace_id" json:"trace_id"`
	Metadata             datatypes.JSON `json:"metadata"`
	CreatedAt            time.Time      `json:"created_at"`
}

func (AuditLog) TableName() string { return "iam_audit_logs" }
