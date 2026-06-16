package service

type MeService struct {
	*Service
}

func (c *Service) Me() MeService {
	return MeService{Service: c}
}

func (c *Service) ResolveMe(ctx RequestContext) (MeResponse, error) {
	return c.Me().Resolve(ctx)
}

func (c *Service) ListMenus(ctx RequestContext) ([]MenuNode, error) {
	return c.Me().ListMenus(ctx)
}

func (c MeService) Resolve(ctx RequestContext) (MeResponse, error) {
	account, tenant, err := c.resolveAccount(ctx)
	if err != nil {
		return MeResponse{}, err
	}
	decision, err := c.evaluateAuthz(ctx, account, CheckRequest{Resource: "me", Action: ActionRead})
	if err != nil {
		return MeResponse{}, err
	}
	if !decision.Allowed {
		return MeResponse{}, Forbidden(decision.Reason)
	}

	permissions, permissionSets, groups, err := c.resolveAccess(ctx, account)
	if err != nil {
		return MeResponse{}, err
	}

	var employee *Employee
	if account.EmployeeID != "" {
		v, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, account.EmployeeID)
		if err != nil {
			return MeResponse{}, err
		}
		if ok {
			emp := v
			employee = &emp
		}
	}

	role, _, err := c.activeAssumableRole(ctx, account)
	if err != nil {
		return MeResponse{}, err
	}
	var assumedRole *AssumableRole
	if role != nil {
		assumedRole = role
	}

	effectiveMenuKeys := uniqueStrings(menuKeysFromPermissions(permissions))
	capabilities := uniqueStrings(capabilitiesFromPermissions(permissions))

	return MeResponse{
		Tenant:               tenant,
		Account:              account,
		Employee:             employee,
		AssumedRole:          assumedRole,
		UserGroups:           groups,
		PermissionSets:       permissionSets,
		EffectivePermissions: permissions,
		EffectiveMenuKeys:    effectiveMenuKeys,
		Capabilities:         capabilities,
	}, nil
}

func (c MeService) ListMenus(ctx RequestContext) ([]MenuNode, error) {
	me, err := c.ResolveMe(ctx)
	if err != nil {
		return nil, err
	}
	allowed := map[string]struct{}{}
	for _, key := range me.EffectiveMenuKeys {
		allowed[key] = struct{}{}
	}
	return filterMenus(defaultMenuCatalog, allowed), nil
}
