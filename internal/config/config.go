// Package config loads runtime configuration from environment variables with
// sensible defaults so that `make run` works against a local Postgres only.
package config

import (
	"os"
	"strconv"
)

// Config holds all runtime configuration. Defaults are chosen so the existing
// platform-ui frontend (port 8088, X-Account-ID default) keeps working unchanged.
type Config struct {
	APIAddr string

	DBDsn      string
	MigrateDsn string
	RedisURL   string

	AuthzBackend string // "local" | "openfga"

	OpenFGAURL     string
	OpenFGAStoreID string
	OpenFGAModelID string

	KeycloakEnabled bool
	KeycloakIssuer  string
	KeycloakJWKSURL string

	LogLevel string
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	c := Config{
		APIAddr:         env("API_ADDR", ":8088"),
		DBDsn:           env("DB_DSN", "postgres://app_user:app_pass@localhost:5432/nexus?sslmode=disable"),
		MigrateDsn:      env("MIGRATE_DSN", ""),
		RedisURL:        env("REDIS_URL", ""),
		AuthzBackend:    env("AUTHZ_BACKEND", "local"),
		OpenFGAURL:      env("OPENFGA_API_URL", ""),
		OpenFGAStoreID:  env("OPENFGA_STORE_ID", ""),
		OpenFGAModelID:  env("OPENFGA_MODEL_ID", ""),
		KeycloakEnabled: envBool("KEYCLOAK_ENABLED", false),
		KeycloakIssuer:  env("KEYCLOAK_ISSUER", ""),
		KeycloakJWKSURL: env("KEYCLOAK_JWKS_URL", ""),
		LogLevel:        env("LOG_LEVEL", "info"),
	}
	if c.MigrateDsn == "" {
		c.MigrateDsn = c.DBDsn
	}
	return c
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
