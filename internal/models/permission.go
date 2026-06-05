package models

import (
	"time"

	"gorm.io/datatypes"
)

// Permission is a permission point: application.resource.action.
type Permission struct {
	TenantModel
	ApplicationCode string `gorm:"column:application_code" json:"application_code"`
	ResourceType    string `gorm:"column:resource_type" json:"resource_type"`
	Action          string `json:"action"`
	DefaultScope    string `gorm:"column:default_scope" json:"default_scope"`
	RiskLevel       string `gorm:"column:risk_level" json:"risk_level"`
	HighRisk        bool   `gorm:"column:high_risk" json:"high_risk"`
	Description     string `json:"description"`
}

func (Permission) TableName() string { return "iam_permissions" }

// PermissionSet is a reusable bundle of permission points.
type PermissionSet struct {
	TenantModel
	SoftDelete
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Version     int    `json:"version"`
	CopiedFrom  string `gorm:"column:copied_from" json:"copied_from"`
}

func (PermissionSet) TableName() string { return "iam_permission_sets" }

// PermissionSetPermission is the join row between a permission set and a permission.
type PermissionSetPermission struct {
	TenantID        string `gorm:"column:tenant_id" json:"tenant_id"`
	PermissionSetID string `gorm:"column:permission_set_id;primaryKey" json:"permission_set_id"`
	PermissionID    string `gorm:"column:permission_id;primaryKey" json:"permission_id"`
}

func (PermissionSetPermission) TableName() string { return "iam_permission_set_permissions" }

// PermissionSetAssignment binds a permission set to a polymorphic subject
// (user | group | assumable_role) with an effect and optional data scope.
type PermissionSetAssignment struct {
	TenantModel
	SoftDelete
	PermissionSetID string         `gorm:"column:permission_set_id" json:"permission_set_id"`
	SubjectType     string         `gorm:"column:subject_type" json:"subject_type"`
	SubjectID       string         `gorm:"column:subject_id" json:"subject_id"`
	Effect          string         `json:"effect"`
	DataScopeID     string         `gorm:"column:data_scope_id" json:"data_scope_id"`
	Condition       datatypes.JSON `json:"condition"`
	ValidFrom       *time.Time     `gorm:"column:valid_from" json:"valid_from"`
	ValidUntil      *time.Time     `gorm:"column:valid_until" json:"valid_until"`
}

func (PermissionSetAssignment) TableName() string { return "iam_permission_set_assignments" }
