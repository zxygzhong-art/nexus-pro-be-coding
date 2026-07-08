package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/platform/auth"
	postgresplatform "nexus-pro-be/internal/platform/postgres"
	postgresrepo "nexus-pro-be/internal/repository/postgres"
	"nexus-pro-be/internal/service"
)

// main 執行 tenantctl。
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "tenantctl:", err)
		os.Exit(1)
	}
}

// run 解析 tenantctl 子命令。
func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printUsage(os.Stdout)
		return nil
	}
	switch args[0] {
	case "provision":
		return runProvision(args[1:])
	case "openfga-backfill":
		return runOpenFGABackfill(args[1:])
	case "openfga-grant-tenant-admin":
		return runOpenFGAGrantTenantAdmin(args[1:], false)
	case "openfga-grant-tenant-security-admin":
		return runOpenFGAGrantTenantAdmin(args[1:], true)
	case "openfga-grant-agent-tool":
		return runOpenFGAGrantAgentTool(args[1:])
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// runProvision 執行租戶開通。
func runProvision(args []string) error {
	fs := flag.NewFlagSet("tenantctl provision", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tenantID := fs.String("tenant-id", "", "tenant id written to tenant_id and token tenant claim")
	tenantName := fs.String("tenant-name", "", "tenant display name")
	adminEmail := fs.String("admin-email", "", "first administrator email")
	adminName := fs.String("admin-name", "", "first administrator display name")
	adminEmployeeNo := fs.String("admin-employee-no", "ADMIN001", "first administrator employee number")
	provider := fs.String("provider", domain.IdentityProviderKeycloak, "external identity provider")
	keycloakSub := fs.String("keycloak-sub", "", "existing Keycloak user id / OIDC subject")
	provisionKeycloak := fs.Bool("provision-keycloak", false, "ensure the admin user in Keycloak before writing backend records")
	sendInvite := fs.Bool("send-invite", false, "mark Keycloak user with UPDATE_PASSWORD required action")
	databaseURL := fs.String("database-url", strings.TrimSpace(os.Getenv("DATABASE_URL")), "Postgres database URL")
	timeout := fs.Duration("timeout", 30*time.Second, "operation timeout")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	input := service.TenantProvisionInput{
		TenantID:         *tenantID,
		TenantName:       *tenantName,
		AdminEmail:       *adminEmail,
		AdminName:        *adminName,
		AdminEmployeeNo:  *adminEmployeeNo,
		IdentityProvider: *provider,
		IdentitySubject:  *keycloakSub,
	}
	if strings.TrimSpace(*databaseURL) == "" {
		return errors.New("DATABASE_URL or --database-url is required")
	}
	if !*provisionKeycloak && strings.TrimSpace(input.IdentitySubject) == "" {
		return errors.New("--keycloak-sub or --provision-keycloak is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if *provisionKeycloak {
		if strings.TrimSpace(input.IdentitySubject) != "" {
			return errors.New("--provision-keycloak cannot be combined with --keycloak-sub")
		}
		identity, err := ensureKeycloakAdmin(ctx, input, *sendInvite)
		if err != nil {
			return err
		}
		input.IdentityProvider = identity.Provider
		input.IdentitySubject = identity.Subject
	}
	pool, err := postgresplatform.OpenPool(ctx, *databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	result, err := service.New(postgresrepo.NewStore(pool)).ProvisionTenant(ctx, input)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// runOpenFGAGrantTenantAdmin 執行 tenant admin/security_admin tuple 手工授權。
func runOpenFGAGrantTenantAdmin(args []string, securityAdmin bool) error {
	command := "tenantctl openfga-grant-tenant-admin"
	if securityAdmin {
		command = "tenantctl openfga-grant-tenant-security-admin"
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tenantID := fs.String("tenant-id", "", "tenant id")
	accountID := fs.String("account-id", "", "account id to grant")
	databaseURL := fs.String("database-url", strings.TrimSpace(os.Getenv("DATABASE_URL")), "Postgres database URL")
	timeout := fs.Duration("timeout", 30*time.Second, "operation timeout")
	dryRun := fs.Bool("dry-run", false, "compute the tuple without writing local tuples or outbox events")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	svc, logger, closeFn, err := tenantctlService(*databaseURL)
	if err != nil {
		return err
	}
	defer closeFn()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	input := service.OpenFGAGrantTenantAdminInput{
		TenantID:  *tenantID,
		AccountID: *accountID,
		DryRun:    *dryRun,
		Logger:    logger,
	}
	var result service.OpenFGAGrantRelationshipResult
	if securityAdmin {
		result, err = svc.OpenFGAGrantTenantSecurityAdmin(ctx, input)
	} else {
		result, err = svc.OpenFGAGrantTenantAdmin(ctx, input)
	}
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// runOpenFGAGrantAgentTool 執行 agent_tool runner/approver tuple 手工授權。
func runOpenFGAGrantAgentTool(args []string) error {
	fs := flag.NewFlagSet("tenantctl openfga-grant-agent-tool", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tenantID := fs.String("tenant-id", "", "tenant id")
	toolID := fs.String("tool-id", "", "agent tool id, for example knowledge.search")
	accountID := fs.String("account-id", "", "account id to grant")
	relation := fs.String("relation", "runner", "agent_tool relation: runner or approver")
	databaseURL := fs.String("database-url", strings.TrimSpace(os.Getenv("DATABASE_URL")), "Postgres database URL")
	timeout := fs.Duration("timeout", 30*time.Second, "operation timeout")
	dryRun := fs.Bool("dry-run", false, "compute the tuple without writing local tuples or outbox events")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	svc, logger, closeFn, err := tenantctlService(*databaseURL)
	if err != nil {
		return err
	}
	defer closeFn()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	result, err := svc.OpenFGAGrantAgentTool(ctx, service.OpenFGAGrantAgentToolInput{
		TenantID:  *tenantID,
		ToolID:    *toolID,
		AccountID: *accountID,
		Relation:  *relation,
		DryRun:    *dryRun,
		Logger:    logger,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// runOpenFGABackfill 執行 OpenFGA relationship tuple backfill。
func runOpenFGABackfill(args []string) error {
	fs := flag.NewFlagSet("tenantctl openfga-backfill", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tenantID := fs.String("tenant-id", "", "tenant id to backfill")
	databaseURL := fs.String("database-url", strings.TrimSpace(os.Getenv("DATABASE_URL")), "Postgres database URL")
	timeout := fs.Duration("timeout", 5*time.Minute, "operation timeout")
	dryRun := fs.Bool("dry-run", false, "compute missing tuples without writing local tuples or outbox events")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(*tenantID) == "" {
		return errors.New("--tenant-id is required")
	}
	if strings.TrimSpace(*databaseURL) == "" {
		return errors.New("DATABASE_URL or --database-url is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	pool, err := postgresplatform.OpenPool(ctx, *databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	result, err := service.New(postgresrepo.NewStore(pool), service.Options{Logger: logger}).OpenFGABackfillTuples(ctx, service.OpenFGABackfillInput{
		TenantID: *tenantID,
		DryRun:   *dryRun,
		Logger:   logger,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func tenantctlService(databaseURL string) (*service.Service, *slog.Logger, func(), error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, nil, nil, errors.New("DATABASE_URL or --database-url is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	pool, err := postgresplatform.OpenPool(ctx, databaseURL)
	cancel()
	if err != nil {
		return nil, nil, nil, err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	svc := service.New(postgresrepo.NewStore(pool), service.Options{Logger: logger})
	return svc, logger, pool.Close, nil
}

// ensureKeycloakAdmin 透過 Keycloak Admin API 建立或更新首管理員。
func ensureKeycloakAdmin(ctx context.Context, input service.TenantProvisionInput, sendInvite bool) (domain.ProvisionedIdentity, error) {
	if strings.TrimSpace(input.IdentityProvider) != "" && input.IdentityProvider != domain.IdentityProviderKeycloak {
		return domain.ProvisionedIdentity{}, errors.New("--provision-keycloak requires provider=keycloak")
	}
	ids, err := service.DefaultTenantProvisionIDs(input.TenantID)
	if err != nil {
		return domain.ProvisionedIdentity{}, err
	}
	cfg := auth.KeycloakAdminConfig{
		IssuerURL:         strings.TrimSpace(os.Getenv("KEYCLOAK_ISSUER_URL")),
		ClientID:          strings.TrimSpace(os.Getenv("KEYCLOAK_ADMIN_CLIENT_ID")),
		ClientSecret:      os.Getenv("KEYCLOAK_ADMIN_CLIENT_SECRET"),
		SendInviteEmail:   envBool("KEYCLOAK_SEND_INVITE_EMAIL"),
		InviteClientID:    firstNonEmpty(strings.TrimSpace(os.Getenv("KEYCLOAK_INVITE_CLIENT_ID")), strings.TrimSpace(os.Getenv("KEYCLOAK_CLIENT_ID"))),
		InviteRedirectURL: strings.TrimSpace(os.Getenv("KEYCLOAK_INVITE_REDIRECT_URL")),
	}
	client, err := auth.NewKeycloakAdminClient(cfg, &http.Client{Timeout: 10 * time.Second})
	if err != nil {
		return domain.ProvisionedIdentity{}, err
	}
	return client.EnsureUser(ctx, domain.IdentityProvisioningInput{
		TenantID:     strings.TrimSpace(input.TenantID),
		AccountID:    ids.AdminAccountID,
		EmployeeID:   ids.AdminEmployeeID,
		EmployeeNo:   strings.TrimSpace(input.AdminEmployeeNo),
		Email:        strings.TrimSpace(input.AdminEmail),
		DisplayName:  strings.TrimSpace(input.AdminName),
		Enabled:      true,
		SendInvite:   sendInvite,
		InviteClient: cfg.InviteClientID,
		InviteURL:    cfg.InviteRedirectURL,
	})
}

// printUsage 輸出 tenantctl 用法。
func printUsage(out *os.File) {
	fmt.Fprintln(out, `Usage:
  tenantctl provision --tenant-id <id> --tenant-name <name> --admin-email <email> --keycloak-sub <subject>
  tenantctl provision --tenant-id <id> --tenant-name <name> --admin-email <email> --provision-keycloak
  tenantctl openfga-backfill --tenant-id <id>
  tenantctl openfga-grant-tenant-admin --tenant-id <id> --account-id <account-id>
  tenantctl openfga-grant-tenant-security-admin --tenant-id <id> --account-id <account-id>
  tenantctl openfga-grant-agent-tool --tenant-id <id> --tool-id <tool-id> --account-id <account-id> [--relation runner|approver]

Required environment:
  DATABASE_URL

Required for --provision-keycloak:
  KEYCLOAK_ISSUER_URL
  KEYCLOAK_ADMIN_CLIENT_ID
  KEYCLOAK_ADMIN_CLIENT_SECRET

Optional:
  KEYCLOAK_SEND_INVITE_EMAIL=true
  KEYCLOAK_INVITE_CLIENT_ID
  KEYCLOAK_INVITE_REDIRECT_URL`)
}

// envBool 解析布林環境變數。
func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

// firstNonEmpty 回傳第一個非空字串。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
