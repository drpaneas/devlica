package ghcrawl

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v68/github"
)

// timeWindow represents a date range for a search query.
type timeWindow struct {
	from time.Time
	to   time.Time
}

// qualifier returns a GitHub search date qualifier, e.g.
// "created:2024-01-01..2024-01-31" or "updated:2024-01-01..2024-01-31".
func (w timeWindow) qualifier(field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		field = "created"
	}
	return fmt.Sprintf("%s:%s..%s",
		field,
		w.from.Format("2006-01-02"),
		w.to.Format("2006-01-02"))
}

// monthlyWindows splits [from, to] into monthly intervals.
func monthlyWindows(from, to time.Time) []timeWindow {
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	to = time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	if !from.Before(to) {
		return []timeWindow{{from: from, to: to}}
	}

	var windows []timeWindow
	cursor := from
	for cursor.Before(to) {
		nextMonthStart := time.Date(cursor.Year(), cursor.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		end := nextMonthStart.AddDate(0, 0, -1)
		if end.After(to) {
			end = to
		}
		windows = append(windows, timeWindow{from: cursor, to: end})
		cursor = end.AddDate(0, 0, 1)
	}
	return windows
}

// windowedSearchIssues runs the given base search query across monthly time
// windows in parallel, using the token pool for concurrency. Results are
// deduplicated by issue ID.
func (c *Crawler) windowedSearchIssues(ctx context.Context, baseQuery string, since time.Time) ([]*github.Issue, error) {
	return c.windowedSearchIssuesWithQualifier(ctx, baseQuery, since, "created")
}

func (c *Crawler) windowedSearchIssuesWithQualifier(ctx context.Context, baseQuery string, since time.Time, qualifierField string) ([]*github.Issue, error) {
	windows := monthlyWindows(since, time.Now())

	type result struct {
		issues []*github.Issue
		err    error
	}

	results := make([]result, len(windows))
	var wg sync.WaitGroup
	semSize := c.pool.Size()
	if semSize < 1 {
		semSize = 1
	}
	sem := make(chan struct{}, semSize)

	for i, w := range windows {
		wg.Add(1)
		go func(idx int, win timeWindow) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			query := baseQuery + " " + win.qualifier(qualifierField)
			var issues []*github.Issue
			opts := &github.SearchOptions{
				Sort:        "created",
				Order:       "asc",
				ListOptions: github.ListOptions{PerPage: 100},
			}
			for {
				res, resp, err := c.pool.Next().Search.Issues(ctx, query, opts)
				if err != nil {
					results[idx] = result{err: err}
					return
				}
				issues = append(issues, res.Issues...)
				if resp.NextPage == 0 {
					break
				}
				opts.Page = resp.NextPage
			}
			results[idx] = result{issues: issues}
		}(i, w)
	}
	wg.Wait()

	seen := make(map[int64]bool)
	var all []*github.Issue
	for _, r := range results {
		if r.err != nil {
			return all, r.err
		}
		for _, issue := range r.issues {
			id := issue.GetID()
			if !seen[id] {
				seen[id] = true
				all = append(all, issue)
			}
		}
	}
	return all, nil
}
