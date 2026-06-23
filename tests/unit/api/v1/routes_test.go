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

func TestEmployeeOpenAPIRequestBodiesUseNamedSchemas(t *testing.T) {
	refs := openAPIRequestJSONSchemaRefs(t)
	expected := map[string]string{
		"POST /v1/hr/employees":                        "EmployeeInput",
		"POST /v1/hr/employees/preview":                "EmployeeInput",
		"PATCH /v1/hr/employees/{id}":                  "EmployeePatch",
		"POST /v1/hr/employees/{id}/preview":           "EmployeePatch",
		"POST /v1/hr/employees/import/preview":         "EmployeeImportPreviewRequest",
		"POST /v1/hr/employees/import/{id}/confirm":    "EmployeeImportConfirmRequest",
		"POST /v1/hr/employees/export":                 "EmployeeQuery",
		"POST /v1/hr/employees/batch-delete":           "BatchDeleteEmployeesRequest",
		"POST /v1/hr/employees/{id}/invite":            "InviteEmployeeRequest",
		"POST /v1/hr/employees/{id}/status-transition": "EmployeeStatusTransitionRequest",
		"PATCH /v1/hr/employees/{id}/status":           "EmployeeDirectStatusRequest",
	}
	for key, schemaName := range expected {
		if got := refs[key]; got != schemaName {
			t.Fatalf("%s request body uses %q, want %q", key, got, schemaName)
		}
	}
}

func TestEmployeeOpenAPIOperationsDocumentStandardErrors(t *testing.T) {
	routes := openAPIRouteKeys(t)
	refs := openAPIErrorResponseRefs(t)
	expected := map[string]string{
		"400": "ValidationError",
		"401": "Unauthenticated",
		"403": "Forbidden",
		"404": "NotFound",
		"409": "Conflict",
		"500": "InternalError",
	}
	for route := range routes {
		if !strings.Contains(route, " /v1/hr/employees") && !strings.HasSuffix(route, " /v1/hr/employee-options") {
			continue
		}
		for status, name := range expected {
			key := route + " " + status
			if got := refs[key]; got != name {
				t.Fatalf("%s response uses %q, want %q", key, got, name)
			}
		}
	}
}

func TestOpenAPIErrorSchemaSupportsFieldAndRowLocalization(t *testing.T) {
	raw := string(readOpenAPI(t))
	requiredSnippets := map[string]string{
		"FieldError required fields":        "    FieldError:\n      type: object\n      required: [field, code, message]",
		"FieldError tab property":           "        tab:\n          type: string",
		"RowError required fields":          "    RowError:\n      type: object\n      required: [row_number, field_errors]",
		"RowError field errors ref":         "        field_errors:\n          type: array\n          items:\n            $ref: \"#/components/schemas/FieldError\"",
		"Error top-level required":          "    Error:\n      type: object\n      required: [error]",
		"Error body required trace":         "        error:\n          type: object\n          required: [code, message, trace_id]",
		"Error field errors ref":            "            field_errors:\n              type: array\n              items:\n                $ref: \"#/components/schemas/FieldError\"",
		"Error row errors ref":              "            row_errors:\n              type: array\n              items:\n                $ref: \"#/components/schemas/RowError\"",
		"Preview field errors named schema": "        field_errors:\n          type: array\n          items:\n            $ref: \"#/components/schemas/FieldError\"",
	}
	for name, snippet := range requiredSnippets {
		if !strings.Contains(raw, snippet) {
			t.Fatalf("missing OpenAPI %s snippet:\n%s", name, snippet)
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

func openAPIRequestJSONSchemaRefs(t *testing.T) map[string]string {
	t.Helper()
	refs := map[string]string{}
	currentPath := ""
	currentMethod := ""
	inRequestBody := false
	inJSON := false
	for _, line := range strings.Split(string(readOpenAPI(t)), "\n") {
		if strings.HasPrefix(line, "components:") {
			break
		}
		if strings.HasPrefix(line, "  /") {
			currentPath = strings.TrimSuffix(strings.TrimSpace(line), ":")
			currentMethod = ""
			inRequestBody = false
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
			inRequestBody = false
			inJSON = false
			continue
		case "requestBody:":
			inRequestBody = currentMethod != ""
			inJSON = false
			continue
		case "responses:":
			inRequestBody = false
			inJSON = false
			continue
		case "application/json:":
			inJSON = inRequestBody && currentMethod != ""
			continue
		}
		if !inJSON || !strings.HasPrefix(trimmed, "$ref: \"#/components/schemas/") {
			continue
		}
		schema := strings.TrimPrefix(trimmed, "$ref: \"#/components/schemas/")
		schema = strings.TrimSuffix(schema, "\"")
		refs[currentMethod+" "+currentPath] = schema
		inJSON = false
	}
	return refs
}

func openAPIErrorResponseRefs(t *testing.T) map[string]string {
	t.Helper()
	refs := map[string]string{}
	currentPath := ""
	currentMethod := ""
	inResponses := false
	for _, line := range strings.Split(string(readOpenAPI(t)), "\n") {
		if strings.HasPrefix(line, "components:") {
			break
		}
		if strings.HasPrefix(line, "  /") {
			currentPath = strings.TrimSuffix(strings.TrimSpace(line), ":")
			currentMethod = ""
			inResponses = false
			continue
		}
		if currentPath == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case "get:", "post:", "patch:", "delete:":
			currentMethod = strings.ToUpper(strings.TrimSuffix(trimmed, ":"))
			inResponses = false
			continue
		case "responses:":
			inResponses = currentMethod != ""
			continue
		case "requestBody:":
			inResponses = false
			continue
		}
		status, response, ok := openAPIInlineResponseRef(trimmed)
		if inResponses && ok {
			refs[currentMethod+" "+currentPath+" "+status] = response
		}
	}
	return refs
}

func openAPIInlineResponseRef(trimmed string) (string, string, bool) {
	if !strings.HasPrefix(trimmed, "\"") {
		return "", "", false
	}
	statusEnd := strings.Index(trimmed[1:], "\"")
	if statusEnd < 0 {
		return "", "", false
	}
	prefix := `{$ref: "#/components/responses/`
	refStart := strings.Index(trimmed, prefix)
	if refStart < 0 {
		return "", "", false
	}
	status := trimmed[1 : statusEnd+1]
	response := strings.TrimSpace(trimmed[refStart+len(prefix):])
	response = strings.TrimSuffix(response, `"}`)
	return status, response, response != ""
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
