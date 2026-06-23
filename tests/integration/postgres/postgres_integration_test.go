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
	t.Run("employee RLS isolates tenant visibility", func(t *testing.T) {
		requireRLSCapableUser(t, pool)
		if got := countEmployeesVisibleViaRLS(t, pool, tenantA, empA); got != 1 {
			t.Fatalf("expected tenant A RLS scope to see employee %s once, got %d", empA, got)
		}
		if got := countEmployeesVisibleViaRLS(t, pool, tenantB, empA); got != 0 {
			t.Fatalf("expected tenant B RLS scope not to see tenant A employee %s, got %d", empA, got)
		}
	})

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
	if err := store.UpsertOrgUnit(ctx, domain.OrgUnit{ID: "ou_" + suffix, TenantID: tenantA, Name: "HQ " + suffix, Path: []string{"ou_" + suffix}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	app := service.New(store)
	reqCtx := domain.RequestContext{TenantID: tenantA, AccountID: accountID, RequestID: "it-" + suffix, ApprovalConfirmed: true}

	created, err := app.HR().CreateEmployee(reqCtx, validEmployeeCreateInput(suffix, "Integration One", "one_"+suffix+"@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	detail := domain.EmployeeDetailFromEmployee(created)
	if detail.Sections.BasicInfo.NationalID == "" ||
		detail.Sections.EmploymentInfo.OrgUnitID == "" ||
		detail.Sections.EducationMilitaryInfo.HighestEducation == "" ||
		detail.Sections.ContactInfo.MobilePhone == "" ||
		detail.Sections.InsuranceInfo.LaborInsuranceSalary == nil ||
		len(detail.Sections.InternalExperiences) == 0 {
		t.Fatalf("expected complete six-section employee detail, got %+v", detail.Sections)
	}
	if _, ok, err := store.GetEmployee(ctx, tenantB, created.ID); err != nil || ok {
		t.Fatalf("tenant B should not read tenant A employee, ok=%v err=%v", ok, err)
	}
	newPhone := "0911222333"
	updated, err := app.HR().UpdateEmployee(reqCtx, created.ID, domain.UpdateEmployeeInput{Phone: &newPhone})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Phone != newPhone {
		t.Fatalf("expected updated phone, got %+v", updated)
	}

	session, err := app.HR().PreviewEmployeeImport(reqCtx, domain.EmployeeImportPreviewInput{
		Filename: "employees.csv",
		Content:  "員工編號,姓名,Email,部門,職位,類別,電話,狀態,到職日期,主管員工ID\n,Integration Import,import_" + suffix + "@example.com,,HRBP,全職,0911000222,在職,2026-06-01,\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := app.HR().ConfirmEmployeeImport(reqCtx, session.ID, domain.EmployeeImportConfirmInput{Mode: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Summary["confirmed"] != 1 {
		t.Fatalf("expected one confirmed import, got %+v", confirmed.Summary)
	}
	exported, err := app.HR().ExportEmployees(reqCtx, domain.EmployeeQuery{Keyword: "Integration"})
	if err != nil {
		t.Fatal(err)
	}
	if len(exported) < 2 {
		t.Fatalf("expected created and imported employees in export, got %+v", exported)
	}
	resigned, err := app.HR().TransitionEmployeeStatus(reqCtx, created.ID, domain.StatusTransitionInput{Status: "resigned", Reason: "integration offboard", EndDate: "2026-06-30"})
	if err != nil {
		t.Fatal(err)
	}
	if resigned.EmploymentStatus != "resigned" || resigned.ResignDate == nil {
		t.Fatalf("expected resigned employee, got %+v", resigned)
	}
	reinstated, err := app.HR().TransitionEmployeeStatus(reqCtx, created.ID, domain.StatusTransitionInput{Status: "active", Reason: "integration reinstate", StartDate: "2026-07-01"})
	if err != nil {
		t.Fatal(err)
	}
	if reinstated.EmploymentStatus != "active" || reinstated.ResignDate != nil {
		t.Fatalf("expected active reinstated employee, got %+v", reinstated)
	}
	batch, err := app.HR().BatchDeleteEmployees(reqCtx, domain.BatchDeleteEmployeesInput{EmployeeIDs: []string{created.ID}, Reason: "integration cleanup"})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Results) != 1 || !batch.Results[0].Success {
		t.Fatalf("expected successful batch delete, got %+v", batch)
	}
}

func openIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		t.Skip("DATABASE_URL is not set; skipping postgres integration test")
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

func requireRLSCapableUser(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var bypassesRLS bool
	if err := pool.QueryRow(ctx, "select rolsuper or rolbypassrls from pg_roles where rolname = current_user").Scan(&bypassesRLS); err != nil {
		t.Fatal(err)
	}
	if bypassesRLS {
		t.Skip("current DATABASE_URL user bypasses RLS; use a non-superuser role to run RLS integration checks")
	}
}

func countEmployeesVisibleViaRLS(t *testing.T, pool *pgxpool.Pool, tenantID, employeeID string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM employees WHERE id = $1", employeeID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func validEmployeeCreateInput(suffix, name, email string) domain.CreateEmployeeInput {
	orgUnitID := "ou_" + suffix
	return domain.CreateEmployeeInput{
		EmployeeNo:       "E" + strings.NewReplacer("_", "", "-", "").Replace(suffix),
		Name:             name,
		CompanyEmail:     email,
		PersonalEmail:    "personal_" + suffix + "@example.com",
		Phone:            "0911222333",
		OrgUnitID:        orgUnitID,
		Position:         "Engineer",
		Category:         "full_time",
		Status:           "active",
		EmploymentStatus: "active",
		HireDate:         "2026-06-01",
		BasicInfo: map[string]any{
			"name":             name,
			"company_email":    email,
			"personal_email":   "personal_" + suffix + "@example.com",
			"nationality_type": "local",
			"national_id":      "ID-" + suffix,
		},
		EmploymentInfo: map[string]any{
			"org_unit_id":         orgUnitID,
			"position":            "Engineer",
			"category":            "full_time",
			"employment_status":   "active",
			"hire_date":           "2026-06-01",
			"tenure_start_date":   "2026-06-01",
			"manager_employee_id": "",
		},
		EducationMilitaryInfo: map[string]any{
			"highest_education": "master",
			"school":            "NTU",
			"graduation_date":   "2025-06-01",
		},
		ContactInfo: map[string]any{
			"mobile_phone":               "0911222333",
			"address":                    "Taipei",
			"emergency_contact_relation": "spouse",
			"emergency_contact_name":     "Emergency Contact",
			"emergency_contact_phone":    "0922333444",
		},
		InsuranceInfo: map[string]any{
			"labor_insurance_date":    "2026-06-01",
			"labor_insurance_level":   "L1",
			"labor_insurance_salary":  "45800",
			"health_insurance_date":   "2026-06-01",
			"health_insurance_level":  "H1",
			"health_insurance_amount": "826",
		},
	}
}
