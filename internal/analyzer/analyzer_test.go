package analyzer

import (
	"strings"
	"testing"
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
		input := `{"coding_philosophy":"simplicity","code_style_rules":"use gofmt","review_priorities":"correctness","review_voice":"direct","communication_patterns":"bullet points","testing_philosophy":"table-driven","distinctive_traits":"concise","developer_interests":"Go, Kubernetes","project_patterns":"CLI tools","collaboration_style":"upstream contributor"}`
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
		if result.CollaborationStyle != "upstream contributor" {
			t.Errorf("CollaborationStyle = %q, want %q", result.CollaborationStyle, "upstream contributor")
		}
	})

	t.Run("json in code fence", func(t *testing.T) {
		input := "```json\n{\"coding_philosophy\":\"clarity\",\"code_style_rules\":\"\",\"review_priorities\":\"\",\"review_voice\":\"\",\"communication_patterns\":\"\",\"testing_philosophy\":\"\",\"distinctive_traits\":\"\",\"developer_interests\":\"\",\"project_patterns\":\"\",\"collaboration_style\":\"\"}\n```"
		result, err := ParseSynthesis(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.CodingPhilosophy != "clarity" {
			t.Errorf("CodingPhilosophy = %q, want %q", result.CodingPhilosophy, "clarity")
		}
	})

	t.Run("code fence without json tag", func(t *testing.T) {
		input := "```\n{\"coding_philosophy\":\"ok\",\"code_style_rules\":\"\",\"review_priorities\":\"\",\"review_voice\":\"\",\"communication_patterns\":\"\",\"testing_philosophy\":\"\",\"distinctive_traits\":\"\",\"developer_interests\":\"\",\"project_patterns\":\"\",\"collaboration_style\":\"\"}\n```"
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
		input := `{"coding_philosophy":["a","b"],"code_style_rules":"","review_priorities":"","review_voice":"","communication_patterns":"","testing_philosophy":"","distinctive_traits":"","developer_interests":["Go","Rust"],"project_patterns":"","collaboration_style":""}`
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
}
