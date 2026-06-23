package domain

import (
	"strings"
	"time"
)

type EmployeeStatus string
type EmployeeCategory string

const (
	EmployeeStatusActive         EmployeeStatus = "active"
	EmployeeStatusProbation      EmployeeStatus = "probation"
	EmployeeStatusLeaveSuspended EmployeeStatus = "leave_suspended"
	EmployeeStatusOnboarding     EmployeeStatus = "onboarding"
	EmployeeStatusResigned       EmployeeStatus = "resigned"
	EmployeeStatusDeleted        EmployeeStatus = "deleted"
)

const (
	EmployeeCategoryFullTime   EmployeeCategory = "full_time"
	EmployeeCategoryPartTime   EmployeeCategory = "part_time"
	EmployeeCategoryIntern     EmployeeCategory = "intern"
	EmployeeCategoryContractor EmployeeCategory = "contractor"
	EmployeeCategoryOther      EmployeeCategory = "other"
)

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

type OrgUnit struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Code      string    `json:"code,omitempty"`
	Name      string    `json:"name"`
	ParentID  string    `json:"parent_id,omitempty"`
	Path      []string  `json:"path,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateOrgUnitInput struct {
	Code     string `json:"code,omitempty"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id,omitempty"`
}

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

type EmployeeExperience struct {
	ID                string     `json:"id"`
	StartDate         *time.Time `json:"start_date,omitempty"`
	EndDate           *time.Time `json:"end_date,omitempty"`
	Reason            string     `json:"reason"`
	OrgUnitID         string     `json:"org_unit_id,omitempty"`
	ManagerEmployeeID string     `json:"manager_employee_id,omitempty"`
	Position          string     `json:"position,omitempty"`
	Category          string     `json:"category,omitempty"`
	Current           bool       `json:"current"`
	CreatedAt         time.Time  `json:"created_at"`
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

type EmployeeQuery struct {
	Keyword          string `json:"keyword,omitempty"`
	DepartmentID     string `json:"department_id,omitempty"`
	EmploymentStatus string `json:"employment_status,omitempty"`
	Category         string `json:"category,omitempty"`
	Page             int    `json:"page,omitempty"`
	PageSize         int    `json:"page_size,omitempty"`
	Sort             string `json:"sort,omitempty"`
}

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

type EmployeeOptions struct {
	Departments        []OrgUnit `json:"departments"`
	Positions          []string  `json:"positions"`
	EmploymentStatuses []string  `json:"employment_statuses"`
	Categories         []string  `json:"categories"`
	JobGrades          []string  `json:"job_grades"`
	JobLevels          []string  `json:"job_levels"`
}

type EmployeeImportSession struct {
	ID          string              `json:"id"`
	TenantID    string              `json:"tenant_id"`
	Filename    string              `json:"filename"`
	ObjectKey   string              `json:"object_key,omitempty"`
	Status      string              `json:"status"`
	Rows        []EmployeeImportRow `json:"rows"`
	Summary     map[string]any      `json:"summary,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	ExpiresAt   time.Time           `json:"expires_at"`
	ConfirmedAt *time.Time          `json:"confirmed_at,omitempty"`
}

type EmployeeImportRow struct {
	RowNumber int                 `json:"row_number"`
	Input     map[string]string   `json:"input"`
	Employee  CreateEmployeeInput `json:"employee"`
	Errors    []RowError          `json:"errors,omitempty"`
	Valid     bool                `json:"valid"`
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

type BatchEmployeeResult struct {
	RowNumber  int    `json:"row_number,omitempty"`
	EmployeeID string `json:"employee_id"`
	Success    bool   `json:"success"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
}

type BatchEmployeeResponse struct {
	Results []BatchEmployeeResult `json:"results"`
}

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

func NormalizeEmployeeStatus(raw string) string {
	if status, ok := ParseEmployeeStatus(raw); ok {
		return string(status)
	}
	return strings.TrimSpace(raw)
}

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

func NormalizeEmployeeCategory(raw string) string {
	if category, ok := ParseEmployeeCategory(raw); ok {
		return string(category)
	}
	return strings.TrimSpace(raw)
}

func (c EmployeeCategory) Valid() bool {
	switch c {
	case EmployeeCategoryFullTime, EmployeeCategoryPartTime, EmployeeCategoryIntern, EmployeeCategoryContractor, EmployeeCategoryOther:
		return true
	default:
		return false
	}
}

func EmployeeCategories() []string {
	return []string{
		string(EmployeeCategoryFullTime),
		string(EmployeeCategoryPartTime),
		string(EmployeeCategoryIntern),
		string(EmployeeCategoryContractor),
		string(EmployeeCategoryOther),
	}
}
