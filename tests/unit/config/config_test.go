package config_test

import (
	"strings"
	"testing"
	"time"

	"nexus-pro-be/internal/config"
)

// TestLogLevelDefaultsToInfo 驗證 log level defaults to info。
func TestLogLevelDefaultsToInfo(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")

	cfg := config.Load()

	if cfg.LogLevel != "info" {
		t.Fatalf("expected default log level info, got %q", cfg.LogLevel)
	}
}

// TestOpenTelemetryConfig 驗證 open 遙測組態。
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

// TestOpenFGAScopeCheckConfig 驗證 OpenFGA scope check 開關。
func TestOpenFGAScopeCheckConfig(t *testing.T) {
	t.Setenv("OPENFGA_SCOPE_CHECK_ENABLED", "")
	cfg := config.Load()
	if cfg.OpenFGAScopeCheckEnabled {
		t.Fatal("expected OpenFGA scope checks to be disabled by default")
	}

	t.Setenv("OPENFGA_SCOPE_CHECK_ENABLED", "true")
	cfg, err := config.LoadE()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.OpenFGAScopeCheckEnabled {
		t.Fatal("expected OpenFGA scope checks to be enabled")
	}
}

// TestTemporalConfig 驗證 Temporal workflow engine 組態。
func TestTemporalConfig(t *testing.T) {
	cfg := config.Load()
	if cfg.TemporalHostPort != "127.0.0.1:27233" || cfg.TemporalNamespace != "default" || cfg.TemporalTaskQueue != "nexus-workflows" {
		t.Fatalf("unexpected Temporal defaults: %+v", cfg)
	}

	t.Setenv("TEMPORAL_HOST_PORT", "temporal:7233")
	t.Setenv("TEMPORAL_NAMESPACE", "tenant-workflows")
	t.Setenv("TEMPORAL_TASK_QUEUE", "tenant-queue")
	cfg, err := config.LoadE()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TemporalHostPort != "temporal:7233" || cfg.TemporalNamespace != "tenant-workflows" || cfg.TemporalTaskQueue != "tenant-queue" {
		t.Fatalf("unexpected Temporal config: %+v", cfg)
	}
}

// TestTemporalConfigRequiresRequiredFields 驗證 Temporal 必填組態。
func TestTemporalConfigRequiresRequiredFields(t *testing.T) {
	t.Setenv("TEMPORAL_HOST_PORT", "")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "TEMPORAL_HOST_PORT is required") {
		t.Fatalf("expected missing TEMPORAL_HOST_PORT error, got %v", err)
	}
}

// TestNATSConfig 驗證 NATS JetStream 組態。
func TestNATSConfig(t *testing.T) {
	cfg := config.Load()
	if cfg.NATSEnabled {
		t.Fatal("expected NATS to be disabled by default")
	}
	if cfg.NATSURL != "nats://127.0.0.1:24222" || cfg.NATSStream != "NEXUS_EVENTS" || cfg.NATSConsumerPrefix != "nexus" {
		t.Fatalf("unexpected NATS defaults: %+v", cfg)
	}

	t.Setenv("NATS_ENABLED", "true")
	t.Setenv("NATS_URL", "nats://nats:4222")
	t.Setenv("NATS_STREAM", "CUSTOM_EVENTS")
	t.Setenv("NATS_CONSUMER_PREFIX", "custom")
	cfg, err := config.LoadE()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.NATSEnabled || cfg.NATSURL != "nats://nats:4222" || cfg.NATSStream != "CUSTOM_EVENTS" || cfg.NATSConsumerPrefix != "custom" {
		t.Fatalf("unexpected NATS config: %+v", cfg)
	}
}

// TestObjectStoreDirConfig 驗證物件儲存層 dir 組態。
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

// TestSFTPGoObjectStoreConfig 驗證 SFTPGo 物件儲存層組態。
func TestSFTPGoObjectStoreConfig(t *testing.T) {
	t.Setenv("OBJECT_STORE_PROVIDER", "sftpgo")
	t.Setenv("OBJECT_STORE_ENDPOINT", "sftp://localhost:2022")
	t.Setenv("OBJECT_STORE_BUCKET", "nexus-hr-imports")
	t.Setenv("OBJECT_STORE_ACCESS_KEY_ID", "nexus")
	t.Setenv("OBJECT_STORE_SECRET_ACCESS_KEY", "nexus-sftpgo-password")
	t.Setenv("OBJECT_STORE_SFTP_HOST_KEY", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKtftesttesttesttesttesttesttesttesttesttest")
	t.Setenv("OBJECT_STORE_CREATE_BUCKET", "true")

	cfg := config.Load()

	if cfg.ObjectStoreProvider != "sftpgo" || cfg.ObjectStoreEndpoint != "sftp://localhost:2022" || cfg.ObjectStoreBucket != "nexus-hr-imports" {
		t.Fatalf("unexpected sftpgo config: %+v", cfg)
	}
	if cfg.ObjectStoreSFTPHostKey == "" {
		t.Fatal("expected OBJECT_STORE_SFTP_HOST_KEY to be loaded")
	}
	if !cfg.ObjectStoreCreateBucket {
		t.Fatal("expected OBJECT_STORE_CREATE_BUCKET to be true")
	}
}

// TestEHRMSConfig 驗證 eHRMS 組態。
func TestEHRMSConfig(t *testing.T) {
	t.Setenv("EHRMS_BASE_URL", "https://ehrms.example")
	t.Setenv("EHRMS_API_KEY", "test-key")
	t.Setenv("EHRMS_SYNC_ENABLED", "true")
	t.Setenv("EHRMS_SYNC_INTERVAL", "12h")
	t.Setenv("EHRMS_SYNC_MODE", "upsert")
	t.Setenv("EHRMS_SYNC_TENANT_ID", "tenant-1")
	t.Setenv("EHRMS_SYNC_ACCOUNT_ID", "acct-1")
	t.Setenv("EHRMS_SYNC_RUN_ON_START", "true")
	t.Setenv("EHRMS_ATTENDANCE_SYNC_ENABLED", "true")
	t.Setenv("EHRMS_ATTENDANCE_SYNC_INTERVAL", "6h")
	t.Setenv("EHRMS_ATTENDANCE_SYNC_MODE", "upsert")
	t.Setenv("EHRMS_ATTENDANCE_SYNC_TENANT_ID", "tenant-att")
	t.Setenv("EHRMS_ATTENDANCE_SYNC_ACCOUNT_ID", "acct-att")
	t.Setenv("EHRMS_ATTENDANCE_SYNC_RUN_ON_START", "true")
	t.Setenv("EHRMS_ATTENDANCE_SYNC_SINCE", "2026-06-01")

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
	if !cfg.EHRMSAttendanceSyncEnabled || cfg.EHRMSAttendanceSyncInterval != 6*time.Hour || cfg.EHRMSAttendanceSyncMode != "upsert" || cfg.EHRMSAttendanceSyncTenantID != "tenant-att" || cfg.EHRMSAttendanceSyncAccountID != "acct-att" || !cfg.EHRMSAttendanceSyncRunOnStart || cfg.EHRMSAttendanceSyncSince != "2026-06-01" {
		t.Fatalf("unexpected eHRMS attendance sync config: %+v", cfg)
	}
}

// TestEHRMSConfigRequiresURLAndKeyTogether 驗證 eHRMS 組態 requires URL and key together。
func TestEHRMSConfigRequiresURLAndKeyTogether(t *testing.T) {
	t.Setenv("EHRMS_BASE_URL", "https://ehrms.example")
	t.Setenv("EHRMS_API_KEY", "")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "EHRMS_API_KEY is required") {
		t.Fatalf("expected missing EHRMS_API_KEY error, got %v", err)
	}
}

// TestEHRMSSyncConfigRequiresServiceActor 驗證 eHRMS sync 組態 requires 服務 actor。
func TestEHRMSSyncConfigRequiresServiceActor(t *testing.T) {
	t.Setenv("EHRMS_BASE_URL", "https://ehrms.example")
	t.Setenv("EHRMS_API_KEY", "test-key")
	t.Setenv("EHRMS_SYNC_ENABLED", "true")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "EHRMS_SYNC_TENANT_ID is required") || !strings.Contains(err.Error(), "EHRMS_SYNC_ACCOUNT_ID is required") {
		t.Fatalf("expected eHRMS sync actor errors, got %v", err)
	}
}

// TestEHRMSSyncConfigValidatesIntervalAndMode 驗證 eHRMS sync 組態 validates interval and mode。
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

// TestEHRMSAttendanceSyncConfigDefaultsToEmployeeActor 驗證 eHRMS 考勤 sync defaults to employee sync actor。
func TestEHRMSAttendanceSyncConfigDefaultsToEmployeeActor(t *testing.T) {
	t.Setenv("EHRMS_BASE_URL", "https://ehrms.example")
	t.Setenv("EHRMS_API_KEY", "test-key")
	t.Setenv("EHRMS_SYNC_TENANT_ID", "tenant-1")
	t.Setenv("EHRMS_SYNC_ACCOUNT_ID", "acct-1")
	t.Setenv("EHRMS_ATTENDANCE_SYNC_ENABLED", "true")

	cfg, err := config.LoadE()
	if err != nil {
		t.Fatalf("expected eHRMS attendance config to load, got %v", err)
	}
	if cfg.EHRMSAttendanceSyncTenantID != "tenant-1" || cfg.EHRMSAttendanceSyncAccountID != "acct-1" {
		t.Fatalf("expected attendance sync actor to default from employee sync, got %+v", cfg)
	}
}

// TestEHRMSAttendanceSyncConfigValidatesSince 驗證 eHRMS 考勤 sync validates since date。
func TestEHRMSAttendanceSyncConfigValidatesSince(t *testing.T) {
	t.Setenv("EHRMS_BASE_URL", "https://ehrms.example")
	t.Setenv("EHRMS_API_KEY", "test-key")
	t.Setenv("EHRMS_SYNC_TENANT_ID", "tenant-1")
	t.Setenv("EHRMS_SYNC_ACCOUNT_ID", "acct-1")
	t.Setenv("EHRMS_ATTENDANCE_SYNC_ENABLED", "true")
	t.Setenv("EHRMS_ATTENDANCE_SYNC_INTERVAL", "soon")
	t.Setenv("EHRMS_ATTENDANCE_SYNC_MODE", "merge")
	t.Setenv("EHRMS_ATTENDANCE_SYNC_SINCE", "2026/06/01")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "EHRMS_ATTENDANCE_SYNC_INTERVAL must be a positive duration") || !strings.Contains(err.Error(), "EHRMS_ATTENDANCE_SYNC_MODE must be create, update, or upsert") || !strings.Contains(err.Error(), "EHRMS_ATTENDANCE_SYNC_SINCE must be YYYY-MM-DD") {
		t.Fatalf("expected eHRMS attendance sync validation errors, got %v", err)
	}
}

// TestKeycloakProvisioningConfig 驗證 Keycloak 開通組態。
func TestKeycloakProvisioningConfig(t *testing.T) {
	t.Setenv("KEYCLOAK_ISSUER_URL", "https://issuer.example/realms/nexus")
	t.Setenv("KEYCLOAK_CLIENT_ID", "nexus-api")
	t.Setenv("KEYCLOAK_PROVISION_USERS", "true")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_ID", "nexus-admin")
	t.Setenv("KEYCLOAK_ADMIN_CLIENT_SECRET", "secret")
	t.Setenv("KEYCLOAK_SEND_INVITE_EMAIL", "true")
	t.Setenv("KEYCLOAK_INVITE_REDIRECT_URL", "https://app.example/login")

	cfg, err := config.LoadE()
	if err != nil {
		t.Fatalf("expected keycloak provisioning config to load, got %v", err)
	}
	if !cfg.KeycloakProvisionUsers || cfg.KeycloakAdminClientID != "nexus-admin" || !cfg.KeycloakSendInviteEmail || cfg.KeycloakInviteClientID != "nexus-api" {
		t.Fatalf("unexpected keycloak provisioning config: %+v", cfg)
	}
}

// TestKeycloakProvisioningConfigRequiresAdminCredentials 驗證 Keycloak 開通組態 requires 管理員 credentials。
func TestKeycloakProvisioningConfigRequiresAdminCredentials(t *testing.T) {
	t.Setenv("KEYCLOAK_ISSUER_URL", "https://issuer.example/realms/nexus")
	t.Setenv("KEYCLOAK_PROVISION_USERS", "true")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "KEYCLOAK_ADMIN_CLIENT_ID is required") || !strings.Contains(err.Error(), "KEYCLOAK_ADMIN_CLIENT_SECRET is required") {
		t.Fatalf("expected keycloak admin credential errors, got %v", err)
	}
}

// TestValidateStartupAllowsDevelopmentDefaults 驗證 startup allows development defaults。
func TestValidateStartupAllowsDevelopmentDefaults(t *testing.T) {
	cfg := config.Config{Env: "development"}

	if err := cfg.ValidateStartup(); err != nil {
		t.Fatalf("expected development defaults to validate, got %v", err)
	}
}

// TestValidateStartupAcceptsProductionMinimum 驗證 startup accepts production minimum。
func TestValidateStartupAcceptsProductionMinimum(t *testing.T) {
	cfg := config.Config{
		Env:               "production",
		DatabaseURL:       "postgres://nexus:nexus@localhost:5432/nexus_pro_be?sslmode=require",
		KeycloakIssuerURL: "https://issuer.example/realms/nexus",
		KeycloakClientID:  "nexus-api",
		OpenFGAAPIURL:     "https://openfga.example",
		OpenFGAStoreID:    "store-1",
		OpenFGAModelID:    "model-1",
		TemporalHostPort:  "temporal:7233",
		TemporalNamespace: "default",
		TemporalTaskQueue: "nexus-workflows",
		ObjectStoreDir:    "/var/lib/nexus-pro-be/objects",
	}

	if err := cfg.ValidateStartup(); err != nil {
		t.Fatalf("expected production minimum config to validate, got %v", err)
	}
}

// TestValidateStartupRejectsCleartextProductionSecurityURLs 驗證 startup rejects cleartext production security URLs。
func TestValidateStartupRejectsCleartextProductionSecurityURLs(t *testing.T) {
	cfg := config.Config{
		Env:               "production",
		DatabaseURL:       "postgres://nexus:nexus@localhost:5432/nexus_pro_be?sslmode=require",
		KeycloakIssuerURL: "http://issuer.example/realms/nexus",
		KeycloakClientID:  "nexus-api",
		OpenFGAAPIURL:     "http://openfga.example",
		OpenFGAStoreID:    "store-1",
		OpenFGAModelID:    "model-1",
		ObjectStoreDir:    "/var/lib/nexus-pro-be/objects",
	}

	err := cfg.ValidateStartup()
	if err == nil {
		t.Fatal("expected cleartext production security URL validation error")
	}
	for _, want := range []string{"KEYCLOAK_ISSUER_URL", "OPENFGA_API_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected validation error to mention %s, got %v", want, err)
		}
	}
}

// TestValidateStartupRejectsMissingProductionDependencies 驗證 startup rejects missing production 依賴。
func TestValidateStartupRejectsMissingProductionDependencies(t *testing.T) {
	cfg := config.Config{Env: "production"}

	err := cfg.ValidateStartup()
	if err == nil {
		t.Fatal("expected production config validation error")
	}
	for _, want := range []string{"DATABASE_URL", "KEYCLOAK_ISSUER_URL", "KEYCLOAK_CLIENT_ID", "OPENFGA_API_URL", "OPENFGA_STORE_ID", "OPENFGA_MODEL_ID", "TEMPORAL_HOST_PORT", "TEMPORAL_NAMESPACE", "TEMPORAL_TASK_QUEUE", "OBJECT_STORE_PROVIDER"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected validation error to mention %s, got %v", want, err)
		}
	}
}

// TestValidateStartupRejectsProductionDatabaseWithoutSSL 驗證 startup rejects production database without ssl。
func TestValidateStartupRejectsProductionDatabaseWithoutSSL(t *testing.T) {
	base := config.Config{
		Env:               "production",
		KeycloakIssuerURL: "https://issuer.example/realms/nexus",
		KeycloakClientID:  "nexus-api",
		OpenFGAAPIURL:     "https://openfga.example",
		OpenFGAStoreID:    "store-1",
		OpenFGAModelID:    "model-1",
		TemporalHostPort:  "temporal:7233",
		TemporalNamespace: "default",
		TemporalTaskQueue: "nexus-workflows",
		ObjectStoreDir:    "/var/lib/nexus-pro-be/objects",
	}
	for name, databaseURL := range map[string]string{
		"sslmode disable": "postgres://nexus:nexus@db.internal:5432/nexus_pro_be?sslmode=disable",
		"sslmode missing": "postgres://nexus:nexus@db.internal:5432/nexus_pro_be",
		"sslmode prefer":  "postgres://nexus:nexus@db.internal:5432/nexus_pro_be?sslmode=prefer",
		"sslmode allow":   "postgres://nexus:nexus@db.internal:5432/nexus_pro_be?sslmode=allow",
	} {
		cfg := base
		cfg.DatabaseURL = databaseURL
		err := cfg.ValidateStartup()
		if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") || !strings.Contains(err.Error(), "sslmode") {
			t.Fatalf("%s: expected production DATABASE_URL sslmode error, got %v", name, err)
		}
	}
	for _, sslmode := range []string{"require", "verify-ca", "verify-full"} {
		cfg := base
		cfg.DatabaseURL = "postgres://nexus:nexus@db.internal:5432/nexus_pro_be?sslmode=" + sslmode
		if err := cfg.ValidateStartup(); err != nil {
			t.Fatalf("expected sslmode=%s to validate in production, got %v", sslmode, err)
		}
	}
}

// TestValidateStartupRejectsProductionTemporalWithoutHost 驗證 production Temporal host 校驗。
func TestValidateStartupRejectsProductionTemporalWithoutHost(t *testing.T) {
	cfg := config.Config{
		Env:               "production",
		DatabaseURL:       "postgres://nexus:nexus@db.internal:5432/nexus_pro_be?sslmode=require",
		KeycloakIssuerURL: "https://issuer.example/realms/nexus",
		KeycloakClientID:  "nexus-api",
		OpenFGAAPIURL:     "https://openfga.example",
		OpenFGAStoreID:    "store-1",
		OpenFGAModelID:    "model-1",
		ObjectStoreDir:    "/var/lib/nexus-pro-be/objects",
		TemporalNamespace: "default",
		TemporalTaskQueue: "nexus-workflows",
	}

	err := cfg.ValidateStartup()
	if err == nil || !strings.Contains(err.Error(), "TEMPORAL_HOST_PORT") {
		t.Fatalf("expected production Temporal host validation error, got %v", err)
	}
}

// TestValidateStartupRejectsProductionNATSInvalidURL 驗證 production NATS URL 校驗。
func TestValidateStartupRejectsProductionNATSInvalidURL(t *testing.T) {
	cfg := config.Config{
		Env:               "production",
		DatabaseURL:       "postgres://nexus:nexus@db.internal:5432/nexus_pro_be?sslmode=require",
		KeycloakIssuerURL: "https://issuer.example/realms/nexus",
		KeycloakClientID:  "nexus-api",
		OpenFGAAPIURL:     "https://openfga.example",
		OpenFGAStoreID:    "store-1",
		OpenFGAModelID:    "model-1",
		TemporalHostPort:  "temporal:7233",
		TemporalNamespace: "default",
		TemporalTaskQueue: "nexus-workflows",
		ObjectStoreDir:    "/var/lib/nexus-pro-be/objects",
		NATSEnabled:       true,
		NATSURL:           "http://nats:4222",
	}

	err := cfg.ValidateStartup()
	if err == nil || !strings.Contains(err.Error(), "NATS_URL") {
		t.Fatalf("expected production NATS URL validation error, got %v", err)
	}
}

// TestDatabasePoolConfigDefaultsAndOverrides 驗證 database pool 組態 defaults and overrides。
func TestDatabasePoolConfigDefaultsAndOverrides(t *testing.T) {
	cfg := config.Load()
	if cfg.DBMaxConns != 10 || cfg.DBMinConns != 1 || cfg.DBMaxConnLifetime != time.Hour {
		t.Fatalf("unexpected pool defaults: %+v", cfg)
	}

	t.Setenv("DB_MAX_CONNS", "25")
	t.Setenv("DB_MIN_CONNS", "5")
	t.Setenv("DB_MAX_CONN_LIFETIME", "30m")

	cfg, err := config.LoadE()
	if err != nil {
		t.Fatalf("expected pool config to load, got %v", err)
	}
	if cfg.DBMaxConns != 25 || cfg.DBMinConns != 5 || cfg.DBMaxConnLifetime != 30*time.Minute {
		t.Fatalf("unexpected pool config: %+v", cfg)
	}
}

// TestDatabasePoolConfigRejectsInvalidSizes 驗證 database pool 組態 rejects 無效 sizes。
func TestDatabasePoolConfigRejectsInvalidSizes(t *testing.T) {
	t.Setenv("DB_MAX_CONNS", "2")
	t.Setenv("DB_MIN_CONNS", "5")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "DB_MIN_CONNS must not exceed DB_MAX_CONNS") {
		t.Fatalf("expected pool size validation error, got %v", err)
	}
}

// TestCORSAllowedOriginsConfig 驗證 CORS allowed origins 組態。
func TestCORSAllowedOriginsConfig(t *testing.T) {
	cfg := config.Load()
	if len(cfg.CORSAllowedOrigins) != 0 {
		t.Fatalf("expected CORS to be disabled by default, got %+v", cfg.CORSAllowedOrigins)
	}

	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com, https://admin.example.com ,")

	cfg = config.Load()
	if len(cfg.CORSAllowedOrigins) != 2 || cfg.CORSAllowedOrigins[0] != "https://app.example.com" || cfg.CORSAllowedOrigins[1] != "https://admin.example.com" {
		t.Fatalf("unexpected CORS origins: %+v", cfg.CORSAllowedOrigins)
	}
}

// TestTrustedProxiesConfig 驗證 trusted proxies 組態。
func TestTrustedProxiesConfig(t *testing.T) {
	cfg := config.Load()
	if len(cfg.TrustedProxies) != 0 {
		t.Fatalf("expected no trusted proxies by default, got %+v", cfg.TrustedProxies)
	}

	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8, 192.168.1.1 ,")

	cfg, err := config.LoadE()
	if err != nil {
		t.Fatalf("expected trusted proxies config to load, got %v", err)
	}
	if len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[0] != "10.0.0.0/8" || cfg.TrustedProxies[1] != "192.168.1.1" {
		t.Fatalf("unexpected trusted proxies: %+v", cfg.TrustedProxies)
	}
}

// TestTrustedProxiesConfigRejectsInvalidEntries 驗證 trusted proxies 組態 rejects 無效 entries。
func TestTrustedProxiesConfigRejectsInvalidEntries(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8, proxy.internal")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXIES entry must be an IP or CIDR") {
		t.Fatalf("expected trusted proxies validation error, got %v", err)
	}
}

// TestMetricsAddrConfig 驗證指標 addr 組態。
func TestMetricsAddrConfig(t *testing.T) {
	cfg := config.Load()
	if cfg.MetricsAddr != "127.0.0.1:9091" {
		t.Fatalf("expected default metrics address, got %q", cfg.MetricsAddr)
	}

	t.Setenv("METRICS_ADDR", "0.0.0.0:9200")
	cfg = config.Load()
	if cfg.MetricsAddr != "0.0.0.0:9200" {
		t.Fatalf("unexpected metrics address: %q", cfg.MetricsAddr)
	}

	// 明確設定空值會停用 metrics listener。
	t.Setenv("METRICS_ADDR", "")
	cfg = config.Load()
	if cfg.MetricsAddr != "" {
		t.Fatalf("expected empty METRICS_ADDR to disable the listener, got %q", cfg.MetricsAddr)
	}
}

// TestRateLimitConfig 驗證速率限制組態。
func TestRateLimitConfig(t *testing.T) {
	cfg := config.Load()
	if cfg.RateLimitEnabled {
		t.Fatal("expected rate limiting to be disabled by default")
	}

	t.Setenv("RATE_LIMIT_ENABLED", "true")
	t.Setenv("RATE_LIMIT_RPS", "50")
	t.Setenv("RATE_LIMIT_BURST", "100")

	cfg, err := config.LoadE()
	if err != nil {
		t.Fatalf("expected rate limit config to load, got %v", err)
	}
	if !cfg.RateLimitEnabled || cfg.RateLimitRPS != 50 || cfg.RateLimitBurst != 100 {
		t.Fatalf("unexpected rate limit config: %+v", cfg)
	}
}

// TestRateLimitConfigRejectsInvalidLimits 驗證速率限制組態 rejects 無效 limits。
func TestRateLimitConfigRejectsInvalidLimits(t *testing.T) {
	t.Setenv("RATE_LIMIT_ENABLED", "true")
	t.Setenv("RATE_LIMIT_RPS", "0")
	t.Setenv("RATE_LIMIT_BURST", "-1")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "RATE_LIMIT_RPS must be at least 1") || !strings.Contains(err.Error(), "RATE_LIMIT_BURST must be at least 1") {
		t.Fatalf("expected rate limit validation errors, got %v", err)
	}
}

// TestInvalidIntegerConfigReturnsError 驗證無效 integer 組態 returns 錯誤。
func TestInvalidIntegerConfigReturnsError(t *testing.T) {
	t.Setenv("REDIS_DB", "not-a-number")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "REDIS_DB must be an integer") {
		t.Fatalf("expected invalid integer config error, got %v", err)
	}
}

// TestInvalidBooleanConfigReturnsError 驗證無效 boolean 組態 returns 錯誤。
func TestInvalidBooleanConfigReturnsError(t *testing.T) {
	t.Setenv("EHRMS_SYNC_ENABLED", "maybe")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "EHRMS_SYNC_ENABLED must be a boolean") {
		t.Fatalf("expected invalid boolean config error, got %v", err)
	}
}
