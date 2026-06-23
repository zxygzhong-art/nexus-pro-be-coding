package main

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
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

type apiRuntime struct {
	server             *http.Server
	report             startup.Report
	store              repository.Store
	relationshipWriter jobs.RelationshipTupleWriter
	shutdowns          []moduleShutdown
}

type moduleShutdown struct {
	name string
	fn   func(context.Context) error
}

type repositoryModule struct {
	store        repository.Store
	name         string
	dependencies []startup.Dependency
	readiness    map[string]v1api.ReadinessCheck
	shutdown     moduleShutdown
}

type authzSnapshotModule struct {
	cache        service.AuthzSnapshotCache
	dependencies []startup.Dependency
	readiness    map[string]v1api.ReadinessCheck
	shutdown     moduleShutdown
}

type relationshipModule struct {
	checker      service.RelationshipChecker
	writer       jobs.RelationshipTupleWriter
	dependencies []startup.Dependency
	readiness    map[string]v1api.ReadinessCheck
}

type objectStoreModule struct {
	store        service.ObjectStore
	dependencies []startup.Dependency
}

func startModules(ctx context.Context, cfg config.Config, logger *slog.Logger) (*apiRuntime, error) {
	report := startup.Report{
		Name:       "nexus-pro-be",
		Env:        cfg.Env,
		HTTPAddr:   cfg.HTTPAddr,
		Repository: "memory",
	}
	readinessChecks := map[string]v1api.ReadinessCheck{}
	shutdowns := []moduleShutdown{}

	telemetryStatus, telemetryShutdown, err := startTelemetryModule(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}
	shutdowns = append(shutdowns, moduleShutdown{name: "opentelemetry", fn: telemetryShutdown})

	repositoryModule, err := startRepositoryModule(ctx, cfg, logger)
	if err != nil {
		shutdownStartedModules(shutdowns, logger)
		return nil, err
	}
	report.Repository = repositoryModule.name
	report.Dependencies = append(report.Dependencies, repositoryModule.dependencies...)
	mergeReadinessChecks(readinessChecks, repositoryModule.readiness)
	shutdowns = appendModuleShutdown(shutdowns, repositoryModule.shutdown)

	authzSnapshotModule, err := startAuthzSnapshotModule(ctx, cfg, logger)
	if err != nil {
		shutdownStartedModules(shutdowns, logger)
		return nil, err
	}
	report.Dependencies = append(report.Dependencies, authzSnapshotModule.dependencies...)
	mergeReadinessChecks(readinessChecks, authzSnapshotModule.readiness)
	shutdowns = appendModuleShutdown(shutdowns, authzSnapshotModule.shutdown)

	relationshipModule := startRelationshipModule(cfg, logger)
	report.Dependencies = append(report.Dependencies, relationshipModule.dependencies...)
	mergeReadinessChecks(readinessChecks, relationshipModule.readiness)

	objectStoreModule, err := startObjectStoreModule(ctx, cfg, logger)
	if err != nil {
		shutdownStartedModules(shutdowns, logger)
		return nil, err
	}
	report.Dependencies = append(report.Dependencies, objectStoreModule.dependencies...)

	store := repositoryModule.store
	if store == nil {
		store = memory.NewStore()
	}
	if cfg.SeedDemo {
		service.SeedDemo(store)
		logger.Info("demo data seeded")
	}

	authHTTPClient := &http.Client{Timeout: 5 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)}
	serviceOptions := service.Options{
		Logger:        logger,
		AuthzSnapshot: authzSnapshotModule.cache,
		Relationships: relationshipModule.checker,
		ObjectStore:   objectStoreModule.store,
	}
	tokenResolvers := make([]platformauth.TokenResolver, 0, 2)

	oidcProviders, oidcDependencies := configuredOIDCProviders(cfg, authHTTPClient)
	report.Dependencies = append(report.Dependencies, oidcDependencies...)
	if len(oidcProviders) > 0 && strings.TrimSpace(cfg.AuthSessionSigningKey) != "" {
		stateKey := cfg.AuthStateSigningKey
		if strings.TrimSpace(stateKey) == "" {
			stateKey = cfg.AuthSessionSigningKey
		}
		serviceOptions.OIDCProviders = oidcProviders
		serviceOptions.AuthTokenIssuer = platformauth.NewInternalTokenIssuer(cfg.AuthSessionSigningKey, cfg.AuthTokenIssuer, cfg.AuthTokenAudience, 8*time.Hour)
		serviceOptions.AuthStateCodec = platformauth.NewOIDCStateCodec(stateKey, 10*time.Minute)
		tokenResolvers = append(tokenResolvers, platformauth.NewInternalTokenResolver(cfg.AuthSessionSigningKey, cfg.AuthTokenIssuer, cfg.AuthTokenAudience))
		logger.Info("OIDC login enabled", "providers", providerNames(oidcProviders))
	}

	app := service.New(store, serviceOptions)
	apiOptions := v1api.Options{
		AllowDemoContext:      cfg.AllowDemoContext,
		AllowHeaderContext:    cfg.AllowHeaderContext,
		AllowUnsignedJWT:      cfg.AllowUnsignedJWT,
		DisableApprovalHeader: cfg.Env == "production",
		ReadinessChecks:       readinessChecks,
	}

	keycloakResolver, keycloakReadiness, keycloakDependency := configureKeycloakModule(cfg, authHTTPClient, logger)
	if keycloakResolver != nil {
		tokenResolvers = append(tokenResolvers, keycloakResolver)
		readinessChecks["keycloak"] = keycloakReadiness
	}
	report.Dependencies = append(report.Dependencies, keycloakDependency)

	if cfg.OTelEnabled {
		apiOptions.TelemetryServiceName = cfg.OTelServiceName
	}
	if len(tokenResolvers) == 1 {
		apiOptions.TokenResolver = tokenResolvers[0]
	} else if len(tokenResolvers) > 1 {
		apiOptions.TokenResolver = platformauth.NewTokenResolverChain(tokenResolvers...)
	}
	report.Dependencies = append(report.Dependencies, telemetryStatus)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           v1api.New(app, logger, apiOptions).Routes(),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &apiRuntime{
		server:             server,
		report:             report,
		store:              store,
		relationshipWriter: relationshipModule.writer,
		shutdowns:          shutdowns,
	}, nil
}

func startTelemetryModule(ctx context.Context, cfg config.Config, logger *slog.Logger) (startup.Dependency, func(context.Context) error, error) {
	dependency := startup.Dependency{
		Name:   "OpenTelemetry",
		Status: "skipped",
		Target: "OTEL_ENABLED=false",
		Detail: "tracing disabled",
	}
	shutdown, err := telemetry.Init(ctx, telemetry.Config{
		Enabled:  cfg.OTelEnabled,
		Service:  cfg.OTelServiceName,
		Endpoint: cfg.OTelExporterOTLPEndpoint,
		Insecure: cfg.OTelExporterOTLPInsecure,
		Env:      cfg.Env,
	})
	if err != nil {
		logger.Error("opentelemetry initialization failed", "error", err)
		return dependency, nil, err
	}
	if cfg.OTelEnabled {
		logger.Info("opentelemetry tracing enabled", "service", cfg.OTelServiceName, "endpoint", cfg.OTelExporterOTLPEndpoint)
		dependency = startup.Dependency{
			Name:   "OpenTelemetry",
			Status: "enabled",
			Target: cfg.OTelExporterOTLPEndpoint,
			Detail: "service=" + cfg.OTelServiceName,
		}
	}
	return dependency, shutdown, nil
}

func startRepositoryModule(ctx context.Context, cfg config.Config, logger *slog.Logger) (repositoryModule, error) {
	result := repositoryModule{
		name:      "memory",
		readiness: map[string]v1api.ReadinessCheck{},
		dependencies: []startup.Dependency{{
			Name:   "PostgreSQL",
			Status: "skipped",
			Target: "DATABASE_URL not set",
			Detail: "using in-memory store",
		}},
	}
	if cfg.DatabaseURL == "" {
		return result, nil
	}

	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := postgres.OpenPool(startupCtx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		logger.Error("postgres connection failed", "error", err)
		return result, err
	}
	logger.Info("postgres connected")
	result.store = pgstore.NewStore(pool)
	result.name = "postgresql"
	result.dependencies = []startup.Dependency{{
		Name:   "PostgreSQL",
		Status: "connected",
		Target: startup.SafeURL(cfg.DatabaseURL),
		Detail: "repository backend",
	}}
	result.readiness["postgres"] = pool.Ping
	result.shutdown = moduleShutdown{name: "postgres", fn: func(context.Context) error {
		pool.Close()
		return nil
	}}
	return result, nil
}

func startAuthzSnapshotModule(ctx context.Context, cfg config.Config, logger *slog.Logger) (authzSnapshotModule, error) {
	result := authzSnapshotModule{
		readiness: map[string]v1api.ReadinessCheck{},
		dependencies: []startup.Dependency{{
			Name:   "Redis",
			Status: "skipped",
			Target: "REDIS_ADDR not set",
			Detail: "authz snapshots disabled",
		}},
	}
	if cfg.RedisAddr == "" {
		return result, nil
	}

	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	redisClient, err := redisstore.OpenClient(startupCtx, redisstore.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	cancel()
	if err != nil {
		logger.Error("redis connection failed", "error", err)
		return result, err
	}
	logger.Info("redis connected")
	result.cache = redisstore.NewAuthzSnapshotStore(redisClient)
	result.dependencies = []startup.Dependency{{
		Name:   "Redis",
		Status: "connected",
		Target: cfg.RedisAddr,
		Detail: "db=" + strconv.Itoa(cfg.RedisDB),
	}}
	result.readiness["redis"] = func(ctx context.Context) error {
		return redisClient.Ping(ctx).Err()
	}
	result.shutdown = moduleShutdown{name: "redis", fn: func(context.Context) error {
		return redisClient.Close()
	}}
	return result, nil
}

func startRelationshipModule(cfg config.Config, logger *slog.Logger) relationshipModule {
	result := relationshipModule{
		readiness: map[string]v1api.ReadinessCheck{},
	}
	if cfg.OpenFGAAPIURL != "" && cfg.OpenFGAStoreID != "" && cfg.OpenFGAModelID != "" {
		openfgaClient := openfgaclient.NewChecker(cfg.OpenFGAAPIURL, cfg.OpenFGAStoreID, &http.Client{
			Timeout:   5 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		}).WithAuthorizationModelID(cfg.OpenFGAModelID)
		result.checker = openfgaClient
		result.writer = openfgaClient
		result.readiness["openfga"] = openfgaClient.Ping
		logger.Info("openfga relationship checker enabled", "api_url", cfg.OpenFGAAPIURL, "store_id", cfg.OpenFGAStoreID, "model_id", cfg.OpenFGAModelID)
		result.dependencies = append(result.dependencies, startup.Dependency{
			Name:   "OpenFGA",
			Status: "configured",
			Target: startup.SafeURL(cfg.OpenFGAAPIURL),
			Detail: "store=" + cfg.OpenFGAStoreID + " model=" + cfg.OpenFGAModelID,
		})
		return result
	}
	if cfg.OpenFGAAPIURL != "" || cfg.OpenFGAStoreID != "" || cfg.OpenFGAModelID != "" {
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
		result.dependencies = append(result.dependencies, startup.Dependency{
			Name:   "OpenFGA",
			Status: "incomplete",
			Target: "disabled",
			Detail: startup.Missing(missing...),
		})
		return result
	}
	result.dependencies = append(result.dependencies, startup.Dependency{
		Name:   "OpenFGA",
		Status: "skipped",
		Target: "OPENFGA_* not set",
		Detail: "relationship checks off",
	})
	return result
}

func startObjectStoreModule(ctx context.Context, cfg config.Config, logger *slog.Logger) (objectStoreModule, error) {
	result := objectStoreModule{}
	switch cfg.ObjectStoreProvider {
	case "local":
		objectStore, err := objectstore.NewLocal(cfg.ObjectStoreDir)
		if err != nil {
			logger.Error("object store initialization failed", "error", err)
			return result, err
		}
		logger.Info("local object store enabled", "dir", cfg.ObjectStoreDir)
		result.store = objectStore
		result.dependencies = append(result.dependencies, startup.Dependency{
			Name:   "ObjectStore",
			Status: "local",
			Target: cfg.ObjectStoreDir,
			Detail: "ready",
		})
	case "minio", "s3":
		startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		objectStore, err := objectstore.NewMinIO(startupCtx, objectstore.MinIOOptions{
			Provider:        cfg.ObjectStoreProvider,
			Endpoint:        cfg.ObjectStoreEndpoint,
			Bucket:          cfg.ObjectStoreBucket,
			AccessKeyID:     cfg.ObjectStoreAccessKeyID,
			SecretAccessKey: cfg.ObjectStoreSecretAccessKey,
			Region:          cfg.ObjectStoreRegion,
			UseSSL:          cfg.ObjectStoreUseSSL,
			CreateBucket:    cfg.ObjectStoreCreateBucket,
		})
		cancel()
		if err != nil {
			logger.Error("object store initialization failed", "provider", cfg.ObjectStoreProvider, "error", err)
			return result, err
		}
		logger.Info("s3-compatible object store enabled", "provider", cfg.ObjectStoreProvider, "endpoint", startup.SafeURL(cfg.ObjectStoreEndpoint), "bucket", cfg.ObjectStoreBucket)
		result.store = objectStore
		result.dependencies = append(result.dependencies, startup.Dependency{
			Name:   "ObjectStore",
			Status: cfg.ObjectStoreProvider,
			Target: startup.SafeURL(cfg.ObjectStoreEndpoint),
			Detail: "bucket=" + cfg.ObjectStoreBucket,
		})
	default:
		result.dependencies = append(result.dependencies, startup.Dependency{
			Name:   "ObjectStore",
			Status: "memory",
			Target: "OBJECT_STORE_PROVIDER=memory",
			Detail: "import files in memory",
		})
	}
	return result, nil
}

func configureKeycloakModule(cfg config.Config, client *http.Client, logger *slog.Logger) (platformauth.TokenResolver, v1api.ReadinessCheck, startup.Dependency) {
	if cfg.KeycloakIssuerURL != "" && cfg.KeycloakClientID != "" {
		keycloakResolver := platformauth.NewKeycloakTokenResolver(cfg.KeycloakIssuerURL, cfg.KeycloakClientID, client)
		logger.Info("keycloak token resolver enabled", "issuer", cfg.KeycloakIssuerURL, "client_id", cfg.KeycloakClientID)
		return keycloakResolver, keycloakResolver.Ping, startup.Dependency{
			Name:   "Keycloak",
			Status: "configured",
			Target: startup.SafeURL(cfg.KeycloakIssuerURL),
			Detail: "client=" + cfg.KeycloakClientID,
		}
	}
	if cfg.KeycloakIssuerURL != "" || cfg.KeycloakClientID != "" {
		missing := []string{}
		if cfg.KeycloakIssuerURL == "" {
			missing = append(missing, "KEYCLOAK_ISSUER_URL")
		}
		if cfg.KeycloakClientID == "" {
			missing = append(missing, "KEYCLOAK_CLIENT_ID")
		}
		return nil, nil, startup.Dependency{
			Name:   "Keycloak",
			Status: "incomplete",
			Target: "disabled",
			Detail: startup.Missing(missing...),
		}
	}
	return nil, nil, startup.Dependency{
		Name:   "Keycloak",
		Status: "skipped",
		Target: "KEYCLOAK_* not set",
		Detail: "token resolver off",
	}
}

func configuredOIDCProviders(cfg config.Config, client *http.Client) (map[string]service.OIDCProvider, []startup.Dependency) {
	providers := map[string]service.OIDCProvider{}
	deps := []startup.Dependency{}
	add := func(code, display, issuer, clientID, clientSecret, redirectURL string) {
		missing := oidcMissing(clientID, clientSecret, redirectURL)
		if len(missing) == 3 {
			deps = append(deps, startup.Dependency{Name: display, Status: "skipped", Target: code, Detail: "OIDC login disabled"})
			return
		}
		if strings.TrimSpace(cfg.AuthSessionSigningKey) == "" {
			missing = append(missing, "AUTH_SESSION_SIGNING_KEY")
		}
		if len(missing) > 0 {
			deps = append(deps, startup.Dependency{Name: display, Status: "incomplete", Target: startup.SafeURL(issuer), Detail: startup.Missing(missing...)})
			return
		}
		providers[code] = platformauth.NewOIDCProvider(platformauth.OIDCProviderConfig{
			Code:         code,
			IssuerURL:    issuer,
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
		}, client)
		deps = append(deps, startup.Dependency{Name: display, Status: "configured", Target: startup.SafeURL(issuer), Detail: "client=" + clientID})
	}
	add("google", "Google OIDC", cfg.GoogleOIDCIssuerURL, cfg.GoogleOIDCClientID, cfg.GoogleOIDCClientSecret, cfg.GoogleOIDCRedirectURL)
	add("microsoft", "Microsoft OIDC", cfg.MicrosoftOIDCIssuerURL, cfg.MicrosoftOIDCClientID, cfg.MicrosoftOIDCClientSecret, cfg.MicrosoftOIDCRedirectURL)
	return providers, deps
}

func oidcMissing(clientID, clientSecret, redirectURL string) []string {
	missing := []string{}
	if strings.TrimSpace(clientID) == "" {
		missing = append(missing, "CLIENT_ID")
	}
	if strings.TrimSpace(clientSecret) == "" {
		missing = append(missing, "CLIENT_SECRET")
	}
	if strings.TrimSpace(redirectURL) == "" {
		missing = append(missing, "REDIRECT_URL")
	}
	return missing
}

func providerNames(providers map[string]service.OIDCProvider) []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}

func (r *apiRuntime) startBackgroundWorkers(ctx context.Context, logger *slog.Logger) {
	if r.relationshipWriter == nil {
		return
	}
	// The tuple outbox only runs when OpenFGA is fully configured to accept writes.
	processor := jobs.NewAuthzOutboxProcessor(r.store, r.relationshipWriter, logger)
	go processor.Run(ctx, jobs.AuthzOutboxOptions{})
	logger.Info("openfga outbox worker started")
}

func (r *apiRuntime) shutdown(logger *slog.Logger) {
	shutdownStartedModules(r.shutdowns, logger)
}

func appendModuleShutdown(shutdowns []moduleShutdown, shutdown moduleShutdown) []moduleShutdown {
	if shutdown.fn == nil {
		return shutdowns
	}
	return append(shutdowns, shutdown)
}

func mergeReadinessChecks(dst map[string]v1api.ReadinessCheck, src map[string]v1api.ReadinessCheck) {
	for name, check := range src {
		dst[name] = check
	}
}

func shutdownStartedModules(shutdowns []moduleShutdown, logger *slog.Logger) {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for idx := len(shutdowns) - 1; idx >= 0; idx-- {
		if shutdowns[idx].fn == nil {
			continue
		}
		if err := shutdowns[idx].fn(shutdownCtx); err != nil {
			logger.Error("module shutdown failed", "module", shutdowns[idx].name, "error", err)
		}
	}
}
