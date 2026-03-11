package skill

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"text/template"

	"github.com/drpaneas/devlica/internal/analyzer"
)

// Generator writes skill files from analyzed persona data.
type Generator struct {
	outputDir string
}

// NewGenerator returns a Generator that writes to outputDir.
func NewGenerator(outputDir string) *Generator {
	return &Generator{outputDir: outputDir}
}

type codingStyleData struct {
	Username        string
	Philosophy      string
	CodeStyle       string
	Testing         string
	ProjectPatterns string
	CodeExamples    string
	Traits          string
}

type reviewerData struct {
	Username           string
	ReviewPriorities   string
	ReviewDecision     string
	ReviewNits         string
	ReviewContext      string
	ReviewVoice        string
	CollaborationStyle string
}

type developerProfileData struct {
	Username           string
	DeveloperInterests string
	ActivityPatterns   string
	CollaborationStyle string
	Traits             string
}

// Generate produces skill files from the analyzed persona and returns their paths.
func (g *Generator) Generate(username string, persona *analyzer.Persona) ([]string, error) {
	var paths []string
	s := persona.Synthesis

	csData := codingStyleData{
		Username:        username,
		Philosophy:      s.CodingPhilosophy,
		CodeStyle:       s.CodeStyleRules,
		Testing:         s.TestingPhilosophy,
		ProjectPatterns: s.ProjectPatterns,
		Traits:          s.DistinctiveTraits,
	}
	if csData.CodeStyle == "" {
		csData.CodeStyle = persona.CodeStyle
	}
	if csData.Philosophy == "" {
		csData.Philosophy = "See code style rules below."
	}
	if csData.Testing == "" {
		csData.Testing = "No specific testing data was identified."
	}
	if csData.ProjectPatterns == "" {
		csData.ProjectPatterns = "No specific project pattern data was identified."
	}
	csData.CodeExamples = s.CodeExamples
	if csData.CodeExamples == "" {
		csData.CodeExamples = "No representative code examples were identified."
	}
	if csData.Traits == "" {
		csData.Traits = "See code style rules above."
	}

	csPath, err := g.writeSkill(username+"-coding-style", codingStyleTemplate, csData)
	if err != nil {
		return nil, fmt.Errorf("generating coding style skill: %w", err)
	}
	paths = append(paths, csPath)

	rvData := reviewerData{
		Username:           username,
		ReviewPriorities:   s.ReviewPriorities,
		ReviewDecision:     s.ReviewDecisionStyle,
		ReviewNits:         s.ReviewNonBlockingNits,
		ReviewContext:      s.ReviewContext,
		ReviewVoice:        s.ReviewVoice,
		CollaborationStyle: s.CollaborationStyle,
	}
	if rvData.ReviewPriorities == "" {
		rvData.ReviewPriorities = persona.ReviewStyle
	}
	if rvData.ReviewDecision == "" {
		rvData.ReviewDecision = "No specific approval-threshold data was identified."
	}
	if rvData.ReviewNits == "" {
		rvData.ReviewNits = "No specific non-blocking nit patterns were identified."
	}
	if rvData.ReviewContext == "" {
		rvData.ReviewContext = "No specific context-dependent review behavior was identified."
	}
	if rvData.ReviewVoice == "" {
		rvData.ReviewVoice = "No specific review voice data was identified."
	}
	if rvData.CollaborationStyle == "" {
		rvData.CollaborationStyle = "No specific collaboration data was identified."
	}

	rvPath, err := g.writeSkill(username+"-code-reviewer", codeReviewerTemplate, rvData)
	if err != nil {
		return nil, fmt.Errorf("generating code reviewer skill: %w", err)
	}
	paths = append(paths, rvPath)

	dpData := developerProfileData{
		Username:           username,
		DeveloperInterests: s.DeveloperInterests,
		ActivityPatterns:   s.ActivityPatterns,
		CollaborationStyle: s.CollaborationStyle,
		Traits:             s.DistinctiveTraits,
	}
	if dpData.DeveloperInterests == "" {
		dpData.DeveloperInterests = persona.DeveloperIdentity
	}
	if dpData.ActivityPatterns == "" {
		dpData.ActivityPatterns = "No specific activity-pattern data was identified."
	}
	if dpData.CollaborationStyle == "" {
		dpData.CollaborationStyle = "No specific collaboration data was identified."
	}
	if dpData.Traits == "" {
		dpData.Traits = "See developer interests above."
	}

	dpPath, err := g.writeSkill(username+"-developer-profile", developerProfileTemplate, dpData)
	if err != nil {
		return nil, fmt.Errorf("generating developer profile skill: %w", err)
	}
	paths = append(paths, dpPath)

	return paths, nil
}

func (g *Generator) writeSkill(name, tmplStr string, data any) (string, error) {
	tmpl, err := template.New(name).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parsing template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template %s: %w", name, err)
	}

	dir := filepath.Join(g.outputDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating directory %s: %w", dir, err)
	}

	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return "", fmt.Errorf("writing file %s: %w", path, err)
	}

	slog.Info("wrote skill", "path", path)
	return path, nil
}
