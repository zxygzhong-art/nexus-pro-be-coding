package v1

import (
	"net/http"

	platformauth "nexus-pro-be/internal/platform/auth"
)

// TokenContext is the trusted identity extracted from an authenticated token.
type TokenContext = platformauth.TokenContext

// TokenResolver resolves trusted identity from an HTTP request.
type TokenResolver = platformauth.TokenResolver

type noTokenResolver struct{}

// Resolve reports that no authenticated token was present.
func (noTokenResolver) Resolve(*http.Request) (TokenContext, bool, error) {
	return TokenContext{}, false, nil
}

// unsignedJWTResolver is enabled only for explicit local/demo modes.
type unsignedJWTResolver = platformauth.UnsignedJWTResolver
