package sliceutil

import "testing"

func TestStringHelpers(t *testing.T) {
	src := []string{"a", "b", "a"}

	copied := CopyStrings(src)
	copied[0] = "changed"
	if src[0] != "a" {
		t.Fatalf("CopyStrings should not alias source")
	}
	if !ContainsString(src, "b") {
		t.Fatalf("ContainsString should find existing value")
	}
	if ContainsString(src, "missing") {
		t.Fatalf("ContainsString should not find missing value")
	}

	removed := RemoveString(src, "a")
	if len(removed) != 1 || removed[0] != "b" {
		t.Fatalf("RemoveString() = %#v, want only b", removed)
	}
	if got := RemoveString([]string{"a"}, "a"); got != nil {
		t.Fatalf("RemoveString should return nil when all values are removed, got %#v", got)
	}
}
