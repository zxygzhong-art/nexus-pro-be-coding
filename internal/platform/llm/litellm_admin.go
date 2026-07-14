package llm

import (
	"bytes"
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

// LiteLLMAdminConfig defines the LiteLLM proxy admin client config.
type LiteLLMAdminConfig struct {
	BaseURL   string
	APIKey    string
	MasterKey string
	Client    *http.Client
}

// LiteLLMAdminClient syncs local model aliases into LiteLLM and verifies routes.
type LiteLLMAdminClient struct {
	baseURL   string
	apiKey    string
	masterKey string
	client    *http.Client
}

// NewLiteLLMAdminClient creates a LiteLLM admin client.
func NewLiteLLMAdminClient(cfg LiteLLMAdminConfig) (*LiteLLMAdminClient, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("litellm base url is required")
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	masterKey := strings.TrimSpace(cfg.MasterKey)
	if apiKey == "" && masterKey == "" {
		return nil, fmt.Errorf("litellm api key or master key is required")
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &LiteLLMAdminClient{baseURL: baseURL, apiKey: apiKey, masterKey: masterKey, client: client}, nil
}

// SyncModel upserts the local model alias into LiteLLM's model registry.
func (c *LiteLLMAdminClient) SyncModel(ctx context.Context, model domain.AgentModel) (string, error) {
	if c == nil {
		return "", fmt.Errorf("litellm admin client is not configured")
	}
	alias := liteLLMAlias(model)
	if alias == "" {
		return "", fmt.Errorf("litellm model alias is required")
	}
	upstream := liteLLMUpstreamModel(model)
	if upstream == "" {
		upstream = alias
	}
	params := map[string]any{
		"model": upstream,
	}
	if apiKey := strings.TrimSpace(model.APIKey); apiKey != "" {
		params["api_key"] = apiKey
	}
	if apiBase := strings.TrimSpace(model.APIBaseURL); apiBase != "" {
		params["api_base"] = apiBase
	}
	if model.RateLimitRPM > 0 {
		params["rpm"] = model.RateLimitRPM
	}
	if model.TimeoutSeconds > 0 {
		params["timeout"] = model.TimeoutSeconds
	}
	payload := map[string]any{
		"model_name":     alias,
		"litellm_params": params,
		"model_info": map[string]any{
			"id":        model.ID,
			"tenant_id": model.TenantID,
			"name":      model.Name,
			"provider":  model.Provider,
			"status":    model.Status,
		},
	}
	exists, existingAlias, err := c.modelState(ctx, model.ID)
	if err != nil {
		return "", err
	}
	path := "/model/new"
	if exists {
		if existingAlias != "" && existingAlias != alias {
			if _, err := c.DeleteModel(ctx, model.ID); err != nil {
				return "", err
			}
		} else {
			path = "/model/update"
		}
	}
	if _, _, err := c.doJSON(ctx, http.MethodPost, path, c.masterKeyOrAPIKey(), payload); err != nil {
		if path == "/model/update" {
			if _, _, addErr := c.doJSON(ctx, http.MethodPost, "/model/new", c.masterKeyOrAPIKey(), payload); addErr == nil {
				return fmt.Sprintf("synced LiteLLM route %s -> %s", alias, upstream), nil
			}
		}
		return "", err
	}
	return fmt.Sprintf("synced LiteLLM route %s -> %s", alias, upstream), nil
}

// ListManagedModelIDs 列出由 Nexus 穩定 alias 管理的 LiteLLM deployment ID。
func (c *LiteLLMAdminClient) ListManagedModelIDs(ctx context.Context) ([]string, error) {
	if c == nil {
		return nil, fmt.Errorf("litellm admin client is not configured")
	}
	_, body, err := c.doJSON(ctx, http.MethodGet, "/model/info", c.masterKeyOrAPIKey(), nil)
	if err != nil {
		return nil, err
	}
	entries, err := decodeLiteLLMModelInfo(body)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(strings.TrimSpace(entry.ModelName), "nexus-agent-model-") && strings.TrimSpace(entry.ModelInfo.ID) != "" {
			ids = append(ids, strings.TrimSpace(entry.ModelInfo.ID))
		}
	}
	return ids, nil
}

// DeleteModel 依本地模型 ID 冪等刪除 LiteLLM 路由。
func (c *LiteLLMAdminClient) DeleteModel(ctx context.Context, id string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("litellm admin client is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("litellm model id is required")
	}
	status, body, err := c.doJSON(ctx, http.MethodPost, "/model/delete", c.masterKeyOrAPIKey(), map[string]any{"id": id})
	if err != nil && !isLiteLLMModelNotFound(status, body) {
		return "", err
	}
	return fmt.Sprintf("deleted LiteLLM route %s", domain.AgentModelLiteLLMAlias(id)), nil
}

// TestModel sends a minimal chat completion request through LiteLLM.
func (c *LiteLLMAdminClient) TestModel(ctx context.Context, model domain.AgentModel) (string, error) {
	if c == nil {
		return "", fmt.Errorf("litellm admin client is not configured")
	}
	alias := liteLLMAlias(model)
	if alias == "" {
		return "", fmt.Errorf("litellm model alias is required")
	}
	payload := map[string]any{
		"model": alias,
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
		"max_tokens": 1,
	}
	if _, _, err := c.doJSON(ctx, http.MethodPost, "/chat/completions", c.apiKeyOrMasterKey(), payload); err != nil {
		return "", err
	}
	return fmt.Sprintf("LiteLLM route %s responded", alias), nil
}

// modelState 查詢 LiteLLM registry 是否已有相同模型 ID 與現存 alias。
func (c *LiteLLMAdminClient) modelState(ctx context.Context, id string) (bool, string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, "", fmt.Errorf("litellm model id is required")
	}
	path := "/model/info?litellm_model_id=" + url.QueryEscape(id)
	status, body, err := c.doJSON(ctx, http.MethodGet, path, c.masterKeyOrAPIKey(), nil)
	if isLiteLLMModelNotFound(status, body) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	entries, err := decodeLiteLLMModelInfo(body)
	if err != nil {
		return false, "", err
	}
	if len(entries) == 0 {
		return false, "", nil
	}
	return true, strings.TrimSpace(entries[0].ModelName), nil
}

// isLiteLLMModelNotFound 僅將明確的 deployment 不存在回應轉為可新增/冪等刪除語義。
func isLiteLLMModelNotFound(status int, body []byte) bool {
	if status == http.StatusNotFound {
		return true
	}
	if status != http.StatusBadRequest {
		return false
	}
	var response struct {
		Detail struct {
			Error string `json:"error"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(response.Detail.Error))
	return strings.Contains(message, "model id") &&
		strings.Contains(message, "not found") &&
		strings.Contains(message, "litellm proxy")
}

type liteLLMModelInfoEntry struct {
	ModelName string `json:"model_name"`
	ModelInfo struct {
		ID string `json:"id"`
	} `json:"model_info"`
}

// decodeLiteLLMModelInfo 同時接受 LiteLLM data 的單筆與陣列回應。
func decodeLiteLLMModelInfo(body []byte) ([]liteLLMModelInfoEntry, error) {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode LiteLLM model info: %w", err)
	}
	data := strings.TrimSpace(string(envelope.Data))
	if data == "" || data == "null" || data == "[]" || data == "{}" {
		return nil, nil
	}
	if strings.HasPrefix(data, "[") {
		var entries []liteLLMModelInfoEntry
		if err := json.Unmarshal(envelope.Data, &entries); err != nil {
			return nil, fmt.Errorf("decode LiteLLM model info list: %w", err)
		}
		return entries, nil
	}
	var entry liteLLMModelInfoEntry
	if err := json.Unmarshal(envelope.Data, &entry); err != nil {
		return nil, fmt.Errorf("decode LiteLLM model info item: %w", err)
	}
	return []liteLLMModelInfoEntry{entry}, nil
}

// doJSON 執行帶 Bearer token 的 LiteLLM JSON 請求並回傳 HTTP 狀態。
func (c *LiteLLMAdminClient) doJSON(ctx context.Context, method, path, token string, payload map[string]any) (int, []byte, error) {
	if strings.TrimSpace(token) == "" {
		return 0, nil, fmt.Errorf("litellm token is required")
	}
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(encoded)
	}
	basePath, rawQuery, _ := strings.Cut(path, "?")
	endpoint, err := url.JoinPath(c.baseURL, strings.TrimPrefix(basePath, "/"))
	if err != nil {
		return 0, nil, err
	}
	if rawQuery != "" {
		endpoint += "?" + rawQuery
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return 0, nil, err
	}
	if payload != nil {
		req.Header.Set("content-type", "application/json")
	}
	req.Header.Set("authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.StatusCode, respBody, nil
	}
	message := strings.TrimSpace(string(respBody))
	if message == "" {
		message = resp.Status
	}
	return resp.StatusCode, respBody, fmt.Errorf("litellm %s returned %s: %s", path, resp.Status, message)
}

func (c *LiteLLMAdminClient) masterKeyOrAPIKey() string {
	if c.masterKey != "" {
		return c.masterKey
	}
	return c.apiKey
}

func (c *LiteLLMAdminClient) apiKeyOrMasterKey() string {
	if c.apiKey != "" {
		return c.apiKey
	}
	return c.masterKey
}

func liteLLMAlias(model domain.AgentModel) string {
	return domain.AgentModelLiteLLMAlias(model.ID)
}

// liteLLMUpstreamModel 轉換 provider 與上游模型名為 LiteLLM provider/model 格式。
func liteLLMUpstreamModel(model domain.AgentModel) string {
	name := strings.TrimSpace(model.ModelName)
	if name == "" || strings.Contains(name, "/") {
		return name
	}
	switch strings.ToLower(strings.TrimSpace(model.Provider)) {
	case "openai":
		return "openai/" + name
	case "anthropic":
		return "anthropic/" + name
	case "google", "gemini":
		return "gemini/" + name
	case "azure":
		return "azure/" + name
	default:
		return name
	}
}
