//go:build adk

package llm

import (
	"context"
	"errors"
	"iter"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

// LiteLLMConfig defines the OpenAI-compatible LiteLLM model adapter config.
type LiteLLMConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

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
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("agent model name is required")
	}
	return &LiteLLM{
		client: openai.NewClient(
			option.WithAPIKey(cfg.APIKey),
			option.WithBaseURL(strings.TrimRight(cfg.BaseURL, "/")),
		),
		model: strings.TrimSpace(cfg.Model),
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
	params := m.params(req)
	completion, err := m.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, err
	}
	if len(completion.Choices) == 0 {
		return nil, errors.New("litellm returned no choices")
	}
	content := completion.Choices[0].Message.Content
	return llmTextResponse(content, false, true), nil
}

func (m *LiteLLM) generateStream(ctx context.Context, req *model.LLMRequest) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		stream := m.client.Chat.Completions.NewStreaming(ctx, m.params(req))
		for stream.Next() {
			chunk := stream.Current()
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
		yield(llmTextResponse("", false, true), nil)
	}
}

func (m *LiteLLM) params(req *model.LLMRequest) openai.ChatCompletionNewParams {
	modelName := m.model
	if req != nil && strings.TrimSpace(req.Model) != "" {
		modelName = strings.TrimSpace(req.Model)
	}
	return openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(modelName),
		Messages: contentsToMessages(req),
	}
}

func contentsToMessages(req *model.LLMRequest) []openai.ChatCompletionMessageParamUnion {
	if req == nil || len(req.Contents) == 0 {
		return []openai.ChatCompletionMessageParamUnion{openai.UserMessage("")}
	}
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Contents))
	for _, content := range req.Contents {
		text := contentText(content)
		switch strings.ToLower(strings.TrimSpace(content.Role)) {
		case "system":
			messages = append(messages, openai.SystemMessage(text))
		case "model", "assistant":
			messages = append(messages, openai.AssistantMessage(text))
		default:
			messages = append(messages, openai.UserMessage(text))
		}
	}
	return messages
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
