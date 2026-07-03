package domain

// Pagination defaults used across list endpoints.
const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// PageRequest carries common pagination and sorting query parameters.
type PageRequest struct {
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
	Sort     string `json:"sort,omitempty"`
}

// PageResponse wraps a paged list and its total item count.
type PageResponse[T any] struct {
	Items    []T    `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Sort     string `json:"sort"`
}

// MeResponse returns the current account, tenant, IAM grants, and capabilities.
type MeResponse struct {
	Tenant               Tenant          `json:"tenant"`
	Account              Account         `json:"account"`
	Employee             *Employee       `json:"employee,omitempty"`
	AssumedRole          *AssumableRole  `json:"assumed_role,omitempty"`
	UserGroups           []UserGroup     `json:"user_groups"`
	PermissionSets       []PermissionSet `json:"permission_sets"`
	EffectivePermissions []Permission    `json:"effective_permissions"`
	EffectiveMenuKeys    []string        `json:"effective_menu_keys"`
	Capabilities         []string        `json:"capabilities"`
}

// MenuListResponse returns the visible menu tree for the current account.
type MenuListResponse struct {
	Items []MenuNode `json:"items"`
	Total int        `json:"total"`
}

// AssumeRoleResponse returns the created role session and its effective boundary.
type AssumeRoleResponse struct {
	SessionID          string         `json:"session_id"`
	SessionToken       string         `json:"session_token"`
	AssumedRole        AssumableRole  `json:"assumed_role"`
	AccountID          string         `json:"account_id"`
	TenantID           string         `json:"tenant_id"`
	PermissionBoundary map[string]any `json:"permission_boundary,omitempty"`
	ExpiresAt          string         `json:"expires_at"`
}

// AuthzExplainResponse returns an authorization decision with a short explanation.
type AuthzExplainResponse struct {
	Decision CheckResult `json:"decision"`
	Explain  string      `json:"explain"`
}

// AuthzSimulationResponse returns a simulated authorization decision.
type AuthzSimulationResponse struct {
	Decision  CheckResult `json:"decision"`
	Simulated bool        `json:"simulated"`
}
