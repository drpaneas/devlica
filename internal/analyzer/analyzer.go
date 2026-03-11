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

const evidenceCompressionPrompt = `You are preparing evidence for a downstream persona analysis.
Summarize this %s chunk into high-signal bullet points.

Requirements:
- Preserve concrete examples and exact phrasing when possible.
- Keep what is distinctive or repeated.
- Include counts/pattern frequencies if visible in this chunk.
- Do not add speculation.

Chunk %d/%d:
%s`

// SynthesisResult holds the structured fields produced by the LLM synthesis step.
type SynthesisResult struct {
	CodingPhilosophy      string `json:"coding_philosophy"`
	CodeStyleRules        string `json:"code_style_rules"`
	ReviewPriorities      string `json:"review_priorities"`
	ReviewDecisionStyle   string `json:"review_decision_style"`
	ReviewNonBlockingNits string `json:"review_non_blocking_nits"`
	ReviewContext         string `json:"review_context_sensitivity"`
	ReviewVoice           string `json:"review_voice"`
	CommunicationPatterns string `json:"communication_patterns"`
	TestingPhilosophy     string `json:"testing_philosophy"`
	DistinctiveTraits     string `json:"distinctive_traits"`
	DeveloperInterests    string `json:"developer_interests"`
	ActivityPatterns      string `json:"activity_patterns"`
	ProjectPatterns       string `json:"project_patterns"`
	CollaborationStyle    string `json:"collaboration_style"`
	CodeExamples          string `json:"code_examples"`
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
	reviewActivity := buildReviewDataText(data)
	prDescriptions := buildPRDescriptionsText(data)
	issueComments := buildIssueCommentsText(data)
	authoredIssues := buildAuthoredIssuesText(data)
	releaseNotes := buildReleasesText(data)
	discussionsText := buildDiscussionsText(data)
	profileText := buildProfileText(data)
	starredText := buildStarredReposText(data)
	gistsText := buildGistsText(data)
	orgsText := buildOrgsText(data)
	externalPRsText := buildExternalPRsText(data)
	eventsText := buildEventsText(data)
	projectsText := buildProjectsText(data)
	wikiText := buildWikiPagesText(data)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if codeSamples == "" && commitDiffs == "" {
			slog.Warn("no code samples or commit diffs found, skipping code style analysis")
			persona.CodeStyle = "Insufficient data for code style analysis."
			return nil
		}
		codeSamplesPrepared, err := a.compressToFit(gCtx, "code samples", codeSamples)
		if err != nil {
			return fmt.Errorf("compressing code samples: %w", err)
		}
		commitDiffsPrepared, err := a.compressToFit(gCtx, "commit diffs", commitDiffs)
		if err != nil {
			return fmt.Errorf("compressing commit diffs: %w", err)
		}
		slog.Info("analyzing code style")
		prompt := fmt.Sprintf(codeStylePrompt, username, codeSamplesPrepared, commitDiffsPrepared)
		result, err := a.provider.Complete(gCtx, systemPrompt, prompt, nil)
		if err != nil {
			return fmt.Errorf("code style analysis: %w", err)
		}
		persona.CodeStyle = result
		return nil
	})

	g.Go(func() error {
		if reviewActivity == "" {
			slog.Warn("no review comments found, skipping review style analysis")
			persona.ReviewStyle = "Insufficient data for review style analysis."
			return nil
		}
		reviewPrepared, err := a.compressToFit(gCtx, "review activity", reviewActivity)
		if err != nil {
			return fmt.Errorf("compressing review activity: %w", err)
		}
		slog.Info("analyzing review style")
		prompt := fmt.Sprintf(reviewStylePrompt, username, reviewPrepared)
		result, err := a.provider.Complete(gCtx, systemPrompt, prompt, nil)
		if err != nil {
			return fmt.Errorf("review style analysis: %w", err)
		}
		persona.ReviewStyle = result
		return nil
	})

	g.Go(func() error {
		if prDescriptions == "" && issueComments == "" && authoredIssues == "" && releaseNotes == "" && discussionsText == "" {
			slog.Warn("no communication data found, skipping communication analysis")
			persona.Communication = "Insufficient data for communication analysis."
			return nil
		}
		prPrepared, err := a.compressToFit(gCtx, "pull request descriptions", prDescriptions)
		if err != nil {
			return fmt.Errorf("compressing PR descriptions: %w", err)
		}
		issueCommentsPrepared, err := a.compressToFit(gCtx, "issue comments", issueComments)
		if err != nil {
			return fmt.Errorf("compressing issue comments: %w", err)
		}
		authoredIssuesPrepared, err := a.compressToFit(gCtx, "authored issues", authoredIssues)
		if err != nil {
			return fmt.Errorf("compressing authored issues: %w", err)
		}
		releasesPrepared, err := a.compressToFit(gCtx, "release notes", releaseNotes)
		if err != nil {
			return fmt.Errorf("compressing release notes: %w", err)
		}
		discussionsPrepared, err := a.compressToFit(gCtx, "discussions", discussionsText)
		if err != nil {
			return fmt.Errorf("compressing discussions: %w", err)
		}
		slog.Info("analyzing communication style")
		prompt := fmt.Sprintf(communicationPrompt, username,
			prPrepared,
			issueCommentsPrepared,
			authoredIssuesPrepared,
			releasesPrepared,
			discussionsPrepared,
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
		profilePrepared, err := a.compressToFit(gCtx, "profile", profileText)
		if err != nil {
			return fmt.Errorf("compressing profile: %w", err)
		}
		starredPrepared, err := a.compressToFit(gCtx, "starred repositories", starredText)
		if err != nil {
			return fmt.Errorf("compressing starred repositories: %w", err)
		}
		gistsPrepared, err := a.compressToFit(gCtx, "gists", gistsText)
		if err != nil {
			return fmt.Errorf("compressing gists: %w", err)
		}
		orgsPrepared, err := a.compressToFit(gCtx, "organizations", orgsText)
		if err != nil {
			return fmt.Errorf("compressing organizations: %w", err)
		}
		externalPRsPrepared, err := a.compressToFit(gCtx, "external pull requests", externalPRsText)
		if err != nil {
			return fmt.Errorf("compressing external pull requests: %w", err)
		}
		eventsPrepared, err := a.compressToFit(gCtx, "recent activity events", eventsText)
		if err != nil {
			return fmt.Errorf("compressing activity events: %w", err)
		}
		projectsPrepared, err := a.compressToFit(gCtx, "projects", projectsText)
		if err != nil {
			return fmt.Errorf("compressing projects: %w", err)
		}
		wikiPrepared, err := a.compressToFit(gCtx, "wiki pages", wikiText)
		if err != nil {
			return fmt.Errorf("compressing wiki pages: %w", err)
		}
		slog.Info("analyzing developer identity")
		prompt := fmt.Sprintf(developerIdentityPrompt, username,
			profilePrepared,
			starredPrepared,
			gistsPrepared,
			orgsPrepared,
			externalPRsPrepared,
			eventsPrepared,
			projectsPrepared,
			wikiPrepared,
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
	if text == "" {
		return nil, fmt.Errorf("invalid JSON from LLM: empty response")
	}

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

func buildReviewDataText(data *ghcrawl.CrawlResult) string {
	var buckets [][]string
	for _, repo := range data.Repos {
		var items []string
		for _, review := range repo.Reviews {
			stats := ""
			if review.Additions > 0 || review.Deletions > 0 || review.ChangedFiles > 0 {
				stats = fmt.Sprintf(" (+%d/-%d, %d files, %d inline comments)",
					review.Additions, review.Deletions, review.ChangedFiles, review.ReviewCommentCount)
			}
			labels := ""
			if len(review.Labels) > 0 {
				labels = " [" + strings.Join(review.Labels, ", ") + "]"
			}
			body := review.Body
			if body == "" {
				body = "(no summary text)"
			}
			items = append(items, fmt.Sprintf(
				"=== %s PR #%d: %s ===\nAuthor: %s\nState: %s%s%s\nSummary:\n%s\n\n",
				review.Repo,
				review.PRNumber,
				review.PRTitle,
				review.PRAuthor,
				review.State,
				stats,
				labels,
				body,
			))
		}
		for _, rc := range repo.ReviewComments {
			title := rc.PRTitle
			if title == "" {
				title = "(unknown PR title)"
			}
			diff := rc.DiffHunk
			if diff == "" {
				diff = "(no diff hunk available)"
			}
			items = append(items, fmt.Sprintf(
				"=== %s PR #%d: %s (file: %s) ===\nAuthor: %s\nDiff hunk:\n%s\n\nComment:\n%s\n\n",
				repo.FullName,
				rc.PRNumber,
				title,
				rc.Path,
				rc.PRAuthor,
				diff,
				rc.Body,
			))
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
		stats := ""
		if pr.Additions > 0 || pr.Deletions > 0 || pr.ChangedFiles > 0 {
			stats = fmt.Sprintf(" (+%d/-%d, %d files)", pr.Additions, pr.Deletions, pr.ChangedFiles)
		}
		body := pr.Body
		if body == "" {
			body = "(no description)"
		}
		extItems = append(extItems, fmt.Sprintf(
			"=== %s #%d: %s [%s]%s ===\nAuthor: %s\n%s\n\n",
			pr.Repo,
			pr.Number,
			pr.Title,
			pr.State,
			stats,
			pr.Author,
			body,
		))
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
			if f.Content != "" {
				fmt.Fprintf(&b, "  snippet %s:\n%s\n", f.Name, textutil.Truncate(f.Content, 400, "\n..."))
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

func buildDiscussionsText(data *ghcrawl.CrawlResult) string {
	if len(data.Discussions) == 0 {
		return ""
	}
	var buckets [][]string
	repoItems := make(map[string][]string)
	for _, d := range data.Discussions {
		var b strings.Builder
		fmt.Fprintf(&b, "=== %s #%d: %s [%s] ===\nThread author: %s\n",
			d.Repo, d.Number, d.Title, d.Category, d.Author)
		if d.Body != "" {
			fmt.Fprintf(&b, "%s\n", d.Body)
		}
		for _, cm := range d.Comments {
			if cm.Author != "" {
				fmt.Fprintf(&b, "  Comment by %s: %s\n", cm.Author, cm.Body)
				continue
			}
			fmt.Fprintf(&b, "  Comment: %s\n", cm.Body)
		}
		b.WriteByte('\n')
		repoItems[d.Repo] = append(repoItems[d.Repo], b.String())
	}
	for _, items := range repoItems {
		buckets = append(buckets, items)
	}
	return interleave(buckets)
}

func buildProjectsText(data *ghcrawl.CrawlResult) string {
	if len(data.Projects) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range data.Projects {
		visibility := "public"
		if !p.Public {
			visibility = "private"
		}
		fmt.Fprintf(&b, "- [%s] %s (%d items): %s\n",
			visibility, p.Title, p.ItemCount, p.Body)
	}
	return b.String()
}

func buildWikiPagesText(data *ghcrawl.CrawlResult) string {
	var buckets [][]string
	for _, repo := range data.Repos {
		var items []string
		for _, wp := range repo.WikiPages {
			items = append(items, fmt.Sprintf("=== %s - %s ===\n%s\n\n",
				wp.Repo, wp.Title, textutil.Truncate(wp.Content, 2000, "\n... (truncated)")))
		}
		if len(items) > 0 {
			buckets = append(buckets, items)
		}
	}
	return interleave(buckets)
}

func truncateChunk(s string) string {
	return textutil.Truncate(s, maxChunkSize, "\n... (data truncated to fit context window)")
}

func (a *Analyzer) compressToFit(ctx context.Context, label, input string) (string, error) {
	if input == "" || len(input) <= maxChunkSize {
		return input, nil
	}
	current := input
	for pass := 0; pass < 4; pass++ {
		if len(current) <= maxChunkSize {
			return current, nil
		}
		chunks := splitChunks(current, maxChunkSize)
		summaries := make([]string, 0, len(chunks))
		for i, chunk := range chunks {
			prompt := fmt.Sprintf(evidenceCompressionPrompt, label, i+1, len(chunks), chunk)
			out, err := a.provider.Complete(ctx, systemPrompt, prompt, nil)
			if err != nil {
				return "", err
			}
			summaries = append(summaries, out)
		}
		current = strings.Join(summaries, "\n\n")
		label = label + " summary"
	}
	return truncateChunk(current), nil
}

func splitChunks(s string, max int) []string {
	if s == "" || max <= 0 {
		return nil
	}
	if len(s) <= max {
		return []string{s}
	}
	var chunks []string
	remaining := s
	for len(remaining) > 0 {
		if len(remaining) <= max {
			chunks = append(chunks, remaining)
			break
		}
		cut := max
		if idx := strings.LastIndex(remaining[:max], "\n\n"); idx > max/2 {
			cut = idx
		} else if idx := strings.LastIndexByte(remaining[:max], '\n'); idx > max/2 {
			cut = idx
		}
		chunks = append(chunks, remaining[:cut])
		remaining = remaining[cut:]
	}
	return chunks
}
