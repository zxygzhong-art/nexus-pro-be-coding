package service

import (
	"context"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

type agentChatContextKey struct{}

const (
	defaultChatRuntimeTimeout            = 60 * time.Second
	agentChatModeAssistantRecommendation = "assistant_recommendation"
)

// effectiveAgentRuntimeTimeout 统一沿用模型设置，Agent 定义中的旧值仅保留响应相容性。
func effectiveAgentRuntimeTimeout(_ int, modelSeconds int) time.Duration {
	if modelSeconds <= 0 {
		return defaultChatRuntimeTimeout
	}
	return time.Duration(modelSeconds) * time.Second
}

// AgentTool 定義 agent runtime 可呼叫、且受工具與業務權限雙重檢查的工具。
type AgentTool func(context.Context, map[string]any) (map[string]any, error)

// AgentChatEmitFunc 定義 agent chat 事件輸出 callback。
type AgentChatEmitFunc func(context.Context, domain.AgentChatEvent) error

// AgentChatRuntimeRequest 定義 agent runtime 輸入。
type AgentChatRuntimeRequest struct {
	RequestContext domain.RequestContext
	RunID          string
	SessionID      string
	AgentName      string
	AgentRole      string
	ModelName      string
	Message        string
	History        []domain.AgentSessionMessage
	Memories       []domain.AgentMemory
	Mode           string
	Tools          map[string]AgentTool
	SubAgents      []AgentChatSubAgentRuntimeRequest
}

// AgentChatSubAgentRuntimeRequest 定义一个可由主 Agent 委派的运行时成员。
type AgentChatSubAgentRuntimeRequest struct {
	ID        string
	Name      string
	Role      string
	ModelName string
	Tools     map[string]AgentTool
}

type resolvedAgentTeamMember struct {
	ID               string
	Name             string
	Role             string
	ModelName        string
	ToolNames        []string
	KnowledgeBaseIDs []string
}

// AgentChatRuntime 定義 agent chat runtime 行為。
type AgentChatRuntime interface {
	RunAgentChat(context.Context, AgentChatRuntimeRequest, AgentChatEmitFunc) error
}

// WithAgentRequestContext 把 RequestContext 注入 context.Context，供工具執行時還原身份。
func WithAgentRequestContext(ctx context.Context, reqCtx domain.RequestContext) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, agentChatContextKey{}, reqCtx)
}

// AgentRequestContextFromContext 從 context.Context 還原 RequestContext。
func AgentRequestContextFromContext(ctx context.Context) (domain.RequestContext, bool) {
	reqCtx, ok := ctx.Value(agentChatContextKey{}).(domain.RequestContext)
	return reqCtx, ok
}

// Chat 執行流式 agent chat。
func (c AgentService) Chat(ctx RequestContext, input domain.AgentChatInput, emit AgentChatEmitFunc) (AgentRun, error) {
	if c.agentChatRuntime == nil {
		err := domain.E(503, "service_unavailable", "agent chat is disabled").WithReasonCode("agent_chat_disabled")
		return AgentRun{}, err
	}
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionCreate, "")
	if err != nil {
		return AgentRun{}, err
	}
	attachmentIDs := uniqueStrings(input.AttachmentIDs)
	if len(attachmentIDs) > maxAgentChatAttachmentCount {
		return AgentRun{}, BadRequest("a chat message supports at most 8 attachments")
	}
	userMessage := strings.TrimSpace(input.Message)
	if userMessage == "" && len(attachmentIDs) == 0 {
		return AgentRun{}, BadRequest("message is required")
	}
	if userMessage == "" {
		userMessage = "请分析附件。"
	}
	agentID := strings.TrimSpace(input.AgentID)
	sessionID := strings.TrimSpace(input.SessionID)
	var session domain.AgentSession
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "assistant_chat"
	}
	if sessionID != "" {
		session, err = c.currentAgentSession(ctx, account.ID, sessionID)
		if err != nil {
			return AgentRun{}, err
		}
		if session.Status != domain.AgentSessionStatusActive {
			return AgentRun{}, BadRequest("agent session is archived")
		}
		boundAgentID := strings.TrimSpace(session.AgentID)
		if agentID != "" && agentID != boundAgentID {
			return AgentRun{}, BadRequest("agent id does not match the session")
		}
		agentID = boundAgentID
	}
	var configuredTools []string
	var configuredKnowledgeBaseIDs []string
	limitTools := false
	systemPrompt := ""
	modelName := ""
	agentName := ""
	agentRole := ""
	var recommendationCatalog []PlatformAssistant
	var resolvedSubAgents []resolvedAgentTeamMember
	runtimeTimeout := defaultChatRuntimeTimeout
	if agentID != "" {
		definition, err := c.publishedAgentDefinition(ctx, agentID)
		if err != nil {
			return AgentRun{}, err
		}
		limitTools = true
		configuredTools = definition.Tools
		configuredKnowledgeBaseIDs = definition.KnowledgeBaseIDs
		agentName = definition.Name
		agentRole = definition.MainAgentRole
		systemPrompt = strings.TrimSpace(definition.SystemPrompt)
		model, err := c.currentAgentModel(ctx, definition.ModelID)
		if err != nil {
			return AgentRun{}, err
		}
		modelName = strings.TrimSpace(model.LiteLLMModel)
		if modelName == "" {
			modelName = strings.TrimSpace(model.ModelName)
		}
		runtimeTimeout = effectiveAgentRuntimeTimeout(definition.TimeoutSeconds, model.TimeoutSeconds)
		resolvedSubAgents, runtimeTimeout, err = c.resolveAgentTeamMembers(ctx, definition.SubAgents, runtimeTimeout)
		if err != nil {
			return AgentRun{}, err
		}
	}
	if mode == agentChatModeAssistantRecommendation {
		if agentID != "" {
			return AgentRun{}, BadRequest("assistant recommendation cannot be bound to an agent")
		}
		systemPrompt, recommendationCatalog, err = c.agentRecommendationSystemPrompt(ctx)
		if err != nil {
			return AgentRun{}, err
		}
		agentName = "助理推荐"
		agentRole = "根据当前账号可见的助理目录推荐最匹配的助理，并说明选择理由。"
		configuredTools = nil
		limitTools = true
	}
	if sessionID == "" {
		if len(attachmentIDs) > 0 {
			return AgentRun{}, BadRequest("session_id is required when sending attachments")
		}
		session, err = c.createAgentSessionForChat(ctx, account.ID, agentID, agentSessionTitleFromMessage(userMessage))
		if err != nil {
			return AgentRun{}, err
		}
		sessionID = session.ID
	}
	if err := c.ensureNoActiveAgentRun(ctx, sessionID); err != nil {
		return AgentRun{}, err
	}
	for _, fileID := range attachmentIDs {
		file, ok, err := c.store.GetCurrentAgentSessionFile(goContext(ctx), ctx.TenantID, sessionID, fileID)
		if err != nil {
			return AgentRun{}, err
		}
		if !ok {
			return AgentRun{}, NotFound("agent session file", fileID)
		}
		if file.ParseStatus != "ready" {
			return AgentRun{}, Conflict("conversation file is not ready: " + fileID)
		}
	}
	history, err := c.agentChatHistoryForSession(ctx, sessionID)
	if err != nil {
		return AgentRun{}, err
	}
	if err := c.rememberAgentPreferenceIfNeeded(ctx, account.ID, agentID, sessionID, userMessage); err != nil {
		return AgentRun{}, err
	}
	memories, err := c.agentMemoriesForChat(ctx, account.ID, agentID, sessionID, agentMemoryContextLimit)
	if err != nil {
		return AgentRun{}, err
	}
	_, fileContext, err := c.currentAgentFilesForRuntime(ctx, sessionID, attachmentIDs)
	if err != nil {
		return AgentRun{}, err
	}
	runtimeUserMessage := userMessage
	if fileContext != "" {
		runtimeUserMessage += "\n\n" + fileContext
	}
	runtimeMessage := buildAgentRuntimeMessage(systemPrompt, history, memories, runtimeUserMessage)

	run := AgentRun{
		ID:        utils.NewID("arun"),
		TenantID:  ctx.TenantID,
		AccountID: account.ID,
		AgentID:   agentID,
		SessionID: sessionID,
		Mode:      mode,
		Prompt:    userMessage,
		Status:    string(AgentRunStatusQueued),
		CreatedAt: c.Now(),
		UpdatedAt: c.Now(),
	}
	messageID := utils.NewID("amsg")
	messageCreatedAt := c.Now()
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		locked, err := tx.lockCurrentAgentSession(ctx, account.ID, sessionID)
		if err != nil {
			return err
		}
		if locked.Status != domain.AgentSessionStatusActive {
			return BadRequest("agent session is archived").WithReasonCode("agent_session_archived")
		}
		if locked.ContextVersion != session.ContextVersion {
			return Conflict("agent session context changed; retry the message").WithReasonCode("agent_session_context_changed")
		}
		if err := tx.ensureNoActiveAgentRun(ctx, sessionID); err != nil {
			return err
		}
		for _, fileID := range attachmentIDs {
			file, ok, err := tx.store.GetCurrentAgentSessionFile(goContext(ctx), ctx.TenantID, sessionID, fileID)
			if err != nil {
				return err
			}
			if !ok {
				return NotFound("agent session file", fileID)
			}
			if file.ParseStatus != "ready" {
				return Conflict("conversation file is not ready: " + fileID)
			}
		}
		session = locked
		if err := tx.store.UpsertAgentRun(goContext(ctx), run); err != nil {
			return err
		}
		if err := tx.store.InsertAgentSessionMessage(goContext(ctx), domain.AgentSessionMessage{
			ID: messageID, TenantID: ctx.TenantID, SessionID: sessionID, Role: domain.AgentMessageRoleUser,
			Content: userMessage, RunID: run.ID, ContextVersion: session.ContextVersion, CreatedAt: messageCreatedAt,
		}); err != nil {
			return err
		}
		for ordinal, fileID := range attachmentIDs {
			if err := tx.store.MarkAgentSessionFileAttached(goContext(ctx), ctx.TenantID, sessionID, fileID, messageCreatedAt); err != nil {
				return err
			}
			if err := tx.store.InsertAgentMessageAttachment(goContext(ctx), ctx.TenantID, messageID, fileID, ordinal, messageCreatedAt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return AgentRun{}, err
	}
	run, err = c.transitionRun(ctx, run, AgentRunStatusRunning)
	if err != nil {
		return AgentRun{}, err
	}

	if emit == nil {
		emit = func(context.Context, domain.AgentChatEvent) error { return nil }
	}
	baseCtx := goContext(ctx)
	var cancel context.CancelFunc
	baseCtx, cancel = context.WithTimeout(baseCtx, runtimeTimeout)
	defer cancel()
	baseCtx = WithAgentRequestContext(baseCtx, ctx)
	if err := emit(baseCtx, domain.AgentChatEvent{Event: domain.AgentChatEventSession, SessionID: sessionID, RunID: run.ID}); err != nil {
		_ = c.failRun(ctx, run, err)
		return run, err
	}
	answer := strings.Builder{}
	wrappedEmit := func(eventCtx context.Context, event domain.AgentChatEvent) error {
		if event.Event == "" {
			event.Event = domain.AgentChatEventMessageDelta
		}
		if event.Event == domain.AgentChatEventMessageDelta && (len(resolvedSubAgents) == 0 || event.AgentName == "" || event.AgentName == agentName) {
			answer.WriteString(event.Delta)
		}
		return emit(eventCtx, event)
	}
	runtimeSubAgents := make([]AgentChatSubAgentRuntimeRequest, 0, len(resolvedSubAgents))
	for _, member := range resolvedSubAgents {
		runtimeSubAgents = append(runtimeSubAgents, AgentChatSubAgentRuntimeRequest{
			ID:        member.ID,
			Name:      member.Name,
			Role:      member.Role,
			ModelName: member.ModelName,
			Tools:     c.filteredAgentTools(ctx, member.ToolNames, true, wrappedEmit, member.KnowledgeBaseIDs),
		})
	}
	req := AgentChatRuntimeRequest{
		RequestContext: ctx,
		RunID:          run.ID,
		SessionID:      sessionID,
		AgentName:      agentName,
		AgentRole:      agentRole,
		ModelName:      modelName,
		Message:        runtimeMessage,
		History:        history,
		Memories:       memories,
		Mode:           mode,
		Tools:          c.filteredAgentTools(ctx, configuredTools, limitTools, wrappedEmit, configuredKnowledgeBaseIDs),
		SubAgents:      runtimeSubAgents,
	}
	runtimeErr := c.agentChatRuntime.RunAgentChat(baseCtx, req, wrappedEmit)
	if runtimeErr != nil && mode == agentChatModeAssistantRecommendation && strings.TrimSpace(answer.String()) == "" {
		fallbackAnswer := assistantRecommendationFallback(userMessage, recommendationCatalog)
		if fallbackAnswer != "" {
			if emitErr := wrappedEmit(baseCtx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: fallbackAnswer}); emitErr != nil {
				runtimeErr = emitErr
			} else {
				c.logger.Warn("assistant recommendation runtime failed; using visible catalog fallback", "error", runtimeErr, "session_id", sessionID, "run_id", run.ID)
				runtimeErr = nil
			}
		}
	}
	if runtimeErr != nil {
		run.Answer = runtimeErr.Error()
		failed := run
		if next, failErr := c.transitionRun(ctx, run, AgentRunStatusFailed); failErr == nil {
			failed = next
		}
		failed.Answer = runtimeErr.Error()
		_ = c.store.UpsertAgentRun(goContext(ctx), failed)
		return failed, runtimeErr
	}
	run.Answer = strings.TrimSpace(answer.String())
	completedRun, err := c.completeAgentChat(ctx, account.ID, session, run, userMessage)
	if err != nil {
		_ = c.failRun(ctx, run, err)
		return run, err
	}
	run = completedRun
	if err := emit(baseCtx, domain.AgentChatEvent{Event: domain.AgentChatEventDone, RunID: run.ID, Status: string(AgentRunStatusCompleted)}); err != nil {
		return run, err
	}
	return run, nil
}

// agentRecommendationSystemPrompt builds a trusted catalog from assistants visible to the current account.
func (c AgentService) agentRecommendationSystemPrompt(ctx RequestContext) (string, []PlatformAssistant, error) {
	response, err := c.Platform().ListAssistants(ctx, PlatformAssistantsQuery{})
	if err != nil {
		return "", nil, err
	}
	var builder strings.Builder
	builder.WriteString("你正在执行助理推荐任务。只能从下面当前账号可见的助理目录中推荐，不得编造不存在的助理。请优先给出一个最匹配选项，并简要说明理由；没有合适选项时要明确说明。")
	for _, assistant := range response.Data {
		builder.WriteString("\n- ")
		builder.WriteString(strings.TrimSpace(assistant.Title))
		if id := strings.TrimSpace(assistant.ID); id != "" {
			builder.WriteString(" [")
			builder.WriteString(id)
			builder.WriteString("]")
		}
		if tag := strings.TrimSpace(assistant.Tag); tag != "" {
			builder.WriteString("（")
			builder.WriteString(tag)
			builder.WriteString("）")
		}
		if desc := strings.TrimSpace(assistant.Desc); desc != "" {
			builder.WriteString(": ")
			builder.WriteString(desc)
		}
	}
	return builder.String(), response.Data, nil
}

// assistantRecommendationFallback selects only from the already-authorized catalog when the LLM is unavailable.
func assistantRecommendationFallback(message string, catalog []PlatformAssistant) string {
	if len(catalog) == 0 {
		return ""
	}
	best := catalog[0]
	bestScore := assistantRecommendationScore(message, best)
	for _, assistant := range catalog[1:] {
		score := assistantRecommendationScore(message, assistant)
		if score > bestScore {
			best = assistant
			bestScore = score
		}
	}
	detail := "。"
	if description := strings.TrimSpace(best.Desc); description != "" {
		detail = "：" + description
		hasTerminalPunctuation := false
		for _, suffix := range []string{"。", ".", "!", "！", "?", "？"} {
			hasTerminalPunctuation = hasTerminalPunctuation || strings.HasSuffix(description, suffix)
		}
		if !hasTerminalPunctuation {
			detail += "。"
		}
	}
	if bestScore == 0 {
		return "目前目录里没有完全匹配的助理；最接近的是「" + strings.TrimSpace(best.Title) + "」" + detail
	}
	return "当前最匹配的是「" + strings.TrimSpace(best.Title) + "」" + detail
}

// assistantRecommendationScore matches common multilingual task concepts without inventing catalog entries.
func assistantRecommendationScore(message string, assistant PlatformAssistant) int {
	query := strings.ToLower(strings.TrimSpace(message))
	candidate := strings.ToLower(strings.TrimSpace(assistant.Title + " " + assistant.Desc + " " + assistant.Tag))
	if query == "" || candidate == "" {
		return 0
	}
	topicGroups := [][]string{
		{"員工", "员工", "人事", "hr", "請假", "请假", "申請", "申请", "申訴", "申诉"},
		{"客訴", "客诉", "投訴", "投诉", "客服"},
		{"業績", "业绩", "銷售", "销售", "報表", "报表", "分析"},
		{"週報", "周报", "專案", "项目", "進度", "进度"},
		{"新人", "入職", "入职", "onboarding", "培訓", "培训"},
		{"合約", "合同", "法務", "法务"},
		{"資安", "资安", "安全", "風控", "风控", "登入", "登录"},
		{"產品", "产品", "規格", "规格", "提案"},
		{"招聘", "招募", "履歷", "简历", "面試", "面试"},
	}
	score := 0
	for _, group := range topicGroups {
		queryMatches := false
		candidateMatches := false
		for _, keyword := range group {
			queryMatches = queryMatches || strings.Contains(query, keyword)
			candidateMatches = candidateMatches || strings.Contains(candidate, keyword)
		}
		if queryMatches && candidateMatches {
			score += 3
		}
	}
	if strings.Contains(query, strings.ToLower(strings.TrimSpace(assistant.Title))) {
		score += 5
	}
	return score
}

func (c AgentService) ensureNoActiveAgentRun(ctx RequestContext, sessionID string) error {
	count, err := c.store.CountActiveAgentRunsBySession(goContext(ctx), ctx.TenantID, strings.TrimSpace(sessionID))
	if err != nil {
		return err
	}
	if count > 0 {
		return Conflict("agent session already has an active chat run").WithReasonCode("agent_chat_run_active")
	}
	return nil
}

func (c AgentService) agentChatHistoryForSession(ctx RequestContext, sessionID string) ([]domain.AgentSessionMessage, error) {
	items, err := c.store.ListAgentSessionMessages(goContext(ctx), ctx.TenantID, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	lastClearIndex := -1
	for idx, item := range items {
		if isAgentContextClearMessage(item) {
			lastClearIndex = idx
		}
	}
	if lastClearIndex >= 0 {
		items = items[lastClearIndex+1:]
	}
	filtered := make([]domain.AgentSessionMessage, 0, len(items))
	for _, item := range items {
		if item.Role == domain.AgentMessageRoleSystem && isAgentContextClearMessage(item) {
			continue
		}
		filtered = append(filtered, item)
	}
	if len(filtered) > agentSessionHistoryLimit {
		filtered = filtered[len(filtered)-agentSessionHistoryLimit:]
	}
	return filtered, nil
}

func isAgentContextClearMessage(item domain.AgentSessionMessage) bool {
	if item.Role != domain.AgentMessageRoleSystem || item.Metadata == nil {
		return false
	}
	event, _ := item.Metadata["event"].(string)
	return event == agentContextClearedEvent
}

func (c AgentService) createAgentSessionForChat(ctx RequestContext, accountID, agentID, title string) (domain.AgentSession, error) {
	now := c.Now()
	session := domain.AgentSession{
		ID:             utils.NewID("asess"),
		TenantID:       ctx.TenantID,
		AccountID:      accountID,
		AgentID:        strings.TrimSpace(agentID),
		Title:          strings.TrimSpace(title),
		Status:         domain.AgentSessionStatusActive,
		ContextVersion: 1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := c.store.UpsertAgentSession(goContext(ctx), session); err != nil {
		return domain.AgentSession{}, err
	}
	return session, nil
}

// completeAgentChat commits the assistant message, run status, and session timestamp under one context-version lock.
func (c AgentService) completeAgentChat(ctx RequestContext, accountID string, expectedSession domain.AgentSession, run AgentRun, userMessage string) (AgentRun, error) {
	previousStatus := run.Status
	run.Status = string(AgentRunStatusCompleted)
	run.UpdatedAt = c.Now()
	messageCreatedAt := c.Now()
	if err := c.withTransaction(ctx, func(tx AgentService) error {
		session, err := tx.lockCurrentAgentSession(ctx, accountID, expectedSession.ID)
		if err != nil {
			return err
		}
		if session.ContextVersion != expectedSession.ContextVersion {
			return Conflict("agent session context changed while the message was running").WithReasonCode("agent_session_context_changed")
		}
		if err := tx.store.InsertAgentSessionMessage(goContext(ctx), domain.AgentSessionMessage{
			ID:             utils.NewID("amsg"),
			TenantID:       ctx.TenantID,
			SessionID:      session.ID,
			Role:           domain.AgentMessageRoleAssistant,
			Content:        run.Answer,
			RunID:          run.ID,
			ContextVersion: session.ContextVersion,
			CreatedAt:      messageCreatedAt,
		}); err != nil {
			return err
		}
		if strings.TrimSpace(session.Title) == "" {
			session.Title = agentSessionTitleFromMessage(userMessage)
		}
		session.LastMessageAt = &messageCreatedAt
		session.UpdatedAt = messageCreatedAt
		if err := tx.store.UpsertAgentSession(goContext(ctx), session); err != nil {
			return err
		}
		return tx.store.UpsertAgentRun(goContext(ctx), run)
	}); err != nil {
		return AgentRun{}, err
	}
	c.logInfo(ctx, "agent run status changed",
		"run_id", run.ID,
		"mode", run.Mode,
		"previous_status", previousStatus,
		"status", run.Status,
	)
	return run, nil
}

func (c AgentService) rememberAgentPreferenceIfNeeded(ctx RequestContext, accountID, agentID, sessionID, message string) error {
	content := extractAgentAutoMemory(message)
	if content == "" {
		return nil
	}
	now := c.Now()
	memory := domain.AgentMemory{
		ID:         utils.NewID("amem"),
		TenantID:   ctx.TenantID,
		AccountID:  accountID,
		AgentID:    strings.TrimSpace(agentID),
		SessionID:  strings.TrimSpace(sessionID),
		Key:        "preference",
		Content:    content,
		Source:     domain.AgentMemorySourceAuto,
		Importance: 3,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	return c.store.UpsertAgentMemory(goContext(ctx), memory)
}

func (c AgentService) agentMemoriesForChat(ctx RequestContext, accountID, agentID, sessionID string, limit int) ([]domain.AgentMemory, error) {
	if limit <= 0 {
		return nil, nil
	}
	out := make([]domain.AgentMemory, 0, limit)
	seen := map[string]struct{}{}
	appendUnique := func(items []domain.AgentMemory) {
		for _, item := range items {
			if item.Key == agentConfirmationMemoryKey {
				continue
			}
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			out = append(out, item)
			if len(out) >= limit {
				return
			}
		}
	}
	if strings.TrimSpace(agentID) != "" {
		items, err := c.store.ListAgentMemoriesByAccount(goContext(ctx), ctx.TenantID, accountID, strings.TrimSpace(agentID), "", limit)
		if err != nil {
			return nil, err
		}
		exact := make([]domain.AgentMemory, 0, len(items))
		generic := make([]domain.AgentMemory, 0, len(items))
		for _, item := range items {
			if item.AgentID == strings.TrimSpace(agentID) {
				exact = append(exact, item)
			} else {
				generic = append(generic, item)
			}
		}
		appendUnique(exact)
		appendUnique(generic)
	}
	if len(out) < limit {
		items, err := c.store.ListAgentMemoriesByAccount(goContext(ctx), ctx.TenantID, accountID, "", strings.TrimSpace(sessionID), limit)
		if err != nil {
			return nil, err
		}
		appendUnique(items)
	}
	if len(out) < limit {
		items, err := c.store.ListAgentMemoriesByAccount(goContext(ctx), ctx.TenantID, accountID, "", "", limit)
		if err != nil {
			return nil, err
		}
		appendUnique(items)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func extractAgentAutoMemory(message string) string {
	text := strings.TrimSpace(message)
	if text == "" {
		return ""
	}
	for _, marker := range []string{"我叫", "我是", "记住", "記得"} {
		if strings.Contains(text, marker) {
			return firstAgentMemorySentence(text)
		}
	}
	return ""
}

func firstAgentMemorySentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for _, sep := range []string{"。", "！", "？", "\n", ".", "!", "?"} {
		if idx := strings.Index(text, sep); idx >= 0 {
			text = text[:idx]
			break
		}
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > 120 {
		runes = runes[:120]
	}
	return string(runes)
}

func buildAgentRuntimeMessage(systemPrompt string, history []domain.AgentSessionMessage, memories []domain.AgentMemory, message string) string {
	var builder strings.Builder
	if systemPrompt = strings.TrimSpace(systemPrompt); systemPrompt != "" {
		builder.WriteString(systemPrompt)
		builder.WriteString("\n\n")
	}
	builder.WriteString("Known facts:")
	if len(memories) == 0 {
		builder.WriteString("\n- None")
	} else {
		for _, memory := range memories {
			if content := strings.TrimSpace(memory.Content); content != "" {
				builder.WriteString("\n- ")
				builder.WriteString(content)
			}
		}
	}
	if len(history) > 0 {
		builder.WriteString("\n\nConversation history:")
		for _, item := range history {
			content := strings.TrimSpace(item.Content)
			if content == "" {
				continue
			}
			builder.WriteString("\n")
			builder.WriteString(agentMessageRoleLabel(item.Role))
			builder.WriteString(": ")
			builder.WriteString(content)
		}
	}
	builder.WriteString("\n\nUser: ")
	builder.WriteString(strings.TrimSpace(message))
	return builder.String()
}

func agentMessageRoleLabel(role domain.AgentMessageRole) string {
	switch role {
	case domain.AgentMessageRoleAssistant:
		return "Assistant"
	case domain.AgentMessageRoleSystem:
		return "System"
	case domain.AgentMessageRoleTool:
		return "Tool"
	default:
		return "User"
	}
}

// resolveAgentTeamMembers 解析每个子 Agent 的模型路由，并以最慢模型的设置作为 Team 执行上限。
func (c AgentService) resolveAgentTeamMembers(ctx RequestContext, members []domain.AgentTeamMember, timeout time.Duration) ([]resolvedAgentTeamMember, time.Duration, error) {
	out := make([]resolvedAgentTeamMember, 0, len(members))
	for _, member := range members {
		model, err := c.currentAgentModel(ctx, member.ModelID)
		if err != nil {
			return nil, timeout, err
		}
		modelName := strings.TrimSpace(model.LiteLLMModel)
		if modelName == "" {
			modelName = strings.TrimSpace(model.ModelName)
		}
		memberTimeout := effectiveAgentRuntimeTimeout(0, model.TimeoutSeconds)
		if memberTimeout > timeout {
			timeout = memberTimeout
		}
		out = append(out, resolvedAgentTeamMember{
			ID:               member.ID,
			Name:             member.Name,
			Role:             member.Role,
			ModelName:        modelName,
			ToolNames:        append([]string(nil), member.Tools...),
			KnowledgeBaseIDs: append([]string(nil), member.KnowledgeBaseIDs...),
		})
	}
	return out, timeout, nil
}

// filteredAgentTools 只向 runtime 暴露 Agent 定義明確啟用的工具。
func (c AgentService) filteredAgentTools(reqCtx RequestContext, allowed []string, limit bool, emit AgentChatEmitFunc, knowledgeBaseIDs []string) map[string]AgentTool {
	tools := c.agentTools(reqCtx, emit, knowledgeBaseIDs)
	if !limit {
		return tools
	}
	keep := map[string]struct{}{}
	for _, name := range allowed {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			keep[trimmed] = struct{}{}
		}
	}
	out := make(map[string]AgentTool, len(keep))
	for name, tool := range tools {
		if _, ok := keep[name]; ok {
			out[name] = tool
		}
	}
	return out
}

// filteredReadonlyAgentTools 限制試用入口不直接改動業務單據，避免試用 Agent 產生草稿或執行提交審批。
func (c AgentService) filteredReadonlyAgentTools(reqCtx RequestContext, allowed []string, limit bool, emit AgentChatEmitFunc, knowledgeBaseIDs []string) map[string]AgentTool {
	tools := c.filteredAgentTools(reqCtx, allowed, limit, emit, knowledgeBaseIDs)
	readonly := make(map[string]struct{})
	for _, meta := range agentToolCatalog() {
		if meta.Readonly {
			readonly[meta.Value] = struct{}{}
		}
	}
	for name := range tools {
		if _, ok := readonly[name]; !ok {
			delete(tools, name)
		}
	}
	return tools
}

// agentTools 建立目前支援的 Agent 工具並統一套用身份、授權與事件輸出。
func (c AgentService) agentTools(reqCtx RequestContext, emit AgentChatEmitFunc, knowledgeBaseIDs []string) map[string]AgentTool {
	tool := func(name string, execute func(domain.RequestContext, map[string]any) (map[string]any, error)) AgentTool {
		return func(ctx context.Context, args map[string]any) (map[string]any, error) {
			actualCtx, ok := AgentRequestContextFromContext(ctx)
			if !ok {
				actualCtx = reqCtx
			}
			actualCtx.Context = ctx
			_ = emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventToolCall, Name: name, Status: "started"})
			result, err := c.agentToolGateway().Call(actualCtx, AgentToolCall{
				Name: name,
				Authz: CheckRequest{
					ApplicationCode: AppAgent,
					ResourceType:    ResourceTool,
					ResourceID:      name,
					Action:          ActionCall,
				},
				Execute: func() (AgentToolResult, error) {
					data, err := execute(actualCtx, args)
					if err != nil {
						return AgentToolResult{}, err
					}
					return AgentToolResult{Data: data}, nil
				},
			})
			if err != nil {
				_ = emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventToolResult, Name: name, Status: "denied"})
				return nil, err
			}
			if confirmation, ok := result.Data["confirmation"].(*domain.AgentConfirmation); ok && confirmation != nil {
				_ = emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventConfirmation, Confirmation: confirmation})
			}
			_ = emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventToolResult, Name: name, Status: "ok", Data: result.Data})
			return result.Data, nil
		}
	}
	return map[string]AgentTool{
		"knowledge.search": tool("knowledge.search", func(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
			return c.toolKnowledgeSearch(ctx, args, knowledgeBaseIDs)
		}),
		"get_my_profile":                tool("get_my_profile", c.toolGetMyProfile),
		"list_employees":                tool("list_employees", c.toolListEmployees),
		"get_employee":                  tool("get_employee", c.toolGetEmployee),
		"my_leave_balances":             tool("my_leave_balances", c.toolMyLeaveBalances),
		"my_clock_records":              tool("my_clock_records", c.toolMyClockRecords),
		"my_pending_reviews":            tool("my_pending_reviews", c.toolMyPendingReviews),
		"workspace_insights":            tool("workspace_insights", c.toolWorkspaceInsights),
		"list_published_form_templates": tool("list_published_form_templates", c.toolListPublishedFormTemplates),
		"get_published_form_template":   tool("get_published_form_template", c.toolGetPublishedFormTemplate),
		"create_form_draft":             tool("create_form_draft", c.toolCreateFormDraft),
		"update_form_draft":             tool("update_form_draft", c.toolUpdateFormDraft),
		"preview_form_submission":       tool("preview_form_submission", c.toolPreviewFormSubmission),
		"prepare_bulk_review":           tool("prepare_bulk_review", c.toolPrepareBulkReview),
		"form.get_capabilities":         tool("form.get_capabilities", c.toolFormGetCapabilities),
		"form.get_data_source_schema":   tool("form.get_data_source_schema", c.toolFormGetDataSourceSchema),
		"form.create_draft":             tool("form.create_draft", c.toolFormCreateDraft),
		"form.update_draft":             tool("form.update_draft", c.toolFormUpdateDraft),
		"form.validate_draft":           tool("form.validate_draft", c.toolFormValidateDraft),
		"form.preview_draft":            tool("form.preview_draft", c.toolFormPreviewDraft),
		"form.simulate_workflow":        tool("form.simulate_workflow", c.toolFormSimulateWorkflow),
	}
}

// agentToolDescription 提供模型可依循的工具契約，避免猜測寫入與確認語意。
func agentToolDescription(name string) string {
	descriptions := map[string]string{
		"knowledge.search":              "Search tenant knowledge. Args: query string.",
		"get_my_profile":                "Read the current account and employee profile. No args.",
		"list_employees":                "List employee summaries. Optional args: limit.",
		"get_employee":                  "Read one employee. Args: employee_id.",
		"my_leave_balances":             "Read current user's leave balances. No args.",
		"my_clock_records":              "Read current user's clock records. Optional args: limit.",
		"my_pending_reviews":            "Read workflow items the current account can review. No args.",
		"workspace_insights":            "Read workspace insight reports. Optional args: month.",
		"list_published_form_templates": "List published and enabled forms that can be submitted. No args.",
		"get_published_form_template":   "Read one published form schema, allowed data sources, and approval path. Args: template_key.",
		"create_form_draft":             "Create a reversible form draft. Args: template_key and payload object using exact field IDs returned by get_published_form_template. Leave payload field IDs are leave_type, start_at, end_at, hours, reason, and proxy. Preserve explicit user times as RFC3339; omit start_at and end_at only when the user did not provide them.",
		"update_form_draft":             "Replace the payload of the current user's draft. Args: draft_id and complete payload object using exact field IDs. Leave payload field IDs are leave_type, start_at, end_at, hours, reason, and proxy.",
		"preview_form_submission":       "Validate a draft and show a confirmation card. Args: draft_id. Never submits the form.",
		"prepare_bulk_review":           "Prepare a fixed approval batch and show a confirmation card. Args: action, optional form_instance_ids, and reason for reject/return. Never performs approval.",
		"form.get_capabilities":         "Read the schema v2, widgets, metadata-only data sources, and workflow target roles allowed for form authoring.",
		"form.get_data_source_schema":   "Read metadata-only data source fields for form authoring. Never returns business records.",
		"form.create_draft":             "Create a controlled form definition draft. Args: schema object, optional agent_run_id/tool_call_id. Never publishes.",
		"form.update_draft":             "Update a controlled form definition draft. Args: draft_id, revision, and schema object. Never publishes.",
		"form.validate_draft":           "Validate and compile a form definition draft. Args: draft_id. Returns structured errors.",
		"form.preview_draft":            "Preview a form definition draft without side effects. Args: draft_id.",
		"form.simulate_workflow":        "Simulate the configured approval path without starting a real workflow. Args: draft_id.",
	}
	if description := descriptions[name]; description != "" {
		return description
	}
	return "Nexus Pro agent tool: " + name
}

// toolKnowledgeSearch 維持工具目錄與 runtime registry 一致，並沿用目前租戶知識查詢入口。
func (c AgentService) toolKnowledgeSearch(ctx domain.RequestContext, args map[string]any, knowledgeBaseIDs []string) (map[string]any, error) {
	query := strings.TrimSpace(stringFromAny(args["query"]))
	if query == "" {
		query = strings.TrimSpace(stringFromAny(args["text"]))
	}
	if query == "" {
		return nil, BadRequest("query is required")
	}
	answer, references, err := c.answerAgentPrompt(ctx, query, knowledgeBaseIDs)
	if err != nil {
		return nil, err
	}
	return map[string]any{"answer": answer, "references": references}, nil
}

func (c AgentService) toolGetMyProfile(ctx domain.RequestContext, _ map[string]any) (map[string]any, error) {
	me, err := c.Me().Resolve(ctx)
	if err != nil {
		return nil, err
	}
	account := map[string]any{"id": me.Account.ID, "display_name": me.Account.DisplayName, "email": me.Account.Email}
	var employee map[string]any
	if me.Employee != nil {
		employee = map[string]any{"id": me.Employee.ID, "name": me.Employee.Name, "employee_no": me.Employee.EmployeeNo, "position": me.Employee.Position, "org_unit_id": me.Employee.OrgUnitID}
	}
	return map[string]any{"account": account, "employee": employee}, nil
}

func (c AgentService) toolListEmployees(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	limit := intFromToolArgs(args, "limit", 20, 50)
	page, err := c.HR().QueryEmployees(ctx, EmployeeQuery{Page: 1, PageSize: limit})
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(page.Items))
	for _, employee := range page.Items {
		items = append(items, map[string]any{"id": employee.ID, "name": employee.Name, "employee_no": employee.EmployeeNo, "status": employee.Status, "position": employee.Position, "org_unit_id": employee.OrgUnitID})
	}
	return map[string]any{"items": items, "total": page.Total}, nil
}

func (c AgentService) toolGetEmployee(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	id := strings.TrimSpace(stringFromAny(args["employee_id"]))
	if id == "" {
		id = strings.TrimSpace(stringFromAny(args["id"]))
	}
	if id == "" {
		return nil, BadRequest("employee_id is required")
	}
	employee, err := c.HR().GetEmployee(ctx, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": employee.ID, "name": employee.Name, "employee_no": employee.EmployeeNo, "status": employee.Status, "position": employee.Position, "org_unit_id": employee.OrgUnitID}, nil
}

func (c AgentService) toolMyLeaveBalances(ctx domain.RequestContext, _ map[string]any) (map[string]any, error) {
	page, err := c.Attendance().ListLeaveBalancePage(ctx, PageRequest{Page: 1, PageSize: 20})
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": page.Items, "total": page.Total}, nil
}

func (c AgentService) toolMyClockRecords(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	limit := intFromToolArgs(args, "limit", 20, 50)
	page, err := c.Attendance().ListAttendanceClockRecordPage(ctx, domain.AttendanceClockRecordQuery{}, PageRequest{Page: 1, PageSize: limit, Sort: "clocked_at_desc"})
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": page.Items, "total": page.Total}, nil
}

func (c AgentService) toolMyPendingReviews(ctx domain.RequestContext, _ map[string]any) (map[string]any, error) {
	queue, err := c.Workflow().ReviewQueue(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"pending_review": queue.PendingReview}, nil
}

func (c AgentService) toolWorkspaceInsights(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	insights, err := c.Workspace().Insights(ctx, domain.PlatformInsightsQuery{Month: strings.TrimSpace(stringFromAny(args["month"]))})
	if err != nil {
		return nil, err
	}
	return map[string]any{"month": insights.Month, "reports": insights.Reports}, nil
}

func intFromToolArgs(args map[string]any, key string, fallback int, max int) int {
	if args == nil {
		return fallback
	}
	switch v := args[key].(type) {
	case int:
		if v > 0 && v <= max {
			return v
		}
	case float64:
		value := int(v)
		if value > 0 && value <= max {
			return value
		}
	}
	return fallback
}
