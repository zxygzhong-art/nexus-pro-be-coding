package service

import "time"

// apiTimestamp 將時間點統一投影為帶時區的 UTC RFC3339 字串。
func apiTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
