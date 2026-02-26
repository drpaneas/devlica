package ghcrawl

import "testing"

func TestCrawlResult_TotalCommits(t *testing.T) {
	r := &CrawlResult{
		Repos: []RepoData{
			{Commits: make([]CommitData, 5)},
			{Commits: make([]CommitData, 3)},
			{Commits: nil},
		},
	}
	if got := r.TotalCommits(); got != 8 {
		t.Errorf("TotalCommits() = %d, want 8", got)
	}
}

func TestCrawlResult_TotalReviews(t *testing.T) {
	r := &CrawlResult{
		Repos: []RepoData{
			{ReviewComments: make([]ReviewComment, 10)},
			{ReviewComments: make([]ReviewComment, 2)},
		},
	}
	if got := r.TotalReviews(); got != 12 {
		t.Errorf("TotalReviews() = %d, want 12", got)
	}
}

func TestCrawlResult_TotalReviews_FallbackToPRComments(t *testing.T) {
	r := &CrawlResult{
		Repos: []RepoData{
			{PRComments: make([]Comment, 7)},
			{ReviewComments: make([]ReviewComment, 3)},
		},
	}
	if got := r.TotalReviews(); got != 10 {
		t.Errorf("TotalReviews() = %d, want 10", got)
	}
}

func TestCrawlResult_TotalIssues(t *testing.T) {
	r := &CrawlResult{
		AuthoredIssues: make([]IssueData, 15),
	}
	if got := r.TotalIssues(); got != 15 {
		t.Errorf("TotalIssues() = %d, want 15", got)
	}
}

func TestCrawlResult_TotalStarred(t *testing.T) {
	r := &CrawlResult{
		StarredRepos: make([]StarredRepo, 42),
	}
	if got := r.TotalStarred(); got != 42 {
		t.Errorf("TotalStarred() = %d, want 42", got)
	}
}

func TestCrawlResult_TotalGists(t *testing.T) {
	r := &CrawlResult{
		Gists: make([]GistData, 9),
	}
	if got := r.TotalGists(); got != 9 {
		t.Errorf("TotalGists() = %d, want 9", got)
	}
}

func TestCrawlResult_TotalReleases(t *testing.T) {
	r := &CrawlResult{
		Repos: []RepoData{
			{Releases: make([]ReleaseData, 3)},
			{Releases: make([]ReleaseData, 1)},
		},
	}
	if got := r.TotalReleases(); got != 4 {
		t.Errorf("TotalReleases() = %d, want 4", got)
	}
}

func TestCrawlResult_TotalExternalPRs(t *testing.T) {
	r := &CrawlResult{
		ExternalPRs: make([]PullRequestData, 11),
	}
	if got := r.TotalExternalPRs(); got != 11 {
		t.Errorf("TotalExternalPRs() = %d, want 11", got)
	}
}

func TestCrawlResult_Zeros(t *testing.T) {
	r := &CrawlResult{}
	if got := r.TotalCommits(); got != 0 {
		t.Errorf("TotalCommits() = %d, want 0", got)
	}
	if got := r.TotalReviews(); got != 0 {
		t.Errorf("TotalReviews() = %d, want 0", got)
	}
	if got := r.TotalIssues(); got != 0 {
		t.Errorf("TotalIssues() = %d, want 0", got)
	}
	if got := r.TotalStarred(); got != 0 {
		t.Errorf("TotalStarred() = %d, want 0", got)
	}
	if got := r.TotalGists(); got != 0 {
		t.Errorf("TotalGists() = %d, want 0", got)
	}
	if got := r.TotalReleases(); got != 0 {
		t.Errorf("TotalReleases() = %d, want 0", got)
	}
	if got := r.TotalExternalPRs(); got != 0 {
		t.Errorf("TotalExternalPRs() = %d, want 0", got)
	}
}
