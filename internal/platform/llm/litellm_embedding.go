package llm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const (
	defaultLiteLLMEmbeddingModel    = "nexus-pro-embedding"
	defaultLiteLLMEmbeddingProbeTTL = 30 * time.Second
)

// LiteLLMEmbeddingConfig defines the OpenAI-compatible embedding endpoint config.
type LiteLLMEmbeddingConfig struct {
	BaseURL  string
	APIKey   string
	Model    string
	Client   *http.Client
	ProbeTTL time.Duration
}

// LiteLLMEmbeddingClient calls the stable embedding alias exposed by LiteLLM.
type LiteLLMEmbeddingClient struct {
	client       openai.Client
	model        string
	probeTTL     time.Duration
	probeMu      sync.Mutex
	lastProbeAt  time.Time
	lastProbeErr error
}

// NewLiteLLMEmbeddingClient creates a validated embedding client.
func NewLiteLLMEmbeddingClient(cfg LiteLLMEmbeddingConfig) (*LiteLLMEmbeddingClient, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("litellm base url is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("litellm api key is required")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultLiteLLMEmbeddingModel
	}
	opts := []option.RequestOption{
		option.WithAPIKey(strings.TrimSpace(cfg.APIKey)),
		option.WithBaseURL(strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")),
	}
	if cfg.Client != nil {
		opts = append(opts, option.WithHTTPClient(cfg.Client))
	}
	probeTTL := cfg.ProbeTTL
	if probeTTL == 0 {
		probeTTL = defaultLiteLLMEmbeddingProbeTTL
	}
	return &LiteLLMEmbeddingClient{client: openai.NewClient(opts...), model: model, probeTTL: probeTTL}, nil
}

// Model returns the stable LiteLLM alias used to generate and query vectors.
func (c *LiteLLMEmbeddingClient) Model() string {
	if c == nil {
		return ""
	}
	return c.model
}

// Ping verifies the configured embedding route and caches the result to bound provider traffic.
func (c *LiteLLMEmbeddingClient) Ping(ctx context.Context) error {
	if c == nil {
		return errors.New("litellm embedding client is not configured")
	}
	c.probeMu.Lock()
	defer c.probeMu.Unlock()
	now := time.Now()
	if !c.lastProbeAt.IsZero() && c.probeTTL > 0 && now.Sub(c.lastProbeAt) < c.probeTTL {
		return c.lastProbeErr
	}
	_, err := c.Embed(ctx, []string{"nexus readiness probe"})
	if err != nil {
		err = fmt.Errorf("litellm embedding probe failed: %w", err)
	}
	c.lastProbeAt = now
	c.lastProbeErr = err
	return err
}

// Embed returns finite, consistently sized vectors in the same order as inputs.
func (c *LiteLLMEmbeddingClient) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if c == nil {
		return nil, errors.New("litellm embedding client is not configured")
	}
	if len(inputs) == 0 {
		return nil, errors.New("embedding inputs are required")
	}
	for _, input := range inputs {
		if strings.TrimSpace(input) == "" {
			return nil, errors.New("embedding input cannot be empty")
		}
	}
	response, err := c.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input:          openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: inputs},
		Model:          openai.EmbeddingModel(c.model),
		EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat,
	})
	if err != nil {
		return nil, err
	}
	if len(response.Data) != len(inputs) {
		return nil, fmt.Errorf("litellm returned %d embeddings for %d inputs", len(response.Data), len(inputs))
	}
	vectors := make([][]float32, len(inputs))
	dimension := 0
	for _, item := range response.Data {
		if item.Index < 0 || item.Index >= int64(len(vectors)) || vectors[item.Index] != nil {
			return nil, fmt.Errorf("litellm returned invalid embedding index %d", item.Index)
		}
		if len(item.Embedding) == 0 {
			return nil, errors.New("litellm returned an empty embedding")
		}
		if dimension == 0 {
			dimension = len(item.Embedding)
		} else if len(item.Embedding) != dimension {
			return nil, errors.New("litellm returned inconsistent embedding dimensions")
		}
		vector := make([]float32, len(item.Embedding))
		for i, value := range item.Embedding {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, errors.New("litellm returned a non-finite embedding value")
			}
			vector[i] = float32(value)
		}
		vectors[item.Index] = vector
	}
	return vectors, nil
}
