package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 定義組態的資料結構。
type Config struct {
	Env      string
	HTTPAddr string
	LogLevel string

	DatabaseURL       string
	DBMaxConns        int
	DBMinConns        int
	DBMaxConnLifetime time.Duration

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	CORSAllowedOrigins []string

	// TrustedProxies 說明此處的程式契約。
	// trusted 說明此處的程式契約。
	TrustedProxies []string

	// MetricsAddr 說明此處的程式契約。
	// An 說明此處的程式契約。
	MetricsAddr string

	RateLimitEnabled bool
	RateLimitRPS     int
	RateLimitBurst   int

	KeycloakIssuerURL         string
	KeycloakClientID          string
	KeycloakProvisionUsers    bool
	KeycloakAdminClientID     string
	KeycloakAdminClientSecret string
	KeycloakSendInviteEmail   bool
	KeycloakInviteClientID    string
	KeycloakInviteRedirectURL string

	OpenFGAAPIURL            string
	OpenFGAStoreID           string
	OpenFGAModelID           string
	OpenFGAScopeCheckEnabled bool

	ObjectStoreProvider        string
	ObjectStoreDir             string
	ObjectStoreEndpoint        string
	ObjectStoreBucket          string
	ObjectStoreRegion          string
	ObjectStoreAccessKeyID     string
	ObjectStoreSecretAccessKey string
	ObjectStoreUseSSL          bool
	ObjectStoreCreateBucket    bool

	EHRMSBaseURL        string
	EHRMSAPIKey         string
	EHRMSSyncEnabled    bool
	EHRMSSyncInterval   time.Duration
	EHRMSSyncMode       string
	EHRMSSyncTenantID   string
	EHRMSSyncAccountID  string
	EHRMSSyncRunOnStart bool

	OTelEnabled              bool
	OTelServiceName          string
	OTelExporterOTLPEndpoint string
	OTelExporterOTLPInsecure bool
}

// ValidateStartup 驗證 startup。
func (c Config) ValidateStartup() error {
	if c.Env != "production" {
		return nil
	}
	problems := []string{}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		problems = append(problems, "DATABASE_URL is required")
	} else if problem := productionDatabaseSSLProblem(c.DatabaseURL); problem != "" {
		problems = append(problems, problem)
	}
	if strings.TrimSpace(c.KeycloakIssuerURL) == "" {
		problems = append(problems, "KEYCLOAK_ISSUER_URL is required")
	} else if problem := productionHTTPSURLProblem("KEYCLOAK_ISSUER_URL", c.KeycloakIssuerURL); problem != "" {
		problems = append(problems, problem)
	}
	if strings.TrimSpace(c.KeycloakClientID) == "" {
		problems = append(problems, "KEYCLOAK_CLIENT_ID is required")
	}
	if c.KeycloakProvisionUsers {
		if strings.TrimSpace(c.KeycloakAdminClientID) == "" {
			problems = append(problems, "KEYCLOAK_ADMIN_CLIENT_ID is required when KEYCLOAK_PROVISION_USERS=true")
		}
		if strings.TrimSpace(c.KeycloakAdminClientSecret) == "" {
			problems = append(problems, "KEYCLOAK_ADMIN_CLIENT_SECRET is required when KEYCLOAK_PROVISION_USERS=true")
		}
	}
	if strings.TrimSpace(c.KeycloakInviteRedirectURL) != "" {
		if problem := productionHTTPSURLProblem("KEYCLOAK_INVITE_REDIRECT_URL", c.KeycloakInviteRedirectURL); problem != "" {
			problems = append(problems, problem)
		}
	}
	if strings.TrimSpace(c.OpenFGAAPIURL) == "" {
		problems = append(problems, "OPENFGA_API_URL is required")
	} else if problem := productionHTTPSURLProblem("OPENFGA_API_URL", c.OpenFGAAPIURL); problem != "" {
		problems = append(problems, problem)
	}
	if strings.TrimSpace(c.OpenFGAStoreID) == "" {
		problems = append(problems, "OPENFGA_STORE_ID is required")
	}
	if strings.TrimSpace(c.OpenFGAModelID) == "" {
		problems = append(problems, "OPENFGA_MODEL_ID is required")
	}
	switch normalizeObjectStoreProvider(c.ObjectStoreProvider, c.ObjectStoreDir, c.ObjectStoreEndpoint, c.ObjectStoreBucket) {
	case "minio", "s3":
		if strings.TrimSpace(c.ObjectStoreEndpoint) == "" {
			problems = append(problems, "OBJECT_STORE_ENDPOINT is required")
		}
		if strings.TrimSpace(c.ObjectStoreBucket) == "" {
			problems = append(problems, "OBJECT_STORE_BUCKET is required")
		}
		if strings.TrimSpace(c.ObjectStoreAccessKeyID) == "" {
			problems = append(problems, "OBJECT_STORE_ACCESS_KEY_ID is required")
		}
		if strings.TrimSpace(c.ObjectStoreSecretAccessKey) == "" {
			problems = append(problems, "OBJECT_STORE_SECRET_ACCESS_KEY is required")
		}
	case "local":
		if strings.TrimSpace(c.ObjectStoreDir) == "" {
			problems = append(problems, "OBJECT_STORE_DIR is required")
		}
	default:
		problems = append(problems, "OBJECT_STORE_PROVIDER must be minio, s3, or local")
	}
	if strings.TrimSpace(c.EHRMSBaseURL) != "" {
		if problem := productionHTTPSURLProblem("EHRMS_BASE_URL", c.EHRMSBaseURL); problem != "" {
			problems = append(problems, problem)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("production configuration invalid: %s", strings.Join(problems, "; "))
	}
	return nil
}

// Load 載入目前流程。
func Load() Config {
	cfg, _ := LoadE()
	return cfg
}

// LoadE 載入 e。
func LoadE() (Config, error) {
	appEnv := env("APP_ENV", "development")
	problems := []string{}
	objectStoreProvider := normalizeObjectStoreProvider(os.Getenv("OBJECT_STORE_PROVIDER"), os.Getenv("OBJECT_STORE_DIR"), os.Getenv("OBJECT_STORE_ENDPOINT"), os.Getenv("OBJECT_STORE_BUCKET"))
	cfg := Config{
		Env:               appEnv,
		HTTPAddr:          env("HTTP_ADDR", ":8080"),
		LogLevel:          env("LOG_LEVEL", "info"),
		DatabaseURL:       strings.TrimSpace(os.Getenv("DATABASE_URL")),
		DBMaxConns:        envInt("DB_MAX_CONNS", 10, &problems),
		DBMinConns:        envInt("DB_MIN_CONNS", 1, &problems),
		DBMaxConnLifetime: envDuration("DB_MAX_CONN_LIFETIME", time.Hour, &problems),
		RedisAddr:         strings.TrimSpace(os.Getenv("REDIS_ADDR")),
		RedisPassword:     os.Getenv("REDIS_PASSWORD"),
		RedisDB:           envInt("REDIS_DB", 0, &problems),

		CORSAllowedOrigins: splitCommaList(os.Getenv("CORS_ALLOWED_ORIGINS")),

		TrustedProxies: splitCommaList(os.Getenv("TRUSTED_PROXIES")),
		MetricsAddr:    envAllowEmpty("METRICS_ADDR", "127.0.0.1:9091"),

		RateLimitEnabled: envBool("RATE_LIMIT_ENABLED", false, &problems),
		RateLimitRPS:     envInt("RATE_LIMIT_RPS", 20, &problems),
		RateLimitBurst:   envInt("RATE_LIMIT_BURST", 40, &problems),

		KeycloakIssuerURL:         strings.TrimSpace(os.Getenv("KEYCLOAK_ISSUER_URL")),
		KeycloakClientID:          strings.TrimSpace(os.Getenv("KEYCLOAK_CLIENT_ID")),
		KeycloakProvisionUsers:    envBool("KEYCLOAK_PROVISION_USERS", false, &problems),
		KeycloakAdminClientID:     strings.TrimSpace(os.Getenv("KEYCLOAK_ADMIN_CLIENT_ID")),
		KeycloakAdminClientSecret: os.Getenv("KEYCLOAK_ADMIN_CLIENT_SECRET"),
		KeycloakSendInviteEmail:   envBool("KEYCLOAK_SEND_INVITE_EMAIL", false, &problems),
		KeycloakInviteClientID:    env("KEYCLOAK_INVITE_CLIENT_ID", strings.TrimSpace(os.Getenv("KEYCLOAK_CLIENT_ID"))),
		KeycloakInviteRedirectURL: strings.TrimSpace(os.Getenv("KEYCLOAK_INVITE_REDIRECT_URL")),

		OpenFGAAPIURL:            strings.TrimSpace(os.Getenv("OPENFGA_API_URL")),
		OpenFGAStoreID:           strings.TrimSpace(os.Getenv("OPENFGA_STORE_ID")),
		OpenFGAModelID:           strings.TrimSpace(os.Getenv("OPENFGA_MODEL_ID")),
		OpenFGAScopeCheckEnabled: envBool("OPENFGA_SCOPE_CHECK_ENABLED", false, &problems),

		ObjectStoreProvider:        objectStoreProvider,
		ObjectStoreDir:             strings.TrimSpace(os.Getenv("OBJECT_STORE_DIR")),
		ObjectStoreEndpoint:        strings.TrimSpace(os.Getenv("OBJECT_STORE_ENDPOINT")),
		ObjectStoreBucket:          strings.TrimSpace(os.Getenv("OBJECT_STORE_BUCKET")),
		ObjectStoreRegion:          env("OBJECT_STORE_REGION", "us-east-1"),
		ObjectStoreAccessKeyID:     strings.TrimSpace(os.Getenv("OBJECT_STORE_ACCESS_KEY_ID")),
		ObjectStoreSecretAccessKey: os.Getenv("OBJECT_STORE_SECRET_ACCESS_KEY"),
		ObjectStoreUseSSL:          envBool("OBJECT_STORE_USE_SSL", false, &problems),
		ObjectStoreCreateBucket:    envBool("OBJECT_STORE_CREATE_BUCKET", false, &problems),

		EHRMSBaseURL:        strings.TrimSpace(os.Getenv("EHRMS_BASE_URL")),
		EHRMSAPIKey:         os.Getenv("EHRMS_API_KEY"),
		EHRMSSyncEnabled:    envBool("EHRMS_SYNC_ENABLED", false, &problems),
		EHRMSSyncInterval:   envDuration("EHRMS_SYNC_INTERVAL", 24*time.Hour, &problems),
		EHRMSSyncMode:       env("EHRMS_SYNC_MODE", "upsert"),
		EHRMSSyncTenantID:   strings.TrimSpace(os.Getenv("EHRMS_SYNC_TENANT_ID")),
		EHRMSSyncAccountID:  strings.TrimSpace(os.Getenv("EHRMS_SYNC_ACCOUNT_ID")),
		EHRMSSyncRunOnStart: envBool("EHRMS_SYNC_RUN_ON_START", false, &problems),

		OTelEnabled:              envBool("OTEL_ENABLED", false, &problems),
		OTelServiceName:          env("OTEL_SERVICE_NAME", "nexus-pro-be"),
		OTelExporterOTLPEndpoint: env("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		OTelExporterOTLPInsecure: envBool("OTEL_EXPORTER_OTLP_INSECURE", true, &problems),
	}
	problems = append(problems, ehrmsConfigProblems(cfg.EHRMSBaseURL, cfg.EHRMSAPIKey)...)
	problems = append(problems, ehrmsSyncConfigProblems(cfg)...)
	problems = append(problems, keycloakProvisioningConfigProblems(cfg)...)
	problems = append(problems, databasePoolConfigProblems(cfg)...)
	problems = append(problems, rateLimitConfigProblems(cfg)...)
	problems = append(problems, trustedProxiesConfigProblems(cfg)...)
	if len(problems) > 0 {
		return cfg, fmt.Errorf("configuration invalid: %s", strings.Join(problems, "; "))
	}
	return cfg, nil
}

// keycloakProvisioningConfigProblems 處理 Keycloak 開通組態 problems。
func keycloakProvisioningConfigProblems(c Config) []string {
	problems := []string{}
	if !c.KeycloakProvisionUsers {
		if c.KeycloakSendInviteEmail {
			problems = append(problems, "KEYCLOAK_SEND_INVITE_EMAIL requires KEYCLOAK_PROVISION_USERS=true")
		}
		return problems
	}
	if strings.TrimSpace(c.KeycloakIssuerURL) == "" {
		problems = append(problems, "KEYCLOAK_ISSUER_URL is required when KEYCLOAK_PROVISION_USERS=true")
	} else if problem := httpURLProblem("KEYCLOAK_ISSUER_URL", c.KeycloakIssuerURL); problem != "" {
		problems = append(problems, problem)
	}
	if strings.TrimSpace(c.KeycloakAdminClientID) == "" {
		problems = append(problems, "KEYCLOAK_ADMIN_CLIENT_ID is required when KEYCLOAK_PROVISION_USERS=true")
	}
	if strings.TrimSpace(c.KeycloakAdminClientSecret) == "" {
		problems = append(problems, "KEYCLOAK_ADMIN_CLIENT_SECRET is required when KEYCLOAK_PROVISION_USERS=true")
	}
	if strings.TrimSpace(c.KeycloakInviteRedirectURL) != "" {
		if problem := httpURLProblem("KEYCLOAK_INVITE_REDIRECT_URL", c.KeycloakInviteRedirectURL); problem != "" {
			problems = append(problems, problem)
		}
	}
	return problems
}

// ehrmsConfigProblems 處理 eHRMS 組態 problems。
func ehrmsConfigProblems(baseURL string, apiKey string) []string {
	baseURL = strings.TrimSpace(baseURL)
	apiKey = strings.TrimSpace(apiKey)
	if baseURL == "" && apiKey == "" {
		return nil
	}
	problems := []string{}
	if baseURL == "" {
		problems = append(problems, "EHRMS_BASE_URL is required when EHRMS_API_KEY is set")
	} else if problem := httpURLProblem("EHRMS_BASE_URL", baseURL); problem != "" {
		problems = append(problems, problem)
	}
	if apiKey == "" {
		problems = append(problems, "EHRMS_API_KEY is required when EHRMS_BASE_URL is set")
	}
	return problems
}

// ehrmsSyncConfigProblems 處理 eHRMS sync 組態 problems。
func ehrmsSyncConfigProblems(c Config) []string {
	if !c.EHRMSSyncEnabled {
		return nil
	}
	problems := []string{}
	if strings.TrimSpace(c.EHRMSBaseURL) == "" {
		problems = append(problems, "EHRMS_BASE_URL is required when EHRMS_SYNC_ENABLED=true")
	}
	if strings.TrimSpace(c.EHRMSAPIKey) == "" {
		problems = append(problems, "EHRMS_API_KEY is required when EHRMS_SYNC_ENABLED=true")
	}
	if strings.TrimSpace(c.EHRMSSyncTenantID) == "" {
		problems = append(problems, "EHRMS_SYNC_TENANT_ID is required when EHRMS_SYNC_ENABLED=true")
	}
	if strings.TrimSpace(c.EHRMSSyncAccountID) == "" {
		problems = append(problems, "EHRMS_SYNC_ACCOUNT_ID is required when EHRMS_SYNC_ENABLED=true")
	}
	switch strings.ToLower(strings.TrimSpace(c.EHRMSSyncMode)) {
	case "", "create", "update", "upsert":
	default:
		problems = append(problems, "EHRMS_SYNC_MODE must be create, update, or upsert")
	}
	return problems
}

// databasePoolConfigProblems 處理 database pool 組態 problems。
func databasePoolConfigProblems(c Config) []string {
	problems := []string{}
	if c.DBMaxConns < 1 {
		problems = append(problems, "DB_MAX_CONNS must be at least 1")
	}
	if c.DBMinConns < 0 {
		problems = append(problems, "DB_MIN_CONNS must be zero or positive")
	}
	if c.DBMaxConns >= 1 && c.DBMinConns > c.DBMaxConns {
		problems = append(problems, "DB_MIN_CONNS must not exceed DB_MAX_CONNS")
	}
	return problems
}

// rateLimitConfigProblems 處理速率限制組態 problems。
func rateLimitConfigProblems(c Config) []string {
	if !c.RateLimitEnabled {
		return nil
	}
	problems := []string{}
	if c.RateLimitRPS < 1 {
		problems = append(problems, "RATE_LIMIT_RPS must be at least 1 when RATE_LIMIT_ENABLED=true")
	}
	if c.RateLimitBurst < 1 {
		problems = append(problems, "RATE_LIMIT_BURST must be at least 1 when RATE_LIMIT_ENABLED=true")
	}
	return problems
}

// trustedProxiesConfigProblems 處理 trusted proxies 組態 problems。
func trustedProxiesConfigProblems(c Config) []string {
	problems := []string{}
	for _, entry := range c.TrustedProxies {
		if net.ParseIP(entry) != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(entry); err == nil {
			continue
		}
		problems = append(problems, fmt.Sprintf("TRUSTED_PROXIES entry must be an IP or CIDR: %q", entry))
	}
	return problems
}

// httpURLProblem 處理 HTTP URL problem。
func httpURLProblem(name string, raw string) string {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return name + " must be a valid http or https URL"
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return name + " must be a valid http or https URL"
	}
	return ""
}

// productionDatabaseSSLProblem 處理 production database ssl problem。
func productionDatabaseSSLProblem(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "DATABASE_URL must be a valid URL in production"
	}
	switch strings.ToLower(strings.TrimSpace(parsed.Query().Get("sslmode"))) {
	case "require", "verify-ca", "verify-full":
		return ""
	case "":
		// pgx 預設 sslmode=prefer，可能靜默退回明文連線。
		return "DATABASE_URL must set sslmode=require, verify-ca, or verify-full in production"
	default:
		return "DATABASE_URL must not use a non-TLS sslmode in production (require, verify-ca, or verify-full)"
	}
}

// productionHTTPSURLProblem 處理 production httpsurl problem。
func productionHTTPSURLProblem(name string, raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return name + " must be an https URL in production"
	}
	return ""
}

// normalizeObjectStoreProvider 正規化物件儲存層提供者。
func normalizeObjectStoreProvider(provider, dir, endpoint, bucket string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider != "" {
		return provider
	}
	if strings.TrimSpace(endpoint) != "" || strings.TrimSpace(bucket) != "" {
		return "minio"
	}
	if strings.TrimSpace(dir) != "" {
		return "local"
	}
	return "memory"
}

// splitCommaList 拆分comma 列表。
func splitCommaList(raw string) []string {
	values := []string{}
	for _, part := range strings.Split(raw, ",") {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

// envAllowEmpty 處理 env allow 空值。
func envAllowEmpty(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(v)
	}
	return fallback
}

// env 處理 env。
func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// envInt 處理 env 整數。
func envInt(key string, fallback int, problems *[]string) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		*problems = append(*problems, fmt.Sprintf("%s must be an integer: %q", key, v))
		return fallback
	}
	return parsed
}

// envBool 處理 env 布林值。
func envBool(key string, fallback bool, problems *[]string) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		*problems = append(*problems, fmt.Sprintf("%s must be a boolean: %q", key, v))
		return fallback
	}
	return parsed
}

// envDuration 處理 env duration。
func envDuration(key string, fallback time.Duration, problems *[]string) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil || parsed <= 0 {
		*problems = append(*problems, fmt.Sprintf("%s must be a positive duration: %q", key, v))
		return fallback
	}
	return parsed
}
