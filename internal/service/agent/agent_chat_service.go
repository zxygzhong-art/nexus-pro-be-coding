package agent

import (
	"context"
	"strings"
	"sync"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

type agentChatContextKey struct{}

const (
	defaultChatRuntimeTimeout            = 60 * time.Second
	staleAgentRunGracePeriod             = 30 * time.Second
	interruptedAgentRunMessage           = "agent chat was interrupted before completion"
	agentChatModeAssistantRecommendation = "assistant_recommendation"
)

// effectiveAgentRuntimeTimeout 統一沿用模型設置，Agent 定義中的舊值僅保留響應相容性。
func effectiveAgentRuntimeTimeout(_ int, modelSeconds int) time.Duration {
	if modelSeconds <= 0 {
		return defaultChatRuntimeTimeout
	}
	return time.Duration(modelSeconds) * time.Second
}

// AgentChatEmitFunc 定義 agent chat 事件輸出 callback。

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
	if c.AgentChatRuntime() == nil {
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
		userMessage = "請分析附件。"
	}
	agentID := strings.TrimSpace(input.AgentID)
	sessionID := strings.TrimSpace(input.SessionID)
	var session domain.AgentSession
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "assistant_chat"
	}
	if sessionID != "" {
		session, err = c.CurrentAgentSession(ctx, account.ID, sessionID)
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
	var resolvedSubAgents []ResolvedAgentTeamMember
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
		agentName = "助理推薦"
		agentRole = "根據當前賬號可見的助理目錄推薦最匹配的助理，並說明選擇理由。"
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
	if err := c.recoverStaleAgentRuns(ctx, sessionID, runtimeTimeout); err != nil {
		return AgentRun{}, err
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
	baseCtx = WithAgentChatExecutionContext(baseCtx, AgentChatExecutionContext{
		AgentID: agentID, SessionID: sessionID, RunID: run.ID, ContextVersion: session.ContextVersion,
	})
	if err := emit(baseCtx, domain.AgentChatEvent{Event: domain.AgentChatEventSession, SessionID: sessionID, RunID: run.ID}); err != nil {
		_ = c.FailRun(ctx, run, err)
		return run, err
	}
	answer := strings.Builder{}
	artifactEvents := make([]domain.AgentChatEvent, 0, 4)
	var eventMu sync.Mutex
	var usageMu sync.Mutex
	tokenUsage := domain.AgentTokenUsage{}
	var llmCallCount int64
	// wrappedEmit normalizes runtime events before they cross the public stream boundary.
	wrappedEmit := func(eventCtx context.Context, event domain.AgentChatEvent) error {
		if event.Event == "" {
			event.Event = domain.AgentChatEventMessageDelta
		}
		if event.Event == domain.AgentChatEventError {
			event = AgentRuntimeFailureEvent(ctx, run.ID)
		}
		eventMu.Lock()
		if event.Event == domain.AgentChatEventMessageDelta && (len(resolvedSubAgents) == 0 || event.AgentName == "" || event.AgentName == agentName) {
			answer.WriteString(event.Delta)
		}
		if shouldPersistAgentArtifact(event) {
			artifactEvents = append(artifactEvents, event)
		}
		eventMu.Unlock()
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
		RecordUsage: func(usage domain.AgentTokenUsage) {
			usageMu.Lock()
			llmCallCount++
			tokenUsage.InputTokens += usage.InputTokens
			tokenUsage.CachedTokens += usage.CachedTokens
			tokenUsage.OutputTokens += usage.OutputTokens
			tokenUsage.TotalTokens += usage.TotalTokens
			usageMu.Unlock()
		},
	}
	runtimeErr := c.AgentChatRuntime().RunAgentChat(baseCtx, req, wrappedEmit)
	usageComplete := runtimeErr == nil
	if runtimeErr != nil && mode == agentChatModeAssistantRecommendation && strings.TrimSpace(answer.String()) == "" {
		fallbackAnswer := assistantRecommendationFallback(userMessage, recommendationCatalog)
		if fallbackAnswer != "" {
			if emitErr := wrappedEmit(baseCtx, domain.AgentChatEvent{Event: domain.AgentChatEventMessageDelta, Delta: fallbackAnswer}); emitErr != nil {
				runtimeErr = emitErr
			} else {
				c.Logger().Warn("assistant recommendation runtime failed; using visible catalog fallback", "error", runtimeErr, "session_id", sessionID, "run_id", run.ID)
				runtimeErr = nil
			}
		}
	}
	usageMu.Lock()
	run.LLMCallCount = llmCallCount
	run.InputTokens = tokenUsage.InputTokens
	run.CachedTokens = tokenUsage.CachedTokens
	run.OutputTokens = tokenUsage.OutputTokens
	run.TotalTokens = tokenUsage.TotalTokens
	run.UsageComplete = usageComplete && llmCallCount > 0
	usageMu.Unlock()
	if runtimeErr != nil {
		c.LogWarn(ctx, "agent chat runtime failed",
			"run_id", run.ID,
			"session_id", sessionID,
			"error", runtimeErr,
		)
		failed, failErr := c.failAgentChat(ctx, account.ID, session, run)
		if failErr != nil {
			c.LogWarn(ctx, "persist agent chat failure marker failed", "run_id", run.ID, "session_id", sessionID, "error", failErr)
			run.Answer = agentRuntimeFailureAnswer(ctx)
			_ = c.FailRun(ctx, run, failErr)
			failed = run
			failed.Status = string(AgentRunStatusFailed)
		}
		return failed, agentRuntimeFailureError(ctx)
	}
	eventMu.Lock()
	run.Answer = strings.TrimSpace(answer.String())
	persistedArtifacts := append([]domain.AgentChatEvent(nil), artifactEvents...)
	eventMu.Unlock()
	completedRun, err := c.completeAgentChat(ctx, account.ID, session, run, userMessage, persistedArtifacts)
	if err != nil {
		_ = c.FailRun(ctx, run, err)
		return run, err
	}
	run = completedRun
	if err := emit(baseCtx, domain.AgentChatEvent{Event: domain.AgentChatEventDone, RunID: run.ID, Status: string(AgentRunStatusCompleted)}); err != nil {
		return run, err
	}
	return run, nil
}

// failAgentChat atomically persists a safe assistant failure marker, failed run, and session activity.
func (c AgentService) failAgentChat(ctx RequestContext, accountID string, expectedSession domain.AgentSession, run AgentRun) (AgentRun, error) {
	previousStatus := run.Status
	run.Status = string(AgentRunStatusFailed)
	run.Answer = agentRuntimeFailureAnswer(ctx)
	run.UpdatedAt = c.Now()
	messageCreatedAt := c.Now()
	metadata := map[string]any{
		"status":      "failed",
		"reason_code": AgentRuntimeFailureReasonCode,
	}
	if traceID := strings.TrimSpace(ctx.TraceID); traceID != "" {
		metadata["trace_id"] = traceID
	}
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
			Metadata:       metadata,
			CreatedAt:      messageCreatedAt,
		}); err != nil {
			return err
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
	c.LogInfo(ctx, "agent run status changed", "run_id", run.ID, "mode", run.Mode, "previous_status", previousStatus, "status", run.Status)
	return run, nil
}

// agentRecommendationSystemPrompt builds a trusted catalog from assistants visible to the current account.
func (c AgentService) agentRecommendationSystemPrompt(ctx RequestContext) (string, []PlatformAssistant, error) {
	response, err := c.Platform().ListAssistants(ctx, PlatformAssistantsQuery{})
	if err != nil {
		return "", nil, err
	}
	var builder strings.Builder
	builder.WriteString("你正在執行助理推薦任務。只能從下面當前賬號可見的助理目錄中推薦，不得編造不存在的助理。請優先給出一個最匹配選項，並簡要說明理由；沒有合適選項時要明確說明。")
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
		return "目前目錄裏沒有完全匹配的助理；最接近的是「" + strings.TrimSpace(best.Title) + "」" + detail
	}
	return "當前最匹配的是「" + strings.TrimSpace(best.Title) + "」" + detail
}

// assistantRecommendationScore matches common multilingual task concepts without inventing catalog entries.
func assistantRecommendationScore(message string, assistant PlatformAssistant) int {
	query := strings.ToLower(strings.TrimSpace(message))
	candidate := strings.ToLower(strings.TrimSpace(assistant.Title + " " + assistant.Desc + " " + assistant.Tag))
	if query == "" || candidate == "" {
		return 0
	}
	topicGroups := [][]string{
		{"員工", "員工", "人事", "hr", "請假", "請假", "申請", "申請", "申訴", "申訴"},
		{"客訴", "客訴", "投訴", "投訴", "客服"},
		{"業績", "業績", "銷售", "銷售", "報表", "報表", "分析"},
		{"週報", "週報", "專案", "項目", "進度", "進度"},
		{"新人", "入職", "入職", "onboarding", "培訓", "培訓"},
		{"合約", "合同", "法務", "法務"},
		{"資安", "資安", "安全", "風控", "風控", "登入", "登錄"},
		{"產品", "產品", "規格", "規格", "提案"},
		{"招聘", "招募", "履歷", "簡歷", "面試", "面試"},
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

// recoverStaleAgentRuns releases a session after its previous process died before persisting a terminal status.
func (c AgentService) recoverStaleAgentRuns(ctx RequestContext, sessionID string, runtimeTimeout time.Duration) error {
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	if runtimeTimeout <= 0 {
		runtimeTimeout = defaultChatRuntimeTimeout
	}
	now := c.Now()
	staleBefore := now.Add(-(runtimeTimeout + staleAgentRunGracePeriod))
	recovered, err := c.store.FailStaleAgentRunsBySession(
		goContext(ctx),
		ctx.TenantID,
		strings.TrimSpace(sessionID),
		staleBefore,
		now,
		interruptedAgentRunMessage,
	)
	if err != nil {
		return err
	}
	if recovered > 0 {
		c.LogWarn(ctx, "stale agent runs recovered",
			"session_id", sessionID,
			"recovered_runs", recovered,
			"stale_before", staleBefore,
		)
	}
	return nil
}

func (c AgentService) agentChatHistoryForSession(ctx RequestContext, sessionID string) ([]domain.AgentSessionMessage, error) {
	items, err := c.store.ListAgentSessionMessages(goContext(ctx), ctx.TenantID, strings.TrimSpace(sessionID), domain.KeysetPage{})
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
func (c AgentService) completeAgentChat(ctx RequestContext, accountID string, expectedSession domain.AgentSession, run AgentRun, userMessage string, artifacts []domain.AgentChatEvent) (AgentRun, error) {
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
		for index, artifact := range artifacts {
			metadata, err := agentArtifactMessageMetadata(artifact)
			if err != nil {
				return err
			}
			if err := tx.store.InsertAgentSessionMessage(goContext(ctx), domain.AgentSessionMessage{
				ID:             utils.NewID("amsg"),
				TenantID:       ctx.TenantID,
				SessionID:      session.ID,
				Role:           domain.AgentMessageRoleTool,
				Content:        "",
				RunID:          run.ID,
				ContextVersion: session.ContextVersion,
				Metadata:       metadata,
				CreatedAt:      messageCreatedAt.Add(time.Duration(index) * time.Nanosecond),
			}); err != nil {
				return err
			}
		}
		if err := tx.store.InsertAgentSessionMessage(goContext(ctx), domain.AgentSessionMessage{
			ID:             utils.NewID("amsg"),
			TenantID:       ctx.TenantID,
			SessionID:      session.ID,
			Role:           domain.AgentMessageRoleAssistant,
			Content:        run.Answer,
			RunID:          run.ID,
			ContextVersion: session.ContextVersion,
			CreatedAt:      messageCreatedAt.Add(time.Duration(len(artifacts)) * time.Nanosecond),
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
	c.LogInfo(ctx, "agent run status changed",
		"run_id", run.ID,
		"mode", run.Mode,
		"previous_status", previousStatus,
		"status", run.Status,
	)
	return run, nil
}

// shouldPersistAgentArtifact limits replay storage to UI-safe form artifacts already sent to the current user.
func shouldPersistAgentArtifact(event domain.AgentChatEvent) bool {
	if event.Event != domain.AgentChatEventToolResult || event.Status != "ok" || len(event.Data) == 0 {
		return false
	}
	if event.Name == "get_published_form_template" || event.Name == "create_form_draft" || event.Name == "update_form_draft" {
		return true
	}
	return strings.HasPrefix(event.Name, "form.")
}

// agentArtifactMessageMetadata serializes the event as an opaque JSON string so dynamic form field IDs stay unchanged.
func agentArtifactMessageMetadata(event domain.AgentChatEvent) (map[string]any, error) {
	return domain.EncodeAgentArtifactMetadata(map[string]any{
		"event":  event.Event,
		"name":   event.Name,
		"status": event.Status,
		"data":   event.Data,
	})
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
			if item.Key == AgentConfirmationMemoryKey {
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
	for _, marker := range []string{"我叫", "我是", "記住", "記得"} {
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

// resolveAgentTeamMembers 解析每個子 Agent 的模型路由，並以最慢模型的設置作為 Team 執行上限。
func (c AgentService) resolveAgentTeamMembers(ctx RequestContext, members []domain.AgentTeamMember, timeout time.Duration) ([]ResolvedAgentTeamMember, time.Duration, error) {
	out := make([]ResolvedAgentTeamMember, 0, len(members))
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
		out = append(out, ResolvedAgentTeamMember{
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
	for _, meta := range domain.AgentToolCatalog() {
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
			result, err := c.AgentToolGateway().Call(actualCtx, AgentToolCall{
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
		"check_leave_eligibility":       tool("check_leave_eligibility", c.toolCheckLeaveEligibility),
		"my_clock_records":              tool("my_clock_records", c.toolMyClockRecords),
		"my_attendance_summary":         tool("my_attendance_summary", c.toolMyAttendanceSummary),
		"my_form_history":               tool("my_form_history", c.toolMyFormHistory),
		"my_pending_reviews":            tool("my_pending_reviews", c.toolMyPendingReviews),
		"workspace_insights":            tool("workspace_insights", c.toolWorkspaceInsights),
		"list_published_form_templates": tool("list_published_form_templates", c.ToolListPublishedFormTemplates),
		"get_published_form_template":   tool("get_published_form_template", c.ToolGetPublishedFormTemplate),
		"create_form_draft":             tool("create_form_draft", c.ToolCreateFormDraft),
		"update_form_draft":             tool("update_form_draft", c.ToolUpdateFormDraft),
		"preview_form_submission":       tool("preview_form_submission", c.ToolPreviewFormSubmission),
		"prepare_bulk_review":           tool("prepare_bulk_review", c.ToolPrepareBulkReview),
		"form.get_capabilities":         tool("form.get_capabilities", c.ToolFormGetCapabilities),
		"form.get_data_source_schema":   tool("form.get_data_source_schema", c.ToolFormGetDataSourceSchema),
		"form.create_draft":             tool("form.create_draft", c.ToolFormCreateDraft),
		"form.update_draft":             tool("form.update_draft", c.ToolFormUpdateDraft),
		"form.validate_draft":           tool("form.validate_draft", c.ToolFormValidateDraft),
		"form.preview_draft":            tool("form.preview_draft", c.ToolFormPreviewDraft),
		"form.simulate_workflow":        tool("form.simulate_workflow", c.ToolFormSimulateWorkflow),
	}
}

// agentToolDescription 提供模型可依循的工具契約，避免猜測寫入與確認語意。
func agentToolDescription(name string) string {
	descriptions := map[string]string{
		"knowledge.search":              "Search tenant knowledge. Args: query string.",
		"get_my_profile":                "Read the current account and employee profile. No args.",
		"list_employees":                "List employee summaries. Optional args: limit.",
		"get_employee":                  "Read one employee. Args: employee_id.",
		"my_leave_balances":             "Read only the current employee's leave balances. No args. initialized=false means balance data is missing, not zero; never claim zero balance from an empty items list.",
		"check_leave_eligibility":       "Deterministically check one leave type under the active policy. Args: leave_type, date (YYYY-MM-DD or RFC3339), and hours. Use this before creating a leave draft; do not infer eligibility from my_leave_balances. Missing or insufficient balance is non-blocking: eligible remains true and you must continue creating the draft without balance reservation. Only an unsupported or inactive leave type blocks draft creation.",
		"my_clock_records":              "Read current user's clock records. Optional args: limit.",
		"my_attendance_summary":         "Read the current employee's attendance summary for the current month, including attendance days, worked hours, approved leave days, approved overtime hours, and today's clock status. No args.",
		"my_form_history":               "Read only the current account's form applications. Optional args: template_key, status, and limit. Use template_key=leave-request for leave history.",
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

// toolMyLeaveBalances separates missing balance data from a real zero balance and never returns other employees.
func (c AgentService) toolMyLeaveBalances(ctx domain.RequestContext, _ map[string]any) (map[string]any, error) {
	employeeID, items, err := c.currentEmployeeLeaveBalances(ctx)
	if err != nil {
		return nil, err
	}
	initialized := len(items) > 0
	status := "available"
	message := "Current employee leave balance data is available."
	if !initialized {
		status = "not_initialized"
		message = "Current employee leave balance data is not initialized. Do not treat missing data as a zero balance."
	}
	return map[string]any{
		"employee_id": employeeID,
		"initialized": initialized,
		"status":      status,
		"message":     message,
		"items":       items,
		"total":       len(items),
	}, nil
}

// toolCheckLeaveEligibility applies the same policy and balance prerequisites used by leave submission.
func (c AgentService) toolCheckLeaveEligibility(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	leaveTypeRaw := strings.TrimSpace(stringFromAny(args["leave_type"]))
	if leaveTypeRaw == "" {
		return nil, BadRequest("leave_type is required")
	}
	dateRaw := strings.TrimSpace(stringFromAny(args["date"]))
	if dateRaw == "" {
		return nil, BadRequest("date is required")
	}
	requestedDate, err := utils.ParseDate(dateRaw)
	if err != nil {
		return nil, BadRequest("date must be YYYY-MM-DD or RFC3339")
	}
	hours := floatFromToolArgs(args, "hours")
	if hours <= 0 {
		return nil, BadRequest("hours must be greater than zero")
	}

	employeeID, _, err := c.currentEmployeeLeaveBalances(ctx)
	if err != nil {
		return nil, err
	}
	evaluation, err := c.Attendance().EvaluateLeaveRequestRules(ctx, employeeID, leaveTypeRaw, requestedDate, requestedDate.Add(time.Duration(hours*float64(time.Hour))), hours)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"employee_id":              employeeID,
		"leave_type_id":            evaluation.LeaveTypeID,
		"leave_type":               evaluation.LeaveType,
		"requested_date":           requestedDate.Format(time.DateOnly),
		"required_hours":           hours,
		"supported":                evaluation.Status != LeaveEvaluationUnsupported,
		"eligible":                 evaluation.Eligible,
		"status":                   evaluation.Status,
		"message":                  evaluation.Message,
		"policy_version":           evaluation.PolicyVersion,
		"proof_required":           evaluation.ProofRequired,
		"policy_balance_required":  evaluation.Rule.RequiresBalance,
		"balance_required":         evaluation.BalanceRequired,
		"balance_initialized":      evaluation.BalanceInitialized,
		"balance_fallback_applied": evaluation.BalanceFallbackReason != "",
		"balance_fallback_reason":  evaluation.BalanceFallbackReason,
	}
	if evaluation.Status == LeaveEvaluationUnsupported {
		return result, nil
	}
	result["leave_type_name"] = evaluation.LeaveTypeName
	if evaluation.BalanceFallbackReason != "" {
		if evaluation.BalanceInitialized {
			result["remaining_hours"] = evaluation.AvailableHours
		}
		result["reason"] = "balance_unavailable_fallback"
		return result, nil
	}
	if !evaluation.BalanceRequired {
		result["reason"] = "balance_not_required"
		return result, nil
	}
	result["remaining_hours"] = evaluation.AvailableHours
	result["reason"] = "sufficient_balance"
	return result, nil
}

// currentEmployeeLeaveBalances narrows any authorized attendance scope to the account's own employee record.
func (c AgentService) currentEmployeeLeaveBalances(ctx domain.RequestContext) (string, []LeaveBalance, error) {
	account, _, err := c.ResolveAccount(ctx)
	if err != nil {
		return "", nil, err
	}
	employeeID := strings.TrimSpace(account.EmployeeID)
	if employeeID == "" {
		return "", nil, BadRequest("current account is not linked to an employee")
	}
	items, err := c.Attendance().ListLeaveBalances(ctx)
	if err != nil {
		return "", nil, err
	}
	current := make([]LeaveBalance, 0, len(items))
	for _, item := range items {
		if item.EmployeeID == employeeID {
			current = append(current, item)
		}
	}
	return employeeID, current, nil
}

func (c AgentService) toolMyClockRecords(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	limit := intFromToolArgs(args, "limit", 20, 50)
	page, err := c.Attendance().ListAttendanceClockRecordPage(ctx, domain.AttendanceClockRecordQuery{}, PageRequest{Page: 1, PageSize: limit, Sort: "clocked_at_desc"})
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": page.Items, "total": page.Total}, nil
}

// toolMyAttendanceSummary returns the same self-scoped monthly projection shown on the platform home page.
func (c AgentService) toolMyAttendanceSummary(ctx domain.RequestContext, _ map[string]any) (map[string]any, error) {
	summary, err := c.Platform().ClockSummary(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"month":                   c.Now().Format("2006-01"),
		"date_label":              summary.DateLabel,
		"checked_in_at":           summary.CheckedInAt,
		"checked_out_at":          summary.CheckedOutAt,
		"location":                summary.Location,
		"attendance_days":         summary.MonthlyAttendanceDays,
		"worked_hours":            summary.MonthlyHours,
		"approved_overtime_hours": summary.MonthlyOvertimeHours,
		"approved_leave_days":     summary.LeaveDays,
	}, nil
}

// toolMyFormHistory lists only the caller's applications and optionally narrows them to one published form type.
func (c AgentService) toolMyFormHistory(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	limit := intFromToolArgs(args, "limit", 20, 50)
	templateKey := strings.TrimSpace(stringFromAny(args["template_key"]))
	status := strings.TrimSpace(stringFromAny(args["status"]))
	page, err := c.Workflow().ListFormInstancePage(ctx, domain.FormInstanceQuery{
		TemplateKey: templateKey,
		Status:      status,
		Mine:        true,
	}, PageRequest{Page: 1, PageSize: limit, Sort: "submitted_at_desc"})
	if err != nil {
		return nil, err
	}
	templates, err := c.Workflow().ListFormTemplates(ctx)
	if err != nil {
		return nil, err
	}
	templateByID := make(map[string]domain.FormTemplate, len(templates))
	for _, template := range templates {
		templateByID[template.ID] = template
	}
	items := make([]map[string]any, 0, len(page.Items))
	for _, instance := range page.Items {
		template := templateByID[instance.TemplateID]
		items = append(items, map[string]any{
			"id":            instance.ID,
			"template_key":  template.Key,
			"template_name": template.Name,
			"status":        instance.Status,
			"payload":       instance.Payload,
			"submitted_at":  instance.SubmittedAt,
			"updated_at":    instance.UpdatedAt,
		})
	}
	return map[string]any{"items": items, "total": page.Total}, nil
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

// floatFromToolArgs reads JSON numeric tool arguments without accepting stringly typed values.
func floatFromToolArgs(args map[string]any, key string) float64 {
	if args == nil {
		return 0
	}
	switch value := args[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return 0
	}
}
