package agent

import (
	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository"
	"nexus-pro-api/internal/service"
)

// base.go — agent 子領域包的基底：收窄 store、建構子、交易輔助。
// 自 internal/service 拆出（見 docs/workflow-naming.md 同系列的拆分計畫）。

type agentStore interface {
	repository.AgentStore
	repository.KnowledgeStore
	repository.OutboxStore
}

// 介面滿足斷言：拆包後仍須符合 api 層依賴的 facade 契約。
var _ service.AgentFacade = AgentService{}

// New 以共用基底建構 Agent 領域服務（取代舊的 (*service.Service).Agent() accessor）。
func New(base *service.Service) AgentService {
	return AgentService{Service: base, store: base.Store()}
}

// withTransaction 讓 Agent 寫入與其管理稽覈在同一租戶交易中完成。
func (c AgentService) withTransaction(ctx RequestContext, fn func(AgentService) error) error {
	return c.Service.WithTenantTransaction(ctx, func(tx *service.Service) error {
		return fn(New(tx))
	})
}

// requireAgentAuthz 處理 require agent 授權的服務流程。
func (c AgentService) requireAgentAuthz(ctx RequestContext, resource ResourceType, action Action, resourceID string) (Account, CheckResult, error) {
	return c.Service.RequireServiceAuthz(ctx, AppAgent, resource, action, resourceID)
}

// ExecuteConfirmation 執行使用者已確認的高風險動作（實作在基底 service 包的跨域執行器）。
func (c AgentService) ExecuteConfirmation(ctx RequestContext, id string, input domain.ExecuteAgentConfirmationInput) (domain.AgentConfirmationExecution, error) {
	return c.Service.ExecuteAgentConfirmation(ctx, id, input)
}
