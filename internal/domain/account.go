package domain

import "time"

// AccountStatus is the lifecycle state for a login-capable account.
type AccountStatus string

// Account status values shared by services and API responses.
const (
	AccountStatusActive        AccountStatus = "active"
	AccountStatusDisabled      AccountStatus = "disabled"
	AccountStatusPendingInvite AccountStatus = "pending_invite"
)

// Account represents a tenant-scoped user identity and its direct IAM grants.
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
	CreatedAt              time.Time `json:"created_at"`
}

// AuthenticatedPrincipal is the external identity extracted from an authenticated token.
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

// UserIdentity links one external identity provider subject to a tenant-scoped local account.
type UserIdentity struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	AccountID string    `json:"account_id"`
	Provider  string    `json:"provider"`
	Subject   string    `json:"subject"`
	Email     string    `json:"email,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// IdentityResolution is the local account context derived from an authenticated principal.
type IdentityResolution struct {
	TenantID  string        `json:"tenant_id"`
	AccountID string        `json:"account_id"`
	Identity  *UserIdentity `json:"identity,omitempty"`
}
