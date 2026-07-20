package ehrms_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nexus-pro-api/internal/platform/ehrms"
)

// TestListAttendanceFetchesAndNormalizesEnglishFields 驗證 attendance endpoint and field normalization。
func TestListAttendanceFetchesAndNormalizesEnglishFields(t *testing.T) {
	var gotPath string
	var gotAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
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
	rows, err := client.ListAttendance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/attendance" || gotAPIKey != "secret" {
		t.Fatalf("unexpected request path/header: path=%s apiKey=%s", gotPath, gotAPIKey)
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
	balances, err := client.ListLeaveBalances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(balances) != 1 || balances[0]["員工編號"] != "IKM017" || balances[0]["假別"] != "annual" || balances[0]["餘額"] != "8" {
		t.Fatalf("unexpected leave balances: %+v", balances)
	}
	details, err := client.ListLeaveDetails(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(details) != 1 || details[0]["員工編號"] != "IKM017" || details[0]["日期"] != "2026-06-11" || details[0]["開始時間"] != "09:00" {
		t.Fatalf("unexpected leave details: %+v", details)
	}
}
