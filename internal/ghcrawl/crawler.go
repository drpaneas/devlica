package ghcrawl

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/drpaneas/devlica/internal/textutil"
	"github.com/google/go-github/v68/github"
	"golang.org/x/sync/errgroup"
)

const (
	maxCommitsPerRepo = 50
	maxPRsPerRepo     = 30
	maxReviewsPerPR   = 50
	maxCodeSamples    = 5
	maxFileSizeBytes  = 32 * 1024
	maxPatchLen       = 4096
	crawlConcurrency  = 5
	maxIssueComments  = 500
	maxSearchResults  = 200
	maxStarredRepos   = 500
	maxGists          = 100
	maxEvents         = 300
)

// Crawler fetches a GitHub user's repositories, commits, PRs, and comments.
type Crawler struct {
	client   *github.Client
	maxRepos int
}

// NewCrawler returns a Crawler authenticated with the given token.
// maxRepos controls how many repos get deep-crawled (commits, PRs, code samples).
func NewCrawler(token string, maxRepos int) *Crawler {
	return &Crawler{
		client:   newGitHubClient(token),
		maxRepos: maxRepos,
	}
}

// Crawl collects activity data for the given GitHub user.
func (c *Crawler) Crawl(ctx context.Context, username string) (*CrawlResult, error) {
	result := &CrawlResult{}

	profile, err := c.fetchProfile(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("fetching profile: %w", err)
	}
	result.User = profile

	readme, err := c.fetchProfileREADME(ctx, username)
	if err != nil {
		slog.Debug("no profile README", "error", err)
	} else {
		result.User.ProfileREADME = readme
	}

	repos, err := c.fetchRepos(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("listing repos: %w", err)
	}

	// Select a diverse set of repos for deep-crawling, ensuring coverage
	// across languages, time periods, and activity levels rather than
	// just the most recently pushed repos.
	deepCrawl := selectDiverseRepos(repos, c.maxRepos, username)

	var mu sync.Mutex
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(crawlConcurrency)
	for _, repo := range deepCrawl {
		g.Go(func() error {
			rd, err := c.crawlRepo(gCtx, username, repo)
			if err != nil {
				slog.Warn("skipping repo", "repo", repo.GetFullName(), "error", err)
				return nil
			}
			mu.Lock()
			result.Repos = append(result.Repos, rd)
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Include remaining repos as metadata-only (no deep crawl).
	if len(repos) > c.maxRepos {
		for _, repo := range repos[c.maxRepos:] {
			owner := repo.GetOwner().GetLogin()
			rd := RepoData{
				Name:          repo.GetName(),
				FullName:      repo.GetFullName(),
				Description:   repo.GetDescription(),
				Language:      repo.GetLanguage(),
				Stars:         repo.GetStargazersCount(),
				Forks:         repo.GetForksCount(),
				Topics:        repo.Topics,
				IsOwner:       strings.EqualFold(owner, username),
				IsFork:        repo.GetFork(),
				Archived:      repo.GetArchived(),
				DefaultBranch: repo.GetDefaultBranch(),
				OpenIssues:    repo.GetOpenIssuesCount(),
				CreatedAt:     repo.GetCreatedAt().Time,
				UpdatedAt:     repo.GetUpdatedAt().Time,
			}
			if repo.GetLicense() != nil {
				rd.License = repo.GetLicense().GetSPDXID()
			}
			result.Repos = append(result.Repos, rd)
		}
	}

	// Always search external reviews (not just when owned repos have zero).
	crawledRepos := make(map[string]bool, len(result.Repos))
	for _, r := range result.Repos {
		crawledRepos[r.FullName] = true
	}
	extRepos, err := c.fetchExternalReviews(ctx, username, crawledRepos)
	if err != nil {
		slog.Warn("could not fetch external reviews", "error", err)
	} else if len(extRepos) > 0 {
		for _, r := range extRepos {
			slog.Info("found external review activity",
				"repo", r.FullName,
				"line_comments", len(r.ReviewComments),
				"pr_comments", len(r.PRComments),
			)
		}
		result.Repos = append(result.Repos, extRepos...)
	}

	// Fetch independent data sources concurrently.
	g2, gCtx2 := errgroup.WithContext(ctx)

	g2.Go(func() error {
		comments, err := c.fetchIssueComments(gCtx2, username)
		if err != nil {
			slog.Warn("could not fetch issue comments", "error", err)
		} else {
			mu.Lock()
			result.IssueComments = comments
			mu.Unlock()
		}
		return nil
	})

	g2.Go(func() error {
		starred, err := c.fetchStarredRepos(gCtx2, username)
		if err != nil {
			slog.Warn("could not fetch starred repos", "error", err)
		} else {
			mu.Lock()
			result.StarredRepos = starred
			mu.Unlock()
		}
		return nil
	})

	g2.Go(func() error {
		gists, err := c.fetchGists(gCtx2, username)
		if err != nil {
			slog.Warn("could not fetch gists", "error", err)
		} else {
			mu.Lock()
			result.Gists = gists
			mu.Unlock()
		}
		return nil
	})

	g2.Go(func() error {
		orgs, err := c.fetchOrgs(gCtx2, username)
		if err != nil {
			slog.Warn("could not fetch orgs", "error", err)
		} else {
			mu.Lock()
			result.Orgs = orgs
			mu.Unlock()
		}
		return nil
	})

	g2.Go(func() error {
		events, err := c.fetchEvents(gCtx2, username)
		if err != nil {
			slog.Warn("could not fetch events", "error", err)
		} else {
			mu.Lock()
			result.Events = events
			mu.Unlock()
		}
		return nil
	})

	g2.Go(func() error {
		issues, err := c.fetchAuthoredIssues(gCtx2, username)
		if err != nil {
			slog.Warn("could not fetch authored issues", "error", err)
		} else {
			mu.Lock()
			result.AuthoredIssues = issues
			mu.Unlock()
		}
		return nil
	})

	g2.Go(func() error {
		extPRs, err := c.fetchExternalPRs(gCtx2, username)
		if err != nil {
			slog.Warn("could not fetch external PRs", "error", err)
		} else {
			mu.Lock()
			result.ExternalPRs = extPRs
			mu.Unlock()
		}
		return nil
	})

	if err := g2.Wait(); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Crawler) fetchProfile(ctx context.Context, username string) (UserProfile, error) {
	user, _, err := c.client.Users.Get(ctx, username)
	if err != nil {
		return UserProfile{}, err
	}
	return UserProfile{
		Login:           user.GetLogin(),
		Name:            user.GetName(),
		Bio:             user.GetBio(),
		Company:         user.GetCompany(),
		Location:        user.GetLocation(),
		Blog:            user.GetBlog(),
		Email:           user.GetEmail(),
		TwitterUsername: user.GetTwitterUsername(),
		Hireable:        user.GetHireable(),
		Followers:       user.GetFollowers(),
		Following:       user.GetFollowing(),
		PublicRepos:     user.GetPublicRepos(),
		CreatedAt:       user.GetCreatedAt().Time,
	}, nil
}

func (c *Crawler) fetchProfileREADME(ctx context.Context, username string) (string, error) {
	readme, _, err := c.client.Repositories.GetReadme(ctx, username, username, nil)
	if err != nil {
		return "", err
	}
	content, err := readme.GetContent()
	if err != nil {
		return "", err
	}
	return truncate(content, 4000), nil
}

func (c *Crawler) fetchRepos(ctx context.Context, username string) ([]*github.Repository, error) {
	opts := &github.RepositoryListByUserOptions{
		Sort:        "pushed",
		Direction:   "desc",
		Type:        "all",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var all []*github.Repository
	for {
		repos, resp, err := c.client.Repositories.ListByUser(ctx, username, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, repos...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].GetPushedAt().After(all[j].GetPushedAt().Time)
	})
	return all, nil
}

func (c *Crawler) crawlRepo(ctx context.Context, username string, repo *github.Repository) (RepoData, error) {
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()
	slog.Debug("crawling repo", "repo", repo.GetFullName())

	rd := RepoData{
		Name:          name,
		FullName:      repo.GetFullName(),
		Description:   repo.GetDescription(),
		Language:      repo.GetLanguage(),
		Stars:         repo.GetStargazersCount(),
		Forks:         repo.GetForksCount(),
		Topics:        repo.Topics,
		IsOwner:       strings.EqualFold(owner, username),
		IsFork:        repo.GetFork(),
		Archived:      repo.GetArchived(),
		DefaultBranch: repo.GetDefaultBranch(),
		OpenIssues:    repo.GetOpenIssuesCount(),
		CreatedAt:     repo.GetCreatedAt().Time,
		UpdatedAt:     repo.GetUpdatedAt().Time,
	}
	if repo.GetLicense() != nil {
		rd.License = repo.GetLicense().GetSPDXID()
	}

	langs, _, err := c.client.Repositories.ListLanguages(ctx, owner, name)
	if err == nil {
		rd.Languages = langs
	}

	rd.Commits = c.fetchCommits(ctx, owner, name, username)
	rd.PRs = c.fetchPRs(ctx, owner, name, username)
	rd.ReviewComments = c.fetchReviewComments(ctx, owner, name, username)
	if len(rd.ReviewComments) == 0 {
		slog.Debug("no line-level review comments, trying PR conversation comments", "repo", repo.GetFullName())
		rd.PRComments = c.fetchPRConversationComments(ctx, owner, name, username)
	}
	rd.CodeSamples = c.fetchCodeSamples(ctx, owner, name)
	rd.Releases = c.fetchReleases(ctx, owner, name, username)

	return rd, nil
}

func (c *Crawler) fetchCommits(ctx context.Context, owner, repo, author string) []CommitData {
	// Fetch all available commits (up to maxCommitsPerRepo) to get the full
	// timeline, then sample evenly across the history for patch details so
	// we capture how the developer's style evolved over time.
	opts := &github.CommitsListOptions{
		Author:      author,
		ListOptions: github.ListOptions{PerPage: maxCommitsPerRepo},
	}

	commits, _, err := c.client.Repositories.ListCommits(ctx, owner, repo, opts)
	if err != nil {
		slog.Debug("could not list commits", "repo", owner+"/"+repo, "error", err)
		return nil
	}

	// Pick evenly spaced indices for patch fetching (20 patches spread
	// across the full commit list instead of only the most recent 20).
	const maxPatches = 20
	patchIndices := spreadIndices(len(commits), maxPatches)
	patchSet := make(map[int]bool, len(patchIndices))
	for _, i := range patchIndices {
		patchSet[i] = true
	}

	var result []CommitData
	for i, cm := range commits {
		cd := CommitData{
			SHA:     cm.GetSHA(),
			Message: cm.GetCommit().GetMessage(),
			Date:    cm.GetCommit().GetAuthor().GetDate().Time,
		}

		if patchSet[i] {
			detail, _, err := c.client.Repositories.GetCommit(ctx, owner, repo, cm.GetSHA(), nil)
			if err == nil {
				cd.Patch = extractPatch(detail.Files)
				cd.Additions = detail.GetStats().GetAdditions()
				cd.Deletions = detail.GetStats().GetDeletions()
				cd.FilesChanged = len(detail.Files)
			}
		}
		result = append(result, cd)
	}
	return result
}

// spreadIndices returns up to count evenly spaced indices across [0, total).
func spreadIndices(total, count int) []int {
	if total <= 0 {
		return nil
	}
	if count >= total {
		indices := make([]int, total)
		for i := range indices {
			indices[i] = i
		}
		return indices
	}
	indices := make([]int, count)
	step := float64(total-1) / float64(count-1)
	for i := range indices {
		indices[i] = int(float64(i) * step)
	}
	return indices
}

func extractPatch(files []*github.CommitFile) string {
	var b strings.Builder
	for _, f := range files {
		patch := f.GetPatch()
		if patch == "" {
			continue
		}
		fmt.Fprintf(&b, "--- %s ---\n", f.GetFilename())
		if len(patch) > maxPatchLen {
			b.WriteString(textutil.Truncate(patch, maxPatchLen, "\n... (truncated)\n"))
		} else {
			b.WriteString(patch)
			b.WriteByte('\n')
		}
		if b.Len() > maxPatchLen*3 {
			break
		}
	}
	return b.String()
}

func (c *Crawler) fetchPRs(ctx context.Context, owner, repo, username string) []PullRequestData {
	opts := &github.PullRequestListOptions{
		State:       "all",
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: maxPRsPerRepo},
	}

	prs, _, err := c.client.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		slog.Debug("could not list PRs", "repo", owner+"/"+repo, "error", err)
		return nil
	}

	var result []PullRequestData
	for _, pr := range prs {
		if !strings.EqualFold(pr.GetUser().GetLogin(), username) {
			continue
		}
		prd := PullRequestData{
			Repo:         owner + "/" + repo,
			Number:       pr.GetNumber(),
			Title:        pr.GetTitle(),
			Body:         truncate(pr.GetBody(), 2000),
			State:        pr.GetState(),
			Date:         pr.GetCreatedAt().Time,
			Additions:    pr.GetAdditions(),
			Deletions:    pr.GetDeletions(),
			ChangedFiles: pr.GetChangedFiles(),
		}
		for _, lbl := range pr.Labels {
			prd.Labels = append(prd.Labels, lbl.GetName())
		}
		if pr.MergedAt != nil {
			t := pr.GetMergedAt().Time
			prd.MergedAt = &t
		}
		if pr.ClosedAt != nil {
			t := pr.GetClosedAt().Time
			prd.ClosedAt = &t
		}
		result = append(result, prd)
	}
	return result
}

func (c *Crawler) fetchReviewComments(ctx context.Context, owner, repo, username string) []ReviewComment {
	opts := &github.PullRequestListCommentsOptions{
		Sort:        "created",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	comments, _, err := c.client.PullRequests.ListComments(ctx, owner, repo, 0, opts)
	if err != nil {
		slog.Debug("could not list review comments", "repo", owner+"/"+repo, "error", err)
		return nil
	}

	var result []ReviewComment
	for _, cm := range comments {
		if !strings.EqualFold(cm.GetUser().GetLogin(), username) {
			continue
		}
		result = append(result, ReviewComment{
			Repo:     owner + "/" + repo,
			Body:     truncate(cm.GetBody(), 1000),
			Path:     cm.GetPath(),
			DiffHunk: truncate(cm.GetDiffHunk(), 2000),
			Date:     cm.GetCreatedAt().Time,
		})
		if len(result) >= maxReviewsPerPR {
			break
		}
	}
	return result
}

func (c *Crawler) fetchPRConversationComments(ctx context.Context, owner, repo, username string) []Comment {
	opts := &github.PullRequestListOptions{
		State:       "all",
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: maxPRsPerRepo},
	}

	prs, _, err := c.client.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		slog.Debug("could not list PRs for conversation comments", "repo", owner+"/"+repo, "error", err)
		return nil
	}

	var result []Comment
	for _, pr := range prs {
		if strings.EqualFold(pr.GetUser().GetLogin(), username) {
			continue
		}
		if len(result) >= maxReviewsPerPR {
			break
		}
		comments, _, err := c.client.Issues.ListComments(ctx, owner, repo, pr.GetNumber(), &github.IssueListCommentsOptions{
			Sort:        github.String("created"),
			Direction:   github.String("desc"),
			ListOptions: github.ListOptions{PerPage: 30},
		})
		if err != nil {
			continue
		}
		for _, cm := range comments {
			if !strings.EqualFold(cm.GetUser().GetLogin(), username) {
				continue
			}
			result = append(result, Comment{
				Repo: owner + "/" + repo,
				Body: truncate(cm.GetBody(), 1000),
				URL:  cm.GetHTMLURL(),
				Date: cm.GetCreatedAt().Time,
			})
			if len(result) >= maxReviewsPerPR {
				break
			}
		}
	}
	return result
}

func (c *Crawler) fetchCodeSamples(ctx context.Context, owner, repo string) []CodeSample {
	tree, _, err := c.client.Git.GetTree(ctx, owner, repo, "HEAD", true)
	if err != nil {
		return nil
	}

	var candidates []string
	var workflows []string
	for _, entry := range tree.Entries {
		if entry.GetType() != "blob" {
			continue
		}
		p := entry.GetPath()
		name := path.Base(p)
		if isWorkflowFile(p) {
			if entry.GetSize() <= maxFileSizeBytes {
				workflows = append(workflows, p)
			}
			continue
		}
		if isInterestingFile(name) || isSourceFile(name) {
			if entry.GetSize() <= maxFileSizeBytes {
				candidates = append(candidates, p)
			}
		}
	}

	var samples []CodeSample
	for _, p := range workflows {
		if len(samples) >= maxCodeSamples+3 {
			break
		}
		fileContent, _, _, err := c.client.Repositories.GetContents(ctx, owner, repo, p, nil)
		if err != nil || fileContent == nil {
			continue
		}
		content, err := fileContent.GetContent()
		if err != nil {
			continue
		}
		samples = append(samples, CodeSample{Path: p, Content: content})
	}

	for _, p := range candidates {
		if len(samples) >= maxCodeSamples+3 {
			break
		}
		fileContent, _, _, err := c.client.Repositories.GetContents(ctx, owner, repo, p, nil)
		if err != nil || fileContent == nil {
			continue
		}
		content, err := fileContent.GetContent()
		if err != nil {
			continue
		}
		samples = append(samples, CodeSample{Path: p, Content: content})
	}
	return samples
}

func (c *Crawler) fetchReleases(ctx context.Context, owner, repo, username string) []ReleaseData {
	opts := &github.ListOptions{PerPage: 30}
	releases, _, err := c.client.Repositories.ListReleases(ctx, owner, repo, opts)
	if err != nil {
		slog.Debug("could not list releases", "repo", owner+"/"+repo, "error", err)
		return nil
	}

	var result []ReleaseData
	for _, rel := range releases {
		if !strings.EqualFold(rel.GetAuthor().GetLogin(), username) {
			continue
		}
		result = append(result, ReleaseData{
			Repo:      owner + "/" + repo,
			TagName:   rel.GetTagName(),
			Name:      rel.GetName(),
			Body:      truncate(rel.GetBody(), 2000),
			CreatedAt: rel.GetCreatedAt().Time,
		})
	}
	return result
}

func (c *Crawler) fetchExternalReviews(ctx context.Context, username string, crawledRepos map[string]bool) ([]RepoData, error) {
	query := fmt.Sprintf("commenter:%s is:pr -user:%s", username, username)
	searchOpts := &github.SearchOptions{
		Sort:        "updated",
		Order:       "desc",
		ListOptions: github.ListOptions{PerPage: 30},
	}

	issues, _, err := c.client.Search.Issues(ctx, query, searchOpts)
	if err != nil {
		return nil, err
	}

	type prRef struct {
		owner  string
		repo   string
		number int
	}

	repoToPRs := make(map[string][]prRef)
	for _, issue := range issues.Issues {
		owner, repo, err := ownerRepoFromURL(issue.GetRepositoryURL())
		if err != nil {
			continue
		}
		fullName := owner + "/" + repo
		if crawledRepos[fullName] {
			continue
		}
		repoToPRs[fullName] = append(repoToPRs[fullName], prRef{owner, repo, issue.GetNumber()})
	}

	var result []RepoData
	for fullName, refs := range repoToPRs {
		rd := RepoData{
			Name:     refs[0].repo,
			FullName: fullName,
			IsOwner:  false,
		}

		for _, ref := range refs {
			rc, _, err := c.client.PullRequests.ListComments(ctx, ref.owner, ref.repo, ref.number, &github.PullRequestListCommentsOptions{
				Sort:        "created",
				Direction:   "desc",
				ListOptions: github.ListOptions{PerPage: 50},
			})
			if err == nil {
				for _, cm := range rc {
					if !strings.EqualFold(cm.GetUser().GetLogin(), username) {
						continue
					}
					rd.ReviewComments = append(rd.ReviewComments, ReviewComment{
						Repo:     fullName,
						Body:     truncate(cm.GetBody(), 1000),
						Path:     cm.GetPath(),
						DiffHunk: truncate(cm.GetDiffHunk(), 2000),
						Date:     cm.GetCreatedAt().Time,
					})
					if len(rd.ReviewComments) >= maxReviewsPerPR {
						break
					}
				}
			}
			if len(rd.ReviewComments) >= maxReviewsPerPR {
				break
			}

			ic, _, err := c.client.Issues.ListComments(ctx, ref.owner, ref.repo, ref.number, &github.IssueListCommentsOptions{
				Sort:        github.String("created"),
				Direction:   github.String("desc"),
				ListOptions: github.ListOptions{PerPage: 30},
			})
			if err == nil {
				for _, cm := range ic {
					if !strings.EqualFold(cm.GetUser().GetLogin(), username) {
						continue
					}
					rd.PRComments = append(rd.PRComments, Comment{
						Repo: fullName,
						Body: truncate(cm.GetBody(), 1000),
						URL:  cm.GetHTMLURL(),
						Date: cm.GetCreatedAt().Time,
					})
					if len(rd.PRComments) >= maxReviewsPerPR {
						break
					}
				}
			}
			if len(rd.PRComments) >= maxReviewsPerPR {
				break
			}
		}

		if len(rd.ReviewComments) > 0 || len(rd.PRComments) > 0 {
			result = append(result, rd)
		}
	}

	return result, nil
}

func (c *Crawler) fetchIssueComments(ctx context.Context, username string) ([]Comment, error) {
	query := fmt.Sprintf("commenter:%s", username)
	searchOpts := &github.SearchOptions{
		Sort:        "updated",
		Order:       "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var allComments []Comment
	for {
		issues, resp, err := c.client.Search.Issues(ctx, query, searchOpts)
		if err != nil {
			return allComments, err
		}
		for _, issue := range issues.Issues {
			if len(allComments) >= maxIssueComments {
				return allComments, nil
			}
			owner, repo, err := ownerRepoFromURL(issue.GetRepositoryURL())
			if err != nil {
				continue
			}

			opts := &github.IssueListCommentsOptions{
				Sort:        github.String("created"),
				Direction:   github.String("desc"),
				ListOptions: github.ListOptions{PerPage: 100},
			}
			comments, _, err := c.client.Issues.ListComments(ctx, owner, repo, issue.GetNumber(), opts)
			if err != nil {
				continue
			}
			for _, cm := range comments {
				if strings.EqualFold(cm.GetUser().GetLogin(), username) {
					allComments = append(allComments, Comment{
						Repo: owner + "/" + repo,
						Body: truncate(cm.GetBody(), 1000),
						URL:  cm.GetHTMLURL(),
						Date: cm.GetCreatedAt().Time,
					})
				}
			}
		}
		if resp.NextPage == 0 || len(allComments) >= maxIssueComments {
			break
		}
		searchOpts.Page = resp.NextPage
	}

	return allComments, nil
}

func (c *Crawler) fetchStarredRepos(ctx context.Context, username string) ([]StarredRepo, error) {
	opts := &github.ActivityListStarredOptions{
		Sort:        "created",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var result []StarredRepo
	for {
		starred, resp, err := c.client.Activity.ListStarred(ctx, username, opts)
		if err != nil {
			return result, err
		}
		for _, sr := range starred {
			repo := sr.GetRepository()
			result = append(result, StarredRepo{
				Name:        repo.GetName(),
				FullName:    repo.GetFullName(),
				Description: truncate(repo.GetDescription(), 500),
				Language:    repo.GetLanguage(),
				Topics:      repo.Topics,
				Stars:       repo.GetStargazersCount(),
			})
			if len(result) >= maxStarredRepos {
				return result, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result, nil
}

func (c *Crawler) fetchGists(ctx context.Context, username string) ([]GistData, error) {
	opts := &github.GistListOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var result []GistData
	for {
		gists, resp, err := c.client.Gists.List(ctx, username, opts)
		if err != nil {
			return result, err
		}
		for _, g := range gists {
			gd := GistData{
				ID:          g.GetID(),
				Description: truncate(g.GetDescription(), 500),
				Public:      g.GetPublic(),
				CreatedAt:   g.GetCreatedAt().Time,
				UpdatedAt:   g.GetUpdatedAt().Time,
			}
			for name, f := range g.Files {
				gd.Files = append(gd.Files, GistFile{
					Name:     string(name),
					Language: f.GetLanguage(),
				})
			}
			result = append(result, gd)
			if len(result) >= maxGists {
				return result, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result, nil
}

func (c *Crawler) fetchOrgs(ctx context.Context, username string) ([]string, error) {
	opts := &github.ListOptions{PerPage: 100}
	orgs, _, err := c.client.Organizations.List(ctx, username, opts)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, org := range orgs {
		result = append(result, org.GetLogin())
	}
	return result, nil
}

func (c *Crawler) fetchEvents(ctx context.Context, username string) ([]EventData, error) {
	opts := &github.ListOptions{PerPage: 100}

	var result []EventData
	for {
		events, resp, err := c.client.Activity.ListEventsPerformedByUser(ctx, username, true, opts)
		if err != nil {
			return result, err
		}
		for _, ev := range events {
			result = append(result, EventData{
				Type:      ev.GetType(),
				Repo:      ev.GetRepo().GetName(),
				CreatedAt: ev.GetCreatedAt().Time,
				Summary:   eventSummary(ev),
			})
			if len(result) >= maxEvents {
				return result, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result, nil
}

func eventSummary(ev *github.Event) string {
	switch ev.GetType() {
	case "PushEvent":
		return fmt.Sprintf("pushed to %s", ev.GetRepo().GetName())
	case "CreateEvent":
		return fmt.Sprintf("created in %s", ev.GetRepo().GetName())
	case "DeleteEvent":
		return fmt.Sprintf("deleted in %s", ev.GetRepo().GetName())
	case "ForkEvent":
		return fmt.Sprintf("forked %s", ev.GetRepo().GetName())
	case "IssuesEvent":
		return fmt.Sprintf("issue activity in %s", ev.GetRepo().GetName())
	case "IssueCommentEvent":
		return fmt.Sprintf("commented on issue in %s", ev.GetRepo().GetName())
	case "PullRequestEvent":
		return fmt.Sprintf("PR activity in %s", ev.GetRepo().GetName())
	case "PullRequestReviewEvent":
		return fmt.Sprintf("reviewed PR in %s", ev.GetRepo().GetName())
	case "PullRequestReviewCommentEvent":
		return fmt.Sprintf("review comment in %s", ev.GetRepo().GetName())
	case "WatchEvent":
		return fmt.Sprintf("starred %s", ev.GetRepo().GetName())
	case "ReleaseEvent":
		return fmt.Sprintf("release in %s", ev.GetRepo().GetName())
	default:
		return ev.GetType()
	}
}

func (c *Crawler) fetchAuthoredIssues(ctx context.Context, username string) ([]IssueData, error) {
	query := fmt.Sprintf("author:%s is:issue", username)
	searchOpts := &github.SearchOptions{
		Sort:        "created",
		Order:       "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var result []IssueData
	for {
		issues, resp, err := c.client.Search.Issues(ctx, query, searchOpts)
		if err != nil {
			return result, err
		}
		for _, issue := range issues.Issues {
			owner, repo, err := ownerRepoFromURL(issue.GetRepositoryURL())
			if err != nil {
				continue
			}
			id := IssueData{
				Repo:      owner + "/" + repo,
				Number:    issue.GetNumber(),
				Title:     issue.GetTitle(),
				Body:      truncate(issue.GetBody(), 2000),
				State:     issue.GetState(),
				CreatedAt: issue.GetCreatedAt().Time,
			}
			for _, lbl := range issue.Labels {
				id.Labels = append(id.Labels, lbl.GetName())
			}
			result = append(result, id)
			if len(result) >= maxSearchResults {
				return result, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		searchOpts.Page = resp.NextPage
	}
	return result, nil
}

func (c *Crawler) fetchExternalPRs(ctx context.Context, username string) ([]PullRequestData, error) {
	query := fmt.Sprintf("author:%s is:pr -user:%s", username, username)
	searchOpts := &github.SearchOptions{
		Sort:        "created",
		Order:       "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var result []PullRequestData
	for {
		issues, resp, err := c.client.Search.Issues(ctx, query, searchOpts)
		if err != nil {
			return result, err
		}
		for _, issue := range issues.Issues {
			owner, repo, err := ownerRepoFromURL(issue.GetRepositoryURL())
			if err != nil {
				continue
			}
			prd := PullRequestData{
				Repo:   owner + "/" + repo,
				Number: issue.GetNumber(),
				Title:  issue.GetTitle(),
				Body:   truncate(issue.GetBody(), 2000),
				State:  issue.GetState(),
				Date:   issue.GetCreatedAt().Time,
			}
			for _, lbl := range issue.Labels {
				prd.Labels = append(prd.Labels, lbl.GetName())
			}
			if issue.ClosedAt != nil {
				t := issue.GetClosedAt().Time
				prd.ClosedAt = &t
			}
			if issue.PullRequestLinks != nil {
				pr, _, err := c.client.PullRequests.Get(ctx, owner, repo, issue.GetNumber())
				if err == nil {
					prd.Additions = pr.GetAdditions()
					prd.Deletions = pr.GetDeletions()
					prd.ChangedFiles = pr.GetChangedFiles()
					if pr.MergedAt != nil {
						t := pr.GetMergedAt().Time
						prd.MergedAt = &t
					}
				}
			}
			result = append(result, prd)
			if len(result) >= maxSearchResults {
				return result, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		searchOpts.Page = resp.NextPage
	}
	return result, nil
}

// selectDiverseRepos picks repos that maximize coverage across the developer's
// full history instead of just the N most recently pushed. It ensures:
//   - Every language the developer uses is represented
//   - Both old and recent repos are included (temporal spread)
//   - Owned repos are preferred over forks
//   - Higher-activity repos (stars, forks) are preferred within each group
func selectDiverseRepos(repos []*github.Repository, maxRepos int, username string) []*github.Repository {
	if len(repos) <= maxRepos {
		return repos
	}

	selected := make(map[int]bool)

	// Group owned (non-fork) repos by primary language.
	langGroups := make(map[string][]int)
	for i, r := range repos {
		if r.GetFork() || !strings.EqualFold(r.GetOwner().GetLogin(), username) {
			continue
		}
		lang := r.GetLanguage()
		if lang == "" {
			lang = "_none"
		}
		langGroups[lang] = append(langGroups[lang], i)
	}

	// Round-robin one repo per language until we've used half the budget.
	// This guarantees language diversity.
	langBudget := maxRepos / 2
	if langBudget < 1 {
		langBudget = 1
	}
	for round := 0; len(selected) < langBudget; round++ {
		added := false
		for _, indices := range langGroups {
			if round < len(indices) && len(selected) < langBudget {
				selected[indices[round]] = true
				added = true
			}
		}
		if !added {
			break
		}
	}

	// Fill remaining budget with temporal diversity: sort unselected owned
	// repos by creation date (oldest first) and pick evenly spaced ones.
	var unselected []int
	for i, r := range repos {
		if selected[i] || r.GetFork() {
			continue
		}
		if !strings.EqualFold(r.GetOwner().GetLogin(), username) {
			continue
		}
		unselected = append(unselected, i)
	}
	// Sort by creation date ascending (oldest first).
	sort.Slice(unselected, func(a, b int) bool {
		return repos[unselected[a]].GetCreatedAt().Before(repos[unselected[b]].GetCreatedAt().Time)
	})
	remaining := maxRepos - len(selected)
	if remaining > 0 && len(unselected) > 0 {
		step := len(unselected) / remaining
		if step < 1 {
			step = 1
		}
		for i := 0; i < len(unselected) && len(selected) < maxRepos; i += step {
			selected[unselected[i]] = true
		}
	}

	// If still under budget, add forks / collaborator repos by activity.
	if len(selected) < maxRepos {
		var forks []int
		for i, r := range repos {
			if selected[i] {
				continue
			}
			if r.GetFork() || !strings.EqualFold(r.GetOwner().GetLogin(), username) {
				forks = append(forks, i)
			}
		}
		sort.Slice(forks, func(a, b int) bool {
			return repos[forks[a]].GetStargazersCount() > repos[forks[b]].GetStargazersCount()
		})
		for _, i := range forks {
			if len(selected) >= maxRepos {
				break
			}
			selected[i] = true
		}
	}

	result := make([]*github.Repository, 0, len(selected))
	for i := range repos {
		if selected[i] {
			result = append(result, repos[i])
		}
	}
	return result
}

func ownerRepoFromURL(rawURL string) (owner, repo string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("unexpected repository URL path: %s", u.Path)
	}
	return parts[len(parts)-2], parts[len(parts)-1], nil
}

var interestingFiles = map[string]bool{
	"main.go": true, "main.py": true, "main.rs": true, "main.ts": true, "main.js": true,
	"app.go": true, "app.py": true, "app.ts": true, "app.js": true,
	"makefile": true, "dockerfile": true, "justfile": true,
}

func isInterestingFile(name string) bool {
	return interestingFiles[strings.ToLower(name)]
}

var sourceExts = map[string]bool{
	".go": true, ".py": true, ".rs": true, ".ts": true, ".js": true,
	".java": true, ".rb": true, ".c": true, ".cpp": true, ".h": true,
}

func isSourceFile(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	return sourceExts[ext]
}

func isWorkflowFile(p string) bool {
	return strings.HasPrefix(p, ".github/workflows/") &&
		(strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml"))
}

func truncate(s string, max int) string {
	return textutil.Truncate(s, max, "...")
}
