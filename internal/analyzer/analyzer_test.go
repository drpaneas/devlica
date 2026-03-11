package analyzer

import (
	"strings"
	"testing"

	"github.com/drpaneas/devlica/internal/ghcrawl"
)

func TestInterleave(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got := interleave(nil)
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("single bucket", func(t *testing.T) {
		got := interleave([][]string{{"a", "b", "c"}})
		if got != "abc" {
			t.Errorf("expected 'abc', got %q", got)
		}
	})

	t.Run("round robin across buckets", func(t *testing.T) {
		buckets := [][]string{
			{"A1-", "A2-", "A3-"},
			{"B1-", "B2-"},
			{"C1-"},
		}
		got := interleave(buckets)
		// Round 0: A1 B1 C1, Round 1: A2 B2, Round 2: A3
		want := "A1-B1-C1-A2-B2-A3-"
		if got != want {
			t.Errorf("interleave = %q, want %q", got, want)
		}
	})

	t.Run("ensures fair representation under truncation", func(t *testing.T) {
		// Repo A has 100 items, Repo B has 2 items.
		// With sequential iteration, B would be at the end.
		// With interleave, B items appear early.
		var bigBucket []string
		for i := 0; i < 100; i++ {
			bigBucket = append(bigBucket, "A-")
		}
		smallBucket := []string{"B1-", "B2-"}
		got := interleave([][]string{bigBucket, smallBucket})
		// B1 should appear at position 1 (after A[0]), not at position 100
		idx := strings.Index(got, "B1-")
		if idx < 0 || idx > 10 {
			t.Errorf("B1 should appear within first 10 chars but found at %d", idx)
		}
	})
}

func TestParseSynthesis(t *testing.T) {
	t.Run("raw json with all fields", func(t *testing.T) {
		input := `{"coding_philosophy":"simplicity","code_style_rules":"use gofmt","review_priorities":"correctness","review_decision_style":"block on correctness","review_non_blocking_nits":"wording","review_context_sensitivity":"stricter on risky changes","review_voice":"direct","communication_patterns":"bullet points","testing_philosophy":"table-driven","distinctive_traits":"concise","developer_interests":"Go, Kubernetes","activity_patterns":"steady upstream work","project_patterns":"CLI tools","collaboration_style":"upstream contributor"}`
		result, err := ParseSynthesis(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.CodingPhilosophy != "simplicity" {
			t.Errorf("CodingPhilosophy = %q, want %q", result.CodingPhilosophy, "simplicity")
		}
		if result.DistinctiveTraits != "concise" {
			t.Errorf("DistinctiveTraits = %q, want %q", result.DistinctiveTraits, "concise")
		}
		if result.DeveloperInterests != "Go, Kubernetes" {
			t.Errorf("DeveloperInterests = %q, want %q", result.DeveloperInterests, "Go, Kubernetes")
		}
		if result.ProjectPatterns != "CLI tools" {
			t.Errorf("ProjectPatterns = %q, want %q", result.ProjectPatterns, "CLI tools")
		}
		if result.ReviewDecisionStyle != "block on correctness" {
			t.Errorf("ReviewDecisionStyle = %q, want %q", result.ReviewDecisionStyle, "block on correctness")
		}
		if result.ActivityPatterns != "steady upstream work" {
			t.Errorf("ActivityPatterns = %q, want %q", result.ActivityPatterns, "steady upstream work")
		}
		if result.CollaborationStyle != "upstream contributor" {
			t.Errorf("CollaborationStyle = %q, want %q", result.CollaborationStyle, "upstream contributor")
		}
	})

	t.Run("json in code fence", func(t *testing.T) {
		input := "```json\n{\"coding_philosophy\":\"clarity\",\"code_style_rules\":\"\",\"review_priorities\":\"\",\"review_decision_style\":\"\",\"review_non_blocking_nits\":\"\",\"review_context_sensitivity\":\"\",\"review_voice\":\"\",\"communication_patterns\":\"\",\"testing_philosophy\":\"\",\"distinctive_traits\":\"\",\"developer_interests\":\"\",\"activity_patterns\":\"\",\"project_patterns\":\"\",\"collaboration_style\":\"\"}\n```"
		result, err := ParseSynthesis(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.CodingPhilosophy != "clarity" {
			t.Errorf("CodingPhilosophy = %q, want %q", result.CodingPhilosophy, "clarity")
		}
	})

	t.Run("code fence without json tag", func(t *testing.T) {
		input := "```\n{\"coding_philosophy\":\"ok\",\"code_style_rules\":\"\",\"review_priorities\":\"\",\"review_decision_style\":\"\",\"review_non_blocking_nits\":\"\",\"review_context_sensitivity\":\"\",\"review_voice\":\"\",\"communication_patterns\":\"\",\"testing_philosophy\":\"\",\"distinctive_traits\":\"\",\"developer_interests\":\"\",\"activity_patterns\":\"\",\"project_patterns\":\"\",\"collaboration_style\":\"\"}\n```"
		result, err := ParseSynthesis(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.CodingPhilosophy != "ok" {
			t.Errorf("CodingPhilosophy = %q, want %q", result.CodingPhilosophy, "ok")
		}
	})

	t.Run("missing new fields still parses", func(t *testing.T) {
		input := `{"coding_philosophy":"simplicity","code_style_rules":"use gofmt","review_priorities":"correctness","review_voice":"direct","communication_patterns":"bullet points","testing_philosophy":"table-driven","distinctive_traits":"concise"}`
		result, err := ParseSynthesis(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.CodingPhilosophy != "simplicity" {
			t.Errorf("CodingPhilosophy = %q, want %q", result.CodingPhilosophy, "simplicity")
		}
		if result.DeveloperInterests != "" {
			t.Errorf("DeveloperInterests = %q, want empty", result.DeveloperInterests)
		}
	})

	t.Run("array values normalized to strings", func(t *testing.T) {
		input := `{"coding_philosophy":["a","b"],"code_style_rules":"","review_priorities":"","review_decision_style":"","review_non_blocking_nits":"","review_context_sensitivity":"","review_voice":"","communication_patterns":"","testing_philosophy":"","distinctive_traits":"","developer_interests":["Go","Rust"],"activity_patterns":"","project_patterns":"","collaboration_style":""}`
		result, err := ParseSynthesis(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.CodingPhilosophy != "a\nb" {
			t.Errorf("CodingPhilosophy = %q, want %q", result.CodingPhilosophy, "a\nb")
		}
		if result.DeveloperInterests != "Go\nRust" {
			t.Errorf("DeveloperInterests = %q, want %q", result.DeveloperInterests, "Go\nRust")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := ParseSynthesis("this is not json")
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("empty response", func(t *testing.T) {
		_, err := ParseSynthesis("   \n\t")
		if err == nil {
			t.Error("expected error for empty response")
		}
	})
}

func TestBuildReviewDataText(t *testing.T) {
	data := &ghcrawl.CrawlResult{
		Repos: []ghcrawl.RepoData{
			{
				FullName: "acme/project",
				Reviews: []ghcrawl.ReviewData{
					{
						Repo:               "acme/project",
						PRNumber:           42,
						PRTitle:            "Fix parser edge case",
						PRAuthor:           "alice",
						State:              "CHANGES_REQUESTED",
						Body:               "Please handle nil input before parsing.",
						Additions:          10,
						Deletions:          2,
						ChangedFiles:       1,
						ReviewCommentCount: 1,
					},
				},
				ReviewComments: []ghcrawl.ReviewComment{
					{
						Repo:     "acme/project",
						PRNumber: 42,
						PRTitle:  "Fix parser edge case",
						PRAuthor: "alice",
						Path:     "parser.go",
						DiffHunk: "@@ -10,2 +10,4 @@",
						Body:     "This branch still panics on empty slices.",
					},
				},
			},
		},
	}

	got := buildReviewDataText(data)
	if !strings.Contains(got, "State: CHANGES_REQUESTED") {
		t.Fatalf("expected review state in output, got %q", got)
	}
	if !strings.Contains(got, "Diff hunk:") {
		t.Fatalf("expected diff hunk label in output, got %q", got)
	}
	if !strings.Contains(got, "Please handle nil input before parsing.") {
		t.Fatalf("expected review summary body in output, got %q", got)
	}
}

func TestBuildDiscussionsText(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		data := &ghcrawl.CrawlResult{}
		got := buildDiscussionsText(data)
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("with discussions", func(t *testing.T) {
		data := &ghcrawl.CrawlResult{
			Discussions: []ghcrawl.DiscussionData{
				{
					Repo:     "acme/project",
					Number:   1,
					Title:    "Design RFC",
					Body:     "Proposal for new API",
					Category: "Ideas",
					Author:   "alice",
					Comments: []ghcrawl.Comment{
						{Author: "alice", Body: "I like this approach"},
					},
				},
			},
		}
		got := buildDiscussionsText(data)
		if !strings.Contains(got, "Design RFC") {
			t.Errorf("expected discussion title, got %q", got)
		}
		if !strings.Contains(got, "Ideas") {
			t.Errorf("expected category, got %q", got)
		}
		if !strings.Contains(got, "I like this approach") {
			t.Errorf("expected comment body, got %q", got)
		}
		if !strings.Contains(got, "Thread author: alice") {
			t.Errorf("expected thread author label, got %q", got)
		}
		if !strings.Contains(got, "Comment by alice") {
			t.Errorf("expected comment author label, got %q", got)
		}
	})
}

func TestBuildProjectsText(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		data := &ghcrawl.CrawlResult{}
		got := buildProjectsText(data)
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("with projects", func(t *testing.T) {
		data := &ghcrawl.CrawlResult{
			Projects: []ghcrawl.ProjectData{
				{Title: "Roadmap", Body: "Q1 plan", Public: true, ItemCount: 15},
			},
		}
		got := buildProjectsText(data)
		if !strings.Contains(got, "Roadmap") {
			t.Errorf("expected project title, got %q", got)
		}
		if !strings.Contains(got, "15 items") {
			t.Errorf("expected item count, got %q", got)
		}
	})
}

func TestBuildWikiPagesText(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		data := &ghcrawl.CrawlResult{}
		got := buildWikiPagesText(data)
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("with wiki pages", func(t *testing.T) {
		data := &ghcrawl.CrawlResult{
			Repos: []ghcrawl.RepoData{
				{
					FullName: "acme/project",
					WikiPages: []ghcrawl.WikiPage{
						{Repo: "acme/project", Title: "Home", Content: "# Welcome"},
					},
				},
			},
		}
		got := buildWikiPagesText(data)
		if !strings.Contains(got, "Home") {
			t.Errorf("expected wiki title, got %q", got)
		}
		if !strings.Contains(got, "# Welcome") {
			t.Errorf("expected wiki content, got %q", got)
		}
	})
}

func TestBuildGistsTextIncludesContentSnippet(t *testing.T) {
	data := &ghcrawl.CrawlResult{
		Gists: []ghcrawl.GistData{
			{
				Description: "helpers",
				Files: []ghcrawl.GistFile{
					{Name: "script.sh", Language: "Shell", Content: "echo hello"},
				},
			},
		},
	}

	got := buildGistsText(data)
	if !strings.Contains(got, "snippet script.sh") {
		t.Fatalf("expected gist snippet label in output, got %q", got)
	}
	if !strings.Contains(got, "echo hello") {
		t.Fatalf("expected gist content in output, got %q", got)
	}
}
