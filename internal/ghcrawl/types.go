package ghcrawl

import "time"

// CrawlResult holds all data collected from a user's GitHub activity.
type CrawlResult struct {
	User           UserProfile
	Repos          []RepoData
	IssueComments  []Comment
	StarredRepos   []StarredRepo
	Gists          []GistData
	Orgs           []string
	AuthoredIssues []IssueData
	ExternalPRs    []PullRequestData
	Events         []EventData
	Releases       []ReleaseData
}

// TotalCommits returns the sum of commits across all repos.
func (r *CrawlResult) TotalCommits() int {
	n := 0
	for _, repo := range r.Repos {
		n += len(repo.Commits)
	}
	return n
}

// TotalReviews returns the sum of review comments across all repos.
// It includes PR conversation comments as a fallback when no line-level
// review comments exist for a repo.
func (r *CrawlResult) TotalReviews() int {
	n := 0
	for _, repo := range r.Repos {
		if len(repo.ReviewComments) > 0 {
			n += len(repo.ReviewComments)
		} else {
			n += len(repo.PRComments)
		}
	}
	return n
}

func (r *CrawlResult) TotalIssues() int    { return len(r.AuthoredIssues) }
func (r *CrawlResult) TotalStarred() int   { return len(r.StarredRepos) }
func (r *CrawlResult) TotalGists() int     { return len(r.Gists) }
func (r *CrawlResult) TotalReleases() int  { return len(r.Releases) }
func (r *CrawlResult) TotalExternalPRs() int { return len(r.ExternalPRs) }

// UserProfile holds GitHub profile information.
type UserProfile struct {
	Login           string
	Name            string
	Bio             string
	Company         string
	Location        string
	Blog            string
	Email           string
	TwitterUsername string
	Hireable        bool
	Followers       int
	Following       int
	PublicRepos     int
	CreatedAt       time.Time
	ProfileREADME   string
}

// RepoData holds crawled data for a single repository.
type RepoData struct {
	Name           string
	FullName       string
	Description    string
	Language       string
	Languages      map[string]int
	Stars          int
	Forks          int
	Topics         []string
	IsOwner        bool
	IsFork         bool
	Archived       bool
	License        string
	DefaultBranch  string
	OpenIssues     int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Commits        []CommitData
	PRs            []PullRequestData
	ReviewComments []ReviewComment
	PRComments     []Comment
	CodeSamples    []CodeSample
	Releases       []ReleaseData
}

// CommitData holds a commit's metadata, optional diff patch, and change stats.
type CommitData struct {
	SHA          string
	Message      string
	Date         time.Time
	Patch        string
	Additions    int
	Deletions    int
	FilesChanged int
}

// PullRequestData holds metadata for a pull request.
type PullRequestData struct {
	Repo           string
	Number         int
	Title          string
	Body           string
	State          string
	Labels         []string
	Date           time.Time
	MergedAt       *time.Time
	ClosedAt       *time.Time
	Additions      int
	Deletions      int
	ChangedFiles   int
	ReviewDecision string
}

// ReviewComment holds a single PR review comment.
type ReviewComment struct {
	Repo     string
	Body     string
	Path     string
	DiffHunk string
	Date     time.Time
}

// Comment holds an issue or PR conversation comment.
type Comment struct {
	Repo string
	Body string
	URL  string
	Date time.Time
}

// CodeSample holds a source file's path and content.
type CodeSample struct {
	Path    string
	Content string
}

// StarredRepo holds metadata for a repository the user has starred.
type StarredRepo struct {
	Name        string
	FullName    string
	Description string
	Language    string
	Topics      []string
	Stars       int
}

// GistData holds metadata for a user's gist.
type GistData struct {
	ID          string
	Description string
	Files       []GistFile
	Public      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// GistFile holds the name and language of a single file within a gist.
type GistFile struct {
	Name     string
	Language string
}

// IssueData holds metadata for an issue authored by the user.
type IssueData struct {
	Repo      string
	Number    int
	Title     string
	Body      string
	State     string
	Labels    []string
	CreatedAt time.Time
}

// EventData holds a single GitHub event from the user's activity timeline.
type EventData struct {
	Type      string
	Repo      string
	CreatedAt time.Time
	Summary   string
}

// ReleaseData holds metadata for a release authored by the user.
type ReleaseData struct {
	Repo      string
	TagName   string
	Name      string
	Body      string
	CreatedAt time.Time
}
