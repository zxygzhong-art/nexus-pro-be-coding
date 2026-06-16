package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	v1api "nexus-pro-be/internal/api/v1"
	"nexus-pro-be/internal/config"
	platformauth "nexus-pro-be/internal/platform/auth"
	openfgaclient "nexus-pro-be/internal/platform/openfga"
	"nexus-pro-be/internal/platform/postgres"
	redisstore "nexus-pro-be/internal/platform/redis"
	"nexus-pro-be/internal/platform/telemetry"
	"nexus-pro-be/internal/repository"
	"nexus-pro-be/internal/repository/memory"
	pgstore "nexus-pro-be/internal/repository/postgres"
	"nexus-pro-be/internal/service"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(cfg.LogLevel)}))
	telemetryShutdown, err := telemetry.Init(context.Background(), telemetry.Config{
		Enabled:  cfg.OTelEnabled,
		Service:  cfg.OTelServiceName,
		Endpoint: cfg.OTelExporterOTLPEndpoint,
		Insecure: cfg.OTelExporterOTLPInsecure,
		Env:      cfg.Env,
	})
	if err != nil {
		logger.Error("opentelemetry initialization failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetryShutdown(shutdownCtx); err != nil {
			logger.Error("opentelemetry shutdown failed", "error", err)
		}
	}()
	if cfg.OTelEnabled {
		logger.Info("opentelemetry tracing enabled", "service", cfg.OTelServiceName, "endpoint", cfg.OTelExporterOTLPEndpoint)
	}
	var store repository.Store
	var authzSnapshot service.AuthzSnapshotCache
	var relationships service.RelationshipChecker
	var objectStore service.ObjectStore
	readinessChecks := map[string]v1api.ReadinessCheck{}
	if cfg.DatabaseURL != "" {
		startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		pool, err := postgres.OpenPool(startupCtx, cfg.DatabaseURL)
		cancel()
		if err != nil {
			logger.Error("postgres connection failed", "error", err)
			os.Exit(1)
		}
		defer pool.Close()
		logger.Info("postgres connected")
		store = pgstore.NewStore(pool)
		readinessChecks["postgres"] = pool.Ping
	}
	if cfg.RedisAddr != "" {
		startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		redisClient, err := redisstore.OpenClient(startupCtx, redisstore.Options{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		})
		cancel()
		if err != nil {
			logger.Error("redis connection failed", "error", err)
			os.Exit(1)
		}
		defer redisClient.Close()
		logger.Info("redis connected")
		authzSnapshot = redisstore.NewAuthzSnapshotStore(redisClient)
		readinessChecks["redis"] = func(ctx context.Context) error {
			return redisClient.Ping(ctx).Err()
		}
	}
	if cfg.OpenFGAAPIURL != "" && cfg.OpenFGAStoreID != "" {
		relationships = openfgaclient.NewChecker(cfg.OpenFGAAPIURL, cfg.OpenFGAStoreID, &http.Client{
			Timeout:   5 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		})
		logger.Info("openfga relationship checker enabled", "api_url", cfg.OpenFGAAPIURL, "store_id", cfg.OpenFGAStoreID)
	}
	if cfg.ObjectStoreDir != "" {
		objectStore, err = service.NewLocalObjectStore(cfg.ObjectStoreDir)
		if err != nil {
			logger.Error("object store initialization failed", "error", err)
			os.Exit(1)
		}
		logger.Info("local object store enabled", "dir", cfg.ObjectStoreDir)
	}

	if store == nil {
		store = memory.NewStore()
	}
	if cfg.SeedDemo {
		service.SeedDemo(store)
		logger.Info("demo data seeded")
	}
	app := service.New(store, service.Options{AuthzSnapshot: authzSnapshot, Relationships: relationships, ObjectStore: objectStore})
	apiOptions := v1api.Options{
		AllowDemoContext:   cfg.AllowDemoContext,
		AllowHeaderContext: cfg.AllowHeaderContext,
		AllowUnsignedJWT:   cfg.AllowUnsignedJWT,
		ReadinessChecks:    readinessChecks,
	}
	if cfg.Env == "production" && (cfg.KeycloakIssuerURL == "" || cfg.KeycloakClientID == "") {
		logger.Error("production requires Keycloak OIDC configuration", "missing", "KEYCLOAK_ISSUER_URL or KEYCLOAK_CLIENT_ID")
		os.Exit(1)
	}
	if cfg.KeycloakIssuerURL != "" && cfg.KeycloakClientID != "" {
		apiOptions.TokenResolver = platformauth.NewKeycloakTokenResolver(cfg.KeycloakIssuerURL, cfg.KeycloakClientID, &http.Client{
			Timeout:   5 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		})
		logger.Info("keycloak token resolver enabled", "issuer", cfg.KeycloakIssuerURL, "client_id", cfg.KeycloakClientID)
	}
	if cfg.OTelEnabled {
		apiOptions.TelemetryServiceName = cfg.OTelServiceName
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           v1api.New(app, logger, apiOptions).Routes(),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		logger.Info("nexus-pro-be started", "addr", cfg.HTTPAddr)
		errs <- server.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", "error", err)
			os.Exit(1)
		}
	case err := <-errs:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}
}

func logLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
