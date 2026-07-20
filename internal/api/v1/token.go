package v1

import (
	"net/http"

	"nexus-pro-api/internal/domain"
	platformauth "nexus-pro-api/internal/platform/auth"
)

// TokenContext 表示 token context。
type TokenContext = domain.AuthenticatedPrincipal

// TokenResolver 表示 token resolver。
type TokenResolver = platformauth.TokenResolver

type noTokenResolver struct{}

// Resolve 解析 token 並回傳可信身分。
func (noTokenResolver) Resolve(*http.Request) (TokenContext, bool, error) {
	return TokenContext{}, false, nil
}
