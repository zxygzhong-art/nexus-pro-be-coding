package service

func (c HRService) QueryEmployees(ctx RequestContext, query EmployeeQuery) (PageResponse[Employee], error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return PageResponse[Employee]{}, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionRead})
	if err != nil {
		return PageResponse[Employee]{}, err
	}
	query = normalizeEmployeeQuery(query)
	if !decision.Allowed {
		c.logInfo(ctx, "employee query returned no rows because authorization was denied",
			"reason", decision.Reason,
			"missing_permissions", decision.MissingPermissions,
		)
		return PageResponse[Employee]{Items: []Employee{}, Page: query.Page, PageSize: query.PageSize, Sort: query.Sort}, nil
	}
	authzAudit := AuthzAudit{service: c.Service, target: AuditTarget{Event: "hr.employee.query", Resource: string(ResourceEmployeeCollection)}, decision: decision}
	if employeeDecisionCanUseStorePage(decision) {
		items, total, err := c.store.ListEmployeePageByQuery(goContext(ctx), ctx.TenantID, query)
		if err != nil {
			return PageResponse[Employee]{}, err
		}
		items, err = c.applyEmployeeDecision(ctx, account, items, decision)
		if err != nil {
			return PageResponse[Employee]{}, err
		}
		if err := authzAudit.Commit(ctx); err != nil {
			return PageResponse[Employee]{}, err
		}
		return PageResponse[Employee]{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, Sort: query.Sort}, nil
	}
	items, err := c.listEmployeesForQuery(ctx, query)
	if err != nil {
		return PageResponse[Employee]{}, err
	}
	items, err = c.applyEmployeeDecision(ctx, account, items, decision)
	if err != nil {
		return PageResponse[Employee]{}, err
	}
	items = filterEmployeeQuery(items, query)
	sortEmployees(items, query.Sort)
	total := len(items)
	items = paginateEmployees(items, query.Page, query.PageSize)
	if err := authzAudit.Commit(ctx); err != nil {
		return PageResponse[Employee]{}, err
	}
	return PageResponse[Employee]{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, Sort: query.Sort}, nil
}

func (c HRService) GetEmployee(ctx RequestContext, id string) (Employee, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return Employee{}, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionRead})
	if err != nil {
		return Employee{}, err
	}
	if !decision.Allowed {
		return Employee{}, forbiddenAuthz(decision)
	}
	employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return Employee{}, err
	}
	if !ok {
		return Employee{}, NotFound("employee", id)
	}
	visible, err := c.applyEmployeeDecision(ctx, account, []Employee{employee}, decision)
	if err != nil {
		return Employee{}, err
	}
	if len(visible) == 0 {
		return Employee{}, forbiddenDataScope("employee is outside data scope")
	}
	return visible[0], nil
}

func (c HRService) CreateEmployeeAggregate(ctx RequestContext, input CreateEmployeeInput) (Employee, error) {
	_, _, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionCreate},
		AuditTarget{Event: "hr.employee.create", Resource: string(ResourceEmployee)},
	)
	if err != nil {
		return Employee{}, err
	}
	var employee Employee
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
		next, err := tx.employeeFromCreateInput(ctx, input)
		if err != nil {
			return err
		}
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.touchEmployeeAuthzIfNeeded(ctx, Employee{}, next, string(EventEmployeeAuthzSubjectCreate)); err != nil {
			return err
		}
		if err := tx.linkEmployeeAccount(ctx, next); err != nil {
			return err
		}
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeCreated), next.ID, map[string]any{"employee_id": next.ID}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.create", string(ResourceEmployee), next.ID, string(SeverityMedium), map[string]any{"name": next.Name}); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx); err != nil {
			return err
		}
		employee = next
		return nil
	}); err != nil {
		return Employee{}, err
	}
	c.logInfo(ctx, "employee created",
		"employee_id", employee.ID,
		"employee_no", employee.EmployeeNo,
		"status", employeeStatus(employee),
		"account_id", employee.AccountID,
	)
	return employee, nil
}

func (c HRService) UpdateEmployee(ctx RequestContext, id string, input UpdateEmployeeInput) (Employee, error) {
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, ResourceID: id, Action: ActionUpdate},
		AuditTarget{Event: "hr.employee.update", Resource: string(ResourceEmployee), Target: id},
	)
	if err != nil {
		return Employee{}, err
	}
	var employee Employee
	previousStatus := ""
	if err := c.withTenantTransaction(ctx, func(tx *Service) error {
		next, ok, err := tx.store.GetEmployee(goContext(ctx), ctx.TenantID, id)
		if err != nil {
			return err
		}
		if !ok {
			return NotFound("employee", id)
		}
		visible, err := tx.filterEmployeesByDecision(ctx, account, []Employee{next}, decision)
		if err != nil {
			return err
		}
		if len(visible) == 0 {
			return forbiddenDataScope("employee is outside data scope")
		}
		if fields := forbiddenEmployeePatchFields(input, decision.FieldPolicies); len(fields) > 0 {
			return domainValidation("employee field policy denied update", fields...)
		}
		before := next
		previousStatus = employeeStatus(before)
		if err := tx.applyEmployeePatch(ctx, &next, input); err != nil {
			return err
		}
		next = tx.appendHistoryForChangedEmployment(before, next, "資料更新")
		if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.touchEmployeeAuthzIfNeeded(ctx, before, next, string(EventEmployeeAuthzSubjectUpdate)); err != nil {
			return err
		}
		if err := tx.linkEmployeeAccount(ctx, next); err != nil {
			return err
		}
		if err := tx.appendEmployeeEvent(ctx, string(EventEmployeeUpdated), next.ID, map[string]any{"employee_id": next.ID}); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.employee.update", string(ResourceEmployee), next.ID, string(SeverityMedium), map[string]any{"changed": true}); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx); err != nil {
			return err
		}
		employee = next
		return nil
	}); err != nil {
		return Employee{}, err
	}
	c.logInfo(ctx, "employee updated",
		"employee_id", employee.ID,
		"employee_no", employee.EmployeeNo,
		"previous_status", previousStatus,
		"status", employeeStatus(employee),
		"account_id", employee.AccountID,
	)
	return employee, nil
}

func (c HRService) EmployeeStats(ctx RequestContext, query EmployeeQuery) (EmployeeStats, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return EmployeeStats{}, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionRead})
	if err != nil {
		return EmployeeStats{}, err
	}
	if !decision.Allowed {
		return EmployeeStats{}, nil
	}
	query = normalizeEmployeeQuery(EmployeeQuery{
		Keyword:          query.Keyword,
		DepartmentID:     query.DepartmentID,
		EmploymentStatus: query.EmploymentStatus,
		Category:         query.Category,
		Sort:             query.Sort,
	})
	items, err := c.listEmployeesForQuery(ctx, query)
	if err != nil {
		return EmployeeStats{}, err
	}
	items, err = c.applyEmployeeDecision(ctx, account, items, decision)
	if err != nil {
		return EmployeeStats{}, err
	}
	items = filterEmployeeQuery(items, query)
	now := c.Now()
	stats := EmployeeStats{Total: len(items)}
	for _, item := range items {
		status := employeeStatus(item)
		switch status {
		case string(EmployeeStatusActive):
			stats.Active++
		case string(EmployeeStatusOnboarding):
			stats.Onboarding++
		case string(EmployeeStatusProbation):
			stats.Probation++
		case string(EmployeeStatusLeaveSuspended):
			stats.LeaveSuspended++
		case string(EmployeeStatusResigned):
			stats.Resigned++
		}
		if item.HireDate != nil && sameMonth(*item.HireDate, now) {
			stats.HiredThisMonth++
		}
		if item.ResignDate != nil && sameMonth(*item.ResignDate, now) {
			stats.LeftThisMonth++
		}
	}
	return stats, nil
}

func (c HRService) EmployeeOptions(ctx RequestContext) (EmployeeOptions, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return EmployeeOptions{}, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmployee, Action: ActionRead})
	if err != nil {
		return EmployeeOptions{}, err
	}
	if !decision.Allowed {
		return EmployeeOptions{}, forbiddenAuthz(decision)
	}
	employees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
	if err != nil {
		return EmployeeOptions{}, err
	}
	employees, err = c.applyEmployeeDecision(ctx, account, employees, decision)
	if err != nil {
		return EmployeeOptions{}, err
	}
	units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return EmployeeOptions{}, err
	}
	departments := employeeDepartmentOptions(units, employees)
	return EmployeeOptions{
		Departments:        departments,
		Positions:          uniqueSorted(employeeStringValues(employees, func(v Employee) string { return v.Position })),
		EmploymentStatuses: EmployeeStatuses(false),
		Categories:         EmployeeCategories(),
		JobGrades:          []string{"P1", "P2", "P3", "M1", "M2", "M3"},
		JobLevels:          []string{"junior", "mid", "senior", "staff"},
	}, nil
}

func employeeDepartmentOptions(units []OrgUnit, employees []Employee) []OrgUnit {
	visible := map[string]struct{}{}
	for _, employee := range employees {
		if employee.OrgUnitID != "" {
			visible[employee.OrgUnitID] = struct{}{}
		}
	}
	out := make([]OrgUnit, 0, len(visible))
	for _, unit := range units {
		if _, ok := visible[unit.ID]; ok {
			out = append(out, unit)
		}
	}
	return out
}
