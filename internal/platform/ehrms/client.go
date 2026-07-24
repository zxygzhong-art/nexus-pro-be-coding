package ehrms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
)

// Client 定義 client 的資料結構。
type Client struct {
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	requestSlots chan struct{}
}

// RequestError 保留上游失敗分類，但不要求服務層暴露回應內容。
type RequestError struct {
	Operation  string
	StatusCode int
	Cause      error
}

func (e *RequestError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("ehrms %s returned %d", e.Operation, e.StatusCode)
	}
	return fmt.Sprintf("ehrms %s request failed", e.Operation)
}

func (e *RequestError) Unwrap() error { return e.Cause }

// Temporary 表示有限重試可能成功的網路或上游錯誤。
func (e *RequestError) Temporary() bool {
	return e.StatusCode == 0 || e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

const maxEmployeesResponseBytes = 10 << 20
const maxDepartmentsResponseBytes = 5 << 20
const maxPositionsResponseBytes = 5 << 20
const maxAttendanceResponseBytes = 20 << 20
const maxLeaveBalancesResponseBytes = 10 << 20
const maxLeaveDetailsResponseBytes = 10 << 20
const maxLeaveTypesResponseBytes = 5 << 20
const MaxConcurrentRequests = 10

// NewClient 建立 client。
func NewClient(baseURL string, apiKey string, httpClient *http.Client) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("EHRMS_BASE_URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("EHRMS_BASE_URL must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("EHRMS_BASE_URL must be an http or https URL")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("EHRMS_API_KEY is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		baseURL:      baseURL,
		apiKey:       apiKey,
		httpClient:   httpClient,
		requestSlots: make(chan struct{}, MaxConcurrentRequests),
	}, nil
}

// Ping 檢查外部服務連線狀態。
func (c *Client) Ping(ctx context.Context) error {
	release, err := c.acquireRequestSlot(ctx)
	if err != nil {
		return err
	}
	defer release()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ehrms health returned %d", resp.StatusCode)
	}
	return nil
}

// ListEmployees 列出員工。
func (c *Client) ListEmployees(ctx context.Context) ([]domain.EHRMSEmployeeRecord, error) {
	body, err := c.getJSON(ctx, "/employees", maxEmployeesResponseBytes, "employees")
	if err != nil {
		return nil, err
	}
	raw, err := decodeJSONObjectRows(body, "employees")
	if err != nil {
		return nil, err
	}
	rows := make([]domain.EHRMSEmployeeRecord, 0, len(raw))
	for _, row := range raw {
		rows = append(rows, domain.EHRMSEmployeeRecord(stringRecordFromJSON(row)))
	}
	return normalizeEmployeeRecords(rows), nil
}

// ListDepartments 列出部門組織樹。
func (c *Client) ListDepartments(ctx context.Context) ([]domain.EHRMSDepartmentRecord, error) {
	body, err := c.getJSON(ctx, "/departments", maxDepartmentsResponseBytes, "departments")
	if err != nil {
		return nil, err
	}
	raw, err := decodeJSONObjectRows(body, "departments")
	if err != nil {
		return nil, err
	}
	rows := make([]domain.EHRMSDepartmentRecord, 0, len(raw))
	for _, row := range raw {
		rows = append(rows, domain.EHRMSDepartmentRecord(stringRecordFromJSON(row)))
	}
	return normalizeDepartmentRecords(rows), nil
}

// ListPositions 列出崗位清單。
func (c *Client) ListPositions(ctx context.Context) ([]domain.EHRMSPositionRecord, error) {
	body, err := c.getJSON(ctx, "/positions", maxPositionsResponseBytes, "positions")
	if err != nil {
		return nil, err
	}
	raw, err := decodeJSONObjectRows(body, "positions")
	if err != nil {
		return nil, err
	}
	rows := make([]domain.EHRMSPositionRecord, 0, len(raw))
	for _, row := range raw {
		rows = append(rows, domain.EHRMSPositionRecord(stringRecordFromJSON(row)))
	}
	return normalizePositionRecords(rows), nil
}

// ListAttendance 列出考勤日彙總。
func (c *Client) ListAttendance(ctx context.Context, query domain.EHRMSAttendanceQuery) ([]domain.EHRMSAttendanceRecord, error) {
	body, err := c.getJSON(ctx, ehrmsAttendanceQueryPath(query), maxAttendanceResponseBytes, "attendance")
	if err != nil {
		return nil, err
	}
	raw, err := decodeJSONObjectRows(body, "attendance")
	if err != nil {
		return nil, err
	}
	rows := make([]domain.EHRMSAttendanceRecord, 0, len(raw))
	for _, row := range raw {
		rows = append(rows, domain.EHRMSAttendanceRecord(stringRecordFromJSON(row)))
	}
	rows = normalizeAttendanceRecords(rows)
	if err := validateEmployeeScopedRows(rows, query.EmployeeID, "attendance"); err != nil {
		return nil, err
	}
	return rows, nil
}

func ehrmsAttendanceQueryPath(query domain.EHRMSAttendanceQuery) string {
	values := url.Values{}
	if employeeID := strings.TrimSpace(query.EmployeeID); employeeID != "" {
		values.Set("emp", employeeID)
	}
	if start := strings.TrimSpace(query.Start); start != "" {
		values.Set("start", start)
	}
	if end := strings.TrimSpace(query.End); end != "" {
		values.Set("end", end)
	}
	return ehrmsQueryPath("/attendance", values)
}

func ehrmsLeaveEntitlementQueryPath(query domain.EHRMSAttendanceQuery) string {
	values := url.Values{}
	if employeeID := strings.TrimSpace(query.EmployeeID); employeeID != "" {
		values.Set("emp", employeeID)
	}
	return ehrmsQueryPath("/leave-entitlement", values)
}

func ehrmsLeaveQueryPath(query domain.EHRMSAttendanceQuery) string {
	values := url.Values{}
	if employeeID := strings.TrimSpace(query.EmployeeID); employeeID != "" {
		values.Set("emp", employeeID)
	}
	if year := strings.TrimSpace(query.Year); year != "" {
		values.Set("year", year)
	}
	return ehrmsQueryPath("/leave", values)
}

func ehrmsQueryPath(path string, values url.Values) string {
	if encoded := values.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

// ListLeaveBalances 列出假別餘額（上游 GET /leave-entitlement）。
// 上游回傳為員工年度 entitlement 巢狀結構；此處展平為服務層使用的 flat balance rows。
func (c *Client) ListLeaveBalances(ctx context.Context, query domain.EHRMSAttendanceQuery) ([]domain.EHRMSLeaveBalanceRecord, error) {
	body, err := c.getJSON(ctx, ehrmsLeaveEntitlementQueryPath(query), maxLeaveBalancesResponseBytes, "leave entitlements")
	if err != nil {
		return nil, err
	}
	raw, err := decodeJSONObjectRows(body, "leave entitlements")
	if err != nil {
		return nil, err
	}
	flat := flattenLeaveEntitlementRows(raw)
	rows := make([]domain.EHRMSLeaveBalanceRecord, 0, len(flat))
	for _, row := range flat {
		rows = append(rows, domain.EHRMSLeaveBalanceRecord(stringRecordFromJSON(row)))
	}
	rows = normalizeLeaveBalanceRecords(rows)
	if err := validateEmployeeScopedRows(rows, query.EmployeeID, "leave entitlements"); err != nil {
		return nil, err
	}
	return rows, nil
}

func firstRecordValue(record map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(record[key]); value != "" {
			return value
		}
		for candidate, raw := range record {
			if strings.EqualFold(candidate, key) {
				if value := strings.TrimSpace(raw); value != "" {
					return value
				}
			}
		}
	}
	return ""
}

// ListLeaveDetails 列出已休逐筆明細（上游 GET /leave）。
// 上游回傳為假別聚合（含 balances/details）；此處只展平 details 供同步寫入 leave_records。
func (c *Client) ListLeaveDetails(ctx context.Context, query domain.EHRMSAttendanceQuery) ([]domain.EHRMSLeaveDetailRecord, error) {
	body, err := c.getJSON(ctx, ehrmsLeaveQueryPath(query), maxLeaveDetailsResponseBytes, "leave")
	if err != nil {
		return nil, err
	}
	raw, err := decodeJSONObjectRows(body, "leave")
	if err != nil {
		return nil, err
	}
	flat := flattenLeaveDetailRows(raw)
	rows := make([]domain.EHRMSLeaveDetailRecord, 0, len(flat))
	for _, row := range flat {
		rows = append(rows, domain.EHRMSLeaveDetailRecord(stringRecordFromJSON(row)))
	}
	rows = normalizeLeaveDetailRecords(rows)
	if err := validateEmployeeScopedRows(rows, query.EmployeeID, "leave"); err != nil {
		return nil, err
	}
	return rows, nil
}

func validateEmployeeScopedRows[T ~map[string]string](rows []T, employeeID string, label string) error {
	expected := strings.TrimSpace(employeeID)
	if expected == "" {
		return nil
	}
	for _, row := range rows {
		if !strings.EqualFold(strings.TrimSpace(row["員工編號"]), expected) {
			return fmt.Errorf("ehrms %s response included employee outside requested scope", label)
		}
	}
	return nil
}

// ListLeaveTypes 列出假別目錄。
func (c *Client) ListLeaveTypes(ctx context.Context) ([]domain.EHRMSLeaveTypeRecord, error) {
	body, err := c.getJSON(ctx, "/leave-types", maxLeaveTypesResponseBytes, "leave types")
	if err != nil {
		return nil, err
	}
	raw, err := decodeJSONObjectRowsOrEnvelope(body, "leave types", "leave_types", "items", "data")
	if err != nil {
		return nil, err
	}
	rows := make([]domain.EHRMSLeaveTypeRecord, 0, len(raw))
	for _, row := range raw {
		rows = append(rows, domain.EHRMSLeaveTypeRecord(stringRecordFromJSON(row)))
	}
	return normalizeLeaveTypeRecords(rows), nil
}

func (c *Client) getJSON(ctx context.Context, path string, maxBytes int, label string) ([]byte, error) {
	release, err := c.acquireRequestSlot(ctx)
	if err != nil {
		return nil, &RequestError{Operation: label, Cause: err}
	}
	defer release()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &RequestError{Operation: label, Cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, &RequestError{Operation: label, StatusCode: resp.StatusCode, Cause: fmt.Errorf("upstream response: %s", strings.TrimSpace(string(body)))}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)+1))
	if err != nil {
		return nil, fmt.Errorf("read ehrms %s: %w", label, err)
	}
	if len(body) > maxBytes {
		return nil, fmt.Errorf("ehrms %s response exceeds %d bytes", label, maxBytes)
	}
	return body, nil
}

// acquireRequestSlot limits the shared client to ten in-flight upstream calls.
// Requests start as soon as a slot is available; no fixed interval is applied.
func (c *Client) acquireRequestSlot(ctx context.Context) (func(), error) {
	select {
	case c.requestSlots <- struct{}{}:
		return func() { <-c.requestSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func decodeJSONObjectRows(body []byte, label string) ([]map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var raw []map[string]any
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode ehrms %s: %w", label, err)
	}
	return raw, nil
}

func decodeJSONObjectRowsOrEnvelope(body []byte, label string, envelopeKeys ...string) ([]map[string]any, error) {
	rows, err := decodeJSONObjectRows(body, label)
	if err == nil {
		return rows, nil
	}

	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var envelope map[string]json.RawMessage
	if decodeErr := decoder.Decode(&envelope); decodeErr != nil {
		return nil, err
	}
	for _, key := range envelopeKeys {
		raw, ok := envelope[key]
		if !ok {
			continue
		}
		rows, decodeErr := decodeJSONObjectRows(raw, label)
		if decodeErr != nil {
			return nil, decodeErr
		}
		return rows, nil
	}
	return nil, err
}
