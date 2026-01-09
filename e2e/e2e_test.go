package ghtransport

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"testing"

	ghtransport "github.com/bored-engineer/github-conditional-http-transport"
	memory "github.com/bored-engineer/github-conditional-http-transport/memory"
	"github.com/google/go-github/v81/github"
	oauth2githubapp "github.com/int128/oauth2-github-app"
)

// testTransport is a http.RoundTripper that adds an Authorization header to the request.
type testTransport struct {
	Authorization []string
	Offset        int
	Parent        http.RoundTripper
}

// RoundTrip implements the http.RoundTripper interface.
func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", t.Authorization[t.Offset])
	t.Offset++
	return t.Parent.RoundTrip(req)
}

// appToken fetches an access token for the given GitHub App.
func appToken(idx int) string {
	privateKey, err := oauth2githubapp.LoadPrivateKey(os.Getenv("GH_APP_PRIVATE_KEY_" + strconv.Itoa(idx)))
	if err != nil {
		panic(fmt.Errorf("oauth2githubapp.LoadPrivateKey failed: %w", err))
	}
	app := &oauth2githubapp.Config{
		PrivateKey:     privateKey,
		AppID:          os.Getenv("GH_APP_CLIENT_ID_" + strconv.Itoa(idx)),
		InstallationID: os.Getenv("GH_APP_INSTALLATION_ID_" + strconv.Itoa(idx)),
	}
	token, err := app.Token(context.Background())
	if err != nil {
		panic(fmt.Errorf("(*oauth2githubapp.Config).Token failed: %w", err))
	}
	return token.AccessToken
}

func TestE2E(t *testing.T) {

	// Retrieve different access tokens for the each app
	appToken1, appToken2, appToken3 := appToken(1), appToken(2), appToken(3)

	tests := map[string]*testTransport{
		"PAT": {
			Authorization: []string{
				"Bearer " + os.Getenv("GH_TOKEN_1"),
				"Bearer " + os.Getenv("GH_TOKEN_1"),
				"Bearer " + os.Getenv("GH_TOKEN_2"),
				"Bearer " + os.Getenv("GH_TOKEN_3"),
			},
		},
		"Basic": {
			Authorization: []string{
				"Basic " + base64.StdEncoding.EncodeToString([]byte(os.Getenv("GH_CLIENT_ID_1")+":"+os.Getenv("GH_CLIENT_SECRET_1"))),
				"Basic " + base64.StdEncoding.EncodeToString([]byte(os.Getenv("GH_CLIENT_ID_1")+":"+os.Getenv("GH_CLIENT_SECRET_1"))),
				"Basic " + base64.StdEncoding.EncodeToString([]byte(os.Getenv("GH_CLIENT_ID_2")+":"+os.Getenv("GH_CLIENT_SECRET_2"))),
				"Basic " + base64.StdEncoding.EncodeToString([]byte(os.Getenv("GH_CLIENT_ID_3")+":"+os.Getenv("GH_CLIENT_SECRET_3"))),
			},
		},
		"App": {
			Authorization: []string{
				"Bearer " + appToken1,
				"Bearer " + appToken1,
				"Bearer " + appToken2,
				"Bearer " + appToken3,
			},
		},
	}

	for name, transport := range tests {
		t.Run(name, func(t *testing.T) {
			storage := memory.NewStorage()

			transport.Parent = ghtransport.NewTransport(storage, nil)
			client := github.NewClient(&http.Client{
				Transport: transport,
			})

			var cachedRequestID string
			for idx := range len(transport.Authorization) {
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
			}
		})
	}
}
