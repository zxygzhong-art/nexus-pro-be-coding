package ehrms_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/platform/ehrms"
)

// TestListEmployeesCoercesNonStringFields 驗證員工 JSON 出現陣列/布林/數字欄位時仍可解碼並正規化。
func TestListEmployeesCoercesNonStringFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/employees" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{
			"emp_id":"IKM001",
			"name_zh":"王小明",
			"clock_required":true,
			"dept_code":"C01",
			"tags":["remote","contractor"],
			"headcount":3,
			"quit_date":null
		}]`))
	}))
	defer server.Close()

	client, err := ehrms.NewClient(server.URL, "secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.WithRequestInterval(0)
	rows, err := client.ListEmployees(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one employee row, got %+v", rows)
	}
	row := rows[0]
	if row["員工編號"] != "IKM001" || row["中文姓名"] != "王小明" || row["部門代碼"] != "C01" {
		t.Fatalf("expected normalized employee identity fields, got %+v", row)
	}
	if row["上下班刷卡"] != "true" || row["tags"] != "remote, contractor" || row["headcount"] != "3" || row["離職日期"] != "" {
		t.Fatalf("expected coerced non-string fields, got %+v", row)
	}
}

// TestListAttendanceFetchesAndNormalizesEnglishFields 驗證 attendance endpoint and field normalization。
func TestListAttendanceFetchesAndNormalizesEnglishFields(t *testing.T) {
	var gotPath string
	var gotAPIKey string
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAPIKey = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{
			"emp_id":"IKM017",
			"date":"2026-06-08",
			"shift_start":"09:00",
			"shift_end":"18:00",
			"shift_hours":8,
			"daily_hours":8,
			"clock_hours":8
		}]`))
	}))
	defer server.Close()

	client, err := ehrms.NewClient(server.URL, "secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.WithRequestInterval(0)
	rows, err := client.ListAttendance(context.Background(), domain.EHRMSAttendanceQuery{
		EmployeeID: "IKM017",
		Start:      "2026-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/attendance" || gotQuery != "emp_id=IKM017&start=2026-01-01" || gotAPIKey != "secret" {
		t.Fatalf("unexpected request: path=%s query=%s apiKey=%s", gotPath, gotQuery, gotAPIKey)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one attendance row, got %+v", rows)
	}
	row := rows[0]
	if row["員工編號"] != "IKM017" || row["日期"] != "2026-06-08" || row["班別開始"] != "09:00" || row["刷卡工時"] != "8" {
		t.Fatalf("expected normalized attendance fields, got %+v", row)
	}
}

// TestRequestErrorClassifiesRetryableStatuses 驗證只重試 429 與 5xx，不重試認證錯誤。
func TestRequestErrorClassifiesRetryableStatuses(t *testing.T) {
	for _, tt := range []struct {
		status    int
		temporary bool
	}{{http.StatusUnauthorized, false}, {http.StatusTooManyRequests, true}, {http.StatusServiceUnavailable, true}} {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "upstream detail", tt.status) }))
			defer server.Close()
			client, err := ehrms.NewClient(server.URL, "secret", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			client.WithRequestInterval(0)
			_, err = client.ListEmployees(context.Background())
			var requestErr *ehrms.RequestError
			if !errors.As(err, &requestErr) || requestErr.Temporary() != tt.temporary {
				t.Fatalf("err=%v temporary=%v, want %v", err, requestErr != nil && requestErr.Temporary(), tt.temporary)
			}
			if strings.Contains(err.Error(), "upstream detail") {
				t.Fatal("unexpected raw upstream detail exposure")
			}
		})
	}
}

func TestListDepartmentsAndPositions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/departments", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"code":"C01","name":"Root","parent_code":null,"closed":false,"depth":0},{"code":"C0101","name":"Child","parent_code":"C01","closed":true,"depth":1}]`))
	})
	mux.HandleFunc("/positions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"job_code":"0704","job_title_zh":"工程師","job_title_en":"Engineer","headcount":3}]`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := ehrms.NewClient(server.URL, "secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.WithRequestInterval(0)
	departments, err := client.ListDepartments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(departments) != 2 || departments[1]["部門代碼"] != "C0101" || departments[1]["上級部門代碼"] != "C01" || departments[1]["部門已關閉"] != "true" {
		t.Fatalf("unexpected departments: %+v", departments)
	}
	positions, err := client.ListPositions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 1 || positions[0]["職務代碼"] != "0704" || positions[0]["職務中文名稱"] != "工程師" {
		t.Fatalf("unexpected positions: %+v", positions)
	}
}

func TestListLeaveTypes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/leave-types", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "secret" {
			t.Fatalf("expected X-API-Key header, got %q", r.Header.Get("X-API-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"code":"annual","name":"特休假","name_en":"Annual Leave","max_value":14,"unit":"days"},{"leave_code":"sick_full","leave_type":"全薪病假","maxValue":30,"unit":"days"}]`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := ehrms.NewClient(server.URL, "secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.WithRequestInterval(0)
	rows, err := client.ListLeaveTypes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 leave types, got %+v", rows)
	}
	if rows[0]["假別代碼"] != "annual" || rows[0]["假別名稱"] != "特休假" || rows[0]["最大值"] != "14" || rows[0]["單位"] != "days" {
		t.Fatalf("unexpected leave type[0]: %+v", rows[0])
	}
	if rows[1]["假別代碼"] != "sick_full" || rows[1]["假別名稱"] != "全薪病假" || rows[1]["最大值"] != "30" {
		t.Fatalf("unexpected leave type[1]: %+v", rows[1])
	}
}

func TestListLeaveTypesAcceptsLeaveTypesEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/leave-types" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"leave_types":[{"code":"annual","name":"特休假","name_en":"Annual Leave"}]}`))
	}))
	defer server.Close()

	client, err := ehrms.NewClient(server.URL, "secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.WithRequestInterval(0)
	rows, err := client.ListLeaveTypes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["假別代碼"] != "annual" || rows[0]["假別名稱"] != "特休假" {
		t.Fatalf("unexpected enveloped leave types: %+v", rows)
	}
}

func TestListLeaveBalancesAndDetails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/leave-balance", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "secret" {
			t.Fatalf("expected X-API-Key header, got %q", r.Header.Get("X-API-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"emp_id":"IKM017","year":2026,"leave_type":"annual","unit":"days","quota":10,"used":2,"remaining":8,"grant_start":"2026-01-01","expire_date":"2026-12-31"}]`))
	})
	mux.HandleFunc("/leave-detail", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "secret" {
			t.Fatalf("expected X-API-Key header, got %q", r.Header.Get("X-API-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"emp_id":"IKM017","date":"2026-06-11","leave_type":"annual","start":"09:00","end":"13:00","hours":"4"}]`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := ehrms.NewClient(server.URL, "secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.WithRequestInterval(0)
	query := domain.EHRMSAttendanceQuery{EmployeeID: "IKM017", Start: "2026-01-01"}
	balances, err := client.ListLeaveBalances(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(balances) != 1 || balances[0]["員工編號"] != "IKM017" || balances[0]["假別"] != "annual" || balances[0]["餘額"] != "8" {
		t.Fatalf("unexpected leave balances: %+v", balances)
	}
	details, err := client.ListLeaveDetails(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(details) != 1 || details[0]["員工編號"] != "IKM017" || details[0]["日期"] != "2026-06-11" || details[0]["開始時間"] != "09:00" {
		t.Fatalf("unexpected leave details: %+v", details)
	}
}

func TestClientSerializesAndSpacesUpstreamRequests(t *testing.T) {
	var active int32
	var maxActive int32
	var startsMu sync.Mutex
	starts := make([]time.Time, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := atomic.AddInt32(&active, 1)
		for {
			maximum := atomic.LoadInt32(&maxActive)
			if current <= maximum || atomic.CompareAndSwapInt32(&maxActive, maximum, current) {
				break
			}
		}
		startsMu.Lock()
		starts = append(starts, time.Now())
		startsMu.Unlock()
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&active, -1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client, err := ehrms.NewClient(server.URL, "secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.WithRequestInterval(20 * time.Millisecond)
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, callErr := client.ListEmployees(context.Background())
			errs <- callErr
		}()
	}
	wg.Wait()
	close(errs)
	for callErr := range errs {
		if callErr != nil {
			t.Fatal(callErr)
		}
	}
	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Fatalf("maximum concurrent upstream requests = %d, want 1", got)
	}
	startsMu.Lock()
	sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })
	gotStarts := append([]time.Time(nil), starts...)
	startsMu.Unlock()
	if len(gotStarts) != 3 {
		t.Fatalf("request starts = %d, want 3", len(gotStarts))
	}
	for i := 1; i < len(gotStarts); i++ {
		if gap := gotStarts[i].Sub(gotStarts[i-1]); gap < 15*time.Millisecond {
			t.Fatalf("request start gap = %s, want at least 15ms", gap)
		}
	}
}
