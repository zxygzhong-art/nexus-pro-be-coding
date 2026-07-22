package service

import "nexus-pro-api/internal/repository"

type meStore interface {
	repository.AccountStore
	repository.IAMStore
	repository.IdentityStore
	repository.EmployeeStore
	repository.OrgStore
	repository.AuditStore
}

type identityStore interface {
	repository.TenantStore
	repository.AccountStore
	repository.IdentityStore
}

type iamStore interface {
	repository.AccountStore
	repository.IAMStore
}

type hrStore interface {
	repository.AccountStore
	repository.OrgStore
	repository.PositionStore
	repository.EmployeeStore
	repository.AuthzEventStore
	repository.OutboxStore
}

type attendanceStore interface {
	repository.AttendanceStore
	repository.EmployeeStore
	repository.OrgStore
	repository.WorkflowStore
}

type workflowStore interface {
	repository.AccountStore
	repository.EmployeeStore
	repository.IAMStore
	repository.WorkflowStore
	repository.NotificationStore
	repository.OutboxStore
}

type auditStore interface {
	repository.AuditStore
}

// withTransaction 附加 transaction 的服務流程。
func (c MeService) withTransaction(ctx RequestContext, fn func(MeService) error) error {
	return c.Service.withTenantTransaction(ctx, func(tx *Service) error {
		return fn(tx.Me())
	})
}

// withTransaction 附加 transaction 的服務流程。
func (c IAMService) withTransaction(ctx RequestContext, fn func(IAMService) error) error {
	return c.Service.withTenantTransaction(ctx, func(tx *Service) error {
		return fn(tx.IAM())
	})
}

// withTransaction 附加 transaction 的服務流程。
func (c HRService) withTransaction(ctx RequestContext, fn func(HRService) error) error {
	return c.Service.withTenantTransaction(ctx, func(tx *Service) error {
		return fn(tx.HR())
	})
}

// withTransaction 附加 transaction 的服務流程。
func (c AttendanceService) withTransaction(ctx RequestContext, fn func(AttendanceService) error) error {
	return c.Service.withTenantTransaction(ctx, func(tx *Service) error {
		return fn(tx.Attendance())
	})
}

// withTransaction 附加 transaction 的服務流程。
func (c WorkflowService) withTransaction(ctx RequestContext, fn func(WorkflowService) error) error {
	return c.Service.withTenantTransaction(ctx, func(tx *Service) error {
		return fn(tx.Workflow())
	})
}
