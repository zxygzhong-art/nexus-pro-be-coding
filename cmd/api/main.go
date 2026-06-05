// Command api is the nexus-pro backend HTTP server entrypoint. It wires config →
// db → repository → authz engine → adapters → handlers → gin router.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/adapters/authorizer"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/adapters/identity"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/audit"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/authz"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/config"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/db"
	hrhandler "git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/hr/handler"
	hrservice "git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/hr/service"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/iam/handler"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/iam/service"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/repository"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/server"
)

func main() {
	cfg := config.Load()

	gdb, err := db.Open(cfg.DBDsn)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	repo := repository.New()
	recorder := audit.NewRecorder(gdb)

	// Authorization backend: local engine over the repository, or OpenFGA (stub).
	var az authorizer.Authorizer = authz.NewLocalEngine(repo)
	if cfg.AuthzBackend == "openfga" {
		az = authorizer.NewOpenFGAAuthorizer(cfg.OpenFGAURL, cfg.OpenFGAStoreID, cfg.OpenFGAModelID)
	}

	// Identity: bearer (Keycloak, stub) first when enabled, then dev headers.
	var providers []identity.Provider
	if cfg.KeycloakEnabled {
		providers = append(providers, identity.NewKeycloakProvider(cfg.KeycloakIssuer, cfg.KeycloakJWKSURL, true))
	}
	providers = append(providers, identity.NewHeaderProvider())
	idp := identity.NewChain(providers...)

	identitySvc := service.NewIdentityService(az)
	assumeSvc := service.NewAssumableRoleService(repo, recorder)
	h := handler.New(repo, az, identitySvc, assumeSvc)
	hrH := hrhandler.New(hrservice.NewEmployeeService(), hrservice.NewOrgUnitService())

	srv := server.New(cfg.APIAddr, server.Deps{
		GDB:       gdb,
		Repo:      repo,
		Authz:     az,
		Recorder:  recorder,
		Identity:  idp,
		Handler:   h,
		HRHandler: hrH,
	})

	go func() {
		log.Printf("nexus-pro api listening on %s (authz=%s)", cfg.APIAddr, cfg.AuthzBackend)
		if err := srv.ListenAndServe(); err != nil && err.Error() != "http: Server closed" {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx, srv); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
