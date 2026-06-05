package models

import "time"

// UserGroup is the primary day-to-day authorization subject within a tenant.
type UserGroup struct {
	TenantModel
	SoftDelete
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	TemplateID  string `gorm:"column:template_id" json:"template_id"`
}

func (UserGroup) TableName() string { return "iam_user_groups" }

// GroupMembership links an account to a user group with an optional validity window.
type GroupMembership struct {
	TenantModel
	AccountID   string     `gorm:"column:account_id" json:"account_id"`
	GroupID     string     `gorm:"column:group_id" json:"group_id"`
	ValidFrom   *time.Time `gorm:"column:valid_from" json:"valid_from"`
	ValidUntil  *time.Time `gorm:"column:valid_until" json:"valid_until"`
	Source      string     `json:"source"`
	ApprovalRef string     `gorm:"column:approval_ref" json:"approval_ref"`
}

func (GroupMembership) TableName() string { return "iam_group_memberships" }
