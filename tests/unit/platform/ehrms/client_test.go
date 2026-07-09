package ehrms_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"nexus-pro-be/internal/platform/ehrms"
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
