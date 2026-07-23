package memory

import (
	"encoding/json"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

// copyKnowledgeDocumentChunk copies the embedding to keep memory transactions isolated.
func copyKnowledgeDocumentChunk(v KnowledgeDocumentChunk) KnowledgeDocumentChunk {
	v.Embedding = append([]float32(nil), v.Embedding...)
	return v
}

// copyFormDefinitionDraft 複製草稿及其巢狀 schema，避免 memory store 暴露可變引用。
func copyFormDefinitionDraft(v domain.FormDefinitionDraft) domain.FormDefinitionDraft {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out domain.FormDefinitionDraft
	if err := json.Unmarshal(raw, &out); err != nil {
		return v
	}
	return out
}

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

// copyUserGroup 複製使用者羣組。
func copyUserGroup(v UserGroup) UserGroup {
	v.MemberAccountIDs = utils.CopyStrings(v.MemberAccountIDs)
	v.PermissionSetIDs = utils.CopyStrings(v.PermissionSetIDs)
	return v
}

// copyGroupMembership 複製使用者羣組成員關係。
func copyGroupMembership(v GroupMembership) GroupMembership {
	if v.ValidUntil != nil {
		t := *v.ValidUntil
		v.ValidUntil = &t
	}
	return v
}

// copyPermissionSet 複製權限集合。
func copyPermissionSet(v PermissionSet) PermissionSet {
	v.Permissions = copyPermissions(v.Permissions)
	return v
}

// copyPermissionPackage 複製權限包。
func copyPermissionPackage(v PermissionPackage) PermissionPackage {
	v.Content = copyPermissionPackageContent(v.Content)
	if v.PublishedAt != nil {
		t := *v.PublishedAt
		v.PublishedAt = &t
	}
	return v
}

// copyPermissionPackageContent 複製權限包內容。
func copyPermissionPackageContent(v PermissionPackageContent) PermissionPackageContent {
	if len(v.ResourceTypes) > 0 {
		v.ResourceTypes = append([]domain.PermissionPackageResourceType(nil), v.ResourceTypes...)
		for i := range v.ResourceTypes {
			v.ResourceTypes[i].Actions = utils.CopyStrings(v.ResourceTypes[i].Actions)
		}
	}
	if len(v.Actions) > 0 {
		v.Actions = append([]domain.PermissionPackageAction(nil), v.Actions...)
	}
	v.Permissions = copyPermissions(v.Permissions)
	if len(v.Menus) > 0 {
		v.Menus = copyPermissionPackageMenus(v.Menus)
	}
	if len(v.Buttons) > 0 {
		v.Buttons = append([]domain.PermissionPackageButton(nil), v.Buttons...)
	}
	if len(v.Fields) > 0 {
		v.Fields = append([]domain.PermissionPackageField(nil), v.Fields...)
	}
	if len(v.DataScopes) > 0 {
		v.DataScopes = append([]domain.PermissionPackageDataScope(nil), v.DataScopes...)
		for i := range v.DataScopes {
			v.DataScopes[i].Params = utils.CopyStringMap(v.DataScopes[i].Params)
		}
	}
	if len(v.PermissionSetTemplates) > 0 {
		v.PermissionSetTemplates = append([]domain.PermissionSetTemplateContent(nil), v.PermissionSetTemplates...)
		for i := range v.PermissionSetTemplates {
			v.PermissionSetTemplates[i].Permissions = copyPermissions(v.PermissionSetTemplates[i].Permissions)
		}
	}
	if len(v.UserGroupTemplates) > 0 {
		v.UserGroupTemplates = append([]domain.UserGroupTemplateContent(nil), v.UserGroupTemplates...)
		for i := range v.UserGroupTemplates {
			v.UserGroupTemplates[i].PermissionSetTemplateKeys = utils.CopyStrings(v.UserGroupTemplates[i].PermissionSetTemplateKeys)
		}
	}
	if len(v.AssumableRoleTemplates) > 0 {
		v.AssumableRoleTemplates = append([]domain.AssumableRoleTemplateContent(nil), v.AssumableRoleTemplates...)
		for i := range v.AssumableRoleTemplates {
			v.AssumableRoleTemplates[i].PermissionSetTemplateKeys = utils.CopyStrings(v.AssumableRoleTemplates[i].PermissionSetTemplateKeys)
			v.AssumableRoleTemplates[i].TrustPolicy = utils.CopyStringMap(v.AssumableRoleTemplates[i].TrustPolicy)
			v.AssumableRoleTemplates[i].PermissionBoundary = utils.CopyStringMap(v.AssumableRoleTemplates[i].PermissionBoundary)
		}
	}
	if len(v.FGAMappings) > 0 {
		v.FGAMappings = append([]domain.PermissionPackageFGAMapping(nil), v.FGAMappings...)
	}
	return v
}

// copyPermissionPackageMenus 複製權限包選單。
func copyPermissionPackageMenus(src []domain.PermissionPackageMenu) []domain.PermissionPackageMenu {
	if len(src) == 0 {
		return nil
	}
	dst := make([]domain.PermissionPackageMenu, len(src))
	for i, item := range src {
		item.Children = copyPermissionPackageMenus(item.Children)
		dst[i] = item
	}
	return dst
}

// copyPermissionSetTemplate 複製權限集合模板。
func copyPermissionSetTemplate(v PermissionSetTemplate) PermissionSetTemplate {
	v.Content.Permissions = copyPermissions(v.Content.Permissions)
	return v
}

// copyUserGroupTemplate 複製使用者羣組模板。
func copyUserGroupTemplate(v UserGroupTemplate) UserGroupTemplate {
	v.Content.PermissionSetTemplateKeys = utils.CopyStrings(v.Content.PermissionSetTemplateKeys)
	return v
}

// copyAssumableRoleTemplate 複製可承擔角色模板。
func copyAssumableRoleTemplate(v AssumableRoleTemplate) AssumableRoleTemplate {
	v.Content.PermissionSetTemplateKeys = utils.CopyStrings(v.Content.PermissionSetTemplateKeys)
	v.Content.TrustPolicy = utils.CopyStringMap(v.Content.TrustPolicy)
	v.Content.PermissionBoundary = utils.CopyStringMap(v.Content.PermissionBoundary)
	return v
}

// copyPermissionPackageImport 複製權限包導入記錄。
func copyPermissionPackageImport(v PermissionPackageImport) PermissionPackageImport {
	v.ArtifactIDMap = utils.CopyStringMap(v.ArtifactIDMap)
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

// copyPosition 複製崗位。
func copyPosition(v Position) Position { return v }

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
func copyLeaveBalance(v LeaveBalance) LeaveBalance {
	v.RawPayload = utils.CopyStringMap(v.RawPayload)
	if v.LastSyncedAt != nil {
		t := *v.LastSyncedAt
		v.LastSyncedAt = &t
	}
	return v
}

// copyAttendancePolicy 複製考勤政策。
func copyAttendancePolicy(v AttendancePolicy) AttendancePolicy {
	v.WorkTime.TimeOptions = utils.CopyStrings(v.WorkTime.TimeOptions)
	v.WorkTime.WeekendOptions = utils.CopyStrings(v.WorkTime.WeekendOptions)
	v.WorkTime.CycleStartOptions = utils.CopyStrings(v.WorkTime.CycleStartOptions)
	v.WorkTime.CycleEndOptions = utils.CopyStrings(v.WorkTime.CycleEndOptions)
	if v.EffectiveFrom != nil {
		t := *v.EffectiveFrom
		v.EffectiveFrom = &t
	}
	return v
}

// copyLeaveRequest preserves immutable rule and evaluation snapshots across reads.
func copyLeaveRequest(v LeaveRequest) LeaveRequest {
	v.RuleSnapshot = utils.CopyStringMap(v.RuleSnapshot)
	v.EvaluationSnapshot = utils.CopyStringMap(v.EvaluationSnapshot)
	return v
}

func copyLeaveBalanceEntry(v LeaveBalanceEntry) LeaveBalanceEntry {
	v.Metadata = utils.CopyStringMap(v.Metadata)
	return v
}

func copyExternalLeaveRecord(v ExternalLeaveRecord) ExternalLeaveRecord {
	v.RawPayload = utils.CopyStringMap(v.RawPayload)
	if v.DeletedAt != nil {
		t := *v.DeletedAt
		v.DeletedAt = &t
	}
	return v
}

// copyAttendanceWorksite 複製考勤工作地點。
func copyAttendanceWorksite(v AttendanceWorksite) AttendanceWorksite { return v }

// copyAttendanceClockRecord 複製考勤打卡 record。
func copyAttendanceClockRecord(v AttendanceClockRecord) AttendanceClockRecord {
	v.DeviceInfo = utils.CopyStringMap(v.DeviceInfo)
	if v.VoidedAt != nil {
		t := *v.VoidedAt
		v.VoidedAt = &t
	}
	return v
}

// copyAttendanceDailySummary 複製考勤日彙總。
func copyAttendanceDailySummary(v AttendanceDailySummary) AttendanceDailySummary {
	v.Payload = utils.CopyStringMap(v.Payload)
	return v
}

func copyAttendanceDayProjection(v AttendanceDayProjection) AttendanceDayProjection {
	v.AnomalyReasons = utils.CopyStrings(v.AnomalyReasons)
	v.Payload = utils.CopyStringMap(v.Payload)
	if v.ScheduledStartAt != nil {
		t := *v.ScheduledStartAt
		v.ScheduledStartAt = &t
	}
	if v.ScheduledEndAt != nil {
		t := *v.ScheduledEndAt
		v.ScheduledEndAt = &t
	}
	if v.ClockIn != nil {
		item := copyAttendanceClockRecord(*v.ClockIn)
		v.ClockIn = &item
	}
	if v.ClockOut != nil {
		item := copyAttendanceClockRecord(*v.ClockOut)
		v.ClockOut = &item
	}
	if v.LastPunch != nil {
		item := copyAttendanceClockRecord(*v.LastPunch)
		v.LastPunch = &item
	}
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
	if v.DeletedAt != nil {
		t := *v.DeletedAt
		v.DeletedAt = &t
	}
	return v
}

// copyFormTemplateVersion 複製不可變表單版本。
func copyFormTemplateVersion(v FormTemplateVersion) FormTemplateVersion {
	v.Schema = utils.CopyStringMap(v.Schema)
	if v.PublishedAt != nil {
		t := *v.PublishedAt
		v.PublishedAt = &t
	}
	return v
}

// copyFormInstance 複製表單實例。
func copyFormInstance(v FormInstance) FormInstance {
	v.Payload = utils.CopyStringMap(v.Payload)
	return v
}

// copyFormInstanceFieldValues 複製類型化欄位投影。
func copyFormInstanceFieldValues(values []FormInstanceFieldValue) []FormInstanceFieldValue {
	out := make([]FormInstanceFieldValue, len(values))
	for index, value := range values {
		out[index] = value
		if value.ValueBoolean != nil {
			booleanValue := *value.ValueBoolean
			out[index].ValueBoolean = &booleanValue
		}
		out[index].ValueJSON = append([]byte(nil), value.ValueJSON...)
	}
	return out
}

// copyPlatformTaskRecordItem 複製平臺任務 record 項目。
func copyPlatformTaskRecordItem(v PlatformTaskRecordItem) PlatformTaskRecordItem { return v }

// copyPlatformTaskTodoRecord 複製平臺任務待辦 record。
func copyPlatformTaskTodoRecord(v PlatformTaskTodoRecord) PlatformTaskTodoRecord { return v }

// copyAgentRun 複製 agent 執行。
func copyAgentRun(v AgentRun) AgentRun {
	v.References = copyRefs(v.References)
	v.ToolDecisions = copyCheckResults(v.ToolDecisions)
	return v
}

// copyAgentModel 複製 agent 模型。
func copyAgentModel(v AgentModel) AgentModel {
	if v.LastTestedAt != nil {
		t := *v.LastTestedAt
		v.LastTestedAt = &t
	}
	if v.LastSyncedAt != nil {
		t := *v.LastSyncedAt
		v.LastSyncedAt = &t
	}
	return v
}

// copyAgentDefinition 複製 agent 定義。
func copyAgentDefinition(v AgentDefinition) AgentDefinition {
	v.SuggestedQuestions = utils.CopyStrings(v.SuggestedQuestions)
	v.SuggestedQuestionTranslations = copyLocalizedAgentSuggestedQuestions(v.SuggestedQuestionTranslations)
	v.Tools = utils.CopyStrings(v.Tools)
	v.ExternalToolIDs = utils.CopyStrings(v.ExternalToolIDs)
	v.KnowledgeBaseIDs = utils.CopyStrings(v.KnowledgeBaseIDs)
	v.SubAgents = copyAgentTeamMembers(v.SubAgents)
	v.VisibilityTargets = utils.CopyStrings(v.VisibilityTargets)
	v.Versions = copyAgentDefinitionVersions(v.Versions)
	v.Usage.TopPrompts = utils.CopyStrings(v.Usage.TopPrompts)
	if v.Usage.LastRunAt != nil {
		t := *v.Usage.LastRunAt
		v.Usage.LastRunAt = &t
	}
	return v
}

// copyAgentDefinitionVersion 複製 agent 版本。
func copyAgentDefinitionVersion(v AgentDefinitionVersion) AgentDefinitionVersion {
	v.SuggestedQuestions = utils.CopyStrings(v.SuggestedQuestions)
	v.SuggestedQuestionTranslations = copyLocalizedAgentSuggestedQuestions(v.SuggestedQuestionTranslations)
	v.Tools = utils.CopyStrings(v.Tools)
	v.ExternalToolIDs = utils.CopyStrings(v.ExternalToolIDs)
	v.KnowledgeBaseIDs = utils.CopyStrings(v.KnowledgeBaseIDs)
	v.VisibilityTargets = utils.CopyStrings(v.VisibilityTargets)
	v.SubAgents = copyAgentTeamMembers(v.SubAgents)
	return v
}

// copyLocalizedAgentSuggestedQuestions keeps translation maps isolated between in-memory snapshots.
func copyLocalizedAgentSuggestedQuestions(
	src []LocalizedAgentSuggestedQuestion,
) []LocalizedAgentSuggestedQuestion {
	if src == nil {
		return nil
	}
	result := make([]LocalizedAgentSuggestedQuestion, len(src))
	for index, item := range src {
		translations := make(map[string]string, len(item.Translations))
		for locale, value := range item.Translations {
			translations[locale] = value
		}
		result[index] = LocalizedAgentSuggestedQuestion{Translations: translations}
	}
	return result
}

// copyAgentTeamMembers 深複製 Team 成員及其工具集合。
func copyAgentTeamMembers(src []AgentTeamMember) []AgentTeamMember {
	if len(src) == 0 {
		return nil
	}
	out := make([]AgentTeamMember, len(src))
	copy(out, src)
	for i := range out {
		out[i].Tools = utils.CopyStrings(out[i].Tools)
		out[i].ExternalToolIDs = utils.CopyStrings(out[i].ExternalToolIDs)
		out[i].KnowledgeBaseIDs = utils.CopyStrings(out[i].KnowledgeBaseIDs)
	}
	return out
}

func copyAgentDefinitionVersions(src []AgentDefinitionVersion) []AgentDefinitionVersion {
	if len(src) == 0 {
		return nil
	}
	out := make([]AgentDefinitionVersion, len(src))
	for i, item := range src {
		out[i] = copyAgentDefinitionVersion(item)
	}
	return out
}

// copyAgentSession 複製 agent session。
func copyAgentSession(v AgentSession) AgentSession {
	if v.LastMessageAt != nil {
		t := *v.LastMessageAt
		v.LastMessageAt = &t
	}
	return v
}

// copyAgentSessionMessage 複製 agent session message。
func copyAgentSessionMessage(v AgentSessionMessage) AgentSessionMessage {
	v.Metadata = utils.CopyStringMap(v.Metadata)
	if v.Attachments != nil {
		v.Attachments = append([]domain.AgentSessionFile(nil), v.Attachments...)
	}
	return v
}

// copyAgentSessionFile copies nullable retention and attachment metadata.
func copyAgentSessionFile(v domain.AgentSessionFile) domain.AgentSessionFile {
	if v.ExpiresAt != nil {
		expiresAt := *v.ExpiresAt
		v.ExpiresAt = &expiresAt
	}
	if v.Ordinal != nil {
		ordinal := *v.Ordinal
		v.Ordinal = &ordinal
	}
	return v
}

func copyFormInstanceFile(v domain.FormInstanceFile) domain.FormInstanceFile {
	if v.ExpiresAt != nil {
		expiresAt := *v.ExpiresAt
		v.ExpiresAt = &expiresAt
	}
	return v
}

// copyAgentMemory 複製 agent memory。
func copyAgentMemory(v AgentMemory) AgentMemory {
	if v.ExpiresAt != nil {
		t := *v.ExpiresAt
		v.ExpiresAt = &t
	}
	return v
}

// copyAgentExternalTool isolates mutable capability schemas and timestamps.
func copyAgentExternalTool(v AgentExternalTool) AgentExternalTool {
	if v.LastTestedAt != nil {
		t := *v.LastTestedAt
		v.LastTestedAt = &t
	}
	if v.ArchivedAt != nil {
		t := *v.ArchivedAt
		v.ArchivedAt = &t
	}
	v.Capabilities = append([]domain.ExternalToolCapability(nil), v.Capabilities...)
	for i := range v.Capabilities {
		v.Capabilities[i].InputSchema = utils.CopyStringMap(v.Capabilities[i].InputSchema)
		v.Capabilities[i].OutputSchema = utils.CopyStringMap(v.Capabilities[i].OutputSchema)
		if v.Capabilities[i].ArchivedAt != nil {
			t := *v.Capabilities[i].ArchivedAt
			v.Capabilities[i].ArchivedAt = &t
		}
	}
	return v
}

func copyExternalToolCapability(v domain.ExternalToolCapability) domain.ExternalToolCapability {
	v.InputSchema = utils.CopyStringMap(v.InputSchema)
	v.OutputSchema = utils.CopyStringMap(v.OutputSchema)
	if v.ArchivedAt != nil {
		archivedAt := *v.ArchivedAt
		v.ArchivedAt = &archivedAt
	}
	return v
}

func copyExecutionStep(v domain.ExecutionStep) domain.ExecutionStep {
	v.InputSummary = utils.CopyStringMap(v.InputSummary)
	v.OutputSummary = utils.CopyStringMap(v.OutputSummary)
	if v.StartedAt != nil {
		startedAt := *v.StartedAt
		v.StartedAt = &startedAt
	}
	if v.CompletedAt != nil {
		completedAt := *v.CompletedAt
		v.CompletedAt = &completedAt
	}
	return v
}

func copyAgentRevisionExternalTools(src []domain.AgentRevisionExternalTool) []domain.AgentRevisionExternalTool {
	out := append([]domain.AgentRevisionExternalTool(nil), src...)
	for index := range out {
		out[index].Config = utils.CopyStringMap(out[index].Config)
	}
	return out
}

func copyAgentConfirmation(v domain.AgentConfirmationRecord) domain.AgentConfirmationRecord {
	v.PublicPayload = utils.CopyStringMap(v.PublicPayload)
	v.ActionPayload = utils.CopyStringMap(v.ActionPayload)
	v.ResultPayload = utils.CopyStringMap(v.ResultPayload)
	if v.ConsumedAt != nil {
		consumedAt := *v.ConsumedAt
		v.ConsumedAt = &consumedAt
	}
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

// copyAuditLog 複製稽覈 log。
func copyAuditLog(v AuditLog) AuditLog {
	v.Details = utils.CopyStringMap(v.Details)
	return v
}

// copyOutboxEvent 複製 outbox 事件。
func copyOutboxEvent(v OutboxEvent) OutboxEvent {
	v.Payload = utils.CopyStringMap(v.Payload)
	if v.MaxAttempts != nil {
		n := *v.MaxAttempts
		v.MaxAttempts = &n
	}
	if v.ClaimExpiresAt != nil {
		t := *v.ClaimExpiresAt
		v.ClaimExpiresAt = &t
	}
	if v.LastAttemptAt != nil {
		t := *v.LastAttemptAt
		v.LastAttemptAt = &t
	}
	if v.ProcessedAt != nil {
		t := *v.ProcessedAt
		v.ProcessedAt = &t
	}
	if v.DeadLetteredAt != nil {
		t := *v.DeadLetteredAt
		v.DeadLetteredAt = &t
	}
	return v
}

// relationshipTupleKey 處理關係 tuple key。
func relationshipTupleKey(v AuthzRelationshipTuple) string {
	return v.ObjectType + "\x00" + v.ObjectID + "\x00" + v.Relation + "\x00" + v.SubjectType + "\x00" + v.SubjectID
}
