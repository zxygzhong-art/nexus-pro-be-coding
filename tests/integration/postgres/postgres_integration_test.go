package postgres_integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"nexus-pro-be/internal/config"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	v1api "nexus-pro-be/internal/api/v1"
	"nexus-pro-be/internal/domain"
	openfgaclient "nexus-pro-be/internal/platform/openfga"
	pgplatform "nexus-pro-be/internal/platform/postgres"
	"nexus-pro-be/internal/repository"
	postgresrepo "nexus-pro-be/internal/repository/postgres"
	"nexus-pro-be/internal/service"
)

// TestPostgresRepositoryCriticalSemantics 驗證 Postgres repository critical semantics。
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

	t.Run("tenants RLS lets system task list every tenant", func(t *testing.T) {
		requireRLSCapableUser(t, pool)
		requireTenantsSystemReadPolicy(t, pool)
		tenants, err := store.ListTenants(ctx)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, tenant := range tenants {
			seen[tenant.ID] = true
		}
		if !seen[tenantA] || !seen[tenantB] {
			t.Fatalf("expected ListTenants under RLS to include %s and %s, got %d tenants", tenantA, tenantB, len(tenants))
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

	t.Run("attendance multi-punch boundaries are deterministic", func(t *testing.T) {
		workDate := "2026-06-10"
		base := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
		records := []domain.AttendanceClockRecord{
			{ID: "acr_" + suffix + "_in_first", TenantID: tenantA, EmployeeID: empA, WorkDate: workDate, Direction: "clock_in", ClientEventID: "evt_" + suffix + "_in_first", ClockedAt: base, RecordStatus: "accepted", Source: "geofence", CreatedAt: base},
			{ID: "acr_" + suffix + "_in_middle", TenantID: tenantA, EmployeeID: empA, WorkDate: workDate, Direction: "clock_in", ClientEventID: "evt_" + suffix + "_in_middle", ClockedAt: base.Add(4 * time.Hour), RecordStatus: "accepted", Source: "geofence", CreatedAt: base.Add(4 * time.Hour)},
			{ID: "acr_" + suffix + "_out_middle", TenantID: tenantA, EmployeeID: empA, WorkDate: workDate, Direction: "clock_out", ClientEventID: "evt_" + suffix + "_out_middle", ClockedAt: base.Add(2 * time.Hour), RecordStatus: "accepted", Source: "geofence", CreatedAt: base.Add(2 * time.Hour)},
			{ID: "acr_" + suffix + "_out_last", TenantID: tenantA, EmployeeID: empA, WorkDate: workDate, Direction: "clock_out", ClientEventID: "evt_" + suffix + "_out_last", ClockedAt: base.Add(10 * time.Hour), RecordStatus: "accepted", Source: "geofence", CreatedAt: base.Add(10 * time.Hour)},
			{ID: "acr_" + suffix + "_out_voided", TenantID: tenantA, EmployeeID: empA, WorkDate: workDate, Direction: "clock_out", ClientEventID: "evt_" + suffix + "_out_voided", ClockedAt: base.Add(11 * time.Hour), RecordStatus: "accepted", Source: "geofence", Voided: true, CreatedAt: base.Add(11 * time.Hour)},
		}
		for _, record := range records {
			if err := store.UpsertAttendanceClockRecord(ctx, record); err != nil {
				t.Fatal(err)
			}
		}

		duplicateEvent := records[0]
		duplicateEvent.ID += "_duplicate"
		if err := store.UpsertAttendanceClockRecord(ctx, duplicateEvent); err == nil {
			t.Fatal("expected duplicate client_event_id to conflict")
		}
		byEvent, ok, err := store.GetAttendanceClockRecordByClientEventID(ctx, tenantA, records[2].ClientEventID)
		if err != nil || !ok || byEvent.ID != records[2].ID {
			t.Fatalf("expected client event lookup, ok=%v record=%+v err=%v", ok, byEvent, err)
		}
		earliest, ok, err := store.GetEarliestAcceptedAttendanceClockIn(ctx, tenantA, empA, workDate)
		if err != nil || !ok || earliest.ID != records[0].ID {
			t.Fatalf("expected earliest clock-in, ok=%v record=%+v err=%v", ok, earliest, err)
		}
		latestOut, ok, err := store.GetLatestAcceptedAttendanceClockOut(ctx, tenantA, empA, workDate)
		if err != nil || !ok || latestOut.ID != records[3].ID {
			t.Fatalf("expected latest non-voided clock-out, ok=%v record=%+v err=%v", ok, latestOut, err)
		}
		latest, ok, err := store.GetLatestAcceptedAttendanceClockRecord(ctx, tenantA, empA, workDate)
		if err != nil || !ok || latest.ID != records[3].ID {
			t.Fatalf("expected latest non-voided clock record, ok=%v record=%+v err=%v", ok, latest, err)
		}

		correction := domain.AttendanceCorrectionRequest{
			ID: "acorr_" + suffix, TenantID: tenantA, EmployeeID: empA,
			Direction: "clock_out", RequestedClockedAt: base.Add(10 * time.Hour), WorkDate: workDate,
			CorrectionType: "replace_record", TargetClockRecordID: records[2].ID,
			Reason: "replace mistaken punch", Status: "pending", CreatedAt: base, UpdatedAt: base,
		}
		if err := store.UpsertAttendanceCorrectionRequest(ctx, correction); err != nil {
			t.Fatal(err)
		}
		correction.ReplacementClockRecordID = records[3].ID
		correction.Status = "approved"
		correction.UpdatedAt = base.Add(time.Minute)
		if err := store.UpsertAttendanceCorrectionRequest(ctx, correction); err != nil {
			t.Fatal(err)
		}
		storedCorrection, ok, err := store.GetAttendanceCorrectionRequest(ctx, tenantA, correction.ID)
		if err != nil || !ok || storedCorrection.CorrectionType != "replace_record" || storedCorrection.TargetClockRecordID != records[2].ID || storedCorrection.ReplacementClockRecordID != records[3].ID {
			t.Fatalf("expected correction audit links to round-trip, ok=%v correction=%+v err=%v", ok, storedCorrection, err)
		}
	})

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
	updated, reserved, found, err := store.ReserveLeaveBalance(ctx, tenantA, empA, " annual ", 8, now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !found || !reserved || updated.RemainingHours != 8 {
		t.Fatalf("expected trimmed leave type to reserve hours, got found=%v reserved=%v balance=%+v", found, reserved, updated)
	}

	ehrmsBalance := balance
	ehrmsBalance.ID = "ehrms_" + suffix
	ehrmsBalance.RemainingHours = 24
	ehrmsBalance.Source = "ehrms"
	ehrmsBalance.UpdatedAt = now.Add(2 * time.Minute)
	if err := store.UpsertLeaveBalance(ctx, ehrmsBalance); err != nil {
		t.Fatalf("expected employee/type conflict to update existing balance: %v", err)
	}
	balances, err := store.ListLeaveBalances(ctx, tenantA)
	if err != nil {
		t.Fatal(err)
	}
	matched := make([]domain.LeaveBalance, 0, 1)
	for _, item := range balances {
		if item.EmployeeID == empA && item.LeaveType == "annual" {
			matched = append(matched, item)
		}
	}
	if len(matched) != 1 || matched[0].ID != balance.ID || matched[0].RemainingHours != 24 || matched[0].Source != "ehrms" {
		t.Fatalf("expected one updated balance preserving original id, got %+v", matched)
	}
}

// TestHRCoreCRUDPostgresAcceptanceSemantics 驗證 HR core crud Postgres acceptance semantics。
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
	reqCtx := domain.RequestContext{TenantID: tenantA, AccountID: accountID, RequestID: "it-" + suffix}

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

// TestEmployeeHTTPPostgresAcceptanceTraceAuthzAndFieldPolicy 驗證員工 HTTP Postgres acceptance trace 授權 and 欄位政策。
func TestEmployeeHTTPPostgresAcceptanceTraceAuthzAndFieldPolicy(t *testing.T) {
	spanRecorder := installIntegrationSpanRecorder(t)
	pool := openIntegrationPool(t)
	defer pool.Close()
	requireMigratedSchema(t, pool)
	store := postgresrepo.NewStore(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	suffix := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_") + "_" + time.Now().UTC().Format("150405000000")
	tenantID := "tenant_" + suffix
	hrAccountID := "acct_" + suffix + "_hr"
	limitedAccountID := "acct_" + suffix + "_limited"
	rebacAccountID := "acct_" + suffix + "_rebac"
	permissionSetID := "ps_" + suffix + "_hr"
	rebacPermissionSetID := "ps_" + suffix + "_rebac"
	employeeID := "emp_" + suffix
	orgUnitID := "ou_" + suffix
	hireDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	var openFGACheckPath string
	var openFGATraceParent string
	openFGAServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		openFGACheckPath = r.URL.Path
		openFGATraceParent = r.Header.Get("Traceparent")
		if r.URL.Path != "/stores/store-1/check" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": true})
	}))
	defer openFGAServer.Close()
	openFGATransport := openFGAServer.Client().Transport
	if openFGATransport == nil {
		openFGATransport = http.DefaultTransport
	}
	relationshipChecker := openfgaclient.NewChecker(openFGAServer.URL, "store-1", &http.Client{
		Transport: otelhttp.NewTransport(openFGATransport),
	}).WithAuthorizationModelID("model-1")

	if err := store.UpsertTenant(ctx, domain.Tenant{ID: tenantID, Name: tenantID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(ctx, domain.PermissionSet{
		ID:       permissionSetID,
		TenantID: tenantID,
		Name:     "HR HTTP Acceptance",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all"},
			{Resource: "hr.employee", Action: "export", Scope: "all"},
			{Resource: "hr.employee", Action: "delete", Scope: "all"},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(ctx, domain.PermissionSet{
		ID:       rebacPermissionSetID,
		TenantID: tenantID,
		Name:     "HR ReBAC Acceptance",
		Permissions: []domain.Permission{
			{Resource: "hr.employee", Action: "read", Scope: "all", Relation: "viewer"},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(ctx, domain.Account{ID: hrAccountID, TenantID: tenantID, Status: "active", DirectPermissionSetIDs: []string{permissionSetID}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	upsertIntegrationIdentity(t, store, tenantID, hrAccountID, now)
	if err := store.UpsertAccount(ctx, domain.Account{ID: limitedAccountID, TenantID: tenantID, Status: "active", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	upsertIntegrationIdentity(t, store, tenantID, limitedAccountID, now)
	if err := store.UpsertAccount(ctx, domain.Account{ID: rebacAccountID, TenantID: tenantID, Status: "active", DirectPermissionSetIDs: []string{rebacPermissionSetID}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	upsertIntegrationIdentity(t, store, tenantID, rebacAccountID, now)
	if err := store.UpsertOrgUnit(ctx, domain.OrgUnit{ID: orgUnitID, TenantID: tenantID, Name: "HTTP HQ " + suffix, Path: []string{orgUnitID}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFieldPolicy(ctx, domain.FieldPolicy{
		ID:              "fp_" + suffix + "_hide_phone",
		TenantID:        tenantID,
		ApplicationCode: "hr",
		ResourceType:    "employee",
		FieldName:       "phone",
		Effect:          "hide",
		CreatedAt:       now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFieldPolicy(ctx, domain.FieldPolicy{
		ID:              "fp_" + suffix + "_deny_national",
		TenantID:        tenantID,
		ApplicationCode: "hr",
		ResourceType:    "employee",
		FieldName:       "national_id",
		Effect:          "deny",
		CreatedAt:       now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(ctx, domain.Employee{
		ID:               employeeID,
		TenantID:         tenantID,
		EmployeeNo:       "E" + strings.NewReplacer("_", "", "-", "").Replace(suffix),
		Name:             "HTTP Integration",
		CompanyEmail:     "http_" + suffix + "@example.com",
		Phone:            "0912345678",
		OrgUnitID:        orgUnitID,
		Position:         "Engineer",
		Category:         "full_time",
		Status:           "active",
		EmploymentStatus: "active",
		HireDate:         &hireDate,
		BasicInfo:        map[string]any{"nationality_type": "local", "national_id": "A123456789"},
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatal(err)
	}

	handler := v1api.New(service.New(store, service.Options{Relationships: relationshipChecker}), nil, v1api.Options{
		TelemetryServiceName: "nexus-pro-be-it",
		TokenResolver:        integrationTokenResolver{},
	}).Routes()

	deniedReq := httptest.NewRequest(http.MethodGet, "/v1/hr/employees", nil)
	addIntegrationHeaders(deniedReq, tenantID, limitedAccountID, "req-"+suffix+"-denied")
	deniedRec := httptest.NewRecorder()
	handler.ServeHTTP(deniedRec, deniedReq)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing employee read permission, got %d: %s", deniedRec.Code, deniedRec.Body.String())
	}
	deniedErr := decodeIntegrationError(t, deniedRec.Body.Bytes())
	if deniedErr.ReasonCode != "menu_denied" || deniedErr.TraceID == "" {
		t.Fatalf("expected menu_denied with trace_id, got %+v", deniedErr)
	}
	logs, err := store.ListAuditLogs(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	deniedLog, ok := findIntegrationAuditLog(logs, "hr.employee.read")
	if !ok || deniedLog.Details["authz_decision"] != false || deniedLog.Details["reason_code"] != "menu_denied" {
		t.Fatalf("expected permission-denied audit log, got log=%+v all=%+v", deniedLog, logs)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/hr/employees?page=1&page_size=10", nil)
	addIntegrationHeaders(listReq, tenantID, hrAccountID, "req-"+suffix+"-list")
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for employee list, got %d: %s", listRec.Code, listRec.Body.String())
	}
	page := decodeIntegrationData[domain.PageResponse[domain.Employee]](t, listRec.Body.Bytes())
	if len(page.Items) != 1 || page.Items[0].ID != employeeID {
		t.Fatalf("expected one employee page item, got %+v", page)
	}
	if page.Items[0].Phone != "" {
		t.Fatalf("expected phone to be hidden by field policy, got %+v", page.Items[0])
	}
	if _, ok := page.Items[0].BasicInfo["national_id"]; ok {
		t.Fatalf("expected national_id to be removed by field policy, got %+v", page.Items[0].BasicInfo)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/v1/hr/employees/"+employeeID, nil)
	addIntegrationHeaders(detailReq, tenantID, rebacAccountID, "req-"+suffix+"-openfga")
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for OpenFGA-backed employee detail, got %d: %s", detailRec.Code, detailRec.Body.String())
	}
	if openFGACheckPath != "/stores/store-1/check" || openFGATraceParent == "" {
		t.Fatalf("expected traced OpenFGA check, path=%q traceparent=%q", openFGACheckPath, openFGATraceParent)
	}
	detailTraceID := integrationTraceIDForSpanNamePrefix(spanRecorder, "GET /v1/hr/employees/")
	if detailTraceID == "" || !strings.Contains(openFGATraceParent, detailTraceID) {
		t.Fatalf("expected OpenFGA traceparent %q to contain BFF detail trace %q; spans=%v", openFGATraceParent, detailTraceID, integrationSpanNames(spanRecorder))
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/v1/hr/employees/export", nil)
	addIntegrationHeaders(exportReq, tenantID, hrAccountID, "req-"+suffix+"-export")
	exportRec := httptest.NewRecorder()
	handler.ServeHTTP(exportRec, exportReq)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for employee export, got %d: %s", exportRec.Code, exportRec.Body.String())
	}
	if strings.Contains(exportRec.Body.String(), "0912345678") {
		t.Fatalf("expected exported CSV to omit hidden phone, got %q", exportRec.Body.String())
	}
	logs, err = store.ListAuditLogs(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	exportLog, ok := findIntegrationAuditLog(logs, "hr.employee.export")
	if !ok {
		t.Fatalf("expected employee export audit log, got %+v", logs)
	}
	if exportLog.TraceID == "" || exportLog.TraceID == "req-"+suffix+"-export" {
		t.Fatalf("expected export audit trace_id from OTel, got %+v", exportLog)
	}
	if exportLog.Details["trace_id"] != exportLog.TraceID || exportLog.Details["request_id"] != "req-"+suffix+"-export" {
		t.Fatalf("expected export audit details to retain trace/request IDs, got %+v", exportLog.Details)
	}
	if !integrationSpanWithTrace(spanRecorder, exportLog.TraceID, "GET /v1/hr/employees/export") {
		t.Fatalf("expected BFF export span on trace %s, got %v", exportLog.TraceID, integrationSpanNames(spanRecorder))
	}
	if !integrationSpanWithTrace(spanRecorder, exportLog.TraceID, "service.authz.authorize") {
		t.Fatalf("expected HR Core authz span on trace %s, got %v", exportLog.TraceID, integrationSpanNames(spanRecorder))
	}
	if !integrationSpanWithTracePrefix(spanRecorder, exportLog.TraceID, "postgres.") {
		t.Fatalf("expected Postgres span on trace %s, got %v", exportLog.TraceID, integrationSpanNames(spanRecorder))
	}

	deleteReq := httptest.NewRequest(http.MethodPost, "/v1/hr/employees/batch-delete", strings.NewReader(`{"employee_ids":["`+employeeID+`"],"reason":"integration cleanup"}`))
	addIntegrationHeaders(deleteReq, tenantID, hrAccountID, "req-"+suffix+"-delete")
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for batch delete, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
	result := decodeIntegrationData[domain.BatchEmployeeResponse](t, deleteRec.Body.Bytes())
	if len(result.Results) != 1 || !result.Results[0].Success {
		t.Fatalf("expected successful batch delete result, got %+v", result)
	}
	logs, err = store.ListAuditLogs(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	deleteLog, ok := findIntegrationAuditLog(logs, "hr.employee.batch_delete")
	if !ok || deleteLog.Details["request_id"] != "req-"+suffix+"-delete" || deleteLog.Details["reason"] != "integration cleanup" {
		t.Fatalf("expected high-risk batch delete audit log, got log=%+v all=%+v", deleteLog, logs)
	}
}

// TestAttendanceClockHTTPPostgresFieldPolicy 驗證考勤打卡 HTTP Postgres 欄位政策。
func TestAttendanceClockHTTPPostgresFieldPolicy(t *testing.T) {
	pool := openIntegrationPool(t)
	defer pool.Close()
	requireMigratedSchema(t, pool)
	store := postgresrepo.NewStore(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	clockNow := time.Date(2026, 6, 10, 1, 0, 0, 0, time.UTC)
	suffix := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_") + "_" + time.Now().UTC().Format("150405000000")
	tenantID := "tenant_" + suffix
	employeeID := "emp_" + suffix
	adminEmployeeID := "emp_" + suffix + "_admin"
	employeeAccountID := "acct_" + suffix + "_employee"
	adminAccountID := "acct_" + suffix + "_admin"
	worksiteID := "aws_" + suffix
	shiftID := "ash_" + suffix

	if err := store.UpsertTenant(ctx, domain.Tenant{ID: tenantID, Name: tenantID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(ctx, domain.PermissionSet{
		ID:       "ps_" + suffix + "_self",
		TenantID: tenantID,
		Name:     "Attendance Self",
		Permissions: []domain.Permission{
			{Resource: "attendance.clock", Action: "create", Scope: "self"},
			{Resource: "attendance.clock", Action: "read", Scope: "self"},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(ctx, domain.PermissionSet{
		ID:       "ps_" + suffix + "_admin",
		TenantID: tenantID,
		Name:     "Attendance Admin",
		Permissions: []domain.Permission{
			{Resource: "attendance.clock", Action: "read", Scope: "all"},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(ctx, domain.Account{ID: employeeAccountID, TenantID: tenantID, EmployeeID: employeeID, Status: "active", DirectPermissionSetIDs: []string{"ps_" + suffix + "_self"}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	upsertIntegrationIdentity(t, store, tenantID, employeeAccountID, now)
	if err := store.UpsertAccount(ctx, domain.Account{ID: adminAccountID, TenantID: tenantID, EmployeeID: adminEmployeeID, Status: "active", DirectPermissionSetIDs: []string{"ps_" + suffix + "_admin"}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	upsertIntegrationIdentity(t, store, tenantID, adminAccountID, now)
	if err := store.UpsertEmployee(ctx, domain.Employee{ID: employeeID, TenantID: tenantID, Name: "Clock Target", CompanyEmail: employeeID + "@example.com", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEmployee(ctx, domain.Employee{ID: adminEmployeeID, TenantID: tenantID, Name: "Clock Admin", CompanyEmail: adminEmployeeID + "@example.com", Status: "active", EmploymentStatus: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAttendanceWorksite(ctx, domain.AttendanceWorksite{ID: worksiteID, TenantID: tenantID, Name: "HQ", Latitude: 25.033964, Longitude: 121.564468, RadiusMeters: 200, Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAttendanceShift(ctx, domain.AttendanceShift{ID: shiftID, TenantID: tenantID, Name: "Day Shift", ClockInStart: "08:00", ClockInEnd: "10:00", ClockOutStart: "17:00", ClockOutEnd: "19:00", Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	handler := v1api.New(service.New(store, service.Options{Now: func() time.Time { return clockNow }}), nil, v1api.Options{TokenResolver: integrationTokenResolver{}}).Routes()

	createReq := httptest.NewRequest(http.MethodPost, "/v1/attendance/clock-records", strings.NewReader(`{"direction":"clock_in","latitude":25.034,"longitude":121.5645,"accuracy_meters":12,"location_source":"gps","device_id":"phone-1","device_info":{"os":"ios"}}`))
	addIntegrationHeaders(createReq, tenantID, employeeAccountID, "req-"+suffix+"-clock-create")
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for clock create, got %d: %s", createRec.Code, createRec.Body.String())
	}
	created := decodeIntegrationData[domain.AttendanceClockRecord](t, createRec.Body.Bytes())
	if created.Latitude != 0 || created.Longitude != 0 || created.AccuracyMeters != 0 || created.DeviceID != "" || created.DeviceInfo != nil {
		t.Fatalf("expected create response to hide GPS and device evidence, got %+v", created)
	}
	raw, ok, err := store.GetEarliestAcceptedAttendanceClockIn(ctx, tenantID, employeeID, created.WorkDate)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || raw.Latitude == 0 || raw.Longitude == 0 || raw.AccuracyMeters != 12 || raw.DeviceID != "phone-1" || raw.DeviceInfo["location_source"] != "gps" || raw.DeviceInfo["os"] != "ios" {
		t.Fatalf("expected raw clock evidence to remain in PostgreSQL, ok=%v record=%+v", ok, raw)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/attendance/clock-records?employee_id="+employeeID, nil)
	addIntegrationHeaders(listReq, tenantID, adminAccountID, "req-"+suffix+"-clock-list")
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for clock list, got %d: %s", listRec.Code, listRec.Body.String())
	}
	page := decodeIntegrationData[domain.PageResponse[domain.AttendanceClockRecord]](t, listRec.Body.Bytes())
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("expected one clock record page item, got %+v", page)
	}
	redacted := page.Items[0]
	if redacted.Latitude != 0 || redacted.Longitude != 0 || redacted.AccuracyMeters != 0 || redacted.DistanceMeters != 0 || redacted.DeviceID != "" || redacted.DeviceInfo != nil {
		t.Fatalf("expected default clock read to hide GPS and device evidence, got %+v", redacted)
	}

	for _, field := range []string{"latitude", "longitude", "accuracy_meters", "distance_meters", "device_id", "device_info", "location_source"} {
		if err := store.UpsertFieldPolicy(ctx, domain.FieldPolicy{
			ID:              "fp_" + suffix + "_" + strings.ReplaceAll(field, "_", "-"),
			TenantID:        tenantID,
			ApplicationCode: "attendance",
			ResourceType:    "clock",
			FieldName:       field,
			Effect:          "allow",
			PermissionID:    "attendance.clock.read",
			CreatedAt:       now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	allowedReq := httptest.NewRequest(http.MethodGet, "/v1/attendance/clock-records?employee_id="+employeeID, nil)
	addIntegrationHeaders(allowedReq, tenantID, adminAccountID, "req-"+suffix+"-clock-list-allow")
	allowedRec := httptest.NewRecorder()
	handler.ServeHTTP(allowedRec, allowedReq)
	if allowedRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for allowed clock list, got %d: %s", allowedRec.Code, allowedRec.Body.String())
	}
	allowedPage := decodeIntegrationData[domain.PageResponse[domain.AttendanceClockRecord]](t, allowedRec.Body.Bytes())
	if allowedPage.Total != 1 || len(allowedPage.Items) != 1 {
		t.Fatalf("expected one allowed clock record page item, got %+v", allowedPage)
	}
	visible := allowedPage.Items[0]
	if visible.Latitude != raw.Latitude || visible.Longitude != raw.Longitude || visible.AccuracyMeters != 12 || visible.DeviceID != "phone-1" || visible.DeviceInfo["location_source"] != "gps" || visible.DeviceInfo["os"] != "ios" {
		t.Fatalf("expected allow field policies to reveal GPS and device evidence, got %+v", visible)
	}
}

// TestPostgresKnowledgeVectorSearch verifies pgvector persistence, ranking, and dimension isolation.
func TestPostgresKnowledgeVectorSearch(t *testing.T) {
	pool := openIntegrationPool(t)
	defer pool.Close()
	requireMigratedSchema(t, pool)
	var chunksReady bool
	if err := pool.QueryRow(context.Background(), "select to_regclass('public.knowledge_document_chunks') is not null").Scan(&chunksReady); err != nil {
		t.Fatal(err)
	}
	if !chunksReady {
		t.Skip("knowledge vector schema is not migrated")
	}
	store := postgresrepo.NewStore(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	suffix := now.Format("150405000000")
	tenantID := "tenant_vector_" + suffix
	baseID := "kb_vector_" + suffix
	documentID := "kdoc_vector_" + suffix
	if err := store.UpsertTenant(ctx, domain.Tenant{ID: tenantID, Name: tenantID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	err := store.WithTenantTransaction(ctx, tenantID, func(tx repository.Store) error {
		if err := tx.UpsertKnowledgeBase(ctx, domain.KnowledgeBase{ID: baseID, TenantID: tenantID, Name: "Policies", ChunkMode: "fixed", ChunkSize: 500, ChunkOverlap: 50, CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		if err := tx.UpsertKnowledgeDocument(ctx, domain.KnowledgeDocument{
			ID: documentID, TenantID: tenantID, KnowledgeBaseID: baseID, Title: "Leave", Content: "Annual leave", SourceType: "text",
			OriginalFilename: "leave.md", ContentType: "text/markdown", SizeBytes: 12, SHA256: "fixture-sha",
			ObjectProvider: "memory", ObjectKey: "knowledge/leave.md", ParseStatus: "ready", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		return tx.ReplaceKnowledgeDocumentChunks(ctx, tenantID, documentID, []domain.KnowledgeDocumentChunk{
			{ID: "kchunk_" + suffix + "_leave", TenantID: tenantID, KnowledgeBaseID: baseID, DocumentID: documentID, Ordinal: 0, Content: "Annual leave policy", EmbeddingModel: "nexus-pro-embedding", EmbeddingDimensions: 2, Embedding: []float32{1, 0}, CreatedAt: now},
			{ID: "kchunk_" + suffix + "_payroll", TenantID: tenantID, KnowledgeBaseID: baseID, DocumentID: documentID, Ordinal: 1, Content: "Payroll schedule", EmbeddingModel: "nexus-pro-embedding", EmbeddingDimensions: 2, Embedding: []float32{0, 1}, CreatedAt: now},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	storedBase, ok, err := store.GetKnowledgeBase(ctx, tenantID, baseID)
	if err != nil || !ok || storedBase.ChunkMode != "fixed" || storedBase.ChunkSize != 500 || storedBase.ChunkOverlap != 50 {
		t.Fatalf("knowledge chunk configuration did not round-trip: base=%+v ok=%v err=%v", storedBase, ok, err)
	}
	storedDocument, ok, err := store.GetKnowledgeDocument(ctx, tenantID, baseID, documentID)
	if err != nil || !ok || storedDocument.SourceType != "text" || storedDocument.OriginalFilename != "leave.md" || storedDocument.ObjectKey != "knowledge/leave.md" {
		t.Fatalf("knowledge upload metadata did not round-trip: document=%+v ok=%v err=%v", storedDocument, ok, err)
	}
	matches, err := store.SearchKnowledgeDocumentChunks(ctx, tenantID, []string{baseID}, "nexus-pro-embedding", []float32{1, 0}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0].Ordinal != 0 || matches[0].Score < 0.99 {
		t.Fatalf("unexpected pgvector ranking: %+v", matches)
	}
	dimensionMismatch, err := store.SearchKnowledgeDocumentChunks(ctx, tenantID, []string{baseID}, "nexus-pro-embedding", []float32{1, 0, 0}, 5)
	if err != nil || len(dimensionMismatch) != 0 {
		t.Fatalf("expected dimension-isolated empty result, matches=%+v err=%v", dimensionMismatch, err)
	}
}

// openIntegrationPool 驗證 open integration pool。
func openIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := config.DatabaseURLFromEnv()
	if dsn == "" {
		t.Skip("DB_* is not set; skipping postgres integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgplatform.OpenPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

// requireMigratedSchema 驗證 require migrated schema。
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

// requireRLSCapableUser 驗證 require RLS capable 使用者。
func requireRLSCapableUser(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var bypassesRLS bool
	if err := pool.QueryRow(ctx, "select rolsuper or rolbypassrls from pg_roles where rolname = current_user").Scan(&bypassesRLS); err != nil {
		t.Fatal(err)
	}
	if bypassesRLS {
		t.Skip("current DB user bypasses RLS; use a non-superuser role to run RLS integration checks")
	}
}

// requireTenantsSystemReadPolicy 驗證 require 租戶 system read 政策。
func requireTenantsSystemReadPolicy(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var exists bool
	if err := pool.QueryRow(ctx, "select exists (select 1 from pg_policies where tablename = 'tenants' and policyname = 'system_read_tenants')").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Skip("tenants system_read_tenants policy is not migrated; run current migrations to exercise system-task RLS reads")
	}
}

// countEmployeesVisibleViaRLS 驗證 count 員工可見 via RLS。
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

// validEmployeeCreateInput 驗證有效員工 create 輸入。
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

type integrationErrorPayload struct {
	Code       domain.ErrorCode `json:"code"`
	ReasonCode string           `json:"reason_code"`
	TraceID    string           `json:"trace_id"`
}

// decodeIntegrationData 驗證 decode integration 資料。
func decodeIntegrationData[T any](t *testing.T, body []byte) T {
	t.Helper()
	var payload struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Data
}

// decodeIntegrationError 驗證 decode integration 錯誤。
func decodeIntegrationError(t *testing.T, body []byte) integrationErrorPayload {
	t.Helper()
	var payload struct {
		Error integrationErrorPayload `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Error
}

// addIntegrationHeaders 驗證 add integration headers。
func addIntegrationHeaders(req *http.Request, tenantID, accountID, requestID string) {
	req.Header.Set("Authorization", "Bearer "+tenantID+":"+accountID)
	req.Header.Set("X-Request-ID", requestID)
}

// upsertIntegrationIdentity 驗證 upsert integration 身分。
func upsertIntegrationIdentity(t *testing.T, store repository.Store, tenantID, accountID string, now time.Time) {
	t.Helper()
	if err := store.UpsertUserIdentity(context.Background(), domain.UserIdentity{
		ID:        "uid_" + tenantID + "_" + accountID,
		TenantID:  tenantID,
		AccountID: accountID,
		Provider:  "keycloak",
		Subject:   accountID,
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
}

type integrationTokenResolver struct{}

// Resolve 驗證目標路徑。
func (integrationTokenResolver) Resolve(req *http.Request) (v1api.TokenContext, bool, error) {
	const prefix = "Bearer "
	header := strings.TrimSpace(req.Header.Get("Authorization"))
	if !strings.HasPrefix(header, prefix) {
		return v1api.TokenContext{}, false, nil
	}
	tenantID, accountID, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(header, prefix)), ":")
	if !ok || tenantID == "" || accountID == "" {
		return v1api.TokenContext{}, false, nil
	}
	return v1api.TokenContext{
		Provider: "keycloak",
		Subject:  accountID,
		TenantID: tenantID,
	}, true, nil
}

// findIntegrationAuditLog 驗證 find integration 稽核 log。
func findIntegrationAuditLog(logs []domain.AuditLog, action string) (domain.AuditLog, bool) {
	for _, log := range logs {
		if log.Action == action {
			return log, true
		}
	}
	return domain.AuditLog{}, false
}

// installIntegrationSpanRecorder 驗證 install integration span recorder。
func installIntegrationSpanRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	})
	return recorder
}

// integrationSpanWithTrace 驗證 integration span with trace。
func integrationSpanWithTrace(recorder *tracetest.SpanRecorder, traceID, name string) bool {
	for _, span := range recorder.Ended() {
		if span.SpanContext().TraceID().String() == traceID && span.Name() == name {
			return true
		}
	}
	return false
}

// integrationSpanWithTracePrefix 驗證 integration span with trace prefix。
func integrationSpanWithTracePrefix(recorder *tracetest.SpanRecorder, traceID, prefix string) bool {
	for _, span := range recorder.Ended() {
		if span.SpanContext().TraceID().String() == traceID && strings.HasPrefix(span.Name(), prefix) {
			return true
		}
	}
	return false
}

// integrationTraceIDForSpanNamePrefix 驗證 integration trace ID for span 名稱 prefix。
func integrationTraceIDForSpanNamePrefix(recorder *tracetest.SpanRecorder, prefix string) string {
	for _, span := range recorder.Ended() {
		if strings.HasPrefix(span.Name(), prefix) {
			return span.SpanContext().TraceID().String()
		}
	}
	return ""
}

// integrationSpanNames 驗證 integration span names。
func integrationSpanNames(recorder *tracetest.SpanRecorder) []string {
	names := make([]string, 0)
	for _, span := range recorder.Ended() {
		names = append(names, span.Name())
	}
	sort.Strings(names)
	return names
}
