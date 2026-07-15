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

	"nexus-pro-be/internal/config"
	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/jobs"
	"nexus-pro-be/internal/platform/auth"
	"nexus-pro-be/internal/platform/ehrms"
	openfgaclient "nexus-pro-be/internal/platform/openfga"
	postgresplatform "nexus-pro-be/internal/platform/postgres"
	temporalplatform "nexus-pro-be/internal/platform/temporal"
	postgresrepo "nexus-pro-be/internal/repository/postgres"
	"nexus-pro-be/internal/service"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	sdkclient "go.temporal.io/sdk/client"
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
	case "ensure-default-form-templates":
		return runEnsureDefaultFormTemplates(args[1:])
	case "openfga-backfill":
		return runOpenFGABackfill(args[1:])
	case "temporal-backfill-form-workflows":
		return runTemporalBackfillFormWorkflows(args[1:])
	case "openfga-grant-tenant-admin":
		return runOpenFGAGrantTenantAdmin(args[1:], false)
	case "openfga-grant-tenant-security-admin":
		return runOpenFGAGrantTenantAdmin(args[1:], true)
	case "openfga-grant-agent-tool":
		return runOpenFGAGrantAgentTool(args[1:])
	case "ehrms-sync":
		return runEHRMSSyncPipeline(args[1:])
	case "ehrms-sync-employees":
		return runEHRMSSyncEmployees(args[1:])
	case "ehrms-sync-attendance":
		return runEHRMSSyncAttendance(args[1:])
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// runEnsureDefaultFormTemplates 冪等補齊既有租戶缺少的內建表單模板。
func runEnsureDefaultFormTemplates(args []string) error {
	fs := flag.NewFlagSet("tenantctl ensure-default-form-templates", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tenantID := fs.String("tenant-id", "", "tenant id to backfill")
	databaseURL := fs.String("database-url", config.DatabaseURLFromEnv(), "Postgres database URL")
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
	if strings.TrimSpace(*tenantID) == "" {
		return errors.New("--tenant-id is required")
	}
	if strings.TrimSpace(*databaseURL) == "" {
		return errors.New("DB_HOST/DB_USERNAME/DB_NAME or --database-url is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	pool, err := postgresplatform.OpenPool(ctx, *databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	created, err := service.New(postgresrepo.NewStore(pool)).EnsureTenantDefaultFormTemplates(ctx, *tenantID)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"tenant_id": strings.TrimSpace(*tenantID), "created": created})
}

type temporalBackfillFormWorkflowsResult struct {
	TenantID string                                   `json:"tenant_id"`
	DryRun   bool                                     `json:"dry_run"`
	Scanned  int                                      `json:"scanned"`
	Started  int                                      `json:"started"`
	Skipped  int                                      `json:"skipped"`
	Failed   int                                      `json:"failed"`
	Items    []temporalBackfillFormWorkflowResultItem `json:"items"`
}

type temporalBackfillFormWorkflowResultItem struct {
	FormInstanceID string `json:"form_instance_id"`
	Status         string `json:"status"`
	WorkflowRunID  string `json:"workflow_run_id,omitempty"`
	Action         string `json:"action"`
	Error          string `json:"error,omitempty"`
}

// runTemporalBackfillFormWorkflows backfills Temporal executions for active form approvals.
func runTemporalBackfillFormWorkflows(args []string) error {
	fs := flag.NewFlagSet("tenantctl temporal-backfill-form-workflows", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tenantID := fs.String("tenant-id", "", "tenant id to backfill")
	databaseURL := fs.String("database-url", config.DatabaseURLFromEnv(), "Postgres database URL")
	temporalBaseURL := fs.String("temporal-base-url", firstNonEmpty(os.Getenv("TEMPORAL_BASE_URL"), "127.0.0.1:27233"), "Temporal base URL (host:port)")
	temporalNamespace := fs.String("temporal-namespace", firstNonEmpty(os.Getenv("TEMPORAL_NAMESPACE"), "default"), "Temporal namespace")
	temporalTaskQueue := fs.String("temporal-task-queue", firstNonEmpty(os.Getenv("TEMPORAL_TASK_QUEUE"), "nexus-workflows"), "Temporal task queue")
	timeout := fs.Duration("timeout", 5*time.Minute, "operation timeout")
	dryRun := fs.Bool("dry-run", false, "scan candidates without starting Temporal workflows")
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
		return errors.New("DB_HOST/DB_USERNAME/DB_NAME or --database-url is required")
	}
	if !*dryRun && strings.TrimSpace(*temporalBaseURL) == "" {
		return errors.New("TEMPORAL_BASE_URL or --temporal-base-url is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	pool, err := postgresplatform.OpenPool(ctx, *databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	store := postgresrepo.NewStore(pool)
	if _, ok, err := store.GetTenant(ctx, *tenantID); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("tenant %s not found", *tenantID)
	}

	result := temporalBackfillFormWorkflowsResult{TenantID: strings.TrimSpace(*tenantID), DryRun: *dryRun}
	items, err := store.ListFormInstances(ctx, result.TenantID)
	if err != nil {
		return err
	}

	var workflowClient temporalplatform.FormApprovalClient
	var temporalClient sdkclient.Client
	if !*dryRun {
		client, err := temporalplatform.Dial(ctx, temporalplatform.Config{
			HostPort:  *temporalBaseURL,
			Namespace: *temporalNamespace,
			TaskQueue: *temporalTaskQueue,
		})
		if err != nil {
			return err
		}
		temporalClient = client
		workflowClient = temporalplatform.NewFormApprovalClient(client, *temporalTaskQueue)
	}
	if temporalClient != nil {
		defer temporalClient.Close()
	}

	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			result.Skipped++
			result.Items = append(result.Items, temporalBackfillFormWorkflowResultItem{
				FormInstanceID: item.ID,
				Status:         item.Status,
				Action:         "skip_empty_form_instance_id",
			})
			continue
		}
		if !temporalBackfillCandidateStatus(item.Status) {
			result.Skipped++
			continue
		}
		result.Scanned++
		run, ok, err := store.GetWorkflowRunByFormInstance(ctx, result.TenantID, item.ID)
		if err != nil {
			result.Failed++
			result.Items = append(result.Items, temporalBackfillFormWorkflowResultItem{
				FormInstanceID: item.ID,
				Status:         item.Status,
				Action:         "error",
				Error:          err.Error(),
			})
			continue
		}
		if ok && temporalBackfillTerminalRunStatus(run.Status) {
			result.Skipped++
			result.Items = append(result.Items, temporalBackfillFormWorkflowResultItem{
				FormInstanceID: item.ID,
				Status:         item.Status,
				WorkflowRunID:  run.ID,
				Action:         "skip_terminal_run",
			})
			continue
		}
		resultItem := temporalBackfillFormWorkflowResultItem{
			FormInstanceID: item.ID,
			Status:         item.Status,
			Action:         "start",
		}
		start := domain.FormApprovalWorkflowStart{
			TenantID:                result.TenantID,
			FormInstanceID:          item.ID,
			DefaultRemindAfterHours: domain.DefaultFormApprovalRemindAfterHours,
		}
		if ok {
			start.RunID = run.ID
			start.StageDefinitionsJSON = run.StageDefinitionsJSON
			resultItem.WorkflowRunID = run.ID
		}
		if *dryRun {
			resultItem.Action = "would_start"
			result.Items = append(result.Items, resultItem)
			continue
		}
		active, err := temporalBackfillWorkflowActive(ctx, temporalClient, result.TenantID, item.ID)
		if err != nil {
			result.Failed++
			resultItem.Action = "error"
			resultItem.Error = err.Error()
			result.Items = append(result.Items, resultItem)
			continue
		}
		if active {
			result.Skipped++
			resultItem.Action = "skip_active_workflow"
			result.Items = append(result.Items, resultItem)
			continue
		}
		if err := workflowClient.StartFormApprovalWorkflow(ctx, start); err != nil {
			result.Failed++
			resultItem.Action = "error"
			resultItem.Error = err.Error()
			result.Items = append(result.Items, resultItem)
			continue
		}
		if err := restoreFormApprovalAfterTemporalBackfill(ctx, store, result.TenantID, item, run, ok); err != nil {
			result.Failed++
			resultItem.Action = "error"
			resultItem.Error = err.Error()
			result.Items = append(result.Items, resultItem)
			continue
		}
		result.Started++
		result.Items = append(result.Items, resultItem)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		return err
	}
	if result.Failed > 0 {
		return fmt.Errorf("failed to backfill %d form workflows", result.Failed)
	}
	return nil
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
	databaseURL := fs.String("database-url", config.DatabaseURLFromEnv(), "Postgres database URL")
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
		return errors.New("DB_HOST/DB_USERNAME/DB_NAME or --database-url is required")
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
	databaseURL := fs.String("database-url", config.DatabaseURLFromEnv(), "Postgres database URL")
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

// runOpenFGAGrantAgentTool 執行 agent_tool runner tuple 手工授權。
func runOpenFGAGrantAgentTool(args []string) error {
	fs := flag.NewFlagSet("tenantctl openfga-grant-agent-tool", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tenantID := fs.String("tenant-id", "", "tenant id")
	toolID := fs.String("tool-id", "", "agent tool id, for example knowledge.search")
	accountID := fs.String("account-id", "", "account id to grant")
	databaseURL := fs.String("database-url", config.DatabaseURLFromEnv(), "Postgres database URL")
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

// runEHRMSSyncPipeline 依序同步部門/崗位/員工與考勤。
func runEHRMSSyncPipeline(args []string) error {
	fs := flag.NewFlagSet("tenantctl ehrms-sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tenantID := fs.String("tenant-id", strings.TrimSpace(os.Getenv("EHRMS_SYNC_TENANT_ID")), "tenant id")
	accountID := fs.String("account-id", strings.TrimSpace(os.Getenv("EHRMS_SYNC_ACCOUNT_ID")), "service account id")
	mode := fs.String("mode", firstNonEmpty(strings.TrimSpace(os.Getenv("EHRMS_SYNC_MODE")), "upsert"), "employee sync mode: create, update, or upsert")
	attendanceMode := fs.String("attendance-mode", firstNonEmpty(strings.TrimSpace(os.Getenv("EHRMS_ATTENDANCE_SYNC_MODE")), "upsert"), "attendance sync mode: create, update, or upsert")
	since := fs.String("since", strings.TrimSpace(os.Getenv("EHRMS_ATTENDANCE_SYNC_SINCE")), "optional YYYY-MM-DD lower bound for attendance")
	databaseURL := fs.String("database-url", config.DatabaseURLFromEnv(), "Postgres database URL")
	timeout := fs.Duration("timeout", 10*time.Minute, "operation timeout")
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
		return errors.New("--tenant-id or EHRMS_SYNC_TENANT_ID is required")
	}
	if strings.TrimSpace(*accountID) == "" {
		return errors.New("--account-id or EHRMS_SYNC_ACCOUNT_ID is required")
	}
	if strings.TrimSpace(*databaseURL) == "" {
		return errors.New("DB_HOST/DB_USERNAME/DB_NAME or --database-url is required")
	}
	cfg, err := config.LoadE()
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.EHRMSBaseURL) == "" || strings.TrimSpace(cfg.EHRMSAPIKey) == "" {
		return errors.New("EHRMS_BASE_URL and EHRMS_API_KEY are required")
	}
	if strings.TrimSpace(cfg.OpenFGAAPIURL) == "" || strings.TrimSpace(cfg.OpenFGAStoreID) == "" || strings.TrimSpace(cfg.OpenFGAModelID) == "" {
		return errors.New("OPENFGA_BASE_URL, OPENFGA_STORE_ID, and OPENFGA_MODEL_ID are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	pool, err := postgresplatform.OpenPool(ctx, *databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	httpClient := &http.Client{Timeout: 60 * time.Second}
	checker := openfgaclient.NewChecker(cfg.OpenFGAAPIURL, cfg.OpenFGAStoreID, httpClient).WithAuthorizationModelID(cfg.OpenFGAModelID)
	ehrmsClient, err := ehrms.NewClient(cfg.EHRMSBaseURL, cfg.EHRMSAPIKey, httpClient)
	if err != nil {
		return err
	}
	svc := service.New(postgresrepo.NewStore(pool), service.Options{
		Logger:        logger,
		Relationships: checker,
		EHRMSClient:   ehrmsClient,
	})
	scheduler := jobs.NewEHRMSPipelineScheduler(svc.HR(), svc.Attendance(), logger)
	result, err := scheduler.SyncOnce(ctx, jobs.EHRMSPipelineOptions{
		EmployeeMode:      *mode,
		EmployeeTenantID:  *tenantID,
		EmployeeAccountID: *accountID,
		AttendanceEnabled: true,
		AttendanceMode:    *attendanceMode,
		AttendanceSince:   *since,
		AttendanceTenant:  firstNonEmpty(strings.TrimSpace(os.Getenv("EHRMS_ATTENDANCE_SYNC_TENANT_ID")), *tenantID),
		AttendanceAccount: firstNonEmpty(strings.TrimSpace(os.Getenv("EHRMS_ATTENDANCE_SYNC_ACCOUNT_ID")), *accountID),
	})
	if err != nil {
		var appErr *domain.AppError
		if errors.As(err, &appErr) && len(appErr.RowErrors) > 0 {
			encoder := json.NewEncoder(os.Stderr)
			encoder.SetIndent("", "  ")
			_ = encoder.Encode(appErr.RowErrors[:min(20, len(appErr.RowErrors))])
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(result)
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// runEHRMSSyncEmployees 從已配置的 eHRMS 上游同步員工資料。
func runEHRMSSyncEmployees(args []string) error {
	fs := flag.NewFlagSet("tenantctl ehrms-sync-employees", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tenantID := fs.String("tenant-id", strings.TrimSpace(os.Getenv("EHRMS_SYNC_TENANT_ID")), "tenant id")
	accountID := fs.String("account-id", strings.TrimSpace(os.Getenv("EHRMS_SYNC_ACCOUNT_ID")), "service account id with hr.employee.import")
	mode := fs.String("mode", firstNonEmpty(strings.TrimSpace(os.Getenv("EHRMS_SYNC_MODE")), "upsert"), "sync mode: create, update, or upsert")
	databaseURL := fs.String("database-url", config.DatabaseURLFromEnv(), "Postgres database URL")
	timeout := fs.Duration("timeout", 10*time.Minute, "operation timeout")
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
		return errors.New("--tenant-id or EHRMS_SYNC_TENANT_ID is required")
	}
	if strings.TrimSpace(*accountID) == "" {
		return errors.New("--account-id or EHRMS_SYNC_ACCOUNT_ID is required")
	}
	if strings.TrimSpace(*databaseURL) == "" {
		return errors.New("DB_HOST/DB_USERNAME/DB_NAME or --database-url is required")
	}
	cfg, err := config.LoadE()
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.EHRMSBaseURL) == "" || strings.TrimSpace(cfg.EHRMSAPIKey) == "" {
		return errors.New("EHRMS_BASE_URL and EHRMS_API_KEY are required")
	}
	if strings.TrimSpace(cfg.OpenFGAAPIURL) == "" || strings.TrimSpace(cfg.OpenFGAStoreID) == "" || strings.TrimSpace(cfg.OpenFGAModelID) == "" {
		return errors.New("OPENFGA_BASE_URL, OPENFGA_STORE_ID, and OPENFGA_MODEL_ID are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	pool, err := postgresplatform.OpenPool(ctx, *databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	httpClient := &http.Client{Timeout: 60 * time.Second}
	checker := openfgaclient.NewChecker(cfg.OpenFGAAPIURL, cfg.OpenFGAStoreID, httpClient).WithAuthorizationModelID(cfg.OpenFGAModelID)
	ehrmsClient, err := ehrms.NewClient(cfg.EHRMSBaseURL, cfg.EHRMSAPIKey, httpClient)
	if err != nil {
		return err
	}
	svc := service.New(postgresrepo.NewStore(pool), service.Options{
		Logger:        logger,
		Relationships: checker,
		EHRMSClient:   ehrmsClient,
	})
	scheduler := jobs.NewEHRMSEmployeeSyncScheduler(svc.HR(), logger)
	result, err := scheduler.SyncOnce(ctx, jobs.EHRMSEmployeeSyncOptions{
		TenantID:  *tenantID,
		AccountID: *accountID,
		Mode:      *mode,
	})
	if err != nil {
		var appErr *domain.AppError
		if errors.As(err, &appErr) && len(appErr.RowErrors) > 0 {
			encoder := json.NewEncoder(os.Stderr)
			encoder.SetIndent("", "  ")
			_ = encoder.Encode(appErr.RowErrors[:min(20, len(appErr.RowErrors))])
		}
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// runEHRMSSyncAttendance 從已配置的 eHRMS 上游同步考勤日彙總。
func runEHRMSSyncAttendance(args []string) error {
	fs := flag.NewFlagSet("tenantctl ehrms-sync-attendance", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tenantID := fs.String("tenant-id", firstNonEmpty(strings.TrimSpace(os.Getenv("EHRMS_ATTENDANCE_SYNC_TENANT_ID")), strings.TrimSpace(os.Getenv("EHRMS_SYNC_TENANT_ID"))), "tenant id")
	accountID := fs.String("account-id", firstNonEmpty(strings.TrimSpace(os.Getenv("EHRMS_ATTENDANCE_SYNC_ACCOUNT_ID")), strings.TrimSpace(os.Getenv("EHRMS_SYNC_ACCOUNT_ID"))), "service account id with attendance.clock.import")
	mode := fs.String("mode", firstNonEmpty(strings.TrimSpace(os.Getenv("EHRMS_ATTENDANCE_SYNC_MODE")), "upsert"), "sync mode: create, update, or upsert")
	since := fs.String("since", strings.TrimSpace(os.Getenv("EHRMS_ATTENDANCE_SYNC_SINCE")), "optional YYYY-MM-DD lower bound")
	databaseURL := fs.String("database-url", config.DatabaseURLFromEnv(), "Postgres database URL")
	timeout := fs.Duration("timeout", 10*time.Minute, "operation timeout")
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
		return errors.New("--tenant-id or EHRMS_ATTENDANCE_SYNC_TENANT_ID/EHRMS_SYNC_TENANT_ID is required")
	}
	if strings.TrimSpace(*accountID) == "" {
		return errors.New("--account-id or EHRMS_ATTENDANCE_SYNC_ACCOUNT_ID/EHRMS_SYNC_ACCOUNT_ID is required")
	}
	if strings.TrimSpace(*databaseURL) == "" {
		return errors.New("DB_HOST/DB_USERNAME/DB_NAME or --database-url is required")
	}
	cfg, err := config.LoadE()
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.EHRMSBaseURL) == "" || strings.TrimSpace(cfg.EHRMSAPIKey) == "" {
		return errors.New("EHRMS_BASE_URL and EHRMS_API_KEY are required")
	}
	if strings.TrimSpace(cfg.OpenFGAAPIURL) == "" || strings.TrimSpace(cfg.OpenFGAStoreID) == "" || strings.TrimSpace(cfg.OpenFGAModelID) == "" {
		return errors.New("OPENFGA_BASE_URL, OPENFGA_STORE_ID, and OPENFGA_MODEL_ID are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	pool, err := postgresplatform.OpenPool(ctx, *databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	httpClient := &http.Client{Timeout: 60 * time.Second}
	checker := openfgaclient.NewChecker(cfg.OpenFGAAPIURL, cfg.OpenFGAStoreID, httpClient).WithAuthorizationModelID(cfg.OpenFGAModelID)
	ehrmsClient, err := ehrms.NewClient(cfg.EHRMSBaseURL, cfg.EHRMSAPIKey, httpClient)
	if err != nil {
		return err
	}
	svc := service.New(postgresrepo.NewStore(pool), service.Options{
		Logger:        logger,
		Relationships: checker,
		EHRMSClient:   ehrmsClient,
	})
	scheduler := jobs.NewEHRMSAttendanceSyncScheduler(svc.Attendance(), logger)
	result, err := scheduler.SyncOnce(ctx, jobs.EHRMSAttendanceSyncOptions{
		TenantID:  *tenantID,
		AccountID: *accountID,
		Mode:      *mode,
		Since:     *since,
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
	databaseURL := fs.String("database-url", config.DatabaseURLFromEnv(), "Postgres database URL")
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
		return errors.New("DB_HOST/DB_USERNAME/DB_NAME or --database-url is required")
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
		return nil, nil, nil, errors.New("DB_HOST/DB_USERNAME/DB_NAME or --database-url is required")
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
		IssuerURL:         strings.TrimSpace(os.Getenv("KEYCLOAK_BASE_URL")),
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
	  tenantctl ensure-default-form-templates --tenant-id <id>
  tenantctl openfga-backfill --tenant-id <id>
  tenantctl temporal-backfill-form-workflows --tenant-id <id> [--dry-run]
  tenantctl openfga-grant-tenant-admin --tenant-id <id> --account-id <account-id>
  tenantctl openfga-grant-tenant-security-admin --tenant-id <id> --account-id <account-id>
  tenantctl openfga-grant-agent-tool --tenant-id <id> --tool-id <tool-id> --account-id <account-id>
  tenantctl ehrms-sync [--tenant-id <id>] [--account-id <account-id>] [--mode upsert] [--attendance-mode upsert] [--since YYYY-MM-DD]
  tenantctl ehrms-sync-employees [--tenant-id <id>] [--account-id <account-id>] [--mode upsert]
  tenantctl ehrms-sync-attendance [--tenant-id <id>] [--account-id <account-id>] [--mode upsert] [--since YYYY-MM-DD]

Required environment:
  DB_HOST / DB_PORT / DB_USERNAME / DB_PASSWORD / DB_NAME
  TEMPORAL_BASE_URL
  TEMPORAL_NAMESPACE
  TEMPORAL_TASK_QUEUE

Required for ehrms-sync / ehrms-sync-employees:
  EHRMS_BASE_URL
  EHRMS_API_KEY
  OPENFGA_BASE_URL
  OPENFGA_STORE_ID
  OPENFGA_MODEL_ID
  EHRMS_SYNC_TENANT_ID (or --tenant-id)
  EHRMS_SYNC_ACCOUNT_ID (or --account-id)

Required for ehrms-sync-attendance:
  EHRMS_BASE_URL
  EHRMS_API_KEY
  OPENFGA_BASE_URL
  OPENFGA_STORE_ID
  OPENFGA_MODEL_ID
  EHRMS_ATTENDANCE_SYNC_TENANT_ID or EHRMS_SYNC_TENANT_ID (or --tenant-id)
  EHRMS_ATTENDANCE_SYNC_ACCOUNT_ID or EHRMS_SYNC_ACCOUNT_ID (or --account-id)

Required for --provision-keycloak:
  KEYCLOAK_BASE_URL
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

func temporalBackfillCandidateStatus(status string) bool {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "submitted", "in_review", "pending", "pending_review", domain.WorkflowFormStatusWorkflowStartFailed:
		return true
	default:
		return false
	}
}

func temporalBackfillTerminalRunStatus(status string) bool {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case domain.WorkflowRunStatusCompleted, domain.WorkflowRunStatusCancelled, domain.WorkflowRunStatusReturned:
		return true
	default:
		return false
	}
}

func restoreFormApprovalAfterTemporalBackfill(
	ctx context.Context,
	store *postgresrepo.Store,
	tenantID string,
	item domain.FormInstance,
	run domain.WorkflowRun,
	hasRun bool,
) error {
	now := time.Now().UTC()
	changed := false
	if strings.EqualFold(strings.TrimSpace(item.Status), domain.WorkflowFormStatusWorkflowStartFailed) {
		item.Status = domain.WorkflowFormStatusInReview
		item.UpdatedAt = now
		changed = true
	}
	if changed {
		if err := store.UpsertFormInstance(ctx, item); err != nil {
			return err
		}
	}
	if hasRun && strings.EqualFold(strings.TrimSpace(run.Status), domain.WorkflowRunStatusStartFailed) {
		run.Status = domain.WorkflowRunStatusRunning
		run.UpdatedAt = now
		if err := store.UpsertWorkflowRun(ctx, run); err != nil {
			return err
		}
	}
	_ = tenantID
	return nil
}

func temporalBackfillWorkflowActive(ctx context.Context, client sdkclient.Client, tenantID, formInstanceID string) (bool, error) {
	if client == nil {
		return false, nil
	}
	description, err := client.DescribeWorkflow(ctx, domain.FormApprovalWorkflowID(tenantID, formInstanceID), "")
	if err != nil {
		var notFound *serviceerror.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		return false, err
	}
	return description.Status == enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, nil
}
