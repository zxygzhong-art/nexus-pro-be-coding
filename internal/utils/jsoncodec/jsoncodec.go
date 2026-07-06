package jsoncodec

import (
	"encoding/json"
	"reflect"

	"nexus-pro-be/internal/domain"
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

// Map 映射目前流程。
func Map(b []byte) map[string]any {
	if len(b) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

// Permissions 處理權限。
func Permissions(b []byte) []domain.Permission {
	if len(b) == 0 {
		return nil
	}
	var out []domain.Permission
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
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
