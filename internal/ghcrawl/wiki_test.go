package ghcrawl

import (
	"strings"
	"testing"
)

func TestIsWikiPage(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Home.md", true},
		{"Guide.markdown", true},
		{"page.textile", true},
		{"notes.rst", true},
		{"doc.asciidoc", true},
		{"page.org", true},
		{"image.png", false},
		{"data.json", false},
		{"script.go", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWikiPage(tt.name); got != tt.want {
				t.Errorf("isWikiPage(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestWikiCloneURL(t *testing.T) {
	t.Run("public clone URL", func(t *testing.T) {
		got := wikiCloneURL("octocat", "hello")
		want := "https://github.com/octocat/hello.wiki.git"
		if got != want {
			t.Fatalf("wikiCloneURL() = %q, want %q", got, want)
		}
	})
}

func TestGitHubCloneEnv(t *testing.T) {
	t.Run("empty token leaves env unchanged", func(t *testing.T) {
		env := gitHubCloneEnv("")
		if len(env) == 0 {
			t.Fatal("expected inherited environment")
		}
	})

	t.Run("token uses git config env instead of url userinfo", func(t *testing.T) {
		env := gitHubCloneEnv("ghp_tok")
		var (
			hasConfigCount bool
			hasConfigKey   bool
			hasConfigValue bool
		)
		for _, kv := range env {
			switch kv {
			case "GIT_CONFIG_COUNT=1":
				hasConfigCount = true
			case "GIT_CONFIG_KEY_0=http.https://github.com/.extraheader":
				hasConfigKey = true
			}
			if strings.HasPrefix(kv, "GIT_CONFIG_VALUE_0=") {
				hasConfigValue = true
				if kv == "GIT_CONFIG_VALUE_0=AUTHORIZATION: basic ghp_tok" {
					t.Fatalf("expected encoded auth header, got raw token env %q", kv)
				}
			}
		}
		if !hasConfigCount || !hasConfigKey || !hasConfigValue {
			t.Fatalf("missing git auth env entries: %v", env)
		}
	})
}
