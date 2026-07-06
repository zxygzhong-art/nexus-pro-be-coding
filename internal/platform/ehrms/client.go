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

	"nexus-pro-be/internal/domain"
)

// Client 定義 client 的資料結構。
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

const maxEmployeesResponseBytes = 10 << 20

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
	return &Client{baseURL: baseURL, apiKey: apiKey, httpClient: httpClient}, nil
}

// Ping 檢查外部服務連線狀態。
func (c *Client) Ping(ctx context.Context) error {
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/employees", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("ehrms employees returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxEmployeesResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read ehrms employees: %w", err)
	}
	if len(body) > maxEmployeesResponseBytes {
		return nil, fmt.Errorf("ehrms employees response exceeds %d bytes", maxEmployeesResponseBytes)
	}
	var rows []domain.EHRMSEmployeeRecord
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode ehrms employees: %w", err)
	}
	return rows, nil
}
