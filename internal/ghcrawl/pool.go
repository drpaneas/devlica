package ghcrawl

import (
	"sync/atomic"

	"github.com/google/go-github/v68/github"
)

// TokenPool distributes GitHub API requests across multiple authenticated
// clients using round-robin selection.
type TokenPool struct {
	clients []*github.Client
	counter atomic.Uint64
}

// NewTokenPool creates a pool of GitHub REST clients, one per token.
func NewTokenPool(tokens []string) *TokenPool {
	if len(tokens) == 0 {
		return &TokenPool{clients: []*github.Client{newGitHubClient("")}}
	}
	clients := make([]*github.Client, len(tokens))
	for i, tok := range tokens {
		clients[i] = newGitHubClient(tok)
	}
	return &TokenPool{clients: clients}
}

// Next returns the next client in round-robin order.
func (p *TokenPool) Next() *github.Client {
	if len(p.clients) == 0 {
		return newGitHubClient("")
	}
	idx := p.counter.Add(1) - 1
	return p.clients[idx%uint64(len(p.clients))]
}

// Size returns the number of tokens in the pool.
func (p *TokenPool) Size() int {
	return len(p.clients)
}
