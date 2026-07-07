package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	v1api "nexus-pro-be/internal/api/v1"
	"nexus-pro-be/internal/config"
	"nexus-pro-be/internal/jobs"
	platformauth "nexus-pro-be/internal/platform/auth"
	"nexus-pro-be/internal/platform/ehrms"
	"nexus-pro-be/internal/platform/objectstore"
	openfgaclient "nexus-pro-be/internal/platform/openfga"
	"nexus-pro-be/internal/platform/postgres"
	redisstore "nexus-pro-be/internal/platform/redis"
	"nexus-pro-be/internal/platform/telemetry"
	"nexus-pro-be/internal/repository"
	pgstore "nexus-pro-be/internal/repository/postgres"
	"nexus-pro-be/internal/service"
	"nexus-pro-be/internal/startup"

	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type apiRuntime struct {
	server                     *http.Server
	metricsServer              *http.Server
	report                     startup.Report
	store                      repository.Store
	relationshipWriter         jobs.RelationshipTupleWriter
	ehrmsSyncScheduler         *jobs.EHRMSEmployeeSyncScheduler
	ehrmsSyncOptions           jobs.EHRMSEmployeeSyncOptions
	identityProvisioningOutbox *jobs.IdentityProvisioningOutboxProcessor
	shutdowns                  []moduleShutdown
	workers                    sync.WaitGroup
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
	client       *goredis.Client
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

// startModules 啟動模組。
func startModules(ctx context.Context, cfg config.Config, logger *slog.Logger) (*apiRuntime, error) {
	report := startup.Report{
		Name:       "nexus-pro-be",
		Env:        cfg.Env,
		HTTPAddr:   cfg.HTTPAddr,
		Repository: "postgresql",
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
		return nil, errors.New("postgres repository is required")
	}

	authHTTPClient := &http.Client{Timeout: 5 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)}
	serviceOptions := service.Options{
		Logger:        logger,
		AuthzSnapshot: authzSnapshotModule.cache,
		Relationships: relationshipModule.checker,
		ObjectStore:   objectStoreModule.store,
	}
	tokenResolvers := make([]platformauth.TokenResolver, 0, 2)

	identityProvisioner, keycloakAdminReadiness, keycloakAdminDependency, err := configuredKeycloakProvisioner(cfg, authHTTPClient, logger)
	if err != nil {
		shutdownStartedModules(shutdowns, logger)
		return nil, err
	}
	report.Dependencies = append(report.Dependencies, keycloakAdminDependency)
	if identityProvisioner != nil {
		serviceOptions.IdentityProvisioner = identityProvisioner
		readinessChecks["keycloak_admin"] = keycloakAdminReadiness
	}

	ehrmsClient, ehrmsDependency, err := configuredEHRMSClient(cfg, authHTTPClient)
	if err != nil {
		shutdownStartedModules(shutdowns, logger)
		return nil, err
	}
	report.Dependencies = append(report.Dependencies, ehrmsDependency)
	serviceOptions.EHRMSClient = ehrmsClient

	app := service.New(store, serviceOptions)
	if err := app.SyncPermissionCatalogForAllTenants(ctx); err != nil {
		shutdownStartedModules(shutdowns, logger)
		return nil, err
	}
	ehrmsSyncScheduler, ehrmsSyncOptions, ehrmsSyncDependency := configuredEHRMSSyncScheduler(cfg, app.HR(), ehrmsClient != nil, logger)
	report.Dependencies = append(report.Dependencies, ehrmsSyncDependency)
	apiOptions := v1api.Options{
		DisableApprovalHeader: cfg.Env == "production",
		ReadinessChecks:       readinessChecks,
		CORSAllowedOrigins:    cfg.CORSAllowedOrigins,
		TrustedProxies:        cfg.TrustedProxies,
		RateLimiter:           configuredRateLimiter(cfg, authzSnapshotModule.client, logger),
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
	}
	report.Dependencies = append(report.Dependencies, telemetryStatus)

	apiInstance := v1api.New(app, logger, apiOptions)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiInstance.Routes(),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	runtime := &apiRuntime{
		server:             server,
		report:             report,
		store:              store,
		relationshipWriter: relationshipModule.writer,
		ehrmsSyncScheduler: ehrmsSyncScheduler,
		ehrmsSyncOptions:   ehrmsSyncOptions,
		shutdowns:          shutdowns,
	}
	if identityProvisioner != nil {
		// 重試 fast path 失敗後仍停留在佇列中的 Keycloak provisioning。
		runtime.identityProvisioningOutbox = jobs.NewIdentityProvisioningOutboxProcessor(store, app, logger)
	}
	if cfg.MetricsAddr != "" {
		// /metrics 使用獨立 listener，避免 scrape endpoint 暴露在業務連接埠上。
		// 它會重用 API instance 的 registry。
		// 指標收集中介層仍保留在業務 router 上。
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", apiInstance.MetricsHandler())
		runtime.metricsServer = &http.Server{
			Addr:              cfg.MetricsAddr,
			Handler:           metricsMux,
			ReadTimeout:       15 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
	}
	return runtime, nil
}

// startTelemetryModule 啟動遙測模組。
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

// startRepositoryModule 啟動repository 模組。
func startRepositoryModule(ctx context.Context, cfg config.Config, logger *slog.Logger) (repositoryModule, error) {
	result := repositoryModule{
		name:      "postgresql",
		readiness: map[string]v1api.ReadinessCheck{},
	}
	if cfg.DatabaseURL == "" {
		return result, errors.New("DATABASE_URL is required")
	}

	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := postgres.OpenPool(startupCtx, cfg.DatabaseURL, postgres.PoolOptions{
		MaxConns:        cfg.DBMaxConns,
		MinConns:        cfg.DBMinConns,
		MaxConnLifetime: cfg.DBMaxConnLifetime,
	})
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

// startAuthzSnapshotModule 啟動授權快照模組。
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
	result.client = redisClient
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

// startRelationshipModule 啟動關係模組。
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

// startObjectStoreModule 啟動物件儲存層模組。
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

// configureKeycloakModule 組態化Keycloak 模組。
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

// configuredKeycloakProvisioner 處理 configured Keycloak provisioner。
func configuredKeycloakProvisioner(cfg config.Config, client *http.Client, logger *slog.Logger) (service.IdentityProvisioner, v1api.ReadinessCheck, startup.Dependency, error) {
	if !cfg.KeycloakProvisionUsers {
		return nil, nil, startup.Dependency{
			Name:   "Keycloak Admin",
			Status: "skipped",
			Target: "KEYCLOAK_PROVISION_USERS=false",
			Detail: "user provisioning disabled",
		}, nil
	}
	missing := []string{}
	if strings.TrimSpace(cfg.KeycloakIssuerURL) == "" {
		missing = append(missing, "KEYCLOAK_ISSUER_URL")
	}
	if strings.TrimSpace(cfg.KeycloakAdminClientID) == "" {
		missing = append(missing, "KEYCLOAK_ADMIN_CLIENT_ID")
	}
	if strings.TrimSpace(cfg.KeycloakAdminClientSecret) == "" {
		missing = append(missing, "KEYCLOAK_ADMIN_CLIENT_SECRET")
	}
	if len(missing) > 0 {
		dependency := startup.Dependency{Name: "Keycloak Admin", Status: "incomplete", Target: "disabled", Detail: startup.Missing(missing...)}
		return nil, nil, dependency, errors.New("keycloak admin provisioning is enabled but incomplete")
	}
	provisioner, err := platformauth.NewKeycloakAdminClient(platformauth.KeycloakAdminConfig{
		IssuerURL:         cfg.KeycloakIssuerURL,
		ClientID:          cfg.KeycloakAdminClientID,
		ClientSecret:      cfg.KeycloakAdminClientSecret,
		SendInviteEmail:   cfg.KeycloakSendInviteEmail,
		InviteClientID:    cfg.KeycloakInviteClientID,
		InviteRedirectURL: cfg.KeycloakInviteRedirectURL,
	}, client)
	if err != nil {
		dependency := startup.Dependency{Name: "Keycloak Admin", Status: "invalid", Target: startup.SafeURL(cfg.KeycloakIssuerURL), Detail: err.Error()}
		return nil, nil, dependency, err
	}
	logger.Info("keycloak admin provisioning enabled", "issuer", cfg.KeycloakIssuerURL, "client_id", cfg.KeycloakAdminClientID, "send_invite_email", cfg.KeycloakSendInviteEmail)
	return provisioner, provisioner.Ping, startup.Dependency{
		Name:   "Keycloak Admin",
		Status: "configured",
		Target: startup.SafeURL(cfg.KeycloakIssuerURL),
		Detail: "client=" + cfg.KeycloakAdminClientID,
	}, nil
}

// configuredRateLimiter 處理 configured 速率限流器。
func configuredRateLimiter(cfg config.Config, redisClient *goredis.Client, logger *slog.Logger) v1api.RateLimiter {
	if !cfg.RateLimitEnabled {
		return nil
	}
	if redisClient != nil {
		logger.Info("redis rate limiter enabled", "rps", cfg.RateLimitRPS, "burst", cfg.RateLimitBurst)
		return redisstore.NewFixedWindowRateLimiter(redisClient, cfg.RateLimitRPS, cfg.RateLimitBurst)
	}
	logger.Info("in-process rate limiter enabled", "rps", cfg.RateLimitRPS, "burst", cfg.RateLimitBurst)
	return v1api.NewLocalRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst)
}

// configuredEHRMSClient 處理 configured eHRMS client。
func configuredEHRMSClient(cfg config.Config, client *http.Client) (service.EHRMSClient, startup.Dependency, error) {
	if strings.TrimSpace(cfg.EHRMSBaseURL) == "" && strings.TrimSpace(cfg.EHRMSAPIKey) == "" {
		return nil, startup.Dependency{Name: "eHRMS", Status: "skipped", Target: "EHRMS_* not set", Detail: "employee sync disabled"}, nil
	}
	missing := []string{}
	if strings.TrimSpace(cfg.EHRMSBaseURL) == "" {
		missing = append(missing, "EHRMS_BASE_URL")
	}
	if strings.TrimSpace(cfg.EHRMSAPIKey) == "" {
		missing = append(missing, "EHRMS_API_KEY")
	}
	if len(missing) > 0 {
		return nil, startup.Dependency{Name: "eHRMS", Status: "incomplete", Target: "disabled", Detail: startup.Missing(missing...)}, nil
	}
	clientAdapter, err := ehrms.NewClient(cfg.EHRMSBaseURL, cfg.EHRMSAPIKey, client)
	if err != nil {
		return nil, startup.Dependency{Name: "eHRMS", Status: "invalid", Target: startup.SafeURL(cfg.EHRMSBaseURL), Detail: err.Error()}, err
	}
	return clientAdapter, startup.Dependency{Name: "eHRMS", Status: "configured", Target: startup.SafeURL(cfg.EHRMSBaseURL), Detail: "employee sync enabled"}, nil
}

// configuredEHRMSSyncScheduler 處理 configured eHRMS sync scheduler。
func configuredEHRMSSyncScheduler(cfg config.Config, svc jobs.EHRMSEmployeeSyncService, ehrmsConfigured bool, logger *slog.Logger) (*jobs.EHRMSEmployeeSyncScheduler, jobs.EHRMSEmployeeSyncOptions, startup.Dependency) {
	opts := jobs.EHRMSEmployeeSyncOptions{
		Interval:   cfg.EHRMSSyncInterval,
		Mode:       cfg.EHRMSSyncMode,
		TenantID:   cfg.EHRMSSyncTenantID,
		AccountID:  cfg.EHRMSSyncAccountID,
		RunOnStart: cfg.EHRMSSyncRunOnStart,
	}
	if !cfg.EHRMSSyncEnabled {
		return nil, opts, startup.Dependency{Name: "eHRMS Scheduler", Status: "skipped", Target: "EHRMS_SYNC_ENABLED=false", Detail: "periodic employee sync disabled"}
	}
	if !ehrmsConfigured {
		return nil, opts, startup.Dependency{Name: "eHRMS Scheduler", Status: "incomplete", Target: "disabled", Detail: "eHRMS upstream is not configured"}
	}
	detail := "interval=" + cfg.EHRMSSyncInterval.String() + " mode=" + strings.TrimSpace(cfg.EHRMSSyncMode)
	if strings.TrimSpace(cfg.EHRMSSyncMode) == "" {
		detail = "interval=" + cfg.EHRMSSyncInterval.String() + " mode=upsert"
	}
	return jobs.NewEHRMSEmployeeSyncScheduler(svc, logger), opts, startup.Dependency{
		Name:   "eHRMS Scheduler",
		Status: "configured",
		Target: "tenant=" + cfg.EHRMSSyncTenantID + " account=" + cfg.EHRMSSyncAccountID,
		Detail: detail,
	}
}

// startBackgroundWorkers 啟動background worker。
func (r *apiRuntime) startBackgroundWorkers(ctx context.Context, logger *slog.Logger) {
	if r.relationshipWriter != nil {
		// 只有在 OpenFGA 已完整設定且可接受寫入時，outbox dispatcher 才會執行。
		dispatcher := jobs.NewOutboxDispatcher(r.store, r.relationshipWriter, logger)
		r.workers.Add(1)
		go func() {
			defer r.workers.Done()
			dispatcher.Run(ctx, jobs.OutboxDispatchOptions{})
		}()
		logger.Info("outbox dispatcher started")
	}
	if r.ehrmsSyncScheduler != nil {
		r.workers.Add(1)
		go func() {
			defer r.workers.Done()
			r.ehrmsSyncScheduler.Run(ctx, r.ehrmsSyncOptions)
		}()
		logger.Info("eHRMS employee sync scheduler started", "interval", r.ehrmsSyncOptions.Interval.String(), "mode", r.ehrmsSyncOptions.Mode, "tenant_id", r.ehrmsSyncOptions.TenantID, "account_id", r.ehrmsSyncOptions.AccountID)
	}
	if r.identityProvisioningOutbox != nil {
		r.workers.Add(1)
		go func() {
			defer r.workers.Done()
			r.identityProvisioningOutbox.Run(ctx, jobs.IdentityProvisioningOutboxOptions{})
		}()
		logger.Info("identity provisioning outbox worker started")
	}
}

// waitForBackgroundWorkers 處理 wait for background worker。
func (r *apiRuntime) waitForBackgroundWorkers(timeout time.Duration, logger *slog.Logger) {
	done := make(chan struct{})
	go func() {
		r.workers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		logger.Warn("background workers did not stop before timeout", "timeout", timeout.String())
	}
}

// shutdown 關閉目前流程。
func (r *apiRuntime) shutdown(logger *slog.Logger) {
	shutdownStartedModules(r.shutdowns, logger)
}

// appendModuleShutdown 附加模組 shutdown。
func appendModuleShutdown(shutdowns []moduleShutdown, shutdown moduleShutdown) []moduleShutdown {
	if shutdown.fn == nil {
		return shutdowns
	}
	return append(shutdowns, shutdown)
}

// mergeReadinessChecks 合併就緒檢查 checks。
func mergeReadinessChecks(dst map[string]v1api.ReadinessCheck, src map[string]v1api.ReadinessCheck) {
	for name, check := range src {
		dst[name] = check
	}
}

// shutdownStartedModules 關閉started 模組。
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
