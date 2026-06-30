package e2e

import (
	"net/http"
	"testing"

	ghtransport "github.com/bored-engineer/github-conditional-http-transport"
	"github.com/bored-engineer/github-conditional-http-transport/memory"
	"github.com/google/go-github/v81/github"
)

// TestIssuesSpeculativeEmptyArray verifies the speculative `[]` ETag guess (added when there is no
// cached response) results in a 304 Not Modified, by filtering on a label that will never match a
// valid issue on this repository.
func TestIssuesSpeculativeEmptyArray(t *testing.T) {
	storage := &testStorage{memory.NewStorage()}

	client := github.NewClient(&http.Client{
		Transport: &testTransport{
			Authorization: []string{"Bearer " + testEnv(t, "GH_TOKEN_ALPHA")},
			Parent:        ghtransport.NewTransport(storage, nil),
		},
	})

	// Verify that the cache is empty
	if keys := storage.Keys(); len(keys) != 0 {
		t.Fatalf("storage should have no keys, got %v", keys)
	}

	issues, resp, err := client.Issues.ListByRepo(t.Context(), "bored-engineer", "github-conditional-http-transport", &github.IssueListByRepoOptions{
		Labels: []string{"this-label-will-never-match-any-valid-issue-zzzzzzzz"},
	})
	if err != nil {
		t.Fatalf("(*github.IssuesService).ListByRepo failed: %v", err)
	}

	// The speculative `[]` ETag guess should have matched, resulting in a 304 upstream
	// that gets translated back into a 200 OK with an empty body by the transport.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("response status code should be %d, got %d", http.StatusOK, resp.StatusCode)
	}
	if len(issues) != 0 {
		t.Fatalf("issues should be empty, got %d", len(issues))
	}

	// Since the upstream returned a 304, nothing should have been written to storage
	if keys := storage.Keys(); len(keys) != 0 {
		t.Fatalf("storage should have no keys, got %v", keys)
	}
}
