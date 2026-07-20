package jsoncodec

import (
	"encoding/json"
	"reflect"

	"nexus-pro-api/internal/domain"
)

// Must 處理 must。
func Must(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return zero(v)
	}
	if string(b) == "null" {
		return zero(v)
	}
	return b
}

// Map 映射目前流程。解碼失敗時 fail-closed 回傳 nil;需要錯誤時改用 MapE。
func Map(b []byte) map[string]any {
	out, _ := MapE(b)
	return out
}

// MapE 解碼 JSON object,失敗時回傳明確錯誤而不是靜默吞掉。
func MapE(b []byte) (map[string]any, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Permissions 處理權限。解碼失敗時 fail-closed 回傳 nil;需要錯誤時改用 PermissionsE。
func Permissions(b []byte) []domain.Permission {
	out, _ := PermissionsE(b)
	return out
}

// PermissionsE 解碼權限陣列,失敗時回傳明確錯誤而不是靜默吞掉。
func PermissionsE(b []byte) ([]domain.Permission, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var out []domain.Permission
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// zero 處理 zero。
func zero(v any) []byte {
	if v == nil {
		return []byte("{}")
	}
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		return []byte("[]")
	default:
		return []byte("{}")
	}
}
