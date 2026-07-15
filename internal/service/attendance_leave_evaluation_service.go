package service

import (
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

const (
	leaveEvaluationEligible            = "eligible"
	leaveEvaluationEligibleNoBalance   = "eligible_without_balance"
	leaveEvaluationUnsupported         = "unsupported_leave_type"
	leaveEvaluationBalanceMissing      = "balance_not_initialized"
	leaveEvaluationBalanceInsufficient = "insufficient_balance"
)

// EvaluateLeaveRequest runs the same policy decision used by API, workflow, and agent submissions.
func (c AttendanceService) EvaluateLeaveRequest(ctx RequestContext, input EvaluateLeaveRequestInput) (LeaveRequestEvaluation, error) {
	_, employeeID, err := c.authorizeLeaveRequestEmployee(ctx, input.EmployeeID)
	if err != nil {
		return LeaveRequestEvaluation{}, err
	}
	startAt, err := utils.ParseDateTime(input.StartAt)
	if err != nil {
		return LeaveRequestEvaluation{}, BadRequest("start_at must be RFC3339 or YYYY-MM-DD")
	}
	endAt, err := utils.ParseDateTime(input.EndAt)
	if err != nil {
		return LeaveRequestEvaluation{}, BadRequest("end_at must be RFC3339 or YYYY-MM-DD")
	}
	if !endAt.After(startAt) {
		return LeaveRequestEvaluation{}, BadRequest("end_at must be after start_at")
	}
	return c.evaluateLeaveRequestRules(ctx, employeeID, input.LeaveType, startAt, endAt, input.Hours)
}

// authorizeLeaveRequestEmployee preserves employee data-scope checks for dry-run and create paths.
func (c AttendanceService) authorizeLeaveRequestEmployee(ctx RequestContext, requestedEmployeeID string) (Account, string, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return Account{}, "", err
	}
	employeeID := strings.TrimSpace(requestedEmployeeID)
	if employeeID == "" {
		employeeID = account.EmployeeID
	}
	if employeeID == "" {
		return Account{}, "", BadRequest("employee_id is required")
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{
		ApplicationCode: AppAttendance, ResourceType: ResourceLeave, ResourceID: employeeID,
		Target: employeeID, TargetEmployeeID: employeeID, Action: ActionCreate,
	})
	if err != nil {
		return Account{}, "", err
	}
	if !decision.Allowed {
		return Account{}, "", Forbidden(decision.Reason)
	}
	allowed, all, err := c.attendanceEmployeeScope(ctx, account, decision)
	if err != nil {
		return Account{}, "", err
	}
	if !all {
		if _, ok := allowed[employeeID]; !ok {
			return Account{}, "", Forbidden("employee is outside data scope")
		}
	}
	if _, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, employeeID); err != nil {
		return Account{}, "", err
	} else if !ok {
		return Account{}, "", NotFound("employee", employeeID)
	}
	return account, employeeID, nil
}

// evaluateLeaveRequestRules freezes the effective rule and reports balance readiness without mutating state.
func (c AttendanceService) evaluateLeaveRequestRules(ctx RequestContext, employeeID, leaveTypeRaw string, startAt, endAt time.Time, requestedHours float64) (LeaveRequestEvaluation, error) {
	policy, err := c.loadAttendancePolicyResponse(ctx)
	if err != nil {
		return LeaveRequestEvaluation{}, err
	}
	hours := requestedHours
	if hours <= 0 {
		hours = CalculateLeaveHoursWithinPolicy(startAt, endAt, policy.WorkTime)
	}
	if hours <= 0 {
		return LeaveRequestEvaluation{}, BadRequest("selected time does not include working hours")
	}
	leaveTypeCode := normalizeLeaveTypeCode(leaveTypeRaw)
	leaveType, supported := findLeaveTypeInPolicy(policy, leaveTypeCode)
	rule := leaveRuleSnapshot(policy.Version, leaveType)
	evaluation := LeaveRequestEvaluation{
		Eligible: false, Status: leaveEvaluationUnsupported,
		Message:    "The requested leave type is not active in the current attendance policy.",
		EmployeeID: employeeID, LeaveTypeID: domain.StableLeaveTypeID(leaveTypeCode),
		LeaveType: leaveTypeCode, Hours: hours, PolicyVersion: policy.Version, Rule: rule,
	}
	if !supported || !leaveType.Active {
		return evaluation, nil
	}
	evaluation.LeaveTypeName = leaveType.Name
	evaluation.LeaveTypeID = evaluation.Rule.LeaveTypeID
	evaluation.BalanceRequired = leaveType.RequiresBalance
	evaluation.ProofRequired = leaveType.ProofAfterHours != nil && hours >= *leaveType.ProofAfterHours
	evaluation.Rule = leaveRuleSnapshot(policy.Version, leaveType)
	if !leaveType.RequiresBalance {
		evaluation.Eligible = true
		evaluation.Status = leaveEvaluationEligible
		evaluation.Message = "The requested leave type does not require a balance."
		return evaluation, nil
	}
	balances, err := c.store.ListLeaveBalances(goContext(ctx), ctx.TenantID)
	if err != nil {
		return LeaveRequestEvaluation{}, err
	}
	for _, balance := range balances {
		if balance.EmployeeID != employeeID || !strings.EqualFold(strings.TrimSpace(balance.LeaveType), leaveTypeCode) || !leaveBalanceCoversDate(balance, startAt) {
			continue
		}
		evaluation.BalanceInitialized = true
		if balance.RemainingHours > evaluation.AvailableHours {
			evaluation.AvailableHours = balance.RemainingHours
		}
	}
	if !evaluation.BalanceInitialized {
		return applyLeaveBalanceFallback(evaluation, leaveEvaluationBalanceMissing), nil
	}
	if evaluation.AvailableHours < hours {
		return applyLeaveBalanceFallback(evaluation, leaveEvaluationBalanceInsufficient), nil
	}
	evaluation.Eligible = true
	evaluation.Status = leaveEvaluationEligible
	evaluation.Message = "The leave request is eligible under the published policy."
	return evaluation, nil
}

// applyLeaveBalanceFallback keeps leave submission available when no reservable balance exists.
func applyLeaveBalanceFallback(evaluation LeaveRequestEvaluation, reason string) LeaveRequestEvaluation {
	evaluation.Eligible = true
	evaluation.Status = leaveEvaluationEligibleNoBalance
	evaluation.BalanceRequired = false
	evaluation.BalanceFallbackReason = reason
	switch reason {
	case leaveEvaluationBalanceMissing:
		evaluation.Message = "The requested leave type has no initialized balance, so the request will continue without reserving balance."
	case leaveEvaluationBalanceInsufficient:
		evaluation.Message = "The available leave balance is insufficient, so the request will continue without reserving balance."
	default:
		evaluation.Message = "The request will continue without reserving leave balance."
	}
	return evaluation
}

// leaveBalanceCoversDate checks the entitlement period before reporting availability.
func leaveBalanceCoversDate(balance LeaveBalance, at time.Time) bool {
	date := at.Format(time.DateOnly)
	return (balance.PeriodStart == "" || balance.PeriodStart <= date) && (balance.PeriodEnd == "" || balance.PeriodEnd >= date)
}

// leaveRuleSnapshot captures the immutable policy fields needed after later policy changes.
func leaveRuleSnapshot(policyVersion int, leaveType AttendanceLeaveType) domain.LeaveRuleSnapshot {
	leaveTypeID := strings.TrimSpace(leaveType.ID)
	if leaveTypeID == "" {
		leaveTypeID = domain.StableLeaveTypeID(leaveType.Code)
	}
	return domain.LeaveRuleSnapshot{
		LeaveTypeID: leaveTypeID, Code: leaveType.Code, Name: leaveType.Name,
		Unit: leaveType.Unit, GrantMode: leaveType.GrantMode, RequiresBalance: leaveType.RequiresBalance,
		PaidRatio: leaveType.PaidRatio, ProofAfterHours: cloneFloatPtr(leaveType.ProofAfterHours), PolicyVersion: policyVersion,
	}
}

// leaveRuleSnapshotMap converts the typed rule into a JSON-safe persistence snapshot.
func leaveRuleSnapshotMap(rule domain.LeaveRuleSnapshot) map[string]any {
	out := map[string]any{
		"leave_type_id": rule.LeaveTypeID, "code": rule.Code, "name": rule.Name, "unit": rule.Unit,
		"grant_mode": rule.GrantMode, "requires_balance": rule.RequiresBalance, "paid_ratio": rule.PaidRatio,
		"policy_version": rule.PolicyVersion,
	}
	if rule.ProofAfterHours != nil {
		out["proof_after_hours"] = *rule.ProofAfterHours
	}
	return out
}

// leaveEvaluationSnapshotMap records the decision facts shown to the requester at submission time.
func leaveEvaluationSnapshotMap(evaluation LeaveRequestEvaluation) map[string]any {
	return map[string]any{
		"eligible": evaluation.Eligible, "status": evaluation.Status, "message": evaluation.Message,
		"hours": evaluation.Hours, "balance_required": evaluation.BalanceRequired,
		"balance_initialized": evaluation.BalanceInitialized, "available_hours": evaluation.AvailableHours,
		"balance_fallback_reason": evaluation.BalanceFallbackReason, "proof_required": evaluation.ProofRequired,
	}
}

// leaveEvaluationError preserves the existing create API error contract for failed dry-run decisions.
func leaveEvaluationError(evaluation LeaveRequestEvaluation) error {
	switch evaluation.Status {
	case leaveEvaluationUnsupported:
		return BadRequest("unknown leave type")
	case leaveEvaluationBalanceMissing:
		return BadRequest("leave balance is required for this leave type")
	case leaveEvaluationBalanceInsufficient:
		return BadRequest("leave balance is insufficient")
	default:
		return BadRequest("leave request is not eligible")
	}
}

// resolveExternalLeaveTypeCode accepts explicit mappings and known policy aliases, and records unknown upstream codes.
func (c AttendanceService) resolveExternalLeaveTypeCode(ctx RequestContext, source, externalCode string, asOf time.Time) (string, string, bool, error) {
	mapping, found, err := c.store.GetLeaveTypeExternalMapping(goContext(ctx), ctx.TenantID, source, externalCode, asOf)
	if err != nil {
		return "", "", false, err
	}
	if found {
		if code := strings.TrimSpace(mapping.LeaveTypeCode); code != "" {
			return normalizeLeaveTypeCode(code), mapping.LeaveTypeID, true, nil
		}
		if code := strings.TrimPrefix(strings.TrimSpace(mapping.LeaveTypeID), "lt_"); code != "" {
			return normalizeLeaveTypeCode(code), mapping.LeaveTypeID, true, nil
		}
	}
	if strings.TrimSpace(externalCode) == "" {
		return "", "", false, nil
	}
	code := normalizeLeaveTypeCode(externalCode)
	policy, err := c.loadAttendancePolicyResponse(ctx)
	if err != nil {
		return "", "", false, err
	}
	for _, leaveType := range policy.LeaveTypes {
		if strings.EqualFold(leaveType.Code, code) {
			return leaveType.Code, leaveType.ID, true, nil
		}
	}
	now := c.Now()
	issue := domain.LeaveTypeSyncIssue{
		ID:       ehrmsStableID("ltsi", ctx.TenantID, source, strings.ToLower(strings.TrimSpace(externalCode)), leaveSyncIssueUnmapped),
		TenantID: ctx.TenantID, Source: strings.ToLower(strings.TrimSpace(source)), ExternalCode: strings.TrimSpace(externalCode),
		IssueCode: leaveSyncIssueUnmapped, Message: "upstream leave code requires HR mapping",
		Occurrences: 1, Status: "open", FirstSeenAt: now, LastSeenAt: now,
	}
	if err := c.store.UpsertLeaveTypeSyncIssue(goContext(ctx), issue); err != nil {
		return "", "", false, err
	}
	return "", "", false, nil
}
