package v1_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v2"

	v1api "nexus-pro-api/internal/api/v1"
	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// TestOpenAPIYAMLParsesStructurally 驗證 open API YAML parses structurally。
func TestOpenAPIYAMLParsesStructurally(t *testing.T) {
	var doc struct {
		OpenAPI string `yaml:"openapi"`
		Info    struct {
			Title       string `yaml:"title"`
			Version     string `yaml:"version"`
			Description string `yaml:"description"`
		} `yaml:"info"`
		Paths      map[string]map[string]any `yaml:"paths"`
		Components struct {
			Parameters map[string]any `yaml:"parameters"`
			Responses  map[string]any `yaml:"responses"`
			Schemas    map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(readOpenAPI(t), &doc); err != nil {
		t.Fatalf("openapi.yaml should parse as YAML: %v", err)
	}
	if doc.OpenAPI != "3.0.3" || doc.Info.Title == "" || doc.Info.Version == "" {
		t.Fatalf("unexpected OpenAPI metadata: %+v", doc.Info)
	}
	if !strings.Contains(doc.Info.Description, "globally unique across tenants") {
		t.Fatalf("expected OpenAPI metadata to document tenant resource id invariants: %q", doc.Info.Description)
	}
	for _, path := range []string{"/v1/hr/employees", "/v1/attendance/clock-records", "/v1/iam/permission-sets"} {
		if _, ok := doc.Paths[path]; !ok {
			t.Fatalf("expected structured OpenAPI path %s", path)
		}
	}
	for _, schema := range []string{"Employee", "AttendanceClockRecord", "Error"} {
		if _, ok := doc.Components.Schemas[schema]; !ok {
			t.Fatalf("expected structured OpenAPI schema %s", schema)
		}
	}
	if len(doc.Components.Parameters) == 0 || len(doc.Components.Responses) == 0 {
		t.Fatal("expected OpenAPI components to include parameters and responses")
	}
}

// TestPlatformClockSummaryIsOptionalInAggregates keeps permission-projected widgets aligned with the public contract.
func TestPlatformClockSummaryIsOptionalInAggregates(t *testing.T) {
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string       `yaml:"required"`
				Properties map[string]any `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(readOpenAPI(t), &doc); err != nil {
		t.Fatalf("openapi.yaml should parse as YAML: %v", err)
	}
	for _, name := range []string{"PlatformAssistantsResponse", "PlatformTasksResponse"} {
		schema, ok := doc.Components.Schemas[name]
		if !ok {
			t.Fatalf("expected OpenAPI schema %s", name)
		}
		if _, ok := schema.Properties["clock_summary"]; !ok {
			t.Fatalf("expected %s to define optional clock_summary", name)
		}
		for _, required := range schema.Required {
			if required == "clock_summary" {
				t.Fatalf("expected %s.clock_summary to be optional", name)
			}
		}
	}
	clockSchema, ok := doc.Components.Schemas["PlatformClockSummary"]
	if !ok {
		t.Fatal("expected PlatformClockSummary schema")
	}
	if _, ok := clockSchema.Properties["monthly_overtime_hours"]; !ok {
		t.Fatal("expected PlatformClockSummary to document monthly_overtime_hours")
	}
	foundRequired := false
	for _, required := range clockSchema.Required {
		if required == "monthly_overtime_hours" {
			foundRequired = true
			break
		}
	}
	if !foundRequired {
		t.Fatal("expected monthly_overtime_hours to be required like the non-omitempty response field")
	}
}

// TestRegisteredRoutesMatchAuthzPolicies 驗證 registered 路由 match 授權政策。
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
		docKey := route.Method + " " + openAPIPath(route.Path)
		if isPublicRoute(route.Path) {
			documentedRoutes[docKey] = struct{}{}
			continue
		}
		key := route.Method + " " + route.Path
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

// TestPermissionSetAssignmentPoliciesUseDedicatedResource 驗證權限集合指派政策 use dedicated resource。
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

// TestIAMOptionRoutesUseEntityReadPolicies 驗證 IAM options 端點沿用實體 read 權限並已註冊路由。
func TestIAMOptionRoutesUseEntityReadPolicies(t *testing.T) {
	want := map[string]string{
		"GET /v1/iam/accounts/options":        "account",
		"GET /v1/iam/permission-sets/options": "permission_set",
		"GET /v1/iam/user-groups/options":     "user_group",
		"GET /v1/iam/assumable-roles/options": "assumable_role",
	}
	found := map[string]struct{}{}
	for _, policy := range domain.DefaultRoutePolicies {
		key := policy.Method + " " + policy.Path
		resource, ok := want[key]
		if !ok {
			continue
		}
		found[key] = struct{}{}
		if policy.ApplicationCode != "iam" || policy.ResourceType != resource || policy.Action != "read" {
			t.Fatalf("unexpected policy for %s: %+v", key, policy)
		}
		if policy.RiskLevel != "" {
			t.Fatalf("options policy %s should stay low risk like hr.employee.options, got %q", key, policy.RiskLevel)
		}
	}
	for key := range want {
		if _, ok := found[key]; !ok {
			t.Fatalf("missing authz policy for options route %s", key)
		}
	}

	router, ok := v1api.New(service.New(memory.NewStore()), nil).Routes().(*gin.Engine)
	if !ok {
		t.Fatal("expected routes to be backed by gin engine")
	}
	registered := map[string]struct{}{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	for key := range want {
		if _, ok := registered[key]; !ok {
			t.Fatalf("options route not registered: %s", key)
		}
	}
}

// TestWorkflowReviewMutationsRequireApprove prevents read-only reviewers from mutating workflow state.
func TestWorkflowReviewMutationsRequireApprove(t *testing.T) {
	want := map[string]bool{
		"/v1/workflows/reviews/bulk-action": false,
		"/v1/workflows/forms/:id/approve":   false,
		"/v1/workflows/forms/:id/reject":    false,
		"/v1/workflows/forms/:id/return":    false,
	}
	for _, policy := range domain.DefaultRoutePolicies {
		if _, ok := want[policy.Path]; !ok {
			continue
		}
		want[policy.Path] = true
		if policy.Method != http.MethodPost || policy.Action != "approve" {
			t.Fatalf("workflow review mutation must require approve: %+v", policy)
		}
	}
	for path, found := range want {
		if !found {
			t.Fatalf("missing workflow review policy for %s", path)
		}
	}
}

// TestLocalLeaveTypePolicySeparatesReadAndUpdate protects the tenant override mutation.
func TestLocalLeaveTypePolicySeparatesReadAndUpdate(t *testing.T) {
	want := map[string]string{
		"GET /v1/attendance/leave-types":       "read",
		"PATCH /v1/attendance/leave-types/:id": "update",
	}
	for _, policy := range domain.DefaultRoutePolicies {
		key := policy.Method + " " + policy.Path
		action, ok := want[key]
		if !ok {
			continue
		}
		if policy.Action != action {
			t.Fatalf("expected %s to require %s, got %+v", key, action, policy)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing local leave type policies: %+v", want)
	}
}

// TestAgentUsagePoliciesUseDedicatedHighRiskResource prevents definition readers regaining account usage access.
func TestAgentUsagePoliciesUseDedicatedHighRiskResource(t *testing.T) {
	found := 0
	for _, policy := range domain.DefaultRoutePolicies {
		if !strings.HasPrefix(policy.Path, "/v1/workspace/agent-usage") {
			continue
		}
		found++
		if policy.ApplicationCode != "agent" || policy.ResourceType != "usage" || policy.Action != "read" {
			t.Fatalf("expected dedicated agent.usage:read policy, got %+v", policy)
		}
		if policy.RiskLevel != domain.RiskHigh {
			t.Fatalf("expected usage policy to be high risk, got %+v", policy)
		}
	}
	if found != 2 {
		t.Fatalf("expected overview and session usage policies, got %d", found)
	}
}

// TestDocumentedJSONSuccessResponsesUseDataEnvelope 驗證 documented JSON success 回應 use 資料 envelope。
func TestDocumentedJSONSuccessResponsesUseDataEnvelope(t *testing.T) {
	raw := string(readOpenAPI(t))
	for _, inlineStatus := range []string{`"200": {description:`, `"201": {description:`} {
		if strings.Contains(raw, inlineStatus) {
			t.Fatalf("documented success responses must include JSON data envelope content, found inline %s", inlineStatus)
		}
	}

	refs := openAPISuccessJSONSchemaRefs(t)
	expected := map[string]string{
		"GET /v1/hr/employees 200":                       "EmployeeListDataResponse",
		"GET /v1/hr/employees/{id} 200":                  "EmployeeDetailDataResponse",
		"PATCH /v1/hr/employees/{id} 200":                "EmployeeDetailDataResponse",
		"GET /v1/hr/employees/stats 200":                 "EmployeeStatsDataResponse",
		"POST /v1/hr/employees/ehrms/sync 200":           "EHRMSEmployeeSyncDataResponse",
		"POST /v1/org/units/ehrms/sync 200":              "EHRMSOrgUnitSyncDataResponse",
		"POST /v1/attendance/ehrms/leave-types/sync 200": "EHRMSLeaveTypeSyncDataResponse",
		"POST /v1/attendance/ehrms/sync 200":             "EHRMSAttendanceSyncDataResponse",
		"GET /v1/workspace/org-units-directory 200":      "WorkspaceOrgUnitDirectoryDataResponse",
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

// TestEmployeeOpenAPIRequestBodiesUseNamedSchemas 驗證員工 OpenAPI 請求 bodies use named schemas。
func TestEmployeeOpenAPIRequestBodiesUseNamedSchemas(t *testing.T) {
	refs := openAPIRequestJSONSchemaRefs(t)
	expected := map[string]string{
		"PATCH /v1/hr/employees/{id}":      "EmployeePatch",
		"POST /v1/hr/employees/ehrms/sync": "EHRMSEmployeeSyncRequest",
		"POST /v1/attendance/ehrms/sync":   "EHRMSAttendanceSyncRequest",
	}
	for key, schemaName := range expected {
		if got := refs[key]; got != schemaName {
			t.Fatalf("%s request body uses %q, want %q", key, got, schemaName)
		}
	}
}

// TestAssumableRoleCreateOpenAPIUsesSafetyContract verifies that the public request names every service-required policy.
func TestAssumableRoleCreateOpenAPIUsesSafetyContract(t *testing.T) {
	refs := openAPIRequestJSONSchemaRefs(t)
	if got := refs["POST /v1/iam/assumable-roles"]; got != "CreateAssumableRoleInput" {
		t.Fatalf("assumable role create request body uses %q, want CreateAssumableRoleInput", got)
	}

	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Required []string `yaml:"required"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(readOpenAPI(t), &doc); err != nil {
		t.Fatalf("openapi.yaml should parse as YAML: %v", err)
	}
	required := map[string]bool{}
	for _, field := range doc.Components.Schemas["CreateAssumableRoleInput"].Required {
		required[field] = true
	}
	for _, field := range []string{"name", "trusted", "trust_policy", "permission_boundary"} {
		if !required[field] {
			t.Fatalf("CreateAssumableRoleInput must require %s", field)
		}
	}

	errors := openAPIErrorResponseRefs(t)
	for status, response := range map[string]string{"400": "ValidationError", "401": "Unauthenticated", "403": "Forbidden"} {
		key := "POST /v1/iam/assumable-roles " + status
		if got := errors[key]; got != response {
			t.Fatalf("%s response uses %q, want %q", key, got, response)
		}
	}
}

// TestEmployeeOpenAPIOperationsDocumentStandardErrors 驗證員工 OpenAPI operations document standard 錯誤。
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

// TestOpenAPIErrorSchemaSupportsFieldAndRowLocalization 驗證 OpenAPI 錯誤 schema supports 欄位 and 列 localization。
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

// isPublicRoute 驗證 public 路由。
func isPublicRoute(path string) bool {
	switch path {
	case "/healthz", "/readyz", "/openapi.yaml", "/swagger", "/swagger/*any", "/v1/auth/sso/google/verify":
		return true
	default:
		return false
	}
}

// openAPIPath 驗證 OpenAPI path。
func openAPIPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			parts[i] = "{" + part[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}

// openAPIRouteKeys 驗證 OpenAPI 路由 keys。
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
		case "get", "post", "put", "patch", "delete":
			keys[strings.ToUpper(method)+" "+currentPath] = struct{}{}
		}
	}
	return keys
}

// openAPISuccessJSONSchemaRefs 驗證 OpenAPI success JSON schema refs。
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
		case "get:", "post:", "put:", "patch:", "delete:":
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

// openAPIRequestJSONSchemaRefs 驗證 OpenAPI 請求 JSON schema refs。
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
		case "get:", "post:", "put:", "patch:", "delete:":
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

// openAPIErrorResponseRefs 驗證 OpenAPI 錯誤回應 refs。
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
		case "get:", "post:", "put:", "patch:", "delete:":
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

// openAPIInlineResponseRef 驗證 OpenAPI inline 回應 ref。
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

// readOpenAPI 驗證 OpenAPI。
func readOpenAPI(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "docs", "openapi.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
