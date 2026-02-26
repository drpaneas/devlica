package llm

import (
	"context"
	"fmt"
)

// ProviderName identifies a supported LLM provider.
type ProviderName string

const (
	ProviderOpenAI    ProviderName = "openai"
	ProviderAnthropic ProviderName = "anthropic"
	ProviderOllama    ProviderName = "ollama"
)

// CompleteOptions controls per-request LLM parameters.
// A nil value uses provider-specific defaults.
type CompleteOptions struct {
	Temperature *float32
	MaxTokens   int
}

// ProviderConfig holds the configuration needed to construct a Provider.
type ProviderConfig struct {
	Name       ProviderName
	APIKey     string
	Model      string
	OllamaHost string
}

// Provider abstracts an LLM completion backend.
type Provider interface {
	Complete(ctx context.Context, system, prompt string, opts *CompleteOptions) (string, error)
}

// NewProvider creates a Provider for the given configuration.
func NewProvider(cfg ProviderConfig) (Provider, error) {
	switch cfg.Name {
	case ProviderOpenAI:
		return newOpenAI(cfg.APIKey, cfg.Model), nil
	case ProviderAnthropic:
		return newAnthropic(cfg.APIKey, cfg.Model), nil
	case ProviderOllama:
		return newOllama(cfg.OllamaHost, cfg.Model), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider: %s", cfg.Name)
	}
}
