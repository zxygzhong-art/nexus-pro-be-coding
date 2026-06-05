package models

import "gorm.io/datatypes"

// DataScope is a structured row-level data range (own/department/tenant/...).
type DataScope struct {
	TenantModel
	Name       string         `json:"name"`
	ScopeType  string         `gorm:"column:scope_type" json:"scope_type"`
	Conditions datatypes.JSON `json:"conditions"`
}

func (DataScope) TableName() string { return "iam_data_scopes" }

// PolicyCondition is a reusable JSON condition tree (never raw SQL).
type PolicyCondition struct {
	TenantModel
	Name        string         `json:"name"`
	Expression  datatypes.JSON `json:"expression"`
	Description string         `json:"description"`
}

func (PolicyCondition) TableName() string { return "iam_policy_conditions" }
