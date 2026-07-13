package service

import (
	"context"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

type agentChatContextKey struct{}

const defaultChatRuntimeTimeout = 60 * time.Second

// effectiveAgentRuntimeTimeout 取 Agent 與模型中較嚴格的正值逾時，避免任一設定被 runtime 忽略。
func effectiveAgentRuntimeTimeout(agentSeconds, modelSeconds int) time.Duration {
	seconds := agentSeconds
	if seconds <= 0 || modelSeconds > 0 && modelSeconds < seconds {
		seconds = modelSeconds
	}
	if seconds <= 0 {
		return defaultChatRuntimeTimeout
	}
	return time.Duration(seconds) * time.Second
}

// AgentReadOnlyTool 定義 agent runtime 可呼叫的只讀工具。
type AgentReadOnlyTool func(context.Context, map[string]any) (map[string]any, error)

// AgentChatEmitFunc 定義 agent chat 事件輸出 callback。
type AgentChatEmitFunc func(context.Context, domain.AgentChatEvent) error

// AgentChatRuntimeRequest 定義 agent runtime 輸入。
type AgentChatRuntimeRequest struct {
	RequestContext domain.RequestContext
	RunID          string
	SessionID      string
	ModelName      string
	Message        string
	History        []domain.AgentSessionMessage
	Memories       []domain.AgentMemory
	Mode           string
	Tools          map[string]AgentReadOnlyTool
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
		err := domain.E(503, "service_unavailable", "agent chat is disabled")
		err.ReasonCode = "agent_chat_disabled"
		return AgentRun{}, err
	}
	account, _, err := c.requireAgentAuthz(ctx, ResourceType("run"), ActionCreate, "")
	if err != nil {
		return AgentRun{}, err
	}
	userMessage := strings.TrimSpace(input.Message)
	if userMessage == "" {
		return AgentRun{}, BadRequest("message is required")
	}
	agentID := strings.TrimSpace(input.AgentID)
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID != "" {
		session, err := c.currentAgentSession(ctx, account.ID, sessionID)
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
	limitTools := false
	systemPrompt := ""
	modelName := ""
	runtimeTimeout := defaultChatRuntimeTimeout
	if agentID != "" {
		definition, err := c.publishedAgentDefinition(ctx, agentID)
		if err != nil {
			return AgentRun{}, err
		}
		limitTools = true
		configuredTools = definition.Tools
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
	}
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "assistant_chat"
	}
	if sessionID == "" {
		session, err := c.createAgentSessionForChat(ctx, account.ID, agentID, agentSessionTitleFromMessage(userMessage))
		if err != nil {
			return AgentRun{}, err
		}
		sessionID = session.ID
	}
	if err := c.ensureNoActiveAgentRun(ctx, sessionID); err != nil {
		return AgentRun{}, err
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
	runtimeMessage := buildAgentRuntimeMessage(systemPrompt, history, memories, userMessage)

	run := AgentRun{
		ID:        utils.NewID("arun"),
		TenantID:  ctx.TenantID,
		AccountID: account.ID,
		AgentID:   agentID,
		SessionID: sessionID,
		Mode:      mode,
		Prompt:    runtimeMessage,
		Status:    string(AgentRunStatusQueued),
		CreatedAt: c.Now(),
		UpdatedAt: c.Now(),
	}
	if err := c.store.UpsertAgentRun(goContext(ctx), run); err != nil {
		return AgentRun{}, err
	}
	if err := c.store.InsertAgentSessionMessage(goContext(ctx), domain.AgentSessionMessage{
		ID:        utils.NewID("amsg"),
		TenantID:  ctx.TenantID,
		SessionID: sessionID,
		Role:      domain.AgentMessageRoleUser,
		Content:   userMessage,
		RunID:     run.ID,
		CreatedAt: c.Now(),
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
		if event.Event == domain.AgentChatEventMessageDelta {
			answer.WriteString(event.Delta)
		}
		return emit(eventCtx, event)
	}
	req := AgentChatRuntimeRequest{
		RequestContext: ctx,
		RunID:          run.ID,
		SessionID:      sessionID,
		ModelName:      modelName,
		Message:        runtimeMessage,
		History:        history,
		Memories:       memories,
		Mode:           mode,
		Tools:          c.filteredAgentReadOnlyTools(ctx, configuredTools, limitTools, wrappedEmit),
	}
	if err := c.agentChatRuntime.RunAgentChat(baseCtx, req, wrappedEmit); err != nil {
		run.Answer = err.Error()
		failed := run
		if next, failErr := c.transitionRun(ctx, run, AgentRunStatusFailed); failErr == nil {
			failed = next
		}
		failed.Answer = err.Error()
		_ = c.store.UpsertAgentRun(goContext(ctx), failed)
		return failed, err
	}
	run.Answer = strings.TrimSpace(answer.String())
	run, err = c.transitionRun(ctx, run, AgentRunStatusCompleted)
	if err != nil {
		return AgentRun{}, err
	}
	if err := c.store.InsertAgentSessionMessage(goContext(ctx), domain.AgentSessionMessage{
		ID:        utils.NewID("amsg"),
		TenantID:  ctx.TenantID,
		SessionID: sessionID,
		Role:      domain.AgentMessageRoleAssistant,
		Content:   run.Answer,
		RunID:     run.ID,
		CreatedAt: c.Now(),
	}); err != nil {
		return AgentRun{}, err
	}
	if err := c.updateAgentSessionAfterChat(ctx, account.ID, sessionID, userMessage); err != nil {
		return AgentRun{}, err
	}
	if err := emit(baseCtx, domain.AgentChatEvent{Event: domain.AgentChatEventDone, RunID: run.ID, Status: string(AgentRunStatusCompleted)}); err != nil {
		return run, err
	}
	return run, nil
}

func (c AgentService) ensureNoActiveAgentRun(ctx RequestContext, sessionID string) error {
	count, err := c.store.CountActiveAgentRunsBySession(goContext(ctx), ctx.TenantID, strings.TrimSpace(sessionID))
	if err != nil {
		return err
	}
	if count > 0 {
		return Conflict("agent session already has an active chat run")
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
		ID:        utils.NewID("asess"),
		TenantID:  ctx.TenantID,
		AccountID: accountID,
		AgentID:   strings.TrimSpace(agentID),
		Title:     strings.TrimSpace(title),
		Status:    domain.AgentSessionStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := c.store.UpsertAgentSession(goContext(ctx), session); err != nil {
		return domain.AgentSession{}, err
	}
	return session, nil
}

func (c AgentService) updateAgentSessionAfterChat(ctx RequestContext, accountID, sessionID, userMessage string) error {
	session, err := c.currentAgentSession(ctx, accountID, sessionID)
	if err != nil {
		return err
	}
	now := c.Now()
	if strings.TrimSpace(session.Title) == "" {
		session.Title = agentSessionTitleFromMessage(userMessage)
	}
	session.LastMessageAt = &now
	session.UpdatedAt = now
	return c.store.UpsertAgentSession(goContext(ctx), session)
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

func (c AgentService) filteredAgentReadOnlyTools(reqCtx RequestContext, allowed []string, limit bool, emit AgentChatEmitFunc) map[string]AgentReadOnlyTool {
	tools := c.agentReadOnlyTools(reqCtx, emit)
	if !limit {
		return tools
	}
	keep := map[string]struct{}{}
	for _, name := range allowed {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			keep[trimmed] = struct{}{}
		}
	}
	out := make(map[string]AgentReadOnlyTool, len(keep))
	for name, tool := range tools {
		if _, ok := keep[name]; ok {
			out[name] = tool
		}
	}
	return out
}

func (c AgentService) agentReadOnlyTools(reqCtx RequestContext, emit AgentChatEmitFunc) map[string]AgentReadOnlyTool {
	tool := func(name string, execute func(domain.RequestContext, map[string]any) (map[string]any, error)) AgentReadOnlyTool {
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
			_ = emit(ctx, domain.AgentChatEvent{Event: domain.AgentChatEventToolResult, Name: name, Status: "ok"})
			return result.Data, nil
		}
	}
	return map[string]AgentReadOnlyTool{
		"knowledge.search":   tool("knowledge.search", c.toolKnowledgeSearch),
		"get_my_profile":     tool("get_my_profile", c.toolGetMyProfile),
		"list_employees":     tool("list_employees", c.toolListEmployees),
		"get_employee":       tool("get_employee", c.toolGetEmployee),
		"my_leave_balances":  tool("my_leave_balances", c.toolMyLeaveBalances),
		"my_clock_records":   tool("my_clock_records", c.toolMyClockRecords),
		"my_pending_reviews": tool("my_pending_reviews", c.toolMyPendingReviews),
		"workspace_insights": tool("workspace_insights", c.toolWorkspaceInsights),
	}
}

// toolKnowledgeSearch 維持工具目錄與 runtime registry 一致，並沿用目前租戶知識查詢入口。
func (c AgentService) toolKnowledgeSearch(ctx domain.RequestContext, args map[string]any) (map[string]any, error) {
	query := strings.TrimSpace(stringFromAny(args["query"]))
	if query == "" {
		query = strings.TrimSpace(stringFromAny(args["text"]))
	}
	if query == "" {
		return nil, BadRequest("query is required")
	}
	answer, references, err := c.answerAgentPrompt(ctx, query)
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
