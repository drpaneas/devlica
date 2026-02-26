package config

import (
	"testing"

	"github.com/drpaneas/devlica/internal/llm"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid openai config",
			cfg: Config{
				Username:    "testuser",
				GitHubToken: "ghp_fake",
				Provider:    llm.ProviderOpenAI,
				APIKey:      "sk-fake",
				MaxRepos:    10,
			},
		},
		{
			name: "valid anthropic config",
			cfg: Config{
				Username:    "testuser",
				GitHubToken: "ghp_fake",
				Provider:    llm.ProviderAnthropic,
				APIKey:      "sk-ant-fake",
				MaxRepos:    5,
			},
		},
		{
			name: "valid ollama config without api key",
			cfg: Config{
				Username:    "testuser",
				GitHubToken: "ghp_fake",
				Provider:    llm.ProviderOllama,
				MaxRepos:    3,
			},
		},
		{
			name: "missing username",
			cfg: Config{
				GitHubToken: "ghp_fake",
				Provider:    llm.ProviderOpenAI,
				APIKey:      "sk-fake",
				MaxRepos:    10,
			},
			wantErr: true,
		},
		{
			name: "missing github token",
			cfg: Config{
				Username: "testuser",
				Provider: llm.ProviderOpenAI,
				APIKey:   "sk-fake",
				MaxRepos: 10,
			},
			wantErr: true,
		},
		{
			name: "invalid provider",
			cfg: Config{
				Username:    "testuser",
				GitHubToken: "ghp_fake",
				Provider:    "gemini",
				MaxRepos:    10,
			},
			wantErr: true,
		},
		{
			name: "openai missing api key",
			cfg: Config{
				Username:    "testuser",
				GitHubToken: "ghp_fake",
				Provider:    llm.ProviderOpenAI,
				MaxRepos:    10,
			},
			wantErr: true,
		},
		{
			name: "max repos zero",
			cfg: Config{
				Username:    "testuser",
				GitHubToken: "ghp_fake",
				Provider:    llm.ProviderOpenAI,
				APIKey:      "sk-fake",
				MaxRepos:    0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultModel(t *testing.T) {
	tests := []struct {
		provider llm.ProviderName
		want     string
	}{
		{llm.ProviderOpenAI, "gpt-4o"},
		{llm.ProviderAnthropic, "claude-sonnet-4-5"},
		{llm.ProviderOllama, "llama3"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			got := DefaultModel(tt.provider)
			if got != tt.want {
				t.Errorf("DefaultModel(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}
