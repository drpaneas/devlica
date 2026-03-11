package benchmark

import (
	"strings"
	"testing"
)

func TestParseDryRunReview(t *testing.T) {
	input := `{"decision":"request_changes","concerns":["nil handling","tests"],"comment":"Please guard against nil input first."}`

	got, err := parseDryRunReview(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Decision != "request_changes" {
		t.Fatalf("Decision = %q, want %q", got.Decision, "request_changes")
	}
	if len(got.Concerns) != 2 {
		t.Fatalf("expected 2 concerns, got %d", len(got.Concerns))
	}
	if got.Comment != "Please guard against nil input first." {
		t.Fatalf("Comment = %q, want parsed comment", got.Comment)
	}
}

func TestFormatGeneratedReview(t *testing.T) {
	got := formatGeneratedReview(&dryRunReview{
		Decision: "comment",
		Concerns: []string{"naming", "readability"},
		Comment:  "How about renaming this helper?",
	})

	if !strings.Contains(got, "Decision: comment") {
		t.Fatalf("expected decision in formatted output, got %q", got)
	}
	if !strings.Contains(got, "- naming") {
		t.Fatalf("expected concerns list in formatted output, got %q", got)
	}
	if !strings.Contains(got, "How about renaming this helper?") {
		t.Fatalf("expected comment in formatted output, got %q", got)
	}
}
