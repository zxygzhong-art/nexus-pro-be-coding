package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	v1api "nexus-pro-be/internal/api/v1"
	"nexus-pro-be/internal/config"
	"nexus-pro-be/internal/jobs"
	platformauth "nexus-pro-be/internal/platform/auth"
	"nexus-pro-be/internal/platform/objectstore"
	openfgaclient "nexus-pro-be/internal/platform/openfga"
	"nexus-pro-be/internal/platform/postgres"
	redisstore "nexus-pro-be/internal/platform/redis"
	"nexus-pro-be/internal/platform/telemetry"
	"nexus-pro-be/internal/repository"
	"nexus-pro-be/internal/repository/memory"
	pgstore "nexus-pro-be/internal/repository/postgres"
	"nexus-pro-be/internal/service"
	"nexus-pro-be/internal/startup"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	cfg, err := config.LoadE()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(cfg.LogLevel)}))
	if err != nil {
		logger.Error("invalid startup configuration", "error", err)
		os.Exit(1)
	}
	if err := cfg.ValidateStartup(); err != nil {
		logger.Error("invalid startup configuration", "error", err)
		os.Exit(1)
	}
	startupReport := startup.Report{
		Name:       "nexus-pro-be",
		Env:        cfg.Env,
		HTTPAddr:   cfg.HTTPAddr,
		Repository: "memory",
	}
	telemetryStatus := startup.Dependency{
		Name:   "OpenTelemetry",
		Status: "skipped",
		Target: "OTEL_ENABLED=false",
		Detail: "tracing disabled",
	}
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
		telemetryStatus = startup.Dependency{
			Name:   "OpenTelemetry",
			Status: "enabled",
			Target: cfg.OTelExporterOTLPEndpoint,
			Detail: "service=" + cfg.OTelServiceName,
		}
	}
	var store repository.Store
	var authzSnapshot service.AuthzSnapshotCache
	var relationships service.RelationshipChecker
	var relationshipWriter jobs.RelationshipTupleWriter
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
		startupReport.Repository = "postgresql"
		startupReport.Dependencies = append(startupReport.Dependencies, startup.Dependency{
			Name:   "PostgreSQL",
			Status: "connected",
			Target: startup.SafeURL(cfg.DatabaseURL),
			Detail: "repository backend",
		})
		readinessChecks["postgres"] = pool.Ping
	} else {
		startupReport.Dependencies = append(startupReport.Dependencies, startup.Dependency{
			Name:   "PostgreSQL",
			Status: "skipped",
			Target: "DATABASE_URL not set",
			Detail: "using in-memory store",
		})
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
		startupReport.Dependencies = append(startupReport.Dependencies, startup.Dependency{
			Name:   "Redis",
			Status: "connected",
			Target: cfg.RedisAddr,
			Detail: "db=" + strconv.Itoa(cfg.RedisDB),
		})
		readinessChecks["redis"] = func(ctx context.Context) error {
			return redisClient.Ping(ctx).Err()
		}
	} else {
		startupReport.Dependencies = append(startupReport.Dependencies, startup.Dependency{
			Name:   "Redis",
			Status: "skipped",
			Target: "REDIS_ADDR not set",
			Detail: "authz snapshots disabled",
		})
	}
	if cfg.OpenFGAAPIURL != "" && cfg.OpenFGAStoreID != "" && cfg.OpenFGAModelID != "" {
		openfgaClient := openfgaclient.NewChecker(cfg.OpenFGAAPIURL, cfg.OpenFGAStoreID, &http.Client{
			Timeout:   5 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		}).WithAuthorizationModelID(cfg.OpenFGAModelID)
		relationships = openfgaClient
		relationshipWriter = openfgaClient
		readinessChecks["openfga"] = openfgaClient.Ping
		logger.Info("openfga relationship checker enabled", "api_url", cfg.OpenFGAAPIURL, "store_id", cfg.OpenFGAStoreID, "model_id", cfg.OpenFGAModelID)
		startupReport.Dependencies = append(startupReport.Dependencies, startup.Dependency{
			Name:   "OpenFGA",
			Status: "configured",
			Target: startup.SafeURL(cfg.OpenFGAAPIURL),
			Detail: "store=" + cfg.OpenFGAStoreID + " model=" + cfg.OpenFGAModelID,
		})
	} else if cfg.OpenFGAAPIURL != "" || cfg.OpenFGAStoreID != "" || cfg.OpenFGAModelID != "" {
		missing := []string{}
		if cfg.OpenFGAAPIURL == "" {
			missing = append(missing, "OPENFGA_API_URL")
		}
		if cfg.OpenFGAStoreID == "" {
			missing = append(missing, "OPENFGA_STORE_ID")
		}
		if cfg.OpenFGAModelID == "" {
			missing = append(missing, "OPENFGA_MODEL_ID")
		}
		startupReport.Dependencies = append(startupReport.Dependencies, startup.Dependency{
			Name:   "OpenFGA",
			Status: "incomplete",
			Target: "disabled",
			Detail: startup.Missing(missing...),
		})
	} else {
		startupReport.Dependencies = append(startupReport.Dependencies, startup.Dependency{
			Name:   "OpenFGA",
			Status: "skipped",
			Target: "OPENFGA_* not set",
			Detail: "relationship checks off",
		})
	}
	if cfg.ObjectStoreDir != "" {
		objectStore, err = objectstore.NewLocal(cfg.ObjectStoreDir)
		if err != nil {
			logger.Error("object store initialization failed", "error", err)
			os.Exit(1)
		}
		logger.Info("local object store enabled", "dir", cfg.ObjectStoreDir)
		startupReport.Dependencies = append(startupReport.Dependencies, startup.Dependency{
			Name:   "ObjectStore",
			Status: "local",
			Target: cfg.ObjectStoreDir,
			Detail: "ready",
		})
	} else {
		startupReport.Dependencies = append(startupReport.Dependencies, startup.Dependency{
			Name:   "ObjectStore",
			Status: "memory",
			Target: "OBJECT_STORE_DIR not set",
			Detail: "import files in memory",
		})
	}

	if store == nil {
		store = memory.NewStore()
	}
	if cfg.SeedDemo {
		service.SeedDemo(store)
		logger.Info("demo data seeded")
	}
	app := service.New(store, service.Options{Logger: logger, AuthzSnapshot: authzSnapshot, Relationships: relationships, ObjectStore: objectStore})
	apiOptions := v1api.Options{
		AllowDemoContext:      cfg.AllowDemoContext,
		AllowHeaderContext:    cfg.AllowHeaderContext,
		AllowUnsignedJWT:      cfg.AllowUnsignedJWT,
		DisableApprovalHeader: cfg.Env == "production",
		ReadinessChecks:       readinessChecks,
	}
	if cfg.KeycloakIssuerURL != "" && cfg.KeycloakClientID != "" {
		keycloakResolver := platformauth.NewKeycloakTokenResolver(cfg.KeycloakIssuerURL, cfg.KeycloakClientID, &http.Client{
			Timeout:   5 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		})
		apiOptions.TokenResolver = keycloakResolver
		readinessChecks["keycloak"] = keycloakResolver.Ping
		logger.Info("keycloak token resolver enabled", "issuer", cfg.KeycloakIssuerURL, "client_id", cfg.KeycloakClientID)
		startupReport.Dependencies = append(startupReport.Dependencies, startup.Dependency{
			Name:   "Keycloak",
			Status: "configured",
			Target: startup.SafeURL(cfg.KeycloakIssuerURL),
			Detail: "client=" + cfg.KeycloakClientID,
		})
	} else if cfg.KeycloakIssuerURL != "" || cfg.KeycloakClientID != "" {
		missing := []string{}
		if cfg.KeycloakIssuerURL == "" {
			missing = append(missing, "KEYCLOAK_ISSUER_URL")
		}
		if cfg.KeycloakClientID == "" {
			missing = append(missing, "KEYCLOAK_CLIENT_ID")
		}
		startupReport.Dependencies = append(startupReport.Dependencies, startup.Dependency{
			Name:   "Keycloak",
			Status: "incomplete",
			Target: "disabled",
			Detail: startup.Missing(missing...),
		})
	} else {
		startupReport.Dependencies = append(startupReport.Dependencies, startup.Dependency{
			Name:   "Keycloak",
			Status: "skipped",
			Target: "KEYCLOAK_* not set",
			Detail: "token resolver off",
		})
	}
	if cfg.OTelEnabled {
		apiOptions.TelemetryServiceName = cfg.OTelServiceName
	}
	startupReport.Dependencies = append(startupReport.Dependencies, telemetryStatus)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           v1api.New(app, logger, apiOptions).Routes(),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	if err := startup.Print(os.Stdout, startupReport); err != nil {
		logger.Warn("startup report render failed", "error", err)
	}

	errs := make(chan error, 1)
	go func() {
		logger.Info("nexus-pro-be started", "addr", cfg.HTTPAddr)
		errs <- server.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if relationshipWriter != nil {
		processor := jobs.NewAuthzOutboxProcessor(store, relationshipWriter, logger)
		go processor.Run(ctx, jobs.AuthzOutboxOptions{})
		logger.Info("openfga outbox worker started")
	}

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
