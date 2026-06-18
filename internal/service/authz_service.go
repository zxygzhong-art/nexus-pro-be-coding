package service

import "go.opentelemetry.io/otel/attribute"

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

func (c AuthzService) Check(ctx RequestContext, req CheckRequest) (result CheckResult, err error) {
	ctx, span := startServiceSpan(ctx, "service.authz.check", authzSpanAttributes(req)...)
	defer func() {
		setAuthzSpanResult(span, result)
		finishServiceSpan(span, err)
	}()
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	result, err = c.evaluateAuthz(ctx, account, req)
	return result, err
}

func (c AuthzService) BatchCheck(ctx RequestContext, req BatchCheckRequest) (result BatchCheckResult, err error) {
	ctx, span := startServiceSpan(ctx, "service.authz.batch_check")
	defer func() {
		span.SetAttributes(attribute.Int("authz.batch_size", len(req.Checks)))
		finishServiceSpan(span, err)
	}()
	results := make([]CheckResult, 0, len(req.Checks))
	for _, item := range req.Checks {
		itemResult, err := c.Check(ctx, item)
		if err != nil {
			return BatchCheckResult{}, err
		}
		results = append(results, itemResult)
	}
	return BatchCheckResult{Results: results}, nil
}

func (c AuthzService) ValidateApprovalInstance(ctx RequestContext, req CheckRequest) error {
	return c.Service.ValidateApprovalInstance(ctx, req)
}
