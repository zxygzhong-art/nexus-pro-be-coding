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

	// DatabaseURL is derived from DB_HOST/DB_PORT/DB_USERNAME/DB_PASSWORD/DB_NAME/DB_SSLMODE.
	DatabaseURL       string
	DBHost            string
	DBPort            string
	DBUsername        string
	DBPassword        string
	DBName            string
	DBSSLMode         string
	DBMaxConns        int
	DBMinConns        int
	DBMaxConnLifetime time.Duration

	// RedisAddr is derived from REDIS_HOST/REDIS_PORT.
	RedisAddr     string
	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       int

	CORSAllowedOrigins []string

	// TrustedProxies 說明此處的程式契約。
	// trusted 說明此處的程式契約。
	TrustedProxies []string

	// MetricsAddr 說明此處的程式契約。
	// An 說明此處的程式契約。
	MetricsAddr string

	RateLimitEnabled    bool
	RateLimitRPS        int
	RateLimitBurst      int
	RateLimitFailClosed bool

	SwaggerEnabled bool

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

	TemporalBaseURL   string
	TemporalNamespace string
	TemporalTaskQueue string

	NATSEnabled        bool
	NATSURL            string
	NATSStream         string
	NATSConsumerPrefix string

	LiteLLMBaseURL        string
	LiteLLMAPIKey         string
	LiteLLMMasterKey      string
	LiteLLMEmbeddingModel string

	AgentToolCredentialEncryptionKey string

	ObjectStoreProvider                string
	ObjectStoreDir                     string
	ObjectStoreEndpoint                string
	ObjectStoreBucket                  string
	ObjectStoreRegion                  string
	ObjectStoreAccessKeyID             string
	ObjectStoreSecretAccessKey         string
	ObjectStoreSFTPHostKey             string
	ObjectStoreSFTPInsecureSkipHostKey bool
	ObjectStoreUseSSL                  bool
	ObjectStoreCreateBucket            bool

	EHRMSBaseURL        string
	EHRMSAPIKey         string
	EHRMSSyncEnabled    bool
	EHRMSSyncInterval   time.Duration
	EHRMSSyncMode       string
	EHRMSSyncTenantID   string
	EHRMSSyncAccountID  string
	EHRMSSyncRunOnStart bool

	EHRMSAttendanceSyncEnabled    bool
	EHRMSAttendanceSyncInterval   time.Duration
	EHRMSAttendanceSyncMode       string
	EHRMSAttendanceSyncTenantID   string
	EHRMSAttendanceSyncAccountID  string
	EHRMSAttendanceSyncRunOnStart bool
	EHRMSAttendanceSyncSince      string

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
		problems = append(problems, "DB_HOST, DB_USERNAME, and DB_NAME are required")
	} else if problem := productionDatabaseSSLProblem(c.DBSSLMode); problem != "" {
		problems = append(problems, problem)
	}
	if strings.TrimSpace(c.KeycloakIssuerURL) == "" {
		problems = append(problems, "KEYCLOAK_BASE_URL is required")
	} else if problem := productionHTTPSURLProblem("KEYCLOAK_BASE_URL", c.KeycloakIssuerURL); problem != "" {
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
		problems = append(problems, "OPENFGA_BASE_URL is required")
	} else if problem := productionHTTPSURLProblem("OPENFGA_BASE_URL", c.OpenFGAAPIURL); problem != "" {
		problems = append(problems, problem)
	}
	if strings.TrimSpace(c.OpenFGAStoreID) == "" {
		problems = append(problems, "OPENFGA_STORE_ID is required")
	}
	if strings.TrimSpace(c.OpenFGAModelID) == "" {
		problems = append(problems, "OPENFGA_MODEL_ID is required")
	}
	problems = append(problems, temporalConfigProblems(c)...)
	if c.NATSEnabled {
		if strings.TrimSpace(c.NATSURL) == "" {
			problems = append(problems, "NATS_BASE_URL is required when NATS_ENABLED=true")
		} else if problem := natsURLProblem("NATS_BASE_URL", c.NATSURL); problem != "" {
			problems = append(problems, problem)
		}
	}
	switch normalizeObjectStoreProvider(c.ObjectStoreProvider, c.ObjectStoreDir, c.ObjectStoreEndpoint, c.ObjectStoreBucket) {
	case "sftpgo":
		if strings.TrimSpace(c.ObjectStoreEndpoint) == "" {
			problems = append(problems, "SFTPGO_BASE_URL is required")
		} else if problem := sftpgoEndpointProblem(c.ObjectStoreEndpoint); problem != "" {
			problems = append(problems, problem)
		}
		if strings.TrimSpace(c.ObjectStoreBucket) == "" {
			problems = append(problems, "SFTPGO_ROOT_BUCKET or OBJECT_STORE_BUCKET is required as the SFTPGo root directory")
		}
		if strings.TrimSpace(c.ObjectStoreAccessKeyID) == "" {
			problems = append(problems, "SFTPGO_USERNAME or OBJECT_STORE_ACCESS_KEY_ID is required as the SFTPGo username")
		}
		if strings.TrimSpace(c.ObjectStoreSecretAccessKey) == "" {
			problems = append(problems, "SFTPGO_PASSWORD or OBJECT_STORE_SECRET_ACCESS_KEY is required as the SFTPGo password")
		}
		if isSFTPGoHTTPEndpoint(c.ObjectStoreEndpoint) {
			if problem := productionHTTPSURLProblem("SFTPGO_BASE_URL", c.ObjectStoreEndpoint); problem != "" {
				problems = append(problems, problem)
			}
		} else {
			if c.ObjectStoreSFTPInsecureSkipHostKey {
				problems = append(problems, "OBJECT_STORE_SFTP_INSECURE_SKIP_HOST_KEY must not be enabled in production")
			}
			if strings.TrimSpace(c.ObjectStoreSFTPHostKey) == "" {
				problems = append(problems, "OBJECT_STORE_SFTP_HOST_KEY is required in production when OBJECT_STORE_PROVIDER=sftpgo uses sftp://")
			}
		}
	case "local":
		if strings.TrimSpace(c.ObjectStoreDir) == "" {
			problems = append(problems, "OBJECT_STORE_DIR is required")
		}
	default:
		problems = append(problems, "OBJECT_STORE_PROVIDER must be sftpgo or local")
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
	sftpgoBaseURL := strings.TrimSpace(os.Getenv("SFTPGO_BASE_URL"))
	objectStoreBucket := firstNonEmptyEnv("SFTPGO_ROOT_BUCKET", "OBJECT_STORE_BUCKET")
	objectStoreAccessKeyID := firstNonEmptyEnv("SFTPGO_USERNAME", "OBJECT_STORE_ACCESS_KEY_ID")
	objectStoreSecretAccessKey := firstNonEmptyEnv("SFTPGO_PASSWORD", "OBJECT_STORE_SECRET_ACCESS_KEY")
	objectStoreProvider := normalizeObjectStoreProvider(os.Getenv("OBJECT_STORE_PROVIDER"), os.Getenv("OBJECT_STORE_DIR"), sftpgoBaseURL, objectStoreBucket)
	dbHost := strings.TrimSpace(os.Getenv("DB_HOST"))
	dbPort := env("DB_PORT", "5432")
	dbUsername := strings.TrimSpace(os.Getenv("DB_USERNAME"))
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := strings.TrimSpace(os.Getenv("DB_NAME"))
	dbSSLMode := env("DB_SSLMODE", "disable")
	redisHost := strings.TrimSpace(os.Getenv("REDIS_HOST"))
	redisPort := env("REDIS_PORT", "6379")
	cfg := Config{
		Env:               appEnv,
		HTTPAddr:          env("HTTP_ADDR", ":8080"),
		LogLevel:          env("LOG_LEVEL", "info"),
		DBHost:            dbHost,
		DBPort:            dbPort,
		DBUsername:        dbUsername,
		DBPassword:        dbPassword,
		DBName:            dbName,
		DBSSLMode:         dbSSLMode,
		DatabaseURL:       BuildDatabaseURL(dbHost, dbPort, dbUsername, dbPassword, dbName, dbSSLMode),
		DBMaxConns:        envInt("DB_MAX_CONNS", 10, &problems),
		DBMinConns:        envInt("DB_MIN_CONNS", 1, &problems),
		DBMaxConnLifetime: envDuration("DB_MAX_CONN_LIFETIME", time.Hour, &problems),
		RedisHost:         redisHost,
		RedisPort:         redisPort,
		RedisAddr:         buildRedisAddr(redisHost, redisPort),
		RedisPassword:     os.Getenv("REDIS_PASSWORD"),
		RedisDB:           envInt("REDIS_DB", 0, &problems),

		CORSAllowedOrigins: splitCommaList(os.Getenv("CORS_ALLOWED_ORIGINS")),

		TrustedProxies: splitCommaList(os.Getenv("TRUSTED_PROXIES")),
		MetricsAddr:    envAllowEmpty("METRICS_ADDR", "127.0.0.1:9091"),

		RateLimitEnabled:    envBool("RATE_LIMIT_ENABLED", false, &problems),
		RateLimitRPS:        envInt("RATE_LIMIT_RPS", 20, &problems),
		RateLimitBurst:      envInt("RATE_LIMIT_BURST", 40, &problems),
		RateLimitFailClosed: envBool("RATE_LIMIT_FAIL_CLOSED", false, &problems),

		SwaggerEnabled: envBool("SWAGGER_ENABLED", appEnv != "production", &problems),

		KeycloakIssuerURL:         strings.TrimSpace(os.Getenv("KEYCLOAK_BASE_URL")),
		KeycloakClientID:          strings.TrimSpace(os.Getenv("KEYCLOAK_CLIENT_ID")),
		KeycloakProvisionUsers:    envBool("KEYCLOAK_PROVISION_USERS", false, &problems),
		KeycloakAdminClientID:     strings.TrimSpace(os.Getenv("KEYCLOAK_ADMIN_CLIENT_ID")),
		KeycloakAdminClientSecret: os.Getenv("KEYCLOAK_ADMIN_CLIENT_SECRET"),
		KeycloakSendInviteEmail:   envBool("KEYCLOAK_SEND_INVITE_EMAIL", false, &problems),
		KeycloakInviteClientID:    env("KEYCLOAK_INVITE_CLIENT_ID", strings.TrimSpace(os.Getenv("KEYCLOAK_CLIENT_ID"))),
		KeycloakInviteRedirectURL: strings.TrimSpace(os.Getenv("KEYCLOAK_INVITE_REDIRECT_URL")),

		OpenFGAAPIURL:            strings.TrimSpace(os.Getenv("OPENFGA_BASE_URL")),
		OpenFGAStoreID:           strings.TrimSpace(os.Getenv("OPENFGA_STORE_ID")),
		OpenFGAModelID:           strings.TrimSpace(os.Getenv("OPENFGA_MODEL_ID")),
		OpenFGAScopeCheckEnabled: envBool("OPENFGA_SCOPE_CHECK_ENABLED", false, &problems),

		TemporalBaseURL:   envAllowEmpty("TEMPORAL_BASE_URL", "127.0.0.1:27233"),
		TemporalNamespace: env("TEMPORAL_NAMESPACE", "default"),
		TemporalTaskQueue: env("TEMPORAL_TASK_QUEUE", "nexus-workflows"),

		NATSEnabled:        envBool("NATS_ENABLED", false, &problems),
		NATSURL:            env("NATS_BASE_URL", "nats://127.0.0.1:24222"),
		NATSStream:         env("NATS_STREAM", "NEXUS_EVENTS"),
		NATSConsumerPrefix: env("NATS_CONSUMER_PREFIX", "nexus"),

		LiteLLMBaseURL:        env("LITELLM_BASE_URL", "http://127.0.0.1:4000"),
		LiteLLMAPIKey:         os.Getenv("LITELLM_API_KEY"),
		LiteLLMMasterKey:      os.Getenv("LITELLM_MASTER_KEY"),
		LiteLLMEmbeddingModel: env("LITELLM_EMBEDDING_MODEL", "nexus-pro-embedding"),

		AgentToolCredentialEncryptionKey: strings.TrimSpace(os.Getenv("AGENT_TOOL_CREDENTIAL_ENCRYPTION_KEY")),

		ObjectStoreProvider:                objectStoreProvider,
		ObjectStoreDir:                     strings.TrimSpace(os.Getenv("OBJECT_STORE_DIR")),
		ObjectStoreEndpoint:                sftpgoBaseURL,
		ObjectStoreBucket:                  objectStoreBucket,
		ObjectStoreRegion:                  env("OBJECT_STORE_REGION", "us-east-1"),
		ObjectStoreAccessKeyID:             objectStoreAccessKeyID,
		ObjectStoreSecretAccessKey:         objectStoreSecretAccessKey,
		ObjectStoreSFTPHostKey:             strings.TrimSpace(os.Getenv("OBJECT_STORE_SFTP_HOST_KEY")),
		ObjectStoreSFTPInsecureSkipHostKey: envBool("OBJECT_STORE_SFTP_INSECURE_SKIP_HOST_KEY", false, &problems),
		ObjectStoreUseSSL:                  envBool("OBJECT_STORE_USE_SSL", false, &problems),
		ObjectStoreCreateBucket:            envBool("OBJECT_STORE_CREATE_BUCKET", false, &problems),

		EHRMSBaseURL:        strings.TrimSpace(os.Getenv("EHRMS_BASE_URL")),
		EHRMSAPIKey:         os.Getenv("EHRMS_API_KEY"),
		EHRMSSyncEnabled:    envBool("EHRMS_SYNC_ENABLED", false, &problems),
		EHRMSSyncInterval:   envDuration("EHRMS_SYNC_INTERVAL", 24*time.Hour, &problems),
		EHRMSSyncMode:       env("EHRMS_SYNC_MODE", "upsert"),
		EHRMSSyncTenantID:   strings.TrimSpace(os.Getenv("EHRMS_SYNC_TENANT_ID")),
		EHRMSSyncAccountID:  strings.TrimSpace(os.Getenv("EHRMS_SYNC_ACCOUNT_ID")),
		EHRMSSyncRunOnStart: envBool("EHRMS_SYNC_RUN_ON_START", false, &problems),

		EHRMSAttendanceSyncEnabled:    envBool("EHRMS_ATTENDANCE_SYNC_ENABLED", false, &problems),
		EHRMSAttendanceSyncInterval:   envDuration("EHRMS_ATTENDANCE_SYNC_INTERVAL", 30*24*time.Hour, &problems),
		EHRMSAttendanceSyncMode:       env("EHRMS_ATTENDANCE_SYNC_MODE", "upsert"),
		EHRMSAttendanceSyncTenantID:   strings.TrimSpace(os.Getenv("EHRMS_ATTENDANCE_SYNC_TENANT_ID")),
		EHRMSAttendanceSyncAccountID:  strings.TrimSpace(os.Getenv("EHRMS_ATTENDANCE_SYNC_ACCOUNT_ID")),
		EHRMSAttendanceSyncRunOnStart: envBool("EHRMS_ATTENDANCE_SYNC_RUN_ON_START", false, &problems),
		EHRMSAttendanceSyncSince:      strings.TrimSpace(os.Getenv("EHRMS_ATTENDANCE_SYNC_SINCE")),

		OTelEnabled:              envBool("OTEL_ENABLED", false, &problems),
		OTelServiceName:          env("OTEL_SERVICE_NAME", "nexus-pro-be"),
		OTelExporterOTLPEndpoint: env("OTEL_BASE_URL", "localhost:4317"),
		OTelExporterOTLPInsecure: envBool("OTEL_EXPORTER_OTLP_INSECURE", true, &problems),
	}
	if cfg.EHRMSAttendanceSyncTenantID == "" {
		cfg.EHRMSAttendanceSyncTenantID = cfg.EHRMSSyncTenantID
	}
	if cfg.EHRMSAttendanceSyncAccountID == "" {
		cfg.EHRMSAttendanceSyncAccountID = cfg.EHRMSSyncAccountID
	}
	problems = append(problems, ehrmsConfigProblems(cfg.EHRMSBaseURL, cfg.EHRMSAPIKey)...)
	problems = append(problems, ehrmsSyncConfigProblems(cfg)...)
	problems = append(problems, ehrmsAttendanceSyncConfigProblems(cfg)...)
	problems = append(problems, keycloakProvisioningConfigProblems(cfg)...)
	problems = append(problems, databasePoolConfigProblems(cfg)...)
	problems = append(problems, rateLimitConfigProblems(cfg)...)
	problems = append(problems, temporalConfigProblems(cfg)...)
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
		problems = append(problems, "KEYCLOAK_BASE_URL is required when KEYCLOAK_PROVISION_USERS=true")
	} else if problem := httpURLProblem("KEYCLOAK_BASE_URL", c.KeycloakIssuerURL); problem != "" {
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

// ehrmsAttendanceSyncConfigProblems 僅在考勤同步啟用時校驗其獨立配置。
// interval 最小值僅在 attendance-only（未開員工 sync）相容路徑強制。
func ehrmsAttendanceSyncConfigProblems(c Config) []string {
	if !c.EHRMSAttendanceSyncEnabled {
		return nil
	}
	attendanceOnly := !c.EHRMSSyncEnabled
	problems := []string{}
	if attendanceOnly {
		if strings.TrimSpace(c.EHRMSBaseURL) == "" {
			problems = append(problems, "EHRMS_BASE_URL is required when EHRMS_ATTENDANCE_SYNC_ENABLED=true")
		}
		if strings.TrimSpace(c.EHRMSAPIKey) == "" {
			problems = append(problems, "EHRMS_API_KEY is required when EHRMS_ATTENDANCE_SYNC_ENABLED=true")
		}
		if strings.TrimSpace(c.EHRMSAttendanceSyncTenantID) == "" {
			problems = append(problems, "EHRMS_ATTENDANCE_SYNC_TENANT_ID is required when EHRMS_ATTENDANCE_SYNC_ENABLED=true")
		}
		if strings.TrimSpace(c.EHRMSAttendanceSyncAccountID) == "" {
			problems = append(problems, "EHRMS_ATTENDANCE_SYNC_ACCOUNT_ID is required when EHRMS_ATTENDANCE_SYNC_ENABLED=true")
		}
		if c.EHRMSAttendanceSyncInterval > 0 && c.EHRMSAttendanceSyncInterval < 30*24*time.Hour {
			problems = append(problems, "EHRMS_ATTENDANCE_SYNC_INTERVAL must be at least 720h (30 days)")
		}
	}
	switch strings.ToLower(strings.TrimSpace(c.EHRMSAttendanceSyncMode)) {
	case "", "create", "update", "upsert":
	default:
		problems = append(problems, "EHRMS_ATTENDANCE_SYNC_MODE must be create, update, or upsert")
	}
	if since := strings.TrimSpace(c.EHRMSAttendanceSyncSince); since != "" {
		if _, err := time.Parse(time.DateOnly, since); err != nil {
			problems = append(problems, "EHRMS_ATTENDANCE_SYNC_SINCE must be YYYY-MM-DD")
		}
	}
	return problems
}

// temporalConfigProblems 處理 Temporal 組態 problems。
func temporalConfigProblems(c Config) []string {
	problems := []string{}
	if strings.TrimSpace(c.TemporalBaseURL) == "" {
		problems = append(problems, "TEMPORAL_BASE_URL is required")
	}
	if strings.TrimSpace(c.TemporalNamespace) == "" {
		problems = append(problems, "TEMPORAL_NAMESPACE is required")
	}
	if strings.TrimSpace(c.TemporalTaskQueue) == "" {
		problems = append(problems, "TEMPORAL_TASK_QUEUE is required")
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

// buildRedisAddr builds host:port from discrete REDIS_* fields.
func buildRedisAddr(host, port string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if strings.TrimSpace(port) == "" {
		port = "6379"
	}
	return net.JoinHostPort(host, port)
}

// BuildDatabaseURL builds a Postgres DSN from discrete DB_* fields.
func BuildDatabaseURL(host, port, username, password, name, sslmode string) string {
	host = strings.TrimSpace(host)
	username = strings.TrimSpace(username)
	name = strings.TrimSpace(name)
	if host == "" || username == "" || name == "" {
		return ""
	}
	if strings.TrimSpace(port) == "" {
		port = "5432"
	}
	if strings.TrimSpace(sslmode) == "" {
		sslmode = "disable"
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(username, password),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + name,
	}
	q := url.Values{}
	q.Set("sslmode", sslmode)
	u.RawQuery = q.Encode()
	return u.String()
}

// DatabaseURLFromEnv builds a Postgres DSN from DB_* environment variables.
func DatabaseURLFromEnv() string {
	return BuildDatabaseURL(
		os.Getenv("DB_HOST"),
		env("DB_PORT", "5432"),
		os.Getenv("DB_USERNAME"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		env("DB_SSLMODE", "disable"),
	)
}

// productionDatabaseSSLProblem 處理 production database ssl problem。
func productionDatabaseSSLProblem(sslmode string) string {
	switch strings.ToLower(strings.TrimSpace(sslmode)) {
	case "require", "verify-ca", "verify-full":
		return ""
	case "":
		return "DB_SSLMODE must be require, verify-ca, or verify-full in production"
	default:
		return "DB_SSLMODE must not use a non-TLS mode in production (require, verify-ca, or verify-full)"
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

// natsURLProblem validates one or more NATS client URLs.
func natsURLProblem(name string, raw string) string {
	values := strings.Split(raw, ",")
	for _, entry := range values {
		value := strings.TrimSpace(entry)
		if value == "" {
			return name + " must be a valid NATS URL"
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" {
			return name + " must be a valid NATS URL"
		}
		switch strings.ToLower(parsed.Scheme) {
		case "nats", "tls", "ws", "wss":
		default:
			return name + " must use nats, tls, ws, or wss scheme"
		}
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
		return "sftpgo"
	}
	if strings.TrimSpace(dir) != "" {
		return "local"
	}
	return "memory"
}

// firstNonEmptyEnv returns the first non-empty environment value among keys.
func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

// isSFTPGoHTTPEndpoint reports whether the SFTPGo endpoint uses HTTP(S).
func isSFTPGoHTTPEndpoint(endpoint string) bool {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

// sftpgoEndpointProblem validates SFTPGo endpoint schemes.
func sftpgoEndpointProblem(endpoint string) string {
	value := strings.TrimSpace(endpoint)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "SFTPGO_BASE_URL must be a valid http(s) or sftp URL"
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "sftp":
		return ""
	default:
		return "SFTPGO_BASE_URL must use http, https, or sftp"
	}
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
