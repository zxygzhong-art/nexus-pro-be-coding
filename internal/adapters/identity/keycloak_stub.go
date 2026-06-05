package identity

import "context"

// KeycloakProvider validates a bearer JWT against Keycloak (OIDC) and maps the
// token subject to an account via iam_user_identities. It is a stub: until
// KEYCLOAK_ENABLED=true and JWKS validation is implemented, Resolve returns
// ErrNotImplemented for tokens. The middleware falls back to the header provider
// in development.
type KeycloakProvider struct {
	Issuer  string
	JWKSURL string
	Enabled bool
}

// NewKeycloakProvider constructs the stub.
func NewKeycloakProvider(issuer, jwksURL string, enabled bool) *KeycloakProvider {
	return &KeycloakProvider{Issuer: issuer, JWKSURL: jwksURL, Enabled: enabled}
}

// Resolve validates the bearer token. Not yet implemented.
//
// TODO: fetch JWKS from JWKSURL, verify signature/issuer/audience/expiry, then
// look up iam_user_identities by (provider='keycloak', subject=sub) to resolve
// tenant_id + account_id.
func (p *KeycloakProvider) Resolve(_ context.Context, h Headers) (Identity, error) {
	if h.BearerToken == "" {
		return Identity{}, ErrUnauthenticated
	}
	return Identity{}, ErrNotImplemented
}
