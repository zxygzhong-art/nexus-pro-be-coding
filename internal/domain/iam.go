package domain

import "time"

type UserGroup struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	MemberAccountIDs []string  `json:"member_account_ids"`
	PermissionSetIDs []string  `json:"permission_set_ids"`
	CreatedAt        time.Time `json:"created_at"`
}

type PermissionSet struct {
	ID          string       `json:"id"`
	TenantID    string       `json:"tenant_id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Permissions []Permission `json:"permissions"`
	CreatedAt   time.Time    `json:"created_at"`
}

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

type DataScope struct {
	ID        string         `json:"id"`
	TenantID  string         `json:"tenant_id"`
	Code      string         `json:"code"`
	Name      string         `json:"name"`
	ScopeType string         `json:"scope_type"`
	Params    map[string]any `json:"params,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

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

type PermissionVersion struct {
	TenantID string `json:"tenant_id"`
	Version  int64  `json:"version"`
}

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

type AuthzRelationshipTupleOperation string

const (
	AuthzRelationshipTupleWrite  AuthzRelationshipTupleOperation = "write"
	AuthzRelationshipTupleDelete AuthzRelationshipTupleOperation = "delete"
)

type AuthzRelationshipTupleChange struct {
	Operation AuthzRelationshipTupleOperation `json:"operation"`
	Tuple     AuthzRelationshipTuple          `json:"tuple"`
}

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
}

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

type BatchCheckRequest struct {
	Checks []CheckRequest `json:"checks"`
}

type BatchCheckResult struct {
	Results []CheckResult `json:"results"`
}

type AssumedRoleDecision struct {
	SessionID string `json:"session_id,omitempty"`
	RoleID    string `json:"role_id"`
	Name      string `json:"name,omitempty"`
}
