package utils_test

import (
	"regexp"
	"strings"
	"testing"

	"nexus-pro-be/internal/utils"
)

func TestStringHelpers(t *testing.T) {
	src := []string{"a", "b", "a"}

	copied := utils.CopyStrings(src)
	copied[0] = "changed"
	if src[0] != "a" {
		t.Fatalf("CopyStrings should not alias source")
	}
	if !utils.ContainsString(src, "b") {
		t.Fatalf("ContainsString should find existing value")
	}
	if utils.ContainsString(src, "missing") {
		t.Fatalf("ContainsString should not find missing value")
	}

	removed := utils.RemoveString(src, "a")
	if len(removed) != 1 || removed[0] != "b" {
		t.Fatalf("RemoveString() = %#v, want only b", removed)
	}
	if got := utils.RemoveString([]string{"a"}, "a"); got != nil {
		t.Fatalf("RemoveString should return nil when all values are removed, got %#v", got)
	}
}

func TestNewSecretIDIsHighEntropyAndOpaque(t *testing.T) {
	first, err := utils.NewSecretID("sess")
	if err != nil {
		t.Fatal(err)
	}
	second, err := utils.NewSecretID("sess")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first, "sess_") || !strings.HasPrefix(second, "sess_") {
		t.Fatalf("expected prefixed secret IDs, got %q and %q", first, second)
	}
	if first == second {
		t.Fatalf("secret IDs should not repeat: %q", first)
	}
	if len(first) < len("sess_")+40 {
		t.Fatalf("secret ID should carry at least 256 bits of encoded entropy, got %q", first)
	}
	if regexp.MustCompile(`^sess-\d+-\d{6}$`).MatchString(first) {
		t.Fatalf("secret ID should not use timestamp-counter format: %q", first)
	}
	if strings.ContainsAny(first, "/+=") {
		t.Fatalf("secret ID should be raw URL-safe base64, got %q", first)
	}
}
