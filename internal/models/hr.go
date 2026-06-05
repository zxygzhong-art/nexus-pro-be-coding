package models

import (
	"time"

	"gorm.io/datatypes"
)

// OrgUnit is an organization node (HR Core). Business logic TBD.
type OrgUnit struct {
	TenantModel
	SoftDelete
	ParentID          string `gorm:"column:parent_id" json:"parent_id"`
	Code              string `json:"code"`
	Name              string `json:"name"`
	ManagerEmployeeID string `gorm:"column:manager_employee_id" json:"manager_employee_id"`
	Level             int    `json:"level"`
	SortOrder         int    `gorm:"column:sort_order" json:"sort_order"`
}

func (OrgUnit) TableName() string { return "hr_org_units" }

// Employee is an employee record (HR Core, 员工管理). Schema mirrors the PRD's
// six tabs: queryable/state fields are columns; self-contained optional sections
// are JSONB. Business logic (CRUD/import/export/state machine) is not implemented.
type Employee struct {
	TenantModel
	SoftDelete

	// 基本资料
	EmployeeNo        string     `gorm:"column:employee_no" json:"employee_no"`
	CardNo            string     `gorm:"column:card_no" json:"card_no"`
	PhotoURL          string     `gorm:"column:photo_url" json:"photo_url"`
	NationalityType   string     `gorm:"column:nationality_type" json:"nationality_type"`
	NameZh            string     `gorm:"column:name_zh" json:"name_zh"`
	FirstName         string     `gorm:"column:first_name" json:"first_name"`
	LastName          string     `gorm:"column:last_name" json:"last_name"`
	CompanyEmail      string     `gorm:"column:company_email" json:"company_email"`
	PersonalEmail     string     `gorm:"column:personal_email" json:"personal_email"`
	OfficePhoneExt    string     `gorm:"column:office_phone_ext" json:"office_phone_ext"`
	Mobile            string     `json:"mobile"`
	Gender            string     `json:"gender"`
	Birthday          *time.Time `json:"birthday"`
	Birthplace        string     `json:"birthplace"`
	Nationality       string     `json:"nationality"`
	MaritalStatus     string     `gorm:"column:marital_status" json:"marital_status"`
	HasCriminalRecord *bool      `gorm:"column:has_criminal_record" json:"has_criminal_record"`

	// 在职资料
	AccountID                  string     `gorm:"column:account_id" json:"account_id"`
	CompanyName                string     `gorm:"column:company_name" json:"company_name"`
	OrgUnitID                  string     `gorm:"column:org_unit_id" json:"org_unit_id"`
	Title                      string     `json:"title"`
	JobGrade                   string     `gorm:"column:job_grade" json:"job_grade"`
	JobLevel                   string     `gorm:"column:job_level" json:"job_level"`
	IsManager                  bool       `gorm:"column:is_manager" json:"is_manager"`
	SupervisorEmployeeID       string     `gorm:"column:supervisor_employee_id" json:"supervisor_employee_id"`
	DeputyEmployeeID           string     `gorm:"column:deputy_employee_id" json:"deputy_employee_id"`
	HireDate                   *time.Time `gorm:"column:hire_date" json:"hire_date"`
	ProbationEndDate           *time.Time `gorm:"column:probation_end_date" json:"probation_end_date"`
	ExpectedRegularizationDate *time.Time `gorm:"column:expected_regularization_date" json:"expected_regularization_date"`
	RecruitmentSource          string     `gorm:"column:recruitment_source" json:"recruitment_source"`
	EmploymentStatus           string     `gorm:"column:employment_status" json:"employment_status"`
	Shift                      string     `json:"shift"`
	Category                   string     `json:"category"`
	SeniorityStartDate         *time.Time `gorm:"column:seniority_start_date" json:"seniority_start_date"`
	ClockInOut                 *bool      `gorm:"column:clock_in_out" json:"clock_in_out"`
	ResponsibilityType         string     `gorm:"column:responsibility_type" json:"responsibility_type"`
	Remark                     string     `json:"remark"`

	// status-conditional
	LeaveStartDate *time.Time `gorm:"column:leave_start_date" json:"leave_start_date"`
	LeaveEndDate   *time.Time `gorm:"column:leave_end_date" json:"leave_end_date"`
	ResignDate     *time.Time `gorm:"column:resign_date" json:"resign_date"`
	ResignReason   string     `gorm:"column:resign_reason" json:"resign_reason"`

	// optional tabs (JSONB sections; see migration for documented keys)
	RegulatoryIdentity datatypes.JSON `gorm:"column:regulatory_identity" json:"regulatory_identity"`
	ForeignProfile     datatypes.JSON `gorm:"column:foreign_profile" json:"foreign_profile"`
	Physiological      datatypes.JSON `json:"physiological"`
	Education          datatypes.JSON `json:"education"`
	MilitaryService    datatypes.JSON `gorm:"column:military_service" json:"military_service"`
	Contact            datatypes.JSON `json:"contact"`
	EmergencyContact   datatypes.JSON `gorm:"column:emergency_contact" json:"emergency_contact"`
	Insurance          datatypes.JSON `json:"insurance"`
}

func (Employee) TableName() string { return "hr_employees" }

// EmployeeAssignment is one row of 内部经历 / 异动历史 (1:N to Employee).
type EmployeeAssignment struct {
	TenantModel
	EmployeeID   string     `gorm:"column:employee_id" json:"employee_id"`
	StartDate    *time.Time `gorm:"column:start_date" json:"start_date"`
	EndDate      *time.Time `gorm:"column:end_date" json:"end_date"`
	ChangeReason string     `gorm:"column:change_reason" json:"change_reason"` // 新进/转调/升迁/降调/留停复职
	OrgUnitID    string     `gorm:"column:org_unit_id" json:"org_unit_id"`
	Title        string     `json:"title"`
	Category     string     `json:"category"`
	IsCurrent    bool       `gorm:"column:is_current" json:"is_current"`
}

func (EmployeeAssignment) TableName() string { return "hr_employee_assignments" }
