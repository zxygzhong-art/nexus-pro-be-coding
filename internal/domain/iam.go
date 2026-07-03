package domain

import "time"

// UserGroup groups accounts so IAM grants can be managed together.
type UserGroup struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	MemberAccountIDs []string  `json:"member_account_ids"`
	PermissionSetIDs []string  `json:"permission_set_ids"`
	CreatedAt        time.Time `json:"created_at"`
}

// CreateUserGroupInput carries the payload for creating a user group.
type CreateUserGroupInput struct {
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	PermissionSetIDs []string `json:"permission_set_ids,omitempty"`
	MemberAccountIDs []string `json:"member_account_ids,omitempty"`
}

// PermissionSet groups permissions that can be assigned to accounts or groups.
type PermissionSet struct {
	ID          string       `json:"id"`
	TenantID    string       `json:"tenant_id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Permissions []Permission `json:"permissions"`
	CreatedAt   time.Time    `json:"created_at"`
}

// CreatePermissionSetInput carries the payload for creating a permission set.
type CreatePermissionSetInput struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Permissions []Permission `json:"permissions"`
}

// Permission describes one action over a resource and optional scope.
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

// AssumableRole describes a temporary role that an account may assume.
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

// CreateAssumableRoleInput carries the payload for creating an assumable role.
type CreateAssumableRoleInput struct {
	Name                   string         `json:"name"`
	Description            string         `json:"description,omitempty"`
	PermissionSetIDs       []string       `json:"permission_set_ids,omitempty"`
	Trusted                bool           `json:"trusted"`
	TrustPolicy            map[string]any `json:"trust_policy,omitempty"`
	PermissionBoundary     map[string]any `json:"permission_boundary,omitempty"`
	SessionDurationSeconds int            `json:"session_duration_seconds,omitempty"`
}

// PermissionSetAssignment attaches a permission set to one IAM principal.
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

// CreatePermissionSetAssignmentInput carries the payload for assigning a permission set.
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

// DataScope limits the data visible under a permission assignment.
type DataScope struct {
	ID        string         `json:"id"`
	TenantID  string         `json:"tenant_id"`
	Code      string         `json:"code"`
	Name      string         `json:"name"`
	ScopeType string         `json:"scope_type"`
	Params    map[string]any `json:"params,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// CreateDataScopeInput carries the payload for creating a data scope.
type CreateDataScopeInput struct {
	Code      string         `json:"code"`
	Name      string         `json:"name"`
	ScopeType string         `json:"scope_type"`
	Params    map[string]any `json:"params,omitempty"`
}

// FieldPolicy controls field-level visibility or masking for a resource.
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

// CreateFieldPolicyInput carries the payload for creating a field policy.
type CreateFieldPolicyInput struct {
	ApplicationCode string `json:"application_code"`
	ResourceType    string `json:"resource_type"`
	FieldName       string `json:"field_name"`
	Effect          string `json:"effect"`
	MaskStrategy    string `json:"mask_strategy,omitempty"`
	PermissionID    string `json:"permission_id,omitempty"`
}

// AssumableRoleSession records an active assumed-role session for an account.
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

// AssumeRoleInput carries the requested duration and optional session policy.
type AssumeRoleInput struct {
	Reason          string         `json:"reason,omitempty"`
	DurationMinutes int            `json:"duration_minutes,omitempty"`
	SessionPolicy   map[string]any `json:"session_policy,omitempty"`
}

// PermissionVersion tracks the tenant-wide authorization cache version.
type PermissionVersion struct {
	TenantID string `json:"tenant_id"`
	Version  int64  `json:"version"`
}

// AuthzOutboxEvent records relationship changes waiting for external sync.
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

// AuthzRelationshipTuple is the local representation of an OpenFGA tuple.
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

// AuthzRelationshipTupleOperation identifies a tuple write or delete.
type AuthzRelationshipTupleOperation string

// Authz tuple operation values used by the outbox processor.
const (
	AuthzRelationshipTupleWrite  AuthzRelationshipTupleOperation = "write"
	AuthzRelationshipTupleDelete AuthzRelationshipTupleOperation = "delete"
)

// AuthzRelationshipTupleChange combines an operation with a relationship tuple.
type AuthzRelationshipTupleChange struct {
	Operation AuthzRelationshipTupleOperation `json:"operation"`
	Tuple     AuthzRelationshipTuple          `json:"tuple"`
}

// CheckRequest asks whether the current account can perform an action.
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

// CheckResult describes the authorization decision and the evidence behind it.
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

// BatchCheckRequest groups authorization checks into one request.
type BatchCheckRequest struct {
	Checks []CheckRequest `json:"checks"`
}

// BatchCheckResult returns authorization decisions in request order.
type BatchCheckResult struct {
	Results []CheckResult `json:"results"`
}

// AssumedRoleDecision describes the assumed-role context that influenced a decision.
type AssumedRoleDecision struct {
	SessionID string `json:"session_id,omitempty"`
	RoleID    string `json:"role_id"`
	Name      string `json:"name,omitempty"`
}
