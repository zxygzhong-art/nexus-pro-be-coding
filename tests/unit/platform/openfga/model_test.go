package openfga_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestAuthorizationModelContainsOrgScopeTypes 驗證模型包含本批組織 scope types。
func TestAuthorizationModelContainsOrgScopeTypes(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	modelPath := filepath.Join(filepath.Dir(filename), "..", "..", "..", "..", "ops", "openfga", "model.json")
	raw, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	var model struct {
		SchemaVersion   string `json:"schema_version"`
		TypeDefinitions []struct {
			Type      string                    `json:"type"`
			Relations map[string]map[string]any `json:"relations"`
		} `json:"type_definitions"`
	}
	if err := json.Unmarshal(raw, &model); err != nil {
		t.Fatal(err)
	}
	if model.SchemaVersion != "1.1" {
		t.Fatalf("schema_version = %q, want 1.1", model.SchemaVersion)
	}
	types := map[string]map[string]map[string]any{}
	for _, def := range model.TypeDefinitions {
		types[def.Type] = def.Relations
	}
	for _, typ := range []string{"tenant", "org_unit", "user_group", "hr.employee"} {
		if _, ok := types[typ]; !ok {
			t.Fatalf("expected type %q in model", typ)
		}
	}
	for typ, relations := range map[string][]string{
		"tenant":      {"member"},
		"org_unit":    {"parent", "member", "manager", "member_recursive"},
		"user_group":  {"member"},
		"hr.employee": {"owner", "manager", "org", "read"},
	} {
		for _, relation := range relations {
			if _, ok := types[typ][relation]; !ok {
				t.Fatalf("expected %s#%s in model", typ, relation)
			}
		}
	}
}
