package service

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
)

var assumedRoleBearerPattern = regexp.MustCompile(`(?i)(^|[^[:alnum:]])sess_[a-z0-9_-]{16,}`)

// sanitizeAuditLog redacts credentials at the read boundary so historical
// records written before credential-safe auditing cannot leak to API clients.
func sanitizeAuditLog(log AuditLog) AuditLog {
	log.Target = sanitizeAuditText(log.Target)
	log.Result = sanitizeAuditText(log.Result)
	log.TraceID = sanitizeAuditText(log.TraceID)
	log.Details = sanitizeAuditDetails(log.Details)
	return log
}

func sanitizeAuditLogs(logs []AuditLog) []AuditLog {
	if len(logs) == 0 {
		return logs
	}
	out := make([]AuditLog, len(logs))
	for i, log := range logs {
		out[i] = sanitizeAuditLog(log)
	}
	return out
}

func sanitizeAuditDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return nil
	}
	out := make(map[string]any, len(details))
	for key, value := range details {
		if sensitiveAuditDetailKey(key) {
			continue
		}
		if sanitized, ok := sanitizeAuditValue(value); ok {
			out[key] = sanitized
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sanitizeAuditValue(value any) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, true
	case string:
		return sanitizeAuditText(typed), true
	case bool, float32, float64, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, json.Number:
		return typed, true
	case map[string]any:
		return sanitizeAuditDetails(typed), true
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			if sanitized, ok := sanitizeAuditValue(item); ok {
				out = append(out, sanitized)
			}
		}
		return out, true
	default:
		// Normalize typed maps, slices and structs to their actual JSON surface,
		// then recurse. If a value cannot be inspected, omit it fail-closed.
		raw, err := json.Marshal(typed)
		if err != nil {
			return nil, false
		}
		var generic any
		if err := json.Unmarshal(raw, &generic); err != nil {
			return nil, false
		}
		return sanitizeAuditValue(generic)
	}
}

func sensitiveAuditDetailKey(key string) bool {
	compact := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, strings.TrimSpace(key))
	if strings.Contains(compact, "assumedrolesession") {
		return true
	}
	switch compact {
	case "sessionid", "sessiontoken", "authorization", "proxyauthorization",
		"token", "bearer", "bearertoken", "accesstoken", "refreshtoken",
		"idtoken", "apikey", "authsecret", "clientsecret", "password",
		"cookie", "setcookie":
		return true
	}
	return strings.HasSuffix(compact, "accesstoken") ||
		strings.HasSuffix(compact, "refreshtoken") ||
		strings.HasSuffix(compact, "bearertoken") ||
		strings.HasSuffix(compact, "authsecret") ||
		strings.HasSuffix(compact, "clientsecret")
}

func sanitizeAuditText(value string) string {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "bearer ") || assumedRoleBearerPattern.MatchString(value) {
		return "[REDACTED]"
	}
	return value
}
