package domain

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

type MenuListResponse struct {
	Items []MenuNode `json:"items"`
	Total int        `json:"total"`
}

type AssumeRoleResponse struct {
	SessionID          string         `json:"session_id"`
	SessionToken       string         `json:"session_token"`
	AssumedRole        AssumableRole  `json:"assumed_role"`
	AccountID          string         `json:"account_id"`
	TenantID           string         `json:"tenant_id"`
	PermissionBoundary map[string]any `json:"permission_boundary,omitempty"`
	ExpiresAt          string         `json:"expires_at"`
}

type AuthzExplainResponse struct {
	Decision CheckResult `json:"decision"`
	Explain  string      `json:"explain"`
}

type AuthzSimulationResponse struct {
	Decision  CheckResult `json:"decision"`
	Simulated bool        `json:"simulated"`
}
