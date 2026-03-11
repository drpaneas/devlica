package ghcrawl

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/shurcooL/githubv4"
)

// GraphQLPool distributes GraphQL requests across multiple authenticated clients.
type GraphQLPool struct {
	clients []*githubv4.Client
	counter atomic.Uint64
}

// NewGraphQLPool creates a pool of GitHub GraphQL clients, one per token.
func NewGraphQLPool(tokens []string) *GraphQLPool {
	if len(tokens) == 0 {
		return &GraphQLPool{clients: []*githubv4.Client{githubv4.NewClient(newGitHubHTTPClient(""))}}
	}
	clients := make([]*githubv4.Client, len(tokens))
	for i, tok := range tokens {
		clients[i] = githubv4.NewClient(newGitHubHTTPClient(tok))
	}
	return &GraphQLPool{clients: clients}
}

// Next returns the next client in round-robin order.
func (p *GraphQLPool) Next() *githubv4.Client {
	if len(p.clients) == 0 {
		return githubv4.NewClient(newGitHubHTTPClient(""))
	}
	idx := p.counter.Add(1) - 1
	return p.clients[idx%uint64(len(p.clients))]
}

func (c *Crawler) fetchDiscussions(ctx context.Context, username string, repos []RepoData) []DiscussionData {
	var all []DiscussionData
	for _, repo := range repos {
		if !repo.IsOwner {
			continue
		}
		parts := splitOwnerRepo(repo.FullName)
		if parts == nil {
			continue
		}
		discussions := c.fetchRepoDiscussions(ctx, parts[0], parts[1], username)
		all = append(all, discussions...)
	}
	return all
}

func (c *Crawler) fetchRepoDiscussions(ctx context.Context, owner, repo, username string) []DiscussionData {
	var query struct {
		Repository struct {
			Discussions struct {
				Nodes []struct {
					Number    int
					Title     string
					Body      string
					URL       string
					CreatedAt time.Time
					Author    struct {
						Login string
					}
					Category struct {
						Name string
					}
					Comments struct {
						Nodes []struct {
							Body      string
							CreatedAt time.Time
							Author    struct {
								Login string
							}
						}
					} `graphql:"comments(first: 10)"`
				}
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"discussions(first: 20, after: $cursor)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}

	variables := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"repo":   githubv4.String(repo),
		"cursor": (*githubv4.String)(nil),
	}

	fullName := owner + "/" + repo
	var result []DiscussionData
	for {
		err := c.gqlPool.Next().Query(ctx, &query, variables)
		if err != nil {
			slog.Debug("could not fetch discussions", "repo", fullName, "error", err)
			return result
		}
		for _, d := range query.Repository.Discussions.Nodes {
			dd := DiscussionData{
				Repo:      fullName,
				Number:    d.Number,
				Title:     d.Title,
				Body:      truncate(d.Body, 2000),
				Category:  d.Category.Name,
				Author:    d.Author.Login,
				URL:       d.URL,
				CreatedAt: d.CreatedAt,
			}
			for _, cm := range d.Comments.Nodes {
				dd.Comments = append(dd.Comments, Comment{
					Repo:   fullName,
					Author: cm.Author.Login,
					Body:   truncate(cm.Body, 1000),
					Date:   cm.CreatedAt,
				})
			}
			filtered, ok := filterDiscussionForUser(username, dd)
			if !ok {
				continue
			}
			result = append(result, filtered)
		}
		if !query.Repository.Discussions.PageInfo.HasNextPage {
			break
		}
		cursor := githubv4.String(query.Repository.Discussions.PageInfo.EndCursor)
		variables["cursor"] = &cursor
	}
	return result
}

func filterDiscussionForUser(username string, discussion DiscussionData) (DiscussionData, bool) {
	username = strings.TrimSpace(username)
	if username == "" {
		return discussion, true
	}

	authoredDiscussion := strings.EqualFold(strings.TrimSpace(discussion.Author), username)
	filteredComments := discussion.Comments[:0]
	for _, comment := range discussion.Comments {
		if strings.EqualFold(strings.TrimSpace(comment.Author), username) {
			filteredComments = append(filteredComments, comment)
		}
	}
	discussion.Comments = filteredComments

	if authoredDiscussion {
		return discussion, true
	}
	if len(discussion.Comments) == 0 {
		return DiscussionData{}, false
	}

	// Keep only the user's comments when they participated in someone else's
	// thread so persona synthesis does not attribute the original post to them.
	discussion.Body = ""
	return discussion, true
}

func (c *Crawler) fetchProjects(ctx context.Context, username string) []ProjectData {
	var query struct {
		User struct {
			ProjectsV2 struct {
				Nodes []struct {
					Title            string
					ShortDescription string
					URL              string
					Public           bool
					CreatedAt        time.Time
					Items            struct {
						TotalCount int
					}
				}
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"projectsV2(first: 20, after: $cursor)"`
		} `graphql:"user(login: $login)"`
	}

	variables := map[string]interface{}{
		"login":  githubv4.String(username),
		"cursor": (*githubv4.String)(nil),
	}

	var result []ProjectData
	for {
		err := c.gqlPool.Next().Query(ctx, &query, variables)
		if err != nil {
			slog.Debug("could not fetch projects", "username", username, "error", err)
			return result
		}
		for _, p := range query.User.ProjectsV2.Nodes {
			result = append(result, ProjectData{
				Title:     p.Title,
				Body:      truncate(p.ShortDescription, 2000),
				URL:       p.URL,
				Public:    p.Public,
				CreatedAt: p.CreatedAt,
				ItemCount: p.Items.TotalCount,
			})
		}
		if !query.User.ProjectsV2.PageInfo.HasNextPage {
			break
		}
		cursor := githubv4.String(query.User.ProjectsV2.PageInfo.EndCursor)
		variables["cursor"] = &cursor
	}
	return result
}

func splitOwnerRepo(fullName string) []string {
	for i, c := range fullName {
		if c == '/' {
			if i > 0 && i < len(fullName)-1 {
				return []string{fullName[:i], fullName[i+1:]}
			}
			return nil
		}
	}
	return nil
}
