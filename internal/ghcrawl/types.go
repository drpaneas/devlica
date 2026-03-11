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
	Discussions    []DiscussionData
	Projects       []ProjectData
}

// TotalCommits returns the sum of commits across all repos.
func (r *CrawlResult) TotalCommits() int {
	n := 0
	for _, repo := range r.Repos {
		n += len(repo.Commits)
	}
	return n
}

// TotalReviews returns the sum of review artifacts across all repos.
// It counts review summaries, line comments, and falls back to PR conversation
// comments when no richer review data exists for a repo.
func (r *CrawlResult) TotalReviews() int {
	n := 0
	for _, repo := range r.Repos {
		if len(repo.Reviews) > 0 || len(repo.ReviewComments) > 0 {
			n += len(repo.Reviews)
			n += len(repo.ReviewComments)
		} else {
			n += len(repo.PRComments)
		}
	}
	return n
}

func (r *CrawlResult) TotalIssues() int  { return len(r.AuthoredIssues) }
func (r *CrawlResult) TotalStarred() int { return len(r.StarredRepos) }
func (r *CrawlResult) TotalGists() int   { return len(r.Gists) }
func (r *CrawlResult) TotalReleases() int {
	n := 0
	for _, repo := range r.Repos {
		n += len(repo.Releases)
	}
	return n
}
func (r *CrawlResult) TotalExternalPRs() int { return len(r.ExternalPRs) }
func (r *CrawlResult) TotalDiscussions() int { return len(r.Discussions) }
func (r *CrawlResult) TotalProjects() int    { return len(r.Projects) }

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
	Reviews        []ReviewData
	ReviewComments []ReviewComment
	PRComments     []Comment
	CodeSamples    []CodeSample
	Releases       []ReleaseData
	WikiPages      []WikiPage
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
	URL            string
	Title          string
	Body           string
	Author         string
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

// ReviewData holds metadata for a submitted PR review.
type ReviewData struct {
	Repo               string
	PRNumber           int
	PRTitle            string
	PRAuthor           string
	Body               string
	State              string
	SubmittedAt        time.Time
	CommitID           string
	URL                string
	Labels             []string
	Additions          int
	Deletions          int
	ChangedFiles       int
	ReviewCommentCount int
}

// ReviewComment holds a single PR review comment.
type ReviewComment struct {
	Repo     string
	PRNumber int
	PRTitle  string
	PRAuthor string
	Body     string
	Path     string
	DiffHunk string
	URL      string
	Date     time.Time
}

// Comment holds an issue or PR conversation comment.
type Comment struct {
	Repo   string
	Author string
	Body   string
	URL    string
	Date   time.Time
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
	Content  string
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

// DiscussionData holds metadata for a GitHub discussion.
type DiscussionData struct {
	Repo      string
	Number    int
	Title     string
	Body      string
	Category  string
	Author    string
	URL       string
	CreatedAt time.Time
	Comments  []Comment
}

// ProjectData holds metadata for a GitHub Projects v2 project.
type ProjectData struct {
	Title     string
	Body      string
	URL       string
	Public    bool
	CreatedAt time.Time
	ItemCount int
}

// WikiPage holds the title and content of a repository wiki page.
type WikiPage struct {
	Repo    string
	Title   string
	Content string
}
