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
	Decision        CheckResult           `json:"decision"`
	EvaluatedGrants []AuthzEvaluatedGrant `json:"evaluated_grants"`
	DenySources     []string              `json:"deny_sources,omitempty"`
	BoundaryEffects []AuthzBoundaryEffect `json:"boundary_effects,omitempty"`
	ScopeDerivation AuthzScopeDerivation  `json:"scope_derivation"`
}

// AuthzEvaluatedGrant 定義授權說明中的 grant 評估項。
type AuthzEvaluatedGrant struct {
	Source          string  `json:"source"`
	SourceID        string  `json:"source_id,omitempty"`
	PermissionSetID string  `json:"permission_set_id,omitempty"`
	Permission      string  `json:"permission,omitempty"`
	Effect          string  `json:"effect,omitempty"`
	Matched         bool    `json:"matched"`
	Scope           Scope   `json:"scope,omitempty"`
	ExcludedBy      *string `json:"excluded_by"`
}

// AuthzBoundaryEffect 定義 permission boundary 對決策的影響。
type AuthzBoundaryEffect struct {
	Source     string `json:"source"`
	Permission string `json:"permission,omitempty"`
	Effect     string `json:"effect"`
	Matched    bool   `json:"matched"`
	ExcludedBy string `json:"excluded_by,omitempty"`
}

// AuthzScopeDerivation 定義授權資料範圍推導過程。
type AuthzScopeDerivation struct {
	Normal            Scope `json:"normal,omitempty"`
	Assumed           Scope `json:"assumed,omitempty"`
	Boundary          Scope `json:"boundary,omitempty"`
	Final             Scope `json:"final,omitempty"`
	IntersectionEmpty bool  `json:"intersection_empty,omitempty"`
}

// AuthzSimulationRequest 定義授權模擬請求。
type AuthzSimulationRequest struct {
	AccountID string                   `json:"account_id,omitempty"`
	Check     CheckRequest             `json:"check"`
	Overrides AuthzSimulationOverrides `json:"overrides,omitempty"`
}

// AuthzSimulationOverrides 定義授權模擬覆蓋項。
type AuthzSimulationOverrides struct {
	AddUserGroups        []string                   `json:"add_user_groups,omitempty"`
	RemoveUserGroups     []string                   `json:"remove_user_groups,omitempty"`
	AddPermissionSets    []string                   `json:"add_permission_sets,omitempty"`
	RemovePermissionSets []string                   `json:"remove_permission_sets,omitempty"`
	AssumeRoleID         string                     `json:"assume_role_id,omitempty"`
	PermissionSetChanges []AuthzPermissionSetChange `json:"permission_set_changes,omitempty"`
}

// AuthzPermissionSetChange 定義權限集合模擬變更。
type AuthzPermissionSetChange struct {
	PermissionSetID   string   `json:"permission_set_id"`
	AddPermissions    []string `json:"add_permissions,omitempty"`
	RemovePermissions []string `json:"remove_permissions,omitempty"`
}

// AuthzSimulationResponse 定義授權模擬回應的資料結構。
type AuthzSimulationResponse struct {
	Before CheckResult         `json:"before"`
	After  CheckResult         `json:"after"`
	Diff   AuthzSimulationDiff `json:"diff"`
}

// AuthzSimulationDiff 定義授權模擬差異。
type AuthzSimulationDiff struct {
	AllowedChanged            bool     `json:"allowed_changed"`
	BeforeAllowed             bool     `json:"before_allowed"`
	AfterAllowed              bool     `json:"after_allowed"`
	ScopeChanged              bool     `json:"scope_changed"`
	BeforeScope               Scope    `json:"before_scope,omitempty"`
	AfterScope                Scope    `json:"after_scope,omitempty"`
	AddedMatchedBy            []string `json:"added_matched_by,omitempty"`
	RemovedMatchedBy          []string `json:"removed_matched_by,omitempty"`
	AddedMatchedPermissions   []string `json:"added_matched_permissions,omitempty"`
	RemovedMatchedPermissions []string `json:"removed_matched_permissions,omitempty"`
}
