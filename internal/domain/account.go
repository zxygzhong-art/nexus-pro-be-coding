package domain

import (
	"errors"
	"strings"
	"time"
)

// AccountStatus 表示帳號狀態。
type AccountStatus string

// 下列常數定義此模組使用的固定值。
const (
	AccountStatusActive        AccountStatus = "active"
	AccountStatusDisabled      AccountStatus = "disabled"
	AccountStatusPendingInvite AccountStatus = "pending_invite"
)

const (
	PreferredLocaleZHTW    = "zh-TW"
	PreferredLocaleENUS    = "en-US"
	DefaultPreferredLocale = PreferredLocaleZHTW
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
	PreferredLocale        string    `json:"preferred_locale"`
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

// UpdateMePreferencesInput contains account-level preferences owned by the authenticated user.
type UpdateMePreferencesInput struct {
	PreferredLocale string `json:"preferred_locale"`
}

// Validate accepts only locales supported by the current frontend message catalog.
func (input UpdateMePreferencesInput) Validate() error {
	if input.PreferredLocale == PreferredLocaleZHTW || input.PreferredLocale == PreferredLocaleENUS {
		return nil
	}
	return ValidationFailed("account preferences are invalid", []FieldError{{
		Field:   "preferred_locale",
		Code:    "unsupported",
		Message: "preferred locale must be one of zh-TW or en-US",
	}})
}

// PreferredLocaleWithDefault preserves explicit values while defaulting legacy account inputs.
func PreferredLocaleWithDefault(value string) string {
	if strings.TrimSpace(value) == "" {
		return DefaultPreferredLocale
	}
	return value
}

// ChangePasswordInput contains only the password values needed for one self-service update.
type ChangePasswordInput struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
	Confirmation    string `json:"confirmation"`
}

// Validate rejects incomplete, oversized, mismatched, or no-op password changes before any identity-provider call.
func (input ChangePasswordInput) Validate() error {
	fields := make([]FieldError, 0, 3)
	if input.CurrentPassword == "" {
		fields = append(fields, FieldError{Field: "current_password", Code: "required", Message: "current password is required"})
	} else if len(input.CurrentPassword) > 512 {
		fields = append(fields, FieldError{Field: "current_password", Code: "invalid", Message: "current password is too long"})
	}
	if input.NewPassword == "" {
		fields = append(fields, FieldError{Field: "new_password", Code: "required", Message: "new password is required"})
	} else if len(input.NewPassword) > 512 {
		fields = append(fields, FieldError{Field: "new_password", Code: "invalid", Message: "new password is too long"})
	}
	if input.Confirmation == "" {
		fields = append(fields, FieldError{Field: "confirmation", Code: "required", Message: "password confirmation is required"})
	} else if len(input.Confirmation) > 512 {
		fields = append(fields, FieldError{Field: "confirmation", Code: "invalid", Message: "password confirmation is too long"})
	} else if input.NewPassword != input.Confirmation {
		fields = append(fields, FieldError{Field: "confirmation", Code: "invalid", Message: "password confirmation does not match"})
	}
	if input.CurrentPassword != "" && strings.TrimSpace(input.NewPassword) != "" && input.CurrentPassword == input.NewPassword {
		fields = append(fields, FieldError{Field: "new_password", Code: "invalid", Message: "new password must differ from current password"})
	}
	if len(fields) > 0 {
		return ValidationFailed("password change input is invalid", fields)
	}
	return nil
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

// IdentityPasswordChangeInput binds a password update to the already resolved local identity owner.
type IdentityPasswordChangeInput struct {
	TenantID        string
	AccountID       string
	Subject         string
	CurrentPassword string
	NewPassword     string
}

var (
	// ErrIdentityCurrentPasswordInvalid prevents adapters from leaking raw identity-provider failures.
	ErrIdentityCurrentPasswordInvalid = errors.New("identity current password is invalid")
	// ErrIdentityPasswordRejected identifies a new password rejected by the provider policy.
	ErrIdentityPasswordRejected = errors.New("identity password was rejected")
	// ErrIdentityPasswordUnavailable identifies missing credentials or an unavailable provider operation.
	ErrIdentityPasswordUnavailable = errors.New("identity password change is unavailable")
)

// 下列常數定義此模組使用的固定值。
const (
	IdentityProvisioningStatusPending    = "pending"
	IdentityProvisioningStatusProcessing = "processing"
	IdentityProvisioningStatusSucceeded  = "succeeded"
	IdentityProvisioningStatusFailed     = "failed"
)

// IdentityProvisioningOutboxEvent 定義身分開通 outbox 事件的資料結構。
type IdentityProvisioningOutboxEvent struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	AccountID      string     `json:"account_id"`
	EmployeeID     string     `json:"employee_id,omitempty"`
	EmployeeNo     string     `json:"employee_no,omitempty"`
	Email          string     `json:"email"`
	DisplayName    string     `json:"display_name,omitempty"`
	Enabled        bool       `json:"enabled"`
	SendInvite     bool       `json:"send_invite,omitempty"`
	Status         string     `json:"status"`
	RetryCount     int        `json:"retry_count"`
	LastError      string     `json:"last_error,omitempty"`
	NextAttemptAt  time.Time  `json:"next_attempt_at"`
	ClaimExpiresAt *time.Time `json:"claim_expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// IdentityResolution 定義身分 resolution 的資料結構。
type IdentityResolution struct {
	TenantID  string        `json:"tenant_id"`
	AccountID string        `json:"account_id"`
	Identity  *UserIdentity `json:"identity,omitempty"`
}
