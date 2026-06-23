package v1_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	v1api "nexus-pro-be/internal/api/v1"
	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestRegisteredRoutesMatchAuthzPolicies(t *testing.T) {
	router, ok := v1api.New(service.New(memory.NewStore()), nil).Routes().(*gin.Engine)
	if !ok {
		t.Fatal("expected routes to be backed by gin engine")
	}

	policies := map[string]struct{}{}
	for _, policy := range domain.DefaultRoutePolicies {
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
	for _, policy := range domain.DefaultRoutePolicies {
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

func TestDocumentedJSONSuccessResponsesUseDataEnvelope(t *testing.T) {
	raw := string(readOpenAPI(t))
	for _, inlineStatus := range []string{`"200": {description:`, `"201": {description:`} {
		if strings.Contains(raw, inlineStatus) {
			t.Fatalf("documented success responses must include JSON data envelope content, found inline %s", inlineStatus)
		}
	}

	refs := openAPISuccessJSONSchemaRefs(t)
	expected := map[string]string{
		"GET /v1/hr/employees 200":                         "EmployeeListDataResponse",
		"POST /v1/hr/employees 201":                        "EmployeeDetailDataResponse",
		"POST /v1/hr/employees/preview 200":                "EmployeePreviewDataResponse",
		"GET /v1/hr/employees/{id} 200":                    "EmployeeDetailDataResponse",
		"PATCH /v1/hr/employees/{id} 200":                  "EmployeeDetailDataResponse",
		"DELETE /v1/hr/employees/{id} 200":                 "EmployeeDataResponse",
		"POST /v1/hr/employees/{id}/preview 200":           "EmployeePreviewDataResponse",
		"POST /v1/hr/employees/{id}/avatar 200":            "EmployeeDataResponse",
		"DELETE /v1/hr/employees/{id}/avatar 200":          "EmployeeDataResponse",
		"GET /v1/hr/employees/stats 200":                   "EmployeeStatsDataResponse",
		"GET /v1/hr/employee-options 200":                  "EmployeeOptionsDataResponse",
		"POST /v1/hr/employees/import/preview 201":         "EmployeeImportSessionDataResponse",
		"POST /v1/hr/employees/import/{id}/confirm 200":    "EmployeeImportSessionDataResponse",
		"POST /v1/hr/employees/export 200":                 "EmployeeExportDataResponse",
		"POST /v1/hr/employees/batch-delete 200":           "BatchEmployeeDataResponse",
		"POST /v1/hr/employees/{id}/invite 200":            "EmployeeDataResponse",
		"POST /v1/hr/employees/{id}/status-transition 200": "EmployeeDataResponse",
		"PATCH /v1/hr/employees/{id}/status 200":           "EmployeeDataResponse",
	}
	for key, want := range expected {
		if got := refs[key]; got != want {
			t.Fatalf("documented success response %s uses %q, want %q", key, got, want)
		}
	}
	for key, schema := range refs {
		if !strings.HasSuffix(schema, "DataResponse") {
			t.Fatalf("documented success response %s uses %q without data envelope", key, schema)
		}
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
	keys := map[string]struct{}{}
	currentPath := ""
	for _, line := range strings.Split(string(readOpenAPI(t)), "\n") {
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

func openAPISuccessJSONSchemaRefs(t *testing.T) map[string]string {
	t.Helper()
	refs := map[string]string{}
	currentPath := ""
	currentMethod := ""
	currentStatus := ""
	inJSON := false
	for _, line := range strings.Split(string(readOpenAPI(t)), "\n") {
		if strings.HasPrefix(line, "components:") {
			break
		}
		if strings.HasPrefix(line, "  /") {
			currentPath = strings.TrimSuffix(strings.TrimSpace(line), ":")
			currentMethod = ""
			currentStatus = ""
			inJSON = false
			continue
		}
		if currentPath == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case "get:", "post:", "patch:", "delete:":
			currentMethod = strings.ToUpper(strings.TrimSuffix(trimmed, ":"))
			currentStatus = ""
			inJSON = false
			continue
		case "application/json:":
			inJSON = currentStatus != "" && currentMethod != ""
			continue
		}
		if strings.HasPrefix(trimmed, "\"") && strings.Contains(trimmed, "\":") {
			statusEnd := strings.Index(trimmed[1:], "\"")
			if statusEnd < 0 {
				currentStatus = ""
				inJSON = false
				continue
			}
			status := trimmed[1 : statusEnd+1]
			if (status == "200" || status == "201") && currentMethod != "" {
				currentStatus = status
			} else {
				currentStatus = ""
			}
			inJSON = false
			continue
		}
		if !inJSON || !strings.HasPrefix(trimmed, "$ref: \"#/components/schemas/") {
			continue
		}
		schema := strings.TrimPrefix(trimmed, "$ref: \"#/components/schemas/")
		schema = strings.TrimSuffix(schema, "\"")
		refs[currentMethod+" "+currentPath+" "+currentStatus] = schema
		inJSON = false
	}
	return refs
}

func readOpenAPI(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "docs", "openapi.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
