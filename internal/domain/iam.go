package domain

import "time"

// UserGroup 定義使用者群組的資料結構。
type UserGroup struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	MemberAccountIDs []string  `json:"member_account_ids"`
	PermissionSetIDs []string  `json:"permission_set_ids"`
	CreatedAt        time.Time `json:"created_at"`
}

// CreateUserGroupInput 定義使用者群組輸入的資料結構。
type CreateUserGroupInput struct {
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	PermissionSetIDs []string `json:"permission_set_ids,omitempty"`
	MemberAccountIDs []string `json:"member_account_ids,omitempty"`
}

// PermissionSet 定義權限集合的資料結構。
type PermissionSet struct {
	ID          string       `json:"id"`
	TenantID    string       `json:"tenant_id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Permissions []Permission `json:"permissions"`
	CreatedAt   time.Time    `json:"created_at"`
}

// CreatePermissionSetInput 定義權限集合輸入的資料結構。
type CreatePermissionSetInput struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Permissions []Permission `json:"permissions"`
}

// Permission 定義權限的資料結構。
type Permission struct {
	ApplicationCode ApplicationCode `json:"application_code,omitempty"`
	ResourceType    ResourceType    `json:"resource_type,omitempty"`
	Resource        string          `json:"resource"`
	Action          Action          `json:"action"`
	Target          string          `json:"target,omitempty"`
	Scope           Scope           `json:"scope,omitempty"`
	Effect          string          `json:"effect,omitempty"`
	RiskLevel       string          `json:"risk_level,omitempty"`
	Relation        string          `json:"relation,omitempty"`
	MenuKey         string          `json:"menu_key,omitempty"`
}

// AssumableRole 定義 assumable 角色的資料結構。
type AssumableRole struct {
	ID                     string         `json:"id"`
	TenantID               string         `json:"tenant_id"`
	Name                   string         `json:"name"`
	Description            string         `json:"description,omitempty"`
	PermissionSetIDs       []string       `json:"permission_set_ids"`
	Trusted                bool           `json:"trusted"`
	TrustPolicy            map[string]any `json:"trust_policy,omitempty"`
	PermissionBoundary     map[string]any `json:"permission_boundary,omitempty"`
	SessionDurationSeconds int            `json:"session_duration_seconds,omitempty"`
	CreatedAt              time.Time      `json:"created_at"`
}

// CreateAssumableRoleInput 定義 assumable 角色輸入的資料結構。
type CreateAssumableRoleInput struct {
	Name                   string         `json:"name"`
	Description            string         `json:"description,omitempty"`
	PermissionSetIDs       []string       `json:"permission_set_ids,omitempty"`
	Trusted                bool           `json:"trusted"`
	TrustPolicy            map[string]any `json:"trust_policy,omitempty"`
	PermissionBoundary     map[string]any `json:"permission_boundary,omitempty"`
	SessionDurationSeconds int            `json:"session_duration_seconds,omitempty"`
}

// PermissionSetAssignment 定義權限集合指派的資料結構。
type PermissionSetAssignment struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	PrincipalType   string     `json:"principal_type"`
	PrincipalID     string     `json:"principal_id"`
	PermissionSetID string     `json:"permission_set_id"`
	Effect          string     `json:"effect"`
	DataScopeID     string     `json:"data_scope_id,omitempty"`
	ConditionID     string     `json:"condition_id,omitempty"`
	StartsAt        *time.Time `json:"starts_at,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// CreatePermissionSetAssignmentInput 定義權限集合指派輸入的資料結構。
type CreatePermissionSetAssignmentInput struct {
	PrincipalType   string `json:"principal_type"`
	PrincipalID     string `json:"principal_id"`
	PermissionSetID string `json:"permission_set_id"`
	Effect          string `json:"effect,omitempty"`
	DataScopeID     string `json:"data_scope_id,omitempty"`
	ConditionID     string `json:"condition_id,omitempty"`
	StartsAt        string `json:"starts_at,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
}

// DataScope 定義資料範圍的資料結構。
type DataScope struct {
	ID        string         `json:"id"`
	TenantID  string         `json:"tenant_id"`
	Code      string         `json:"code"`
	Name      string         `json:"name"`
	ScopeType string         `json:"scope_type"`
	Params    map[string]any `json:"params,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// CreateDataScopeInput 定義資料範圍輸入的資料結構。
type CreateDataScopeInput struct {
	Code      string         `json:"code"`
	Name      string         `json:"name"`
	ScopeType string         `json:"scope_type"`
	Params    map[string]any `json:"params,omitempty"`
}

// FieldPolicy 定義欄位政策的資料結構。
type FieldPolicy struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	ApplicationCode string    `json:"application_code"`
	ResourceType    string    `json:"resource_type"`
	FieldName       string    `json:"field_name"`
	Effect          string    `json:"effect"`
	MaskStrategy    string    `json:"mask_strategy,omitempty"`
	PermissionID    string    `json:"permission_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// CreateFieldPolicyInput 定義欄位政策輸入的資料結構。
type CreateFieldPolicyInput struct {
	ApplicationCode string `json:"application_code"`
	ResourceType    string `json:"resource_type"`
	FieldName       string `json:"field_name"`
	Effect          string `json:"effect"`
	MaskStrategy    string `json:"mask_strategy,omitempty"`
	PermissionID    string `json:"permission_id,omitempty"`
}

// AssumableRoleSession 定義 assumable 角色 session 的資料結構。
type AssumableRoleSession struct {
	ID                 string         `json:"id"`
	TenantID           string         `json:"tenant_id"`
	AccountID          string         `json:"account_id"`
	AssumableRoleID    string         `json:"assumable_role_id"`
	SessionPolicy      map[string]any `json:"session_policy,omitempty"`
	PermissionBoundary map[string]any `json:"permission_boundary,omitempty"`
	ExpiresAt          time.Time      `json:"expires_at"`
	RevokedAt          *time.Time     `json:"revoked_at,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
}

// AssumeRoleInput 定義角色輸入的資料結構。
type AssumeRoleInput struct {
	Reason          string         `json:"reason,omitempty"`
	DurationMinutes int            `json:"duration_minutes,omitempty"`
	SessionPolicy   map[string]any `json:"session_policy,omitempty"`
}

// PermissionVersion 定義權限 version 的資料結構。
type PermissionVersion struct {
	TenantID string `json:"tenant_id"`
	Version  int64  `json:"version"`
}

// AuthzOutboxEvent 定義授權 outbox 事件的資料結構。
type AuthzOutboxEvent struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	EventType   string         `json:"event_type"`
	Payload     map[string]any `json:"payload,omitempty"`
	Status      string         `json:"status"`
	RetryCount  int            `json:"retry_count"`
	LastError   string         `json:"last_error,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	ProcessedAt *time.Time     `json:"processed_at,omitempty"`
}

// AuthzRelationshipTuple 定義授權關係 tuple 的資料結構。
type AuthzRelationshipTuple struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	ObjectType  string    `json:"object_type"`
	ObjectID    string    `json:"object_id"`
	Relation    string    `json:"relation"`
	SubjectType string    `json:"subject_type"`
	SubjectID   string    `json:"subject_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// AuthzRelationshipTupleOperation 表示授權關係 tuple operation。
type AuthzRelationshipTupleOperation string

// 下列常數定義此模組使用的固定值。
const (
	AuthzRelationshipTupleWrite  AuthzRelationshipTupleOperation = "write"
	AuthzRelationshipTupleDelete AuthzRelationshipTupleOperation = "delete"
)

// AuthzRelationshipTupleChange 定義授權關係 tuple change 的資料結構。
type AuthzRelationshipTupleChange struct {
	Operation AuthzRelationshipTupleOperation `json:"operation"`
	Tuple     AuthzRelationshipTuple          `json:"tuple"`
}

// CheckRequest 定義請求的資料結構。
type CheckRequest struct {
	ApplicationCode ApplicationCode `json:"application_code,omitempty"`
	ResourceType    ResourceType    `json:"resource_type,omitempty"`
	ResourceID      string          `json:"resource_id,omitempty"`
	Action          Action          `json:"action"`
	Context         map[string]any  `json:"context,omitempty"`

	Resource         string `json:"resource,omitempty"`
	Target           string `json:"target,omitempty"`
	Scope            Scope  `json:"scope,omitempty"`
	TargetEmployeeID string `json:"target_employee_id,omitempty"`
	RouteMethod      string `json:"route_method,omitempty"`
	RoutePath        string `json:"route_path,omitempty"`
}

// CheckResult 定義結果的資料結構。
type CheckResult struct {
	Allowed            bool                 `json:"allowed"`
	Reason             string               `json:"reason"`
	MatchedBy          []string             `json:"matched_by,omitempty"`
	MatchedPermissions []string             `json:"matched_permissions,omitempty"`
	MissingPermissions []string             `json:"missing_permissions,omitempty"`
	PermissionSetIDs   []string             `json:"permission_set_ids,omitempty"`
	Scope              Scope                `json:"scope,omitempty"`
	EffectiveScope     Scope                `json:"effective_scope,omitempty"`
	Conditions         map[string]any       `json:"conditions,omitempty"`
	FieldPolicies      map[string]string    `json:"field_policies,omitempty"`
	AssumedRole        *AssumedRoleDecision `json:"assumed_role,omitempty"`
	PermissionBoundary map[string]any       `json:"permission_boundary,omitempty"`
	RequiresApproval   bool                 `json:"requires_approval,omitempty"`
	RiskLevel          string               `json:"risk_level,omitempty"`
	ApprovalType       string               `json:"approval_type,omitempty"`
	ApprovalReason     string               `json:"approval_reason,omitempty"`
	Resource           string               `json:"resource,omitempty"`
	ApplicationCode    ApplicationCode      `json:"application_code,omitempty"`
	ResourceType       ResourceType         `json:"resource_type,omitempty"`
	ResourceID         string               `json:"resource_id,omitempty"`
	Action             Action               `json:"action"`
	Target             string               `json:"target,omitempty"`
}

// BatchCheckRequest 定義批次 check 請求的資料結構。
type BatchCheckRequest struct {
	Checks []CheckRequest `json:"checks"`
}

// BatchCheckResult 定義批次 check 結果的資料結構。
type BatchCheckResult struct {
	Results []CheckResult `json:"results"`
}

// AssumedRoleDecision 定義 assumed 角色決策的資料結構。
type AssumedRoleDecision struct {
	SessionID string `json:"session_id,omitempty"`
	RoleID    string `json:"role_id"`
	Name      string `json:"name,omitempty"`
}
