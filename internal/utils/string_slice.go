package utils

import "strings"

// FirstNonEmpty 取得第一個non 空值。
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// ContainsString 檢查是否包含字串。
func ContainsString(src []string, target string) bool {
	for _, v := range src {
		if v == target {
			return true
		}
	}
	return false
}
