package service

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
)

func parseEmployeeImport(filename string, raw []byte) ([]EmployeeImportRow, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".xlsx" {
		return parseEmployeeXLSX(raw)
	}
	return parseEmployeeCSV(raw)
}

func importContentType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	default:
		return "text/csv; charset=utf-8"
	}
}

func parseEmployeeCSV(raw []byte) ([]EmployeeImportRow, error) {
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})))
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}
	return employeeRowsFromRecords(records)
}

func parseEmployeeXLSX(raw []byte) ([]EmployeeImportRow, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("parse xlsx: %w", err)
	}
	files := map[string]*zip.File{}
	for _, file := range zr.File {
		files[file.Name] = file
	}
	shared, err := readXLSXSharedStrings(files["xl/sharedStrings.xml"])
	if err != nil {
		return nil, err
	}
	sheet := files["xl/worksheets/sheet1.xml"]
	if sheet == nil {
		return nil, fmt.Errorf("xlsx sheet1.xml not found")
	}
	records, err := readXLSXSheet(sheet, shared)
	if err != nil {
		return nil, err
	}
	return employeeRowsFromRecords(records)
}

func employeeRowsFromRecords(records [][]string) ([]EmployeeImportRow, error) {
	if len(records) < 2 {
		return nil, fmt.Errorf("import file must include header and at least one data row")
	}
	rows := make([]EmployeeImportRow, 0, len(records)-1)
	for i, record := range records[1:] {
		record = padRecord(record, employeeImportColumnCount())
		rows = append(rows, EmployeeImportRow{
			RowNumber: i + 2,
			Input:     employeeImportInputFromRecord(record),
			Employee:  employeeCreateInputFromImportRecord(record),
		})
	}
	return rows, nil
}

type xlsxSST struct {
	Items []struct {
		Text string `xml:"t"`
	} `xml:"si"`
}

func readXLSXSharedStrings(file *zip.File) ([]string, error) {
	if file == nil {
		return nil, nil
	}
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	var sst xlsxSST
	if err := xml.NewDecoder(rc).Decode(&sst); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(sst.Items))
	for _, item := range sst.Items {
		out = append(out, item.Text)
	}
	return out, nil
}

type xlsxWorksheet struct {
	Rows []struct {
		Cells []struct {
			Ref   string `xml:"r,attr"`
			Type  string `xml:"t,attr"`
			Value string `xml:"v"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

func readXLSXSheet(file *zip.File, shared []string) ([][]string, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	var sheet xlsxWorksheet
	if err := xml.Unmarshal(raw, &sheet); err != nil {
		return nil, err
	}
	records := make([][]string, 0, len(sheet.Rows))
	for _, row := range sheet.Rows {
		record := make([]string, employeeImportColumnCount())
		for idx, cell := range row.Cells {
			col := idx
			if cell.Ref != "" {
				col = xlsxColumnIndex(cell.Ref)
			}
			if col < 0 || col >= len(record) {
				continue
			}
			value := cell.Value
			if cell.Type == "s" {
				i, _ := strconv.Atoi(value)
				if i >= 0 && i < len(shared) {
					value = shared[i]
				}
			}
			record[col] = value
		}
		records = append(records, record)
	}
	return records, nil
}

func xlsxColumnIndex(ref string) int {
	col := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		col = col*26 + int(r-'A'+1)
	}
	return col - 1
}

func normalizeImportDate(value string) string {
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

func padRecord(record []string, size int) []string {
	if len(record) >= size {
		return record
	}
	out := make([]string, size)
	copy(out, record)
	return out
}
