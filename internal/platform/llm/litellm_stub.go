//go:build !adk

package llm

import (
	"errors"
	"strings"
)

// LiteLLMConfig defines the OpenAI-compatible LiteLLM model adapter config.
type LiteLLMConfig struct {
	BaseURL string
	APIKey  string
}

// LiteLLM is a placeholder when the ADK/OpenAI dependency build is unavailable.
type LiteLLM struct {
	BaseURL string
	APIKey  string
}

// NewLiteLLM validates config and returns a placeholder model for non-ADK builds.
func NewLiteLLM(cfg LiteLLMConfig) (*LiteLLM, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("litellm base url is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("litellm api key is required")
	}
	return &LiteLLM{BaseURL: cfg.BaseURL, APIKey: cfg.APIKey}, nil
}
