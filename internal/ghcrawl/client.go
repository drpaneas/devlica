package ghcrawl

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"
)

func newGitHubClient(token string) *github.Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := &http.Client{
		Transport: &rateLimitTransport{
			base: &oauth2.Transport{
				Source: ts,
				Base:   http.DefaultTransport,
			},
		},
		Timeout: 30 * time.Second,
	}
	return github.NewClient(httpClient)
}

// rateLimitTransport wraps an http.RoundTripper and pauses when rate-limited.
type rateLimitTransport struct {
	base http.RoundTripper
}

const maxRetries = 3

func (t *rateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := range maxRetries {
		resp, err = t.base.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		isRateLimited := resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests

		// Proactively pause when approaching rate limit, but only if the
		// current response is not already rate-limited (avoids double-sleep).
		if !isRateLimited {
			if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining != "" {
				rem, parseErr := strconv.Atoi(remaining)
				if parseErr == nil && rem <= 10 {
					resetStr := resp.Header.Get("X-RateLimit-Reset")
					resetUnix, parseErr := strconv.ParseInt(resetStr, 10, 64)
					if parseErr == nil {
						wait := time.Until(time.Unix(resetUnix, 0))
						if wait > 0 && wait < 15*time.Minute {
							slog.Warn("approaching github rate limit, pausing",
								"remaining", rem, "wait", wait.Round(time.Second))
							if err := sleepContext(req.Context(), wait+time.Second); err != nil {
								resp.Body.Close()
								return nil, err
							}
						}
					}
				}
			}
			return resp, nil
		}

		retryAfter := resp.Header.Get("Retry-After")
		secs, parseErr := strconv.Atoi(retryAfter)
		if parseErr != nil || secs <= 0 || secs >= 900 {
			return resp, nil
		}

		slog.Warn("rate limited, retrying", "retry_after", secs, "attempt", attempt+1)
		resp.Body.Close()
		if err := sleepContext(req.Context(), time.Duration(secs)*time.Second); err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("github rate limit: retries exhausted after %d attempts", maxRetries)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
