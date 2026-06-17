package postgres_integration_test

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository"
	postgresrepo "nexus-pro-be/internal/repository/postgres"
	"nexus-pro-be/internal/service"
)

func TestPostgresRepositoryCriticalSemantics(t *testing.T) {
	pool := openIntegrationPool(t)
	defer pool.Close()
	requireMigratedSchema(t, pool)
	store := postgresrepo.NewStore(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	suffix := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_") + "_" + time.Now().UTC().Format("150405000000")
	tenantA := "tenant_" + suffix + "_a"
	tenantB := "tenant_" + suffix + "_b"
	empA := "emp_" + suffix + "_a"
	empB := "emp_" + suffix + "_b"

	for _, tenantID := range []string{tenantA, tenantB} {
		if err := store.UpsertTenant(ctx, domain.Tenant{ID: tenantID, Name: tenantID, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertEmployee(ctx, domain.Employee{ID: empA, TenantID: tenantA, Name: "Tenant A", CompanyEmail: empA + "@example.com", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(ctx, domain.Employee{ID: empB, TenantID: tenantB, Name: "Tenant B", CompanyEmail: empB + "@example.com", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	employees, err := store.ListEmployees(ctx, tenantA)
	if err != nil {
		t.Fatal(err)
	}
	for _, employee := range employees {
		if employee.TenantID != tenantA {
			t.Fatalf("tenant A list leaked employee from %s: %+v", employee.TenantID, employee)
		}
	}

	sentinel := errors.New("rollback sentinel")
	rolledBackID := "emp_" + suffix + "_rollback"
	err = store.WithTenantTransaction(ctx, tenantA, func(tx repository.Store) error {
		if err := tx.UpsertEmployee(ctx, domain.Employee{ID: rolledBackID, TenantID: tenantA, Name: "Rollback", CompanyEmail: rolledBackID + "@example.com", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected rollback sentinel, got %v", err)
	}
	if _, ok, err := store.GetEmployee(ctx, tenantA, rolledBackID); err != nil || ok {
		t.Fatalf("expected rollback employee to be absent, ok=%v err=%v", ok, err)
	}

	prefix := "PX" + now.Format("150405")
	numbers := make([]string, 8)
	var wg sync.WaitGroup
	errs := make(chan error, len(numbers))
	for i := range numbers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			value, err := store.NextEmployeeNo(ctx, tenantA, prefix)
			if err != nil {
				errs <- err
				return
			}
			numbers[i] = value
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(numbers)
	for i := 1; i < len(numbers); i++ {
		if numbers[i] == numbers[i-1] {
			t.Fatalf("duplicate employee number generated: %v", numbers)
		}
	}

	balance := domain.LeaveBalance{ID: "lb_" + suffix, TenantID: tenantA, EmployeeID: empA, LeaveType: "annual", RemainingHours: 16, UpdatedAt: now}
	if err := store.UpsertLeaveBalance(ctx, balance); err != nil {
		t.Fatal(err)
	}
	updated, reserved, found, err := store.ReserveLeaveBalance(ctx, tenantA, empA, " annual ", 8, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !found || !reserved || updated.RemainingHours != 8 {
		t.Fatalf("expected trimmed leave type to reserve hours, got found=%v reserved=%v balance=%+v", found, reserved, updated)
	}
}

func TestHRCoreCRUDPostgresAcceptanceSemantics(t *testing.T) {
	pool := openIntegrationPool(t)
	defer pool.Close()
	requireMigratedSchema(t, pool)
	store := postgresrepo.NewStore(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	suffix := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_") + "_" + time.Now().UTC().Format("150405000000")
	tenantA := "tenant_" + suffix + "_a"
	tenantB := "tenant_" + suffix + "_b"
	accountID := "acct_" + suffix

	for _, tenantID := range []string{tenantA, tenantB} {
		if err := store.UpsertTenant(ctx, domain.Tenant{ID: tenantID, Name: tenantID, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertPermissionSet(ctx, domain.PermissionSet{
		ID:       "ps_" + suffix,
		TenantID: tenantA,
		Name:     "HR Acceptance",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "create", Scope: "all"},
			{Resource: "hr.employee", Action: "read", Scope: "all"},
			{Resource: "hr.employee", Action: "update", Scope: "all"},
			{Resource: "hr.employee", Action: "delete", Scope: "all"},
			{Resource: "hr.employee", Action: "import", Scope: "all"},
			{Resource: "hr.employee", Action: "export", Scope: "all"},
			{Resource: "hr.employee", Action: "status_transition", Scope: "all"},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(ctx, domain.Account{ID: accountID, TenantID: tenantA, Status: "active", DirectPermissionSetIDs: []string{"ps_" + suffix}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	app := service.New(store)
	reqCtx := domain.RequestContext{TenantID: tenantA, AccountID: accountID, RequestID: "it-" + suffix, ApprovalConfirmed: true}

	created, err := app.CreateEmployee(reqCtx, domain.CreateEmployeeInput{Name: "Integration One", CompanyEmail: "one_" + suffix + "@example.com", Status: "active", EmploymentStatus: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetEmployee(ctx, tenantB, created.ID); err != nil || ok {
		t.Fatalf("tenant B should not read tenant A employee, ok=%v err=%v", ok, err)
	}
	newPhone := "0911222333"
	updated, err := app.UpdateEmployee(reqCtx, created.ID, domain.UpdateEmployeeInput{Phone: &newPhone})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Phone != newPhone {
		t.Fatalf("expected updated phone, got %+v", updated)
	}

	session, err := app.PreviewEmployeeImport(reqCtx, domain.EmployeeImportPreviewInput{
		Filename: "employees.csv",
		Content:  "員工編號,姓名,Email,部門,職位,類別,電話,狀態,到職日期,主管員工ID\n,Integration Import,import_" + suffix + "@example.com,,HRBP,全職,0911000222,在職,2026-06-01,\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := app.ConfirmEmployeeImport(reqCtx, session.ID, domain.EmployeeImportConfirmInput{Mode: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Summary["confirmed"] != 1 {
		t.Fatalf("expected one confirmed import, got %+v", confirmed.Summary)
	}
	exported, err := app.ExportEmployees(reqCtx, domain.EmployeeQuery{Keyword: "Integration"})
	if err != nil {
		t.Fatal(err)
	}
	if len(exported) < 2 {
		t.Fatalf("expected created and imported employees in export, got %+v", exported)
	}
	resigned, err := app.TransitionEmployeeStatus(reqCtx, created.ID, domain.StatusTransitionInput{Status: "resigned", Reason: "integration offboard", EndDate: "2026-06-30"})
	if err != nil {
		t.Fatal(err)
	}
	if resigned.EmploymentStatus != "resigned" || resigned.ResignDate == nil {
		t.Fatalf("expected resigned employee, got %+v", resigned)
	}
	reinstated, err := app.TransitionEmployeeStatus(reqCtx, created.ID, domain.StatusTransitionInput{Status: "active", Reason: "integration reinstate", StartDate: "2026-07-01"})
	if err != nil {
		t.Fatal(err)
	}
	if reinstated.EmploymentStatus != "active" || reinstated.ResignDate != nil {
		t.Fatalf("expected active reinstated employee, got %+v", reinstated)
	}
	batch, err := app.BatchDeleteEmployees(reqCtx, domain.BatchDeleteEmployeesInput{EmployeeIDs: []string{created.ID}, Reason: "integration cleanup"})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Results) != 1 || !batch.Results[0].Success {
		t.Fatalf("expected successful batch delete, got %+v", batch)
	}
}

func openIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL/DATABASE_URL is not set; skipping postgres integration test")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 4
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	return pool
}

func requireMigratedSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var ready bool
	if err := pool.QueryRow(ctx, "select to_regclass('public.tenants') is not null and to_regclass('public.employees') is not null and to_regclass('public.employee_number_sequences') is not null").Scan(&ready); err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Skip("postgres schema is not migrated; skipping integration semantics test")
	}
}
