package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/drpaneas/devlica/internal/ghcrawl"
	"github.com/drpaneas/devlica/internal/llm"
	"github.com/drpaneas/devlica/internal/textutil"
	"golang.org/x/sync/errgroup"
)

const maxChunkSize = 30000 // bytes per LLM input chunk

// SynthesisResult holds the structured fields produced by the LLM synthesis step.
type SynthesisResult struct {
	CodingPhilosophy      string `json:"coding_philosophy"`
	CodeStyleRules        string `json:"code_style_rules"`
	ReviewPriorities      string `json:"review_priorities"`
	ReviewVoice           string `json:"review_voice"`
	CommunicationPatterns string `json:"communication_patterns"`
	TestingPhilosophy     string `json:"testing_philosophy"`
	DistinctiveTraits     string `json:"distinctive_traits"`
	DeveloperInterests    string `json:"developer_interests"`
	ProjectPatterns       string `json:"project_patterns"`
	CollaborationStyle    string `json:"collaboration_style"`
}

// Persona holds all analysis results for a developer.
type Persona struct {
	Username          string
	CodeStyle         string
	ReviewStyle       string
	Communication     string
	DeveloperIdentity string
	Synthesis         *SynthesisResult
}

// Analyzer uses an LLM provider to extract a developer persona from crawled data.
type Analyzer struct {
	provider llm.Provider
}

// New returns an Analyzer that uses the given LLM provider.
func New(provider llm.Provider) *Analyzer {
	return &Analyzer{provider: provider}
}

// Analyze runs parallel LLM analyses on the crawl data and synthesizes a Persona.
func (a *Analyzer) Analyze(ctx context.Context, username string, data *ghcrawl.CrawlResult) (*Persona, error) {
	persona := &Persona{Username: username}

	codeSamples := buildCodeSamplesText(data)
	commitDiffs := buildCommitDiffsText(data)
	reviewComments := buildReviewCommentsText(data)
	prDescriptions := buildPRDescriptionsText(data)
	issueComments := buildIssueCommentsText(data)
	authoredIssues := buildAuthoredIssuesText(data)
	releaseNotes := buildReleasesText(data)
	profileText := buildProfileText(data)
	starredText := buildStarredReposText(data)
	gistsText := buildGistsText(data)
	orgsText := buildOrgsText(data)
	externalPRsText := buildExternalPRsText(data)
	eventsText := buildEventsText(data)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if codeSamples == "" && commitDiffs == "" {
			slog.Warn("no code samples or commit diffs found, skipping code style analysis")
			persona.CodeStyle = "Insufficient data for code style analysis."
			return nil
		}
		slog.Info("analyzing code style")
		prompt := fmt.Sprintf(codeStylePrompt, username, truncateChunk(codeSamples), truncateChunk(commitDiffs))
		result, err := a.provider.Complete(gCtx, systemPrompt, prompt, nil)
		if err != nil {
			return fmt.Errorf("code style analysis: %w", err)
		}
		persona.CodeStyle = result
		return nil
	})

	g.Go(func() error {
		if reviewComments == "" {
			slog.Warn("no review comments found, skipping review style analysis")
			persona.ReviewStyle = "Insufficient data for review style analysis."
			return nil
		}
		slog.Info("analyzing review style")
		prompt := fmt.Sprintf(reviewStylePrompt, username, truncateChunk(reviewComments))
		result, err := a.provider.Complete(gCtx, systemPrompt, prompt, nil)
		if err != nil {
			return fmt.Errorf("review style analysis: %w", err)
		}
		persona.ReviewStyle = result
		return nil
	})

	g.Go(func() error {
		if prDescriptions == "" && issueComments == "" && authoredIssues == "" && releaseNotes == "" {
			slog.Warn("no communication data found, skipping communication analysis")
			persona.Communication = "Insufficient data for communication analysis."
			return nil
		}
		slog.Info("analyzing communication style")
		prompt := fmt.Sprintf(communicationPrompt, username,
			truncateChunk(prDescriptions),
			truncateChunk(issueComments),
			truncateChunk(authoredIssues),
			truncateChunk(releaseNotes),
		)
		result, err := a.provider.Complete(gCtx, systemPrompt, prompt, nil)
		if err != nil {
			return fmt.Errorf("communication analysis: %w", err)
		}
		persona.Communication = result
		return nil
	})

	g.Go(func() error {
		if profileText == "" && starredText == "" && gistsText == "" && externalPRsText == "" {
			slog.Warn("no identity data found, skipping developer identity analysis")
			persona.DeveloperIdentity = "Insufficient data for developer identity analysis."
			return nil
		}
		slog.Info("analyzing developer identity")
		prompt := fmt.Sprintf(developerIdentityPrompt, username,
			truncateChunk(profileText),
			truncateChunk(starredText),
			truncateChunk(gistsText),
			truncateChunk(orgsText),
			truncateChunk(externalPRsText),
			truncateChunk(eventsText),
		)
		result, err := a.provider.Complete(gCtx, systemPrompt, prompt, nil)
		if err != nil {
			return fmt.Errorf("developer identity analysis: %w", err)
		}
		persona.DeveloperIdentity = result
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	slog.Info("synthesizing developer persona")
	synthesisInput := fmt.Sprintf(synthesisPrompt,
		username,
		truncateChunk(persona.CodeStyle),
		truncateChunk(persona.ReviewStyle),
		truncateChunk(persona.Communication),
		truncateChunk(persona.DeveloperIdentity),
	)
	raw, err := a.provider.Complete(ctx, systemPrompt, synthesisInput, nil)
	if err != nil {
		return nil, fmt.Errorf("persona synthesis: %w", err)
	}

	synthesis, err := ParseSynthesis(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing synthesis JSON: %w", err)
	}
	persona.Synthesis = synthesis

	return persona, nil
}

// ParseSynthesis extracts a SynthesisResult from the LLM response. It handles
// both raw JSON and JSON wrapped in markdown code fences.
func ParseSynthesis(raw string) (*SynthesisResult, error) {
	text := strings.TrimSpace(raw)

	// Only strip code fences when the response has non-JSON preamble.
	// If it already starts with '{', the ``` may be inside a string value
	// (e.g. "Use GitHub ```suggestion...```") and stripping would destroy it.
	if text[0] != '{' {
		if idx := strings.Index(text, "```"); idx >= 0 {
			text = text[idx+3:]
			text = strings.TrimPrefix(text, "json")
			if end := strings.LastIndex(text, "```"); end >= 0 {
				text = text[:end]
			}
			text = strings.TrimSpace(text)
		}
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &rawMap); err != nil {
		sanitized := textutil.SanitizeJSON(text)
		if err2 := json.Unmarshal([]byte(sanitized), &rawMap); err2 != nil {
			return nil, fmt.Errorf("invalid JSON from LLM: %w\nraw response (first 500 bytes): %s",
				err, textutil.Truncate(raw, 500, "..."))
		}
	}

	for k, v := range rawMap {
		trimmed := strings.TrimSpace(string(v))
		if len(trimmed) > 0 && trimmed[0] == '[' {
			var items []string
			if err := json.Unmarshal(v, &items); err == nil {
				joined, _ := json.Marshal(strings.Join(items, "\n"))
				rawMap[k] = joined
			}
		}
	}

	normalized, err := json.Marshal(rawMap)
	if err != nil {
		return nil, fmt.Errorf("re-marshaling normalized JSON: %w", err)
	}

	var result SynthesisResult
	if err := json.Unmarshal(normalized, &result); err != nil {
		return nil, fmt.Errorf("invalid JSON from LLM after normalization: %w\nraw response (first 500 bytes): %s",
			err, textutil.Truncate(raw, 500, "..."))
	}
	return &result, nil
}

func buildCodeSamplesText(data *ghcrawl.CrawlResult) string {
	// Collect per-repo item lists, then interleave so each repo gets
	// fair representation within the context window.
	var buckets [][]string
	for _, repo := range data.Repos {
		var items []string
		for _, sample := range repo.CodeSamples {
			items = append(items, fmt.Sprintf("=== %s/%s ===\n%s\n\n", repo.FullName, sample.Path, sample.Content))
		}
		if len(items) > 0 {
			buckets = append(buckets, items)
		}
	}
	return interleave(buckets)
}

func buildCommitDiffsText(data *ghcrawl.CrawlResult) string {
	var buckets [][]string
	for _, repo := range data.Repos {
		var items []string
		for _, commit := range repo.Commits {
			if commit.Patch == "" {
				continue
			}
			sha := commit.SHA
			if len(sha) > 8 {
				sha = sha[:8]
			}
			stats := ""
			if commit.Additions > 0 || commit.Deletions > 0 {
				stats = fmt.Sprintf(" (+%d/-%d, %d files)", commit.Additions, commit.Deletions, commit.FilesChanged)
			}
			items = append(items, fmt.Sprintf("=== %s - %s%s ===\nMessage: %s\n%s\n\n",
				repo.FullName, sha, stats, commit.Message, commit.Patch))
		}
		if len(items) > 0 {
			buckets = append(buckets, items)
		}
	}
	return interleave(buckets)
}

func buildReviewCommentsText(data *ghcrawl.CrawlResult) string {
	var buckets [][]string
	for _, repo := range data.Repos {
		var items []string
		for _, rc := range repo.ReviewComments {
			items = append(items, fmt.Sprintf("=== %s (file: %s) ===\n%s\n\n", repo.FullName, rc.Path, rc.Body))
		}
		if len(items) == 0 {
			for _, cm := range repo.PRComments {
				items = append(items, fmt.Sprintf("=== %s (PR comment) ===\n%s\n\n", repo.FullName, cm.Body))
			}
		}
		if len(items) > 0 {
			buckets = append(buckets, items)
		}
	}
	return interleave(buckets)
}

func buildPRDescriptionsText(data *ghcrawl.CrawlResult) string {
	var buckets [][]string
	for _, repo := range data.Repos {
		var items []string
		for _, pr := range repo.PRs {
			if pr.Body == "" {
				continue
			}
			items = append(items, fmt.Sprintf("=== %s #%d: %s ===\n%s\n\n", repo.FullName, pr.Number, pr.Title, pr.Body))
		}
		if len(items) > 0 {
			buckets = append(buckets, items)
		}
	}
	// External PRs as their own bucket.
	var extItems []string
	for _, pr := range data.ExternalPRs {
		if pr.Body == "" {
			continue
		}
		extItems = append(extItems, fmt.Sprintf("=== %s #%d: %s ===\n%s\n\n", pr.Repo, pr.Number, pr.Title, pr.Body))
	}
	if len(extItems) > 0 {
		buckets = append(buckets, extItems)
	}
	return interleave(buckets)
}

func buildIssueCommentsText(data *ghcrawl.CrawlResult) string {
	// Group issue comments by repo, then interleave.
	repoComments := make(map[string][]string)
	for _, cm := range data.IssueComments {
		repoComments[cm.Repo] = append(repoComments[cm.Repo],
			fmt.Sprintf("=== %s ===\n%s\n\n", cm.Repo, cm.Body))
	}
	var buckets [][]string
	for _, items := range repoComments {
		buckets = append(buckets, items)
	}
	return interleave(buckets)
}

// interleave round-robins across buckets so each source gets fair
// representation. Takes one item from each bucket per round.
func interleave(buckets [][]string) string {
	if len(buckets) == 0 {
		return ""
	}
	var b strings.Builder
	maxLen := 0
	for _, bucket := range buckets {
		if len(bucket) > maxLen {
			maxLen = len(bucket)
		}
	}
	for round := 0; round < maxLen; round++ {
		for _, bucket := range buckets {
			if round < len(bucket) {
				b.WriteString(bucket[round])
			}
		}
	}
	return b.String()
}

func buildAuthoredIssuesText(data *ghcrawl.CrawlResult) string {
	var b strings.Builder
	for _, issue := range data.AuthoredIssues {
		labels := ""
		if len(issue.Labels) > 0 {
			labels = " [" + strings.Join(issue.Labels, ", ") + "]"
		}
		fmt.Fprintf(&b, "=== %s #%d: %s (%s)%s ===\n%s\n\n",
			issue.Repo, issue.Number, issue.Title, issue.State, labels, issue.Body)
	}
	return b.String()
}

func buildReleasesText(data *ghcrawl.CrawlResult) string {
	var b strings.Builder
	for _, repo := range data.Repos {
		for _, rel := range repo.Releases {
			if rel.Body == "" {
				continue
			}
			fmt.Fprintf(&b, "=== %s %s: %s ===\n%s\n\n", rel.Repo, rel.TagName, rel.Name, rel.Body)
		}
	}
	return b.String()
}

func buildProfileText(data *ghcrawl.CrawlResult) string {
	u := data.User
	var b strings.Builder
	fmt.Fprintf(&b, "Login: %s\n", u.Login)
	if u.Name != "" {
		fmt.Fprintf(&b, "Name: %s\n", u.Name)
	}
	if u.Bio != "" {
		fmt.Fprintf(&b, "Bio: %s\n", u.Bio)
	}
	if u.Company != "" {
		fmt.Fprintf(&b, "Company: %s\n", u.Company)
	}
	if u.Location != "" {
		fmt.Fprintf(&b, "Location: %s\n", u.Location)
	}
	if u.Blog != "" {
		fmt.Fprintf(&b, "Blog: %s\n", u.Blog)
	}
	if u.Email != "" {
		fmt.Fprintf(&b, "Email: %s\n", u.Email)
	}
	if u.TwitterUsername != "" {
		fmt.Fprintf(&b, "Twitter: @%s\n", u.TwitterUsername)
	}
	fmt.Fprintf(&b, "Followers: %d, Following: %d\n", u.Followers, u.Following)
	fmt.Fprintf(&b, "Public repos: %d\n", u.PublicRepos)
	if !u.CreatedAt.IsZero() {
		fmt.Fprintf(&b, "Account created: %s\n", u.CreatedAt.Format("2006-01-02"))
	}
	if u.ProfileREADME != "" {
		fmt.Fprintf(&b, "\nProfile README:\n%s\n", u.ProfileREADME)
	}

	var repoSummary strings.Builder
	langCount := make(map[string]int)
	licenseCount := make(map[string]int)
	for _, repo := range data.Repos {
		if !repo.IsOwner {
			continue
		}
		if repo.Language != "" {
			langCount[repo.Language]++
		}
		if repo.License != "" {
			licenseCount[repo.License]++
		}
	}
	if len(langCount) > 0 {
		fmt.Fprintf(&repoSummary, "\nLanguage distribution across owned repos:\n")
		for lang, count := range langCount {
			fmt.Fprintf(&repoSummary, "  %s: %d repos\n", lang, count)
		}
	}
	if len(licenseCount) > 0 {
		fmt.Fprintf(&repoSummary, "\nLicense preferences:\n")
		for lic, count := range licenseCount {
			fmt.Fprintf(&repoSummary, "  %s: %d repos\n", lic, count)
		}
	}
	b.WriteString(repoSummary.String())

	return b.String()
}

func buildStarredReposText(data *ghcrawl.CrawlResult) string {
	if len(data.StarredRepos) == 0 {
		return ""
	}
	var b strings.Builder
	langCount := make(map[string]int)
	for _, sr := range data.StarredRepos {
		if sr.Language != "" {
			langCount[sr.Language]++
		}
	}
	if len(langCount) > 0 {
		fmt.Fprintf(&b, "Languages of starred repos:\n")
		for lang, count := range langCount {
			fmt.Fprintf(&b, "  %s: %d\n", lang, count)
		}
		b.WriteString("\n")
	}
	limit := 50
	if len(data.StarredRepos) < limit {
		limit = len(data.StarredRepos)
	}
	for _, sr := range data.StarredRepos[:limit] {
		desc := sr.Description
		if len(desc) > 100 {
			desc = desc[:100] + "..."
		}
		topics := ""
		if len(sr.Topics) > 0 {
			topics = " [" + strings.Join(sr.Topics, ", ") + "]"
		}
		fmt.Fprintf(&b, "- %s (%s, %d stars)%s: %s\n", sr.FullName, sr.Language, sr.Stars, topics, desc)
	}
	if len(data.StarredRepos) > limit {
		fmt.Fprintf(&b, "... and %d more starred repos\n", len(data.StarredRepos)-limit)
	}
	return b.String()
}

func buildGistsText(data *ghcrawl.CrawlResult) string {
	if len(data.Gists) == 0 {
		return ""
	}
	var b strings.Builder
	for _, g := range data.Gists {
		visibility := "public"
		if !g.Public {
			visibility = "private"
		}
		var fileNames []string
		for _, f := range g.Files {
			if f.Language != "" {
				fileNames = append(fileNames, fmt.Sprintf("%s (%s)", f.Name, f.Language))
			} else {
				fileNames = append(fileNames, f.Name)
			}
		}
		fmt.Fprintf(&b, "- [%s] %s: %s | files: %s\n",
			visibility, g.CreatedAt.Format("2006-01-02"), g.Description, strings.Join(fileNames, ", "))
	}
	return b.String()
}

func buildOrgsText(data *ghcrawl.CrawlResult) string {
	if len(data.Orgs) == 0 {
		return "No public organization memberships."
	}
	return "Member of: " + strings.Join(data.Orgs, ", ")
}

func buildExternalPRsText(data *ghcrawl.CrawlResult) string {
	if len(data.ExternalPRs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, pr := range data.ExternalPRs {
		stats := ""
		if pr.Additions > 0 || pr.Deletions > 0 {
			stats = fmt.Sprintf(" (+%d/-%d, %d files)", pr.Additions, pr.Deletions, pr.ChangedFiles)
		}
		fmt.Fprintf(&b, "- %s #%d: %s [%s]%s\n", pr.Repo, pr.Number, pr.Title, pr.State, stats)
	}
	return b.String()
}

func buildEventsText(data *ghcrawl.CrawlResult) string {
	if len(data.Events) == 0 {
		return ""
	}
	var b strings.Builder
	typeCount := make(map[string]int)
	for _, ev := range data.Events {
		typeCount[ev.Type]++
	}
	fmt.Fprintf(&b, "Recent activity summary (%d events):\n", len(data.Events))
	for t, count := range typeCount {
		fmt.Fprintf(&b, "  %s: %d\n", t, count)
	}
	b.WriteString("\nRecent events:\n")
	limit := 30
	if len(data.Events) < limit {
		limit = len(data.Events)
	}
	for _, ev := range data.Events[:limit] {
		fmt.Fprintf(&b, "  %s: %s\n", ev.CreatedAt.Format("2006-01-02"), ev.Summary)
	}
	return b.String()
}

func truncateChunk(s string) string {
	return textutil.Truncate(s, maxChunkSize, "\n... (data truncated to fit context window)")
}
