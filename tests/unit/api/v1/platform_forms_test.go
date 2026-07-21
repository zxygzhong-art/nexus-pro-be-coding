package v1_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
)

// seedPlatformFormsFixture 在 demo fixture 上補一個表單範本與兩筆表單實例。
func seedPlatformFormsFixture(store *memory.Store, now time.Time) {
	_ = store.UpsertFormTemplate(context.Background(), domain.FormTemplate{
		ID:        "ft-leave",
		TenantID:  "demo",
		Key:       "leave-request",
		Name:      "請假申請單",
		Schema:    map[string]any{"enabled": true},
		CreatedAt: now,
	})
	_ = store.UpsertFormInstance(context.Background(), domain.FormInstance{
		ID:                 "fi-leave-1",
		TenantID:           "demo",
		TemplateID:         "ft-leave",
		ApplicantAccountID: "acct-admin",
		Status:             "in_review",
		Payload:            map[string]any{"reason": "家庭因素請假"},
		SubmittedAt:        now.Add(-time.Hour),
		UpdatedAt:          now.Add(-time.Hour),
	})
	_ = store.UpsertFormInstance(context.Background(), domain.FormInstance{
		ID:                 "fi-draft-1",
		TenantID:           "demo",
		TemplateID:         "ft-leave",
		ApplicantAccountID: "acct-admin",
		Status:             "draft",
		Payload:            map[string]any{"reason": "草稿一"},
		SubmittedAt:        now.Add(-2 * time.Hour),
		UpdatedAt:          now.Add(-30 * time.Minute),
	})
}

// TestPlatformFormsReturnsPagedEnvelopesWithoutPayload 驗證 forms 列表改為分頁信封且不含 payload。
func TestPlatformFormsReturnsPagedEnvelopesWithoutPayload(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	handler := newTestAPIForAccountNow("acct-admin", now, func(store *memory.Store) {
		seedPlatformFormsFixture(store, now)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/platform/forms", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for platform forms, got %d: %s", rec.Code, rec.Body.String())
	}
	forms := decodeData[domain.PlatformFormsResponse](t, rec.Body.Bytes())
	if forms.Applications.Total != 1 || forms.Applications.Page != 1 || forms.Applications.PageSize != domain.DefaultPageSize || len(forms.Applications.Items) != 1 {
		t.Fatalf("unexpected applications envelope: %+v", forms.Applications)
	}
	if forms.Drafts.Total != 1 || len(forms.Drafts.Items) != 1 {
		t.Fatalf("unexpected drafts envelope: %+v", forms.Drafts)
	}
	if forms.Applications.Items[0].Payload != nil || forms.Drafts.Items[0].Payload != nil {
		t.Fatalf("list items must not carry payload: %+v %+v", forms.Applications.Items[0], forms.Drafts.Items[0])
	}

	var raw struct {
		Data struct {
			Applications struct {
				Items []map[string]any `json:"items"`
			} `json:"applications"`
			Drafts struct {
				Items []map[string]any `json:"items"`
			} `json:"drafts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, item := range append(raw.Data.Applications.Items, raw.Data.Drafts.Items...) {
		if _, ok := item["payload"]; ok {
			t.Fatalf("payload key must be absent from list items, got %+v", item)
		}
	}
}

// TestPlatformFormsRejectsInvalidPagination 驗證 forms 分頁參數驗證與既有分頁慣例一致。
func TestPlatformFormsRejectsInvalidPagination(t *testing.T) {
	handler := newTestAPI(true)

	badPageReq := httptest.NewRequest(http.MethodGet, "/v1/platform/forms?page=abc", nil)
	badPageRec := httptest.NewRecorder()
	handler.ServeHTTP(badPageRec, badPageReq)
	if badPageRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid page, got %d: %s", badPageRec.Code, badPageRec.Body.String())
	}
	if got := decodeError(t, badPageRec.Body.Bytes()).Code; got != domain.ErrorCodeInvalidQueryInteger {
		t.Fatalf("expected invalid query integer code, got %+v", got)
	}

	badSizeReq := httptest.NewRequest(http.MethodGet, "/v1/platform/forms?page_size=101", nil)
	badSizeRec := httptest.NewRecorder()
	handler.ServeHTTP(badSizeRec, badSizeReq)
	if badSizeRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized page_size, got %d: %s", badSizeRec.Code, badSizeRec.Body.String())
	}
	if got := decodeError(t, badSizeRec.Body.Bytes()).Code; got != domain.ErrorCodeQueryAboveMaximum {
		t.Fatalf("expected query above maximum code, got %+v", got)
	}
}

// TestPlatformFormsAppliesQueryFilters 驗證 status/template/search 查詢參數作用於 forms 列表。
func TestPlatformFormsAppliesQueryFilters(t *testing.T) {
	now := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	handler := newTestAPIForAccountNow("acct-admin", now, func(store *memory.Store) {
		seedPlatformFormsFixture(store, now)
	})

	statusReq := httptest.NewRequest(http.MethodGet, "/v1/platform/forms?status=approved", nil)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for status filter, got %d: %s", statusRec.Code, statusRec.Body.String())
	}
	if got := decodeData[domain.PlatformFormsResponse](t, statusRec.Body.Bytes()); got.Applications.Total != 0 || got.Drafts.Total != 1 {
		t.Fatalf("expected status filter to empty applications only, got %+v", got)
	}

	templateReq := httptest.NewRequest(http.MethodGet, "/v1/platform/forms?template=expense-claim", nil)
	templateRec := httptest.NewRecorder()
	handler.ServeHTTP(templateRec, templateReq)
	if templateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for template filter, got %d: %s", templateRec.Code, templateRec.Body.String())
	}
	if got := decodeData[domain.PlatformFormsResponse](t, templateRec.Body.Bytes()); got.Applications.Total != 0 || got.Drafts.Total != 0 {
		t.Fatalf("expected template filter to exclude leave instances, got %+v", got)
	}

	searchReq := httptest.NewRequest(http.MethodGet, "/v1/platform/forms?search=%E8%AB%8B%E5%81%87", nil)
	searchRec := httptest.NewRecorder()
	handler.ServeHTTP(searchRec, searchReq)
	if searchRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for search filter, got %d: %s", searchRec.Code, searchRec.Body.String())
	}
	if got := decodeData[domain.PlatformFormsResponse](t, searchRec.Body.Bytes()); got.Applications.Total != 1 || got.Drafts.Total != 1 {
		t.Fatalf("expected title search to match template name, got %+v", got)
	}

	missReq := httptest.NewRequest(http.MethodGet, "/v1/platform/forms?search=nomatchkeyword", nil)
	missRec := httptest.NewRecorder()
	handler.ServeHTTP(missRec, missReq)
	if missRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for search miss, got %d: %s", missRec.Code, missRec.Body.String())
	}
	if got := decodeData[domain.PlatformFormsResponse](t, missRec.Body.Bytes()); got.Applications.Total != 0 || got.Drafts.Total != 0 {
		t.Fatalf("expected search miss to return empty envelopes, got %+v", got)
	}
}
