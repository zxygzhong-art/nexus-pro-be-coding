package service

import (
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

const (
	leaveEvaluationEligible            = "eligible"
	leaveEvaluationEligibleNoBalance   = "eligible_without_balance"
	LeaveEvaluationUnsupported         = "unsupported_leave_type"
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
	return c.EvaluateLeaveRequestRules(ctx, employeeID, input.LeaveType, startAt, endAt, input.Hours)
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

// EvaluateLeaveRequestRules freezes the effective rule and reports balance readiness without mutating state.
func (c AttendanceService) EvaluateLeaveRequestRules(ctx RequestContext, employeeID, leaveTypeRaw string, startAt, endAt time.Time, requestedHours float64) (LeaveRequestEvaluation, error) {
	policy, err := c.loadAttendancePolicyResponse(ctx)
	if err != nil {
		return LeaveRequestEvaluation{}, err
	}
	hours := requestedHours
	if hours <= 0 {
		hours = CalculateLeaveHoursWithinPolicy(startAt, endAt, policy.WorkTime)
	}
	requestedMinutes := leaveMinutes(hours)
	if requestedMinutes <= 0 {
		return LeaveRequestEvaluation{}, BadRequest("selected time does not include working hours")
	}
	leaveTypes, err := c.loadLeaveTypes(ctx)
	if err != nil {
		return LeaveRequestEvaluation{}, err
	}
	leaveType, supported := findLeaveType(leaveTypes, leaveTypeRaw, true)
	leaveTypeCode := normalizeLeaveTypeCode(leaveTypeRaw)
	rule := leaveTypeRule(leaveType)
	rule.PolicyVersion = policy.Version
	if !supported {
		rule = domain.LeaveRuleSnapshot{LeaveTypeID: domain.StableLeaveTypeID(leaveTypeCode), Code: leaveTypeCode, PolicyVersion: policy.Version}
	} else {
		leaveTypeCode = leaveType.Code
	}
	evaluation := LeaveRequestEvaluation{
		Eligible: false, Status: LeaveEvaluationUnsupported,
		Message:    "The requested leave type is not enabled in the system leave catalog.",
		EmployeeID: employeeID, LeaveTypeID: domain.StableLeaveTypeID(leaveTypeCode),
		LeaveType: leaveTypeCode, RequestedMinutes: requestedMinutes, PolicyVersion: policy.Version, Rule: rule,
	}
	if !supported {
		return evaluation, nil
	}
	evaluation.LeaveTypeName = rule.Name
	evaluation.LeaveTypeID = evaluation.Rule.LeaveTypeID
	evaluation.BalanceRequired = rule.RequiresBalance
	evaluation.ProofRequired = rule.ProofAfterHours != nil && requestedMinutes >= leaveMinutes(*rule.ProofAfterHours)
	evaluation.Rule = rule
	if !rule.RequiresBalance {
		evaluation.Eligible = true
		evaluation.Status = leaveEvaluationEligible
		evaluation.Message = "The requested leave type does not require a balance."
		return evaluation, nil
	}
	balances, err := c.listEffectiveLeaveBalances(ctx)
	if err != nil {
		return LeaveRequestEvaluation{}, err
	}
	for _, balance := range balances {
		if balance.EmployeeID != employeeID || balance.LeaveTypeID != evaluation.LeaveTypeID || !leaveBalanceCoversDate(balance, startAt) {
			continue
		}
		evaluation.BalanceInitialized = true
		if balance.RemainingMinutes > 0 {
			evaluation.AvailableMinutes += balance.RemainingMinutes
		}
	}
	if !evaluation.BalanceInitialized {
		return applyLeaveBalanceFallback(evaluation, leaveEvaluationBalanceMissing), nil
	}
	if evaluation.AvailableMinutes < requestedMinutes {
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

// leaveRuleSnapshotMap converts the typed rule into a JSON-safe persistence snapshot.
func leaveRuleSnapshotMap(rule domain.LeaveRuleSnapshot) map[string]any {
	out := map[string]any{
		"leave_type_id": rule.LeaveTypeID, "code": rule.Code, "name": rule.Name,
		"grant_mode": rule.GrantMode, "requires_balance": rule.RequiresBalance,
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
		"requested_minutes": evaluation.RequestedMinutes, "balance_required": evaluation.BalanceRequired,
		"balance_initialized": evaluation.BalanceInitialized, "available_minutes": evaluation.AvailableMinutes,
		"balance_fallback_reason": evaluation.BalanceFallbackReason, "proof_required": evaluation.ProofRequired,
	}
}

// leaveEvaluationError preserves the existing create API error contract for failed dry-run decisions.
func leaveEvaluationError(evaluation LeaveRequestEvaluation) error {
	switch evaluation.Status {
	case LeaveEvaluationUnsupported:
		return BadRequest("unknown leave type")
	case leaveEvaluationBalanceMissing:
		return BadRequest("leave balance is required for this leave type")
	case leaveEvaluationBalanceInsufficient:
		return BadRequest("leave balance is insufficient")
	default:
		return BadRequest("leave request is not eligible")
	}
}

// resolveExternalLeaveTypeCode resolves a stable upstream code first, then falls
// back to the tenant catalog identity for feeds that still send canonical codes.
func (c AttendanceService) resolveExternalLeaveTypeCode(ctx RequestContext, source, externalCode, externalCategoryCode string, asOf time.Time) (string, string, bool, error) {
	if strings.TrimSpace(externalCode) == "" {
		return "", "", false, nil
	}
	if ref, ok, err := c.store.GetLeaveTypeExternalRef(goContext(ctx), ctx.TenantID, source, externalCode, externalCategoryCode, asOf); err != nil {
		return "", "", false, err
	} else if ok {
		leaveTypes, loadErr := c.loadLeaveTypes(ctx)
		if loadErr != nil {
			return "", "", false, loadErr
		}
		for _, item := range leaveTypes {
			if item.ID == ref.LeaveTypeID && item.Enabled {
				return item.Code, item.ID, true, nil
			}
		}
	}
	leaveTypes, err := c.loadLeaveTypes(ctx)
	if err != nil {
		return "", "", false, err
	}
	if leaveType, ok := findLeaveType(leaveTypes, externalCode, true); ok {
		return leaveType.Code, leaveType.ID, true, nil
	}
	return "", "", false, nil
}

func (c AttendanceService) resolveEHRMSLeaveType(ctx RequestContext, externalCode, externalCategoryCode, displayName string, asOf time.Time) (string, string, bool, error) {
	if strings.TrimSpace(externalCode) != "" {
		if code, id, found, err := c.resolveExternalLeaveTypeCode(ctx, ehrmsAttendanceSource, externalCode, externalCategoryCode, asOf); err != nil || found {
			return code, id, found, err
		}
	}
	items, err := c.loadLeaveTypes(ctx)
	if err != nil {
		return "", "", false, err
	}
	var matched LeaveType
	for _, candidate := range []string{displayName, externalCode} {
		wanted := normalizeLeaveTypeCode(candidate)
		for _, item := range items {
			if !item.Enabled {
				continue
			}
			if normalizeLeaveTypeCode(item.Code) == wanted || normalizeLeaveTypeCode(item.NameZH) == wanted || normalizeLeaveTypeCode(item.NameEN) == wanted {
				matched = item
				break
			}
		}
		if matched.ID != "" {
			break
		}
	}
	if matched.ID == "" {
		return "", "", false, nil
	}
	if strings.TrimSpace(externalCode) != "" {
		now := c.Now()
		if err := c.store.UpsertLeaveTypeExternalRef(goContext(ctx), domain.LeaveTypeExternalRef{
			ID: utils.NewID("lter"), TenantID: ctx.TenantID, SourceSystem: ehrmsAttendanceSource,
			ExternalCode: strings.TrimSpace(externalCode), ExternalCategoryCode: strings.TrimSpace(externalCategoryCode),
			LeaveTypeID: matched.ID, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return "", "", false, err
		}
	}
	return matched.Code, matched.ID, true, nil
}
