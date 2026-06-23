package utils

import (
	"fmt"
	"strings"
	"time"
)

// ParseDate parses a YYYY-MM-DD calendar date in UTC.
func ParseDate(value string) (time.Time, error) {
	layouts := []string{time.RFC3339, "2006-01-02"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date: %s", value)
}

// ParseDateTime parses an RFC3339 timestamp.
func ParseDateTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	return ParseDate(value)
}
