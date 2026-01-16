package e2e

import (
	"errors"
	"log"
	"net/http"
	"os"
	"sort"
	"testing"

	ghtransport "github.com/bored-engineer/github-conditional-http-transport"
	"github.com/bored-engineer/github-conditional-http-transport/memory"
	"github.com/google/go-github/v81/github"
)

// testEnv retrieves the value of the given environment variable and skips the test if it is not set.
func testEnv(t *testing.T, key string) string {
	value := os.Getenv(key)
	if value == "" {
		t.Skipf("environment variable %s is not set, skipping test", key)
	}
	return value
}

// testStorage extends the memory.Storage adding methods for listing the keys in the storage.
type testStorage struct {
	*memory.Storage
}

func (s *testStorage) Keys() (keys []string) {
	s.Storage.Map.Range(func(key, value interface{}) bool {
		keys = append(keys, key.(string))
		return true
	})
	sort.Strings(keys)
	return
}

// testTransport is a http.RoundTripper that adds an Authorization header to the request.
type testTransport struct {
	Authorization []string
	Parent        http.RoundTripper
}

// RoundTrip implements the http.RoundTripper interface.
func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(t.Authorization) == 0 {
		return nil, errors.New("no more Authorization headers")
	}
	req.Header.Set("Authorization", t.Authorization[0])
	t.Authorization = t.Authorization[1:]
	return t.Parent.RoundTrip(req)
}

// testE2E performs an E2E test with the given authorization headers.
func testE2E(t *testing.T, authorization ...string) {
	storage := &testStorage{memory.NewStorage()}

	client := github.NewClient(&http.Client{
		Transport: &testTransport{
			// We use the first token to populate the cache so we include it twice
			Authorization: append([]string{authorization[0]}, authorization...),
			Parent:        ghtransport.NewTransport(storage, nil),
		},
	})

	// Verify that the cache is empty
	if keys := storage.Keys(); len(keys) != 0 {
		t.Fatalf("storage should have no keys, got %v", keys)
	}

	var cachedRequestID string
	for idx := range len(authorization) + 1 {
		// Perform the request
		user, resp, err := client.Users.Get(t.Context(), "bored-engineer")
		if err != nil {
			log.Fatalf("(*github.Client).Users.Get failed: %v", err)
		}

		// Verify we got an API response
		if user.GetLogin() != "bored-engineer" {
			t.Fatalf("user should be bored-engineer, got %q", user.GetLogin())
		}

		// Verify that every cached request has the original X-Github-Request-Id header
		if idx == 0 {
			cachedRequestID = resp.Header.Get("X-Github-Request-Id")
		} else {
			if resp.Header.Get(ghtransport.CachedRequestIDHeader) != cachedRequestID {
				t.Fatalf("%s header (%q) should be %q", ghtransport.CachedRequestIDHeader, resp.Header.Get(ghtransport.CachedRequestIDHeader), cachedRequestID)
			}
		}

		// Verify that the storage has the expected number of keys
		if keys := storage.Keys(); len(keys) != 1 {
			t.Fatalf("storage should have 1 key, got %v", keys)
		}
	}
}
