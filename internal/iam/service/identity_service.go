// Package service holds IAM application services orchestrating the authorizer
// and repository. Business HR logic is intentionally out of scope here.
package service

import (
	"context"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/adapters/authorizer"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/authz"
)

// IdentityService computes capability summaries for the current principal.
type IdentityService struct {
	az authorizer.Authorizer
}

// NewIdentityService builds the service.
func NewIdentityService(az authorizer.Authorizer) *IdentityService {
	return &IdentityService{az: az}
}

// capabilityChecks are the pre-computed capabilities the frontend consumes.
var capabilityChecks = []struct {
	name string
	req  authz.Request
}{
	{"can_manage_hr", authz.Request{ApplicationCode: "hr", ResourceType: "employee", Action: "write"}},
	{"can_manage_iam", authz.Request{ApplicationCode: "iam", ResourceType: "iam", Action: "write"}},
	{"can_run_agent", authz.Request{ApplicationCode: "hr", ResourceType: "agent", Action: "run"}},
}

// Capabilities returns the capability map for the principal in ctx.
func (s *IdentityService) Capabilities(ctx context.Context) (map[string]authz.Decision, error) {
	out := make(map[string]authz.Decision, len(capabilityChecks))
	for _, cc := range capabilityChecks {
		dec, err := s.az.Check(ctx, cc.req)
		if err != nil {
			return nil, err
		}
		out[cc.name] = dec
	}
	return out, nil
}
