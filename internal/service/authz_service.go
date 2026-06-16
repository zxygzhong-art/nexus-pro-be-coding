package service

type AuthzService struct {
	*Service
}

func (c *Service) Authz() AuthzService {
	return AuthzService{Service: c}
}

func (c *Service) Check(ctx RequestContext, req CheckRequest) (CheckResult, error) {
	return c.Authz().Check(ctx, req)
}

func (c *Service) BatchCheck(ctx RequestContext, req BatchCheckRequest) (BatchCheckResult, error) {
	return c.Authz().BatchCheck(ctx, req)
}

func (c AuthzService) Check(ctx RequestContext, req CheckRequest) (CheckResult, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	return c.evaluateAuthz(ctx, account, req)
}

func (c AuthzService) BatchCheck(ctx RequestContext, req BatchCheckRequest) (BatchCheckResult, error) {
	results := make([]CheckResult, 0, len(req.Checks))
	for _, item := range req.Checks {
		result, err := c.Check(ctx, item)
		if err != nil {
			return BatchCheckResult{}, err
		}
		results = append(results, result)
	}
	return BatchCheckResult{Results: results}, nil
}
