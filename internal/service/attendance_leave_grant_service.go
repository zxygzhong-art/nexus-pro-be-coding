package service

// GrantLeaveBalances is retained as a compatibility endpoint. Local annual
// grants were policy-seed driven and are retired; EHRMS synchronization owns
// leave-balance creation and updates.
func (c AttendanceService) GrantLeaveBalances(ctx RequestContext, _ GrantLeaveBalancesInput) (GrantLeaveBalancesResult, error) {
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceLeave, ActionUpdate, ""); err != nil {
		return GrantLeaveBalancesResult{}, err
	}
	return GrantLeaveBalancesResult{}, BadRequest("local leave balance grants are retired; use EHRMS attendance sync")
}
