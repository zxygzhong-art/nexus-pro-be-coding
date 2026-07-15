package v1_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

// TestPatchMePreferencesPersistsPreferredLocale verifies the account preference is returned immediately and by subsequent /me reads.
func TestPatchMePreferencesPersistsPreferredLocale(t *testing.T) {
	var storeRef *memory.Store
	handler := newTestAPIForAccountNow("acct-employee", time.Now(), func(store *memory.Store) {
		storeRef = store
	})
	initialReq := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	initialRec := httptest.NewRecorder()
	handler.ServeHTTP(initialRec, initialReq)
	if initialRec.Code != http.StatusOK {
		t.Fatalf("expected initial GET /me 200, got %d: %s", initialRec.Code, initialRec.Body.String())
	}
	initial := decodeData[service.MeResponse](t, initialRec.Body.Bytes())
	if initial.Account.PreferredLocale != domain.DefaultPreferredLocale {
		t.Fatalf("expected default locale %q, got %q", domain.DefaultPreferredLocale, initial.Account.PreferredLocale)
	}

	req := httptest.NewRequest(http.MethodPatch, "/v1/me/preferences", strings.NewReader(`{"preferred_locale":"en-US"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	updated := decodeData[service.MeResponse](t, rec.Body.Bytes())
	if updated.Account.PreferredLocale != domain.PreferredLocaleENUS {
		t.Fatalf("expected en-US preference, got %q", updated.Account.PreferredLocale)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected GET /me 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
	refreshed := decodeData[service.MeResponse](t, getRec.Body.Bytes())
	if refreshed.Account.PreferredLocale != domain.PreferredLocaleENUS {
		t.Fatalf("GET /me did not expose persisted locale: %+v", refreshed.Account)
	}

	audits, err := storeRef.ListAuditLogs(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, audit := range audits {
		if audit.Action == "platform.me.preferences.update" && audit.Target == "acct-employee" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected account preference update audit")
	}
}

// TestPatchMePreferencesRejectsUnsupportedLocales verifies the public contract accepts only shipped locales.
func TestPatchMePreferencesRejectsUnsupportedLocales(t *testing.T) {
	handler := newTestAPIForAccountNow("acct-employee", time.Now(), nil)
	for name, body := range map[string]string{
		"empty":       `{"preferred_locale":""}`,
		"unsupported": `{"preferred_locale":"zh-CN"}`,
		"unknown":     `{"locale":"en-US"}`,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, "/v1/me/preferences", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestPatchMePreferencesRequiresUpdatePermission verifies me.read does not grant preference writes.
func TestPatchMePreferencesRequiresUpdatePermission(t *testing.T) {
	handler := newTestAPIForAccountNow("acct-audit", time.Now(), nil)
	req := httptest.NewRequest(http.MethodPatch, "/v1/me/preferences", strings.NewReader(`{"preferred_locale":"en-US"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
