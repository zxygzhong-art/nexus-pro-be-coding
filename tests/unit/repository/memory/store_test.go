package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository"
	"nexus-pro-api/internal/repository/memory"
)

// TestNextEmployeeNoIncrementsAcrossCalls 驗證 next 員工 no increments across calls。
func TestNextEmployeeNoIncrementsAcrossCalls(t *testing.T) {
	store := memory.NewStore()
	ctx := context.Background()
	if err := store.UpsertEmployee(ctx, domain.Employee{
		ID:         "emp-1",
		TenantID:   "tenant-1",
		EmployeeNo: "IKL002",
		CreatedAt:  time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	first, err := store.NextEmployeeNo(ctx, "tenant-1", "IKL")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.NextEmployeeNo(ctx, "tenant-1", "IKL")
	if err != nil {
		t.Fatal(err)
	}

	if first != "IKL003" || second != "IKL004" {
		t.Fatalf("NextEmployeeNo() = %q then %q, want IKL003 then IKL004", first, second)
	}
}

// TestEHRMSSyncLockerIsNonBlocking 驗證同步互斥不依賴運行資料儲存。
func TestEHRMSSyncLockerIsNonBlocking(t *testing.T) {
	store := memory.NewStore()
	ctx := context.Background()

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		acquired, err := store.WithEHRMSSyncLock(ctx, "tenant-1", "pipeline", func() error { close(entered); <-release; return nil })
		if err != nil || !acquired {
			t.Errorf("first lock acquired=%v err=%v", acquired, err)
		}
	}()
	<-entered
	acquired, err := store.WithEHRMSSyncLock(ctx, "tenant-1", "pipeline", func() error { return nil })
	if err != nil || acquired {
		t.Fatalf("second lock acquired=%v err=%v", acquired, err)
	}
	close(release)
	<-done
}

// TestListEmployeePageByQueryMatchesMemoryFiltering 驗證員工分頁 by 查詢 matches memory filtering。
func TestListEmployeePageByQueryMatchesMemoryFiltering(t *testing.T) {
	store := memory.NewStore()
	ctx := context.Background()
	now := time.Now()
	employees := []domain.Employee{
		{ID: "emp-1", TenantID: "tenant-1", EmployeeNo: "IKL001", Name: "One", Status: "active", CreatedAt: now},
		{ID: "emp-2", TenantID: "tenant-1", EmployeeNo: "IKL002", Name: "Two", Status: "active", CreatedAt: now.Add(time.Minute)},
		{ID: "emp-3", TenantID: "tenant-1", EmployeeNo: "IKL003", Name: "Deleted", Status: "deleted", CreatedAt: now.Add(2 * time.Minute)},
	}
	for _, employee := range employees {
		if err := store.UpsertEmployee(ctx, employee); err != nil {
			t.Fatal(err)
		}
	}

	items, total, err := store.ListEmployeePageByQuery(ctx, "tenant-1", domain.EmployeeQuery{
		Page:     1,
		PageSize: 1,
		Sort:     "created_at_desc",
	})
	if err != nil {
		t.Fatal(err)
	}

	if total != 2 {
		t.Fatalf("total = %d, want 2 active employees", total)
	}
	if len(items) != 1 || items[0].ID != "emp-2" {
		t.Fatalf("items = %#v, want newest active employee", items)
	}

	scoped, scopedTotal, err := store.ListEmployeePageByQuery(ctx, "tenant-1", domain.EmployeeQuery{
		Page:     1,
		PageSize: 2,
		Scope:    domain.EmployeeScopeConstraint{EmployeeIDs: []string{"emp-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if scopedTotal != 1 || len(scoped) != 1 || scoped[0].ID != "emp-1" {
		t.Fatalf("scoped page = %#v total=%d, want only emp-1", scoped, scopedTotal)
	}
}

// TestWithTenantTransactionCommitsAndRollsBack 驗證租戶 transaction commits and rolls back。
func TestWithTenantTransactionCommitsAndRollsBack(t *testing.T) {
	store := memory.NewStore()
	ctx := context.Background()
	now := time.Now()

	err := store.WithTenantTransaction(ctx, "tenant-1", func(tx repository.Store) error {
		return tx.UpsertTenant(ctx, domain.Tenant{ID: "tenant-rollback", Name: "Rollback", CreatedAt: now})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetTenant(ctx, "tenant-rollback"); err != nil || !ok {
		t.Fatalf("expected committed tenant, ok=%v err=%v", ok, err)
	}

	err = store.WithTenantTransaction(ctx, "tenant-1", func(tx repository.Store) error {
		if err := tx.UpsertTenant(ctx, domain.Tenant{ID: "tenant-error", Name: "Error", CreatedAt: now}); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected transaction error")
	}
	if _, ok, err := store.GetTenant(ctx, "tenant-error"); err != nil || ok {
		t.Fatalf("expected error transaction to roll back, ok=%v err=%v", ok, err)
	}
}

// TestWithTenantTransactionRollsBackPanic 驗證租戶 transaction rolls back panic。
func TestWithTenantTransactionRollsBackPanic(t *testing.T) {
	store := memory.NewStore()
	ctx := context.Background()
	now := time.Now()

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("expected panic from transaction body")
		}
		if _, ok, err := store.GetTenant(ctx, "tenant-panic"); err != nil || ok {
			t.Fatalf("expected panic transaction to roll back, ok=%v err=%v", ok, err)
		}
	}()

	_ = store.WithTenantTransaction(ctx, "tenant-1", func(tx repository.Store) error {
		if err := tx.UpsertTenant(ctx, domain.Tenant{ID: "tenant-panic", Name: "Panic", CreatedAt: now}); err != nil {
			return err
		}
		panic("force rollback")
	})
}

// TestAttendanceClockRecordMultiPunchBoundariesAndIdempotency 驗證多次打卡邊界、作廢排除及客戶端事件冪等性。
func TestAttendanceClockRecordMultiPunchBoundariesAndIdempotency(t *testing.T) {
	store := memory.NewStore()
	ctx := context.Background()
	base := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	records := []domain.AttendanceClockRecord{
		{ID: "acr-in-first", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10", Direction: "clock_in", ClientEventID: "evt-in-first", ClockedAt: base, RecordStatus: "accepted", Source: "geofence", CreatedAt: base},
		{ID: "acr-out-middle", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10", Direction: "clock_out", ClientEventID: "evt-out-middle", ClockedAt: base.Add(2 * time.Hour), RecordStatus: "accepted", Source: "geofence", CreatedAt: base.Add(2 * time.Hour)},
		{ID: "acr-in-middle", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10", Direction: "clock_in", ClientEventID: "evt-in-middle", ClockedAt: base.Add(4 * time.Hour), RecordStatus: "accepted", Source: "geofence", CreatedAt: base.Add(4 * time.Hour)},
		{ID: "acr-out-last", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10", Direction: "clock_out", ClientEventID: "evt-out-last", ClockedAt: base.Add(10 * time.Hour), RecordStatus: "accepted", Source: "geofence", CreatedAt: base.Add(10 * time.Hour)},
		{ID: "acr-out-last-z", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10", Direction: "clock_out", ClientEventID: "evt-out-last-z", ClockedAt: base.Add(10 * time.Hour), RecordStatus: "accepted", Source: "geofence", CreatedAt: base.Add(10 * time.Hour)},
		{ID: "acr-out-voided", TenantID: "tenant-1", EmployeeID: "emp-1", WorkDate: "2026-06-10", Direction: "clock_out", ClientEventID: "evt-out-voided", ClockedAt: base.Add(11 * time.Hour), RecordStatus: "accepted", Source: "geofence", Voided: true, CreatedAt: base.Add(11 * time.Hour)},
	}
	for _, record := range records {
		if err := store.UpsertAttendanceClockRecord(ctx, record); err != nil {
			t.Fatal(err)
		}
	}

	duplicateEvent := records[0]
	duplicateEvent.ID = "acr-duplicate-event"
	err := store.UpsertAttendanceClockRecord(ctx, duplicateEvent)
	if appErr, ok := domain.AsAppError(err); !ok || appErr.Code != "conflict" {
		t.Fatalf("expected client event conflict, got %v", err)
	}
	byEvent, ok, err := store.GetAttendanceClockRecordByClientEventID(ctx, "tenant-1", "evt-out-middle")
	if err != nil || !ok || byEvent.ID != "acr-out-middle" {
		t.Fatalf("expected idempotency lookup, ok=%v record=%+v err=%v", ok, byEvent, err)
	}
	earliestIn, ok, err := store.GetEarliestAcceptedAttendanceClockIn(ctx, "tenant-1", "emp-1", "2026-06-10")
	if err != nil || !ok || earliestIn.ID != "acr-in-first" {
		t.Fatalf("expected earliest clock-in, ok=%v record=%+v err=%v", ok, earliestIn, err)
	}
	latestOut, ok, err := store.GetLatestAcceptedAttendanceClockOut(ctx, "tenant-1", "emp-1", "2026-06-10")
	if err != nil || !ok || latestOut.ID != "acr-out-last-z" {
		t.Fatalf("expected latest non-voided clock-out, ok=%v record=%+v err=%v", ok, latestOut, err)
	}
	latest, ok, err := store.GetLatestAcceptedAttendanceClockRecord(ctx, "tenant-1", "emp-1", "2026-06-10")
	if err != nil || !ok || latest.ID != "acr-out-last-z" {
		t.Fatalf("expected latest non-voided clock record, ok=%v record=%+v err=%v", ok, latest, err)
	}
}

// TestUpsertLeaveBalanceUsesEmployeeTypeAndPeriodIdentity verifies period balances stay independent.
func TestUpsertLeaveBalanceUsesEmployeeTypeAndPeriodIdentity(t *testing.T) {
	store := memory.NewStore()
	ctx := context.Background()
	now := time.Now().UTC()
	first := domain.LeaveBalance{
		ID: "policy-balance", TenantID: "tenant-1", EmployeeID: "emp-1", LeaveType: "annual",
		RemainingHours: 8, Source: "policy", UpdatedAt: now,
	}
	if err := store.UpsertLeaveBalance(ctx, first); err != nil {
		t.Fatal(err)
	}
	fromEHRMS := first
	fromEHRMS.ID = "ehrms-balance"
	fromEHRMS.RemainingHours = 24
	fromEHRMS.Source = "ehrms"
	fromEHRMS.UpdatedAt = now.Add(time.Minute)
	if err := store.UpsertLeaveBalance(ctx, fromEHRMS); err != nil {
		t.Fatal(err)
	}

	balances, err := store.ListLeaveBalances(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(balances) != 1 {
		t.Fatalf("expected one balance for employee/type identity, got %+v", balances)
	}
	if balances[0].ID != first.ID || balances[0].RemainingHours != 24 || balances[0].Source != "ehrms" {
		t.Fatalf("expected existing balance identity with eHRMS values, got %+v", balances[0])
	}

	nextPeriod := first
	nextPeriod.ID = "next-period"
	nextPeriod.PeriodStart = "2027-01-01"
	nextPeriod.PeriodEnd = "2027-12-31"
	if err := store.UpsertLeaveBalance(ctx, nextPeriod); err != nil {
		t.Fatal(err)
	}
	balances, err = store.ListLeaveBalances(ctx, "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(balances) != 2 {
		t.Fatalf("expected a new balance identity for another period, got %+v", balances)
	}
}

// TestPlatformTaskStoreScopesRecordsByAccount 驗證平臺任務儲存層範圍 records by 帳號。
func TestPlatformTaskStoreScopesRecordsByAccount(t *testing.T) {
	store := memory.NewStore()
	ctx := context.Background()
	now := time.Now()
	item := domain.PlatformTaskRecordItem{
		ID:        "pti-1",
		TenantID:  "tenant-1",
		AccountID: "acct-a",
		WorkDate:  "2026/07/01",
		Title:     "Owner task",
		Category:  "Backend",
		Product:   "Nexus",
		Hours:     1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.UpsertPlatformTaskItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetPlatformTaskItem(ctx, "tenant-1", "acct-b", "pti-1"); err != nil || ok {
		t.Fatalf("expected cross-account task item lookup to miss, ok=%v err=%v", ok, err)
	}
	if err := store.DeletePlatformTaskItem(ctx, "tenant-1", "acct-b", "pti-1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetPlatformTaskItem(ctx, "tenant-1", "acct-a", "pti-1"); err != nil || !ok {
		t.Fatalf("expected owner task item to remain after cross-account delete, ok=%v err=%v", ok, err)
	}
	item.AccountID = "acct-b"
	if appErr, ok := domain.AsAppError(store.UpsertPlatformTaskItem(ctx, item)); !ok || appErr.Code != "conflict" {
		t.Fatalf("expected cross-account task item upsert conflict, got %v", appErr)
	}

	todo := domain.PlatformTaskTodoRecord{
		ID:        "ptd-1",
		TenantID:  "tenant-1",
		AccountID: "acct-a",
		Text:      "Owner todo",
		Status:    "open",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.UpsertPlatformTaskTodo(ctx, todo); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetPlatformTaskTodo(ctx, "tenant-1", "acct-b", "ptd-1"); err != nil || ok {
		t.Fatalf("expected cross-account task todo lookup to miss, ok=%v err=%v", ok, err)
	}
	if err := store.DeletePlatformTaskTodo(ctx, "tenant-1", "acct-b", "ptd-1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetPlatformTaskTodo(ctx, "tenant-1", "acct-a", "ptd-1"); err != nil || !ok {
		t.Fatalf("expected owner task todo to remain after cross-account delete, ok=%v err=%v", ok, err)
	}
	todo.AccountID = "acct-b"
	if appErr, ok := domain.AsAppError(store.UpsertPlatformTaskTodo(ctx, todo)); !ok || appErr.Code != "conflict" {
		t.Fatalf("expected cross-account task todo upsert conflict, got %v", appErr)
	}
}

// TestUserIdentityLookupAndList 驗證使用者身分 lookup and 列表。
func TestUserIdentityLookupAndList(t *testing.T) {
	store := memory.NewStore()
	ctx := context.Background()
	now := time.Now()
	identity := domain.UserIdentity{
		ID:        "uid-1",
		TenantID:  "tenant-1",
		AccountID: "acct-1",
		Provider:  "google",
		Subject:   "google-subject",
		Email:     "user@example.com",
		CreatedAt: now,
	}
	if err := store.UpsertUserIdentity(ctx, identity); err != nil {
		t.Fatal(err)
	}

	got, ok, err := store.GetUserIdentity(ctx, "tenant-1", "google", "google-subject")
	if err != nil || !ok || got.AccountID != "acct-1" {
		t.Fatalf("expected identity lookup to resolve account, got=%+v ok=%v err=%v", got, ok, err)
	}
	items, err := store.ListUserIdentities(ctx, "tenant-1", "acct-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Subject != "google-subject" {
		t.Fatalf("expected one listed identity, got %+v", items)
	}
}

// TestOptimisticLockingRejectsStaleWrites 驗證樂觀鎖拒絕過期版本的寫入。
func TestOptimisticLockingRejectsStaleWrites(t *testing.T) {
	store := memory.NewStore()
	ctx := context.Background()
	now := time.Now()

	account := domain.Account{ID: "acct-1", TenantID: "tenant-1", DisplayName: "A", Status: "active", CreatedAt: now}
	if err := store.UpsertAccount(ctx, account); err != nil {
		t.Fatal(err)
	}

	first, ok, err := store.GetAccount(ctx, "tenant-1", "acct-1")
	if err != nil || !ok || first.Version != 1 {
		t.Fatalf("expected version 1 after insert, got=%+v ok=%v err=%v", first, ok, err)
	}
	second := first

	first.DisplayName = "writer-1"
	if err := store.UpsertAccount(ctx, first); err != nil {
		t.Fatal(err)
	}

	second.DisplayName = "writer-2"
	err = store.UpsertAccount(ctx, second)
	appErr, isApp := domain.AsAppError(err)
	if !isApp || appErr.Status != 409 {
		t.Fatalf("expected 409 conflict for stale write, got %v", err)
	}

	// 盲寫(version 0)不受樂觀鎖限制,維持既有 upsert 語義。
	blind := domain.Account{ID: "acct-1", TenantID: "tenant-1", DisplayName: "blind", Status: "active", CreatedAt: now}
	if err := store.UpsertAccount(ctx, blind); err != nil {
		t.Fatal(err)
	}
	got, _, err := store.GetAccount(ctx, "tenant-1", "acct-1")
	if err != nil || got.Version != 3 {
		t.Fatalf("expected version 3 after blind write, got=%+v err=%v", got, err)
	}

	group := domain.UserGroup{ID: "grp-1", TenantID: "tenant-1", Name: "G", CreatedAt: now}
	if err := store.UpsertUserGroup(ctx, group); err != nil {
		t.Fatal(err)
	}
	g1, _, _ := store.GetUserGroup(ctx, "tenant-1", "grp-1")
	g2 := g1
	g1.Name = "G1"
	if err := store.UpsertUserGroup(ctx, g1); err != nil {
		t.Fatal(err)
	}
	g2.Name = "G2"
	if appErr, ok := domain.AsAppError(store.UpsertUserGroup(ctx, g2)); !ok || appErr.Status != 409 {
		t.Fatalf("expected 409 for stale user group write")
	}

	instance := domain.FormInstance{ID: "fi-1", TenantID: "tenant-1", TemplateID: "ft-1", ApplicantAccountID: "acct-1", Status: "draft", SubmittedAt: now, UpdatedAt: now}
	if err := store.UpsertFormInstance(ctx, instance); err != nil {
		t.Fatal(err)
	}
	f1, _, _ := store.GetFormInstance(ctx, "tenant-1", "fi-1")
	f2 := f1
	f1.Status = "in_review"
	if err := store.UpsertFormInstance(ctx, f1); err != nil {
		t.Fatal(err)
	}
	f2.Status = "approved"
	if appErr, ok := domain.AsAppError(store.UpsertFormInstance(ctx, f2)); !ok || appErr.Status != 409 {
		t.Fatalf("expected 409 for stale form instance write")
	}
}
