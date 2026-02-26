package config

import (
	"fmt"
	"os"
	"regexp"

	"github.com/drpaneas/devlica/internal/llm"
)

var validUsername = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?$`)

// Config holds all runtime configuration for devlica.
type Config struct {
	Username    string
	GitHubToken string
	Provider    llm.ProviderName
	Model       string
	OllamaHost  string
	APIKey      string
	OutputDir   string
	MaxRepos    int
	Verbose     bool
}

// Validate checks that all required fields are set and consistent.
func (c *Config) Validate() error {
	if c.Username == "" {
		return fmt.Errorf("github username is required")
	}
	if !validUsername.MatchString(c.Username) {
		return fmt.Errorf("invalid github username %q", c.Username)
	}
	if c.GitHubToken == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}
	switch c.Provider {
	case llm.ProviderOpenAI, llm.ProviderAnthropic, llm.ProviderOllama:
	default:
		return fmt.Errorf("unsupported LLM provider %q: must be openai, anthropic, or ollama", c.Provider)
	}
	if c.APIKey == "" && c.Provider != llm.ProviderOllama {
		return fmt.Errorf("%s requires an API key (set %s)", c.Provider, envKeyForProvider(c.Provider))
	}
	if c.MaxRepos < 1 {
		return fmt.Errorf("--max-repos must be at least 1")
	}
	return nil
}

// LoadFromEnv populates environment-dependent fields (tokens, keys, hosts).
func (c *Config) LoadFromEnv() {
	c.GitHubToken = os.Getenv("GITHUB_TOKEN")
	c.OllamaHost = os.Getenv("OLLAMA_HOST")
	if c.OllamaHost == "" {
		c.OllamaHost = "http://localhost:11434"
	}
	switch c.Provider {
	case llm.ProviderOpenAI:
		c.APIKey = os.Getenv("OPENAI_API_KEY")
	case llm.ProviderAnthropic:
		c.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	}
}

// DefaultModel returns the default model name for the given provider.
func DefaultModel(provider llm.ProviderName) string {
	switch provider {
	case llm.ProviderOpenAI:
		return "gpt-4o"
	case llm.ProviderAnthropic:
		return "claude-sonnet-4-5"
	case llm.ProviderOllama:
		return "llama3"
	default:
		return ""
	}
}

func envKeyForProvider(provider llm.ProviderName) string {
	switch provider {
	case llm.ProviderOpenAI:
		return "OPENAI_API_KEY"
	case llm.ProviderAnthropic:
		return "ANTHROPIC_API_KEY"
	default:
		return ""
	}
}
