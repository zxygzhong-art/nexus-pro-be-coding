package models

import (
	"time"

	"gorm.io/datatypes"
)

// PermissionBoundary caps the effective permissions of an assumed identity.
type PermissionBoundary struct {
	TenantModel
	Name               string         `json:"name"`
	AllowedPermissions datatypes.JSON `gorm:"column:allowed_permissions" json:"allowed_permissions"`
	ScopeType          string         `gorm:"column:scope_type" json:"scope_type"`
	ScopeConditions    datatypes.JSON `gorm:"column:scope_conditions" json:"scope_conditions"`
}

func (PermissionBoundary) TableName() string { return "iam_permission_boundaries" }

// AssumableRole is a high-security identity trusted principals can temporarily assume.
type AssumableRole struct {
	TenantModel
	SoftDelete
	Name                 string `json:"name"`
	Description          string `json:"description"`
	PermissionBoundaryID string `gorm:"column:permission_boundary_id" json:"permission_boundary_id"`
	MaxSessionMinutes    int    `gorm:"column:max_session_minutes" json:"max_session_minutes"`
	RequiresApproval     bool   `gorm:"column:requires_approval" json:"requires_approval"`
	AuditLevel           string `gorm:"column:audit_level" json:"audit_level"`
}

func (AssumableRole) TableName() string { return "iam_assumable_roles" }

// TrustPolicy defines who may assume a role and under what conditions.
type TrustPolicy struct {
	TenantModel
	AssumableRoleID string         `gorm:"column:assumable_role_id" json:"assumable_role_id"`
	Policy          datatypes.JSON `json:"policy"`
}

func (TrustPolicy) TableName() string { return "iam_trust_policies" }

// SessionPolicy is a reusable per-session restriction template.
type SessionPolicy struct {
	TenantModel
	AssumableRoleID string         `gorm:"column:assumable_role_id" json:"assumable_role_id"`
	Policy          datatypes.JSON `json:"policy"`
}

func (SessionPolicy) TableName() string { return "iam_session_policies" }

// AssumableRoleSession is a live assume session. It has no updated_at (sessions
// transition status but are not GORM-updated), so fields are declared explicitly.
type AssumableRoleSession struct {
	ID                   string         `gorm:"primaryKey;type:text" json:"id"`
	TenantID             string         `gorm:"column:tenant_id" json:"tenant_id"`
	AccountID            string         `gorm:"column:account_id" json:"account_id"`
	AssumableRoleID      string         `gorm:"column:assumable_role_id" json:"assumable_role_id"`
	PermissionBoundaryID string         `gorm:"column:permission_boundary_id" json:"permission_boundary_id"`
	SessionPolicy        datatypes.JSON `gorm:"column:session_policy" json:"session_policy"`
	Reason               string         `json:"reason"`
	Status               string         `json:"status"`
	ExpiresAt            time.Time      `gorm:"column:expires_at" json:"expires_at"`
	CreatedAt            time.Time      `json:"created_at"`
}

func (AssumableRoleSession) TableName() string { return "iam_assumable_role_sessions" }
