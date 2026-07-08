package v1_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nexus-pro-be/internal/domain"
)

func TestUserGroupMemberRoutes(t *testing.T) {
	handler := newTestAPI(true)

	patchReq := httptest.NewRequest(http.MethodPatch, "/v1/iam/user-groups/ug-employee", strings.NewReader(`{"description":"Updated"}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.Header.Set("X-Approval-Confirmed", "true")
	patchRec := httptest.NewRecorder()
	handler.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for user group update, got %d: %s", patchRec.Code, patchRec.Body.String())
	}
	updated := decodeData[domain.UserGroup](t, patchRec.Body.Bytes())
	if updated.ID != "ug-employee" || updated.Description != "Updated" {
		t.Fatalf("unexpected updated user group: %+v", updated)
	}

	addReq := httptest.NewRequest(http.MethodPost, "/v1/iam/user-groups/ug-employee/members", strings.NewReader(`{"account_id":"acct-admin","source":"manual"}`))
	addReq.Header.Set("Content-Type", "application/json")
	addReq.Header.Set("X-Approval-Confirmed", "true")
	addRec := httptest.NewRecorder()
	handler.ServeHTTP(addRec, addReq)
	if addRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for member add, got %d: %s", addRec.Code, addRec.Body.String())
	}
	membership := decodeData[domain.GroupMembership](t, addRec.Body.Bytes())
	if membership.UserGroupID != "ug-employee" || membership.AccountID != "acct-admin" {
		t.Fatalf("unexpected membership response: %+v", membership)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/iam/user-groups/ug-employee/members?page=1&page_size=10", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for member list, got %d: %s", listRec.Code, listRec.Body.String())
	}
	page := decodeData[domain.PageResponse[domain.GroupMembership]](t, listRec.Body.Bytes())
	if page.Total == 0 {
		t.Fatalf("expected member list to include added member, got %+v", page)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/iam/user-groups/ug-employee/members/acct-admin", nil)
	deleteReq.Header.Set("X-Approval-Confirmed", "true")
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for member remove, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
}
