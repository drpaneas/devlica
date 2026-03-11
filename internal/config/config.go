package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/drpaneas/devlica/internal/llm"
)

var validUsername = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?$`)

// Config holds all runtime configuration for devlica.
type Config struct {
	Username        string
	GitHubTokens    []string
	PrivateToken    string
	Provider        llm.ProviderName
	Model           string
	OllamaHost      string
	APIKey          string
	UseVertexAI     bool
	VertexRegion    string
	VertexProjectID string
	OutputDir       string
	MaxRepos        int
	Exhaustive      bool
	Verbose         bool
}

// Validate checks that all required fields are set and consistent.
func (c *Config) Validate() error {
	if c.Username == "" {
		return fmt.Errorf("github username is required")
	}
	if !validUsername.MatchString(c.Username) {
		return fmt.Errorf("invalid github username %q", c.Username)
	}
	if len(c.GitHubTokens) == 0 {
		return fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}
	switch c.Provider {
	case llm.ProviderOpenAI, llm.ProviderAnthropic, llm.ProviderOllama:
	default:
		return fmt.Errorf("unsupported LLM provider %q: must be openai, anthropic, or ollama", c.Provider)
	}
	if c.Provider == llm.ProviderOpenAI && c.APIKey == "" {
		return fmt.Errorf("%s requires an API key (set %s)", c.Provider, envKeyForProvider(c.Provider))
	}
	if c.Provider == llm.ProviderAnthropic {
		if c.UseVertexAI {
			if c.VertexProjectID == "" {
				return fmt.Errorf("anthropic Vertex AI mode requires ANTHROPIC_VERTEX_PROJECT_ID")
			}
			if c.VertexRegion == "" {
				return fmt.Errorf("anthropic Vertex AI mode requires CLOUD_ML_REGION")
			}
		} else if c.APIKey == "" {
			return fmt.Errorf("anthropic requires ANTHROPIC_API_KEY or Vertex AI settings (CLAUDE_CODE_USE_VERTEX=1, ANTHROPIC_VERTEX_PROJECT_ID, CLOUD_ML_REGION)")
		}
	}
	if !c.Exhaustive && c.MaxRepos < 1 {
		return fmt.Errorf("--max-repos must be at least 1")
	}
	if c.Exhaustive && c.MaxRepos < 0 {
		return fmt.Errorf("--max-repos must be at least 0 when --exhaustive is enabled")
	}
	return nil
}

// LoadFromEnv populates environment-dependent fields (tokens, keys, hosts).
func (c *Config) LoadFromEnv() {
	c.GitHubTokens = loadGitHubTokens()
	c.PrivateToken = os.Getenv("GITHUB_PRIVATE_TOKEN")
	c.OllamaHost = os.Getenv("OLLAMA_HOST")
	if c.OllamaHost == "" {
		c.OllamaHost = "http://localhost:11434"
	}
	switch c.Provider {
	case llm.ProviderOpenAI:
		c.APIKey = os.Getenv("OPENAI_API_KEY")
	case llm.ProviderAnthropic:
		c.APIKey = os.Getenv("ANTHROPIC_API_KEY")
		c.VertexProjectID = firstNonEmpty(
			os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID"),
			os.Getenv("GCLOUD_PROJECT"),
			os.Getenv("GOOGLE_CLOUD_PROJECT"),
		)
		c.VertexRegion = os.Getenv("CLOUD_ML_REGION")
		c.UseVertexAI = parseBoolEnv("CLAUDE_CODE_USE_VERTEX")
	}
}

// loadGitHubTokens reads GITHUB_TOKEN as the primary token, then scans
// GITHUB_TOKEN_1, GITHUB_TOKEN_2, ... for additional tokens.
func loadGitHubTokens() []string {
	var tokens []string
	if primary := os.Getenv("GITHUB_TOKEN"); primary != "" {
		tokens = append(tokens, primary)
	}
	for i := 1; ; i++ {
		tok := os.Getenv("GITHUB_TOKEN_" + strconv.Itoa(i))
		if tok == "" {
			break
		}
		tokens = append(tokens, tok)
	}
	return tokens
}

// DefaultModel returns the default model name for the given provider.
func DefaultModel(provider llm.ProviderName) string {
	switch provider {
	case llm.ProviderOpenAI:
		return "gpt-4o"
	case llm.ProviderAnthropic:
		return "claude-opus-4-6"
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

func parseBoolEnv(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
