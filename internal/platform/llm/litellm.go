package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"iter"
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
	return llmResponseFromMessage(completion.Choices[0].Message, aliases)
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
		for stream.Next() {
			chunk := stream.Current()
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
		if len(accumulator.Choices) > 0 && len(accumulator.Choices[0].Message.ToolCalls) > 0 {
			response, err := llmResponseFromMessage(accumulator.Choices[0].Message, aliases)
			yield(response, err)
			return
		}
		yield(llmTextResponse("", false, true), nil)
	}
}

// params 将 ADK 的系统指令、对话与函数声明完整映射到 OpenAI Chat Completions。
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
	}, aliases, nil
}

// contentsToMessages 保留函数调用 ID 与函数回应，使 ADK 能执行多轮工具循环。
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
	callIDsByName := map[string]string{}
	for contentIndex, content := range req.Contents {
		if content == nil {
			continue
		}
		text := contentText(content)
		role := strings.ToLower(strings.TrimSpace(content.Role))
		if role == "model" || role == "assistant" {
			message := openai.AssistantMessage(text)
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
				message.OfAssistant.ToolCalls = append(message.OfAssistant.ToolCalls, openai.ChatCompletionMessageToolCallParam{
					ID: callID,
					Function: openai.ChatCompletionMessageToolCallFunctionParam{
						Name:      openAIToolName(call.Name),
						Arguments: string(arguments),
					},
				})
				callIDsByName[call.Name] = callID
			}
			messages = append(messages, message)
			continue
		}
		if text != "" {
			switch role {
			case "system":
				messages = append(messages, openai.SystemMessage(text))
			default:
				messages = append(messages, openai.UserMessage(text))
			}
		}
		for _, part := range content.Parts {
			if part == nil || part.FunctionResponse == nil {
				continue
			}
			response := part.FunctionResponse
			callID := strings.TrimSpace(response.ID)
			if callID == "" {
				callID = callIDsByName[response.Name]
			}
			if callID == "" {
				return nil, fmt.Errorf("function response %s is missing its call id", response.Name)
			}
			payload, err := json.Marshal(response.Response)
			if err != nil {
				return nil, fmt.Errorf("marshal function response %s: %w", response.Name, err)
			}
			messages = append(messages, openai.ToolMessage(string(payload), callID))
		}
	}
	if len(messages) == 0 {
		messages = append(messages, openai.UserMessage(""))
	}
	return messages, nil
}

// openAITools 将 ADK JSON Schema 声明转换为 OpenAI 函数工具，并记录安全名称映射。
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

// functionParameters 通过 JSON 边界兼容 ADK 的标准 Schema 与 JSON Schema 两种声明。
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
	return parameters, nil
}

// openAIToolName 为含点号或过长的 ADK 工具生成稳定的 OpenAI 合法别名。
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

// llmResponseFromMessage 将 OpenAI 的文本或函数调用恢复成 ADK 可执行回应。
func llmResponseFromMessage(message openai.ChatCompletionMessage, aliases map[string]string) (*model.LLMResponse, error) {
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
		Content:      &genai.Content{Role: "model", Parts: parts},
		TurnComplete: true,
	}, nil
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
