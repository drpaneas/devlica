package ghcrawl

import (
	"strings"
	"testing"

	"github.com/google/go-github/v68/github"
)

func TestExtractPatch(t *testing.T) {
	t.Run("empty files", func(t *testing.T) {
		got := extractPatch(nil)
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("files with no patch", func(t *testing.T) {
		files := []*github.CommitFile{
			{Filename: github.Ptr("a.go"), Patch: github.Ptr("")},
		}
		got := extractPatch(files)
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("normal patch", func(t *testing.T) {
		files := []*github.CommitFile{
			{Filename: github.Ptr("main.go"), Patch: github.Ptr("+fmt.Println()")},
		}
		got := extractPatch(files)
		if !strings.Contains(got, "main.go") {
			t.Errorf("expected filename in patch, got %q", got)
		}
		if !strings.Contains(got, "+fmt.Println()") {
			t.Errorf("expected patch content, got %q", got)
		}
	})

	t.Run("large patch is truncated", func(t *testing.T) {
		bigPatch := strings.Repeat("x", maxPatchLen+100)
		files := []*github.CommitFile{
			{Filename: github.Ptr("big.go"), Patch: &bigPatch},
		}
		got := extractPatch(files)
		if !strings.Contains(got, "(truncated)") {
			t.Errorf("expected truncation marker, got length %d", len(got))
		}
	})
}

func TestIsInterestingFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"main.go", true},
		{"Main.Go", true},
		{"Dockerfile", true},
		{"Makefile", true},
		{"README.md", false},
		{"utils.go", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isInterestingFile(tt.name); got != tt.want {
				t.Errorf("isInterestingFile(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsSourceFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"main.go", true},
		{"lib.py", true},
		{"app.rs", true},
		{"index.ts", true},
		{"style.css", false},
		{"readme.md", false},
		{"noext", false},
		{"", false},
		{"MAIN.GO", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSourceFile(tt.name); got != tt.want {
				t.Errorf("isSourceFile(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsWorkflowFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{".github/workflows/ci.yml", true},
		{".github/workflows/release.yaml", true},
		{".github/workflows/nested/test.yml", true},
		{".github/dependabot.yml", false},
		{"workflows/ci.yml", false},
		{".github/workflows/ci.json", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isWorkflowFile(tt.path); got != tt.want {
				t.Errorf("isWorkflowFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestEventSummary(t *testing.T) {
	ev := &github.Event{
		Type: github.Ptr("PushEvent"),
		Repo: &github.Repository{Name: github.Ptr("octocat/hello")},
	}
	got := eventSummary(ev)
	if !strings.Contains(got, "pushed") {
		t.Errorf("eventSummary(PushEvent) = %q, want it to contain 'pushed'", got)
	}

	ev.Type = github.Ptr("UnknownEvent")
	got = eventSummary(ev)
	if got != "UnknownEvent" {
		t.Errorf("eventSummary(UnknownEvent) = %q, want 'UnknownEvent'", got)
	}
}

func TestSpreadIndices(t *testing.T) {
	tests := []struct {
		name  string
		total int
		count int
		want  []int
	}{
		{"empty", 0, 5, nil},
		{"fewer than count", 3, 10, []int{0, 1, 2}},
		{"exact", 5, 5, []int{0, 1, 2, 3, 4}},
		{"spread 10 into 3", 10, 3, []int{0, 4, 9}},
		{"spread 6 into 3", 6, 3, []int{0, 2, 5}},
		{"single", 10, 1, []int{0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := spreadIndices(tt.total, tt.count)
			if len(got) != len(tt.want) {
				t.Fatalf("spreadIndices(%d, %d) = %v (len %d), want %v (len %d)",
					tt.total, tt.count, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("spreadIndices(%d, %d)[%d] = %d, want %d",
						tt.total, tt.count, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSelectDiverseRepos(t *testing.T) {
	mkRepo := func(name, lang string, fork bool, owner string) *github.Repository {
		r := &github.Repository{
			Name:     github.Ptr(name),
			FullName: github.Ptr(owner + "/" + name),
			Language: github.Ptr(lang),
			Fork:     github.Ptr(fork),
			Owner:    &github.User{Login: github.Ptr(owner)},
		}
		return r
	}

	t.Run("fewer repos than budget", func(t *testing.T) {
		repos := []*github.Repository{mkRepo("a", "Go", false, "user")}
		got := selectDiverseRepos(repos, 10, "user")
		if len(got) != 1 {
			t.Errorf("expected 1 repo, got %d", len(got))
		}
	})

	t.Run("language diversity", func(t *testing.T) {
		repos := []*github.Repository{
			mkRepo("go1", "Go", false, "user"),
			mkRepo("go2", "Go", false, "user"),
			mkRepo("go3", "Go", false, "user"),
			mkRepo("py1", "Python", false, "user"),
			mkRepo("rs1", "Rust", false, "user"),
			mkRepo("ts1", "TypeScript", false, "user"),
		}
		got := selectDiverseRepos(repos, 4, "user")
		if len(got) != 4 {
			t.Fatalf("expected 4 repos, got %d", len(got))
		}
		langs := make(map[string]bool)
		for _, r := range got {
			langs[r.GetLanguage()] = true
		}
		// Should include at least 3 different languages (Go, Python, Rust, TypeScript - pick 4 repos)
		if len(langs) < 3 {
			t.Errorf("expected at least 3 languages, got %d: %v", len(langs), langs)
		}
	})

	t.Run("forks deprioritized", func(t *testing.T) {
		repos := []*github.Repository{
			mkRepo("owned", "Go", false, "user"),
			mkRepo("fork1", "Go", true, "user"),
			mkRepo("fork2", "Python", true, "user"),
		}
		got := selectDiverseRepos(repos, 2, "user")
		if len(got) != 2 {
			t.Fatalf("expected 2 repos, got %d", len(got))
		}
		hasOwned := false
		for _, r := range got {
			if r.GetName() == "owned" {
				hasOwned = true
			}
		}
		if !hasOwned {
			t.Error("expected owned repo to be selected over forks")
		}
	})
}

func TestOwnerRepoFromURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "standard api url",
			url:       "https://api.github.com/repos/octocat/hello-world",
			wantOwner: "octocat",
			wantRepo:  "hello-world",
		},
		{
			name:      "trailing slash",
			url:       "https://api.github.com/repos/octocat/hello-world/",
			wantOwner: "octocat",
			wantRepo:  "hello-world",
		},
		{
			name:    "too few path segments",
			url:     "https://api.github.com/repos",
			wantErr: true,
		},
		{
			name:    "invalid url",
			url:     "://bad",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := ownerRepoFromURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ownerRepoFromURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if owner != tt.wantOwner || repo != tt.wantRepo {
				t.Errorf("ownerRepoFromURL(%q) = (%q, %q), want (%q, %q)",
					tt.url, owner, repo, tt.wantOwner, tt.wantRepo)
			}
		})
	}
}
