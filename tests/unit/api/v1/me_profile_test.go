package v1_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// TestPatchMeProfileUpdatesOnlySelfServiceFields verifies the authenticated employee can update the five allowlisted fields.
func TestPatchMeProfileUpdatesOnlySelfServiceFields(t *testing.T) {
	var storeRef *memory.Store
	handler := newTestAPIForAccountNow("acct-employee", time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC), func(store *memory.Store) {
		storeRef = store
		set, ok, err := store.GetPermissionSet(context.Background(), "demo", "ps-employee")
		if err != nil || !ok {
			t.Fatalf("load employee permission set: ok=%v err=%v", ok, err)
		}
		for index := range set.Permissions {
			if set.Permissions[index].Resource == "me" && set.Permissions[index].Action == "update" {
				set.Permissions[index].Scope = "self"
			}
		}
		if err := store.UpsertPermissionSet(context.Background(), set); err != nil {
			t.Fatalf("update employee permission set: %v", err)
		}
	})
	body := `{"english_name":" Alice Chen ","mobile_phone":" 0912-345-678 ","extension":" 812 ","slack":" @alice ","emergency_contact_name":" Lin Chen "}`
	req := httptest.NewRequest(http.MethodPatch, "/v1/me/profile", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	me := decodeData[service.MeResponse](t, rec.Body.Bytes())
	if me.Employee == nil {
		t.Fatal("expected linked employee in response")
	}
	if me.Employee.Name != "Demo Employee" || me.Employee.EmployeeNo != "E0002" || me.Employee.CompanyEmail != "employee@demo.local" {
		t.Fatalf("immutable employee fields changed: %+v", *me.Employee)
	}
	if me.Employee.Phone != "0912-345-678" || me.Employee.BasicInfo["name_en"] != "Alice Chen" || me.Employee.BasicInfo["name_en_source"] != "self" {
		t.Fatalf("profile basic fields not updated: %+v", *me.Employee)
	}
	if me.Employee.ContactInfo["mobile_phone"] != "0912-345-678" || me.Employee.ContactInfo["extension"] != "812" || me.Employee.ContactInfo["slack"] != "@alice" || me.Employee.ContactInfo["emergency_contact_name"] != "Lin Chen" {
		t.Fatalf("profile contact fields not updated: %+v", me.Employee.ContactInfo)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected GET /me 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
	refreshed := decodeData[service.MeResponse](t, getRec.Body.Bytes())
	if refreshed.Employee == nil || refreshed.Employee.ContactInfo["extension"] != "812" {
		t.Fatalf("GET /me did not expose updated profile: %+v", refreshed.Employee)
	}

	events, err := storeRef.ListOutboxEvents(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	foundEmployeeEvent := false
	for _, event := range events {
		if event.EventType == "employee.updated" && event.AggregateID == "emp-employee" {
			foundEmployeeEvent = true
		}
	}
	if !foundEmployeeEvent {
		t.Fatal("expected employee.updated outbox event")
	}
	audits, err := storeRef.ListAuditLogs(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	foundProfileAudit := false
	for _, audit := range audits {
		if audit.Action == "platform.me.profile.update" && audit.Target == "emp-employee" {
			foundProfileAudit = true
		}
	}
	if !foundProfileAudit {
		t.Fatal("expected profile update audit")
	}
	activityJSON, err := json.Marshal(struct {
		Events any `json:"events"`
		Audits any `json:"audits"`
	}{Events: events, Audits: audits})
	if err != nil {
		t.Fatal(err)
	}
	for _, privateValue := range []string{"Alice Chen", "0912-345-678", "Lin Chen"} {
		if strings.Contains(string(activityJSON), privateValue) {
			t.Fatalf("audit or outbox payload leaked profile value %q", privateValue)
		}
	}
}

// TestPatchMeProfileInitializesMissingProfileMaps covers employees provisioned without optional JSON profile sections.
func TestPatchMeProfileInitializesMissingProfileMaps(t *testing.T) {
	handler := newTestAPIForAccountNow("acct-employee", time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), func(store *memory.Store) {
		set, ok, err := store.GetPermissionSet(context.Background(), "demo", "ps-employee")
		if err != nil || !ok {
			t.Fatalf("load employee permission set: ok=%v err=%v", ok, err)
		}
		for index := range set.Permissions {
			if set.Permissions[index].Resource == "me" && set.Permissions[index].Action == "update" {
				set.Permissions[index].Scope = "self"
			}
		}
		if err := store.UpsertPermissionSet(context.Background(), set); err != nil {
			t.Fatalf("update employee permission set: %v", err)
		}

		employee, ok, err := store.GetEmployee(context.Background(), "demo", "emp-employee")
		if err != nil || !ok {
			t.Fatalf("load employee: ok=%v err=%v", ok, err)
		}
		employee.BasicInfo = nil
		employee.ContactInfo = nil
		if err := store.UpsertEmployee(context.Background(), employee); err != nil {
			t.Fatalf("clear optional profile maps: %v", err)
		}
	})

	req := httptest.NewRequest(http.MethodPatch, "/v1/me/profile", strings.NewReader(`{"english_name":"QA Empty Map","slack":"qa-empty-map"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	me := decodeData[service.MeResponse](t, rec.Body.Bytes())
	if me.Employee == nil || me.Employee.BasicInfo["name_en"] != "QA Empty Map" || me.Employee.ContactInfo["slack"] != "qa-empty-map" {
		t.Fatalf("missing profile maps were not initialized: %+v", me.Employee)
	}
}

// TestPatchMeProfileRejectsUnknownOrEmptyPayloads verifies immutable fields cannot be smuggled into the self-service contract.
func TestPatchMeProfileRejectsUnknownOrEmptyPayloads(t *testing.T) {
	handler := newTestAPIForAccountNow("acct-employee", time.Now(), nil)
	for name, body := range map[string]string{
		"unknown immutable field": `{"name":"Hacked"}`,
		"empty payload":           `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, "/v1/me/profile", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestPatchMeProfileRequiresUpdatePermission verifies me.read alone does not grant profile writes.
func TestPatchMeProfileRequiresUpdatePermission(t *testing.T) {
	handler := newTestAPIForAccountNow("acct-audit", time.Now(), nil)
	req := httptest.NewRequest(http.MethodPatch, "/v1/me/profile", strings.NewReader(`{"slack":"@audit"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
