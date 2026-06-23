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
