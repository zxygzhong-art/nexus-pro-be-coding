package domain

import "time"

// PermissionType 表示權限點類型。
type PermissionType string

// 下列常數定義權限 catalog 支援的類型。
const (
	PermissionTypeMenu   PermissionType = "menu"
	PermissionTypeAPI    PermissionType = "api"
	PermissionTypeButton PermissionType = "button"
	PermissionTypeField  PermissionType = "field"
	PermissionTypeScope  PermissionType = "scope"
)

// UserGroup 定義使用者群組的資料結構。
type UserGroup struct {
	ID                   string    `json:"id"`
	TenantID             string    `json:"tenant_id"`
	Name                 string    `json:"name"`
	Description          string    `json:"description,omitempty"`
	MemberAccountIDs     []string  `json:"member_account_ids"`
	PermissionSetIDs     []string  `json:"permission_set_ids"`
	SourceTemplateKey    string    `json:"source_template_key,omitempty"`
	SourcePackageVersion string    `json:"source_package_version,omitempty"`
	Version              int64     `json:"version"`
	CreatedAt            time.Time `json:"created_at"`
}

// GroupMembership 定義使用者群組成員關係的資料結構。
type GroupMembership struct {
	ID                 string     `json:"id"`
	TenantID           string     `json:"tenant_id"`
	UserGroupID        string     `json:"user_group_id"`
	AccountID          string     `json:"account_id"`
	ValidFrom          time.Time  `json:"valid_from"`
	ValidUntil         *time.Time `json:"valid_until,omitempty"`
	Source             string     `json:"source"`
	ApprovalInstanceID string     `json:"approval_instance_id,omitempty"`
	CreatedBy          string     `json:"created_by,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

// CreateUserGroupInput 定義使用者群組輸入的資料結構。
type CreateUserGroupInput struct {
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	PermissionSetIDs []string `json:"permission_set_ids,omitempty"`
	MemberAccountIDs []string `json:"member_account_ids,omitempty"`
}

// UpdateUserGroupInput 定義使用者群組更新輸入的資料結構。
type UpdateUserGroupInput struct {
	Name             *string  `json:"name,omitempty"`
	Description      *string  `json:"description,omitempty"`
	PermissionSetIDs []string `json:"permission_set_ids,omitempty"`
}

// AddUserGroupMemberInput 定義新增使用者群組成員輸入的資料結構。
type AddUserGroupMemberInput struct {
	AccountID          string `json:"account_id"`
	ValidUntil         string `json:"valid_until,omitempty"`
	Source             string `json:"source,omitempty"`
	ApprovalInstanceID string `json:"approval_instance_id,omitempty"`
}

// PermissionSet 定義權限集合的資料結構。
type PermissionSet struct {
	ID                   string       `json:"id"`
	TenantID             string       `json:"tenant_id"`
	Name                 string       `json:"name"`
	Description          string       `json:"description,omitempty"`
	Permissions          []Permission `json:"permissions"`
	SourceTemplateKey    string       `json:"source_template_key,omitempty"`
	SourcePackageVersion string       `json:"source_package_version,omitempty"`
	CreatedAt            time.Time    `json:"created_at"`
}

// CreatePermissionSetInput 定義權限集合輸入的資料結構。
type CreatePermissionSetInput struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Permissions []Permission `json:"permissions"`
}

// Permission 定義權限的資料結構。
type Permission struct {
	ID              string          `json:"id,omitempty"`
	TenantID        string          `json:"tenant_id,omitempty"`
	ApplicationCode ApplicationCode `json:"application_code,omitempty"`
	ResourceType    ResourceType    `json:"resource_type,omitempty"`
	PermissionType  PermissionType  `json:"permission_type,omitempty"`
	Resource        string          `json:"resource"`
	Action          Action          `json:"action"`
	Target          string          `json:"target,omitempty"`
	Scope           Scope           `json:"scope,omitempty"`
	Effect          string          `json:"effect,omitempty"`
	RiskLevel       string          `json:"risk_level,omitempty"`
	Severity        string          `json:"severity,omitempty"`
	Relation        string          `json:"relation,omitempty"`
	MenuKey         string          `json:"menu_key,omitempty"`
	Name            string          `json:"name,omitempty"`
	Description     string          `json:"description,omitempty"`
	HighRisk        bool            `json:"high_risk,omitempty"`
}

// PermissionCatalogItem 定義可落庫治理的權限 catalog 項。
type PermissionCatalogItem struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	Application    string         `json:"application"`
	Resource       string         `json:"resource"`
	Action         string         `json:"action"`
	PermissionType PermissionType `json:"permission_type"`
	MenuKey        string         `json:"menu_key,omitempty"`
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	HighRisk       bool           `json:"high_risk"`
	Severity       string         `json:"severity,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// IAMApplication 定義 IAM application 目錄項。
type IAMApplication struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// IAMResourceType 定義 IAM resource type 目錄項。
type IAMResourceType struct {
	ApplicationCode string   `json:"application_code"`
	ResourceType    string   `json:"resource_type"`
	Actions         []string `json:"actions"`
}

// MenuItem 定義落庫的選單項。
type MenuItem struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Key       string    `json:"key"`
	Label     string    `json:"label"`
	Path      string    `json:"path"`
	Icon      string    `json:"icon,omitempty"`
	ParentKey string    `json:"parent_key,omitempty"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

// PermissionSetItem 定義權限集合與權限 catalog 的關聯。
type PermissionSetItem struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	PermissionSetID string    `json:"permission_set_id"`
	PermissionID    string    `json:"permission_id"`
	CreatedAt       time.Time `json:"created_at"`
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
	SourceTemplateKey      string         `json:"source_template_key,omitempty"`
	SourcePackageVersion   string         `json:"source_package_version,omitempty"`
	CreatedAt              time.Time      `json:"created_at"`
}

// PermissionPackageStatus 表示權限包生命週期狀態。
type PermissionPackageStatus string

// 下列常數定義權限包支援的狀態。
const (
	PermissionPackageStatusDraft      PermissionPackageStatus = "draft"
	PermissionPackageStatusPublished  PermissionPackageStatus = "published"
	PermissionPackageStatusDeprecated PermissionPackageStatus = "deprecated"
)

// PermissionPackage 定義可版本化遷移的權限包快照。
type PermissionPackage struct {
	ID              string                   `json:"id"`
	ApplicationCode string                   `json:"application_code"`
	Version         string                   `json:"version"`
	Status          PermissionPackageStatus  `json:"status"`
	Content         PermissionPackageContent `json:"content"`
	Checksum        string                   `json:"checksum"`
	CreatedAt       time.Time                `json:"created_at"`
	PublishedAt     *time.Time               `json:"published_at,omitempty"`
}

// PermissionPackageContent 定義權限包 JSON schema 的根物件。
type PermissionPackageContent struct {
	ApplicationCode        string                          `json:"application_code"`
	Version                string                          `json:"version"`
	ResourceTypes          []PermissionPackageResourceType `json:"resource_types"`
	Actions                []PermissionPackageAction       `json:"actions"`
	Permissions            []Permission                    `json:"permissions"`
	Menus                  []PermissionPackageMenu         `json:"menus"`
	Buttons                []PermissionPackageButton       `json:"buttons"`
	Fields                 []PermissionPackageField        `json:"fields"`
	DataScopes             []PermissionPackageDataScope    `json:"data_scopes"`
	PermissionSetTemplates []PermissionSetTemplateContent  `json:"permission_set_templates"`
	UserGroupTemplates     []UserGroupTemplateContent      `json:"user_group_templates"`
	AssumableRoleTemplates []AssumableRoleTemplateContent  `json:"assumable_role_templates"`
	FGAMappings            []PermissionPackageFGAMapping   `json:"fga_mappings"`
}

// PermissionPackageResourceType 定義包內 resource type 目錄。
type PermissionPackageResourceType struct {
	ApplicationCode string   `json:"application_code"`
	ResourceType    string   `json:"resource_type"`
	Actions         []string `json:"actions"`
	Name            string   `json:"name,omitempty"`
	Description     string   `json:"description,omitempty"`
}

// PermissionPackageAction 定義包內 action 目錄。
type PermissionPackageAction struct {
	Action      string `json:"action"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	HighRisk    bool   `json:"high_risk,omitempty"`
}

// PermissionPackageMenu 定義包內 menu 節點。
type PermissionPackageMenu struct {
	Key       string                  `json:"key"`
	Label     string                  `json:"label"`
	Path      string                  `json:"path,omitempty"`
	Icon      string                  `json:"icon,omitempty"`
	Children  []PermissionPackageMenu `json:"children,omitempty"`
	SortOrder int                     `json:"sort_order,omitempty"`
}

// PermissionPackageButton 定義包內 button 權限。
type PermissionPackageButton struct {
	Key         string `json:"key"`
	Resource    string `json:"resource"`
	Action      string `json:"action"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	HighRisk    bool   `json:"high_risk,omitempty"`
}

// PermissionPackageField 定義包內 field 權限。
type PermissionPackageField struct {
	ResourceType string `json:"resource_type"`
	FieldName    string `json:"field_name"`
	Effect       string `json:"effect"`
	Description  string `json:"description,omitempty"`
}

// PermissionPackageDataScope 定義包內 data scope。
type PermissionPackageDataScope struct {
	Code      string         `json:"code"`
	Name      string         `json:"name"`
	ScopeType string         `json:"scope_type"`
	Params    map[string]any `json:"params,omitempty"`
}

// PermissionSetTemplateContent 定義權限集合模板內容。
type PermissionSetTemplateContent struct {
	TemplateKey string       `json:"template_key"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Permissions []Permission `json:"permissions"`
}

// UserGroupTemplateContent 定義使用者群組模板內容。
type UserGroupTemplateContent struct {
	TemplateKey               string   `json:"template_key"`
	Name                      string   `json:"name"`
	Description               string   `json:"description,omitempty"`
	PermissionSetTemplateKeys []string `json:"permission_set_template_keys"`
}

// AssumableRoleTemplateContent 定義可承擔角色模板內容。
type AssumableRoleTemplateContent struct {
	TemplateKey               string         `json:"template_key"`
	Name                      string         `json:"name"`
	Description               string         `json:"description,omitempty"`
	PermissionSetTemplateKeys []string       `json:"permission_set_template_keys"`
	Trusted                   bool           `json:"trusted"`
	TrustPolicy               map[string]any `json:"trust_policy,omitempty"`
	PermissionBoundary        map[string]any `json:"permission_boundary,omitempty"`
	SessionDurationSeconds    int            `json:"session_duration_seconds,omitempty"`
}

// PermissionPackageFGAMapping 定義 resource_type 到 OpenFGA type 的映射。
type PermissionPackageFGAMapping struct {
	ResourceType string `json:"resource_type"`
	OpenFGAType  string `json:"openfga_type"`
}

// PermissionSetTemplate 定義權限集合模板落庫投影。
type PermissionSetTemplate struct {
	ID          string                       `json:"id"`
	PackageID   string                       `json:"package_id"`
	TemplateKey string                       `json:"template_key"`
	Name        string                       `json:"name"`
	Content     PermissionSetTemplateContent `json:"content"`
	Version     string                       `json:"version"`
}

// UserGroupTemplate 定義使用者群組模板落庫投影。
type UserGroupTemplate struct {
	ID          string                   `json:"id"`
	PackageID   string                   `json:"package_id"`
	TemplateKey string                   `json:"template_key"`
	Name        string                   `json:"name"`
	Content     UserGroupTemplateContent `json:"content"`
	Version     string                   `json:"version"`
}

// AssumableRoleTemplate 定義可承擔角色模板落庫投影。
type AssumableRoleTemplate struct {
	ID          string                       `json:"id"`
	PackageID   string                       `json:"package_id"`
	TemplateKey string                       `json:"template_key"`
	Name        string                       `json:"name"`
	Content     AssumableRoleTemplateContent `json:"content"`
	Version     string                       `json:"version"`
}

// PermissionPackageImport 定義租戶導入權限包的記錄。
type PermissionPackageImport struct {
	ID            string         `json:"id"`
	TenantID      string         `json:"tenant_id"`
	PackageID     string         `json:"package_id"`
	Version       string         `json:"version"`
	ImportedAt    time.Time      `json:"imported_at"`
	ImportedBy    string         `json:"imported_by,omitempty"`
	ArtifactIDMap map[string]any `json:"artifact_id_map"`
}

// PermissionPackageImportDiff 定義升級導入的模板差異。
type PermissionPackageImportDiff struct {
	AddedTemplates    []string `json:"added_templates"`
	ChangedTemplates  []string `json:"changed_templates"`
	OrphanedTemplates []string `json:"orphaned_templates"`
}

// PermissionPackageImportResult 定義權限包導入結果。
type PermissionPackageImportResult struct {
	Import        PermissionPackageImport     `json:"import"`
	Package       PermissionPackage           `json:"package"`
	ArtifactIDMap map[string]any              `json:"artifact_id_map"`
	Diff          PermissionPackageImportDiff `json:"diff"`
	Imported      bool                        `json:"imported"`
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

// UpdateAssumableRoleInput 定義 assumable 角色更新輸入。
type UpdateAssumableRoleInput struct {
	Name                   *string        `json:"name,omitempty"`
	Description            *string        `json:"description,omitempty"`
	PermissionSetIDs       []string       `json:"permission_set_ids,omitempty"`
	Trusted                *bool          `json:"trusted,omitempty"`
	TrustPolicy            map[string]any `json:"trust_policy,omitempty"`
	PermissionBoundary     map[string]any `json:"permission_boundary,omitempty"`
	SessionDurationSeconds *int           `json:"session_duration_seconds,omitempty"`
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

// IAMRoleProjection 定義 roles 相容只讀投影。
type IAMRoleProjection struct {
	ID                     string          `json:"id"`
	TenantID               string          `json:"tenant_id"`
	Name                   string          `json:"name"`
	Description            string          `json:"description,omitempty"`
	PermissionSetIDs       []string        `json:"permission_set_ids"`
	PermissionSets         []PermissionSet `json:"permission_sets"`
	Trusted                bool            `json:"trusted"`
	TrustPolicy            map[string]any  `json:"trust_policy,omitempty"`
	PermissionBoundary     map[string]any  `json:"permission_boundary,omitempty"`
	SessionDurationSeconds int             `json:"session_duration_seconds,omitempty"`
	CreatedAt              time.Time       `json:"created_at"`
}

// IAMRoleBindingProjection 定義 role-bindings 相容只讀投影。
type IAMRoleBindingProjection struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenant_id"`
	PrincipalType   string         `json:"principal_type"`
	PrincipalID     string         `json:"principal_id"`
	PermissionSetID string         `json:"permission_set_id"`
	PermissionSet   *PermissionSet `json:"permission_set,omitempty"`
	Effect          string         `json:"effect"`
	DataScopeID     string         `json:"data_scope_id,omitempty"`
	ConditionID     string         `json:"condition_id,omitempty"`
	StartsAt        *time.Time     `json:"starts_at,omitempty"`
	ExpiresAt       *time.Time     `json:"expires_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
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

// UpdateDataScopeInput 定義資料範圍更新輸入。
type UpdateDataScopeInput struct {
	Code      *string        `json:"code,omitempty"`
	Name      *string        `json:"name,omitempty"`
	ScopeType *string        `json:"scope_type,omitempty"`
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

// UpdateFieldPolicyInput 定義欄位政策更新輸入。
type UpdateFieldPolicyInput struct {
	ApplicationCode *string `json:"application_code,omitempty"`
	ResourceType    *string `json:"resource_type,omitempty"`
	FieldName       *string `json:"field_name,omitempty"`
	Effect          *string `json:"effect,omitempty"`
	MaskStrategy    *string `json:"mask_strategy,omitempty"`
	PermissionID    *string `json:"permission_id,omitempty"`
}

// OutboxEventQuery 定義 outbox 事件查詢。
type OutboxEventQuery struct {
	Status     string `json:"status,omitempty"`
	EventType  string `json:"event_type,omitempty"`
	LastError  string `json:"last_error,omitempty"`
	HasError   *bool  `json:"has_error,omitempty"`
	RetryCount *int   `json:"retry_count,omitempty"`
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
