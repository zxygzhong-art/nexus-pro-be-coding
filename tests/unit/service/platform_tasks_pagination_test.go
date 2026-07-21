package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// newPlatformTasksPaginationFixture 建立帶有多筆不同建立時間任務資料的測試環境。
func newPlatformTasksPaginationFixture(t *testing.T, now time.Time) (*memory.Store, *service.Service, domain.RequestContext) {
	t.Helper()
	store := memory.NewStore()
	if err := store.UpsertTenant(context.Background(), domain.Tenant{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPermissionSet(context.Background(), domain.PermissionSet{
		ID:       "ps-platform-tasks-page",
		TenantID: "tenant-1",
		Name:     "Platform Tasks Page",
		Permissions: []domain.Permission{
			{Resource: "me", Action: "read", Scope: "self"},
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(context.Background(), domain.Account{
		ID:                     "acct-1",
		TenantID:               "tenant-1",
		DisplayName:            "Task Pager",
		Status:                 "active",
		DirectPermissionSetIDs: []string{"ps-platform-tasks-page"},
		CreatedAt:              now,
	}); err != nil {
		t.Fatal(err)
	}
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := domain.RequestContext{TenantID: "tenant-1", AccountID: "acct-1"}
	return store, svc, ctx
}

// seedPlatformTaskItem 直接寫入帶指定建立時間的任務項目。
func seedPlatformTaskItem(t *testing.T, store *memory.Store, id, workDate string, createdAt time.Time) {
	t.Helper()
	if err := store.UpsertPlatformTaskItem(context.Background(), domain.PlatformTaskRecordItem{
		ID:        id,
		TenantID:  "tenant-1",
		AccountID: "acct-1",
		WorkDate:  workDate,
		Title:     "Task " + id,
		Category:  "Backend",
		Hours:     1,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}); err != nil {
		t.Fatal(err)
	}
}

// seedPlatformTaskTodo 直接寫入帶指定建立時間的任務待辦。
func seedPlatformTaskTodo(t *testing.T, store *memory.Store, id, status string, createdAt time.Time) {
	t.Helper()
	if err := store.UpsertPlatformTaskTodo(context.Background(), domain.PlatformTaskTodoRecord{
		ID:        id,
		TenantID:  "tenant-1",
		AccountID: "acct-1",
		Text:      "Todo " + id,
		Status:    status,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}); err != nil {
		t.Fatal(err)
	}
}

// platformTaskItemIDs 收集回應 items 的 id 清單。
func platformTaskItemIDs(items []domain.PlatformTaskItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

// TestPlatformTasksDefaultWindowExcludesOldRecords 驗證預設只回傳最近 90 天的資料。
func TestPlatformTasksDefaultWindowExcludesOldRecords(t *testing.T) {
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	store, svc, ctx := newPlatformTasksPaginationFixture(t, now)

	seedPlatformTaskItem(t, store, "pti-recent", "2026/07/01", now.Add(-time.Hour))
	seedPlatformTaskItem(t, store, "pti-old-91d", "2026/04/01", now.Add(-91*24*time.Hour))
	seedPlatformTaskItem(t, store, "pti-old-120d", "2026/03/01", now.Add(-120*24*time.Hour))
	seedPlatformTaskTodo(t, store, "ptd-recent", "open", now.Add(-2*time.Hour))
	seedPlatformTaskTodo(t, store, "ptd-old-100d", "open", now.Add(-100*24*time.Hour))

	tasks, err := svc.Platform().Tasks(ctx, domain.PlatformTasksQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := platformTaskItemIDs(tasks.Items), []string{"pti-recent"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected default window to keep only recent items, got %v", got)
	}
	if len(tasks.Todos) != 1 || tasks.Todos[0].ID != "ptd-recent" {
		t.Fatalf("expected default window to keep only recent todos, got %+v", tasks.Todos)
	}
	if tasks.NextCursor != "" {
		t.Fatalf("expected empty next_cursor when everything fits one page, got %q", tasks.NextCursor)
	}
	if tasks.Items[0].WorkDate != "2026/07/01" {
		t.Fatalf("expected flat items to carry work_date, got %+v", tasks.Items[0])
	}

	// 明確指定 from/to 可涵蓋更舊的資料。
	tasks, err = svc.Platform().Tasks(ctx, domain.PlatformTasksQuery{
		From: now.Add(-200 * 24 * time.Hour),
		To:   now.Add(-95 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := platformTaskItemIDs(tasks.Items), []string{"pti-old-120d"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected explicit window to select only pti-old-120d, got %v", got)
	}
}

// TestPlatformTasksCursorPagination 驗證 (created_at, id) keyset 分頁順序與遊標行為。
func TestPlatformTasksCursorPagination(t *testing.T) {
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	store, svc, ctx := newPlatformTasksPaginationFixture(t, now)

	// 以建立時間倒序應為 pti-1 .. pti-5；其中 pti-3 與 pti-4 同刻，以 id 倒序決勝。
	seedPlatformTaskItem(t, store, "pti-1", "2026/07/01", now.Add(-time.Hour))
	seedPlatformTaskItem(t, store, "pti-2", "2026/06/30", now.Add(-2*time.Hour))
	seedPlatformTaskItem(t, store, "pti-3", "2026/06/29", now.Add(-3*time.Hour))
	seedPlatformTaskItem(t, store, "pti-4", "2026/06/28", now.Add(-3*time.Hour))
	seedPlatformTaskItem(t, store, "pti-5", "2026/06/27", now.Add(-5*time.Hour))

	page1, err := svc.Platform().Tasks(ctx, domain.PlatformTasksQuery{PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := platformTaskItemIDs(page1.Items), []string{"pti-1", "pti-2"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected first page: got %v want %v", got, want)
	}
	if page1.NextCursor == "" {
		t.Fatal("expected next_cursor on first page")
	}
	// records 為同頁資料按日期分組的投影。
	if len(page1.Records) != 2 || page1.Records[0].Date != "2026/07/01" || page1.Records[1].Date != "2026/06/30" {
		t.Fatalf("expected records to group the current page by date desc, got %+v", page1.Records)
	}

	page2, err := svc.Platform().Tasks(ctx, domain.PlatformTasksQuery{PageSize: 2, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	// pti-3 與 pti-4 同 created_at，id 倒序 → pti-4 先。
	if got, want := platformTaskItemIDs(page2.Items), []string{"pti-4", "pti-3"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected second page: got %v want %v", got, want)
	}
	if page2.NextCursor == "" {
		t.Fatal("expected next_cursor on second page")
	}

	page3, err := svc.Platform().Tasks(ctx, domain.PlatformTasksQuery{PageSize: 2, Cursor: page2.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := platformTaskItemIDs(page3.Items), []string{"pti-5"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected third page: got %v want %v", got, want)
	}
	if page3.NextCursor != "" {
		t.Fatalf("expected empty next_cursor on last page, got %q", page3.NextCursor)
	}
}

// TestPlatformTasksQueryValidation 驗證查詢參數的錯誤處理與上限收斂。
func TestPlatformTasksQueryValidation(t *testing.T) {
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	store, svc, ctx := newPlatformTasksPaginationFixture(t, now)
	seedPlatformTaskItem(t, store, "pti-only", "2026/07/01", now.Add(-time.Hour))

	if _, err := svc.Platform().Tasks(ctx, domain.PlatformTasksQuery{Cursor: "not-a-cursor"}); err == nil {
		t.Fatal("expected invalid cursor to be rejected")
	}
	if _, err := svc.Platform().Tasks(ctx, domain.PlatformTasksQuery{
		From: now.Add(-24 * time.Hour),
		To:   now.Add(-48 * time.Hour),
	}); err == nil {
		t.Fatal("expected from after to to be rejected")
	}
	// page_size 超過上限時收斂到 MaxPageSize，而不是報錯。
	tasks, err := svc.Platform().Tasks(ctx, domain.PlatformTasksQuery{PageSize: domain.MaxPageSize + 500})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks.Items) != 1 {
		t.Fatalf("expected oversized page_size to be clamped, got %+v", tasks.Items)
	}
}
