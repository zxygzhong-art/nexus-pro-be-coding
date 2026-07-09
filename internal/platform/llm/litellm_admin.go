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
	upstream := strings.TrimSpace(model.ModelName)
	if upstream == "" {
		upstream = alias
	}
	payload := map[string]any{
		"model_name": alias,
		"litellm_params": map[string]any{
			"model": upstream,
		},
		"model_info": map[string]any{
			"id":        model.ID,
			"tenant_id": model.TenantID,
			"name":      model.Name,
			"provider":  model.Provider,
			"status":    model.Status,
		},
	}
	if err := c.postJSON(ctx, "/model/new", c.masterKeyOrAPIKey(), payload); err != nil {
		return "", err
	}
	return fmt.Sprintf("synced LiteLLM route %s -> %s", alias, upstream), nil
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
	if err := c.postJSON(ctx, "/chat/completions", c.apiKeyOrMasterKey(), payload); err != nil {
		return "", err
	}
	return fmt.Sprintf("LiteLLM route %s responded", alias), nil
}

func (c *LiteLLMAdminClient) postJSON(ctx context.Context, path string, token string, payload map[string]any) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("litellm token is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint, err := url.JoinPath(c.baseURL, strings.TrimPrefix(path, "/"))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	message := strings.TrimSpace(string(respBody))
	if message == "" {
		message = resp.Status
	}
	return fmt.Errorf("litellm %s returned %s: %s", path, resp.Status, message)
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
	if alias := strings.TrimSpace(model.LiteLLMModel); alias != "" {
		return alias
	}
	return strings.TrimSpace(model.ModelName)
}
