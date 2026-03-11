package ghcrawl

import (
	"sync"
	"testing"

	"github.com/google/go-github/v68/github"
)

func TestTokenPoolRoundRobin(t *testing.T) {
	pool := &TokenPool{}
	pool.clients = make([]*github.Client, 3)
	for i := range pool.clients {
		pool.clients[i] = &github.Client{}
	}

	seen := make(map[*github.Client]int)
	for i := 0; i < 9; i++ {
		c := pool.Next()
		seen[c]++
	}
	for client, count := range seen {
		if count != 3 {
			t.Errorf("client %p was used %d times, want 3", client, count)
		}
	}
}

func TestTokenPoolSize(t *testing.T) {
	pool := &TokenPool{}
	pool.clients = make([]*github.Client, 5)
	if got := pool.Size(); got != 5 {
		t.Errorf("Size() = %d, want 5", got)
	}
}

func TestTokenPoolConcurrentAccess(t *testing.T) {
	pool := &TokenPool{}
	pool.clients = make([]*github.Client, 3)
	for i := range pool.clients {
		pool.clients[i] = &github.Client{}
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := pool.Next()
			if c == nil {
				t.Error("got nil client")
			}
		}()
	}
	wg.Wait()
}

func TestTokenPoolEmptyTokensFallsBackToAnonymousClient(t *testing.T) {
	pool := NewTokenPool(nil)
	if got := pool.Size(); got != 1 {
		t.Fatalf("Size() = %d, want 1", got)
	}
	if client := pool.Next(); client == nil {
		t.Fatal("Next() returned nil client")
	}
}
