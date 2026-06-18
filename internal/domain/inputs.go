package domain

type CreateUserGroupInput struct {
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	PermissionSetIDs []string `json:"permission_set_ids,omitempty"`
	MemberAccountIDs []string `json:"member_account_ids,omitempty"`
}

type CreatePermissionSetInput struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Permissions []Permission `json:"permissions"`
}

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

type CreateFieldPolicyInput struct {
	ApplicationCode string `json:"application_code"`
	ResourceType    string `json:"resource_type"`
	FieldName       string `json:"field_name"`
	Effect          string `json:"effect"`
	MaskStrategy    string `json:"mask_strategy,omitempty"`
	PermissionID    string `json:"permission_id,omitempty"`
}

type CreateDataScopeInput struct {
	Code      string         `json:"code"`
	Name      string         `json:"name"`
	ScopeType string         `json:"scope_type"`
	Params    map[string]any `json:"params,omitempty"`
}

type CreateAssumableRoleInput struct {
	Name                   string         `json:"name"`
	Description            string         `json:"description,omitempty"`
	PermissionSetIDs       []string       `json:"permission_set_ids,omitempty"`
	Trusted                bool           `json:"trusted"`
	TrustPolicy            map[string]any `json:"trust_policy,omitempty"`
	PermissionBoundary     map[string]any `json:"permission_boundary,omitempty"`
	SessionDurationSeconds int            `json:"session_duration_seconds,omitempty"`
}

type AssumeRoleInput struct {
	Reason          string         `json:"reason,omitempty"`
	DurationMinutes int            `json:"duration_minutes,omitempty"`
	SessionPolicy   map[string]any `json:"session_policy,omitempty"`
}

type CreateOrgUnitInput struct {
	Code     string `json:"code,omitempty"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id,omitempty"`
}

type CreateEmployeeInput struct {
	EmployeeNo            string               `json:"employee_no,omitempty"`
	Name                  string               `json:"name"`
	CompanyEmail          string               `json:"company_email,omitempty"`
	PersonalEmail         string               `json:"personal_email,omitempty"`
	Phone                 string               `json:"phone,omitempty"`
	OrgUnitID             string               `json:"org_unit_id,omitempty"`
	AccountID             string               `json:"account_id,omitempty"`
	ManagerEmployeeID     string               `json:"manager_employee_id,omitempty"`
	Position              string               `json:"position,omitempty"`
	Category              string               `json:"category,omitempty"`
	Status                string               `json:"status,omitempty"`
	EmploymentStatus      string               `json:"employment_status,omitempty"`
	HireDate              string               `json:"hire_date,omitempty"`
	ResignDate            string               `json:"resign_date,omitempty"`
	BasicInfo             map[string]any       `json:"basic_info,omitempty"`
	EmploymentInfo        map[string]any       `json:"employment_info,omitempty"`
	EducationMilitaryInfo map[string]any       `json:"education_military_info,omitempty"`
	ContactInfo           map[string]any       `json:"contact_info,omitempty"`
	InsuranceInfo         map[string]any       `json:"insurance_info,omitempty"`
	InternalExperiences   []EmployeeExperience `json:"internal_experiences,omitempty"`
}

type UpdateEmployeeInput struct {
	EmployeeNo            *string              `json:"employee_no,omitempty"`
	Name                  *string              `json:"name,omitempty"`
	CompanyEmail          *string              `json:"company_email,omitempty"`
	PersonalEmail         *string              `json:"personal_email,omitempty"`
	Phone                 *string              `json:"phone,omitempty"`
	OrgUnitID             *string              `json:"org_unit_id,omitempty"`
	AccountID             *string              `json:"account_id,omitempty"`
	ManagerEmployeeID     *string              `json:"manager_employee_id,omitempty"`
	Position              *string              `json:"position,omitempty"`
	Category              *string              `json:"category,omitempty"`
	Status                *string              `json:"status,omitempty"`
	EmploymentStatus      *string              `json:"employment_status,omitempty"`
	HireDate              *string              `json:"hire_date,omitempty"`
	ResignDate            *string              `json:"resign_date,omitempty"`
	BasicInfo             map[string]any       `json:"basic_info,omitempty"`
	EmploymentInfo        map[string]any       `json:"employment_info,omitempty"`
	EducationMilitaryInfo map[string]any       `json:"education_military_info,omitempty"`
	ContactInfo           map[string]any       `json:"contact_info,omitempty"`
	InsuranceInfo         map[string]any       `json:"insurance_info,omitempty"`
	InternalExperiences   []EmployeeExperience `json:"internal_experiences,omitempty"`
}

type EmployeePreviewResponse struct {
	Employee    Employee       `json:"employee"`
	FieldErrors []FieldError   `json:"field_errors,omitempty"`
	Diff        map[string]any `json:"diff,omitempty"`
	Valid       bool           `json:"valid"`
}

type EmployeeAvatarInput struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Content     []byte `json:"-"`
}

type EmployeeImportPreviewInput struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

type EmployeeImportConfirmInput struct {
	Mode string `json:"mode,omitempty"`
}

type BatchDeleteEmployeesInput struct {
	EmployeeIDs []string `json:"employee_ids"`
	Reason      string   `json:"reason"`
}

type InviteEmployeeInput struct {
	Email string `json:"email,omitempty"`
}

type UpdateEmployeeStatusInput struct {
	Status string `json:"status"`
}

type StatusTransitionInput struct {
	Status    string         `json:"status"`
	Reason    string         `json:"reason,omitempty"`
	StartDate string         `json:"start_date,omitempty"`
	EndDate   string         `json:"end_date,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

type CreateFormTemplateInput struct {
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema,omitempty"`
}

type SubmitFormInput struct {
	TemplateKey string         `json:"template_key"`
	Payload     map[string]any `json:"payload,omitempty"`
}

type ApproveFormInput struct{}

type CreateLeaveRequestInput struct {
	EmployeeID string  `json:"employee_id,omitempty"`
	LeaveType  string  `json:"leave_type"`
	StartAt    string  `json:"start_at"`
	EndAt      string  `json:"end_at"`
	Hours      float64 `json:"hours"`
	Reason     string  `json:"reason,omitempty"`
}

type CreateAgentRunInput struct {
	Mode   string `json:"mode,omitempty"`
	Prompt string `json:"prompt"`
}
