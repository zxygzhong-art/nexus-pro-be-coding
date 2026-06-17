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
