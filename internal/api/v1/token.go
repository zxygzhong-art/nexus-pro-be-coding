package v1

import (
	"net/http"

	platformauth "nexus-pro-be/internal/platform/auth"
)

type TokenContext = platformauth.TokenContext
type TokenResolver = platformauth.TokenResolver

type noTokenResolver struct{}

func (noTokenResolver) Resolve(*http.Request) (TokenContext, bool, error) {
	return TokenContext{}, false, nil
}

type unsignedJWTResolver = platformauth.UnsignedJWTResolver
