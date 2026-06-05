package models

import "gorm.io/datatypes"

// MenuItem is a navigation node bound to a required permission point.
type MenuItem struct {
	TenantModel
	ApplicationCode      string         `gorm:"column:application_code" json:"application_code"`
	ParentID             string         `gorm:"column:parent_id" json:"parent_id"`
	Label                string         `json:"label"`
	Route                string         `json:"route"`
	Icon                 string         `json:"icon"`
	RequiredPermissionID string         `gorm:"column:required_permission_id" json:"required_permission_id"`
	PageType             string         `gorm:"column:page_type" json:"page_type"`
	PathHierarchy        string         `gorm:"column:path_hierarchy" json:"path_hierarchy"`
	EnabledCondition     datatypes.JSON `gorm:"column:enabled_condition" json:"enabled_condition"`
	SortOrder            int            `gorm:"column:sort_order" json:"sort_order"`
}

func (MenuItem) TableName() string { return "iam_menu_items" }

// ButtonAction is a page action bound to a required permission point.
type ButtonAction struct {
	TenantModel
	ApplicationCode      string `gorm:"column:application_code" json:"application_code"`
	MenuItemID           string `gorm:"column:menu_item_id" json:"menu_item_id"`
	Code                 string `json:"code"`
	Label                string `json:"label"`
	RequiredPermissionID string `gorm:"column:required_permission_id" json:"required_permission_id"`
}

func (ButtonAction) TableName() string { return "iam_button_actions" }

// FieldPolicy describes per-field visibility/masking for a resource type.
type FieldPolicy struct {
	TenantModel
	ApplicationCode      string         `gorm:"column:application_code" json:"application_code"`
	ResourceType         string         `gorm:"column:resource_type" json:"resource_type"`
	Field                string         `json:"field"`
	Effect               string         `json:"effect"`
	Sensitivity          string         `json:"sensitivity"`
	RequiredPermissionID string         `gorm:"column:required_permission_id" json:"required_permission_id"`
	Condition            datatypes.JSON `json:"condition"`
}

func (FieldPolicy) TableName() string { return "iam_field_policies" }
