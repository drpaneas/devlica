package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drpaneas/devlica/internal/analyzer"
)

func TestGenerate(t *testing.T) {
	dir := t.TempDir()
	gen := NewGenerator(dir)

	persona := &analyzer.Persona{
		Username:          "testdev",
		CodeStyle:         "Uses snake_case everywhere.",
		ReviewStyle:       "Focuses on performance.",
		Communication:     "Very direct.",
		DeveloperIdentity: "Go enthusiast, Kubernetes contributor.",
		Synthesis: &analyzer.SynthesisResult{
			CodingPhilosophy:      "Values performance over readability.",
			CodeStyleRules:        "- Use snake_case for variables\n- Keep functions under 20 lines",
			ReviewPriorities:      "1. Performance\n2. Correctness",
			ReviewVoice:           `Blunt and direct. "This is too slow."`,
			CommunicationPatterns: "Short sentences. No fluff.",
			TestingPhilosophy:     "Benchmark everything.",
			DistinctiveTraits:     "Performance-obsessed.",
			DeveloperInterests:    "Go, Kubernetes, performance tooling.",
			ProjectPatterns:       "CLI tools with MIT license, CI via GitHub Actions.",
			CollaborationStyle:    "Active upstream contributor, detailed bug reports.",
		},
	}

	paths, err := gen.Generate("testdev", persona)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	if len(paths) != 3 {
		t.Fatalf("expected 3 skill files, got %d", len(paths))
	}

	csPath := filepath.Join(dir, "testdev-coding-style", "SKILL.md")
	csContent, err := os.ReadFile(csPath)
	if err != nil {
		t.Fatalf("reading coding style skill: %v", err)
	}
	cs := string(csContent)

	if !strings.Contains(cs, "name: testdev-coding-style") {
		t.Error("coding style skill missing frontmatter name")
	}
	if !strings.Contains(cs, "description:") {
		t.Error("coding style skill missing frontmatter description")
	}
	if !strings.Contains(cs, "snake_case") {
		t.Error("coding style skill should contain 'snake_case'")
	}
	if !strings.Contains(cs, "Project Patterns") {
		t.Error("coding style skill should contain 'Project Patterns' section")
	}

	rvPath := filepath.Join(dir, "testdev-code-reviewer", "SKILL.md")
	rvContent, err := os.ReadFile(rvPath)
	if err != nil {
		t.Fatalf("reading code reviewer skill: %v", err)
	}
	rv := string(rvContent)

	if !strings.Contains(rv, "name: testdev-code-reviewer") {
		t.Error("code reviewer skill missing frontmatter name")
	}
	if !strings.Contains(rv, "Performance") {
		t.Error("code reviewer skill should contain 'Performance'")
	}
	if !strings.Contains(rv, "Collaboration Style") {
		t.Error("code reviewer skill should contain 'Collaboration Style' section")
	}

	dpPath := filepath.Join(dir, "testdev-developer-profile", "SKILL.md")
	dpContent, err := os.ReadFile(dpPath)
	if err != nil {
		t.Fatalf("reading developer profile skill: %v", err)
	}
	dp := string(dpContent)

	if !strings.Contains(dp, "name: testdev-developer-profile") {
		t.Error("developer profile skill missing frontmatter name")
	}
	if !strings.Contains(dp, "Go, Kubernetes") {
		t.Error("developer profile skill should contain 'Go, Kubernetes'")
	}
	if !strings.Contains(dp, "CLI tools") {
		t.Error("developer profile skill should contain 'CLI tools'")
	}
}

func TestGenerate_EmptyFields(t *testing.T) {
	dir := t.TempDir()
	gen := NewGenerator(dir)

	persona := &analyzer.Persona{
		Username:          "testdev",
		CodeStyle:         "Fallback code style.",
		ReviewStyle:       "Fallback review style.",
		Communication:     "Fallback communication.",
		DeveloperIdentity: "Fallback identity.",
		Synthesis: &analyzer.SynthesisResult{
			CodingPhilosophy: "Values clarity.",
		},
	}

	paths, err := gen.Generate("testdev", persona)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if len(paths) != 3 {
		t.Fatalf("expected 3 skill files, got %d", len(paths))
	}

	csContent, err := os.ReadFile(filepath.Join(dir, "testdev-coding-style", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	cs := string(csContent)
	if !strings.Contains(cs, "Fallback code style.") {
		t.Error("expected fallback code style when synthesis field is empty")
	}

	rvContent, err := os.ReadFile(filepath.Join(dir, "testdev-code-reviewer", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	rv := string(rvContent)
	if !strings.Contains(rv, "Fallback review style.") {
		t.Error("expected fallback review style when synthesis field is empty")
	}

	dpContent, err := os.ReadFile(filepath.Join(dir, "testdev-developer-profile", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	dp := string(dpContent)
	if !strings.Contains(dp, "Fallback identity.") {
		t.Error("expected fallback developer identity when synthesis field is empty")
	}
}
