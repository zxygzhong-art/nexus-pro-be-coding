package server

import (
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/adapters/authorizer"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/adapters/identity"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/audit"
	hrhandler "git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/hr/handler"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/iam/handler"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/middleware"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/repository"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Deps are the dependencies needed to build the router.
type Deps struct {
	GDB       *gorm.DB
	Repo      *repository.Repository
	Authz     authorizer.Authorizer
	Recorder  *audit.Recorder
	Identity  identity.Provider
	Handler   *handler.Handler
	HRHandler *hrhandler.Handler
}

// register wires all routes and middleware onto the engine.
func register(r *gin.Engine, d Deps) {
	r.Use(middleware.Recovery(), middleware.CORS(), middleware.RequestID())

	enf := middleware.NewEnforcer(d.Authz, d.Recorder)

	// Liveness — no auth, no DB.
	r.GET("/healthz", d.Handler.Healthz)

	v1 := r.Group("/v1")
	v1.Use(middleware.Principal(d.Identity, d.GDB, d.Repo))

	// Runtime endpoints: require an authenticated principal, not a specific permission.
	v1.GET("/me", d.Handler.Me)
	v1.GET("/me/menus", d.Handler.Menus)
	v1.POST("/authz/check", d.Handler.AuthzCheck)
	v1.POST("/authz/batch-check", d.Handler.AuthzBatchCheck)
	v1.POST("/authz/explain", d.Handler.Explain)
	v1.POST("/authz/simulate", d.Handler.Simulate)

	// IAM management — gated by iam read/write (config, not HR business logic).
	read := enf.Require("iam", "iam", "read")
	write := enf.Require("iam", "iam", "write")

	iam := v1.Group("/iam")
	iam.GET("/applications", read, d.Handler.ListApplications)
	iam.GET("/resource-types", read, d.Handler.ListResourceTypes)
	iam.GET("/permissions", read, d.Handler.ListPermissions)

	iam.GET("/user-groups", read, d.Handler.ListUserGroups)
	iam.POST("/user-groups", write, d.Handler.CreateUserGroup)

	iam.GET("/permission-sets", read, d.Handler.ListPermissionSets)
	iam.POST("/permission-sets", write, d.Handler.CreatePermissionSet)

	iam.GET("/permission-set-assignments", read, d.Handler.ListAssignments)
	iam.POST("/permission-set-assignments", write, d.Handler.CreateAssignment)

	iam.GET("/field-policies", read, d.Handler.ListFieldPolicies)
	iam.POST("/field-policies", write, d.Handler.CreateFieldPolicy)

	iam.GET("/data-scopes", read, d.Handler.ListDataScopes)
	iam.POST("/data-scopes", write, d.Handler.CreateDataScope)

	iam.GET("/assumable-roles", read, d.Handler.ListAssumableRoles)
	iam.POST("/assumable-roles", write, d.Handler.CreateAssumableRole)
	iam.POST("/assumable-roles/:id/assume", read, d.Handler.Assume)

	// Compatibility shims for the legacy role API.
	iam.GET("/roles", read, d.Handler.ListRoles)
	iam.GET("/role-bindings", read, d.Handler.ListRoleBindings)

	// Audit log query.
	v1.GET("/audit-logs", enf.Require("iam", "audit_log", "read"), d.Handler.ListAuditLogs)

	// HR Core (员工管理) — permission-gated business landing spots from the PRD.
	// Skeleton: handlers return 501 until the HR domain is implemented.
	v1.GET("/hr/employees", enf.Require("hr", "employee", "read"), d.HRHandler.ListEmployees)
	v1.GET("/hr/employees/export", enf.Require("hr", "employee", "export"), d.HRHandler.ExportEmployees)
	v1.POST("/hr/employees", enf.Require("hr", "employee", "write"), d.HRHandler.CreateEmployee)
	v1.POST("/hr/employees/import", enf.Require("hr", "employee", "import"), d.HRHandler.ImportEmployees)
	v1.GET("/hr/employees/:id", enf.Require("hr", "employee", "read"), d.HRHandler.GetEmployee)
	v1.GET("/hr/employees/:id/assignments", enf.Require("hr", "employee", "read"), d.HRHandler.ListEmployeeAssignments)
	v1.PUT("/hr/employees/:id", enf.Require("hr", "employee", "write"), d.HRHandler.UpdateEmployee)
	v1.DELETE("/hr/employees/:id", enf.Require("hr", "employee", "delete"), d.HRHandler.DeleteEmployee)
	v1.GET("/org/units", enf.Require("hr", "org_unit", "read"), d.HRHandler.ListOrgUnits)
}
