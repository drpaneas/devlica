package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/drpaneas/devlica/internal/analyzer"
	"github.com/drpaneas/devlica/internal/ghcrawl"
	"github.com/drpaneas/devlica/internal/llm"
	"github.com/drpaneas/devlica/internal/textutil"
)

const (
	MaxHeldOut    = 3
	MaxIterations = 5
	TargetScore   = 80.0
)

// HeldOutReview is a review comment withheld from persona building for validation.
type HeldOutReview struct {
	RepoFullName string
	Body         string
	Path         string
	DiffHunk     string
}

// ReviewPair pairs a held-out original review with its dry-run counterpart.
type ReviewPair struct {
	Original  string
	Generated string
	Path      string
	Score     float64
}

// IterationResult holds the outcome of a single benchmark iteration.
type IterationResult struct {
	Iteration int
	Score     float64
	Feedback  string
	Pairs     []ReviewPair
}

// Result holds the overall benchmark outcome.
type Result struct {
	FinalScore float64
	Iterations int
	History    []IterationResult
}

// SplitReviews removes up to max reviews that have non-empty DiffHunks from data
// and returns them as held-out test samples. It modifies data.Repos in place so
// the held-out reviews are not visible during persona analysis.
func SplitReviews(data *ghcrawl.CrawlResult, max int) []HeldOutReview {
	var heldOut []HeldOutReview
	for i := range data.Repos {
		repo := &data.Repos[i]
		var kept []ghcrawl.ReviewComment
		for _, rc := range repo.ReviewComments {
			if len(heldOut) < max && rc.DiffHunk != "" {
				heldOut = append(heldOut, HeldOutReview{
					RepoFullName: repo.FullName,
					Body:         rc.Body,
					Path:         rc.Path,
					DiffHunk:     rc.DiffHunk,
				})
			} else {
				kept = append(kept, rc)
			}
		}
		repo.ReviewComments = kept
	}
	return heldOut
}

// Benchmarker validates persona quality by generating dry-run reviews and
// comparing them against held-out originals.
type Benchmarker struct {
	provider llm.Provider
}

// New returns a Benchmarker that uses the given LLM provider.
func New(provider llm.Provider) *Benchmarker {
	return &Benchmarker{provider: provider}
}

// Run performs the benchmark loop: for each iteration it generates dry-run
// reviews using the persona, compares them with the originals, scores the
// match, and refines the persona if the score is below the target. It runs
// at most MaxIterations times. Returns the benchmark result and a potentially
// refined persona.
func (b *Benchmarker) Run(ctx context.Context, persona *analyzer.Persona, heldOut []HeldOutReview) (*Result, *analyzer.Persona, error) {
	if len(heldOut) == 0 {
		slog.Warn("no held-out reviews available, skipping benchmark")
		return &Result{FinalScore: -1}, persona, nil
	}

	result := &Result{}
	current := clonePersona(persona)

	for iter := 1; iter <= MaxIterations; iter++ {
		slog.Info("benchmark iteration", "iteration", iter, "max", MaxIterations)

		iterResult, err := b.runIteration(ctx, current, heldOut, iter)
		if err != nil {
			return nil, nil, fmt.Errorf("benchmark iteration %d: %w", iter, err)
		}

		result.History = append(result.History, *iterResult)
		result.FinalScore = iterResult.Score
		result.Iterations = iter

		slog.Info("benchmark score", "iteration", iter, "score", fmt.Sprintf("%.1f", iterResult.Score))

		if iterResult.Score >= TargetScore {
			slog.Info("benchmark target reached", "score", fmt.Sprintf("%.1f", iterResult.Score))
			break
		}

		if iter < MaxIterations {
			slog.Info("refining persona", "iteration", iter)
			refined, err := b.refinePersona(ctx, current, iterResult)
			if err != nil {
				return nil, nil, fmt.Errorf("refining persona (iter %d): %w", iter, err)
			}
			current = refined
		}
	}

	return result, current, nil
}

func (b *Benchmarker) runIteration(ctx context.Context, persona *analyzer.Persona, heldOut []HeldOutReview, iter int) (*IterationResult, error) {
	iterResult := &IterationResult{Iteration: iter}
	var totalScore float64
	var feedbackParts []string

	for _, ho := range heldOut {
		generated, err := b.generateDryRunReview(ctx, persona, ho)
		if err != nil {
			return nil, fmt.Errorf("dry-run review: %w", err)
		}

		comp, err := b.compareReviews(ctx, ho, generated)
		if err != nil {
			return nil, fmt.Errorf("comparison: %w", err)
		}

		iterResult.Pairs = append(iterResult.Pairs, ReviewPair{
			Original:  ho.Body,
			Generated: generated,
			Path:      ho.Path,
			Score:     comp.score,
		})
		totalScore += comp.score
		feedbackParts = append(feedbackParts, comp.feedback)
	}

	iterResult.Score = totalScore / float64(len(heldOut))
	iterResult.Feedback = strings.Join(feedbackParts, "\n---\n")
	return iterResult, nil
}

func (b *Benchmarker) generateDryRunReview(ctx context.Context, persona *analyzer.Persona, ho HeldOutReview) (string, error) {
	prompt := fmt.Sprintf(dryRunReviewPrompt,
		persona.Username,
		formatPersonaContext(persona),
		ho.Path,
		ho.DiffHunk,
	)
	return b.provider.Complete(ctx, dryRunSystemPrompt, prompt, nil)
}

type comparisonResult struct {
	score    float64
	feedback string
}

func (b *Benchmarker) compareReviews(ctx context.Context, ho HeldOutReview, generated string) (*comparisonResult, error) {
	prompt := fmt.Sprintf(comparePrompt,
		ho.Path,
		ho.DiffHunk,
		ho.Body,
		generated,
	)
	raw, err := b.provider.Complete(ctx, compareSystemPrompt, prompt, nil)
	if err != nil {
		return nil, err
	}
	return parseComparisonResult(raw)
}

func (b *Benchmarker) refinePersona(ctx context.Context, persona *analyzer.Persona, iter *IterationResult) (*analyzer.Persona, error) {
	var pairsSummary strings.Builder
	for i, pair := range iter.Pairs {
		fmt.Fprintf(&pairsSummary, "--- Review Pair %d (file: %s, score: %.0f) ---\n", i+1, pair.Path, pair.Score)
		fmt.Fprintf(&pairsSummary, "ORIGINAL:\n%s\n\nGENERATED:\n%s\n\n", pair.Original, pair.Generated)
	}

	s := persona.Synthesis
	prompt := fmt.Sprintf(refinePrompt,
		persona.Username,
		iter.Score,
		s.CodingPhilosophy,
		s.CodeStyleRules,
		s.ReviewPriorities,
		s.ReviewVoice,
		s.CommunicationPatterns,
		s.TestingPhilosophy,
		s.DistinctiveTraits,
		s.DeveloperInterests,
		s.ProjectPatterns,
		s.CollaborationStyle,
		iter.Feedback,
		pairsSummary.String(),
	)

	raw, err := b.provider.Complete(ctx, refineSystemPrompt, prompt, nil)
	if err != nil {
		return nil, err
	}

	synthesis, err := analyzer.ParseSynthesis(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing refined synthesis: %w", err)
	}

	refined := clonePersona(persona)
	refined.Synthesis = synthesis
	return refined, nil
}

func formatPersonaContext(p *analyzer.Persona) string {
	s := p.Synthesis
	var b strings.Builder
	fmt.Fprintf(&b, "CODING PHILOSOPHY:\n%s\n\n", s.CodingPhilosophy)
	fmt.Fprintf(&b, "CODE STYLE RULES:\n%s\n\n", s.CodeStyleRules)
	fmt.Fprintf(&b, "REVIEW PRIORITIES:\n%s\n\n", s.ReviewPriorities)
	fmt.Fprintf(&b, "REVIEW VOICE:\n%s\n\n", s.ReviewVoice)
	fmt.Fprintf(&b, "COMMUNICATION PATTERNS:\n%s\n\n", s.CommunicationPatterns)
	fmt.Fprintf(&b, "TESTING PHILOSOPHY:\n%s\n\n", s.TestingPhilosophy)
	fmt.Fprintf(&b, "DISTINCTIVE TRAITS:\n%s\n\n", s.DistinctiveTraits)
	fmt.Fprintf(&b, "DEVELOPER INTERESTS:\n%s\n\n", s.DeveloperInterests)
	fmt.Fprintf(&b, "PROJECT PATTERNS:\n%s\n\n", s.ProjectPatterns)
	fmt.Fprintf(&b, "COLLABORATION STYLE:\n%s\n", s.CollaborationStyle)
	return b.String()
}

func clonePersona(p *analyzer.Persona) *analyzer.Persona {
	clone := *p
	if p.Synthesis != nil {
		s := *p.Synthesis
		clone.Synthesis = &s
	}
	return &clone
}

func parseComparisonResult(raw string) (*comparisonResult, error) {
	text := stripCodeFences(raw)

	var parsed struct {
		Score    float64 `json:"score"`
		Feedback string  `json:"feedback"`
	}
	// Use Decoder to parse the first JSON object, ignoring any trailing
	// commentary the LLM may append after the closing brace.
	dec := json.NewDecoder(strings.NewReader(text))
	if err := dec.Decode(&parsed); err != nil {
		sanitized := textutil.SanitizeJSON(text)
		dec2 := json.NewDecoder(strings.NewReader(sanitized))
		if err2 := dec2.Decode(&parsed); err2 != nil {
			return nil, fmt.Errorf("invalid comparison JSON: %w\nraw (first 500 bytes): %s",
				err, textutil.Truncate(raw, 500, "..."))
		}
	}
	return &comparisonResult{score: parsed.Score, feedback: parsed.Feedback}, nil
}

func stripCodeFences(s string) string {
	text := strings.TrimSpace(s)
	if len(text) > 0 && text[0] != '{' {
		if idx := strings.Index(text, "```"); idx >= 0 {
			text = text[idx+3:]
			text = strings.TrimPrefix(text, "json")
			if end := strings.LastIndex(text, "```"); end >= 0 {
				text = text[:end]
			}
			text = strings.TrimSpace(text)
		}
	}
	return text
}
