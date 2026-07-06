package objectstore_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"nexus-pro-be/internal/platform/objectstore"
)

// TestLocalPutObjectWritesInsideRoot 驗證本機 put 物件 writes inside root。
func TestLocalPutObjectWritesInsideRoot(t *testing.T) {
	root := t.TempDir()
	store, err := objectstore.NewLocal(root)
	if err != nil {
		t.Fatalf("NewLocal() error = %v", err)
	}

	if err := store.PutObject(context.Background(), "imports/session/raw.csv", "text/csv", []byte("hello")); err != nil {
		t.Fatalf("PutObject() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "imports", "session", "raw.csv"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(raw) != "hello" {
		t.Fatalf("stored content = %q, want hello", string(raw))
	}
}

// TestLocalPutObjectRejectsEscapingKeys 驗證本機 put 物件 rejects escaping keys。
func TestLocalPutObjectRejectsEscapingKeys(t *testing.T) {
	store, err := objectstore.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocal() error = %v", err)
	}

	for _, key := range []string{"", "../escape", "imports/../../escape"} {
		if err := store.PutObject(context.Background(), key, "text/plain", []byte("x")); err == nil {
			t.Fatalf("PutObject(%q) error = nil, want error", key)
		}
	}
}
