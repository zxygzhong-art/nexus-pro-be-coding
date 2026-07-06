package domain

// 下列常數定義此模組使用的固定值。
const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// PageRequest 定義分頁請求的資料結構。
type PageRequest struct {
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
	Sort     string `json:"sort,omitempty"`
}

// PageResponse 說明 API 請求或回應契約。
type PageResponse[T any] struct {
	Items    []T    `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Sort     string `json:"sort"`
}

// MeResponse 定義 me 回應的資料結構。
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

// MenuListResponse 定義 menu 列表回應的資料結構。
type MenuListResponse struct {
	Items []MenuNode `json:"items"`
	Total int        `json:"total"`
}

// AssumeRoleResponse 定義角色回應的資料結構。
type AssumeRoleResponse struct {
	SessionID          string         `json:"session_id"`
	SessionToken       string         `json:"session_token"`
	AssumedRole        AssumableRole  `json:"assumed_role"`
	AccountID          string         `json:"account_id"`
	TenantID           string         `json:"tenant_id"`
	PermissionBoundary map[string]any `json:"permission_boundary,omitempty"`
	ExpiresAt          string         `json:"expires_at"`
}

// AuthzExplainResponse 定義授權說明回應的資料結構。
type AuthzExplainResponse struct {
	Decision CheckResult `json:"decision"`
	Explain  string      `json:"explain"`
}

// AuthzSimulationResponse 定義授權模擬回應的資料結構。
type AuthzSimulationResponse struct {
	Decision  CheckResult `json:"decision"`
	Simulated bool        `json:"simulated"`
}
