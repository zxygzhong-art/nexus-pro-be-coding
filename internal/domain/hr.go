package domain

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// EmployeeStatus is the canonical employee lifecycle state.
type EmployeeStatus string

// EmployeeCategory classifies the employee's employment contract type.
type EmployeeCategory string

// EmployeeAccountPolicy selects how employee creation handles login accounts.
type EmployeeAccountPolicy string

// Employee lifecycle states accepted by HR APIs and imports.
const (
	EmployeeStatusActive         EmployeeStatus = "active"
	EmployeeStatusProbation      EmployeeStatus = "probation"
	EmployeeStatusLeaveSuspended EmployeeStatus = "leave_suspended"
	EmployeeStatusOnboarding     EmployeeStatus = "onboarding"
	EmployeeStatusResigned       EmployeeStatus = "resigned"
	EmployeeStatusDeleted        EmployeeStatus = "deleted"
)

// Employee category values accepted by HR APIs and imports.
const (
	EmployeeCategoryFullTime   EmployeeCategory = "full_time"
	EmployeeCategoryPartTime   EmployeeCategory = "part_time"
	EmployeeCategoryIntern     EmployeeCategory = "intern"
	EmployeeCategoryContractor EmployeeCategory = "contractor"
	EmployeeCategoryOther      EmployeeCategory = "other"
)

// Employee account creation/linking policies.
const (
	EmployeeAccountPolicyNone                EmployeeAccountPolicy = "none"
	EmployeeAccountPolicyLinkExisting        EmployeeAccountPolicy = "link_existing"
	EmployeeAccountPolicyCreatePendingInvite EmployeeAccountPolicy = "create_pending_invite"
	EmployeeAccountPolicyCreateActive        EmployeeAccountPolicy = "create_active"
)

// Employee domain event names used for audit and authorization synchronization.
const (
	EventEmployeeCreated            EventType = "employee.created"
	EventEmployeeUpdated            EventType = "employee.updated"
	EventEmployeeInvited            EventType = "employee.invited"
	EventEmployeeImported           EventType = "employee.imported"
	EventEmployeeOffboarded         EventType = "employee.offboarded"
	EventEmployeeReinstated         EventType = "employee.reinstated"
	EventEmployeeStatusChanged      EventType = "employee.status_changed"
	EventEmployeeAuthzSubjectCreate EventType = "hr.employee.authz_subject.create"
	EventEmployeeAuthzSubjectUpdate EventType = "hr.employee.authz_subject.update"
	EventEmployeeAuthzSubjectInvite EventType = "hr.employee.authz_subject.invite"
	EventEmployeeAuthzSubjectImport EventType = "hr.employee.authz_subject.import"
)

// OrgUnit represents one node in the tenant's organization hierarchy.
type OrgUnit struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Code      string    `json:"code,omitempty"`
	Name      string    `json:"name"`
	ParentID  string    `json:"parent_id,omitempty"`
	Path      []string  `json:"path,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateOrgUnitInput carries the payload for creating an organization unit.
type CreateOrgUnitInput struct {
	Code     string `json:"code,omitempty"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id,omitempty"`
}

// Employee is the canonical people-domain profile used by HR and IAM workflows.
type Employee struct {
	ID                    string               `json:"id"`
	TenantID              string               `json:"tenant_id"`
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
	Status                string               `json:"status"`
	EmploymentStatus      string               `json:"employment_status,omitempty"`
	HireDate              *time.Time           `json:"hire_date,omitempty"`
	ResignDate            *time.Time           `json:"resign_date,omitempty"`
	BasicInfo             map[string]any       `json:"basic_info,omitempty"`
	EmploymentInfo        map[string]any       `json:"employment_info,omitempty"`
	EducationMilitaryInfo map[string]any       `json:"education_military_info,omitempty"`
	ContactInfo           map[string]any       `json:"contact_info,omitempty"`
	InsuranceInfo         map[string]any       `json:"insurance_info,omitempty"`
	InternalExperiences   []EmployeeExperience `json:"internal_experiences,omitempty"`
	CreatedAt             time.Time            `json:"created_at"`
	UpdatedAt             time.Time            `json:"updated_at"`
}

// EmployeeDetail is the modal/detail contract exposed by HR CRUD endpoints.
// It keeps legacy top-level employee fields while adding typed section DTOs.
type EmployeeDetail struct {
	Employee
	Preview  EmployeeDetailPreview `json:"preview"`
	Sections EmployeeSections      `json:"sections"`
}

// EmployeeDetailPreview contains the fields commonly shown in employee headers.
type EmployeeDetailPreview struct {
	ID                string     `json:"id"`
	EmployeeNo        string     `json:"employee_no,omitempty"`
	Name              string     `json:"name"`
	CompanyEmail      string     `json:"company_email,omitempty"`
	PersonalEmail     string     `json:"personal_email,omitempty"`
	Phone             string     `json:"phone,omitempty"`
	OrgUnitID         string     `json:"org_unit_id,omitempty"`
	AccountID         string     `json:"account_id,omitempty"`
	ManagerEmployeeID string     `json:"manager_employee_id,omitempty"`
	Position          string     `json:"position,omitempty"`
	Category          string     `json:"category,omitempty"`
	Status            string     `json:"status"`
	EmploymentStatus  string     `json:"employment_status,omitempty"`
	HireDate          *time.Time `json:"hire_date,omitempty"`
	ResignDate        *time.Time `json:"resign_date,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// EmployeeSections groups the six profile tabs used by employee modals.
type EmployeeSections struct {
	BasicInfo             EmployeeBasicInfo             `json:"basic_info"`
	EmploymentInfo        EmployeeEmploymentInfo        `json:"employment_info"`
	EducationMilitaryInfo EmployeeEducationMilitaryInfo `json:"education_military_info"`
	ContactInfo           EmployeeContactInfo           `json:"contact_info"`
	InsuranceInfo         EmployeeInsuranceInfo         `json:"insurance_info"`
	InternalExperiences   []EmployeeExperience          `json:"internal_experiences"`
}

// EmployeeBasicInfo is the typed DTO for the employee basic/profile tab.
type EmployeeBasicInfo struct {
	Name                 string         `json:"name,omitempty"`
	CompanyEmail         string         `json:"company_email,omitempty"`
	PersonalEmail        string         `json:"personal_email,omitempty"`
	NationalityType      string         `json:"nationality_type,omitempty"`
	NationalID           string         `json:"national_id,omitempty"`
	PassportNo           string         `json:"passport_no,omitempty"`
	PassportName         string         `json:"passport_name,omitempty"`
	EntryDate            string         `json:"entry_date,omitempty"`
	ARCNo                string         `json:"arc_no,omitempty"`
	ARCExpiryDate        string         `json:"arc_expiry_date,omitempty"`
	TaxID                string         `json:"tax_id,omitempty"`
	WorkPermitNo         string         `json:"work_permit_no,omitempty"`
	WorkPermitExpiryDate string         `json:"work_permit_expiry_date,omitempty"`
	ContractExpiryDate   string         `json:"contract_expiry_date,omitempty"`
	Broker               string         `json:"broker,omitempty"`
	Avatar               any            `json:"avatar,omitempty"`
	Additional           map[string]any `json:"-"`
}

// EmployeeEmploymentInfo is the typed DTO for employment/on-duty data.
type EmployeeEmploymentInfo struct {
	OrgUnitID         string         `json:"org_unit_id,omitempty"`
	Position          string         `json:"position,omitempty"`
	Category          string         `json:"category,omitempty"`
	ManagerEmployeeID string         `json:"manager_employee_id,omitempty"`
	EmploymentStatus  string         `json:"employment_status,omitempty"`
	HireDate          string         `json:"hire_date,omitempty"`
	ResignDate        string         `json:"resign_date,omitempty"`
	ResignReason      string         `json:"resign_reason,omitempty"`
	Shift             string         `json:"shift,omitempty"`
	TenureStartDate   string         `json:"tenure_start_date,omitempty"`
	Additional        map[string]any `json:"-"`
}

// EmployeeEducationMilitaryInfo is the typed DTO for education and military data.
type EmployeeEducationMilitaryInfo struct {
	HighestEducation     string         `json:"highest_education,omitempty"`
	EducationLevel       string         `json:"education_level,omitempty"`
	Degree               string         `json:"degree,omitempty"`
	School               string         `json:"school,omitempty"`
	SchoolName           string         `json:"school_name,omitempty"`
	GraduationDate       string         `json:"graduation_date,omitempty"`
	MilitaryStatus       string         `json:"military_status,omitempty"`
	MilitaryServiceState string         `json:"military_service_status,omitempty"`
	Additional           map[string]any `json:"-"`
}

// EmployeeContactInfo is the typed DTO for communication/contact data.
type EmployeeContactInfo struct {
	MobilePhone              string         `json:"mobile_phone,omitempty"`
	Phone                    string         `json:"phone,omitempty"`
	Address                  string         `json:"address,omitempty"`
	CommunicationAddress     string         `json:"communication_address,omitempty"`
	EmergencyContactRelation string         `json:"emergency_contact_relation,omitempty"`
	EmergencyContactName     string         `json:"emergency_contact_name,omitempty"`
	EmergencyContactPhone    string         `json:"emergency_contact_phone,omitempty"`
	Additional               map[string]any `json:"-"`
}

// EmployeeInsuranceInfo is the typed DTO for insurance data.
type EmployeeInsuranceInfo struct {
	LaborInsuranceDate    string         `json:"labor_insurance_date,omitempty"`
	LaborInsuranceLevel   string         `json:"labor_insurance_level,omitempty"`
	LaborInsuranceSalary  *float64       `json:"labor_insurance_salary,omitempty"`
	HealthInsuranceDate   string         `json:"health_insurance_date,omitempty"`
	HealthInsuranceLevel  string         `json:"health_insurance_level,omitempty"`
	HealthInsuranceAmount *float64       `json:"health_insurance_amount,omitempty"`
	Additional            map[string]any `json:"-"`
}

// EmployeeExperience records one internal employment or position history item.
type EmployeeExperience struct {
	ID                string     `json:"id"`
	StartDate         *time.Time `json:"start_date,omitempty"`
	EndDate           *time.Time `json:"end_date,omitempty"`
	Reason            string     `json:"reason"`
	OrgUnitID         string     `json:"org_unit_id,omitempty"`
	ManagerEmployeeID string     `json:"manager_employee_id,omitempty"`
	Position          string     `json:"position,omitempty"`
	Category          string     `json:"category,omitempty"`
	Status            string     `json:"status,omitempty"`
	Current           bool       `json:"current"`
	CreatedAt         time.Time  `json:"created_at"`
}

// CreateEmployeeInput carries the full employee creation payload.
type CreateEmployeeInput struct {
	EmployeeNo            string               `json:"employee_no,omitempty"`
	Name                  string               `json:"name"`
	CompanyEmail          string               `json:"company_email,omitempty"`
	PersonalEmail         string               `json:"personal_email,omitempty"`
	Phone                 string               `json:"phone,omitempty"`
	OrgUnitID             string               `json:"org_unit_id,omitempty"`
	AccountID             string               `json:"account_id,omitempty"`
	AccountPolicy         string               `json:"account_policy,omitempty"`
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

// UpdateEmployeeInput carries optional employee fields for partial updates.
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

// EmployeeQuery contains filters, pagination, and sorting for employee lists.
type EmployeeQuery struct {
	Keyword          string                  `json:"keyword,omitempty"`
	DepartmentID     string                  `json:"department_id,omitempty"`
	EmploymentStatus string                  `json:"employment_status,omitempty"`
	Category         string                  `json:"category,omitempty"`
	Page             int                     `json:"page,omitempty"`
	PageSize         int                     `json:"page_size,omitempty"`
	Sort             string                  `json:"sort,omitempty"`
	Scope            EmployeeScopeConstraint `json:"-"`
}

// EmployeeScopeConstraint carries an already-authorized data-scope predicate
// from service authorization into repository employee queries.
type EmployeeScopeConstraint struct {
	EmployeeIDs []string `json:"-"`
	OrgUnitIDs  []string `json:"-"`
	Statuses    []string `json:"-"`
	DenyAll     bool     `json:"-"`
}

// EmployeeStats summarizes employee counts for dashboard surfaces.
type EmployeeStats struct {
	Total          int `json:"total"`
	Active         int `json:"active"`
	Onboarding     int `json:"onboarding"`
	Probation      int `json:"probation"`
	LeaveSuspended int `json:"leave_suspended"`
	Resigned       int `json:"resigned"`
	HiredThisMonth int `json:"hired_this_month"`
	LeftThisMonth  int `json:"left_this_month"`
}

// EmployeeOptions returns selectable HR values visible to the current account.
type EmployeeOptions struct {
	Departments        []OrgUnit `json:"departments"`
	Positions          []string  `json:"positions"`
	EmploymentStatuses []string  `json:"employment_statuses"`
	Categories         []string  `json:"categories"`
	JobGrades          []string  `json:"job_grades"`
	JobLevels          []string  `json:"job_levels"`
}

// EmployeeImportSession stores the preview and confirmation state for one import file.
type EmployeeImportSession struct {
	ID                   string              `json:"id"`
	TenantID             string              `json:"tenant_id"`
	Filename             string              `json:"filename"`
	ObjectProvider       string              `json:"object_provider,omitempty"`
	ObjectBucket         string              `json:"object_bucket,omitempty"`
	ObjectKey            string              `json:"object_key,omitempty"`
	ContentType          string              `json:"content_type,omitempty"`
	SizeBytes            int64               `json:"size_bytes,omitempty"`
	SHA256               string              `json:"sha256,omitempty"`
	Status               string              `json:"status"`
	Rows                 []EmployeeImportRow `json:"rows"`
	Summary              map[string]any      `json:"summary,omitempty"`
	CreatedByAccountID   string              `json:"created_by_account_id,omitempty"`
	ConfirmedByAccountID string              `json:"confirmed_by_account_id,omitempty"`
	CreatedAt            time.Time           `json:"created_at"`
	ExpiresAt            time.Time           `json:"expires_at"`
	ConfirmedAt          *time.Time          `json:"confirmed_at,omitempty"`
}

// EmployeeImportRow stores parsed input and validation results for one spreadsheet row.
type EmployeeImportRow struct {
	RowNumber int                 `json:"row_number"`
	Input     map[string]string   `json:"input"`
	Employee  CreateEmployeeInput `json:"employee"`
	Errors    []RowError          `json:"errors,omitempty"`
	Valid     bool                `json:"valid"`
}

// EmployeePreviewResponse returns validation status and calculated diff for employee edits.
type EmployeePreviewResponse struct {
	Employee    Employee       `json:"employee"`
	Detail      EmployeeDetail `json:"detail"`
	FieldErrors []FieldError   `json:"field_errors,omitempty"`
	Diff        map[string]any `json:"diff,omitempty"`
	Valid       bool           `json:"valid"`
}

// EmployeeAvatarInput carries avatar file metadata and bytes.
type EmployeeAvatarInput struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Content     []byte `json:"-"`
}

// EmployeeImportPreviewInput carries a base64 or text import file payload for preview.
type EmployeeImportPreviewInput struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// EmployeeImportConfirmInput selects how a previously previewed import should be applied.
type EmployeeImportConfirmInput struct {
	Mode          string `json:"mode,omitempty"`
	FailurePolicy string `json:"failure_policy,omitempty"`
}

// BatchDeleteEmployeesInput carries the employees and reason for a bulk delete.
type BatchDeleteEmployeesInput struct {
	EmployeeIDs []string `json:"employee_ids"`
	Reason      string   `json:"reason"`
}

// InviteEmployeeInput carries an optional invite email override.
type InviteEmployeeInput struct {
	Email string `json:"email,omitempty"`
}

// UpdateEmployeeStatusInput carries a direct employee status update.
type UpdateEmployeeStatusInput struct {
	Status string `json:"status"`
}

// StatusTransitionInput carries an employee lifecycle transition and its metadata.
type StatusTransitionInput struct {
	Status    string         `json:"status"`
	Reason    string         `json:"reason,omitempty"`
	StartDate string         `json:"start_date,omitempty"`
	EndDate   string         `json:"end_date,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// BatchEmployeeResult reports the outcome for one employee in a bulk operation.
type BatchEmployeeResult struct {
	RowNumber  int    `json:"row_number,omitempty"`
	EmployeeID string `json:"employee_id"`
	Success    bool   `json:"success"`
	Action     string `json:"action,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
}

// BatchEmployeeResponse wraps bulk employee operation results.
type BatchEmployeeResponse struct {
	Results []BatchEmployeeResult `json:"results"`
}

// EmployeeDetailFromEmployee builds the detail/modal response from the stored aggregate.
func EmployeeDetailFromEmployee(employee Employee) EmployeeDetail {
	return EmployeeDetail{
		Employee: employee,
		Preview: EmployeeDetailPreview{
			ID:                employee.ID,
			EmployeeNo:        employee.EmployeeNo,
			Name:              employee.Name,
			CompanyEmail:      employee.CompanyEmail,
			PersonalEmail:     employee.PersonalEmail,
			Phone:             employee.Phone,
			OrgUnitID:         employee.OrgUnitID,
			AccountID:         employee.AccountID,
			ManagerEmployeeID: employee.ManagerEmployeeID,
			Position:          employee.Position,
			Category:          employee.Category,
			Status:            employee.Status,
			EmploymentStatus:  employee.EmploymentStatus,
			HireDate:          employee.HireDate,
			ResignDate:        employee.ResignDate,
			CreatedAt:         employee.CreatedAt,
			UpdatedAt:         employee.UpdatedAt,
		},
		Sections: EmployeeSectionsFromEmployee(employee),
	}
}

// EmployeeSectionsFromEmployee projects hot fields and JSONB sections into typed tabs.
func EmployeeSectionsFromEmployee(employee Employee) EmployeeSections {
	return EmployeeSections{
		BasicInfo: EmployeeBasicInfo{
			Name:                 firstNonEmpty(employee.Name, sectionString(employee.BasicInfo, "name")),
			CompanyEmail:         firstNonEmpty(employee.CompanyEmail, sectionString(employee.BasicInfo, "company_email"), sectionString(employee.BasicInfo, "email")),
			PersonalEmail:        firstNonEmpty(employee.PersonalEmail, sectionString(employee.BasicInfo, "personal_email")),
			NationalityType:      sectionString(employee.BasicInfo, "nationality_type"),
			NationalID:           sectionString(employee.BasicInfo, "national_id"),
			PassportNo:           sectionString(employee.BasicInfo, "passport_no"),
			PassportName:         sectionString(employee.BasicInfo, "passport_name"),
			EntryDate:            sectionString(employee.BasicInfo, "entry_date"),
			ARCNo:                sectionString(employee.BasicInfo, "arc_no"),
			ARCExpiryDate:        sectionString(employee.BasicInfo, "arc_expiry_date"),
			TaxID:                sectionString(employee.BasicInfo, "tax_id"),
			WorkPermitNo:         sectionString(employee.BasicInfo, "work_permit_no"),
			WorkPermitExpiryDate: sectionString(employee.BasicInfo, "work_permit_expiry_date"),
			ContractExpiryDate:   sectionString(employee.BasicInfo, "contract_expiry_date"),
			Broker:               sectionString(employee.BasicInfo, "broker"),
			Avatar:               employee.BasicInfo["avatar"],
			Additional: sectionAdditional(employee.BasicInfo,
				"name", "company_email", "email", "personal_email", "nationality_type", "national_id",
				"passport_no", "passport_name", "entry_date", "arc_no", "arc_expiry_date", "tax_id",
				"work_permit_no", "work_permit_expiry_date", "contract_expiry_date", "broker", "avatar"),
		},
		EmploymentInfo: EmployeeEmploymentInfo{
			OrgUnitID:         firstNonEmpty(employee.OrgUnitID, sectionString(employee.EmploymentInfo, "org_unit_id"), sectionString(employee.EmploymentInfo, "department_id")),
			Position:          firstNonEmpty(employee.Position, sectionString(employee.EmploymentInfo, "position"), sectionString(employee.EmploymentInfo, "job_title")),
			Category:          firstNonEmpty(employee.Category, sectionString(employee.EmploymentInfo, "category")),
			ManagerEmployeeID: firstNonEmpty(employee.ManagerEmployeeID, sectionString(employee.EmploymentInfo, "manager_employee_id")),
			EmploymentStatus:  firstNonEmpty(employee.EmploymentStatus, employee.Status, sectionString(employee.EmploymentInfo, "employment_status")),
			HireDate:          firstNonEmpty(dateString(employee.HireDate), sectionString(employee.EmploymentInfo, "hire_date")),
			ResignDate:        firstNonEmpty(dateString(employee.ResignDate), sectionString(employee.EmploymentInfo, "resign_date")),
			ResignReason:      sectionString(employee.EmploymentInfo, "resign_reason"),
			Shift:             sectionString(employee.EmploymentInfo, "shift"),
			TenureStartDate:   sectionString(employee.EmploymentInfo, "tenure_start_date"),
			Additional: sectionAdditional(employee.EmploymentInfo,
				"org_unit_id", "department_id", "position", "job_title", "category", "manager_employee_id",
				"employment_status", "hire_date", "resign_date", "resign_reason", "shift", "tenure_start_date"),
		},
		EducationMilitaryInfo: EmployeeEducationMilitaryInfo{
			HighestEducation:     firstNonEmpty(sectionString(employee.EducationMilitaryInfo, "highest_education"), sectionString(employee.EducationMilitaryInfo, "education_level"), sectionString(employee.EducationMilitaryInfo, "degree")),
			EducationLevel:       sectionString(employee.EducationMilitaryInfo, "education_level"),
			Degree:               sectionString(employee.EducationMilitaryInfo, "degree"),
			School:               firstNonEmpty(sectionString(employee.EducationMilitaryInfo, "school"), sectionString(employee.EducationMilitaryInfo, "school_name")),
			SchoolName:           sectionString(employee.EducationMilitaryInfo, "school_name"),
			GraduationDate:       sectionString(employee.EducationMilitaryInfo, "graduation_date"),
			MilitaryStatus:       sectionString(employee.EducationMilitaryInfo, "military_status"),
			MilitaryServiceState: sectionString(employee.EducationMilitaryInfo, "military_service_status"),
			Additional: sectionAdditional(employee.EducationMilitaryInfo,
				"highest_education", "education_level", "degree", "school", "school_name",
				"graduation_date", "military_status", "military_service_status"),
		},
		ContactInfo: EmployeeContactInfo{
			MobilePhone:              firstNonEmpty(employee.Phone, sectionString(employee.ContactInfo, "mobile_phone"), sectionString(employee.ContactInfo, "phone")),
			Phone:                    firstNonEmpty(employee.Phone, sectionString(employee.ContactInfo, "phone")),
			Address:                  firstNonEmpty(sectionString(employee.ContactInfo, "address"), sectionString(employee.ContactInfo, "communication_address")),
			CommunicationAddress:     sectionString(employee.ContactInfo, "communication_address"),
			EmergencyContactRelation: firstNonEmpty(sectionString(employee.ContactInfo, "emergency_contact_relation"), sectionString(employee.ContactInfo, "emergency_relation")),
			EmergencyContactName:     firstNonEmpty(sectionString(employee.ContactInfo, "emergency_contact_name"), sectionString(employee.ContactInfo, "emergency_name")),
			EmergencyContactPhone:    firstNonEmpty(sectionString(employee.ContactInfo, "emergency_contact_phone"), sectionString(employee.ContactInfo, "emergency_phone")),
			Additional: sectionAdditional(employee.ContactInfo,
				"mobile_phone", "phone", "address", "communication_address", "emergency_contact_relation",
				"emergency_relation", "emergency_contact_name", "emergency_name", "emergency_contact_phone", "emergency_phone"),
		},
		InsuranceInfo: EmployeeInsuranceInfo{
			LaborInsuranceDate:    sectionString(employee.InsuranceInfo, "labor_insurance_date"),
			LaborInsuranceLevel:   sectionString(employee.InsuranceInfo, "labor_insurance_level"),
			LaborInsuranceSalary:  sectionNumber(employee.InsuranceInfo, "labor_insurance_salary"),
			HealthInsuranceDate:   sectionString(employee.InsuranceInfo, "health_insurance_date"),
			HealthInsuranceLevel:  sectionString(employee.InsuranceInfo, "health_insurance_level"),
			HealthInsuranceAmount: sectionNumber(employee.InsuranceInfo, "health_insurance_amount"),
			Additional: sectionAdditional(employee.InsuranceInfo,
				"labor_insurance_date", "labor_insurance_level", "labor_insurance_salary",
				"health_insurance_date", "health_insurance_level", "health_insurance_amount"),
		},
		InternalExperiences: append([]EmployeeExperience(nil), employee.InternalExperiences...),
	}
}

func (v EmployeeBasicInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(sectionJSON(v.Additional, map[string]any{
		"name":                    v.Name,
		"company_email":           v.CompanyEmail,
		"personal_email":          v.PersonalEmail,
		"nationality_type":        v.NationalityType,
		"national_id":             v.NationalID,
		"passport_no":             v.PassportNo,
		"passport_name":           v.PassportName,
		"entry_date":              v.EntryDate,
		"arc_no":                  v.ARCNo,
		"arc_expiry_date":         v.ARCExpiryDate,
		"tax_id":                  v.TaxID,
		"work_permit_no":          v.WorkPermitNo,
		"work_permit_expiry_date": v.WorkPermitExpiryDate,
		"contract_expiry_date":    v.ContractExpiryDate,
		"broker":                  v.Broker,
		"avatar":                  v.Avatar,
	}))
}

func (v EmployeeEmploymentInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(sectionJSON(v.Additional, map[string]any{
		"org_unit_id":         v.OrgUnitID,
		"position":            v.Position,
		"category":            v.Category,
		"manager_employee_id": v.ManagerEmployeeID,
		"employment_status":   v.EmploymentStatus,
		"hire_date":           v.HireDate,
		"resign_date":         v.ResignDate,
		"resign_reason":       v.ResignReason,
		"shift":               v.Shift,
		"tenure_start_date":   v.TenureStartDate,
	}))
}

func (v EmployeeEducationMilitaryInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(sectionJSON(v.Additional, map[string]any{
		"highest_education":       v.HighestEducation,
		"education_level":         v.EducationLevel,
		"degree":                  v.Degree,
		"school":                  v.School,
		"school_name":             v.SchoolName,
		"graduation_date":         v.GraduationDate,
		"military_status":         v.MilitaryStatus,
		"military_service_status": v.MilitaryServiceState,
	}))
}

func (v EmployeeContactInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(sectionJSON(v.Additional, map[string]any{
		"mobile_phone":               v.MobilePhone,
		"phone":                      v.Phone,
		"address":                    v.Address,
		"communication_address":      v.CommunicationAddress,
		"emergency_contact_relation": v.EmergencyContactRelation,
		"emergency_contact_name":     v.EmergencyContactName,
		"emergency_contact_phone":    v.EmergencyContactPhone,
	}))
}

func (v EmployeeInsuranceInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(sectionJSON(v.Additional, map[string]any{
		"labor_insurance_date":    v.LaborInsuranceDate,
		"labor_insurance_level":   v.LaborInsuranceLevel,
		"labor_insurance_salary":  v.LaborInsuranceSalary,
		"health_insurance_date":   v.HealthInsuranceDate,
		"health_insurance_level":  v.HealthInsuranceLevel,
		"health_insurance_amount": v.HealthInsuranceAmount,
	}))
}

func sectionString(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	switch v := values[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	default:
		return ""
	}
}

func sectionNumber(values map[string]any, key string) *float64 {
	if len(values) == 0 {
		return nil
	}
	switch v := values[key].(type) {
	case float64:
		return &v
	case float32:
		n := float64(v)
		return &n
	case int:
		n := float64(v)
		return &n
	case int64:
		n := float64(v)
		return &n
	case json.Number:
		if n, err := v.Float64(); err == nil {
			return &n
		}
	case string:
		if n, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return &n
		}
	}
	return nil
}

func sectionAdditional(values map[string]any, known ...string) map[string]any {
	if len(values) == 0 {
		return nil
	}
	knownSet := make(map[string]struct{}, len(known))
	for _, key := range known {
		knownSet[key] = struct{}{}
	}
	out := make(map[string]any)
	for key, value := range values {
		if _, ok := knownSet[key]; ok {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sectionJSON(additional map[string]any, fields map[string]any) map[string]any {
	out := make(map[string]any, len(additional)+len(fields))
	for key, value := range additional {
		out[key] = value
	}
	for key, value := range fields {
		if sectionValuePresent(value) {
			out[key] = value
		}
	}
	return out
}

func sectionValuePresent(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case *float64:
		return v != nil
	default:
		return true
	}
}

func dateString(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// ParseEmployeeStatus normalizes supported localized and API employee status values.
func ParseEmployeeStatus(raw string) (EmployeeStatus, bool) {
	switch strings.TrimSpace(raw) {
	case "在職", "active":
		return EmployeeStatusActive, true
	case "試用中", "probation":
		return EmployeeStatusProbation, true
	case "留停", "on-leave", "leave_suspended":
		return EmployeeStatusLeaveSuspended, true
	case "待加入", "pending", "onboarding":
		return EmployeeStatusOnboarding, true
	case "離職", "resigned":
		return EmployeeStatusResigned, true
	case "已停用", "deleted":
		return EmployeeStatusDeleted, true
	default:
		return "", false
	}
}

// NormalizeEmployeeStatus returns a canonical status value when recognized.
func NormalizeEmployeeStatus(raw string) string {
	if status, ok := ParseEmployeeStatus(raw); ok {
		return string(status)
	}
	return strings.TrimSpace(raw)
}

// ParseEmployeeAccountPolicy normalizes supported employee account lifecycle policies.
func ParseEmployeeAccountPolicy(raw string) (EmployeeAccountPolicy, bool) {
	switch strings.TrimSpace(raw) {
	case "", "none":
		return EmployeeAccountPolicyNone, true
	case "link_existing":
		return EmployeeAccountPolicyLinkExisting, true
	case "create_pending_invite", "pending_invite":
		return EmployeeAccountPolicyCreatePendingInvite, true
	case "create_active", "active":
		return EmployeeAccountPolicyCreateActive, true
	default:
		return "", false
	}
}

// NormalizeEmployeeAccountPolicy returns a canonical account policy value when recognized.
func NormalizeEmployeeAccountPolicy(raw string) string {
	if policy, ok := ParseEmployeeAccountPolicy(raw); ok {
		return string(policy)
	}
	return strings.TrimSpace(raw)
}

// Valid reports whether the status can be used in employee write paths.
func (s EmployeeStatus) Valid(includeDeleted bool) bool {
	switch s {
	case EmployeeStatusActive, EmployeeStatusProbation, EmployeeStatusLeaveSuspended, EmployeeStatusOnboarding, EmployeeStatusResigned:
		return true
	case EmployeeStatusDeleted:
		return includeDeleted
	default:
		return false
	}
}

// EmployeeStatuses returns the canonical status list for option endpoints.
func EmployeeStatuses(includeDeleted bool) []string {
	statuses := []string{
		string(EmployeeStatusActive),
		string(EmployeeStatusProbation),
		string(EmployeeStatusLeaveSuspended),
		string(EmployeeStatusOnboarding),
		string(EmployeeStatusResigned),
	}
	if includeDeleted {
		statuses = append(statuses, string(EmployeeStatusDeleted))
	}
	return statuses
}

// ParseEmployeeCategory normalizes supported localized and API category values.
func ParseEmployeeCategory(raw string) (EmployeeCategory, bool) {
	switch strings.TrimSpace(raw) {
	case "全職", "正職", "full-time", "full_time":
		return EmployeeCategoryFullTime, true
	case "兼職", "part-time", "part_time":
		return EmployeeCategoryPartTime, true
	case "實習", "intern":
		return EmployeeCategoryIntern, true
	case "約聘", "contract", "contractor":
		return EmployeeCategoryContractor, true
	case "其他", "other":
		return EmployeeCategoryOther, true
	default:
		return "", false
	}
}

// NormalizeEmployeeCategory returns a canonical category value when recognized.
func NormalizeEmployeeCategory(raw string) string {
	if category, ok := ParseEmployeeCategory(raw); ok {
		return string(category)
	}
	return strings.TrimSpace(raw)
}

// Valid reports whether the employee category is accepted by write paths.
func (c EmployeeCategory) Valid() bool {
	switch c {
	case EmployeeCategoryFullTime, EmployeeCategoryPartTime, EmployeeCategoryIntern, EmployeeCategoryContractor, EmployeeCategoryOther:
		return true
	default:
		return false
	}
}

// EmployeeCategories returns the canonical category list for option endpoints.
func EmployeeCategories() []string {
	return []string{
		string(EmployeeCategoryFullTime),
		string(EmployeeCategoryPartTime),
		string(EmployeeCategoryIntern),
		string(EmployeeCategoryContractor),
		string(EmployeeCategoryOther),
	}
}
