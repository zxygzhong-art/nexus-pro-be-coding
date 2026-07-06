package startup

import (
	"fmt"
	"io"
	"net/url"
	"strings"
)

// Report 定義 report 的資料結構。
type Report struct {
	Name         string
	Env          string
	HTTPAddr     string
	Repository   string
	Dependencies []Dependency
}

// Dependency 定義依賴的資料結構。
type Dependency struct {
	Name   string
	Status string
	Target string
	Detail string
}

// Print 處理 print。
func Print(w io.Writer, report Report) error {
	_, err := io.WriteString(w, Render(report))
	return err
}

// Render 處理 render。
func Render(report Report) string {
	name := clean(report.Name)
	if name == "" {
		name = "nexus-pro-be"
	}
	env := clean(report.Env)
	if env == "" {
		env = "unknown"
	}
	addr := clean(report.HTTPAddr)
	if addr == "" {
		addr = ":8080"
	}
	repository := clean(report.Repository)
	if repository == "" {
		repository = "unknown"
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  +------------------------------------------------------------+\n")
	fmt.Fprintf(&b, "  | %-58s |\n", name)
	b.WriteString("  | Runtime bootstrap completed.                               |\n")
	b.WriteString("  +------------------------------------------------------------+\n")
	fmt.Fprintf(&b, "    env=%s  addr=%s  repository=%s\n\n", env, addr, repository)
	b.WriteString("    Dependencies\n")
	b.WriteString("    SERVICE           STATUS       TARGET                          DETAIL\n")
	b.WriteString("    ---------------   ----------   -----------------------------   ----------------\n")
	if len(report.Dependencies) == 0 {
		b.WriteString("    application    ready        local                           no external checks\n")
	} else {
		for _, dep := range report.Dependencies {
			fmt.Fprintf(
				&b,
				"    %-15s   %-10s   %-29s   %s\n",
				clean(dep.Name),
				clean(dep.Status),
				clean(dep.Target),
				clean(dep.Detail),
			)
		}
	}
	b.WriteString("\n")
	return b.String()
}

// SafeURL 處理 safe URL。
func SafeURL(raw string) string {
	value := clean(raw)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" {
		return redactInlineSecret(value)
	}
	if parsed.User != nil {
		username := parsed.User.Username()
		if username != "" {
			parsed.User = url.User(username)
		} else {
			parsed.User = nil
		}
	}
	query := parsed.Query()
	for key := range query {
		if isSensitiveKey(key) {
			query.Set(key, "redacted")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// Missing 處理 missing。
func Missing(keys ...string) string {
	missing := make([]string, 0, len(keys))
	for _, key := range keys {
		if clean(key) != "" {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return "missing " + strings.Join(missing, ", ")
}

// clean 處理 clean。
func clean(value string) string {
	return strings.TrimSpace(value)
}

// isSensitiveKey 判斷是否為sensitive key。
func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "password") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "key")
}

// redactInlineSecret 處理 redact inline secret。
func redactInlineSecret(value string) string {
	if at := strings.LastIndex(value, "@"); at >= 0 {
		before := value[:at]
		if colon := strings.LastIndex(before, ":"); colon >= 0 {
			return before[:colon] + ":redacted" + value[at:]
		}
	}
	return value
}
