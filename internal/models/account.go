package models

import "gorm.io/datatypes"

// Account is a platform login account within a tenant.
type Account struct {
	TenantModel
	SoftDelete
	Email       string `json:"email"`
	DisplayName string `gorm:"column:display_name" json:"display_name"`
	Status      string `json:"status"`
	AccountType string `gorm:"column:account_type" json:"account_type"`
}

func (Account) TableName() string { return "iam_accounts" }

// UserIdentity maps an account to an external identity provider subject.
type UserIdentity struct {
	TenantModel
	AccountID string         `gorm:"column:account_id" json:"account_id"`
	Provider  string         `json:"provider"`
	Subject   string         `json:"subject"`
	RawClaims datatypes.JSON `gorm:"column:raw_claims" json:"raw_claims"`
}

func (UserIdentity) TableName() string { return "iam_user_identities" }
