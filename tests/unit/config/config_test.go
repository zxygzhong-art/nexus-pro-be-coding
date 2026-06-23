package config_test

import (
	"strings"
	"testing"

	"nexus-pro-be/internal/config"
)

func TestSeedDemoDefaultsToDisabledInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SEED_DEMO", "")

	cfg := config.Load()

	if cfg.SeedDemo {
		t.Fatal("expected production to disable demo seed by default")
	}
}

func TestSeedDemoCanBeEnabledExplicitly(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SEED_DEMO", "true")

	cfg := config.Load()

	if !cfg.SeedDemo {
		t.Fatal("expected explicit SEED_DEMO=true to enable demo seed")
	}
}

func TestLogLevelDefaultsToInfo(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")

	cfg := config.Load()

	if cfg.LogLevel != "info" {
		t.Fatalf("expected default log level info, got %q", cfg.LogLevel)
	}
}

func TestOpenTelemetryConfig(t *testing.T) {
	t.Setenv("OTEL_ENABLED", "true")
	t.Setenv("OTEL_SERVICE_NAME", "nexus-test")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "tempo:4317")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "false")

	cfg := config.Load()

	if !cfg.OTelEnabled {
		t.Fatal("expected OpenTelemetry to be enabled")
	}
	if cfg.OTelServiceName != "nexus-test" {
		t.Fatalf("unexpected service name: %q", cfg.OTelServiceName)
	}
	if cfg.OTelExporterOTLPEndpoint != "tempo:4317" {
		t.Fatalf("unexpected OTLP endpoint: %q", cfg.OTelExporterOTLPEndpoint)
	}
	if cfg.OTelExporterOTLPInsecure {
		t.Fatal("expected insecure flag to be false")
	}
}

func TestObjectStoreDirConfig(t *testing.T) {
	t.Setenv("OBJECT_STORE_DIR", "/tmp/nexus-objects")

	cfg := config.Load()

	if cfg.ObjectStoreDir != "/tmp/nexus-objects" {
		t.Fatalf("unexpected object store dir: %q", cfg.ObjectStoreDir)
	}
}

func TestValidateStartupAllowsDevelopmentDefaults(t *testing.T) {
	cfg := config.Config{Env: "development"}

	if err := cfg.ValidateStartup(); err != nil {
		t.Fatalf("expected development defaults to validate, got %v", err)
	}
}

func TestValidateStartupAcceptsProductionMinimum(t *testing.T) {
	cfg := config.Config{
		Env:               "production",
		DatabaseURL:       "postgres://nexus:nexus@localhost:5432/nexus_pro_be?sslmode=disable",
		KeycloakIssuerURL: "https://issuer.example/realms/nexus",
		KeycloakClientID:  "nexus-api",
		OpenFGAAPIURL:     "https://openfga.example",
		OpenFGAStoreID:    "store-1",
		OpenFGAModelID:    "model-1",
		ObjectStoreDir:    "/var/lib/nexus-pro-be/objects",
	}

	if err := cfg.ValidateStartup(); err != nil {
		t.Fatalf("expected production minimum config to validate, got %v", err)
	}
}

func TestValidateStartupRejectsMissingProductionDependencies(t *testing.T) {
	cfg := config.Config{Env: "production"}

	err := cfg.ValidateStartup()
	if err == nil {
		t.Fatal("expected production config validation error")
	}
	for _, want := range []string{"DATABASE_URL", "KEYCLOAK_ISSUER_URL", "KEYCLOAK_CLIENT_ID", "OPENFGA_API_URL", "OPENFGA_STORE_ID", "OPENFGA_MODEL_ID", "OBJECT_STORE_DIR"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected validation error to mention %s, got %v", want, err)
		}
	}
}

func TestValidateStartupRejectsUnsafeProductionCompatibilityFlags(t *testing.T) {
	cfg := config.Config{
		Env:                "production",
		DatabaseURL:        "postgres://nexus:nexus@localhost:5432/nexus_pro_be?sslmode=disable",
		KeycloakIssuerURL:  "https://issuer.example/realms/nexus",
		KeycloakClientID:   "nexus-api",
		OpenFGAAPIURL:      "https://openfga.example",
		OpenFGAStoreID:     "store-1",
		OpenFGAModelID:     "model-1",
		ObjectStoreDir:     "/var/lib/nexus-pro-be/objects",
		SeedDemo:           true,
		AllowDemoContext:   true,
		AllowHeaderContext: true,
		AllowUnsignedJWT:   true,
	}

	err := cfg.ValidateStartup()
	if err == nil {
		t.Fatal("expected unsafe production config validation error")
	}
	for _, want := range []string{"SEED_DEMO", "ALLOW_DEMO_CONTEXT", "ALLOW_HEADER_CONTEXT", "ALLOW_UNSIGNED_JWT"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected validation error to mention %s, got %v", want, err)
		}
	}
}

func TestInvalidIntegerConfigReturnsError(t *testing.T) {
	t.Setenv("REDIS_DB", "not-a-number")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "REDIS_DB must be an integer") {
		t.Fatalf("expected invalid integer config error, got %v", err)
	}
}

func TestInvalidBooleanConfigReturnsError(t *testing.T) {
	t.Setenv("ALLOW_UNSIGNED_JWT", "maybe")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "ALLOW_UNSIGNED_JWT must be a boolean") {
		t.Fatalf("expected invalid boolean config error, got %v", err)
	}
}
