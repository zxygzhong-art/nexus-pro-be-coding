package memory

import "nexus-pro-be/internal/repository/internal/sliceutil"

func copyStrings(src []string) []string {
	return sliceutil.CopyStrings(src)
}

func copyStringMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyStringStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

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
	v.UserGroupIDs = copyStrings(v.UserGroupIDs)
	v.DirectPermissionSetIDs = copyStrings(v.DirectPermissionSetIDs)
	return v
}

func copyUserGroup(v UserGroup) UserGroup {
	v.MemberAccountIDs = copyStrings(v.MemberAccountIDs)
	v.PermissionSetIDs = copyStrings(v.PermissionSetIDs)
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
	v.Params = copyStringMap(v.Params)
	return v
}

func copyFieldPolicy(v FieldPolicy) FieldPolicy { return v }

func copyAssumableRole(v AssumableRole) AssumableRole {
	v.PermissionSetIDs = copyStrings(v.PermissionSetIDs)
	v.TrustPolicy = copyStringMap(v.TrustPolicy)
	v.PermissionBoundary = copyStringMap(v.PermissionBoundary)
	return v
}

func copyAssumableRoleSession(v AssumableRoleSession) AssumableRoleSession {
	v.SessionPolicy = copyStringMap(v.SessionPolicy)
	v.PermissionBoundary = copyStringMap(v.PermissionBoundary)
	if v.RevokedAt != nil {
		t := *v.RevokedAt
		v.RevokedAt = &t
	}
	return v
}

func copyOrgUnit(v OrgUnit) OrgUnit {
	v.Path = copyStrings(v.Path)
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
	v.BasicInfo = copyStringMap(v.BasicInfo)
	v.EmploymentInfo = copyStringMap(v.EmploymentInfo)
	v.EducationMilitaryInfo = copyStringMap(v.EducationMilitaryInfo)
	v.ContactInfo = copyStringMap(v.ContactInfo)
	v.InsuranceInfo = copyStringMap(v.InsuranceInfo)
	v.InternalExperiences = copyEmployeeExperiences(v.InternalExperiences)
	return v
}

func copyEmployeeExperiences(src []EmployeeExperience) []EmployeeExperience {
	if len(src) == 0 {
		return nil
	}
	dst := make([]EmployeeExperience, len(src))
	for i, item := range src {
		if item.StartDate != nil {
			t := *item.StartDate
			item.StartDate = &t
		}
		if item.EndDate != nil {
			t := *item.EndDate
			item.EndDate = &t
		}
		dst[i] = item
	}
	return dst
}

func copyEmployeeImportSession(v EmployeeImportSession) EmployeeImportSession {
	v.Rows = copyEmployeeImportRows(v.Rows)
	v.Summary = copyStringMap(v.Summary)
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
		item.Input = copyStringMapString(item.Input)
		item.Employee.BasicInfo = copyStringMap(item.Employee.BasicInfo)
		item.Employee.EmploymentInfo = copyStringMap(item.Employee.EmploymentInfo)
		item.Employee.EducationMilitaryInfo = copyStringMap(item.Employee.EducationMilitaryInfo)
		item.Employee.ContactInfo = copyStringMap(item.Employee.ContactInfo)
		item.Employee.InsuranceInfo = copyStringMap(item.Employee.InsuranceInfo)
		item.Employee.InternalExperiences = copyEmployeeExperiences(item.Employee.InternalExperiences)
		item.Errors = copyRowErrors(item.Errors)
		dst[i] = item
	}
	return dst
}

func copyStringMapString(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
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
	v.Schema = copyStringMap(v.Schema)
	return v
}

func copyFormInstance(v FormInstance) FormInstance {
	v.Payload = copyStringMap(v.Payload)
	return v
}

func copyKnowledgeArticle(v KnowledgeArticle) KnowledgeArticle {
	v.Tags = copyStrings(v.Tags)
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
		item.MatchedPermissions = copyStrings(item.MatchedPermissions)
		item.PermissionSetIDs = copyStrings(item.PermissionSetIDs)
		item.MatchedBy = copyStrings(item.MatchedBy)
		item.MissingPermissions = copyStrings(item.MissingPermissions)
		item.Conditions = copyStringMap(item.Conditions)
		item.FieldPolicies = copyStringStringMap(item.FieldPolicies)
		item.PermissionBoundary = copyStringMap(item.PermissionBoundary)
		if item.AssumedRole != nil {
			assumed := *item.AssumedRole
			item.AssumedRole = &assumed
		}
		dst[i] = item
	}
	return dst
}

func copyAuditLog(v AuditLog) AuditLog {
	v.Details = copyStringMap(v.Details)
	return v
}

func copyAuthzOutboxEvent(v AuthzOutboxEvent) AuthzOutboxEvent {
	v.Payload = copyStringMap(v.Payload)
	if v.ProcessedAt != nil {
		t := *v.ProcessedAt
		v.ProcessedAt = &t
	}
	return v
}
