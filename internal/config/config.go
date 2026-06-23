package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config contains all environment-backed runtime settings for the API process.
type Config struct {
	Env      string
	HTTPAddr string
	SeedDemo bool
	LogLevel string

	DatabaseURL string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	KeycloakIssuerURL  string
	KeycloakClientID   string
	AllowDemoContext   bool
	AllowHeaderContext bool
	AllowUnsignedJWT   bool

	OpenFGAAPIURL  string
	OpenFGAStoreID string
	OpenFGAModelID string

	ObjectStoreDir string

	OTelEnabled              bool
	OTelServiceName          string
	OTelExporterOTLPEndpoint string
	OTelExporterOTLPInsecure bool
}

// ValidateStartup enforces fail-closed production defaults before the server starts.
func (c Config) ValidateStartup() error {
	if c.Env != "production" {
		return nil
	}
	problems := []string{}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		problems = append(problems, "DATABASE_URL is required")
	}
	if strings.TrimSpace(c.KeycloakIssuerURL) == "" {
		problems = append(problems, "KEYCLOAK_ISSUER_URL is required")
	}
	if strings.TrimSpace(c.KeycloakClientID) == "" {
		problems = append(problems, "KEYCLOAK_CLIENT_ID is required")
	}
	if strings.TrimSpace(c.OpenFGAAPIURL) == "" {
		problems = append(problems, "OPENFGA_API_URL is required")
	}
	if strings.TrimSpace(c.OpenFGAStoreID) == "" {
		problems = append(problems, "OPENFGA_STORE_ID is required")
	}
	if strings.TrimSpace(c.OpenFGAModelID) == "" {
		problems = append(problems, "OPENFGA_MODEL_ID is required")
	}
	if strings.TrimSpace(c.ObjectStoreDir) == "" {
		problems = append(problems, "OBJECT_STORE_DIR is required")
	}
	if c.SeedDemo {
		problems = append(problems, "SEED_DEMO must be false")
	}
	if c.AllowDemoContext {
		problems = append(problems, "ALLOW_DEMO_CONTEXT must be false")
	}
	if c.AllowHeaderContext {
		problems = append(problems, "ALLOW_HEADER_CONTEXT must be false")
	}
	if c.AllowUnsignedJWT {
		problems = append(problems, "ALLOW_UNSIGNED_JWT must be false")
	}
	if len(problems) > 0 {
		return fmt.Errorf("production configuration invalid: %s", strings.Join(problems, "; "))
	}
	return nil
}

// Load reads configuration and ignores validation errors for legacy callers.
func Load() Config {
	cfg, _ := LoadE()
	return cfg
}

// LoadE reads configuration and returns all environment parsing errors.
func LoadE() (Config, error) {
	appEnv := env("APP_ENV", "development")
	problems := []string{}
	cfg := Config{
		Env:           appEnv,
		HTTPAddr:      env("HTTP_ADDR", ":8080"),
		SeedDemo:      envBool("SEED_DEMO", appEnv != "production", &problems),
		LogLevel:      env("LOG_LEVEL", "info"),
		DatabaseURL:   strings.TrimSpace(os.Getenv("DATABASE_URL")),
		RedisAddr:     strings.TrimSpace(os.Getenv("REDIS_ADDR")),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       envInt("REDIS_DB", 0, &problems),

		KeycloakIssuerURL:  strings.TrimSpace(os.Getenv("KEYCLOAK_ISSUER_URL")),
		KeycloakClientID:   strings.TrimSpace(os.Getenv("KEYCLOAK_CLIENT_ID")),
		AllowDemoContext:   envBool("ALLOW_DEMO_CONTEXT", false, &problems),
		AllowHeaderContext: envBool("ALLOW_HEADER_CONTEXT", false, &problems),
		AllowUnsignedJWT:   envBool("ALLOW_UNSIGNED_JWT", false, &problems),

		OpenFGAAPIURL:  strings.TrimSpace(os.Getenv("OPENFGA_API_URL")),
		OpenFGAStoreID: strings.TrimSpace(os.Getenv("OPENFGA_STORE_ID")),
		OpenFGAModelID: strings.TrimSpace(os.Getenv("OPENFGA_MODEL_ID")),

		ObjectStoreDir: strings.TrimSpace(os.Getenv("OBJECT_STORE_DIR")),

		OTelEnabled:              envBool("OTEL_ENABLED", false, &problems),
		OTelServiceName:          env("OTEL_SERVICE_NAME", "nexus-pro-be"),
		OTelExporterOTLPEndpoint: env("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		OTelExporterOTLPInsecure: envBool("OTEL_EXPORTER_OTLP_INSECURE", true, &problems),
	}
	if len(problems) > 0 {
		return cfg, fmt.Errorf("configuration invalid: %s", strings.Join(problems, "; "))
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

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
