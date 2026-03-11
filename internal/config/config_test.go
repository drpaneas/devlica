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
				Username:     "testuser",
				GitHubTokens: []string{"ghp_fake"},
				Provider:     llm.ProviderOpenAI,
				APIKey:       "sk-fake",
				MaxRepos:     10,
			},
		},
		{
			name: "valid anthropic config",
			cfg: Config{
				Username:     "testuser",
				GitHubTokens: []string{"ghp_fake"},
				Provider:     llm.ProviderAnthropic,
				APIKey:       "sk-ant-fake",
				MaxRepos:     5,
			},
		},
		{
			name: "valid anthropic vertex config without api key",
			cfg: Config{
				Username:        "testuser",
				GitHubTokens:    []string{"ghp_fake"},
				Provider:        llm.ProviderAnthropic,
				UseVertexAI:     true,
				VertexProjectID: "my-project",
				VertexRegion:    "global",
				MaxRepos:        5,
			},
		},
		{
			name: "valid ollama config without api key",
			cfg: Config{
				Username:     "testuser",
				GitHubTokens: []string{"ghp_fake"},
				Provider:     llm.ProviderOllama,
				MaxRepos:     3,
			},
		},
		{
			name: "single char username",
			cfg: Config{
				Username:     "a",
				GitHubTokens: []string{"ghp_fake"},
				Provider:     llm.ProviderOllama,
				MaxRepos:     1,
			},
		},
		{
			name: "hyphen in middle",
			cfg: Config{
				Username:     "a-b",
				GitHubTokens: []string{"ghp_fake"},
				Provider:     llm.ProviderOllama,
				MaxRepos:     1,
			},
		},
		{
			name: "leading hyphen",
			cfg: Config{
				Username:     "-bad",
				GitHubTokens: []string{"ghp_fake"},
				Provider:     llm.ProviderOllama,
				MaxRepos:     1,
			},
			wantErr: true,
		},
		{
			name: "trailing hyphen",
			cfg: Config{
				Username:     "bad-",
				GitHubTokens: []string{"ghp_fake"},
				Provider:     llm.ProviderOllama,
				MaxRepos:     1,
			},
			wantErr: true,
		},
		{
			name: "path traversal",
			cfg: Config{
				Username:     "../etc",
				GitHubTokens: []string{"ghp_fake"},
				Provider:     llm.ProviderOllama,
				MaxRepos:     1,
			},
			wantErr: true,
		},
		{
			name: "too long username",
			cfg: Config{
				Username:     "abcdefghijklmnopqrstuvwxyz01234567890abcd",
				GitHubTokens: []string{"ghp_fake"},
				Provider:     llm.ProviderOllama,
				MaxRepos:     1,
			},
			wantErr: true,
		},
		{
			name: "missing username",
			cfg: Config{
				GitHubTokens: []string{"ghp_fake"},
				Provider:     llm.ProviderOpenAI,
				APIKey:       "sk-fake",
				MaxRepos:     10,
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
				Username:     "testuser",
				GitHubTokens: []string{"ghp_fake"},
				Provider:     "gemini",
				MaxRepos:     10,
			},
			wantErr: true,
		},
		{
			name: "openai missing api key",
			cfg: Config{
				Username:     "testuser",
				GitHubTokens: []string{"ghp_fake"},
				Provider:     llm.ProviderOpenAI,
				MaxRepos:     10,
			},
			wantErr: true,
		},
		{
			name: "anthropic missing auth config",
			cfg: Config{
				Username:     "testuser",
				GitHubTokens: []string{"ghp_fake"},
				Provider:     llm.ProviderAnthropic,
				MaxRepos:     10,
			},
			wantErr: true,
		},
		{
			name: "anthropic vertex enabled missing project",
			cfg: Config{
				Username:     "testuser",
				GitHubTokens: []string{"ghp_fake"},
				Provider:     llm.ProviderAnthropic,
				UseVertexAI:  true,
				VertexRegion: "global",
				MaxRepos:     10,
			},
			wantErr: true,
		},
		{
			name: "anthropic vertex enabled missing region",
			cfg: Config{
				Username:        "testuser",
				GitHubTokens:    []string{"ghp_fake"},
				Provider:        llm.ProviderAnthropic,
				UseVertexAI:     true,
				VertexProjectID: "my-project",
				MaxRepos:        10,
			},
			wantErr: true,
		},
		{
			name: "max repos zero",
			cfg: Config{
				Username:     "testuser",
				GitHubTokens: []string{"ghp_fake"},
				Provider:     llm.ProviderOpenAI,
				APIKey:       "sk-fake",
				MaxRepos:     0,
			},
			wantErr: true,
		},
		{
			name: "max repos zero allowed in exhaustive mode",
			cfg: Config{
				Username:     "testuser",
				GitHubTokens: []string{"ghp_fake"},
				Provider:     llm.ProviderOpenAI,
				APIKey:       "sk-fake",
				MaxRepos:     0,
				Exhaustive:   true,
			},
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

func TestLoadGitHubTokens(t *testing.T) {
	t.Run("primary only", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "tok-primary")
		t.Setenv("GITHUB_TOKEN_1", "")
		tokens := loadGitHubTokens()
		if len(tokens) != 1 || tokens[0] != "tok-primary" {
			t.Errorf("expected [tok-primary], got %v", tokens)
		}
	})

	t.Run("primary plus numbered", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "tok-0")
		t.Setenv("GITHUB_TOKEN_1", "tok-1")
		t.Setenv("GITHUB_TOKEN_2", "tok-2")
		t.Setenv("GITHUB_TOKEN_3", "")
		tokens := loadGitHubTokens()
		if len(tokens) != 3 {
			t.Fatalf("expected 3 tokens, got %d: %v", len(tokens), tokens)
		}
		if tokens[0] != "tok-0" || tokens[1] != "tok-1" || tokens[2] != "tok-2" {
			t.Errorf("unexpected tokens: %v", tokens)
		}
	})

	t.Run("numbered only without primary", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("GITHUB_TOKEN_1", "tok-1")
		t.Setenv("GITHUB_TOKEN_2", "tok-2")
		t.Setenv("GITHUB_TOKEN_3", "")
		tokens := loadGitHubTokens()
		if len(tokens) != 2 {
			t.Fatalf("expected 2 tokens, got %d: %v", len(tokens), tokens)
		}
	})

	t.Run("none set", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("GITHUB_TOKEN_1", "")
		tokens := loadGitHubTokens()
		if len(tokens) != 0 {
			t.Errorf("expected empty, got %v", tokens)
		}
	})
}

func TestDefaultModel(t *testing.T) {
	tests := []struct {
		provider llm.ProviderName
		want     string
	}{
		{llm.ProviderOpenAI, "gpt-4o"},
		{llm.ProviderAnthropic, "claude-opus-4-6"},
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

func TestLoadFromEnv_AnthropicVertex(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "tok-primary")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "1")
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "vertex-project")
	t.Setenv("CLOUD_ML_REGION", "global")

	cfg := Config{Provider: llm.ProviderAnthropic}
	cfg.LoadFromEnv()

	if !cfg.UseVertexAI {
		t.Fatalf("expected UseVertexAI to be true")
	}
	if cfg.VertexProjectID != "vertex-project" {
		t.Fatalf("expected VertexProjectID to be set, got %q", cfg.VertexProjectID)
	}
	if cfg.VertexRegion != "global" {
		t.Fatalf("expected VertexRegion to be set, got %q", cfg.VertexRegion)
	}
}

func TestLoadFromEnv_AnthropicVertexRequiresFlag(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "tok-primary")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "0")
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "vertex-project")
	t.Setenv("CLOUD_ML_REGION", "us-east5")

	cfg := Config{Provider: llm.ProviderAnthropic}
	cfg.LoadFromEnv()

	if cfg.UseVertexAI {
		t.Fatalf("expected UseVertexAI to be false when CLAUDE_CODE_USE_VERTEX is not enabled")
	}
}
