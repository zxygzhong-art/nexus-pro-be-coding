package utils

import "strings"

// FirstNonEmpty returns the first non-empty string after trimming whitespace.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// ContainsString reports whether src contains target.
func ContainsString(src []string, target string) bool {
	for _, v := range src {
		if v == target {
			return true
		}
	}
	return false
}

// RemoveString returns a copy of src without target.
func RemoveString(src []string, target string) []string {
	if len(src) == 0 {
		return nil
	}
	out := src[:0]
	for _, v := range src {
		if v != target {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return append([]string(nil), out...)
}
