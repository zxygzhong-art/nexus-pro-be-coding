package domain

import "time"

type OrgUnit struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Code      string    `json:"code,omitempty"`
	Name      string    `json:"name"`
	ParentID  string    `json:"parent_id,omitempty"`
	Path      []string  `json:"path,omitempty"`
	CreatedAt time.Time `json:"created_at"`
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
