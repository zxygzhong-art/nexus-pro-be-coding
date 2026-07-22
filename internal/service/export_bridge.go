package service

import (
	"log/slog"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/platform/mcpclient"
)

// export_bridge.go — 子領域包（internal/service/agent 等）存取基底 Service 的橋接。
// 拆分過渡期使用：欄位維持未匯出，只開必要的唯讀 getter 與交易/授權入口。

// Logger 回傳結構化 logger。
func (c *Service) Logger() *slog.Logger { return c.logger }

// KnowledgeEmbedder 回傳知識庫向量化 client。
func (c *Service) KnowledgeEmbedder() KnowledgeEmbedder { return c.knowledgeEmbedder }

// ObjectStore 回傳物件儲存 client。
func (c *Service) ObjectStore() ObjectStore { return c.objectStore }

// CredentialCipher 回傳憑證加解密器。
func (c *Service) CredentialCipher() CredentialCipher { return c.credentialCipher }

// MCPConnector 回傳外部 MCP 連線器。
func (c *Service) MCPConnector() mcpclient.Connector { return c.mcpConnector }

// ExternalHTTPExecutor 回傳手動 HTTP capability 執行器。
func (c *Service) ExternalHTTPExecutor() ExternalHTTPExecutor { return c.externalHTTPExecutor }

// LiteLLMAdmin 回傳 LiteLLM 管理 client。
func (c *Service) LiteLLMAdmin() LiteLLMAdminClient { return c.liteLLMAdmin }

// AgentChatRuntime 回傳 Agent 對話 runtime。
func (c *Service) AgentChatRuntime() AgentChatRuntime { return c.agentChatRuntime }

// WithTenantTransaction 讓子領域包在同一筆租戶交易中組合操作。
func (c *Service) WithTenantTransaction(ctx RequestContext, fn func(*Service) error) error {
	return c.withTenantTransaction(ctx, fn)
}

// RequireServiceAuthz 供子領域包執行「授權 + 帳號解析」的標準前置。
func (c *Service) RequireServiceAuthz(ctx RequestContext, app domain.ApplicationCode, resource domain.ResourceType, action domain.Action, resourceID string) (domain.Account, domain.CheckResult, error) {
	return c.requireServiceAuthz(ctx, app, resource, action, resourceID)
}

// LogInfo 以請求語境記錄 info log。
func (c *Service) LogInfo(ctx RequestContext, message string, args ...any) {
	c.logInfo(ctx, message, args...)
}

// LogWarn 以請求語境記錄 warn log。
func (c *Service) LogWarn(ctx RequestContext, message string, args ...any) {
	c.logWarn(ctx, message, args...)
}

// ResolveAccount 解析請求身分對應的帳號與租戶。
func (c *Service) ResolveAccount(ctx RequestContext) (Account, Tenant, error) {
	return c.resolveAccount(ctx)
}

// RecordAudit 寫入一筆稽覈事件（子領域包用）。
func (c *Service) RecordAudit(ctx RequestContext, action, resource, target, severity string, details map[string]any) error {
	return c.audit(ctx, action, resource, target, severity, details)
}
