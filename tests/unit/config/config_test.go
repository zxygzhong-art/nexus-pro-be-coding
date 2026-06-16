package config_test

import (
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

func TestInvalidIntegerConfigPanics(t *testing.T) {
	t.Setenv("REDIS_DB", "not-a-number")

	defer func() {
		if recover() == nil {
			t.Fatal("expected invalid integer config to panic")
		}
	}()

	_ = config.Load()
}

func TestInvalidBooleanConfigPanics(t *testing.T) {
	t.Setenv("ALLOW_UNSIGNED_JWT", "maybe")

	defer func() {
		if recover() == nil {
			t.Fatal("expected invalid boolean config to panic")
		}
	}()

	_ = config.Load()
}
