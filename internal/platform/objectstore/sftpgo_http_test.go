package objectstore

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestNormalizeSFTPGoHTTPBaseURLAcceptsHTTP verifies HTTP endpoints are normalized.
func TestNormalizeSFTPGoHTTPBaseURLAcceptsHTTP(t *testing.T) {
	got, err := normalizeSFTPGoHTTPBaseURL("http://sftpgo:8080/")
	if err != nil {
		t.Fatalf("normalizeSFTPGoHTTPBaseURL() error = %v", err)
	}
	if got != "http://sftpgo:8080" {
		t.Fatalf("normalizeSFTPGoHTTPBaseURL() = %q, want http://sftpgo:8080", got)
	}
}

// TestSFTPGoHTTPPathForKeyScopesKeysUnderRoot verifies object keys stay under the configured root.
func TestSFTPGoHTTPPathForKeyScopesKeysUnderRoot(t *testing.T) {
	store := &SFTPGoHTTP{root: "/nexus-bucket"}
	got, err := store.pathForKey("/imports/session/raw.csv")
	if err != nil {
		t.Fatalf("pathForKey() error = %v", err)
	}
	if got != "/nexus-bucket/imports/session/raw.csv" {
		t.Fatalf("pathForKey() = %q, want /nexus-bucket/imports/session/raw.csv", got)
	}
}

// TestNewSFTPGoStoreSelectsHTTP verifies http(s) endpoints use the REST adapter.
func TestNewSFTPGoStoreSelectsHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/user/token":
			user, pass, ok := r.BasicAuth()
			if !ok || user != "nexus-service" || pass != "nexus-service" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"token-1","expires_at":"` + time.Now().Add(time.Hour).Format(time.RFC3339) + `"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/user/dirs":
			if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.URL.Query().Get("path") != "/nexus-bucket" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/user/files/upload":
			if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.URL.Query().Get("path") != "/nexus-bucket/imports/a.txt" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			body, _ := io.ReadAll(r.Body)
			if !bytes.Equal(body, []byte("hello")) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	store, err := NewSFTPGoStore(context.Background(), SFTPGoOptions{
		Endpoint:   server.URL,
		Root:       "nexus-bucket",
		Username:   "nexus-service",
		Password:   "nexus-service",
		CreateRoot: true,
	})
	if err != nil {
		t.Fatalf("NewSFTPGoStore() error = %v", err)
	}
	httpStore, ok := store.(*SFTPGoHTTP)
	if !ok {
		t.Fatalf("NewSFTPGoStore() type = %T, want *SFTPGoHTTP", store)
	}
	if err := httpStore.PutObject(context.Background(), "imports/a.txt", "text/plain", []byte("hello")); err != nil {
		t.Fatalf("PutObject() error = %v", err)
	}
}

// TestNewSFTPGoStoreRejectsUnsupportedScheme verifies unsupported schemes fail fast.
func TestNewSFTPGoStoreRejectsUnsupportedScheme(t *testing.T) {
	_, err := NewSFTPGoStore(context.Background(), SFTPGoOptions{
		Endpoint: "ftp://sftpgo:21",
		Root:     "nexus-bucket",
		Username: "nexus-service",
		Password: "nexus-service",
	})
	if err == nil || !strings.Contains(err.Error(), "http, https, or sftp") {
		t.Fatalf("NewSFTPGoStore() error = %v, want unsupported scheme", err)
	}
}
