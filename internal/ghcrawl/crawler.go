package ghcrawl

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/drpaneas/devlica/internal/textutil"
	"github.com/google/go-github/v68/github"
	"golang.org/x/sync/errgroup"
)

const (
	maxCommitsPerRepo = 50
	maxPRsPerRepo     = 30
	maxReviewsPerRepo = 50
	maxCodeSamples    = 5
	maxFileSizeBytes  = 32 * 1024
	maxPatchLen       = 4096
	crawlConcurrency  = 5
	maxIssueComments  = 500
	maxSearchResults  = 200
	maxStarredRepos   = 500
	maxGists          = 100
	maxEvents         = 300
	maxGistContentLen = 2000
)

// Crawler fetches a GitHub user's repositories, commits, PRs, and comments.
type Crawler struct {
	pool          *TokenPool
	gqlPool       *GraphQLPool
	privateClient *github.Client
	privateToken  string
	maxRepos      int
	exhaustive    bool
}

// NewCrawler returns a Crawler authenticated with the given tokens.
// maxRepos controls how many repos get deep-crawled (commits, PRs, code samples).
// privateToken is optional; when set it enables fetching private repos via the
// authenticated user's /user/repos endpoint.
func NewCrawler(tokens []string, privateToken string, maxRepos int, exhaustive bool) *Crawler {
	c := &Crawler{
		pool:         NewTokenPool(tokens),
		gqlPool:      NewGraphQLPool(tokens),
		privateToken: privateToken,
		maxRepos:     maxRepos,
		exhaustive:   exhaustive,
	}
	if privateToken != "" {
		c.privateClient = newGitHubClient(privateToken)
	}
	return c
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

	// In exhaustive mode, deep-crawl all repos. Otherwise select a diverse
	// subset to keep runtime bounded.
	deepCrawl := repos
	if !c.exhaustive {
		// Select a diverse set of repos for deep-crawling, ensuring coverage
		// across languages, time periods, and activity levels rather than
		// just the most recently pushed repos.
		deepCrawl = selectDiverseRepos(repos, c.maxRepos, username)
	}

	deepCrawled := make(map[string]bool, len(deepCrawl))
	for _, r := range deepCrawl {
		deepCrawled[r.GetFullName()] = true
	}

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

	// Include repos not selected for deep-crawling as metadata-only.
	for _, repo := range repos {
		if deepCrawled[repo.GetFullName()] {
			continue
		}
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

	// Always search external reviews (not just when owned repos have zero).
	crawledRepos := make(map[string]bool, len(result.Repos))
	for _, r := range result.Repos {
		crawledRepos[r.FullName] = true
	}
	since := result.User.CreatedAt
	extRepos, err := c.fetchExternalReviews(ctx, username, crawledRepos, since)
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

	// Fetch independent data sources concurrently. Each source handles
	// its own errors (logging warnings), so a WaitGroup suffices.
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		comments, err := c.fetchIssueComments(ctx, username, since)
		if err != nil {
			slog.Warn("could not fetch issue comments", "error", err)
		} else {
			mu.Lock()
			result.IssueComments = comments
			mu.Unlock()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		starred, err := c.fetchStarredRepos(ctx, username)
		if err != nil {
			slog.Warn("could not fetch starred repos", "error", err)
		} else {
			mu.Lock()
			result.StarredRepos = starred
			mu.Unlock()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		gists, err := c.fetchGists(ctx, username)
		if err != nil {
			slog.Warn("could not fetch gists", "error", err)
		} else {
			mu.Lock()
			result.Gists = gists
			mu.Unlock()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		orgs, err := c.fetchOrgs(ctx, username)
		if err != nil {
			slog.Warn("could not fetch orgs", "error", err)
		} else {
			mu.Lock()
			result.Orgs = orgs
			mu.Unlock()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		events, err := c.fetchEvents(ctx, username)
		if err != nil {
			slog.Warn("could not fetch events", "error", err)
		} else {
			mu.Lock()
			result.Events = events
			mu.Unlock()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		issues, err := c.fetchAuthoredIssues(ctx, username, since)
		if err != nil {
			slog.Warn("could not fetch authored issues", "error", err)
		} else {
			mu.Lock()
			result.AuthoredIssues = issues
			mu.Unlock()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		extPRs, err := c.fetchExternalPRs(ctx, username, since)
		if err != nil {
			slog.Warn("could not fetch external PRs", "error", err)
		} else {
			mu.Lock()
			result.ExternalPRs = extPRs
			mu.Unlock()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		discussions := c.fetchDiscussions(ctx, username, result.Repos)
		mu.Lock()
		result.Discussions = discussions
		mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		projects := c.fetchProjects(ctx, username)
		mu.Lock()
		result.Projects = projects
		mu.Unlock()
	}()

	wg.Wait()

	return result, nil
}

func (c *Crawler) fetchProfile(ctx context.Context, username string) (UserProfile, error) {
	user, _, err := c.pool.Next().Users.Get(ctx, username)
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
	readme, _, err := c.pool.Next().Repositories.GetReadme(ctx, username, username, nil)
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
		repos, resp, err := c.pool.Next().Repositories.ListByUser(ctx, username, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, repos...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	if c.privateClient != nil {
		privateRepos, err := c.fetchPrivateRepos(ctx, username)
		if err != nil {
			slog.Warn("could not fetch private repos", "error", err)
		} else {
			seen := make(map[string]bool, len(all))
			for _, r := range all {
				seen[r.GetFullName()] = true
			}
			for _, r := range privateRepos {
				if !seen[r.GetFullName()] {
					all = append(all, r)
				}
			}
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].GetPushedAt().After(all[j].GetPushedAt().Time)
	})
	return all, nil
}

// fetchPrivateRepos uses the private token to list private repos, but only
// when that token authenticates as the same user being analyzed.
func (c *Crawler) fetchPrivateRepos(ctx context.Context, username string) ([]*github.Repository, error) {
	authUser, _, err := c.privateClient.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("resolving private token identity: %w", err)
	}
	authLogin := authUser.GetLogin()
	if !privateTokenMatchesUsername(authLogin, username) {
		slog.Warn("skipping private repos: private token does not match requested user",
			"token_user", authLogin,
			"requested_user", username,
		)
		return nil, nil
	}

	opts := &github.RepositoryListByAuthenticatedUserOptions{
		Sort:        "pushed",
		Direction:   "desc",
		Visibility:  "private",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var all []*github.Repository
	for {
		repos, resp, err := c.privateClient.Repositories.ListByAuthenticatedUser(ctx, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, repos...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	slog.Info("fetched private repos", "count", len(all))
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

	langs, _, err := c.pool.Next().Repositories.ListLanguages(ctx, owner, name)
	if err == nil {
		rd.Languages = langs
	}

	repoPRs := c.fetchRepoPRs(ctx, owner, name)
	rd.Commits = c.fetchCommits(ctx, owner, name, username)
	rd.PRs = c.fetchPRs(ctx, owner, name, username, repoPRs)
	rd.Reviews = c.fetchReviews(ctx, owner, name, username, repoPRs)
	rd.ReviewComments = c.fetchReviewComments(ctx, owner, name, username, repoPRs)
	if len(rd.Reviews) == 0 && len(rd.ReviewComments) == 0 {
		slog.Debug("no submitted reviews or line comments, trying PR conversation comments", "repo", repo.GetFullName())
		rd.PRComments = c.fetchPRConversationComments(ctx, owner, name, username, repoPRs)
	}
	rd.CodeSamples = c.fetchCodeSamples(ctx, owner, name)
	rd.Releases = c.fetchReleases(ctx, owner, name, username)
	if rd.IsOwner && repo.GetHasWiki() {
		rd.WikiPages = fetchWikiPages(ctx, owner, name, c.privateToken)
	}

	return rd, nil
}

func (c *Crawler) fetchRepoPRs(ctx context.Context, owner, repo string) []*github.PullRequest {
	perPage := maxPRsPerRepo
	if c.exhaustive {
		perPage = 100
	}
	opts := &github.PullRequestListOptions{
		State:       "all",
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: perPage},
	}

	var result []*github.PullRequest
	for {
		prs, resp, err := c.pool.Next().PullRequests.List(ctx, owner, repo, opts)
		if err != nil {
			slog.Debug("could not list PRs", "repo", owner+"/"+repo, "error", err)
			return result
		}
		result = append(result, prs...)
		if !c.exhaustive || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result
}

func (c *Crawler) fetchCommits(ctx context.Context, owner, repo, author string) []CommitData {
	// In default mode, fetch recent commits (up to maxCommitsPerRepo) and
	// sample patch details. In exhaustive mode, paginate all commits and
	// fetch patch details for every commit.
	perPage := maxCommitsPerRepo
	if c.exhaustive {
		perPage = 100
	}
	opts := &github.CommitsListOptions{
		Author:      author,
		ListOptions: github.ListOptions{PerPage: perPage},
	}

	var commits []*github.RepositoryCommit
	for {
		page, resp, err := c.pool.Next().Repositories.ListCommits(ctx, owner, repo, opts)
		if err != nil {
			slog.Debug("could not list commits", "repo", owner+"/"+repo, "error", err)
			return nil
		}
		commits = append(commits, page...)
		if !c.exhaustive || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	maxPatches := 20
	if c.exhaustive {
		maxPatches = len(commits)
	}
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
			detail, _, err := c.pool.Next().Repositories.GetCommit(ctx, owner, repo, cm.GetSHA(), nil)
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
	if count == 1 {
		return []int{0}
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

func (c *Crawler) fetchPRs(ctx context.Context, owner, repo, username string, prs []*github.PullRequest) []PullRequestData {
	var result []PullRequestData
	for _, pr := range prs {
		if !strings.EqualFold(pr.GetUser().GetLogin(), username) {
			continue
		}
		detail := pr
		full, _, err := c.pool.Next().PullRequests.Get(ctx, owner, repo, pr.GetNumber())
		if err == nil {
			detail = full
		}
		prd := PullRequestData{
			Repo:         owner + "/" + repo,
			Number:       pr.GetNumber(),
			Title:        pr.GetTitle(),
			URL:          pr.GetHTMLURL(),
			Body:         truncate(pr.GetBody(), 2000),
			Author:       pr.GetUser().GetLogin(),
			State:        pr.GetState(),
			Date:         pr.GetCreatedAt().Time,
			Additions:    detail.GetAdditions(),
			Deletions:    detail.GetDeletions(),
			ChangedFiles: detail.GetChangedFiles(),
		}
		prd.Labels = prLabelNames(detail)
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

func prLabelNames(pr *github.PullRequest) []string {
	var labels []string
	for _, lbl := range pr.Labels {
		labels = append(labels, lbl.GetName())
	}
	return labels
}

func (c *Crawler) fetchReviews(ctx context.Context, owner, repo, username string, prs []*github.PullRequest) []ReviewData {
	detailCache := make(map[int]*github.PullRequest)
	loadDetail := func(number int, fallback *github.PullRequest) *github.PullRequest {
		if detail, ok := detailCache[number]; ok {
			return detail
		}
		detail, _, err := c.pool.Next().PullRequests.Get(ctx, owner, repo, number)
		if err != nil {
			detailCache[number] = fallback
			return fallback
		}
		detailCache[number] = detail
		return detail
	}

	var result []ReviewData
	limit := c.limit(maxReviewsPerRepo)
	for _, pr := range prs {
		if strings.EqualFold(pr.GetUser().GetLogin(), username) {
			continue
		}
		opts := &github.ListOptions{PerPage: 100}
		for {
			reviews, resp, err := c.pool.Next().PullRequests.ListReviews(ctx, owner, repo, pr.GetNumber(), opts)
			if err != nil {
				slog.Debug("could not list reviews", "repo", owner+"/"+repo, "number", pr.GetNumber(), "error", err)
				break
			}
			for _, review := range reviews {
				if !strings.EqualFold(review.GetUser().GetLogin(), username) {
					continue
				}
				if strings.EqualFold(review.GetState(), "PENDING") {
					continue
				}
				detail := loadDetail(pr.GetNumber(), pr)
				result = append(result, ReviewData{
					Repo:               owner + "/" + repo,
					PRNumber:           pr.GetNumber(),
					PRTitle:            pr.GetTitle(),
					PRAuthor:           pr.GetUser().GetLogin(),
					Body:               truncate(review.GetBody(), 1000),
					State:              review.GetState(),
					SubmittedAt:        review.GetSubmittedAt().Time,
					CommitID:           review.GetCommitID(),
					URL:                review.GetHTMLURL(),
					Labels:             prLabelNames(detail),
					Additions:          detail.GetAdditions(),
					Deletions:          detail.GetDeletions(),
					ChangedFiles:       detail.GetChangedFiles(),
					ReviewCommentCount: detail.GetReviewComments(),
				})
				if c.reachedLimit(len(result), limit) {
					return result
				}
			}
			if !c.exhaustive || resp.NextPage == 0 {
				break
			}
			opts.Page = resp.NextPage
		}
	}
	return result
}

func (c *Crawler) fetchReviewComments(ctx context.Context, owner, repo, username string, prs []*github.PullRequest) []ReviewComment {
	opts := &github.PullRequestListCommentsOptions{
		Sort:        "created",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	prByNumber := make(map[int]*github.PullRequest, len(prs))
	for _, pr := range prs {
		prByNumber[pr.GetNumber()] = pr
	}
	loadedByNumber := make(map[int]*github.PullRequest)
	var result []ReviewComment
	limit := c.limit(maxReviewsPerRepo)
	for {
		comments, resp, err := c.pool.Next().PullRequests.ListComments(ctx, owner, repo, 0, opts)
		if err != nil {
			slog.Debug("could not list review comments", "repo", owner+"/"+repo, "error", err)
			break
		}
		for _, cm := range comments {
			if !strings.EqualFold(cm.GetUser().GetLogin(), username) {
				continue
			}
			prNumber := pullRequestNumberFromURL(cm.GetPullRequestURL())
			prTitle := ""
			prAuthor := ""
			pr := loadPullRequest(
				prNumber,
				prByNumber,
				loadedByNumber,
				func(number int) (*github.PullRequest, error) {
					pr, _, err := c.pool.Next().PullRequests.Get(ctx, owner, repo, number)
					return pr, err
				},
			)
			if pr != nil {
				prTitle = pr.GetTitle()
				prAuthor = pr.GetUser().GetLogin()
				if strings.EqualFold(prAuthor, username) {
					continue
				}
			}
			result = append(result, ReviewComment{
				Repo:     owner + "/" + repo,
				PRNumber: prNumber,
				PRTitle:  prTitle,
				PRAuthor: prAuthor,
				Body:     truncate(cm.GetBody(), 1000),
				Path:     cm.GetPath(),
				DiffHunk: truncate(cm.GetDiffHunk(), 2000),
				URL:      cm.GetHTMLURL(),
				Date:     cm.GetCreatedAt().Time,
			})
			if c.reachedLimit(len(result), limit) {
				return result
			}
		}
		if !c.exhaustive || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result
}

func loadPullRequest(
	prNumber int,
	existing map[int]*github.PullRequest,
	loaded map[int]*github.PullRequest,
	fetch func(int) (*github.PullRequest, error),
) *github.PullRequest {
	if prNumber <= 0 {
		return nil
	}
	if pr, ok := existing[prNumber]; ok {
		return pr
	}
	if pr, ok := loaded[prNumber]; ok {
		return pr
	}
	pr, err := fetch(prNumber)
	if err != nil {
		loaded[prNumber] = nil
		return nil
	}
	loaded[prNumber] = pr
	return pr
}

func (c *Crawler) fetchPRConversationComments(ctx context.Context, owner, repo, username string, prs []*github.PullRequest) []Comment {
	var result []Comment
	limit := c.limit(maxReviewsPerRepo)
	perPage := 30
	if c.exhaustive {
		perPage = 100
	}
	for _, pr := range prs {
		if strings.EqualFold(pr.GetUser().GetLogin(), username) {
			continue
		}
		if c.reachedLimit(len(result), limit) {
			break
		}
		opts := &github.IssueListCommentsOptions{
			Sort:        github.Ptr("created"),
			Direction:   github.Ptr("desc"),
			ListOptions: github.ListOptions{PerPage: perPage},
		}
		for {
			comments, resp, err := c.pool.Next().Issues.ListComments(ctx, owner, repo, pr.GetNumber(), opts)
			if err != nil {
				break
			}
			for _, cm := range comments {
				if !strings.EqualFold(cm.GetUser().GetLogin(), username) {
					continue
				}
				result = append(result, Comment{
					Repo:   owner + "/" + repo,
					Author: cm.GetUser().GetLogin(),
					Body:   truncate(cm.GetBody(), 1000),
					URL:    cm.GetHTMLURL(),
					Date:   cm.GetCreatedAt().Time,
				})
				if c.reachedLimit(len(result), limit) {
					break
				}
			}
			if c.reachedLimit(len(result), limit) {
				break
			}
			if !c.exhaustive || resp.NextPage == 0 {
				break
			}
			opts.Page = resp.NextPage
		}
	}
	return result
}

func (c *Crawler) fetchCodeSamples(ctx context.Context, owner, repo string) []CodeSample {
	tree, _, err := c.pool.Next().Git.GetTree(ctx, owner, repo, "HEAD", true)
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
	limit := c.limit(maxCodeSamples + 3)
	for _, p := range workflows {
		if c.reachedLimit(len(samples), limit) {
			break
		}
		fileContent, _, _, err := c.pool.Next().Repositories.GetContents(ctx, owner, repo, p, nil)
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
		if c.reachedLimit(len(samples), limit) {
			break
		}
		fileContent, _, _, err := c.pool.Next().Repositories.GetContents(ctx, owner, repo, p, nil)
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
	var result []ReleaseData
	perPage := 30
	if c.exhaustive {
		perPage = 100
	}
	opts := &github.ListOptions{PerPage: perPage}
	for {
		releases, resp, err := c.pool.Next().Repositories.ListReleases(ctx, owner, repo, opts)
		if err != nil {
			slog.Debug("could not list releases", "repo", owner+"/"+repo, "error", err)
			return result
		}
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
		if !c.exhaustive || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result
}

func (c *Crawler) fetchExternalReviews(ctx context.Context, username string, crawledRepos map[string]bool, since time.Time) ([]RepoData, error) {
	query := fmt.Sprintf("commenter:%s is:pr -user:%s", username, username)

	type prRef struct {
		owner  string
		repo   string
		number int
	}

	reviewLimit := c.limit(maxReviewsPerRepo)
	repoToPRs := make(map[string][]prRef)

	if c.exhaustive {
		issues, err := c.windowedSearchIssuesWithQualifier(ctx, query, since, "updated")
		if err != nil {
			return nil, err
		}
		for _, issue := range issues {
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
	} else {
		searchOpts := &github.SearchOptions{
			Sort:        "updated",
			Order:       "desc",
			ListOptions: github.ListOptions{PerPage: 100},
		}
		searchLimit := c.limit(maxSearchResults)
		totalRefs := 0
		for {
			issues, resp, err := c.pool.Next().Search.Issues(ctx, query, searchOpts)
			if err != nil {
				return nil, err
			}
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
				totalRefs++
				if c.reachedLimit(totalRefs, searchLimit) {
					break
				}
			}
			if c.reachedLimit(totalRefs, searchLimit) || resp.NextPage == 0 {
				break
			}
			searchOpts.Page = resp.NextPage
		}
	}

	var result []RepoData
	for fullName, refs := range repoToPRs {
		rd := RepoData{
			Name:     refs[0].repo,
			FullName: fullName,
			IsOwner:  false,
		}

		for _, ref := range refs {
			pr, _, err := c.pool.Next().PullRequests.Get(ctx, ref.owner, ref.repo, ref.number)
			if err == nil && pr != nil {
				opts := &github.ListOptions{PerPage: 100}
				for {
					reviews, resp, err := c.pool.Next().PullRequests.ListReviews(ctx, ref.owner, ref.repo, ref.number, opts)
					if err != nil {
						break
					}
					for _, review := range reviews {
						if !strings.EqualFold(review.GetUser().GetLogin(), username) {
							continue
						}
						if strings.EqualFold(review.GetState(), "PENDING") {
							continue
						}
						rd.Reviews = append(rd.Reviews, ReviewData{
							Repo:               fullName,
							PRNumber:           ref.number,
							PRTitle:            pr.GetTitle(),
							PRAuthor:           pr.GetUser().GetLogin(),
							Body:               truncate(review.GetBody(), 1000),
							State:              review.GetState(),
							SubmittedAt:        review.GetSubmittedAt().Time,
							CommitID:           review.GetCommitID(),
							URL:                review.GetHTMLURL(),
							Labels:             prLabelNames(pr),
							Additions:          pr.GetAdditions(),
							Deletions:          pr.GetDeletions(),
							ChangedFiles:       pr.GetChangedFiles(),
							ReviewCommentCount: pr.GetReviewComments(),
						})
						if c.reachedLimit(len(rd.Reviews), reviewLimit) {
							break
						}
					}
					if c.reachedLimit(len(rd.Reviews), reviewLimit) || !c.exhaustive || resp.NextPage == 0 {
						break
					}
					opts.Page = resp.NextPage
				}
			}

			rcOpts := &github.PullRequestListCommentsOptions{
				Sort:        "created",
				Direction:   "desc",
				ListOptions: github.ListOptions{PerPage: 100},
			}
			for {
				rc, resp, err := c.pool.Next().PullRequests.ListComments(ctx, ref.owner, ref.repo, ref.number, rcOpts)
				if err != nil {
					break
				}
				for _, cm := range rc {
					if !strings.EqualFold(cm.GetUser().GetLogin(), username) {
						continue
					}
					rd.ReviewComments = append(rd.ReviewComments, ReviewComment{
						Repo:     fullName,
						PRNumber: ref.number,
						PRTitle:  prTitle(pr),
						PRAuthor: prAuthor(pr),
						Body:     truncate(cm.GetBody(), 1000),
						Path:     cm.GetPath(),
						DiffHunk: truncate(cm.GetDiffHunk(), 2000),
						URL:      cm.GetHTMLURL(),
						Date:     cm.GetCreatedAt().Time,
					})
					if c.reachedLimit(len(rd.ReviewComments), reviewLimit) {
						break
					}
				}
				if c.reachedLimit(len(rd.ReviewComments), reviewLimit) || !c.exhaustive || resp.NextPage == 0 {
					break
				}
				rcOpts.Page = resp.NextPage
			}

			icOpts := &github.IssueListCommentsOptions{
				Sort:        github.Ptr("created"),
				Direction:   github.Ptr("desc"),
				ListOptions: github.ListOptions{PerPage: 100},
			}
			for {
				ic, resp, err := c.pool.Next().Issues.ListComments(ctx, ref.owner, ref.repo, ref.number, icOpts)
				if err != nil {
					break
				}
				for _, cm := range ic {
					if !strings.EqualFold(cm.GetUser().GetLogin(), username) {
						continue
					}
					rd.PRComments = append(rd.PRComments, Comment{
						Repo:   fullName,
						Author: cm.GetUser().GetLogin(),
						Body:   truncate(cm.GetBody(), 1000),
						URL:    cm.GetHTMLURL(),
						Date:   cm.GetCreatedAt().Time,
					})
					if c.reachedLimit(len(rd.PRComments), reviewLimit) {
						break
					}
				}
				if c.reachedLimit(len(rd.PRComments), reviewLimit) || !c.exhaustive || resp.NextPage == 0 {
					break
				}
				icOpts.Page = resp.NextPage
			}
		}

		if len(rd.Reviews) > 0 || len(rd.ReviewComments) > 0 || len(rd.PRComments) > 0 {
			result = append(result, rd)
		}
	}

	return result, nil
}

func (c *Crawler) fetchIssueComments(ctx context.Context, username string, since time.Time) ([]Comment, error) {
	query := fmt.Sprintf("commenter:%s", username)

	var searchIssues []*github.Issue
	if c.exhaustive {
		var err error
		searchIssues, err = c.windowedSearchIssuesWithQualifier(ctx, query, since, "updated")
		if err != nil {
			return nil, err
		}
	} else {
		searchOpts := &github.SearchOptions{
			Sort:        "updated",
			Order:       "desc",
			ListOptions: github.ListOptions{PerPage: 100},
		}
		for {
			issues, resp, err := c.pool.Next().Search.Issues(ctx, query, searchOpts)
			if err != nil {
				return nil, err
			}
			searchIssues = append(searchIssues, issues.Issues...)
			if resp.NextPage == 0 || c.reachedLimit(len(searchIssues), c.limit(maxIssueComments)) {
				break
			}
			searchOpts.Page = resp.NextPage
		}
	}

	var allComments []Comment
	limit := c.limit(maxIssueComments)
	for _, issue := range searchIssues {
		if c.reachedLimit(len(allComments), limit) {
			break
		}
		owner, repo, err := ownerRepoFromURL(issue.GetRepositoryURL())
		if err != nil {
			continue
		}
		opts := &github.IssueListCommentsOptions{
			Sort:        github.Ptr("created"),
			Direction:   github.Ptr("desc"),
			ListOptions: github.ListOptions{PerPage: 100},
		}
		for {
			comments, cmResp, err := c.pool.Next().Issues.ListComments(ctx, owner, repo, issue.GetNumber(), opts)
			if err != nil {
				break
			}
			for _, cm := range comments {
				if strings.EqualFold(cm.GetUser().GetLogin(), username) {
					allComments = append(allComments, Comment{
						Repo:   owner + "/" + repo,
						Author: cm.GetUser().GetLogin(),
						Body:   truncate(cm.GetBody(), 1000),
						URL:    cm.GetHTMLURL(),
						Date:   cm.GetCreatedAt().Time,
					})
				}
				if c.reachedLimit(len(allComments), limit) {
					break
				}
			}
			if c.reachedLimit(len(allComments), limit) || !c.exhaustive || cmResp.NextPage == 0 {
				break
			}
			opts.Page = cmResp.NextPage
		}
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
	limit := c.limit(maxStarredRepos)
	for {
		starred, resp, err := c.pool.Next().Activity.ListStarred(ctx, username, opts)
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
			if c.reachedLimit(len(result), limit) {
				return result, nil
			}
		}
		if !c.exhaustive || resp.NextPage == 0 {
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
	limit := c.limit(maxGists)
	for {
		gists, resp, err := c.pool.Next().Gists.List(ctx, username, opts)
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
					Content:  truncate(f.GetContent(), maxGistContentLen),
				})
			}
			result = append(result, gd)
			if c.reachedLimit(len(result), limit) {
				return result, nil
			}
		}
		if !c.exhaustive || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result, nil
}

func (c *Crawler) fetchOrgs(ctx context.Context, username string) ([]string, error) {
	opts := &github.ListOptions{PerPage: 100}
	var result []string
	for {
		orgs, resp, err := c.pool.Next().Organizations.List(ctx, username, opts)
		if err != nil {
			return nil, err
		}
		for _, org := range orgs {
			result = append(result, org.GetLogin())
		}
		if !c.exhaustive || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result, nil
}

func (c *Crawler) fetchEvents(ctx context.Context, username string) ([]EventData, error) {
	opts := &github.ListOptions{PerPage: 100}

	var result []EventData
	limit := c.limit(maxEvents)
	for {
		events, resp, err := c.pool.Next().Activity.ListEventsPerformedByUser(ctx, username, true, opts)
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
			if c.reachedLimit(len(result), limit) {
				return result, nil
			}
		}
		if !c.exhaustive || resp.NextPage == 0 {
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

func (c *Crawler) fetchAuthoredIssues(ctx context.Context, username string, since time.Time) ([]IssueData, error) {
	query := fmt.Sprintf("author:%s is:issue", username)

	var searchIssues []*github.Issue
	if c.exhaustive {
		var err error
		searchIssues, err = c.windowedSearchIssues(ctx, query, since)
		if err != nil {
			return nil, err
		}
	} else {
		searchOpts := &github.SearchOptions{
			Sort:        "created",
			Order:       "desc",
			ListOptions: github.ListOptions{PerPage: 100},
		}
		limit := c.limit(maxSearchResults)
		for {
			issues, resp, err := c.pool.Next().Search.Issues(ctx, query, searchOpts)
			if err != nil {
				return nil, err
			}
			searchIssues = append(searchIssues, issues.Issues...)
			if resp.NextPage == 0 || c.reachedLimit(len(searchIssues), limit) {
				break
			}
			searchOpts.Page = resp.NextPage
		}
	}

	var result []IssueData
	for _, issue := range searchIssues {
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
	}
	return result, nil
}

func (c *Crawler) fetchExternalPRs(ctx context.Context, username string, since time.Time) ([]PullRequestData, error) {
	query := fmt.Sprintf("author:%s is:pr -user:%s", username, username)

	var searchIssues []*github.Issue
	if c.exhaustive {
		var err error
		searchIssues, err = c.windowedSearchIssues(ctx, query, since)
		if err != nil {
			return nil, err
		}
	} else {
		searchOpts := &github.SearchOptions{
			Sort:        "created",
			Order:       "desc",
			ListOptions: github.ListOptions{PerPage: 100},
		}
		limit := c.limit(maxSearchResults)
		for {
			issues, resp, err := c.pool.Next().Search.Issues(ctx, query, searchOpts)
			if err != nil {
				return nil, err
			}
			searchIssues = append(searchIssues, issues.Issues...)
			if resp.NextPage == 0 || c.reachedLimit(len(searchIssues), limit) {
				break
			}
			searchOpts.Page = resp.NextPage
		}
	}

	var result []PullRequestData
	for _, issue := range searchIssues {
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
			pr, _, err := c.pool.Next().PullRequests.Get(ctx, owner, repo, issue.GetNumber())
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

func pullRequestNumberFromURL(rawURL string) int {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if n, err := strconv.Atoi(parts[i]); err == nil {
			return n
		}
	}
	return 0
}

func (c *Crawler) limit(n int) int {
	if c.exhaustive {
		return 0
	}
	return n
}

func (c *Crawler) reachedLimit(current, limit int) bool {
	return limit > 0 && current >= limit
}

func prTitle(pr *github.PullRequest) string {
	if pr == nil {
		return ""
	}
	return pr.GetTitle()
}

func prAuthor(pr *github.PullRequest) string {
	if pr == nil {
		return ""
	}
	return pr.GetUser().GetLogin()
}

func privateTokenMatchesUsername(tokenLogin, requestedUsername string) bool {
	tokenLogin = strings.TrimSpace(tokenLogin)
	requestedUsername = strings.TrimSpace(requestedUsername)
	if tokenLogin == "" || requestedUsername == "" {
		return false
	}
	return strings.EqualFold(tokenLogin, requestedUsername)
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
