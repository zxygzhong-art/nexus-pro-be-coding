package objectstore

import "testing"

// TestSFTPGoPathForKeyRejectsEscapingKeys protects the remote root boundary.
func TestSFTPGoPathForKeyRejectsEscapingKeys(t *testing.T) {
	store := &SFTPGo{root: "/nexus-hr-imports"}

	for _, key := range []string{"", "../escape", "imports/../../escape"} {
		if _, err := store.pathForKey(key); err == nil {
			t.Fatalf("pathForKey(%q) error = nil, want error", key)
		}
	}
}

// TestSFTPGoPathForKeyScopesKeysUnderRoot verifies object keys stay under the configured root.
func TestSFTPGoPathForKeyScopesKeysUnderRoot(t *testing.T) {
	store := &SFTPGo{root: "/nexus-hr-imports"}

	got, err := store.pathForKey("/imports/session/raw.csv")
	if err != nil {
		t.Fatalf("pathForKey() error = %v", err)
	}
	if got != "/nexus-hr-imports/imports/session/raw.csv" {
		t.Fatalf("pathForKey() = %q, want /nexus-hr-imports/imports/session/raw.csv", got)
	}
}

// TestNormalizeSFTPGoEndpointDefaultsPort verifies SFTPGo endpoints can omit the default SFTP port.
func TestNormalizeSFTPGoEndpointDefaultsPort(t *testing.T) {
	got, err := normalizeSFTPGoEndpoint("sftpgo")
	if err != nil {
		t.Fatalf("normalizeSFTPGoEndpoint() error = %v", err)
	}
	if got != "sftpgo:2022" {
		t.Fatalf("normalizeSFTPGoEndpoint() = %q, want sftpgo:2022", got)
	}
}
