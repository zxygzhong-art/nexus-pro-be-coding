package v1_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	v1api "nexus-pro-be/internal/api/v1"
	authzpkg "nexus-pro-be/internal/domain/authz"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestRegisteredRoutesMatchAuthzPolicies(t *testing.T) {
	router, ok := v1api.New(service.New(memory.NewStore()), nil).Routes().(*gin.Engine)
	if !ok {
		t.Fatal("expected routes to be backed by gin engine")
	}

	policies := map[string]struct{}{}
	for _, policy := range authzpkg.DefaultRoutePolicies {
		if strings.HasPrefix(policy.Path, "/internal/") {
			continue
		}
		policies[policy.Method+" "+policy.Path] = struct{}{}
	}

	routes := map[string]struct{}{}
	openAPIRoutes := openAPIRouteKeys(t)
	documentedRoutes := map[string]struct{}{}
	for _, route := range router.Routes() {
		if isPublicRoute(route.Path) {
			continue
		}
		key := route.Method + " " + route.Path
		docKey := route.Method + " " + openAPIPath(route.Path)
		routes[key] = struct{}{}
		documentedRoutes[docKey] = struct{}{}
		if _, ok := policies[key]; !ok {
			t.Fatalf("registered route has no authz policy: %s", key)
		}
		if _, ok := openAPIRoutes[docKey]; !ok {
			t.Fatalf("registered route has no openapi path: %s", docKey)
		}
	}

	for key := range policies {
		if _, ok := routes[key]; !ok {
			t.Fatalf("authz policy has no registered route: %s", key)
		}
	}

	for key := range openAPIRoutes {
		if _, ok := documentedRoutes[key]; !ok {
			t.Fatalf("openapi path has no registered route: %s", key)
		}
	}
}

func TestPermissionSetAssignmentPoliciesUseDedicatedResource(t *testing.T) {
	found := 0
	for _, policy := range authzpkg.DefaultRoutePolicies {
		if policy.Path != "/v1/iam/permission-set-assignments" {
			continue
		}
		found++
		if policy.ResourceType != "permission_set_assignment" {
			t.Fatalf("expected assignment policy to use dedicated resource, got %q for %s", policy.ResourceType, policy.Name)
		}
	}
	if found != 2 {
		t.Fatalf("expected read and create assignment policies, got %d", found)
	}
}

func isPublicRoute(path string) bool {
	switch path {
	case "/healthz", "/readyz", "/openapi.yaml", "/swagger", "/swagger/*any":
		return true
	default:
		return false
	}
}

func openAPIPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			parts[i] = "{" + part[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}

func openAPIRouteKeys(t *testing.T) map[string]struct{} {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "docs", "openapi.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]struct{}{}
	currentPath := ""
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "  /") {
			currentPath = strings.TrimSuffix(strings.TrimSpace(line), ":")
			continue
		}
		if currentPath == "" {
			continue
		}
		method := strings.TrimSuffix(strings.TrimSpace(line), ":")
		switch method {
		case "get", "post", "patch", "delete":
			keys[strings.ToUpper(method)+" "+currentPath] = struct{}{}
		}
	}
	return keys
}
