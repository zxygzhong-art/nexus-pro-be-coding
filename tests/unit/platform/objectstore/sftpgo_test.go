package objectstore_test

import (
	"context"
	"testing"

	"nexus-pro-api/internal/platform/objectstore"
)

// TestSFTPGoPathForKeyRejectsEscapingKeys protects the remote root boundary.
func TestSFTPGoPathForKeyRejectsEscapingKeys(t *testing.T) {
	store, err := objectstore.NewSFTPGo(context.Background(), objectstore.SFTPGoOptions{
		Endpoint: "sftpgo", Root: "nexus-hr-imports", Username: "nexus-service", Password: "secret", InsecureSkipHostKey: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"", "../escape", "imports/../../escape"} {
		if _, err := store.PathForKey(key); err == nil {
			t.Fatalf("pathForKey(%q) error = nil, want error", key)
		}
	}
}

// TestSFTPGoPathForKeyScopesKeysUnderRoot verifies object keys stay under the configured root.
func TestSFTPGoPathForKeyScopesKeysUnderRoot(t *testing.T) {
	store, err := objectstore.NewSFTPGo(context.Background(), objectstore.SFTPGoOptions{
		Endpoint: "sftpgo", Root: "nexus-hr-imports", Username: "nexus-service", Password: "secret", InsecureSkipHostKey: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := store.PathForKey("/imports/session/raw.csv")
	if err != nil {
		t.Fatalf("pathForKey() error = %v", err)
	}
	if got != "/nexus-hr-imports/imports/session/raw.csv" {
		t.Fatalf("pathForKey() = %q, want /nexus-hr-imports/imports/session/raw.csv", got)
	}
}

// TestNormalizeSFTPGoEndpointDefaultsPort verifies SFTPGo endpoints can omit the default SFTP port.
func TestNormalizeSFTPGoEndpointDefaultsPort(t *testing.T) {
	got, err := objectstore.NormalizeSFTPGoEndpoint("sftpgo")
	if err != nil {
		t.Fatalf("normalizeSFTPGoEndpoint() error = %v", err)
	}
	if got != "sftpgo:2022" {
		t.Fatalf("normalizeSFTPGoEndpoint() = %q, want sftpgo:2022", got)
	}
}
