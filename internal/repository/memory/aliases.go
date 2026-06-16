package memory

import "nexus-pro-be/internal/domain"

type Tenant = domain.Tenant
type Account = domain.Account
type UserGroup = domain.UserGroup
type PermissionSet = domain.PermissionSet
type Permission = domain.Permission
type PermissionSetAssignment = domain.PermissionSetAssignment
type DataScope = domain.DataScope
type FieldPolicy = domain.FieldPolicy
type AssumableRole = domain.AssumableRole
type AssumableRoleSession = domain.AssumableRoleSession
type OrgUnit = domain.OrgUnit
type Employee = domain.Employee
type EmployeeExperience = domain.EmployeeExperience
type EmployeeQuery = domain.EmployeeQuery
type EmployeeImportSession = domain.EmployeeImportSession
type EmployeeImportRow = domain.EmployeeImportRow
type LeaveBalance = domain.LeaveBalance
type LeaveRequest = domain.LeaveRequest
type FormTemplate = domain.FormTemplate
type FormInstance = domain.FormInstance
type KnowledgeArticle = domain.KnowledgeArticle
type Reference = domain.Reference
type AgentRun = domain.AgentRun
type CheckResult = domain.CheckResult
type RowError = domain.RowError
type AuditLog = domain.AuditLog
type AuthzOutboxEvent = domain.AuthzOutboxEvent
type PageRequest = domain.PageRequest

const DefaultPage = domain.DefaultPage
const DefaultPageSize = domain.DefaultPageSize
const MaxPageSize = domain.MaxPageSize
