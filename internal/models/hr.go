package models

import "time"

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

// Employee is the employee single-source-of-truth (HR Core, 员工管理). Columns
// mirror the PRD's six tabs faithfully. Business logic (CRUD/import/export/state
// machine) is not implemented in this milestone.
type Employee struct {
	TenantModel
	SoftDelete

	// 分页1 基本资料
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
	Gender            string     `json:"gender"`
	Birthday          *time.Time `json:"birthday"`
	Birthplace        string     `json:"birthplace"`
	Nationality       string     `json:"nationality"`
	MaritalStatus     string     `gorm:"column:marital_status" json:"marital_status"`
	HasCriminalRecord *bool      `gorm:"column:has_criminal_record" json:"has_criminal_record"`
	// 法规身份
	IDNumber           string `gorm:"column:id_number" json:"id_number"`
	NHISubsidyIdentity string `gorm:"column:nhi_subsidy_identity" json:"nhi_subsidy_identity"`
	IndigenousIdentity *bool  `gorm:"column:indigenous_identity" json:"indigenous_identity"`
	DisabilityCategory string `gorm:"column:disability_category" json:"disability_category"`
	DisabilityLevel    string `gorm:"column:disability_level" json:"disability_level"`
	// 外籍员工资料
	PassportNo       string     `gorm:"column:passport_no" json:"passport_no"`
	PassportName     string     `gorm:"column:passport_name" json:"passport_name"`
	EntryDate        *time.Time `gorm:"column:entry_date" json:"entry_date"`
	ARCNo            string     `gorm:"column:arc_no" json:"arc_no"`
	ARCExpiry        *time.Time `gorm:"column:arc_expiry" json:"arc_expiry"`
	TaxID            string     `gorm:"column:tax_id" json:"tax_id"`
	WorkPermitNo     string     `gorm:"column:work_permit_no" json:"work_permit_no"`
	WorkPermitExpiry *time.Time `gorm:"column:work_permit_expiry" json:"work_permit_expiry"`
	ContractExpiry   *time.Time `gorm:"column:contract_expiry" json:"contract_expiry"`
	Agency           string     `json:"agency"`
	// 生理资料
	BloodType string   `gorm:"column:blood_type" json:"blood_type"`
	HeightCm  *float64 `gorm:"column:height_cm" json:"height_cm"`
	WeightKg  *float64 `gorm:"column:weight_kg" json:"weight_kg"`

	// 分页2 在职资料
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
	LeaveStartDate             *time.Time `gorm:"column:leave_start_date" json:"leave_start_date"`
	LeaveEndDate               *time.Time `gorm:"column:leave_end_date" json:"leave_end_date"`
	ResignDate                 *time.Time `gorm:"column:resign_date" json:"resign_date"`
	ResignReason               string     `gorm:"column:resign_reason" json:"resign_reason"`

	// 分页3 学历兵役
	HighestEducation string     `gorm:"column:highest_education" json:"highest_education"`
	Degree           string     `json:"degree"`
	SchoolName       string     `gorm:"column:school_name" json:"school_name"`
	Major            string     `json:"major"`
	EnrollmentDate   *time.Time `gorm:"column:enrollment_date" json:"enrollment_date"`
	GraduationStatus string     `gorm:"column:graduation_status" json:"graduation_status"`
	GraduationDate   *time.Time `gorm:"column:graduation_date" json:"graduation_date"`
	WithdrawalDate   *time.Time `gorm:"column:withdrawal_date" json:"withdrawal_date"`
	MilitaryStatus   string     `gorm:"column:military_status" json:"military_status"`
	MilitaryBranch   string     `gorm:"column:military_branch" json:"military_branch"`
	MilitaryRank     string     `gorm:"column:military_rank" json:"military_rank"`

	// 分页4 通讯资料
	Mobile                string `json:"mobile"`
	HouseholdPhone        string `gorm:"column:household_phone" json:"household_phone"`
	ContactPhone          string `gorm:"column:contact_phone" json:"contact_phone"`
	HouseholdAddress      string `gorm:"column:household_address" json:"household_address"`
	ContactAddress        string `gorm:"column:contact_address" json:"contact_address"`
	EmergencyRelationship string `gorm:"column:emergency_relationship" json:"emergency_relationship"`
	EmergencyName         string `gorm:"column:emergency_name" json:"emergency_name"`
	EmergencyPhone        string `gorm:"column:emergency_phone" json:"emergency_phone"`
	EmergencyAddress      string `gorm:"column:emergency_address" json:"emergency_address"`

	// 分页5 保险资料
	LaborInsuranceDate       *time.Time `gorm:"column:labor_insurance_date" json:"labor_insurance_date"`
	LaborInsuranceGrade      string     `gorm:"column:labor_insurance_grade" json:"labor_insurance_grade"`
	LaborInsuranceSalary     *float64   `gorm:"column:labor_insurance_salary" json:"labor_insurance_salary"`
	OccupationalInjuryGrade  string     `gorm:"column:occupational_injury_grade" json:"occupational_injury_grade"`
	OccupationalInjurySalary *float64   `gorm:"column:occupational_injury_salary" json:"occupational_injury_salary"`
	NHIDate                  *time.Time `gorm:"column:nhi_date" json:"nhi_date"`
	NHIGrade                 string     `gorm:"column:nhi_grade" json:"nhi_grade"`
	NHIAmount                *float64   `gorm:"column:nhi_amount" json:"nhi_amount"`
	NHIDependents            *int       `gorm:"column:nhi_dependents" json:"nhi_dependents"`
	DisabledDependents       *int       `gorm:"column:disabled_dependents" json:"disabled_dependents"`
	SubsidizedDependents     *int       `gorm:"column:subsidized_dependents" json:"subsidized_dependents"`
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
