package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"iter"
	"math"
	"regexp"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

// LiteLLMConfig defines the OpenAI-compatible LiteLLM model adapter config.
type LiteLLMConfig struct {
	BaseURL string
	APIKey  string
}

const liteLLMFallbackModelName = "nexus-agent-fallback"

var openAIToolNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)

// LiteLLM implements ADK model.LLM using the official OpenAI Go SDK.
type LiteLLM struct {
	client openai.Client
	model  string
}

// NewLiteLLM creates an ADK LLM backed by a LiteLLM OpenAI-compatible endpoint.
func NewLiteLLM(cfg LiteLLMConfig) (*LiteLLM, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("litellm base url is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("litellm api key is required")
	}
	return &LiteLLM{
		client: openai.NewClient(
			option.WithAPIKey(cfg.APIKey),
			option.WithBaseURL(strings.TrimRight(cfg.BaseURL, "/")),
		),
		model: liteLLMFallbackModelName,
	}, nil
}

func (m *LiteLLM) Name() string {
	if m == nil {
		return ""
	}
	return m.model
}

func (m *LiteLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	if stream {
		return m.generateStream(ctx, req)
	}
	return func(yield func(*model.LLMResponse, error) bool) {
		resp, err := m.generate(ctx, req)
		yield(resp, err)
	}
}

func (m *LiteLLM) generate(ctx context.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	params, aliases, err := m.params(req)
	if err != nil {
		return nil, err
	}
	completion, err := m.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, err
	}
	if len(completion.Choices) == 0 {
		return nil, errors.New("litellm returned no choices")
	}
	return llmResponseFromMessage(completion.Choices[0].Message, aliases, completion.Usage)
}

func (m *LiteLLM) generateStream(ctx context.Context, req *model.LLMRequest) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		params, aliases, err := m.params(req)
		if err != nil {
			yield(nil, err)
			return
		}
		stream := m.client.Chat.Completions.NewStreaming(ctx, params)
		accumulator := openai.ChatCompletionAccumulator{}
		finalUsage := openai.CompletionUsage{}
		for stream.Next() {
			chunk := stream.Current()
			if hasOpenAIUsage(chunk.Usage) {
				finalUsage = chunk.Usage
			}
			if !accumulator.AddChunk(chunk) {
				yield(nil, errors.New("litellm returned an inconsistent stream"))
				return
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			delta := chunk.Choices[0].Delta.Content
			if delta == "" {
				continue
			}
			if !yield(llmTextResponse(delta, true, false), nil) {
				return
			}
		}
		if err := stream.Err(); err != nil {
			yield(nil, err)
			return
		}
		if !hasOpenAIUsage(finalUsage) {
			finalUsage = accumulator.Usage
		}
		if len(accumulator.Choices) > 0 && len(accumulator.Choices[0].Message.ToolCalls) > 0 {
			response, err := llmResponseFromMessage(accumulator.Choices[0].Message, aliases, finalUsage)
			yield(response, err)
			return
		}
		response := llmTextResponse("", false, true)
		response.UsageMetadata = usageMetadataFromOpenAI(finalUsage)
		yield(response, nil)
	}
}

// params 將 ADK 的系統指令、對話與函數聲明完整映射到 OpenAI Chat Completions。
func (m *LiteLLM) params(req *model.LLMRequest) (openai.ChatCompletionNewParams, map[string]string, error) {
	modelName := m.model
	if req != nil && strings.TrimSpace(req.Model) != "" {
		modelName = strings.TrimSpace(req.Model)
	}
	messages, err := contentsToMessages(req)
	if err != nil {
		return openai.ChatCompletionNewParams{}, nil, err
	}
	tools, aliases, err := openAITools(req)
	if err != nil {
		return openai.ChatCompletionNewParams{}, nil, err
	}
	return openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(modelName),
		Messages: messages,
		Tools:    tools,
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		},
	}, aliases, nil
}

// contentsToMessages 保留函數調用 ID 與函數回應，使 ADK 能執行多輪工具循環。
func contentsToMessages(req *model.LLMRequest) ([]openai.ChatCompletionMessageParamUnion, error) {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0)
	if req != nil && req.Config != nil && req.Config.SystemInstruction != nil {
		if instruction := strings.TrimSpace(contentText(req.Config.SystemInstruction)); instruction != "" {
			messages = append(messages, openai.SystemMessage(instruction))
		}
	}
	if req == nil {
		return append(messages, openai.UserMessage("")), nil
	}
	validCallIDs := map[string]struct{}{}
	callIDsByName := map[string][]string{}
	consumedCallIDs := map[string]struct{}{}
	for contentIndex, content := range req.Contents {
		if content == nil {
			continue
		}
		text := contentText(content)
		role := strings.ToLower(strings.TrimSpace(content.Role))
		if role == "model" || role == "assistant" {
			message := openai.AssistantMessage(text)
			calls := make([]functionCallRef, 0)
			for partIndex, part := range content.Parts {
				if part == nil || part.FunctionCall == nil {
					continue
				}
				call := part.FunctionCall
				callID := strings.TrimSpace(call.ID)
				if callID == "" {
					callID = fmt.Sprintf("call_%d_%d", contentIndex, partIndex)
				}
				arguments, err := json.Marshal(call.Args)
				if err != nil {
					return nil, fmt.Errorf("marshal function call %s: %w", call.Name, err)
				}
				calls = append(calls, functionCallRef{ID: callID, Name: call.Name})
				message.OfAssistant.ToolCalls = append(message.OfAssistant.ToolCalls, openai.ChatCompletionMessageToolCallParam{
					ID: callID,
					Function: openai.ChatCompletionMessageToolCallFunctionParam{
						Name:      openAIToolName(call.Name),
						Arguments: string(arguments),
					},
				})
			}
			if len(calls) > 0 && !hasCompleteFunctionResponses(req.Contents, contentIndex, calls) {
				if strings.TrimSpace(text) != "" {
					messages = append(messages, openai.AssistantMessage(text))
				}
				continue
			}
			for _, call := range calls {
				validCallIDs[call.ID] = struct{}{}
				callIDsByName[call.Name] = append(callIDsByName[call.Name], call.ID)
			}
			messages = append(messages, message)
			continue
		}
		for _, part := range content.Parts {
			if part == nil || part.FunctionResponse == nil {
				continue
			}
			response := part.FunctionResponse
			callID := strings.TrimSpace(response.ID)
			if callID == "" {
				callID = firstUnconsumedCallID(callIDsByName[response.Name], consumedCallIDs)
			}
			if _, ok := validCallIDs[callID]; !ok {
				continue
			}
			if _, consumed := consumedCallIDs[callID]; consumed {
				continue
			}
			payload, err := json.Marshal(response.Response)
			if err != nil {
				return nil, fmt.Errorf("marshal function response %s: %w", response.Name, err)
			}
			messages = append(messages, openai.ToolMessage(string(payload), callID))
			consumedCallIDs[callID] = struct{}{}
		}
		if text != "" {
			switch role {
			case "system":
				messages = append(messages, openai.SystemMessage(text))
			default:
				messages = append(messages, openai.UserMessage(text))
			}
		}
	}
	if len(messages) == 0 {
		messages = append(messages, openai.UserMessage(""))
	}
	return messages, nil
}

type functionCallRef struct {
	ID   string
	Name string
}

// hasCompleteFunctionResponses 只保留能在下一條普通消息前完整配對的工具調用組。
func hasCompleteFunctionResponses(contents []*genai.Content, callIndex int, calls []functionCallRef) bool {
	expected := make(map[string]string, len(calls))
	matched := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		expected[call.ID] = call.Name
	}
	for index := callIndex + 1; index < len(contents); index++ {
		content := contents[index]
		if content == nil {
			continue
		}
		responseCount := 0
		for _, part := range content.Parts {
			if part == nil || part.FunctionResponse == nil {
				continue
			}
			responseCount++
			response := part.FunctionResponse
			callID := strings.TrimSpace(response.ID)
			if callID == "" {
				callID = firstMatchingCallID(calls, response.Name, matched)
			}
			if _, ok := expected[callID]; ok {
				matched[callID] = struct{}{}
			}
		}
		if len(matched) == len(expected) {
			return true
		}
		if responseCount == 0 {
			return false
		}
	}
	return false
}

// firstMatchingCallID 依原始調用順序為缺少 ID 的函數回應補上同名調用 ID。
func firstMatchingCallID(calls []functionCallRef, name string, used map[string]struct{}) string {
	for _, call := range calls {
		if call.Name != name {
			continue
		}
		if _, ok := used[call.ID]; !ok {
			return call.ID
		}
	}
	return ""
}

// firstUnconsumedCallID 返回尚未生成 tool message 的第一個調用 ID。
func firstUnconsumedCallID(callIDs []string, consumed map[string]struct{}) string {
	for _, callID := range callIDs {
		if _, ok := consumed[callID]; !ok {
			return callID
		}
	}
	return ""
}

// openAITools 將 ADK JSON Schema 聲明轉換為 OpenAI 函數工具，並記錄安全名稱映射。
func openAITools(req *model.LLMRequest) ([]openai.ChatCompletionToolParam, map[string]string, error) {
	aliases := map[string]string{}
	if req == nil || req.Config == nil {
		return nil, aliases, nil
	}
	tools := make([]openai.ChatCompletionToolParam, 0)
	for _, group := range req.Config.Tools {
		if group == nil {
			continue
		}
		for _, declaration := range group.FunctionDeclarations {
			if declaration == nil || strings.TrimSpace(declaration.Name) == "" {
				continue
			}
			parameters, err := functionParameters(declaration)
			if err != nil {
				return nil, nil, fmt.Errorf("convert tool schema %s: %w", declaration.Name, err)
			}
			alias := openAIToolName(declaration.Name)
			aliases[alias] = declaration.Name
			tools = append(tools, openai.ChatCompletionToolParam{Function: shared.FunctionDefinitionParam{
				Name:        alias,
				Description: openai.String(declaration.Description),
				Parameters:  parameters,
			}})
		}
	}
	return tools, aliases, nil
}

// functionParameters 通過 JSON 邊界兼容 ADK 的標準 Schema 與 JSON Schema 兩種聲明。
func functionParameters(declaration *genai.FunctionDeclaration) (shared.FunctionParameters, error) {
	schema := declaration.ParametersJsonSchema
	if schema == nil {
		schema = declaration.Parameters
	}
	if schema == nil {
		return shared.FunctionParameters{"type": "object", "properties": map[string]any{}}, nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	parameters := shared.FunctionParameters{}
	if err := json.Unmarshal(raw, &parameters); err != nil {
		return nil, err
	}
	normalizeOpenAIJSONSchema(map[string]any(parameters))
	return parameters, nil
}

// normalizeOpenAIJSONSchema converts ADK/GenAI uppercase schema types to OpenAI JSON Schema types.
func normalizeOpenAIJSONSchema(value any) {
	switch typed := value.(type) {
	case map[string]any:
		if rawType, ok := typed["type"]; ok {
			typed["type"] = normalizeOpenAIJSONSchemaType(rawType)
		}
		for _, child := range typed {
			normalizeOpenAIJSONSchema(child)
		}
	case []any:
		for _, child := range typed {
			normalizeOpenAIJSONSchema(child)
		}
	}
}

// normalizeOpenAIJSONSchemaType normalizes both single and union JSON Schema type declarations.
func normalizeOpenAIJSONSchemaType(value any) any {
	switch typed := value.(type) {
	case string:
		return strings.ToLower(strings.TrimSpace(typed))
	case []any:
		for index, item := range typed {
			if typeName, ok := item.(string); ok {
				typed[index] = strings.ToLower(strings.TrimSpace(typeName))
			}
		}
	}
	return value
}

// openAIToolName 為含點號或過長的 ADK 工具生成穩定的 OpenAI 合法別名。
func openAIToolName(name string) string {
	name = strings.TrimSpace(name)
	if openAIToolNamePattern.MatchString(name) {
		return name
	}
	replacer := strings.NewReplacer(".", "_", ":", "_", "/", "_", " ", "_")
	prefix := replacer.Replace(name)
	if prefix == "" || !regexp.MustCompile(`^[A-Za-z_]`).MatchString(prefix) {
		prefix = "tool_" + prefix
	}
	prefix = regexp.MustCompile(`[^A-Za-z0-9_-]`).ReplaceAllString(prefix, "_")
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(name))
	suffix := fmt.Sprintf("_%08x", hash.Sum32())
	if max := 64 - len(suffix); len(prefix) > max {
		prefix = prefix[:max]
	}
	return prefix + suffix
}

func contentText(content *genai.Content) string {
	if content == nil {
		return ""
	}
	var out strings.Builder
	for _, part := range content.Parts {
		if part != nil && part.Text != "" {
			out.WriteString(part.Text)
		}
	}
	return out.String()
}

// llmResponseFromMessage 將 OpenAI 的文本或函數調用恢復成 ADK 可執行回應。
func llmResponseFromMessage(message openai.ChatCompletionMessage, aliases map[string]string, usage openai.CompletionUsage) (*model.LLMResponse, error) {
	parts := make([]*genai.Part, 0, 1+len(message.ToolCalls))
	if message.Content != "" {
		parts = append(parts, genai.NewPartFromText(message.Content))
	}
	for _, toolCall := range message.ToolCalls {
		args := map[string]any{}
		if strings.TrimSpace(toolCall.Function.Arguments) != "" {
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
				return nil, fmt.Errorf("decode tool call %s arguments: %w", toolCall.Function.Name, err)
			}
		}
		name := aliases[toolCall.Function.Name]
		if name == "" {
			name = toolCall.Function.Name
		}
		part := genai.NewPartFromFunctionCall(name, args)
		part.FunctionCall.ID = toolCall.ID
		parts = append(parts, part)
	}
	return &model.LLMResponse{
		Content:       &genai.Content{Role: "model", Parts: parts},
		UsageMetadata: usageMetadataFromOpenAI(usage),
		TurnComplete:  true,
	}, nil
}

// usageMetadataFromOpenAI preserves LiteLLM billing counters across the ADK boundary.
func usageMetadataFromOpenAI(usage openai.CompletionUsage) *genai.GenerateContentResponseUsageMetadata {
	if !hasOpenAIUsage(usage) {
		return nil
	}
	return &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:        tokenCount32(usage.PromptTokens),
		CachedContentTokenCount: tokenCount32(usage.PromptTokensDetails.CachedTokens),
		CandidatesTokenCount:    tokenCount32(usage.CompletionTokens),
		TotalTokenCount:         tokenCount32(usage.TotalTokens),
	}
}

// hasOpenAIUsage distinguishes an omitted streaming usage block from valid zero counters.
func hasOpenAIUsage(usage openai.CompletionUsage) bool {
	return usage.PromptTokens != 0 || usage.CompletionTokens != 0 || usage.TotalTokens != 0 || usage.PromptTokensDetails.CachedTokens != 0 ||
		usage.JSON.PromptTokens.Valid() || usage.JSON.CompletionTokens.Valid() || usage.JSON.TotalTokens.Valid()
}

// tokenCount32 clamps provider counters to the ADK metadata field width.
func tokenCount32(value int64) int32 {
	if value <= 0 {
		return 0
	}
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(value)
}

func llmTextResponse(text string, partial bool, complete bool) *model.LLMResponse {
	return &model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{genai.NewPartFromText(text)},
		},
		Partial:      partial,
		TurnComplete: complete,
	}
}

var _ model.LLM = (*LiteLLM)(nil)
