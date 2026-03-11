package ghcrawl

import "testing"

func TestSplitOwnerRepo(t *testing.T) {
	tests := []struct {
		input     string
		wantOwner string
		wantRepo  string
		wantNil   bool
	}{
		{"octocat/hello-world", "octocat", "hello-world", false},
		{"org/repo", "org", "repo", false},
		{"noslash", "", "", true},
		{"/leading", "", "", true},
		{"trailing/", "", "", true},
		{"", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitOwnerRepo(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("splitOwnerRepo(%q) = %v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("splitOwnerRepo(%q) = nil, want [%q, %q]", tt.input, tt.wantOwner, tt.wantRepo)
			}
			if got[0] != tt.wantOwner || got[1] != tt.wantRepo {
				t.Errorf("splitOwnerRepo(%q) = [%q, %q], want [%q, %q]",
					tt.input, got[0], got[1], tt.wantOwner, tt.wantRepo)
			}
		})
	}
}

func TestGraphQLPoolEmptyTokensFallsBackToAnonymousClient(t *testing.T) {
	pool := NewGraphQLPool(nil)
	if client := pool.Next(); client == nil {
		t.Fatal("Next() returned nil GraphQL client")
	}
}

func TestFilterDiscussionForUser(t *testing.T) {
	t.Run("drops unrelated discussion", func(t *testing.T) {
		discussion := DiscussionData{
			Author: "alice",
			Comments: []Comment{
				{Author: "bob", Body: "not mine"},
			},
		}

		_, ok := filterDiscussionForUser("carol", discussion)
		if ok {
			t.Fatal("expected unrelated discussion to be dropped")
		}
	})

	t.Run("keeps authored discussion and only authored comments", func(t *testing.T) {
		discussion := DiscussionData{
			Author: "alice",
			Body:   "proposal",
			Comments: []Comment{
				{Author: "alice", Body: "follow-up"},
				{Author: "bob", Body: "other comment"},
			},
		}

		got, ok := filterDiscussionForUser("alice", discussion)
		if !ok {
			t.Fatal("expected authored discussion to be kept")
		}
		if got.Body != "proposal" {
			t.Fatalf("expected original body to remain, got %q", got.Body)
		}
		if len(got.Comments) != 1 || got.Comments[0].Author != "alice" {
			t.Fatalf("expected only authored comments, got %+v", got.Comments)
		}
	})

	t.Run("keeps participated discussion without foreign body", func(t *testing.T) {
		discussion := DiscussionData{
			Author: "alice",
			Body:   "foreign post",
			Comments: []Comment{
				{Author: "bob", Body: "other comment"},
				{Author: "carol", Body: "my comment"},
			},
		}

		got, ok := filterDiscussionForUser("carol", discussion)
		if !ok {
			t.Fatal("expected participated discussion to be kept")
		}
		if got.Body != "" {
			t.Fatalf("expected foreign discussion body to be cleared, got %q", got.Body)
		}
		if len(got.Comments) != 1 || got.Comments[0].Author != "carol" {
			t.Fatalf("expected only authored comments, got %+v", got.Comments)
		}
	})
}
