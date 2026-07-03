package config_test

import (
	"strings"
	"testing"
	"time"

	"nexus-pro-be/internal/config"
)

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

	if cfg.ObjectStoreProvider != "local" {
		t.Fatalf("unexpected object store provider: %q", cfg.ObjectStoreProvider)
	}
	if cfg.ObjectStoreDir != "/tmp/nexus-objects" {
		t.Fatalf("unexpected object store dir: %q", cfg.ObjectStoreDir)
	}
}

func TestMinIOObjectStoreConfig(t *testing.T) {
	t.Setenv("OBJECT_STORE_PROVIDER", "minio")
	t.Setenv("OBJECT_STORE_ENDPOINT", "http://localhost:9000")
	t.Setenv("OBJECT_STORE_BUCKET", "nexus-hr-imports")
	t.Setenv("OBJECT_STORE_ACCESS_KEY_ID", "minioadmin")
	t.Setenv("OBJECT_STORE_SECRET_ACCESS_KEY", "minioadmin")
	t.Setenv("OBJECT_STORE_CREATE_BUCKET", "true")

	cfg := config.Load()

	if cfg.ObjectStoreProvider != "minio" || cfg.ObjectStoreEndpoint != "http://localhost:9000" || cfg.ObjectStoreBucket != "nexus-hr-imports" {
		t.Fatalf("unexpected minio config: %+v", cfg)
	}
	if !cfg.ObjectStoreCreateBucket {
		t.Fatal("expected OBJECT_STORE_CREATE_BUCKET to be true")
	}
}

func TestEHRMSConfig(t *testing.T) {
	t.Setenv("EHRMS_BASE_URL", "https://ehrms.example")
	t.Setenv("EHRMS_API_KEY", "test-key")
	t.Setenv("EHRMS_SYNC_ENABLED", "true")
	t.Setenv("EHRMS_SYNC_INTERVAL", "12h")
	t.Setenv("EHRMS_SYNC_MODE", "upsert")
	t.Setenv("EHRMS_SYNC_TENANT_ID", "tenant-1")
	t.Setenv("EHRMS_SYNC_ACCOUNT_ID", "acct-1")
	t.Setenv("EHRMS_SYNC_RUN_ON_START", "true")

	cfg, err := config.LoadE()
	if err != nil {
		t.Fatalf("expected eHRMS config to load, got %v", err)
	}
	if cfg.EHRMSBaseURL != "https://ehrms.example" || cfg.EHRMSAPIKey != "test-key" {
		t.Fatalf("unexpected eHRMS config: %+v", cfg)
	}
	if !cfg.EHRMSSyncEnabled || cfg.EHRMSSyncInterval != 12*time.Hour || cfg.EHRMSSyncMode != "upsert" || cfg.EHRMSSyncTenantID != "tenant-1" || cfg.EHRMSSyncAccountID != "acct-1" || !cfg.EHRMSSyncRunOnStart {
		t.Fatalf("unexpected eHRMS sync config: %+v", cfg)
	}
}

func TestEHRMSConfigRequiresURLAndKeyTogether(t *testing.T) {
	t.Setenv("EHRMS_BASE_URL", "https://ehrms.example")
	t.Setenv("EHRMS_API_KEY", "")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "EHRMS_API_KEY is required") {
		t.Fatalf("expected missing EHRMS_API_KEY error, got %v", err)
	}
}

func TestEHRMSSyncConfigRequiresServiceActor(t *testing.T) {
	t.Setenv("EHRMS_BASE_URL", "https://ehrms.example")
	t.Setenv("EHRMS_API_KEY", "test-key")
	t.Setenv("EHRMS_SYNC_ENABLED", "true")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "EHRMS_SYNC_TENANT_ID is required") || !strings.Contains(err.Error(), "EHRMS_SYNC_ACCOUNT_ID is required") {
		t.Fatalf("expected eHRMS sync actor errors, got %v", err)
	}
}

func TestEHRMSSyncConfigValidatesIntervalAndMode(t *testing.T) {
	t.Setenv("EHRMS_BASE_URL", "https://ehrms.example")
	t.Setenv("EHRMS_API_KEY", "test-key")
	t.Setenv("EHRMS_SYNC_ENABLED", "true")
	t.Setenv("EHRMS_SYNC_INTERVAL", "soon")
	t.Setenv("EHRMS_SYNC_MODE", "merge")
	t.Setenv("EHRMS_SYNC_TENANT_ID", "tenant-1")
	t.Setenv("EHRMS_SYNC_ACCOUNT_ID", "acct-1")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "EHRMS_SYNC_INTERVAL must be a positive duration") || !strings.Contains(err.Error(), "EHRMS_SYNC_MODE must be create, update, or upsert") {
		t.Fatalf("expected eHRMS sync interval/mode errors, got %v", err)
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

func TestValidateStartupRejectsCleartextProductionSecurityURLs(t *testing.T) {
	cfg := config.Config{
		Env:                       "production",
		DatabaseURL:               "postgres://nexus:nexus@localhost:5432/nexus_pro_be?sslmode=disable",
		KeycloakIssuerURL:         "http://issuer.example/realms/nexus",
		KeycloakClientID:          "nexus-api",
		OpenFGAAPIURL:             "http://openfga.example",
		OpenFGAStoreID:            "store-1",
		OpenFGAModelID:            "model-1",
		ObjectStoreDir:            "/var/lib/nexus-pro-be/objects",
		AuthSessionSigningKey:     "session-secret",
		GoogleOIDCIssuerURL:       "http://accounts.example",
		GoogleOIDCClientID:        "google-client",
		GoogleOIDCClientSecret:    "google-secret",
		GoogleOIDCRedirectURL:     "https://api.example/v1/auth/oidc/google/callback",
		MicrosoftOIDCIssuerURL:    "http://login.example/common/v2.0",
		MicrosoftOIDCClientID:     "microsoft-client",
		MicrosoftOIDCClientSecret: "microsoft-secret",
		MicrosoftOIDCRedirectURL:  "https://api.example/v1/auth/oidc/microsoft/callback",
	}

	err := cfg.ValidateStartup()
	if err == nil {
		t.Fatal("expected cleartext production security URL validation error")
	}
	for _, want := range []string{"KEYCLOAK_ISSUER_URL", "OPENFGA_API_URL", "GOOGLE_OIDC_ISSUER_URL", "MICROSOFT_OIDC_ISSUER_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected validation error to mention %s, got %v", want, err)
		}
	}
}

func TestValidateStartupRejectsMissingProductionDependencies(t *testing.T) {
	cfg := config.Config{Env: "production"}

	err := cfg.ValidateStartup()
	if err == nil {
		t.Fatal("expected production config validation error")
	}
	for _, want := range []string{"DATABASE_URL", "KEYCLOAK_ISSUER_URL", "KEYCLOAK_CLIENT_ID", "OPENFGA_API_URL", "OPENFGA_STORE_ID", "OPENFGA_MODEL_ID", "OBJECT_STORE_PROVIDER"} {
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
		AllowDemoContext:   true,
		AllowHeaderContext: true,
		AllowUnsignedJWT:   true,
	}

	err := cfg.ValidateStartup()
	if err == nil {
		t.Fatal("expected unsafe production config validation error")
	}
	for _, want := range []string{"ALLOW_DEMO_CONTEXT", "ALLOW_HEADER_CONTEXT", "ALLOW_UNSIGNED_JWT"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected validation error to mention %s, got %v", want, err)
		}
	}
}

func TestValidateStartupRejectsIncompleteOIDCProvider(t *testing.T) {
	cfg := config.Config{
		Env:                   "production",
		DatabaseURL:           "postgres://nexus:nexus@localhost:5432/nexus_pro_be?sslmode=disable",
		KeycloakIssuerURL:     "https://issuer.example/realms/nexus",
		KeycloakClientID:      "nexus-api",
		OpenFGAAPIURL:         "https://openfga.example",
		OpenFGAStoreID:        "store-1",
		OpenFGAModelID:        "model-1",
		ObjectStoreDir:        "/var/lib/nexus-pro-be/objects",
		GoogleOIDCIssuerURL:   "https://accounts.google.com",
		GoogleOIDCClientID:    "google-client",
		AuthSessionSigningKey: "session-secret",
	}

	err := cfg.ValidateStartup()
	if err == nil {
		t.Fatal("expected incomplete OIDC config validation error")
	}
	for _, want := range []string{"GOOGLE_OIDC_CLIENT_SECRET", "GOOGLE_OIDC_REDIRECT_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected validation error to mention %s, got %v", want, err)
		}
	}
}

func TestValidateStartupRejectsOIDCWithoutSessionSigningKey(t *testing.T) {
	cfg := config.Config{
		Env:                       "production",
		DatabaseURL:               "postgres://nexus:nexus@localhost:5432/nexus_pro_be?sslmode=disable",
		KeycloakIssuerURL:         "https://issuer.example/realms/nexus",
		KeycloakClientID:          "nexus-api",
		OpenFGAAPIURL:             "https://openfga.example",
		OpenFGAStoreID:            "store-1",
		OpenFGAModelID:            "model-1",
		ObjectStoreDir:            "/var/lib/nexus-pro-be/objects",
		MicrosoftOIDCIssuerURL:    "https://login.microsoftonline.com/common/v2.0",
		MicrosoftOIDCClientID:     "microsoft-client",
		MicrosoftOIDCClientSecret: "microsoft-secret",
		MicrosoftOIDCRedirectURL:  "https://api.example/v1/auth/oidc/microsoft/callback",
	}

	err := cfg.ValidateStartup()
	if err == nil || !strings.Contains(err.Error(), "AUTH_SESSION_SIGNING_KEY") {
		t.Fatalf("expected missing session signing key validation error, got %v", err)
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
