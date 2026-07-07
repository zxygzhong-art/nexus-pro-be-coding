package memory

import "nexus-pro-be/internal/utils"

// copyPermissions 複製權限。
func copyPermissions(src []Permission) []Permission {
	if len(src) == 0 {
		return nil
	}
	dst := make([]Permission, len(src))
	copy(dst, src)
	return dst
}

// copyRefs 複製 refs。
func copyRefs(src []Reference) []Reference {
	if len(src) == 0 {
		return nil
	}
	dst := make([]Reference, len(src))
	copy(dst, src)
	return dst
}

// copyTenant 複製租戶。
func copyTenant(v Tenant) Tenant { return v }

// copyAccount 複製帳號。
func copyAccount(v Account) Account {
	v.UserGroupIDs = utils.CopyStrings(v.UserGroupIDs)
	v.DirectPermissionSetIDs = utils.CopyStrings(v.DirectPermissionSetIDs)
	return v
}

// copyUserIdentity 複製使用者身分。
func copyUserIdentity(v UserIdentity) UserIdentity { return v }

// copyUserGroup 複製使用者群組。
func copyUserGroup(v UserGroup) UserGroup {
	v.MemberAccountIDs = utils.CopyStrings(v.MemberAccountIDs)
	v.PermissionSetIDs = utils.CopyStrings(v.PermissionSetIDs)
	return v
}

// copyPermissionSet 複製權限集合。
func copyPermissionSet(v PermissionSet) PermissionSet {
	v.Permissions = copyPermissions(v.Permissions)
	return v
}

// copyPermissionCatalogItem 複製權限 catalog 項。
func copyPermissionCatalogItem(v PermissionCatalogItem) PermissionCatalogItem { return v }

// copyMenuItem 複製選單項。
func copyMenuItem(v MenuItem) MenuItem { return v }

// copyPermissionSetItem 複製權限集合項。
func copyPermissionSetItem(v PermissionSetItem) PermissionSetItem { return v }

// copyPermissionSetAssignment 複製權限集合指派。
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

// copyDataScope 複製資料範圍。
func copyDataScope(v DataScope) DataScope {
	v.Params = utils.CopyStringMap(v.Params)
	return v
}

// copyFieldPolicy 複製欄位政策。
func copyFieldPolicy(v FieldPolicy) FieldPolicy { return v }

// copyAssumableRole 複製 assumable 角色。
func copyAssumableRole(v AssumableRole) AssumableRole {
	v.PermissionSetIDs = utils.CopyStrings(v.PermissionSetIDs)
	v.TrustPolicy = utils.CopyStringMap(v.TrustPolicy)
	v.PermissionBoundary = utils.CopyStringMap(v.PermissionBoundary)
	return v
}

// copyAssumableRoleSession 複製 assumable 角色 session。
func copyAssumableRoleSession(v AssumableRoleSession) AssumableRoleSession {
	v.SessionPolicy = utils.CopyStringMap(v.SessionPolicy)
	v.PermissionBoundary = utils.CopyStringMap(v.PermissionBoundary)
	if v.RevokedAt != nil {
		t := *v.RevokedAt
		v.RevokedAt = &t
	}
	return v
}

// copyOrgUnit 複製組織單位。
func copyOrgUnit(v OrgUnit) OrgUnit {
	v.Path = utils.CopyStrings(v.Path)
	return v
}

// copyEmployee 複製員工。
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

// copyEmployeeImportSession 複製員工 import session。
func copyEmployeeImportSession(v EmployeeImportSession) EmployeeImportSession {
	v.Rows = copyEmployeeImportRows(v.Rows)
	v.Summary = utils.CopyStringMap(v.Summary)
	if v.ConfirmedAt != nil {
		t := *v.ConfirmedAt
		v.ConfirmedAt = &t
	}
	return v
}

// copyEmployeeImportRows 複製員工 import 列。
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

// copyRowErrors 複製列錯誤。
func copyRowErrors(src []RowError) []RowError {
	if len(src) == 0 {
		return nil
	}
	dst := make([]RowError, len(src))
	copy(dst, src)
	return dst
}

// copyLeaveBalance 複製請假 balance。
func copyLeaveBalance(v LeaveBalance) LeaveBalance { return v }

// copyAttendancePolicy 複製考勤政策。
func copyAttendancePolicy(v AttendancePolicy) AttendancePolicy {
	v.WorkTime.TimeOptions = utils.CopyStrings(v.WorkTime.TimeOptions)
	v.WorkTime.WeekendOptions = utils.CopyStrings(v.WorkTime.WeekendOptions)
	v.WorkTime.CycleStartOptions = utils.CopyStrings(v.WorkTime.CycleStartOptions)
	v.WorkTime.CycleEndOptions = utils.CopyStrings(v.WorkTime.CycleEndOptions)
	if len(v.LeaveTypes) > 0 {
		next := make([]AttendanceLeaveType, len(v.LeaveTypes))
		copy(next, v.LeaveTypes)
		v.LeaveTypes = next
	}
	return v
}

// copyLeaveRequest 複製請假請求。
func copyLeaveRequest(v LeaveRequest) LeaveRequest { return v }

// copyAttendanceWorksite 複製考勤工作地點。
func copyAttendanceWorksite(v AttendanceWorksite) AttendanceWorksite { return v }

// copyAttendanceShift 複製考勤班別。
func copyAttendanceShift(v AttendanceShift) AttendanceShift { return v }

// copyAttendanceShiftAssignment 複製考勤班別指派。
func copyAttendanceShiftAssignment(v AttendanceShiftAssignment) AttendanceShiftAssignment {
	if v.EffectiveTo != nil {
		t := *v.EffectiveTo
		v.EffectiveTo = &t
	}
	return v
}

// copyAttendanceClockRecord 複製考勤打卡 record。
func copyAttendanceClockRecord(v AttendanceClockRecord) AttendanceClockRecord {
	v.DeviceInfo = utils.CopyStringMap(v.DeviceInfo)
	return v
}

// copyAttendanceCorrectionRequest 複製考勤 correction 請求。
func copyAttendanceCorrectionRequest(v AttendanceCorrectionRequest) AttendanceCorrectionRequest {
	if v.ReviewedAt != nil {
		t := *v.ReviewedAt
		v.ReviewedAt = &t
	}
	return v
}

// copyOvertimeRequest 複製加班申請。
func copyOvertimeRequest(v OvertimeRequest) OvertimeRequest { return v }

// copyFormTemplate 複製表單範本。
func copyFormTemplate(v FormTemplate) FormTemplate {
	v.Schema = utils.CopyStringMap(v.Schema)
	return v
}

// copyFormInstance 複製表單實例。
func copyFormInstance(v FormInstance) FormInstance {
	v.Payload = utils.CopyStringMap(v.Payload)
	return v
}

// copyPlatformTaskRecordItem 複製平台任務 record 項目。
func copyPlatformTaskRecordItem(v PlatformTaskRecordItem) PlatformTaskRecordItem { return v }

// copyPlatformTaskTodoRecord 複製平台任務待辦 record。
func copyPlatformTaskTodoRecord(v PlatformTaskTodoRecord) PlatformTaskTodoRecord { return v }

// copyAgentRun 複製 agent 執行。
func copyAgentRun(v AgentRun) AgentRun {
	v.References = copyRefs(v.References)
	v.ToolDecisions = copyCheckResults(v.ToolDecisions)
	return v
}

// copyNotification 複製系統通知。
func copyNotification(v Notification) Notification {
	if v.ExpiresAt != nil {
		t := *v.ExpiresAt
		v.ExpiresAt = &t
	}
	return v
}

// copyNotificationRecipient 複製系統通知投遞狀態。
func copyNotificationRecipient(v NotificationRecipient) NotificationRecipient {
	if v.ReadAt != nil {
		t := *v.ReadAt
		v.ReadAt = &t
	}
	if v.DeletedAt != nil {
		t := *v.DeletedAt
		v.DeletedAt = &t
	}
	return v
}

// copyCheckResults 複製 check 結果。
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

// copyAuditLog 複製稽核 log。
func copyAuditLog(v AuditLog) AuditLog {
	v.Details = utils.CopyStringMap(v.Details)
	return v
}

// copyOutboxEvent 複製 outbox 事件。
func copyOutboxEvent(v OutboxEvent) OutboxEvent {
	v.Payload = utils.CopyStringMap(v.Payload)
	if v.ProcessedAt != nil {
		t := *v.ProcessedAt
		v.ProcessedAt = &t
	}
	return v
}

// relationshipTupleKey 處理關係 tuple key。
func relationshipTupleKey(v AuthzRelationshipTuple) string {
	return v.ObjectType + "\x00" + v.ObjectID + "\x00" + v.Relation + "\x00" + v.SubjectType + "\x00" + v.SubjectID
}
