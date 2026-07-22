package service

import (
	"fmt"
	"strings"

	"nexus-pro-api/internal/utils"
)

const (
	employeeSyncModeCreate = "create"
	employeeSyncModeUpdate = "update"
	employeeSyncModeUpsert = "upsert"
)

func normalizeEmployeeDate(value string) string {
	value = strings.TrimSpace(value)
	if strings.Count(value, "/") == 2 {
		parts := strings.Split(value, "/")
		if len(parts[1]) == 1 {
			parts[1] = "0" + parts[1]
		}
		if len(parts[2]) == 1 {
			parts[2] = "0" + parts[2]
		}
		return strings.Join(parts, "-")
	}
	return value
}

func mergeEmployeeMaps(existing map[string]any, updates map[string]any) map[string]any {
	out := utils.CopyStringMap(existing)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range updates {
		if strings.TrimSpace(fmt.Sprint(value)) == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func employeeRowErrorsFromError(row int, err error) ([]RowError, bool) {
	if err == nil {
		return nil, true
	}
	appErr, ok := AsAppError(err)
	if !ok || appErr.Status >= 500 {
		return nil, false
	}
	if len(appErr.RowErrors) > 0 {
		return appErr.RowErrors, true
	}
	if len(appErr.FieldErrors) > 0 {
		out := make([]RowError, 0, len(appErr.FieldErrors))
		for _, field := range appErr.FieldErrors {
			out = append(out, RowError{Row: row, Field: field.Field, Code: field.Code, Message: field.Message})
		}
		return out, true
	}
	return []RowError{{Row: row, Code: appErr.Code, Message: appErr.Message}}, true
}

func firstEmployeeRowErrorMessage(errors []RowError) string {
	if len(errors) == 0 {
		return "employee sync row failed"
	}
	return errors[0].Message
}

func (c HRService) employeeSyncScopeErrors(ctx RequestContext, account Account, rowNumber int, employee Employee, previous Employee, update bool, decision CheckResult) ([]RowError, error) {
	targets := []Employee{employee}
	if update {
		targets = append(targets, previous)
	}
	visible, err := c.filterEmployeesByDecision(ctx, account, targets, decision)
	if err != nil {
		return nil, err
	}
	if len(visible) == len(targets) {
		return nil, nil
	}
	return []RowError{{
		Row:     rowNumber,
		Field:   "authz_scope",
		Code:    "out_of_scope",
		Message: "employee sync row is outside authorized scope",
	}}, nil
}
