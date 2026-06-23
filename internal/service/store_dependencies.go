package service

import "nexus-pro-be/internal/repository"

type meStore interface {
	repository.EmployeeStore
}

type iamStore interface {
	repository.AccountStore
	repository.IAMStore
}

type hrStore interface {
	repository.AccountStore
	repository.OrgStore
	repository.EmployeeStore
	repository.AuthzEventStore
}

type attendanceStore interface {
	repository.AttendanceStore
	repository.EmployeeStore
	repository.OrgStore
	repository.WorkflowStore
}

type workflowStore interface {
	repository.WorkflowStore
}

type agentStore interface {
	repository.AgentStore
}

type auditStore interface {
	repository.AuditStore
}

func (c IAMService) withTransaction(ctx RequestContext, fn func(IAMService) error) error {
	return c.Service.withTenantTransaction(ctx, func(tx *Service) error {
		return fn(tx.IAM())
	})
}

func (c HRService) withTransaction(ctx RequestContext, fn func(HRService) error) error {
	return c.Service.withTenantTransaction(ctx, func(tx *Service) error {
		return fn(tx.HR())
	})
}

func (c AttendanceService) withTransaction(ctx RequestContext, fn func(AttendanceService) error) error {
	return c.Service.withTenantTransaction(ctx, func(tx *Service) error {
		return fn(tx.Attendance())
	})
}

func (c WorkflowService) withTransaction(ctx RequestContext, fn func(WorkflowService) error) error {
	return c.Service.withTenantTransaction(ctx, func(tx *Service) error {
		return fn(tx.Workflow())
	})
}
