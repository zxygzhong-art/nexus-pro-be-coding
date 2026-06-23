package memory

import "nexus-pro-be/internal/utils"

func copyPermissions(src []Permission) []Permission {
	if len(src) == 0 {
		return nil
	}
	dst := make([]Permission, len(src))
	copy(dst, src)
	return dst
}

func copyRefs(src []Reference) []Reference {
	if len(src) == 0 {
		return nil
	}
	dst := make([]Reference, len(src))
	copy(dst, src)
	return dst
}

func copyTenant(v Tenant) Tenant { return v }

func copyAccount(v Account) Account {
	v.UserGroupIDs = utils.CopyStrings(v.UserGroupIDs)
	v.DirectPermissionSetIDs = utils.CopyStrings(v.DirectPermissionSetIDs)
	return v
}

func copyUserGroup(v UserGroup) UserGroup {
	v.MemberAccountIDs = utils.CopyStrings(v.MemberAccountIDs)
	v.PermissionSetIDs = utils.CopyStrings(v.PermissionSetIDs)
	return v
}

func copyPermissionSet(v PermissionSet) PermissionSet {
	v.Permissions = copyPermissions(v.Permissions)
	return v
}

func copyPermissionSetAssignment(v PermissionSetAssignment) PermissionSetAssignment {
	if v.StartsAt != nil {
		t := *v.StartsAt
		v.StartsAt = &t
	}
	if v.ExpiresAt != nil {
		t := *v.ExpiresAt
		v.ExpiresAt = &t
	}
	return v
}

func copyDataScope(v DataScope) DataScope {
	v.Params = utils.CopyStringMap(v.Params)
	return v
}

func copyFieldPolicy(v FieldPolicy) FieldPolicy { return v }

func copyAssumableRole(v AssumableRole) AssumableRole {
	v.PermissionSetIDs = utils.CopyStrings(v.PermissionSetIDs)
	v.TrustPolicy = utils.CopyStringMap(v.TrustPolicy)
	v.PermissionBoundary = utils.CopyStringMap(v.PermissionBoundary)
	return v
}

func copyAssumableRoleSession(v AssumableRoleSession) AssumableRoleSession {
	v.SessionPolicy = utils.CopyStringMap(v.SessionPolicy)
	v.PermissionBoundary = utils.CopyStringMap(v.PermissionBoundary)
	if v.RevokedAt != nil {
		t := *v.RevokedAt
		v.RevokedAt = &t
	}
	return v
}

func copyOrgUnit(v OrgUnit) OrgUnit {
	v.Path = utils.CopyStrings(v.Path)
	return v
}

func copyEmployee(v Employee) Employee {
	if v.HireDate != nil {
		t := *v.HireDate
		v.HireDate = &t
	}
	if v.ResignDate != nil {
		t := *v.ResignDate
		v.ResignDate = &t
	}
	v.BasicInfo = utils.CopyStringMap(v.BasicInfo)
	v.EmploymentInfo = utils.CopyStringMap(v.EmploymentInfo)
	v.EducationMilitaryInfo = utils.CopyStringMap(v.EducationMilitaryInfo)
	v.ContactInfo = utils.CopyStringMap(v.ContactInfo)
	v.InsuranceInfo = utils.CopyStringMap(v.InsuranceInfo)
	v.InternalExperiences = utils.CopyEmployeeExperiences(v.InternalExperiences)
	return v
}

func copyEmployeeImportSession(v EmployeeImportSession) EmployeeImportSession {
	v.Rows = copyEmployeeImportRows(v.Rows)
	v.Summary = utils.CopyStringMap(v.Summary)
	if v.ConfirmedAt != nil {
		t := *v.ConfirmedAt
		v.ConfirmedAt = &t
	}
	return v
}

func copyEmployeeImportRows(src []EmployeeImportRow) []EmployeeImportRow {
	if len(src) == 0 {
		return nil
	}
	dst := make([]EmployeeImportRow, len(src))
	for i, item := range src {
		item.Input = utils.CopyStringStringMap(item.Input)
		item.Employee.BasicInfo = utils.CopyStringMap(item.Employee.BasicInfo)
		item.Employee.EmploymentInfo = utils.CopyStringMap(item.Employee.EmploymentInfo)
		item.Employee.EducationMilitaryInfo = utils.CopyStringMap(item.Employee.EducationMilitaryInfo)
		item.Employee.ContactInfo = utils.CopyStringMap(item.Employee.ContactInfo)
		item.Employee.InsuranceInfo = utils.CopyStringMap(item.Employee.InsuranceInfo)
		item.Employee.InternalExperiences = utils.CopyEmployeeExperiences(item.Employee.InternalExperiences)
		item.Errors = copyRowErrors(item.Errors)
		dst[i] = item
	}
	return dst
}

func copyRowErrors(src []RowError) []RowError {
	if len(src) == 0 {
		return nil
	}
	dst := make([]RowError, len(src))
	copy(dst, src)
	return dst
}

func copyLeaveBalance(v LeaveBalance) LeaveBalance { return v }

func copyLeaveRequest(v LeaveRequest) LeaveRequest { return v }

func copyFormTemplate(v FormTemplate) FormTemplate {
	v.Schema = utils.CopyStringMap(v.Schema)
	return v
}

func copyFormInstance(v FormInstance) FormInstance {
	v.Payload = utils.CopyStringMap(v.Payload)
	return v
}

func copyKnowledgeArticle(v KnowledgeArticle) KnowledgeArticle {
	v.Tags = utils.CopyStrings(v.Tags)
	return v
}

func copyAgentRun(v AgentRun) AgentRun {
	v.References = copyRefs(v.References)
	v.ToolDecisions = copyCheckResults(v.ToolDecisions)
	return v
}

func copyCheckResults(src []CheckResult) []CheckResult {
	if len(src) == 0 {
		return nil
	}
	dst := make([]CheckResult, len(src))
	for i, item := range src {
		item.MatchedPermissions = utils.CopyStrings(item.MatchedPermissions)
		item.PermissionSetIDs = utils.CopyStrings(item.PermissionSetIDs)
		item.MatchedBy = utils.CopyStrings(item.MatchedBy)
		item.MissingPermissions = utils.CopyStrings(item.MissingPermissions)
		item.Conditions = utils.CopyStringMap(item.Conditions)
		item.FieldPolicies = utils.CopyStringStringMap(item.FieldPolicies)
		item.PermissionBoundary = utils.CopyStringMap(item.PermissionBoundary)
		if item.AssumedRole != nil {
			assumed := *item.AssumedRole
			item.AssumedRole = &assumed
		}
		dst[i] = item
	}
	return dst
}

func copyAuditLog(v AuditLog) AuditLog {
	v.Details = utils.CopyStringMap(v.Details)
	return v
}

func copyAuthzOutboxEvent(v AuthzOutboxEvent) AuthzOutboxEvent {
	v.Payload = utils.CopyStringMap(v.Payload)
	if v.ProcessedAt != nil {
		t := *v.ProcessedAt
		v.ProcessedAt = &t
	}
	return v
}

func copyOutboxEvent(v OutboxEvent) OutboxEvent {
	v.Payload = utils.CopyStringMap(v.Payload)
	if v.ProcessedAt != nil {
		t := *v.ProcessedAt
		v.ProcessedAt = &t
	}
	return v
}

func relationshipTupleKey(v AuthzRelationshipTuple) string {
	return v.ObjectType + "\x00" + v.ObjectID + "\x00" + v.Relation + "\x00" + v.SubjectType + "\x00" + v.SubjectID
}
