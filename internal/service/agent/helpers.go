package agent

import (
	"context"
	"strings"
)

// helpers.go — 自 service 包複製的微型輔助（拆包過渡；語意與原版一致）。

// uniqueStrings 去重去空（與 service 同名函式同語意）。
func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// goContext 取出 RequestContext 內的 go context，未附帶時回 Background。
func goContext(ctx RequestContext) context.Context {
	if ctx.Context != nil {
		return ctx.Context
	}
	return context.Background()
}

// stringFromAny 與 service 同名函式同語意。
func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case Action:
		return string(v)
	case ApplicationCode:
		return string(v)
	case ResourceType:
		return string(v)
	default:
		return ""
	}
}

// stringSet 與 service 同名函式同語意。
func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

// stringSliceFromAny 與 service 同名函式同語意。
func stringSliceFromAny(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	default:
		return nil
	}
}
