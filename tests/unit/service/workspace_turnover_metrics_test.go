package service_test

import (
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
)

// workspaceMonthlyTotalRow 取月度人員異動總計列。
func workspaceMonthlyTotalRow(t *testing.T, item domain.WorkspaceTurnoverMonthly) domain.WorkspaceTurnoverRow {
	t.Helper()
	if len(item.Rows) == 0 {
		t.Fatal("expected at least one monthly row")
	}
	row := item.Rows[len(item.Rows)-1]
	if row.RowType != "total" {
		t.Fatalf("expected last monthly row to be total, got %+v", row)
	}
	return row
}

// workspaceAnnualTotalRow 取年度人員異動總計列。
func workspaceAnnualTotalRow(t *testing.T, item domain.WorkspaceTurnoverAnnual) domain.WorkspaceAnnualRow {
	t.Helper()
	if len(item.Rows) == 0 {
		t.Fatal("expected at least one annual row")
	}
	row := item.Rows[len(item.Rows)-1]
	if row.BU != "總計" {
		t.Fatalf("expected last annual row to be grand total, got %+v", row)
	}
	return row
}

// workspaceKPIByKey 依 key 取 KPI。
func workspaceKPIByKey(t *testing.T, kpis []domain.WorkspaceKPI, key string) domain.WorkspaceKPI {
	t.Helper()
	for _, kpi := range kpis {
		if kpi.Key == key {
			return kpi
		}
	}
	t.Fatalf("missing KPI %q in %+v", key, kpis)
	return domain.WorkspaceKPI{}
}

func insertTurnoverEmployee(t *testing.T, store *memory.Store, id string, status string, hireDate *time.Time, resignDate *time.Time, updatedAt time.Time) {
	t.Helper()
	insertWorkspaceEmployee(t, store, domain.Employee{
		ID:               id,
		EmployeeNo:       id,
		Name:             id,
		Status:           status,
		EmploymentStatus: status,
		HireDate:         hireDate,
		ResignDate:       resignDate,
		CreatedAt:        updatedAt,
		UpdatedAt:        updatedAt,
	})
}

// TestWorkspaceTurnoverAnnualClosesHeadcountIdentity 驗證年度視圖滿足
// 「年初在職 + 年新進 − 年離職 = 年末在職」閉合恆等式，且年淨增減與年末−年初一致。
// 涵蓋：整年在職、年中新進、年中離職、年內新進又離職、無 resign_date 的離職
// （以 updated_at 近似離職時間，需同時計入年初在職才閉合）、未到職 onboarding，
// 以及 resign_date 在往年但 updated_at 被批次同步刷到今年的幻影離職（不得重複計入）。
func TestWorkspaceTurnoverAnnualClosesHeadcountIdentity(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	hire := func(y int, m time.Month, d int) *time.Time { return ptrTime(time.Date(y, m, d, 0, 0, 0, 0, time.UTC)) }
	at := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

	insertTurnoverEmployee(t, store, "emp-stay", "active", hire(2024, 3, 1), nil, at(2026, 1, 5))
	insertTurnoverEmployee(t, store, "emp-hire", "active", hire(2026, 4, 10), nil, at(2026, 4, 10))
	insertTurnoverEmployee(t, store, "emp-resign", "resigned", hire(2023, 1, 1), hire(2026, 3, 15), at(2026, 3, 15))
	insertTurnoverEmployee(t, store, "emp-hire-resign", "resigned", hire(2026, 2, 1), hire(2026, 5, 20), at(2026, 5, 20))
	// 幻影案例：2024 年已離職，2026-07 批次同步刷新 updated_at，不得計入 2026 任何指標。
	insertTurnoverEmployee(t, store, "emp-phantom", "resigned", hire(2020, 1, 1), hire(2024, 6, 1), at(2026, 7, 10))
	// 無 resign_date 的歷史匯入離職：以 updated_at 為有效離職時間。
	insertTurnoverEmployee(t, store, "emp-undated", "resigned", hire(2019, 1, 1), nil, at(2026, 6, 5))
	insertTurnoverEmployee(t, store, "emp-future", "onboarding", hire(2027, 1, 5), nil, at(2026, 6, 1))

	got, err := svc.Workspace().WorkspaceTurnover(ctx, domain.WorkspaceTurnoverQuery{Year: 2026, Month: 7, AnnualYear: 2026})
	if err != nil {
		t.Fatal(err)
	}

	total := workspaceAnnualTotalRow(t, got.Annual)
	if total.Base != 3 || total.Hires != 2 || total.Resigned != 3 || total.Layoff != 0 || total.End != 2 {
		t.Fatalf("unexpected annual total: %+v", total)
	}
	if total.Base+total.Hires-total.Resigned-total.Layoff != total.End {
		t.Fatalf("annual identity broken: base(%d)+hires(%d)-resigned(%d)-layoff(%d) != end(%d)",
			total.Base, total.Hires, total.Resigned, total.Layoff, total.End)
	}
	for _, row := range got.Annual.Rows {
		if row.Base+row.Hires-row.Resigned-row.Layoff != row.End {
			t.Fatalf("annual row identity broken: %+v", row)
		}
	}
	net := workspaceKPIByKey(t, got.Annual.KPIs, "net")
	if net.Value != "-1" {
		t.Fatalf("expected net KPI -1 (hires 2 - separations 3), got %+v", net)
	}
	if total.End-total.Base != -1 {
		t.Fatalf("expected end-base delta -1 to match net KPI, got %+v", total)
	}
}

// TestWorkspaceTurnoverMonthlyExcludesStaleUpdatedAtSeparations 驗證月視圖不把
// 「resign_date 在往年、僅 updated_at 落入本月」的員工重複計入本月離職，
// 且月度數字同樣滿足閉合恆等式。
func TestWorkspaceTurnoverMonthlyExcludesStaleUpdatedAtSeparations(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	hire := func(y int, m time.Month, d int) *time.Time { return ptrTime(time.Date(y, m, d, 0, 0, 0, 0, time.UTC)) }
	at := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

	insertTurnoverEmployee(t, store, "emp-old-resign", "resigned", hire(2020, 1, 1), hire(2024, 6, 1), at(2026, 7, 10))
	insertTurnoverEmployee(t, store, "emp-july-resign", "resigned", hire(2024, 1, 1), hire(2026, 7, 12), at(2026, 7, 12))
	insertTurnoverEmployee(t, store, "emp-stay", "active", hire(2024, 1, 1), nil, at(2026, 1, 1))
	insertTurnoverEmployee(t, store, "emp-july-hire", "active", hire(2026, 7, 3), nil, at(2026, 7, 3))

	got, err := svc.Workspace().WorkspaceTurnover(ctx, domain.WorkspaceTurnoverQuery{Year: 2026, Month: 7, AnnualYear: 2026})
	if err != nil {
		t.Fatal(err)
	}

	total := workspaceMonthlyTotalRow(t, got.Monthly)
	if total.Prev != 2 || total.Hires != 1 || total.Resigned != 1 || total.End != 2 {
		t.Fatalf("unexpected monthly total: %+v", total)
	}
	if total.Prev+total.Hires-total.Resigned-total.Layoff != total.End {
		t.Fatalf("monthly identity broken: %+v", total)
	}
	if total.MonthRate != "50.0%" {
		t.Fatalf("expected month rate 50.0%% (1 ÷ avg(2,2)), got %s", total.MonthRate)
	}
	rate := workspaceKPIByKey(t, got.Monthly.Stats, "rate")
	if rate.Value != "50.0" {
		t.Fatalf("expected rate KPI 50.0, got %+v", rate)
	}
}

// TestWorkspaceTurnoverRateUsesAverageHeadcountDenominator 驗證離職率統一口徑：
// 期間離職 ÷ 期間平均在職（期初與期末快照平均），月報、YTD 與年度一致。
func TestWorkspaceTurnoverRateUsesAverageHeadcountDenominator(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	hire := func(y int, m time.Month, d int) *time.Time { return ptrTime(time.Date(y, m, d, 0, 0, 0, 0, time.UTC)) }
	at := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

	insertTurnoverEmployee(t, store, "emp-a", "active", hire(2024, 1, 1), nil, at(2026, 1, 1))
	insertTurnoverEmployee(t, store, "emp-b", "active", hire(2024, 1, 1), nil, at(2026, 1, 1))
	insertTurnoverEmployee(t, store, "emp-c", "resigned", hire(2024, 1, 1), hire(2026, 7, 5), at(2026, 7, 5))
	insertTurnoverEmployee(t, store, "emp-d", "resigned", hire(2024, 1, 1), hire(2026, 7, 19), at(2026, 7, 19))

	got, err := svc.Workspace().WorkspaceTurnover(ctx, domain.WorkspaceTurnoverQuery{Year: 2026, Month: 7, AnnualYear: 2026})
	if err != nil {
		t.Fatal(err)
	}

	// 月初 4 人、離職 2 人、月末 2 人：平均在職 3 人，離職率 = 2/3 = 66.7%。
	monthly := workspaceMonthlyTotalRow(t, got.Monthly)
	if monthly.Prev != 4 || monthly.Resigned != 2 || monthly.End != 2 {
		t.Fatalf("unexpected monthly total: %+v", monthly)
	}
	if monthly.MonthRate != "66.7%" {
		t.Fatalf("expected month rate 66.7%% (2 ÷ avg(4,2)), got %s", monthly.MonthRate)
	}
	if monthly.YTDRate != "66.7%" {
		t.Fatalf("expected YTD rate 66.7%% (2 ÷ avg(4,2)), got %s", monthly.YTDRate)
	}
	rate := workspaceKPIByKey(t, got.Monthly.Stats, "rate")
	if rate.Value != "66.7" {
		t.Fatalf("expected rate KPI 66.7, got %+v", rate)
	}

	annual := workspaceAnnualTotalRow(t, got.Annual)
	if annual.Base != 4 || annual.Resigned != 2 || annual.End != 2 {
		t.Fatalf("unexpected annual total: %+v", annual)
	}
	if annual.Rate != "66.7%" {
		t.Fatalf("expected annual rate 66.7%% (2 ÷ avg(4,2)), got %s", annual.Rate)
	}
}

// TestWorkspaceOverviewSeparationRateMatchesTurnoverBasis 驗證概覽頁與人員異動頁
// 的離職率口徑一致：當月離職 ÷ 當月平均在職（月初與月末快照平均）。
func TestWorkspaceOverviewSeparationRateMatchesTurnoverBasis(t *testing.T) {
	store, svc, ctx := newWorkspaceFixture(t)
	hire := func(y int, m time.Month, d int) *time.Time { return ptrTime(time.Date(y, m, d, 0, 0, 0, 0, time.UTC)) }
	at := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

	insertTurnoverEmployee(t, store, "emp-a", "active", hire(2024, 1, 1), nil, at(2026, 1, 1))
	insertTurnoverEmployee(t, store, "emp-b", "active", hire(2024, 1, 1), nil, at(2026, 1, 1))
	insertTurnoverEmployee(t, store, "emp-c", "resigned", hire(2024, 1, 1), hire(2026, 7, 12), at(2026, 7, 12))

	overview, err := svc.Workspace().WorkspaceOverview(ctx, domain.WorkspaceOverviewQuery{Year: 2026, Month: 7})
	if err != nil {
		t.Fatal(err)
	}
	if overview.HRSummary.Active != 2 || overview.HRSummary.Separations != 1 {
		t.Fatalf("unexpected HR summary: %+v", overview.HRSummary)
	}
	// 月初 3 人、月末 2 人：平均在職 2.5 人，離職率 = 1/2.5 = 40.0%。
	if overview.HRSummary.SeparationRate != "40.0" {
		t.Fatalf("expected overview separation rate 40.0 (1 ÷ avg(3,2)), got %s", overview.HRSummary.SeparationRate)
	}

	turnover, err := svc.Workspace().WorkspaceTurnover(ctx, domain.WorkspaceTurnoverQuery{Year: 2026, Month: 7, AnnualYear: 2026})
	if err != nil {
		t.Fatal(err)
	}
	rate := workspaceKPIByKey(t, turnover.Monthly.Stats, "rate")
	if rate.Value != overview.HRSummary.SeparationRate {
		t.Fatalf("overview rate %s != turnover rate %s", overview.HRSummary.SeparationRate, rate.Value)
	}
}
