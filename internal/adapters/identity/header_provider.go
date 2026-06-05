package identity

import "context"

// HeaderProvider trusts X-Tenant-ID / X-Account-ID headers. It is the development
// default and preserves the prototype's behavior (defaults tenant-ikala /
// acct-hr-admin) so the existing frontend works unchanged. Account existence is
// validated downstream within the tenant-scoped transaction.
type HeaderProvider struct {
	DefaultTenantID  string
	DefaultAccountID string
}

// NewHeaderProvider builds a provider with prototype-compatible defaults.
func NewHeaderProvider() *HeaderProvider {
	return &HeaderProvider{DefaultTenantID: "tenant-ikala", DefaultAccountID: "acct-hr-admin"}
}

// Resolve maps headers to an Identity, applying defaults when absent.
func (p *HeaderProvider) Resolve(_ context.Context, h Headers) (Identity, error) {
	tenant := h.TenantID
	if tenant == "" {
		tenant = p.DefaultTenantID
	}
	account := h.AccountID
	if account == "" {
		account = p.DefaultAccountID
	}
	return Identity{TenantID: tenant, AccountID: account}, nil
}
