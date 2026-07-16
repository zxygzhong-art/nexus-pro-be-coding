package service

import (
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

// GrantLeaveBalances 依假勤制度發放年度假別餘額。
func (c AttendanceService) GrantLeaveBalances(ctx RequestContext, input GrantLeaveBalancesInput) (GrantLeaveBalancesResult, error) {
	_, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppAttendance, ResourceType: ResourceLeave, Action: ActionUpdate},
		AuditTarget{Event: "attendance.leave_balance.grant", Resource: string(ResourceLeave)},
	)
	if err != nil {
		return GrantLeaveBalancesResult{}, err
	}

	periodStart, periodEnd, err := resolveGrantPeriod(input.PeriodStart, input.PeriodEnd, c.Now())
	if err != nil {
		return GrantLeaveBalancesResult{}, err
	}

	policyResp, err := c.loadAttendancePolicyResponse(ctx)
	if err != nil {
		return GrantLeaveBalancesResult{}, err
	}
	policyVersion := policyResp.Version
	if policyVersion <= 0 {
		policyVersion = 1
	}

	employees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
	if err != nil {
		return GrantLeaveBalancesResult{}, err
	}
	employeeID := strings.TrimSpace(input.EmployeeID)
	targets := make([]Employee, 0, len(employees))
	for _, employee := range employees {
		if employeeID != "" && employee.ID != employeeID {
			continue
		}
		if !isGrantableEmployee(employee) {
			continue
		}
		targets = append(targets, employee)
	}
	if employeeID != "" && len(targets) == 0 {
		return GrantLeaveBalancesResult{}, NotFound("employee", employeeID)
	}

	existing, err := c.store.ListLeaveBalances(goContext(ctx), ctx.TenantID)
	if err != nil {
		return GrantLeaveBalancesResult{}, err
	}
	existingByKey := map[string]LeaveBalance{}
	for _, balance := range existing {
		key := leaveBalanceGrantKey(balance.EmployeeID, balance.LeaveType, balance.PeriodStart, balance.PeriodEnd)
		existingByKey[key] = balance
	}

	result := GrantLeaveBalancesResult{
		PeriodStart:   periodStart.Format("2006-01-02"),
		PeriodEnd:     periodEnd.Format("2006-01-02"),
		PolicyVersion: policyVersion,
	}

	if err := c.Service.withTenantTransaction(ctx, func(txService *Service) error {
		tx := txService.Attendance()
		for _, employee := range targets {
			tenureStart, ok := employeeTenureStart(employee)
			if !ok {
				result.Failed++
				result.RowErrors = append(result.RowErrors, domain.RowError{
					Row:     len(result.RowErrors) + 1,
					Field:   "hire_date",
					Code:    "required",
					Message: "hire_date or tenure_start_date is required for leave grant",
				})
				continue
			}
			jobLevel := employeeJobLevel(employee)
			tenureYears := tenureYearsAt(tenureStart, periodStart)

			for _, leaveType := range policyResp.LeaveTypes {
				if !leaveType.Active || leaveType.GrantMode != domain.LeaveGrantModeAnnualGrant {
					continue
				}
				rule, matched := matchLeaveEntitlement(leaveType.Entitlements, jobLevel, tenureYears)
				if !matched {
					result.Skipped++
					continue
				}
				ratio := 1.0
				if rule.Prorate {
					ratio = employmentRatioInPeriod(tenureStart, employee.ResignDate, periodStart, periodEnd)
				}
				granted := roundHalfHour(rule.QuotaHours * ratio)
				key := leaveBalanceGrantKey(employee.ID, leaveType.Code, result.PeriodStart, result.PeriodEnd)
				current, exists := existingByKey[key]
				if exists && strings.EqualFold(strings.TrimSpace(current.Source), ehrmsAttendanceSource) {
					result.Skipped++
					continue
				}
				used := 0.0
				id := utils.NewID("lb")
				if exists {
					id = current.ID
					used = current.UsedHours
					if used <= 0 && current.GrantedHours > 0 && current.RemainingHours < current.GrantedHours {
						used = current.GrantedHours - current.RemainingHours
					}
					result.Updated++
				} else {
					result.Granted++
				}
				remaining := granted - used
				if remaining < 0 {
					remaining = 0
				}
				ratioCopy := ratio
				balance := LeaveBalance{
					ID:             id,
					TenantID:       ctx.TenantID,
					EmployeeID:     employee.ID,
					LeaveType:      leaveType.Code,
					LeaveTypeID:    leaveType.ID,
					RemainingHours: remaining,
					PeriodStart:    result.PeriodStart,
					PeriodEnd:      result.PeriodEnd,
					GrantedHours:   granted,
					UsedHours:      used,
					Source:         "policy_grant",
					PolicyVersion:  policyVersion,
					ProrateRatio:   &ratioCopy,
					UpdatedAt:      c.Now(),
				}
				if err := tx.store.UpsertLeaveBalance(goContext(ctx), balance); err != nil {
					return err
				}
				existingByKey[key] = balance
			}
		}
		if err := tx.audit(ctx, "attendance.leave_balance.grant", string(ResourceLeave), "", string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{
			"granted":        result.Granted,
			"updated":        result.Updated,
			"skipped":        result.Skipped,
			"failed":         result.Failed,
			"period_start":   result.PeriodStart,
			"period_end":     result.PeriodEnd,
			"policy_version": policyVersion,
			"employee_id":    employeeID,
		})); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		return GrantLeaveBalancesResult{}, err
	}
	return result, nil
}

// leaveBalanceGrantKey keeps policy grants isolated by entitlement period and source ownership.
func leaveBalanceGrantKey(employeeID, leaveType, periodStart, periodEnd string) string {
	return strings.Join([]string{strings.TrimSpace(employeeID), strings.ToLower(strings.TrimSpace(leaveType)), periodStart, periodEnd}, "|")
}

func resolveGrantPeriod(startRaw, endRaw string, now time.Time) (time.Time, time.Time, error) {
	startRaw = strings.TrimSpace(startRaw)
	endRaw = strings.TrimSpace(endRaw)
	if startRaw == "" && endRaw == "" {
		start := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(now.Year(), 12, 31, 0, 0, 0, 0, time.UTC)
		return start, end, nil
	}
	if startRaw == "" || endRaw == "" {
		return time.Time{}, time.Time{}, BadRequest("period_start and period_end are required together")
	}
	start, err := utils.ParseDate(startRaw)
	if err != nil {
		return time.Time{}, time.Time{}, BadRequest("period_start must be YYYY-MM-DD")
	}
	end, err := utils.ParseDate(endRaw)
	if err != nil {
		return time.Time{}, time.Time{}, BadRequest("period_end must be YYYY-MM-DD")
	}
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	if end.Before(start) {
		return time.Time{}, time.Time{}, BadRequest("period_end must be on or after period_start")
	}
	return start, end, nil
}

func isGrantableEmployee(employee Employee) bool {
	return attendanceEmployeeAllowsActiveOperations(employee)
}

func employeeJobLevel(employee Employee) string {
	return strings.TrimSpace(stringFromMap(employee.EmploymentInfo, "job_level"))
}

func employeeTenureStart(employee Employee) (time.Time, bool) {
	if raw := strings.TrimSpace(stringFromMap(employee.EmploymentInfo, "tenure_start_date")); raw != "" {
		if parsed, err := utils.ParseDate(raw); err == nil {
			return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC), true
		}
	}
	if employee.HireDate != nil && !employee.HireDate.IsZero() {
		t := employee.HireDate.UTC()
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true
	}
	if raw := strings.TrimSpace(stringFromMap(employee.EmploymentInfo, "hire_date")); raw != "" {
		if parsed, err := utils.ParseDate(raw); err == nil {
			return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC), true
		}
	}
	return time.Time{}, false
}

func tenureYearsAt(tenureStart, at time.Time) int {
	years := at.Year() - tenureStart.Year()
	anniversary := time.Date(at.Year(), tenureStart.Month(), tenureStart.Day(), 0, 0, 0, 0, time.UTC)
	if at.Before(anniversary) {
		years--
	}
	if years < 0 {
		return 0
	}
	return years
}

func matchLeaveEntitlement(rules []domain.LeaveEntitlementRule, jobLevel string, tenureYears int) (domain.LeaveEntitlementRule, bool) {
	var best domain.LeaveEntitlementRule
	found := false
	for _, rule := range rules {
		if rule.JobLevel != "*" && !strings.EqualFold(rule.JobLevel, jobLevel) {
			continue
		}
		if tenureYears < rule.TenureMinYears {
			continue
		}
		if rule.TenureMaxYears != nil && tenureYears >= *rule.TenureMaxYears {
			continue
		}
		if !found ||
			rule.Priority > best.Priority ||
			(rule.Priority == best.Priority && rule.TenureMinYears > best.TenureMinYears) ||
			(rule.Priority == best.Priority && rule.TenureMinYears == best.TenureMinYears && rule.JobLevel != "*" && best.JobLevel == "*") {
			best = rule
			found = true
		}
	}
	return best, found
}

func employmentRatioInPeriod(tenureStart time.Time, resignDate *time.Time, periodStart, periodEnd time.Time) float64 {
	periodDays := periodEnd.Sub(periodStart).Hours()/24 + 1
	if periodDays <= 0 {
		return 0
	}
	employedStart := tenureStart
	if employedStart.Before(periodStart) {
		employedStart = periodStart
	}
	employedEnd := periodEnd
	if resignDate != nil && !resignDate.IsZero() {
		resign := time.Date(resignDate.UTC().Year(), resignDate.UTC().Month(), resignDate.UTC().Day(), 0, 0, 0, 0, time.UTC)
		if resign.Before(employedEnd) {
			employedEnd = resign
		}
	}
	if employedEnd.Before(employedStart) {
		return 0
	}
	employedDays := employedEnd.Sub(employedStart).Hours()/24 + 1
	ratio := employedDays / periodDays
	if ratio > 1 {
		return 1
	}
	if ratio < 0 {
		return 0
	}
	return ratio
}
