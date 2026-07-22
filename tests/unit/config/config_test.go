package config_test

import (
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/config"
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
	t.Setenv("OTEL_BASE_URL", "tempo:4317")
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

// TestEncryptionKeyConfig verifies the shared encryption key is loaded without transformation.
func TestEncryptionKeyConfig(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "ZmFrZS1rZXk=")
	cfg := config.Load()
	if cfg.EncryptionKey != "ZmFrZS1rZXk=" {
		t.Fatalf("unexpected encryption key: %q", cfg.EncryptionKey)
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

// TestLiteLLMConfig 驗證 LiteLLM 組態。
func TestLiteLLMConfig(t *testing.T) {
	t.Setenv("LITELLM_BASE_URL", "")
	t.Setenv("LITELLM_API_KEY", "")

	cfg, err := config.LoadE()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LiteLLMBaseURL != "http://127.0.0.1:4000" {
		t.Fatalf("unexpected LiteLLM default base URL: %q", cfg.LiteLLMBaseURL)
	}

	t.Setenv("LITELLM_BASE_URL", "http://litellm:4000")
	t.Setenv("LITELLM_API_KEY", "test-key")

	cfg, err = config.LoadE()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LiteLLMBaseURL != "http://litellm:4000" || cfg.LiteLLMAPIKey != "test-key" {
		t.Fatalf("unexpected LiteLLM config: %+v", cfg)
	}
}

// TestTemporalConfig 驗證 Temporal workflow engine 組態。
func TestTemporalConfig(t *testing.T) {
	t.Setenv("WORKFLOW_START_OUTBOX_ENABLED", "")
	t.Setenv("OUTBOX_DISPATCH_ENABLED", "")
	cfg := config.Load()
	if cfg.TemporalBaseURL != "127.0.0.1:27233" || cfg.TemporalNamespace != "default" || cfg.TemporalTaskQueue != "nexus-workflows" {
		t.Fatalf("unexpected Temporal defaults: %+v", cfg)
	}
	if cfg.WorkflowStartOutboxEnabled || !cfg.OutboxDispatchEnabled {
		t.Fatalf("unexpected workflow delivery defaults: %+v", cfg)
	}

	t.Setenv("TEMPORAL_BASE_URL", "temporal:7233")
	t.Setenv("TEMPORAL_NAMESPACE", "tenant-workflows")
	t.Setenv("TEMPORAL_TASK_QUEUE", "tenant-queue")
	t.Setenv("WORKFLOW_START_OUTBOX_ENABLED", "true")
	t.Setenv("OUTBOX_DISPATCH_ENABLED", "false")
	cfg, err := config.LoadE()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TemporalBaseURL != "temporal:7233" || cfg.TemporalNamespace != "tenant-workflows" || cfg.TemporalTaskQueue != "tenant-queue" {
		t.Fatalf("unexpected Temporal config: %+v", cfg)
	}
	if !cfg.WorkflowStartOutboxEnabled || cfg.OutboxDispatchEnabled {
		t.Fatalf("unexpected workflow delivery config: %+v", cfg)
	}
}

// TestTemporalConfigRequiresRequiredFields 驗證 Temporal 必填組態。
func TestTemporalConfigRequiresRequiredFields(t *testing.T) {
	t.Setenv("TEMPORAL_BASE_URL", "")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "TEMPORAL_BASE_URL is required") {
		t.Fatalf("expected missing TEMPORAL_BASE_URL error, got %v", err)
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
	t.Setenv("NATS_BASE_URL", "nats://nats:4222")
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

// TestSFTPGoObjectStoreConfig 驗證 SFTPGo HTTP/HTTPS 物件儲存層組態。
func TestSFTPGoObjectStoreConfig(t *testing.T) {
	t.Setenv("OBJECT_STORE_PROVIDER", "sftpgo")
	t.Setenv("SFTPGO_BASE_URL", "https://sftpgo.example.com")
	t.Setenv("SFTPGO_ROOT_BUCKET", "nexus-bucket")
	t.Setenv("SFTPGO_USERNAME", "nexus-service")
	t.Setenv("SFTPGO_PASSWORD", "nexus-service")
	t.Setenv("OBJECT_STORE_CREATE_BUCKET", "true")

	cfg := config.Load()

	if cfg.ObjectStoreProvider != "sftpgo" || cfg.ObjectStoreEndpoint != "https://sftpgo.example.com" || cfg.ObjectStoreBucket != "nexus-bucket" {
		t.Fatalf("unexpected sftpgo config: %+v", cfg)
	}
	if cfg.ObjectStoreAccessKeyID != "nexus-service" || cfg.ObjectStoreSecretAccessKey != "nexus-service" {
		t.Fatalf("unexpected sftpgo credentials: user=%q pass=%q", cfg.ObjectStoreAccessKeyID, cfg.ObjectStoreSecretAccessKey)
	}
	if !cfg.ObjectStoreCreateBucket {
		t.Fatal("expected OBJECT_STORE_CREATE_BUCKET to be true")
	}
}

// TestEHRMSConfig 驗證 eHRMS 組態。
func TestEHRMSConfig(t *testing.T) {
	t.Setenv("EHRMS_BASE_URL", "https://ehrms.example")
	t.Setenv("EHRMS_API_KEY", "test-key")
	t.Setenv("EHRMS_REQUEST_INTERVAL", "750ms")
	t.Setenv("EHRMS_SYNC_ENABLED", "true")
	t.Setenv("EHRMS_SYNC_MODE", "upsert")
	t.Setenv("EHRMS_SYNC_TENANT_ID", "tenant-1")
	t.Setenv("EHRMS_SYNC_ACCOUNT_ID", "acct-1")
	cfg, err := config.LoadE()
	if err != nil {
		t.Fatalf("expected eHRMS config to load, got %v", err)
	}
	if cfg.EHRMSBaseURL != "https://ehrms.example" || cfg.EHRMSAPIKey != "test-key" || cfg.EHRMSRequestInterval != 750*time.Millisecond {
		t.Fatalf("unexpected eHRMS config: %+v", cfg)
	}
	if !cfg.EHRMSSyncEnabled || cfg.EHRMSSyncMode != "upsert" || cfg.EHRMSSyncTenantID != "tenant-1" || cfg.EHRMSSyncAccountID != "acct-1" {
		t.Fatalf("unexpected eHRMS sync config: %+v", cfg)
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

// TestEHRMSSyncConfigValidatesMode validates the unified eHRMS sync mode.
func TestEHRMSSyncConfigValidatesMode(t *testing.T) {
	t.Setenv("EHRMS_BASE_URL", "https://ehrms.example")
	t.Setenv("EHRMS_API_KEY", "test-key")
	t.Setenv("EHRMS_SYNC_ENABLED", "true")
	t.Setenv("EHRMS_SYNC_MODE", "merge")
	t.Setenv("EHRMS_SYNC_TENANT_ID", "tenant-1")
	t.Setenv("EHRMS_SYNC_ACCOUNT_ID", "acct-1")

	_, err := config.LoadE()
	if err == nil || !strings.Contains(err.Error(), "EHRMS_SYNC_MODE must be create, update, or upsert") {
		t.Fatalf("expected eHRMS sync mode error, got %v", err)
	}
}

// TestKeycloakProvisioningConfig 驗證 Keycloak 開通組態。
func TestKeycloakProvisioningConfig(t *testing.T) {
	t.Setenv("KEYCLOAK_BASE_URL", "https://issuer.example/realms/nexus")
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
	t.Setenv("KEYCLOAK_BASE_URL", "https://issuer.example/realms/nexus")
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

// TestValidateStartupRejectsUnknownEnvironment prevents APP_ENV typos from bypassing production checks.
func TestValidateStartupRejectsUnknownEnvironment(t *testing.T) {
	cfg := config.Config{Env: "prodution"}

	err := cfg.ValidateStartup()
	if err == nil || !strings.Contains(err.Error(), "APP_ENV") {
		t.Fatalf("expected unknown APP_ENV to fail startup validation, got %v", err)
	}
}

// TestValidateStartupAppliesProductionChecksToStaging keeps pre-production behavior aligned with production.
func TestValidateStartupAppliesProductionChecksToStaging(t *testing.T) {
	cfg := config.Config{Env: "staging"}

	err := cfg.ValidateStartup()
	if err == nil || !strings.Contains(err.Error(), "DB_HOST, DB_USERNAME, and DB_NAME") {
		t.Fatalf("expected staging to use production startup checks, got %v", err)
	}
}

// TestValidateStartupAcceptsProductionMinimum 驗證 startup accepts production minimum。
func TestValidateStartupAcceptsProductionMinimum(t *testing.T) {
	cfg := config.Config{
		Env:               "production",
		DatabaseURL:       "postgres://nexus:nexus@localhost:5432/nexus_pro_be?sslmode=require",
		DBSSLMode:         "require",
		KeycloakIssuerURL: "https://issuer.example/realms/nexus",
		KeycloakClientID:  "nexus-api",
		OpenFGAAPIURL:     "https://openfga.example",
		OpenFGAStoreID:    "store-1",
		OpenFGAModelID:    "model-1",
		TemporalBaseURL:   "temporal:7233",
		TemporalNamespace: "default",
		TemporalTaskQueue: "nexus-workflows",
		ObjectStoreDir:    "/var/lib/nexus-pro-api/objects",
	}

	if err := cfg.ValidateStartup(); err != nil {
		t.Fatalf("expected production minimum config to validate, got %v", err)
	}
}

// TestValidateStartupRejectsProductionSFTPGoWithoutHostKey 驗證 production sftp:// 必須提供 host key。
func TestValidateStartupRejectsProductionSFTPGoWithoutHostKey(t *testing.T) {
	cfg := config.Config{
		Env:                        "production",
		DatabaseURL:                "postgres://nexus:nexus@localhost:5432/nexus_pro_be?sslmode=require",
		DBSSLMode:                  "require",
		KeycloakIssuerURL:          "https://issuer.example/realms/nexus",
		KeycloakClientID:           "nexus-api",
		OpenFGAAPIURL:              "https://openfga.example",
		OpenFGAStoreID:             "store-1",
		OpenFGAModelID:             "model-1",
		TemporalBaseURL:            "temporal:7233",
		TemporalNamespace:          "default",
		TemporalTaskQueue:          "nexus-workflows",
		ObjectStoreProvider:        "sftpgo",
		ObjectStoreEndpoint:        "sftp://sftp.example:22",
		ObjectStoreBucket:          "nexus-bucket",
		ObjectStoreAccessKeyID:     "nexus-service",
		ObjectStoreSecretAccessKey: "secret",
	}
	err := cfg.ValidateStartup()
	if err == nil {
		t.Fatal("expected missing SFTP host key to fail production validation")
	}
	if !strings.Contains(err.Error(), "OBJECT_STORE_SFTP_HOST_KEY") {
		t.Fatalf("expected host key error, got %v", err)
	}
}

// TestValidateStartupRejectsProductionSFTPGoHTTPWithoutTLS 驗證 production HTTP SFTPGo 必須使用 https。
func TestValidateStartupRejectsProductionSFTPGoHTTPWithoutTLS(t *testing.T) {
	cfg := config.Config{
		Env:                        "production",
		DatabaseURL:                "postgres://nexus:nexus@localhost:5432/nexus_pro_be?sslmode=require",
		DBSSLMode:                  "require",
		KeycloakIssuerURL:          "https://issuer.example/realms/nexus",
		KeycloakClientID:           "nexus-api",
		OpenFGAAPIURL:              "https://openfga.example",
		OpenFGAStoreID:             "store-1",
		OpenFGAModelID:             "model-1",
		TemporalBaseURL:            "temporal:7233",
		TemporalNamespace:          "default",
		TemporalTaskQueue:          "nexus-workflows",
		ObjectStoreProvider:        "sftpgo",
		ObjectStoreEndpoint:        "http://sftpgo:8080",
		ObjectStoreBucket:          "nexus-bucket",
		ObjectStoreAccessKeyID:     "nexus-service",
		ObjectStoreSecretAccessKey: "secret",
	}
	err := cfg.ValidateStartup()
	if err == nil {
		t.Fatal("expected cleartext SFTPGO_BASE_URL to fail production validation")
	}
	if !strings.Contains(err.Error(), "SFTPGO_BASE_URL") {
		t.Fatalf("expected SFTPGO_BASE_URL https error, got %v", err)
	}
}

// TestValidateStartupRejectsProductionSFTPInsecureSkip 驗證 production 不可跳過 host key。
func TestValidateStartupRejectsProductionSFTPInsecureSkip(t *testing.T) {
	cfg := config.Config{
		Env:                                "production",
		DatabaseURL:                        "postgres://nexus:nexus@localhost:5432/nexus_pro_be?sslmode=require",
		DBSSLMode:                          "require",
		KeycloakIssuerURL:                  "https://issuer.example/realms/nexus",
		KeycloakClientID:                   "nexus-api",
		OpenFGAAPIURL:                      "https://openfga.example",
		OpenFGAStoreID:                     "store-1",
		OpenFGAModelID:                     "model-1",
		TemporalBaseURL:                    "temporal:7233",
		TemporalNamespace:                  "default",
		TemporalTaskQueue:                  "nexus-workflows",
		ObjectStoreProvider:                "sftpgo",
		ObjectStoreEndpoint:                "sftp://sftp.example:22",
		ObjectStoreBucket:                  "nexus-bucket",
		ObjectStoreAccessKeyID:             "nexus-service",
		ObjectStoreSecretAccessKey:         "secret",
		ObjectStoreSFTPHostKey:             "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKtftesttesttesttesttesttesttesttesttesttest",
		ObjectStoreSFTPInsecureSkipHostKey: true,
	}
	err := cfg.ValidateStartup()
	if err == nil {
		t.Fatal("expected insecure skip host key to fail production validation")
	}
}

// TestValidateStartupRejectsCleartextProductionSecurityURLs 驗證 startup rejects cleartext production security URLs。
func TestValidateStartupRejectsCleartextProductionSecurityURLs(t *testing.T) {
	cfg := config.Config{
		Env:               "production",
		DatabaseURL:       "postgres://nexus:nexus@localhost:5432/nexus_pro_be?sslmode=require",
		DBSSLMode:         "require",
		KeycloakIssuerURL: "http://issuer.example/realms/nexus",
		KeycloakClientID:  "nexus-api",
		OpenFGAAPIURL:     "http://openfga.example",
		OpenFGAStoreID:    "store-1",
		OpenFGAModelID:    "model-1",
		ObjectStoreDir:    "/var/lib/nexus-pro-api/objects",
	}

	err := cfg.ValidateStartup()
	if err == nil {
		t.Fatal("expected cleartext production security URL validation error")
	}
	for _, want := range []string{"KEYCLOAK_BASE_URL", "OPENFGA_BASE_URL"} {
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
	for _, want := range []string{"DB_HOST, DB_USERNAME, and DB_NAME", "KEYCLOAK_BASE_URL", "KEYCLOAK_CLIENT_ID", "OPENFGA_BASE_URL", "OPENFGA_STORE_ID", "OPENFGA_MODEL_ID", "TEMPORAL_BASE_URL", "TEMPORAL_NAMESPACE", "TEMPORAL_TASK_QUEUE", "OBJECT_STORE_PROVIDER"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected validation error to mention %s, got %v", want, err)
		}
	}
}

// TestValidateStartupRejectsProductionDatabaseWithoutSSL 驗證 startup rejects production database without ssl。
func TestValidateStartupRejectsProductionDatabaseWithoutSSL(t *testing.T) {
	base := config.Config{
		Env:               "production",
		DatabaseURL:       "postgres://nexus:nexus@db.internal:5432/nexus_pro_be?sslmode=require",
		DBSSLMode:         "require",
		KeycloakIssuerURL: "https://issuer.example/realms/nexus",
		KeycloakClientID:  "nexus-api",
		OpenFGAAPIURL:     "https://openfga.example",
		OpenFGAStoreID:    "store-1",
		OpenFGAModelID:    "model-1",
		TemporalBaseURL:   "temporal:7233",
		TemporalNamespace: "default",
		TemporalTaskQueue: "nexus-workflows",
		ObjectStoreDir:    "/var/lib/nexus-pro-api/objects",
	}
	for name, sslmode := range map[string]string{
		"sslmode disable": "disable",
		"sslmode missing": "",
		"sslmode prefer":  "prefer",
		"sslmode allow":   "allow",
	} {
		cfg := base
		cfg.DBSSLMode = sslmode
		err := cfg.ValidateStartup()
		if err == nil || !strings.Contains(err.Error(), "DB_SSLMODE") {
			t.Fatalf("%s: expected production DB_SSLMODE error, got %v", name, err)
		}
	}
	for _, sslmode := range []string{"require", "verify-ca", "verify-full"} {
		cfg := base
		cfg.DBSSLMode = sslmode
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
		DBSSLMode:         "require",
		KeycloakIssuerURL: "https://issuer.example/realms/nexus",
		KeycloakClientID:  "nexus-api",
		OpenFGAAPIURL:     "https://openfga.example",
		OpenFGAStoreID:    "store-1",
		OpenFGAModelID:    "model-1",
		ObjectStoreDir:    "/var/lib/nexus-pro-api/objects",
		TemporalNamespace: "default",
		TemporalTaskQueue: "nexus-workflows",
	}

	err := cfg.ValidateStartup()
	if err == nil || !strings.Contains(err.Error(), "TEMPORAL_BASE_URL") {
		t.Fatalf("expected production Temporal host validation error, got %v", err)
	}
}

// TestValidateStartupRejectsProductionNATSInvalidURL 驗證 production NATS URL 校驗。
func TestValidateStartupRejectsProductionNATSInvalidURL(t *testing.T) {
	cfg := config.Config{
		Env:               "production",
		DatabaseURL:       "postgres://nexus:nexus@db.internal:5432/nexus_pro_be?sslmode=require",
		DBSSLMode:         "require",
		KeycloakIssuerURL: "https://issuer.example/realms/nexus",
		KeycloakClientID:  "nexus-api",
		OpenFGAAPIURL:     "https://openfga.example",
		OpenFGAStoreID:    "store-1",
		OpenFGAModelID:    "model-1",
		TemporalBaseURL:   "temporal:7233",
		TemporalNamespace: "default",
		TemporalTaskQueue: "nexus-workflows",
		ObjectStoreDir:    "/var/lib/nexus-pro-api/objects",
		NATSEnabled:       true,
		NATSURL:           "http://nats:4222",
	}

	err := cfg.ValidateStartup()
	if err == nil || !strings.Contains(err.Error(), "NATS_BASE_URL") {
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

// TestRateLimitConfigDefaultsToFailClosedInProduction secures the production image without changing local defaults.
func TestRateLimitConfigDefaultsToFailClosedInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")

	cfg, err := config.LoadE()
	if err != nil {
		t.Fatalf("expected production rate limit defaults to load, got %v", err)
	}
	if !cfg.RateLimitEnabled || !cfg.RateLimitFailClosed {
		t.Fatalf("expected production rate limiting to default on and fail closed, got %+v", cfg)
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

// TestOpenFGAAuthTokenConfig 驗證 OpenFGA preshared token 從環境變數載入。
func TestOpenFGAAuthTokenConfig(t *testing.T) {
	t.Setenv("OPENFGA_AUTH_TOKEN", "  dev-token  ")
	cfg, err := config.LoadE()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OpenFGAAuthToken != "dev-token" {
		t.Fatalf("unexpected OpenFGA auth token: %q", cfg.OpenFGAAuthToken)
	}
}

// TestValidateStartupRequiresOpenFGATokenWhenScopeChecksEnabled 驗證 production 啟用 scope check 時 token 必填（fail-closed）。
func TestValidateStartupRequiresOpenFGATokenWhenScopeChecksEnabled(t *testing.T) {
	base := config.Config{
		Env:               "production",
		DatabaseURL:       "postgres://nexus:nexus@db.internal:5432/nexus_pro_be?sslmode=require",
		DBSSLMode:         "require",
		KeycloakIssuerURL: "https://issuer.example/realms/nexus",
		KeycloakClientID:  "nexus-api",
		OpenFGAAPIURL:     "https://openfga.example",
		OpenFGAStoreID:    "store-1",
		OpenFGAModelID:    "model-1",
		TemporalBaseURL:   "temporal:7233",
		TemporalNamespace: "default",
		TemporalTaskQueue: "nexus-workflows",
		ObjectStoreDir:    "/var/lib/nexus-pro-api/objects",
	}

	cfg := base
	cfg.OpenFGAScopeCheckEnabled = true
	err := cfg.ValidateStartup()
	if err == nil || !strings.Contains(err.Error(), "OPENFGA_AUTH_TOKEN") {
		t.Fatalf("expected OPENFGA_AUTH_TOKEN fail-closed error, got %v", err)
	}

	cfg = base
	cfg.OpenFGAScopeCheckEnabled = true
	cfg.OpenFGAAuthToken = "prod-token"
	if err := cfg.ValidateStartup(); err != nil {
		t.Fatalf("expected configured token to satisfy validation, got %v", err)
	}

	cfg = base
	cfg.Env = "development"
	cfg.OpenFGAScopeCheckEnabled = true
	if err := cfg.ValidateStartup(); err != nil {
		t.Fatalf("expected development to tolerate empty token for backward compatibility, got %v", err)
	}
}
