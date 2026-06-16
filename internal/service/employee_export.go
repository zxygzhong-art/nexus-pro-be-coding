package service

import (
	"bytes"
	"encoding/csv"
	"fmt"
)

func (c HRService) ExportEmployeesCSV(ctx RequestContext, query EmployeeQuery) ([]byte, string, error) {
	items, err := c.ExportEmployees(ctx, query)
	if err != nil {
		return nil, "", err
	}
	if len(items) > maxEmployeeExportRows {
		return nil, "", employeeExportLimitError()
	}
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)
	headers := make([]string, 0, len(employeeExportColumns))
	for _, column := range employeeExportColumns {
		headers = append(headers, column.header)
	}
	_ = w.Write(headers)
	for _, item := range items {
		record := make([]string, 0, len(employeeExportColumns))
		for _, column := range employeeExportColumns {
			record = append(record, column.value(item))
		}
		_ = w.Write(record)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "employees.csv", nil
}

func (c HRService) rejectOversizedEmployeeExport(ctx RequestContext, query EmployeeQuery, decision CheckResult) error {
	if employeeDecisionCanUseStorePage(decision) {
		total, err := c.store.CountEmployeesByQuery(goContext(ctx), ctx.TenantID, query)
		if err != nil {
			return err
		}
		if total > maxEmployeeExportRows {
			return employeeExportLimitError()
		}
	}
	return nil
}

func employeeExportLimitError() error {
	return Conflict(fmt.Sprintf("employee export exceeds synchronous limit of %d rows; use async export job", maxEmployeeExportRows))
}
