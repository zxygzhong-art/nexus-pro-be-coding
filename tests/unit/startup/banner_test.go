package startup_test

import (
	"strings"
	"testing"

	"nexus-pro-be/internal/startup"
)

// TestRenderIncludesRuntimeAndDependencies 驗證 render includes runtime and 依賴。
func TestRenderIncludesRuntimeAndDependencies(t *testing.T) {
	output := startup.Render(startup.Report{
		Name:       "nexus-pro-be",
		Env:        "development",
		HTTPAddr:   ":8080",
		Repository: "postgresql",
		Dependencies: []startup.Dependency{
			{Name: "PostgreSQL", Status: "connected", Target: "postgres://nexus@localhost:5432/nexus_pro_be", Detail: "repository backend"},
			{Name: "Redis", Status: "skipped", Target: "REDIS_ADDR not set", Detail: "authz snapshots disabled"},
		},
	})

	for _, want := range []string{
		"nexus-pro-be",
		"env=development",
		"addr=:8080",
		"repository=postgresql",
		"Dependencies",
		"PostgreSQL",
		"connected",
		"Redis",
		"skipped",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected startup output to contain %q:\n%s", want, output)
		}
	}
}

// TestSafeURLRedactsCredentials 驗證 safe URL redacts credentials。
func TestSafeURLRedactsCredentials(t *testing.T) {
	got := startup.SafeURL("postgres://nexus:secret@localhost:5432/nexus_pro_be?sslmode=disable&password=secret")
	if strings.Contains(got, "secret") {
		t.Fatalf("expected credentials to be redacted, got %q", got)
	}
	if !strings.Contains(got, "postgres://nexus@localhost:5432/nexus_pro_be") {
		t.Fatalf("expected sanitized postgres URL to keep safe target fields, got %q", got)
	}
	if !strings.Contains(got, "password=redacted") {
		t.Fatalf("expected sensitive query value to be redacted, got %q", got)
	}
}
