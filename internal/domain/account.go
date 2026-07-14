package domain

import "time"

// AccountStatus 表示帳號狀態。
type AccountStatus string

// 下列常數定義此模組使用的固定值。
const (
	AccountStatusActive        AccountStatus = "active"
	AccountStatusDisabled      AccountStatus = "disabled"
	AccountStatusPendingInvite AccountStatus = "pending_invite"
)

// Account 定義帳號的資料結構。
type Account struct {
	ID                     string    `json:"id"`
	TenantID               string    `json:"tenant_id"`
	DisplayName            string    `json:"display_name"`
	Email                  string    `json:"email,omitempty"`
	EmployeeID             string    `json:"employee_id,omitempty"`
	Status                 string    `json:"status"`
	UserGroupIDs           []string  `json:"user_group_ids"`
	DirectPermissionSetIDs []string  `json:"direct_permission_set_ids"`
	ActiveAssumableRoleID  string    `json:"active_assumable_role_id,omitempty"`
	Version                int64     `json:"version"`
	CreatedAt              time.Time `json:"created_at"`
}

// UpdateMeProfileInput defines the self-service profile fields an authenticated user may change.
type UpdateMeProfileInput struct {
	EnglishName          *string `json:"english_name,omitempty"`
	MobilePhone          *string `json:"mobile_phone,omitempty"`
	Extension            *string `json:"extension,omitempty"`
	Slack                *string `json:"slack,omitempty"`
	EmergencyContactName *string `json:"emergency_contact_name,omitempty"`
}

// AuthenticatedPrincipal 定義 authenticated principal 的資料結構。
type AuthenticatedPrincipal struct {
	Provider   string         `json:"provider"`
	Subject    string         `json:"subject"`
	Email      string         `json:"email,omitempty"`
	Name       string         `json:"name,omitempty"`
	TenantID   string         `json:"tenant_id,omitempty"`
	TenantHint string         `json:"tenant_hint,omitempty"`
	AccountID  string         `json:"account_id,omitempty"`
	Claims     map[string]any `json:"claims,omitempty"`
}

// UserIdentity 定義使用者身分的資料結構。
type UserIdentity struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	AccountID string    `json:"account_id"`
	Provider  string    `json:"provider"`
	Subject   string    `json:"subject"`
	Email     string    `json:"email,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// IdentityProviderKeycloak 定義身分提供者 Keycloak 的固定值。
const IdentityProviderKeycloak = "keycloak"

// SSOLoginVerification 定義 SSO 登入驗證結果。
type SSOLoginVerification struct {
	Provider  string `json:"provider"`
	TenantID  string `json:"tenant_id"`
	AccountID string `json:"account_id"`
	Email     string `json:"email"`
}

// IdentityProvisioningInput 定義身分開通輸入的資料結構。
type IdentityProvisioningInput struct {
	TenantID     string `json:"tenant_id"`
	AccountID    string `json:"account_id"`
	EmployeeID   string `json:"employee_id,omitempty"`
	EmployeeNo   string `json:"employee_no,omitempty"`
	Email        string `json:"email"`
	DisplayName  string `json:"display_name,omitempty"`
	Enabled      bool   `json:"enabled"`
	SendInvite   bool   `json:"send_invite,omitempty"`
	InviteClient string `json:"invite_client,omitempty"`
	InviteURL    string `json:"invite_url,omitempty"`
}

// ProvisionedIdentity 定義 provisioned 身分的資料結構。
type ProvisionedIdentity struct {
	Provider string `json:"provider"`
	Subject  string `json:"subject"`
	Email    string `json:"email,omitempty"`
}

// 下列常數定義此模組使用的固定值。
const (
	IdentityProvisioningStatusPending   = "pending"
	IdentityProvisioningStatusSucceeded = "succeeded"
	IdentityProvisioningStatusFailed    = "failed"
)

// IdentityProvisioningOutboxEvent 定義身分開通 outbox 事件的資料結構。
type IdentityProvisioningOutboxEvent struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	AccountID   string    `json:"account_id"`
	EmployeeID  string    `json:"employee_id,omitempty"`
	EmployeeNo  string    `json:"employee_no,omitempty"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name,omitempty"`
	Enabled     bool      `json:"enabled"`
	SendInvite  bool      `json:"send_invite,omitempty"`
	Status      string    `json:"status"`
	RetryCount  int       `json:"retry_count"`
	LastError   string    `json:"last_error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// IdentityResolution 定義身分 resolution 的資料結構。
type IdentityResolution struct {
	TenantID  string        `json:"tenant_id"`
	AccountID string        `json:"account_id"`
	Identity  *UserIdentity `json:"identity,omitempty"`
}
